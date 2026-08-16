package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"parts-engine/internal/model"
)

// crossBrandRepo is the injectable DB dep for TecDocCrossBrand.
type crossBrandRepo interface {
	// QuerySiblingHits returns aftermarket parts whose articlecrosses row
	// carries the requested OEM number but whose fitment (via
	// articlesvehicletrees) covers vehicles of a sibling manufacturer
	// (e.g. Hyundai -> Kia on shared platforms). The result is aggregated
	// per (siblingMake, siblingModel, platform) so the caller can render
	// one card per sibling model rather than one per part.
	QuerySiblingHits(ctx context.Context, cleanOEM, sourceMake string, limit int) ([]crossBrandRow, error)
}

type crossBrandRow struct {
	SiblingMake  string
	SiblingModel string
	Platform     string
	SharedParts  int
}

// TecDocCrossBrand answers "if I have an OEM number for a Hyundai part,
// which Kia (or reverse) models share it via a platform-mate relationship?"
//
// The relationship exploited is: two vehicles with different manufacturers
// but overlapping fitment in articlesvehicletrees for the same
// aftermarket articles. We surface aggregated CrossBrandHit rows rather
// than raw part matches so the UI can show "Kia Sportage 2015-2020
// (17 shared parts)" cards.
type TecDocCrossBrand struct {
	repo crossBrandRepo
}

func NewTecDocCrossBrand(db *sql.DB) *TecDocCrossBrand {
	if db == nil {
		return &TecDocCrossBrand{}
	}
	return &TecDocCrossBrand{repo: &sqlCrossBrandRepo{db: db}}
}

// FindCrossBrandEquivalents returns sibling-brand vehicles that share
// aftermarket parts with the OEM number's home brand. sourceMake is a
// hint used for logging + potential future disambiguation (e.g. resolve
// "H-only prefix" ranges into Hyundai). limit is clamped to [1, 50].
func (s *TecDocCrossBrand) FindCrossBrandEquivalents(oemNumber, sourceMake string, limit int) ([]model.CrossBrandHit, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if strings.TrimSpace(oemNumber) == "" {
		return nil, fmt.Errorf("empty OEM number")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		return nil, fmt.Errorf("empty OEM number")
	}

	rows, err := s.repo.QuerySiblingHits(context.Background(), clean, sourceMake, limit)
	if err != nil {
		return nil, fmt.Errorf("cross-brand equivalents: %w", err)
	}

	out := make([]model.CrossBrandHit, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		if r.SiblingMake == "" || r.SiblingModel == "" {
			continue
		}
		if strings.EqualFold(r.SiblingMake, sourceMake) {
			// Same-brand hit is not a cross-brand suggestion.
			continue
		}
		key := strings.ToLower(r.SiblingMake + "|" + r.SiblingModel + "|" + r.Platform)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, model.CrossBrandHit{
			SiblingMake:  r.SiblingMake,
			SiblingModel: r.SiblingModel,
			Platform:     r.Platform,
			SharedParts:  r.SharedParts,
		})
	}
	return out, nil
}

// sqlCrossBrandRepo is the production repo bound to MySQL.
type sqlCrossBrandRepo struct {
	db *sql.DB
}

func (r *sqlCrossBrandRepo) QuerySiblingHits(ctx context.Context, cleanOEM, sourceMake string, limit int) ([]crossBrandRow, error) {
	// The platform column is not populated on all TecDoc rows; we fall
	// back to modelseries.modelname when platform is unknown so callers
	// still get a useful label.
	const q = `
		SELECT
			COALESCE(m.manuName, ''),
			COALESCE(ms.modelname, ''),
			COALESCE(ms.platform, ms.modelname, ''),
			COUNT(DISTINCT avt.legacyArticleId) AS shared_parts
		FROM oem_number on2
		JOIN articles a ON a.legacyArticleId = on2.articleId
		JOIN articlesvehicletrees avt ON avt.legacyArticleId = a.legacyArticleId AND avt.linkingTargetType = 'P'
		JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
		JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId
		JOIN manufacturers m ON m.manuId = ms.manuId
		WHERE on2.clean_number = ?
		  AND (? = '' OR m.manuName <> ?)
		GROUP BY m.manuName, ms.modelname, ms.platform
		ORDER BY shared_parts DESC
		LIMIT ?`

	rows, err := logQueryCtx(r.db, ctx, "TecDocCrossBrand.FindCrossBrandEquivalents", q, cleanOEM, sourceMake, sourceMake, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []crossBrandRow
	for rows.Next() {
		var row crossBrandRow
		if err := rows.Scan(&row.SiblingMake, &row.SiblingModel, &row.Platform, &row.SharedParts); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}
