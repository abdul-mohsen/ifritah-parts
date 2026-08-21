package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"parts-engine/internal/model"
)

// crossRefRepo is the injectable DB dependency for TecDocCrossRef.
// The production implementation runs the SQL query against articlecrosses
// (30M rows). Tests inject a stub that returns fixture rows without touching
// a live MySQL database.
type crossRefRepo interface {
	QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error)
	// Batched form: single IN(...) query for N OEM numbers. Returns rows
	// keyed by the input OEM so the caller can map back to the seed article
	// without a per-item lookup.
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
// as structured OEM references. Distinct from TecDoc.SearchByOEM: SearchByOEM
// walks the compact oem_number/oem_search_index tables; SearchCrossReferences
// walks the authoritative cross-reference table itself so the mfrName
// provenance survives into the response.
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
// Given N OEM numbers, it runs a single IN(...) query and returns a map
// keyed by NORMALISED OEM (input passed through NormalizeOEM). Empty or
// duplicate OEM inputs are dropped.
//
// Used by the vehicle-fitment enrichment path so N vehicle rows don't turn
// into N cross-reference queries.
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
// Queries the articlecrosses table via the indexed generated column
// `articlecrosses.oemNumberNormalized` created by
// sql/06_articlecrosses_normalized_oem_index.sql.
//
// The migration is a hard deploy prerequisite. Pre-migration deploys will
// surface a clear "Unknown column" error on every cross_reference call —
// which is intentional. The prior conditional-fallback branch (silently
// falling back to a 3-8 hour full-table scan) was worse: it made the
// system look "working but slow" when it was really "requires a migration
// that nobody ran". Fail loud, fix the migration, deploy: KISS.
type sqlCrossRefRepo struct {
	db *sql.DB
}

func (r *sqlCrossRefRepo) QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error) {
	const q = `
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
		LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'
		WHERE ac.oemNumberNormalized = ?
		LIMIT ?`

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

func (r *sqlCrossRefRepo) QueryCrossRefsBatch(ctx context.Context, cleanOEMs []string, limitPerOEM int) (map[string][]crossRefRow, error) {
	if len(cleanOEMs) == 0 {
		return nil, nil
	}
	if limitPerOEM <= 0 || limitPerOEM > 100 {
		limitPerOEM = 20
	}
	// Deduplicate.
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
	// Cap total result set so a 1000-OEM call can't return hundreds of
	// thousands of rows.
	totalLimit := limitPerOEM * len(uniq)
	if totalLimit > 2000 {
		totalLimit = 2000
	}

	q := fmt.Sprintf(`
		SELECT
			ac.oemNumberNormalized AS clean_oem,
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
		WHERE ac.oemNumberNormalized IN (%s)
		LIMIT %d`, placeholders, totalLimit)

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
// or the last argument if none qualify.
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
