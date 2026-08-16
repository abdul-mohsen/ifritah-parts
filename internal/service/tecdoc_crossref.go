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
// a live MySQL database. Matching the supersession.go pattern.
type crossRefRepo interface {
	QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error)
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

// sqlCrossRefRepo is the production repo bound to a MySQL *sql.DB.
// It runs the articlecrosses query with LEFT JOIN so parts missing from
// the local articles view still surface with an empty article number
// (the raw crossOemNumber alone is still evidence).
type sqlCrossRefRepo struct {
	db *sql.DB
}

func (r *sqlCrossRefRepo) QueryCrossRefs(ctx context.Context, cleanOEM string, limit int) ([]crossRefRow, error) {
	const q = `
		SELECT
			ac.articleCrossNumber,
			COALESCE(ac.mfrName, ''),
			COALESCE(a.legacyArticleId, 0),
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, ''),
			COALESCE(ac.originalOemManufacturer, '')
		FROM articlecrosses ac
		LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE ac.cleanCrossNumber = ?
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

// firstNonEmpty returns the first argument whose trimmed form is not empty,
// or the last argument if none qualify. Kept unexported and package-local.
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
