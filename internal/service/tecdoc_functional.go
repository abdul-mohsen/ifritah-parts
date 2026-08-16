package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// functionalRepo is the injectable DB dep for TecDocFunctional.
type functionalRepo interface {
	// QueryGenericId returns the genericArticleId that legacy2generic maps
	// the given legacyArticleId to, or 0 when no mapping exists.
	QueryGenericId(ctx context.Context, legacyArticleId int) (int, error)
	// QuerySameGeneric returns other articles that map to the same
	// genericArticleId, optionally restricted to a specific vehicle
	// (linkageTargetId) when vehicleId > 0.
	QuerySameGeneric(ctx context.Context, genericId, excludeArticleId, vehicleId, limit int) ([]functionalRow, error)
}

type functionalRow struct {
	LegacyArticleId int
	ArticleNumber   string
	Description     string
	BrandName       string
	GenericId       int
}

// TecDocFunctional finds functional equivalents — parts that fill the same
// role (same genericArticleId in legacy2generic) even though they are not
// listed as direct cross-references in articlecrosses.
//
// This lets a caller answer "which parts are, functionally, alternatives
// to article X?" without depending on an OEM-number chain. When a
// vehicleId is supplied, the result is further filtered to parts that
// fit that vehicle (via articlesvehicletrees) so the response is
// vehicle-safe.
type TecDocFunctional struct {
	repo functionalRepo
}

func NewTecDocFunctional(db *sql.DB) *TecDocFunctional {
	if db == nil {
		return &TecDocFunctional{}
	}
	return &TecDocFunctional{repo: &sqlFunctionalRepo{db: db}}
}

// FindFunctionalEquivalents returns functionally-equivalent alternatives
// for legacyArticleId. If vehicleId > 0, results are constrained to
// parts that fit that vehicle. limit is clamped to [1, 100].
func (s *TecDocFunctional) FindFunctionalEquivalents(legacyArticleId, vehicleId, limit int) ([]model.OEMReference, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if legacyArticleId <= 0 {
		return nil, fmt.Errorf("invalid legacyArticleId: %d", legacyArticleId)
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	ctx := context.Background()
	genericId, err := s.repo.QueryGenericId(ctx, legacyArticleId)
	if err != nil {
		return nil, fmt.Errorf("resolve generic id: %w", err)
	}
	if genericId == 0 {
		// No legacy2generic mapping — nothing functionally equivalent
		// to report. Not an error; the caller decides how to render
		// "no equivalents known".
		return nil, nil
	}

	rows, err := s.repo.QuerySameGeneric(ctx, genericId, legacyArticleId, vehicleId, limit)
	if err != nil {
		return nil, fmt.Errorf("query functional equivalents: %w", err)
	}

	refs := make([]model.OEMReference, 0, len(rows))
	seen := map[int]bool{legacyArticleId: true}
	for _, r := range rows {
		if r.LegacyArticleId == 0 || seen[r.LegacyArticleId] {
			continue
		}
		seen[r.LegacyArticleId] = true
		refs = append(refs, model.OEMReference{
			BrandName:       r.BrandName,
			ArticleNumber:   r.ArticleNumber,
			Description:     r.Description,
			LegacyArticleId: r.LegacyArticleId,
			Manufacturer:    "functional:legacy2generic",
		})
	}
	return refs, nil
}

// sqlFunctionalRepo is the production repo bound to MySQL.
type sqlFunctionalRepo struct {
	db *sql.DB
}

func (r *sqlFunctionalRepo) QueryGenericId(ctx context.Context, legacyArticleId int) (int, error) {
	const q = `SELECT genericArticleId FROM legacy2generic WHERE legacyArticleId = ? LIMIT 1`
	row := logQueryRow(r.db, "TecDocFunctional.QueryGenericId", q, legacyArticleId)
	var id int
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

func (r *sqlFunctionalRepo) QuerySameGeneric(ctx context.Context, genericId, excludeArticleId, vehicleId, limit int) ([]functionalRow, error) {
	// Two flavours: with and without vehicle filter. Kept explicit so
	// the query planner can pick the best index rather than a single
	// query with an OR-branch.
	var q string
	var args []interface{}
	if vehicleId > 0 {
		q = `
			SELECT DISTINCT
				a.legacyArticleId,
				COALESCE(a.articleNumber, ''),
				COALESCE(a.genericArticleDescription, ''),
				COALESCE(ab.brandName, ''),
				lg.genericArticleId
			FROM legacy2generic lg
			JOIN articles a ON a.legacyArticleId = lg.legacyArticleId
			JOIN articlesvehicletrees avt ON avt.legacyArticleId = a.legacyArticleId AND avt.linkingTargetType = 'P'
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE lg.genericArticleId = ?
			  AND a.legacyArticleId <> ?
			  AND avt.linkingTargetId = ?
			LIMIT ?`
		args = []interface{}{genericId, excludeArticleId, vehicleId, limit}
	} else {
		q = `
			SELECT DISTINCT
				a.legacyArticleId,
				COALESCE(a.articleNumber, ''),
				COALESCE(a.genericArticleDescription, ''),
				COALESCE(ab.brandName, ''),
				lg.genericArticleId
			FROM legacy2generic lg
			JOIN articles a ON a.legacyArticleId = lg.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE lg.genericArticleId = ?
			  AND a.legacyArticleId <> ?
			LIMIT ?`
		args = []interface{}{genericId, excludeArticleId, limit}
	}

	rows, err := logQueryCtx(r.db, ctx, "TecDocFunctional.QuerySameGeneric", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []functionalRow
	for rows.Next() {
		var row functionalRow
		if err := rows.Scan(&row.LegacyArticleId, &row.ArticleNumber, &row.Description, &row.BrandName, &row.GenericId); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}
