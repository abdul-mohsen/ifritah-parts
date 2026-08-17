package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"parts-engine/internal/model"
)

// specificationRepo is the injectable DB dep for TecDocSpecifications.
type specificationRepo interface {
	QuerySpecifications(ctx context.Context, legacyArticleId int) ([]specificationRow, error)
}

type specificationRow struct {
	CriteriaDescription string
	RawValue            string
	UnitDescription     string
	CriteriaType        string
}

// TecDocSpecifications reads articlecriteria (27M rows) into structured
// model.Specification values. Unlike the existing TecDoc.ArticleSpecs which
// flattens everything into a map[string]string, this variant preserves the
// unit and criteria-type provenance callers need to render a proper spec
// table per BUGS.md ("Users cannot safely infer connectors, dimensions,
// torque, seals, or installation details" — the map form dropped that).
type TecDocSpecifications struct {
	repo specificationRepo
}

// NewTecDocSpecifications wires the service against a real MySQL *sql.DB.
func NewTecDocSpecifications(db *sql.DB) *TecDocSpecifications {
	if db == nil {
		return &TecDocSpecifications{}
	}
	return &TecDocSpecifications{repo: &sqlSpecificationRepo{db: db}}
}

// FindSpecifications returns the technical specifications recorded in
// TecDoc articlecriteria for the given article. Every row is stamped with
// Source="tecdoc:articlecriteria" so consumers can honestly claim
// manufacturer-confirmed provenance.
func (s *TecDocSpecifications) FindSpecifications(legacyArticleId int) ([]model.Specification, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if legacyArticleId <= 0 {
		return nil, fmt.Errorf("invalid legacyArticleId: %d", legacyArticleId)
	}

	rows, err := s.repo.QuerySpecifications(context.Background(), legacyArticleId)
	if err != nil {
		return nil, fmt.Errorf("find specifications: %w", err)
	}

	specs := make([]model.Specification, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.CriteriaDescription)
		val := strings.TrimSpace(r.RawValue)
		if name == "" || val == "" {
			continue
		}
		specs = append(specs, model.Specification{
			Name:         name,
			Value:        val,
			Unit:         strings.TrimSpace(r.UnitDescription),
			CriteriaType: strings.TrimSpace(r.CriteriaType),
			Source:       "tecdoc:articlecriteria",
		})
	}
	return specs, nil
}

// sqlSpecificationRepo is the production repo bound to MySQL.
type sqlSpecificationRepo struct {
	db *sql.DB
}

func (r *sqlSpecificationRepo) QuerySpecifications(ctx context.Context, legacyArticleId int) ([]specificationRow, error) {
	const q = `
		SELECT
			COALESCE(criteriaDescription, ''),
			COALESCE(rawValue, ''),
			COALESCE(criteriaUnitDescription, ''),
			COALESCE(criteriaType, '')
		FROM articlecriteria
		WHERE legacyArticleId = ?
		ORDER BY criteriaDescription
		LIMIT 200`

	rows, err := logQueryCtx(r.db, ctx, "TecDocSpecifications.FindSpecifications", q, legacyArticleId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []specificationRow
	for rows.Next() {
		var row specificationRow
		if err := rows.Scan(&row.CriteriaDescription, &row.RawValue, &row.UnitDescription, &row.CriteriaType); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}
