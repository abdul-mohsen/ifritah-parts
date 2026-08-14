package service

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"parts-engine/internal/model"
)

var stripChars = regexp.MustCompile(`[-.\s/]`)

// OEMLookup searches the normalized OEM index.
type OEMLookup struct {
	db *sql.DB
}

func NewOEMLookup(db *sql.DB) *OEMLookup {
	return &OEMLookup{db: db}
}

// NormalizeOEM strips dashes, dots, spaces, slashes and lowercases.
func NormalizeOEM(raw string) string {
	return strings.ToLower(stripChars.ReplaceAllString(raw, ""))
}

// Search finds aftermarket parts matching an OEM number (fuzzy on formatting).
func (s *OEMLookup) Search(oemNumber string, limit int) (*model.OEMSearchResult, error) {
	start := time.Now()
	log.Printf("[OEMLookup.Search] START oem=%q limit=%d", oemNumber, limit)

	if s.db == nil {
		log.Printf("[OEMLookup.Search] ABORT db=nil")
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	normalized := NormalizeOEM(oemNumber)
	if normalized == "" {
		log.Printf("[OEMLookup.Search] ABORT empty after normalize")
		return nil, fmt.Errorf("empty OEM number")
	}
	log.Printf("[OEMLookup.Search] normalized=%q", normalized)

	query := `SELECT raw_number, normalized, legacyArticleId, source_table,
	                 mfr_name, brand_name, article_number, description
	          FROM oem_search_index
	          WHERE normalized = ?
	          LIMIT ?`

	rows, err := logQuery(s.db, "OEMLookup.Search", query, normalized, limit*3)
	if err != nil {
		log.Printf("[OEMLookup.Search] QUERY ERROR after %v: %v", time.Since(start), err)
		return nil, fmt.Errorf("OEM search: %w", err)
	}
	log.Printf("[OEMLookup.Search] query returned in %v", time.Since(start))
	defer rows.Close()

	var results []model.OEMReference
	seenArticle := make(map[int]bool)
	for rows.Next() {
		var ref model.OEMReference
		var norm, src sql.NullString
		var mfr, brand, artNum, desc sql.NullString
		if err := rows.Scan(&ref.RawNumber, &norm, &ref.LegacyArticleId, &src,
			&mfr, &brand, &artNum, &desc); err != nil {
			return nil, fmt.Errorf("scan OEM: %w", err)
		}
		ref.Normalized = norm.String
		ref.Manufacturer = mfr.String
		ref.BrandName = brand.String
		ref.ArticleNumber = artNum.String
		ref.Description = desc.String

		// Filter self-references: skip rows where the raw_number IS the queried OEM
		if NormalizeOEM(ref.ArticleNumber) == normalized {
			continue
		}

		// Deduplicate by legacyArticleId (same part from multiple source tables)
		if seenArticle[ref.LegacyArticleId] {
			continue
		}
		seenArticle[ref.LegacyArticleId] = true

		results = append(results, ref)
		if len(results) >= limit {
			break
		}
	}

	log.Printf("[OEMLookup.Search] DONE results=%d (deduped) elapsed=%v", len(results), time.Since(start))

	return &model.OEMSearchResult{
		Query:      oemNumber,
		Normalized: normalized,
		Results:    results,
		Total:      len(results),
	}, nil
}

// OEMNumbersForArticle returns OEM part numbers associated with a legacyArticleId.
func (s *OEMLookup) OEMNumbersForArticle(legacyArticleId int) ([]string, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := logQuery(s.db, "OEMLookup.OEMNumbersForArticle",
		`SELECT DISTINCT raw_number FROM oem_search_index WHERE legacyArticleId = ? LIMIT 20`,
		legacyArticleId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nums []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		nums = append(nums, n)
	}
	return nums, nil
}

// BatchOEMNumbers returns OEM numbers for multiple legacyArticleIds.
func (s *OEMLookup) BatchOEMNumbers(articleIds []int) (map[int][]string, error) {
	if s.db == nil || len(articleIds) == 0 {
		return nil, nil
	}
	result := make(map[int][]string)

	// Process in chunks of 50 to avoid huge IN clauses
	chunkSize := 50
	for i := 0; i < len(articleIds); i += chunkSize {
		end := i + chunkSize
		if end > len(articleIds) {
			end = len(articleIds)
		}
		chunk := articleIds[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}
		ph := "(" + strings.Join(placeholders, ",") + ")"

		rows, err := s.db.Query(
			`SELECT legacyArticleId, raw_number FROM oem_search_index WHERE legacyArticleId IN `+ph+` LIMIT 500`,
			args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var aid int
			var num string
			if err := rows.Scan(&aid, &num); err != nil {
				continue
			}
			result[aid] = append(result[aid], num)
		}
		rows.Close()
	}
	return result, nil
}
