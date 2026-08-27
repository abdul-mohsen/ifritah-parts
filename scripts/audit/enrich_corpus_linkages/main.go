// Command enrich_corpus_linkages augments an audit corpus CSV with the
// LinkageTargetIds + SeedArticleIds it needs so that per-strategy audits
// of `vehicle_fitment` and `spec_match` can hit the strategy path
// end-to-end without the caller having to know a linkage id up front.
//
// Milestone: M0.T4 sub-B (the second half of PR #29). The first half —
// case-insensitive /api/catalog/vehicles — landed on the
// fix/m0-t4-catalog-vehicles-case-insensitive branch (PR #29). This
// tool is deliberately standalone so it can be exercised in CI or
// against a local TecDoc dump without wiring into the server binary.
//
// Input : scripts/audit/corpus-1500-v2.csv (or any CSV with an "OEM"
//
//	column).
//
// Output: same rows + LinkageTargetIds + SeedArticleIds columns.
//
// Query : two joins per OEM against the TecDoc MySQL corpus —
//
//	(a) oem_number → articles → articlesvehicletrees → linkage id
//	(b) oem_number → articles → seed article id (top by dataSupplierId).
//	Both are LIMITed; the top-K ids are joined with '|' for CSV.
//
// Usage (from a repo root that has a MySQL connection):
//
//	go run ./scripts/audit/enrich_corpus_linkages \
//	    -corpus  scripts/audit/corpus-1500-v2.csv \
//	    -out     scripts/audit/corpus-1500-v2.enriched.csv \
//	    -mysql   "user:pass@tcp(host:3306)/tecdoc?parseTime=true" \
//	    -k       5
//
// When -mysql is empty the tool runs in shape-check mode: it re-reads
// the corpus, appends empty LinkageTargetIds/SeedArticleIds columns,
// and exits 0. That lets CI validate the CSV layout without needing
// TecDoc credentials.
//
// Exit codes:
//
//	0  success (rows written, summary on stderr)
//	1  input error (missing corpus, malformed header, unreadable file)
//	2  database error (connection failed, query failed on all rows)
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// linkageQuery reads distinct linkage-target ids for one OEM by
	// walking oem_number → articles → articlesvehicletrees. Filtered
	// to type 'P' (Passenger) — that is the linkage shape the runtime
	// PartsForVehicle strategy consumes.
	linkageQuery = `
		SELECT DISTINCT avt.linkingTargetId
		FROM oem_number oe
		JOIN articles a ON a.legacyArticleId = oe.articleId
		JOIN articlesvehicletrees avt ON avt.legacyArticleId = a.legacyArticleId
		WHERE oe.clean_number = ?
		  AND avt.linkingTargetType = 'P'
		LIMIT ?`

	// seedQuery returns candidate seed article ids for the same OEM.
	// Sorted by dataSupplierId descending — a rough proxy for
	// "canonical" (the newest supplier catalogs typically win). We
	// keep the top-K for downstream spec_match audits.
	seedQuery = `
		SELECT a.legacyArticleId
		FROM oem_number oe
		JOIN articles a ON a.legacyArticleId = oe.articleId
		WHERE oe.clean_number = ?
		ORDER BY a.dataSupplierId DESC
		LIMIT ?`

	// Column name we mutate NormalizeOEM on (before the SQL lookup):
	// TecDoc's oem_number.clean_number is stored uppercase with all
	// non-alphanumeric characters stripped. The corpus already stores
	// the pretty form ("26350-2J001") so we normalize on our end.
	oemColumnName             = "OEM"
	newLinkageTargetIdsColumn = "LinkageTargetIds"
	newSeedArticleIdsColumn   = "SeedArticleIds"

	// perOEMTimeout gates each pair of queries. TecDoc's oem_number
	// table has 21.5M rows; a bad plan on one OEM must not stall the
	// whole run.
	perOEMTimeout = 15 * time.Second
)

func main() {
	corpusPath := flag.String("corpus", "", "path to input corpus CSV (required)")
	outPath := flag.String("out", "", "path to write enriched corpus CSV (required; use '-' for stdout)")
	mysqlDSN := flag.String("mysql", "", "TecDoc MySQL DSN (empty = shape-check mode, no queries)")
	k := flag.Int("k", 5, "how many linkage ids + seed article ids to record per OEM")
	skipHeader := flag.Bool("skip-header", false, "when true, treat the input as headerless and synthesize an OEM-only header")
	sepFlag := flag.String("id-sep", "|", "separator used inside the two new columns (default '|' — CSV-safe)")
	flag.Parse()

	if *corpusPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "-corpus and -out are required")
		flag.Usage()
		os.Exit(1)
	}
	if *k <= 0 || *k > 50 {
		fmt.Fprintf(os.Stderr, "-k must be in [1,50], got %d\n", *k)
		os.Exit(1)
	}

	in, err := os.Open(*corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer in.Close()

	reader := csv.NewReader(in)
	reader.FieldsPerRecord = -1 // allow variable-width rows on edge corpuses
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read header: %v\n", err)
		os.Exit(1)
	}

	if *skipHeader {
		// The row we just consumed as the "header" is actually data.
		// Rewind by seeking back to 0 and reading again with a
		// synthesized header. Cheaper alternative — re-open the file.
		if _, err := in.Seek(0, io.SeekStart); err != nil {
			fmt.Fprintf(os.Stderr, "rewind input: %v\n", err)
			os.Exit(1)
		}
		reader = csv.NewReader(in)
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		// Synthesize: assume column 0 is the OEM number, the rest are opaque.
		header = []string{oemColumnName}
		for i := 1; i < len(header); i++ {
			header = append(header, fmt.Sprintf("col%d", i))
		}
	}

	oemIdx := indexOf(header, oemColumnName)
	if oemIdx < 0 {
		fmt.Fprintf(os.Stderr, "input CSV missing required column %q; got header %v\n", oemColumnName, header)
		os.Exit(1)
	}

	// Refuse to silently overwrite existing enrichment columns —
	// callers usually mean to re-run against a fresh corpus.
	if indexOf(header, newLinkageTargetIdsColumn) >= 0 || indexOf(header, newSeedArticleIdsColumn) >= 0 {
		fmt.Fprintf(os.Stderr, "input CSV already contains %q or %q — refusing to overwrite; strip those columns first\n",
			newLinkageTargetIdsColumn, newSeedArticleIdsColumn)
		os.Exit(1)
	}

	var db *sql.DB
	if *mysqlDSN != "" {
		var err error
		db, err = sql.Open("mysql", *mysqlDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open mysql: %v\n", err)
			os.Exit(2)
		}
		defer db.Close()
		db.SetConnMaxLifetime(2 * time.Minute)
		db.SetMaxOpenConns(4)
		db.SetMaxIdleConns(2)

		if err := db.Ping(); err != nil {
			fmt.Fprintf(os.Stderr, "ping mysql: %v\n", err)
			os.Exit(2)
		}
	} else {
		log.Println("shape-check mode: no -mysql DSN provided; new columns will be written empty on every row")
	}

	// Prepare output writer.
	var outWriter io.Writer
	if *outPath == "-" {
		outWriter = os.Stdout
	} else {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		outWriter = f
	}
	w := csv.NewWriter(outWriter)
	defer w.Flush()

	// Emit augmented header.
	outHeader := append(append([]string{}, header...), newLinkageTargetIdsColumn, newSeedArticleIdsColumn)
	if err := w.Write(outHeader); err != nil {
		fmt.Fprintf(os.Stderr, "write header: %v\n", err)
		os.Exit(1)
	}

	var (
		total       int
		enriched    int
		noLinkage   int
		queryErrors int
	)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Malformed row — skip but count. Emit the row untouched
			// with empty enrichment columns so downstream slicing
			// stays stable.
			log.Printf("skip malformed row (line ~%d): %v", total+2, err)
			continue
		}
		total++

		// Some rows come in short — pad to header length before append.
		if len(row) < len(header) {
			pad := make([]string, len(header)-len(row))
			row = append(row, pad...)
		}

		linkageIDs := ""
		seedIDs := ""
		if db != nil && oemIdx < len(row) {
			oem := strings.TrimSpace(row[oemIdx])
			if oem != "" {
				clean := normalizeOEM(oem)
				ctx, cancel := context.WithTimeout(context.Background(), perOEMTimeout)

				lids, lerr := scanInts(ctx, db, linkageQuery, clean, *k)
				sids, serr := scanInts(ctx, db, seedQuery, clean, *k)
				cancel()

				switch {
				case lerr != nil && !errors.Is(lerr, sql.ErrNoRows):
					log.Printf("linkage query error oem=%q clean=%q err=%v", oem, clean, lerr)
					queryErrors++
				case serr != nil && !errors.Is(serr, sql.ErrNoRows):
					log.Printf("seed query error oem=%q clean=%q err=%v", oem, clean, serr)
					queryErrors++
				}

				if len(lids) > 0 {
					linkageIDs = joinInts(lids, *sepFlag)
					enriched++
				} else if lerr == nil {
					noLinkage++
				}
				if len(sids) > 0 {
					seedIDs = joinInts(sids, *sepFlag)
				}
			}
		}

		outRow := append(append([]string{}, row...), linkageIDs, seedIDs)
		if err := w.Write(outRow); err != nil {
			fmt.Fprintf(os.Stderr, "write row: %v\n", err)
			os.Exit(1)
		}
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "enrich_corpus_linkages: total=%d enriched=%d no_linkage=%d errors=%d\n",
		total, enriched, noLinkage, queryErrors)
}

// indexOf returns the position of name in header, or -1 when absent.
// Comparison is exact (no case-fold, no trim) so the caller controls
// the shape.
func indexOf(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

// normalizeOEM matches TecDoc's oem_number.clean_number stored form:
// uppercase, non-alphanumeric characters stripped. The runtime uses
// the same rule (internal/service/tecdoc.go:NormalizeOEM).
func normalizeOEM(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b = append(b, c)
		}
	}
	return string(b)
}

// scanInts runs one query with (cleanOEM, limit) parameters and
// materialises the int rows. Returns nil (no error) when the query
// yields zero rows so callers can differentiate "not found" from a
// real DB failure via the error return.
func scanInts(ctx context.Context, db *sql.DB, query, cleanOEM string, limit int) ([]int, error) {
	rows, err := db.QueryContext(ctx, query, cleanOEM, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id > 0 {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// joinInts formats a slice of ints using the given separator. Kept as
// a local helper to avoid pulling in strconv+strings.Builder wiring at
// every call site.
func joinInts(xs []int, sep string) string {
	if len(xs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, x := range xs {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(strconv.Itoa(x))
	}
	return b.String()
}
