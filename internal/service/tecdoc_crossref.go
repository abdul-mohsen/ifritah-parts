package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"parts-engine/internal/model"
)

// crossRefRepo is the injectable DB dependency for TecDocCrossRef.
// The production implementation runs the SQL query against articlecrosses
// (30M rows). Tests inject a stub that returns fixture rows without touching
// a live MySQL database. Matching the supersession.go pattern.
type crossRefRepo interface {
	QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error)
	// S2-T4 batched form: single IN(...) query for N OEM numbers.
	// Returns cross-ref rows tagged with the input OEM so the caller can map
	// back to the seed article without a per-item lookup.
	QueryCrossRefsBatch(ctx context.Context, cleanOEMs []string, limitPerOEM int) (map[string][]crossRefRow, error)
}

// crossRefRow is one row from the articlecrosses join. Kept flat so a stub
// can construct it without importing sql.NullString semantics.
type crossRefRow struct {
	RawCrossNumber          string
	MfrName                 string
	LegacyArticleId         int
	ArticleNumber           string
	Description             string
	BrandName               string
	OriginalOEMManufacturer string
}

// TecDocCrossRef surfaces the TecDoc articlecrosses table (30M cross-ref rows)
// as structured OEM references. This is distinct from TecDoc.SearchByOEM in
// tecdoc.go: SearchByOEM walks the compact oem_number/oem_search_index tables;
// SearchCrossReferences walks the authoritative cross-reference table itself
// so the mfrName provenance survives into the response.
type TecDocCrossRef struct {
	repo crossRefRepo
}

// NewTecDocCrossRef wires the service against a real *sql.DB. Returns a
// zero-value TecDocCrossRef when db is nil so callers can still construct
// the service in offline mode; calls then return a "database not connected"
// error without panicking.
func NewTecDocCrossRef(db *sql.DB) *TecDocCrossRef {
	if db == nil {
		return &TecDocCrossRef{}
	}
	return &TecDocCrossRef{repo: &sqlCrossRefRepo{db: db}}
}

// SearchCrossReferences finds articles whose articlecrosses row carries the
// supplied OEM number. Normalizes the input with NormalizeOEM before the
// query so callers can pass raw OEM strings with dashes or whitespace.
//
// limit is clamped to [1, 100]. An empty OEM string returns an error rather
// than issuing a wildcard query — a 30M-row scan without a WHERE key would
// exhaust the connection budget.
func (s *TecDocCrossRef) SearchCrossReferences(oemNumber string, limit int) ([]model.OEMReference, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if strings.TrimSpace(oemNumber) == "" {
		return nil, fmt.Errorf("empty OEM number")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		return nil, fmt.Errorf("empty OEM number")
	}

	ctx := context.Background()
	rows, err := s.repo.QueryCrossRefs(ctx, clean, limit)
	if err != nil {
		return nil, fmt.Errorf("search cross references: %w", err)
	}

	refs := make([]model.OEMReference, 0, len(rows))
	seen := make(map[int]bool, len(rows))
	for _, r := range rows {
		if r.LegacyArticleId != 0 && seen[r.LegacyArticleId] {
			continue
		}
		if r.LegacyArticleId != 0 {
			seen[r.LegacyArticleId] = true
		}
		refs = append(refs, model.OEMReference{
			RawNumber:       r.RawCrossNumber,
			Normalized:      clean,
			Manufacturer:    firstNonEmpty(r.OriginalOEMManufacturer, r.MfrName),
			BrandName:       r.BrandName,
			ArticleNumber:   r.ArticleNumber,
			Description:     r.Description,
			LegacyArticleId: r.LegacyArticleId,
		})
	}
	return refs, nil
}

// SearchCrossReferencesBatch is the batched form of SearchCrossReferences.
// Given N OEM numbers, it runs a single `articlecrosses.cleanCrossNumber IN (?)`
// query and returns a map keyed by NORMALISED OEM (input passed through
// NormalizeOEM). Empty or duplicate OEM inputs are dropped.
//
// This is the S2-T4 batched path used by searchByVehicle enrichment.
// Where SearchCrossReferences would issue one query per vehicle result,
// this issues one query for all of them combined.
func (s *TecDocCrossRef) SearchCrossReferencesBatch(oemNumbers []string, limitPerOEM int) (map[string][]model.OEMReference, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if len(oemNumbers) == 0 {
		return nil, nil
	}
	if limitPerOEM <= 0 || limitPerOEM > 100 {
		limitPerOEM = 20
	}
	cleaned := make([]string, 0, len(oemNumbers))
	for _, o := range oemNumbers {
		c := NormalizeOEM(o)
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	rowsByOEM, err := s.repo.QueryCrossRefsBatch(context.Background(), cleaned, limitPerOEM)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]model.OEMReference, len(rowsByOEM))
	for seed, rows := range rowsByOEM {
		refs := make([]model.OEMReference, 0, len(rows))
		for _, r := range rows {
			refs = append(refs, model.OEMReference{
				RawNumber:       r.RawCrossNumber,
				Normalized:      seed,
				LegacyArticleId: r.LegacyArticleId,
				Manufacturer:    firstNonEmpty(r.OriginalOEMManufacturer, r.MfrName),
				BrandName:       r.BrandName,
				ArticleNumber:   r.ArticleNumber,
				Description:     r.Description,
			})
		}
		out[seed] = refs
	}
	return out, nil
}

// sqlCrossRefRepo is the production repo bound to a MySQL *sql.DB.
// It runs the articlecrosses query with LEFT JOIN so parts missing from
// the local articles view still surface with an empty article number
// (the raw crossOemNumber alone is still evidence).
//
// Performance path (see sql/06_articlecrosses_normalized_oem_index.sql):
//
//	When the generated column `articlecrosses.oemNumberNormalized` (+ its
//	index `idx_articlecrosses_oemNumberNormalized`) exists, the WHERE clause
//	uses that indexed column → O(log n) lookup, sub-10ms.
//
//	When the column doesn't exist (pre-migration deploys), the repo falls
//	back to the correctness-preserving `LOWER(REPLACE(REPLACE(...)))` form
//	that scans the full 30M-row table. This path also produces the correct
//	results but takes 3-8 HOURS per query — the 15s Go ctx deadline fires
//	long before the query finishes. See docs/reports/2026-08-19-post-pr14-
//	data-quality.md §5 for the debug-log evidence.
//
// The `hasNormalizedColumn` flag is probed once at first query and cached.
// A restart is required to pick up a column that appears while the process
// is running — acceptable because the DDL is a deploy-time operation.
type sqlCrossRefRepo struct {
	db *sql.DB

	// probeOnce ensures the information_schema check runs at most once
	// per process lifetime, regardless of concurrent callers.
	probeOnce sync.Once
	// hasNormalizedColumn is set by probeOnce. When true, queries use the
	// fast index; when false, queries fall back to the slow scan and log
	// a WARN so ops notices the migration hasn't been applied.
	hasNormalizedColumn bool
}

// probeGeneratedColumn detects whether the deployed MySQL instance has
// the `articlecrosses.oemNumberNormalized` generated column created by
// sql/06_articlecrosses_normalized_oem_index.sql. Runs once per process
// and caches the result in r.hasNormalizedColumn.
//
// Uses a 2s context so a slow information_schema query on startup can't
// hang the repo — a missing/unreachable schema simply falls back to the
// slow path.
func (r *sqlCrossRefRepo) probeGeneratedColumn() {
	r.probeOnce.Do(func() {
		if r.db == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var n int
		// COLUMN_NAME check works across MySQL 5.7 + 8.0; DATABASE() scopes
		// to the current schema so we don't collide with tests / dev DBs.
		err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE()
			  AND TABLE_NAME   = 'articlecrosses'
			  AND COLUMN_NAME  = 'oemNumberNormalized'`).Scan(&n)
		if err != nil {
			log.Printf("[TecDocCrossRef] WARN: probeGeneratedColumn failed (%v) — falling back to slow full-scan query. Run sql/06_articlecrosses_normalized_oem_index.sql to enable the index.", err)
			return
		}
		if n > 0 {
			r.hasNormalizedColumn = true
			log.Printf("[TecDocCrossRef] fast path enabled — articlecrosses.oemNumberNormalized is present + indexed")
		} else {
			log.Printf("[TecDocCrossRef] WARN: articlecrosses.oemNumberNormalized column is missing — falling back to slow full-scan query. Run sql/06_articlecrosses_normalized_oem_index.sql to enable the index (expected: 3-8 hours per query → <10ms per query).")
		}
	})
}

// crossRefSelectClause is the common SELECT + JOIN prefix used by both
// QueryCrossRefs and QueryCrossRefsBatch. Kept as a const so the WHERE
// clause is the only piece that varies between fast/slow paths.
const crossRefSelectClause = `
		SELECT
			ac.oemNumber,
			COALESCE(m.manuName, ''),
			COALESCE(a.legacyArticleId, 0),
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ac.brandName, ''),
			COALESCE(m.manuName, '')
		FROM articlecrosses ac
		LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
		LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'`

func (r *sqlCrossRefRepo) QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error) {
	r.probeGeneratedColumn()

	// FAST PATH: index on the generated column. Sub-10ms lookup on a
	// 30M-row table.
	//
	// SLOW PATH: correctness fallback for deploys that haven't applied
	// sql/06_articlecrosses_normalized_oem_index.sql yet. Same rows
	// returned, but MySQL disables the index on `oemNumber` because the
	// column is wrapped in LOWER(REPLACE(...)), forcing a full table
	// scan. Empirically 3-8 hours per query on qa.ifritah.com's dataset.
	var whereClause string
	if r.hasNormalizedColumn {
		whereClause = "WHERE ac.oemNumberNormalized = ?"
	} else {
		whereClause = "WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber, '-', ''), ' ', ''), '.', ''), '/', '')) = ?"
	}

	q := crossRefSelectClause + "\n\t\t" + whereClause + "\n\t\tLIMIT ?"

	rows, err := logQueryCtx(r.db, ctx, "TecDocCrossRef.SearchCrossReferences", q, cleanOEM, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []crossRefRow
	for rows.Next() {
		var row crossRefRow
		if err := rows.Scan(
			&row.RawCrossNumber,
			&row.MfrName,
			&row.LegacyArticleId,
			&row.ArticleNumber,
			&row.Description,
			&row.BrandName,
			&row.OriginalOEMManufacturer,
		); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

// QueryCrossRefsBatch runs a single IN(...) query across many OEM numbers.
// Returns a map keyed by the cleaned OEM so callers can associate rows back
// to their seed article without an N+1 loop. limitPerOEM is a soft cap that
// bounds the whole result set to len(cleanOEMs) * limitPerOEM rows.
//
// See QueryCrossRefs for the fast/slow path story — same optimisation
// applies here. Batch queries benefit even more from the indexed column
// because MySQL can seek per input value instead of scanning once for the
// whole IN list.
func (r *sqlCrossRefRepo) QueryCrossRefsBatch(ctx context.Context, cleanOEMs []string, limitPerOEM int) (map[string][]crossRefRow, error) {
	if len(cleanOEMs) == 0 {
		return nil, nil
	}
	r.probeGeneratedColumn()

	if limitPerOEM <= 0 || limitPerOEM > 100 {
		limitPerOEM = 20
	}
	// Deduplicate + build placeholders + args.
	seen := make(map[string]bool, len(cleanOEMs))
	uniq := make([]string, 0, len(cleanOEMs))
	for _, o := range cleanOEMs {
		if o == "" || seen[o] {
			continue
		}
		seen[o] = true
		uniq = append(uniq, o)
	}
	if len(uniq) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(uniq)-1) + "?"
	// Cap the whole result set — safe upper bound so a stray call with 1000
	// OEMs cannot pull hundreds of thousands of rows.
	totalLimit := limitPerOEM * len(uniq)
	if totalLimit > 2000 {
		totalLimit = 2000
	}

	// The seed-OEM column comes first so callers can map rows back to
	// their input. Fast path uses the indexed column directly; slow path
	// re-normalises inline.
	var seedCol, whereClause string
	if r.hasNormalizedColumn {
		seedCol = "ac.oemNumberNormalized"
		whereClause = fmt.Sprintf("WHERE ac.oemNumberNormalized IN (%s)", placeholders)
	} else {
		seedCol = "LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber, '-', ''), ' ', ''), '.', ''), '/', ''))"
		whereClause = fmt.Sprintf("WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber, '-', ''), ' ', ''), '.', ''), '/', '')) IN (%s)", placeholders)
	}

	q := fmt.Sprintf(`
		SELECT
			%s AS clean_oem,
			ac.oemNumber,
			COALESCE(m.manuName, ''),
			COALESCE(a.legacyArticleId, 0),
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ac.brandName, ''),
			COALESCE(m.manuName, '')
		FROM articlecrosses ac
		LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
		LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'
		%s
		LIMIT %d`, seedCol, whereClause, totalLimit)

	args := make([]any, 0, len(uniq))
	for _, o := range uniq {
		args = append(args, o)
	}

	rows, err := logQueryCtx(r.db, ctx, "TecDocCrossRef.SearchCrossReferencesBatch", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]crossRefRow, len(uniq))
	for rows.Next() {
		var seedOEM string
		var row crossRefRow
		if err := rows.Scan(
			&seedOEM,
			&row.RawCrossNumber,
			&row.MfrName,
			&row.LegacyArticleId,
			&row.ArticleNumber,
			&row.Description,
			&row.BrandName,
			&row.OriginalOEMManufacturer,
		); err != nil {
			continue
		}
		out[seedOEM] = append(out[seedOEM], row)
	}
	return out, nil
}

// firstNonEmpty returns the first argument whose trimmed form is not empty,
// or the last argument if none qualify. Kept unexported and package-local.
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
