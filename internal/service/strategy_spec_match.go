package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"parts-engine/internal/model"
)

// ─── S6: SpecMatchStrategy ────────────────────────────────────────────────────
//
// Given a seed article (found via OEM or user-provided), finds all other
// articles in TecDoc that share the same mandatory specifications.
// Two parts are interchangeable when their physical specs match, not only when
// a cross-reference record exists — this unlocks brands absent from articlecrosses.
//
// Hard constraints enforced per category (docs/specs/domain-knowledge.md §3.x):
//   Timing belt:   teeth_count — never relaxed (engine damage)
//   CV axle:       inner_spline_count — never relaxed (physical incompatibility)
//   Spark plug:    thread_diameter, thread_reach, seat_type
//   Oil filter:    thread_size, bypass_pressure
//   Brake rotor:   outer_diameter_mm, vented
//   O2 sensor:     thread_size, wire_count, signal_type
//   All others:    TecDoc isMandatory=1 flag

type SpecMatchStrategy struct{ search *SmartSearch }

func (st *SpecMatchStrategy) Name() string           { return "spec_match" }
func (st *SpecMatchStrategy) ConfidenceBase() float64 { return 0.80 }
func (st *SpecMatchStrategy) Priority() float64       { return 0.80 }

func (st *SpecMatchStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.OEM == "" && req.Query == "" {
		return nil, nil
	}
	if st.search.tecDocSpecs == nil {
		return nil, fmt.Errorf("spec_match requires TecDoc specifications service")
	}

	// Step 1: resolve OEM → legacyArticleId + specs for the seed article
	seedId, seedSpecs, err := st.resolveSeedSpecs(ctx, req)
	if err != nil {
		return nil, err // propagate, not swallow
	}
	if len(seedSpecs) == 0 {
		return nil, nil
	}

	// Step 2: find articles sharing mandatory specs with the seed
	matches, err := st.search.tecDocSpecs.FindBySpecMatch(ctx, seedId, seedSpecs, req.Limit)
	if err != nil {
		return nil, err
	}

	// Step 3: build SmartResult slice, add safety notes for hard constraints
	results := make([]SmartResult, 0, len(matches))
	for _, m := range matches {
		if m.LegacyArticleId == seedId {
			continue // exclude the seed itself
		}
		rule := ClassifyCategory(m.Description)
		note := "Matched by specification"
		if isSafetyCritical(seedSpecs) {
			note = "Matched by specification — verify hard constraints before ordering"
		}
		results = append(results, SmartResult{
			Part: model.Part{
				LegacyArticleId: m.LegacyArticleId,
				ArticleNumber:   m.ArticleNumber,
				Description:     m.Description,
				BrandName:       m.BrandName,
			},
			Confidence:     0.80,
			ConfidenceNote: note,
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  m.BrandName,
		})
	}
	return results, nil
}

func (st *SpecMatchStrategy) resolveSeedSpecs(ctx context.Context, req StrategyRequest) (int, []model.Specification, error) {
	var seedId int

	// Try to get a legacyArticleId from OEM lookup
	if req.OEM != "" && st.search.oem != nil {
		oemResult, err := st.search.oem.Search(req.OEM, 3)
		if err == nil && oemResult != nil && len(oemResult.Results) > 0 {
			seedId = oemResult.Results[0].LegacyArticleId
		}
	}
	if seedId <= 0 && st.search.tecDocCrossRef != nil && req.OEM != "" {
		refs, err := st.search.tecDocCrossRef.SearchCrossReferences(req.OEM, 3)
		if err == nil && len(refs) > 0 {
			seedId = refs[0].LegacyArticleId
		}
	}
	if seedId <= 0 {
		return 0, nil, nil
	}

	specs, err := st.search.tecDocSpecs.FindSpecifications(seedId)
	if err != nil {
		return 0, nil, err
	}
	return seedId, specs, nil
}

// isSafetyCritical returns true when the spec set includes hard constraints
// where a wrong value causes engine damage or physical incompatibility.
func isSafetyCritical(specs []model.Specification) bool {
	critical := map[string]bool{
		"teeth_count":         true, // timing belt
		"inner_spline_count":  true, // CV axle
		"teeth number":        true,
		"number of teeth":     true,
	}
	for _, sp := range specs {
		if critical[normalizeSpecName(sp.Name)] {
			return true
		}
	}
	return false
}

func normalizeSpecName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		} else {
			result = append(result, c)
		}
	}
	return strings.TrimSpace(string(result))
}

// ─── FindBySpecMatch on TecDocSpecifications (S6-T1) ─────────────────────────

// SpecMatchResult is a lightweight article reference returned by spec matching.
type SpecMatchResult struct {
	LegacyArticleId int
	ArticleNumber   string
	Description     string
	BrandName       string
}

// FindBySpecMatch queries articlecriteria in reverse: find all articles whose
// spec values match those of the provided specs.
//
// seedId is the article to EXCLUDE from results (pass 0 to exclude nothing).
// This allows both "find alternatives for a known article" (seedId > 0) and
// "find articles matching derived specs from a parent assembly" (seedId = 0).
//
// Only specs with CriteriaType == "1" (mandatory in TecDoc) are used as hard
// filters. Soft specs are not used as SQL filters.
func (s *TecDocSpecifications) FindBySpecMatch(ctx context.Context, seedId int, seedSpecs []model.Specification, limit int) ([]SpecMatchResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if len(seedSpecs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	// Collect mandatory specs only (CriteriaType == "1").
	// Fall back to all provided specs when none are flagged mandatory.
	type specPair struct{ name, value string }
	var mandatory []specPair
	for _, sp := range seedSpecs {
		if sp.CriteriaType == "1" {
			mandatory = append(mandatory, specPair{strings.TrimSpace(sp.Name), strings.TrimSpace(sp.Value)})
		}
	}
	if len(mandatory) == 0 {
		for _, sp := range seedSpecs {
			name := strings.TrimSpace(sp.Name)
			value := strings.TrimSpace(sp.Value)
			if name != "" && value != "" {
				mandatory = append(mandatory, specPair{name, value})
			}
		}
	}
	if len(mandatory) == 0 {
		return nil, nil
	}

	// Use the most specific (first) mandatory spec as the SQL filter.
	// Callers should pass the hardest constraint first (e.g. teeth_count for timing belts).
	primary := mandatory[0]

	sqlspecRepo, ok := s.repo.(*sqlSpecificationRepo)
	if !ok {
		return nil, fmt.Errorf("FindBySpecMatch requires sqlSpecificationRepo")
	}

	// When seedId == 0 (no article to exclude), use a simpler query without the != filter.
	var (
		rows *sql.Rows
		err  error
	)
	if seedId > 0 {
		const q = `
			SELECT DISTINCT
				a.legacyArticleId,
				COALESCE(a.articleNumber, ''),
				COALESCE(a.genericArticleDescription, ''),
				COALESCE(ab.brandName, '')
			FROM articlecriteria ac
			JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.mfrId AND ab.lang = 'en'
			WHERE ac.criteriaDescription = ?
			  AND ac.rawValue = ?
			  AND ac.legacyArticleId != ?
			ORDER BY ab.brandName
			LIMIT ?`
		rows, err = logQueryCtx(sqlspecRepo.db, ctx, "TecDocSpecifications.FindBySpecMatch", q,
			primary.name, primary.value, seedId, limit)
	} else {
		const q = `
			SELECT DISTINCT
				a.legacyArticleId,
				COALESCE(a.articleNumber, ''),
				COALESCE(a.genericArticleDescription, ''),
				COALESCE(ab.brandName, '')
			FROM articlecriteria ac
			JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.mfrId AND ab.lang = 'en'
			WHERE ac.criteriaDescription = ?
			  AND ac.rawValue = ?
			ORDER BY ab.brandName
			LIMIT ?`
		rows, err = logQueryCtx(sqlspecRepo.db, ctx, "TecDocSpecifications.FindBySpecMatch", q,
			primary.name, primary.value, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SpecMatchResult
	for rows.Next() {
		var r SpecMatchResult
		if scanErr := rows.Scan(&r.LegacyArticleId, &r.ArticleNumber, &r.Description, &r.BrandName); scanErr != nil {
			log.Printf("[FindBySpecMatch] scan err: %v", scanErr)
			continue
		}
		out = append(out, r)
	}
	if rowErr := rows.Err(); rowErr != nil {
		return nil, fmt.Errorf("FindBySpecMatch rows: %w", rowErr)
	}
	return out, nil
}
