package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
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
// isSafetyCritical returns true when a spec set contains one of the hard-
// constraint fields where a wrong value causes engine damage or physical
// incompatibility (docs/specs/domain-knowledge.md §1.4).
//
// Uses the same substring match as safetyRank so a single change to the
// keyword list is enough to keep both paths consistent.
func isSafetyCritical(specs []model.Specification) bool {
	for _, sp := range specs {
		if safetyRank(sp.Name) == 0 {
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
//
// Safety-critical constraints (timing belt teeth_count, CV axle spline_count)
// are moved to the FRONT of the mandatory list so they become the PRIMARY SQL
// filter. Wrong teeth count on a timing belt = engine damage; wrong spline
// count on a CV axle = physical incompatibility. Ordering matters because
// the SQL query uses only the first mandatory spec as its WHERE filter.
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

	// Re-order so safety-critical specs are used first as the SQL filter.
	// docs/specs/domain-knowledge.md §1.4 hard-constraint categories:
	//   timing belt:  teeth_count       — wrong value = engine damage
	//   CV axle:      inner_spline_count — wrong value = physical incompatibility
	//   spark plug:   thread_diameter, thread_reach, seat_type
	//   brake rotor:  outer_diameter_mm, vented
	//   O2 sensor:    thread_size, wire_count, signal_type
	//   oil filter:   thread_size, bypass_pressure
	sort.SliceStable(mandatory, func(i, j int) bool {
		return safetyRank(mandatory[i].name) < safetyRank(mandatory[j].name)
	})

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

	// Post-filter: enforce ALL remaining mandatory constraints in memory.
	// This turns the "first-spec only" SQL filter into an actual multi-spec
	// intersection so a timing belt with matching thread but WRONG teeth count
	// is still rejected. Requires a second query per candidate but the
	// candidate set is already bounded by `limit`, so this is at most
	// `limit * (len(mandatory) - 1)` cheap indexed lookups.
	if len(mandatory) > 1 && len(out) > 0 {
		filtered := make([]SpecMatchResult, 0, len(out))
		for _, r := range out {
			ok := true
			for _, cons := range mandatory[1:] {
				if !s.articleHasSpec(ctx, sqlspecRepo, r.LegacyArticleId, cons.name, cons.value) {
					ok = false
					break
				}
			}
			if ok {
				filtered = append(filtered, r)
			}
		}
		out = filtered
	}
	return out, nil
}

// articleHasSpec checks whether the given article has a specific
// criteriaDescription+rawValue in articlecriteria. Used to enforce multi-spec
// mandatory intersections after the primary SQL filter.
func (s *TecDocSpecifications) articleHasSpec(ctx context.Context, repo *sqlSpecificationRepo, articleId int, name, value string) bool {
	const q = `
		SELECT 1
		FROM articlecriteria
		WHERE legacyArticleId = ?
		  AND criteriaDescription = ?
		  AND rawValue = ?
		LIMIT 1`
	rows, err := logQueryCtx(repo.db, ctx, "TecDocSpecifications.articleHasSpec", q, articleId, name, value)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

// safetyRank orders spec names so safety-critical hard constraints sort first.
// Lower rank = filtered earlier in SQL. See domain-knowledge.md §1.4.
func safetyRank(specName string) int {
	n := strings.ToLower(strings.TrimSpace(specName))
	// Level 0 — critical: wrong value = engine damage or physical incompatibility
	criticalKeywords := []string{
		"teeth", "spline count", "spline", "number of teeth", "tooth count",
		"pitch", "belt teeth",
	}
	for _, kw := range criticalKeywords {
		if strings.Contains(n, kw) {
			return 0
		}
	}
	// Level 1 — hard fit: thread, bore, diameter (physical fit)
	hardFitKeywords := []string{
		"thread diameter", "thread size", "thread reach", "thread pitch",
		"outer diameter", "bore diameter", "inner diameter", "seat type",
	}
	for _, kw := range hardFitKeywords {
		if strings.Contains(n, kw) {
			return 1
		}
	}
	// Level 2 — electrical/signal (o2 sensor connector, wire count)
	electricalKeywords := []string{
		"wire count", "wire number", "signal type", "connector", "connection type",
	}
	for _, kw := range electricalKeywords {
		if strings.Contains(n, kw) {
			return 2
		}
	}
	// Level 3 — everything else
	return 3
}
