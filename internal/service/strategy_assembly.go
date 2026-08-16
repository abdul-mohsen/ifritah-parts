package service

import (
	"context"
	"fmt"
	"strings"

	"parts-engine/internal/model"
)

// ─── S7: AssemblyContextStrategy ─────────────────────────────────────────────
//
// Given a parent component (by article ID), the system reads its physical specs
// and derives what sub-components must fit it — without needing a DB cross-ref.
//
// Examples:
//   Suspension strut (body_diameter=46mm) → coil spring seat=46mm, top mount bolt pattern
//   Radiator (outlet_diameter=32mm)       → upper hose inner_diameter=32mm
//   Alternator (pulley_grooves=6, OAD)    → serpentine belt 6 grooves OAD-compatible
//
// Full parent→child spec table in docs/specs/domain-knowledge.md §1.3.

type AssemblyContextStrategy struct{ search *SmartSearch }

func (st *AssemblyContextStrategy) Name() string           { return "assembly_context" }
func (st *AssemblyContextStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *AssemblyContextStrategy) Priority() float64       { return 0.80 }

func (st *AssemblyContextStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if st.search.tecDocSpecs == nil {
		return nil, fmt.Errorf("assembly_context requires TecDoc specifications service")
	}

	// Resolve seed article ID from OEM or query
	seedId, err := st.resolveSeedId(ctx, req)
	if err != nil || seedId <= 0 {
		return nil, nil
	}

	// Read the parent's specs
	parentSpecs, err := st.search.tecDocSpecs.FindSpecifications(seedId)
	if err != nil || len(parentSpecs) == 0 {
		return nil, nil
	}

	// Identify the parent component type and derive child spec constraints
	parentType := identifyComponentType(parentSpecs, req.Category)
	childConstraints := deriveChildSpecs(parentType, parentSpecs)
	if len(childConstraints) == 0 {
		return nil, nil
	}

	// For each child category constraint, run a spec-match search
	var results []SmartResult
	seen := map[int]bool{seedId: true}

	for _, constraint := range childConstraints {
		childMatches, cerr := st.search.tecDocSpecs.FindBySpecMatch(ctx, 0, []model.Specification{
			{Name: constraint.specName, Value: constraint.specValue, CriteriaType: "1"},
		}, req.Limit/len(childConstraints)+1)
		if cerr != nil || len(childMatches) == 0 {
			continue
		}
		for _, m := range childMatches {
			if seen[m.LegacyArticleId] {
				continue
			}
			seen[m.LegacyArticleId] = true
			rule := ClassifyCategory(m.Description)
			reliability := constraint.reliability
			note := fmt.Sprintf("Sub-part matched via %s spec (parent→child, %s constraint)", constraint.specName, reliability)
			results = append(results, SmartResult{
				Part: model.Part{
					LegacyArticleId: m.LegacyArticleId,
					ArticleNumber:   m.ArticleNumber,
					Description:     m.Description,
					BrandName:       m.BrandName,
				},
				Confidence:     confidence(reliability),
				ConfidenceNote: note,
				FitmentDriver:  driverName(rule.Driver),
				BrandResolved:  m.BrandName,
			})
		}
	}
	return results, nil
}

func (st *AssemblyContextStrategy) resolveSeedId(ctx context.Context, req StrategyRequest) (int, error) {
	if req.OEM != "" && st.search.oem != nil {
		oemResult, err := st.search.oem.Search(req.OEM, 3)
		if err == nil && oemResult != nil && len(oemResult.Results) > 0 {
			return oemResult.Results[0].LegacyArticleId, nil
		}
	}
	return 0, nil
}

// ─── AssemblySpec registry ────────────────────────────────────────────────────

type childSpecConstraint struct {
	specName    string
	specValue   string
	reliability string // "high" | "medium" | "critical"
}

type componentRegistry struct {
	specKeywords    []string // keywords to identify this component type from its specs
	childConstraints func(parentSpecs []model.Specification) []childSpecConstraint
}

// identifyComponentType looks at the parent's spec names to classify the component.
func identifyComponentType(specs []model.Specification, category string) string {
	cat := strings.ToLower(category)
	switch {
	case strings.Contains(cat, "strut") || strings.Contains(cat, "shock"):
		return "strut"
	case strings.Contains(cat, "caliper"):
		return "caliper"
	case strings.Contains(cat, "radiator"):
		return "radiator"
	case strings.Contains(cat, "alternator"):
		return "alternator"
	case strings.Contains(cat, "transmission"):
		return "transmission"
	case strings.Contains(cat, "turbo"):
		return "turbocharger"
	case strings.Contains(cat, "exhaust manifold"):
		return "exhaust_manifold"
	case strings.Contains(cat, "steering"):
		return "steering_rack"
	case strings.Contains(cat, "thermostat"):
		return "thermostat"
	case strings.Contains(cat, "hub") || strings.Contains(cat, "bearing"):
		return "wheel_hub"
	}
	// Inspect spec names for component clues
	for _, sp := range specs {
		n := strings.ToLower(sp.Name)
		switch {
		case strings.Contains(n, "body diameter") || strings.Contains(n, "piston diameter"):
			return "strut"
		case strings.Contains(n, "inlet diameter") || strings.Contains(n, "outlet diameter"):
			return "radiator"
		case strings.Contains(n, "groove") || strings.Contains(n, "pulley"):
			return "alternator"
		case strings.Contains(n, "spline") && strings.Contains(n, "input"):
			return "transmission"
		case strings.Contains(n, "flange diameter") && strings.Contains(n, "turbo"):
			return "turbocharger"
		}
	}
	return "unknown"
}

// deriveChildSpecs maps a parent component type + its specs to child constraints.
// Full table: docs/specs/domain-knowledge.md §1.3
func deriveChildSpecs(componentType string, parentSpecs []model.Specification) []childSpecConstraint {
	specByName := map[string]string{}
	for _, sp := range parentSpecs {
		specByName[strings.ToLower(strings.TrimSpace(sp.Name))] = strings.TrimSpace(sp.Value)
	}

	var constraints []childSpecConstraint

	switch componentType {
	case "strut":
		if v, ok := specByName["body diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"inner diameter", v, "high"})
			constraints = append(constraints, childSpecConstraint{"spring seat diameter", v, "high"})
		}
		if v, ok := specByName["piston diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"bore diameter", v, "medium"})
		}

	case "caliper":
		if v, ok := specByName["piston diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"thickness", v, "high"})
		}
		if v, ok := specByName["pad slot width"]; ok {
			constraints = append(constraints, childSpecConstraint{"width", v, "high"})
		}

	case "radiator":
		for _, kw := range []string{"outlet diameter", "inlet diameter", "upper hose diameter", "lower hose diameter"} {
			if v, ok := specByName[kw]; ok {
				constraints = append(constraints, childSpecConstraint{"inner diameter", v, "high"})
			}
		}

	case "alternator":
		if v, ok := specByName["number of grooves"]; ok {
			constraints = append(constraints, childSpecConstraint{"number of grooves", v, "high"})
		}
		if v, ok := specByName["pulley type"]; ok {
			constraints = append(constraints, childSpecConstraint{"pulley type", v, "high"})
		}

	case "transmission":
		if v, ok := specByName["input shaft - number of teeth"]; ok {
			constraints = append(constraints, childSpecConstraint{"number of teeth", v, "critical"})
		}
		if v, ok := specByName["input shaft - spline"]; ok {
			constraints = append(constraints, childSpecConstraint{"inner spline count", v, "critical"})
		}

	case "turbocharger":
		if v, ok := specByName["inlet connection diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"connection diameter", v, "high"})
		}

	case "exhaust_manifold":
		if v, ok := specByName["connection diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"connection diameter", v, "high"})
		}

	case "thermostat":
		if v, ok := specByName["housing diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"inner diameter", v, "medium"})
		}

	case "wheel_hub":
		if v, ok := specByName["bore diameter"]; ok {
			constraints = append(constraints, childSpecConstraint{"inner diameter", v, "high"})
		}
		if v, ok := specByName["abs ring"]; ok && v != "" {
			constraints = append(constraints, childSpecConstraint{"abs ring", v, "high"})
		}
	}

	return constraints
}

func confidence(reliability string) float64 {
	switch reliability {
	case "critical":
		return 0.90
	case "high":
		return 0.85
	default:
		return 0.70
	}
}

// seen map initializer helper — package-level placeholder; real `seen` maps are local
var _ = map[int]bool{}

// ─── S8: VinAssemblyStrategy ──────────────────────────────────────────────────
//
// Given a VIN or linkageTargetId, the system reads linkagetargets (capacityCC,
// cylinders, fuelType) — which ARE the engine assembly specs — and runs
// AssemblyContextStrategy to find parts that fit those derived specs.
// Bridges the gap where a vehicle exists in the DB but not every part has been
// manually linked via articlesvehicletrees.

type VinAssemblyStrategy struct{ search *SmartSearch }

func (st *VinAssemblyStrategy) Name() string           { return "vin_assembly" }
func (st *VinAssemblyStrategy) ConfidenceBase() float64 { return 0.90 }
func (st *VinAssemblyStrategy) Priority() float64       { return 0.85 }

func (st *VinAssemblyStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.LinkageTargetId <= 0 {
		return nil, nil
	}
	if st.search.tecDocSpecs == nil || st.search.tecdoc == nil {
		return nil, nil
	}

	// Step 1: read vehicle specs from linkagetargets
	vehicleSpecs, err := st.resolveVehicleSpecs(ctx, req.LinkageTargetId)
	if err != nil || len(vehicleSpecs) == 0 {
		return nil, nil
	}

	// Step 2: identify component type from category + vehicle specs
	componentType := "engine" // default for vehicle-based search
	if req.Category != "" {
		componentType = identifyComponentType(nil, req.Category)
		if componentType == "unknown" {
			componentType = "engine"
		}
	}

	// Step 3: derive child constraints for the requested category
	childConstraints := deriveChildSpecs(componentType, vehicleSpecs)
	if len(childConstraints) == 0 {
		// No specific constraints derived — run vehicle fitment as fallback
		fallback := &VehicleFitmentStrategy{search: st.search}
		return fallback.Search(ctx, req)
	}

	// Step 4: for each child constraint, run spec match
	var results []SmartResult
	seenIds := map[int]bool{}
	for _, constraint := range childConstraints {
		matches, cerr := st.search.tecDocSpecs.FindBySpecMatch(ctx, 0, []model.Specification{
			{Name: constraint.specName, Value: constraint.specValue, CriteriaType: "1"},
		}, req.Limit)
		if cerr != nil {
			continue
		}
		for _, m := range matches {
			if seenIds[m.LegacyArticleId] {
				continue
			}
			seenIds[m.LegacyArticleId] = true
			rule := ClassifyCategory(m.Description)
			results = append(results, SmartResult{
				Part: model.Part{
					LegacyArticleId: m.LegacyArticleId,
					ArticleNumber:   m.ArticleNumber,
					Description:     m.Description,
					BrandName:       m.BrandName,
				},
				Confidence:     0.90,
				ConfidenceNote: fmt.Sprintf("Matched via vehicle engine spec (%s=%s)", constraint.specName, constraint.specValue),
				FitmentDriver:  driverName(rule.Driver),
				BrandResolved:  m.BrandName,
			})
		}
	}

	// Merge with direct vehicle fitment results (articlesvehicletrees hits at 0.98)
	if st.search.tecDocVehicle != nil {
		fitmentResults, _ := (&VehicleFitmentStrategy{search: st.search}).Search(ctx, req)
		for _, r := range fitmentResults {
			if !seenIds[r.LegacyArticleId] {
				seenIds[r.LegacyArticleId] = true
				r.Confidence = 0.98
				results = append(results, r)
			}
		}
	}

	return results, nil
}

// resolveVehicleSpecs reads linkagetargets columns and maps them to
// model.Specification so they can be fed into deriveChildSpecs.
func (st *VinAssemblyStrategy) resolveVehicleSpecs(ctx context.Context, linkageTargetId int) ([]model.Specification, error) {
	if st.search.tecdoc == nil {
		return nil, fmt.Errorf("TecDoc service not available")
	}
	// Use TecDoc.ResolveVehicle indirectly — query linkagetargets directly via tecdoc db
	return st.search.tecdoc.LinkageTargetToSpecs(ctx, linkageTargetId)
}
