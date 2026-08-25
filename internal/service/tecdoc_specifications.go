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
	QuerySpecificationsBatch(ctx context.Context, legacyArticleIds []int) (map[int][]specificationRow, error)
}

type specificationRow struct {
	CriteriaDescription string
	RawValue            string
	UnitDescription     string
	CriteriaType        string
	LegacyArticleId     int
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

// FindSpecificationsBatch returns specifications for MANY article ids in a
// single SQL round-trip. Used by enrichResults (M3.S1.T2) to cut per-result
// query fan-out — for a 20-result response the batch version is one query
// instead of twenty.
//
// Requires sql/07_articlecriteria_indexes.sql applied: the IN (?,?,?)
// scan uses the idx_articlecriteria_legacyArticleId index; without it the
// query does N full scans of the 27M-row table.
//
// Returns map[legacyArticleId][]Specification with only articleIds that
// had rows. Empty ids are omitted (caller can distinguish from nil-value).
func (s *TecDocSpecifications) FindSpecificationsBatch(legacyArticleIds []int) (map[int][]model.Specification, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if len(legacyArticleIds) == 0 {
		return map[int][]model.Specification{}, nil
	}
	// Dedupe zero + duplicates
	seen := make(map[int]bool, len(legacyArticleIds))
	ids := make([]int, 0, len(legacyArticleIds))
	for _, id := range legacyArticleIds {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return map[int][]model.Specification{}, nil
	}

	rowsById, err := s.repo.QuerySpecificationsBatch(context.Background(), ids)
	if err != nil {
		return nil, fmt.Errorf("find specifications batch: %w", err)
	}

	out := make(map[int][]model.Specification, len(rowsById))
	for id, rows := range rowsById {
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
		if len(specs) > 0 {
			out[id] = specs
		}
	}
	return out, nil
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

// QuerySpecificationsBatch runs one IN-list query for multiple article
// ids. Uses idx_articlecriteria_legacyArticleId (sql/07) so the plan is
// a bounded index seek per id — total cost ~ len(ids) index seeks vs
// len(ids) full scans without the index.
//
// Caps len(ids) at 100 to protect against runaway batches; caller should
// chunk larger sets.
func (r *sqlSpecificationRepo) QuerySpecificationsBatch(ctx context.Context, legacyArticleIds []int) (map[int][]specificationRow, error) {
	if len(legacyArticleIds) == 0 {
		return nil, nil
	}
	if len(legacyArticleIds) > 100 {
		legacyArticleIds = legacyArticleIds[:100]
	}
	placeholders := make([]string, len(legacyArticleIds))
	args := make([]any, len(legacyArticleIds))
	for i, id := range legacyArticleIds {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `
		SELECT
			legacyArticleId,
			COALESCE(criteriaDescription, ''),
			COALESCE(rawValue, ''),
			COALESCE(criteriaUnitDescription, ''),
			COALESCE(criteriaType, '')
		FROM articlecriteria
		WHERE legacyArticleId IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY legacyArticleId, criteriaDescription
		LIMIT ` + fmt.Sprintf("%d", len(legacyArticleIds)*200)

	rows, err := logQueryCtx(r.db, ctx, "TecDocSpecifications.FindSpecificationsBatch", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int][]specificationRow, len(legacyArticleIds))
	for rows.Next() {
		var row specificationRow
		if err := rows.Scan(&row.LegacyArticleId, &row.CriteriaDescription, &row.RawValue, &row.UnitDescription, &row.CriteriaType); err != nil {
			continue
		}
		out[row.LegacyArticleId] = append(out[row.LegacyArticleId], row)
	}
	return out, nil
}
