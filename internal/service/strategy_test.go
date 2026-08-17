package service

import (
	"context"
	"testing"
	"time"

	"parts-engine/internal/model"
)

// ─── SearchStrategy interface conformance ─────────────────────────────────

// TestSearchStrategy_InterfaceContract verifies every strategy in the registry
// returns a stable Name(), a ConfidenceBase() in [0, 1], and a Priority() in [0, 1].
// This is a cheap contract test — if a new strategy is added and forgets to set
// one of these, the test fails immediately.
func TestSearchStrategy_InterfaceContract(t *testing.T) {
	s := &SmartSearch{}
	strategies := []SearchStrategy{
		&CacheStrategy{search: s},
		&LegacyCascadeStrategy{search: s},
		&ExactOEMStrategy{search: s},
		&CrossReferenceStrategy{search: s},
		&VehicleFitmentStrategy{search: s},
		&SupersessionStrategy{search: s},
		&CrossBrandStrategy{search: s},
		&OwnedCatalogStrategy{search: s},
		&KeywordGatedStrategy{search: s},
		&PrefixInferenceStrategy{search: s},
		&SpecMatchStrategy{search: s},
		&AssemblyContextStrategy{search: s},
		&VinAssemblyStrategy{search: s},
	}
	seenNames := map[string]bool{}
	for _, st := range strategies {
		name := st.Name()
		if name == "" {
			t.Errorf("%T.Name() = empty string", st)
		}
		if seenNames[name] {
			t.Errorf("%T.Name() = %q — duplicate strategy name", st, name)
		}
		seenNames[name] = true
		if cb := st.ConfidenceBase(); cb < 0 || cb > 1 {
			t.Errorf("%s.ConfidenceBase() = %f, want [0, 1]", name, cb)
		}
		if pr := st.Priority(); pr < 0 || pr > 1 {
			t.Errorf("%s.Priority() = %f, want [0, 1]", name, pr)
		}
	}
}

// TestSearchStrategy_Priorities verifies the documented priority ordering.
// Exact OEM must outrank all deductions; combined+priority weighting depends on it.
func TestSearchStrategy_Priorities(t *testing.T) {
	s := &SmartSearch{}
	exact := (&ExactOEMStrategy{search: s}).Priority()
	crossRef := (&CrossReferenceStrategy{search: s}).Priority()
	spec := (&SpecMatchStrategy{search: s}).Priority()
	assembly := (&AssemblyContextStrategy{search: s}).Priority()
	crossBrand := (&CrossBrandStrategy{search: s}).Priority()

	if exact < crossRef {
		t.Errorf("exact_oem priority (%f) must be >= cross_reference (%f)", exact, crossRef)
	}
	if crossRef < spec {
		t.Errorf("cross_reference priority (%f) must be >= spec_match (%f)", crossRef, spec)
	}
	if spec < crossBrand {
		t.Errorf("spec_match priority (%f) must be >= cross_brand (%f)", spec, crossBrand)
	}
	if assembly < crossBrand {
		t.Errorf("assembly_context priority (%f) must be >= cross_brand (%f)", assembly, crossBrand)
	}
}

// ─── AvailableModes / strategyForMode ─────────────────────────────────────

// TestAvailableModes_MinimalRegistry verifies the base modes are always exposed
// even without TecDoc wiring. spec_match / assembly_context / vin_assembly only
// appear when tecDocSpecs != nil.
func TestAvailableModes_MinimalRegistry(t *testing.T) {
	s := &SmartSearch{}
	modes := s.AvailableModes()
	keys := map[string]bool{}
	for _, m := range modes {
		keys[m.Key] = true
	}
	// Base modes MUST always be present
	baseModes := []string{"exact_oem", "cross_reference", "vehicle_fitment", "supersession", "cross_brand", "owned_catalog", "keyword_gated", "legacy", "prefix_inference", "cache", "combined"}
	for _, want := range baseModes {
		if !keys[want] {
			t.Errorf("AvailableModes() missing base mode %q; got %v", want, keys)
		}
	}
	// Spec-based modes MUST NOT appear without tecDocSpecs
	for _, forbidden := range []string{"spec_match", "assembly_context", "vin_assembly"} {
		if keys[forbidden] {
			t.Errorf("AvailableModes() includes %q without tecDocSpecs — guard violated", forbidden)
		}
	}
}

// TestAvailableModes_FullRegistry verifies spec-based modes appear once
// tecDocSpecs is wired.
func TestAvailableModes_FullRegistry(t *testing.T) {
	s := &SmartSearch{tecDocSpecs: &TecDocSpecifications{}}
	keys := map[string]bool{}
	for _, m := range s.AvailableModes() {
		keys[m.Key] = true
	}
	for _, want := range []string{"spec_match", "assembly_context", "vin_assembly"} {
		if !keys[want] {
			t.Errorf("AvailableModes() with tecDocSpecs missing %q; got %v", want, keys)
		}
	}
}

// TestStrategyForMode_UnknownReturnsNil confirms callers can detect an
// unrecognised mode instead of getting a silent fallback.
func TestStrategyForMode_UnknownReturnsNil(t *testing.T) {
	s := &SmartSearch{}
	if got := s.strategyForMode("no-such-mode"); got != nil {
		t.Errorf("strategyForMode(unknown) = %T, want nil", got)
	}
	if got := s.strategyForMode(""); got != nil {
		t.Errorf("strategyForMode(empty) = %T, want nil (caller should not route empty mode here)", got)
	}
}

// TestStrategyForMode_SpecModesRequireTecDoc verifies the guard: without
// tecDocSpecs, spec_match / assembly_context / vin_assembly return nil.
func TestStrategyForMode_SpecModesRequireTecDoc(t *testing.T) {
	s := &SmartSearch{} // no TecDoc wiring
	for _, mode := range []string{"spec_match", "assembly_context", "vin_assembly"} {
		if got := s.strategyForMode(mode); got != nil {
			t.Errorf("strategyForMode(%q) without tecDocSpecs = %T, want nil", mode, got)
		}
	}
}

// ─── ExactOEMStrategy short-circuit tests ─────────────────────────────────

// TestExactOEMStrategy_EmptyOEM_ReturnsNil verifies the strategy short-circuits
// on empty input rather than issuing a DB query.
func TestExactOEMStrategy_EmptyOEM_ReturnsNil(t *testing.T) {
	strategy := &ExactOEMStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{OEM: "", Limit: 10})
	if err != nil {
		t.Errorf("ExactOEMStrategy with empty OEM returned err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("ExactOEMStrategy with empty OEM returned %d results, want 0", len(results))
	}
}

// TestCrossReferenceStrategy_EmptyOEM_ReturnsNil verifies short-circuit.
func TestCrossReferenceStrategy_EmptyOEM_ReturnsNil(t *testing.T) {
	strategy := &CrossReferenceStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{OEM: "", Limit: 10})
	if err != nil {
		t.Errorf("CrossReferenceStrategy with empty OEM err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("CrossReferenceStrategy with empty OEM returned %d results, want 0", len(results))
	}
}

// TestVehicleFitmentStrategy_ZeroLinkage_ReturnsNil verifies short-circuit.
func TestVehicleFitmentStrategy_ZeroLinkage_ReturnsNil(t *testing.T) {
	strategy := &VehicleFitmentStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{LinkageTargetId: 0, Limit: 10})
	if err != nil {
		t.Errorf("VehicleFitmentStrategy with 0 linkage err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("VehicleFitmentStrategy with 0 linkage returned %d results, want 0", len(results))
	}
}

// TestVinAssemblyStrategy_ZeroLinkage_ReturnsNil verifies short-circuit.
func TestVinAssemblyStrategy_ZeroLinkage_ReturnsNil(t *testing.T) {
	strategy := &VinAssemblyStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{LinkageTargetId: 0, Limit: 10})
	if err != nil {
		t.Errorf("VinAssemblyStrategy with 0 linkage err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("VinAssemblyStrategy with 0 linkage returned %d results, want 0", len(results))
	}
}

// ─── safetyRank / spec ordering ───────────────────────────────────────────

// TestSafetyRank_CriticalFirst verifies the ordering guarantee for
// FindBySpecMatch: teeth_count and spline_count outrank thread/diameter.
// Wrong teeth count on a timing belt = engine damage — this test locks in
// the invariant that they get filtered FIRST in the SQL query.
func TestSafetyRank_CriticalFirst(t *testing.T) {
	cases := []struct {
		name     string
		specName string
		maxRank  int
	}{
		{"teeth count", "teeth count", 0},
		{"number of teeth", "Number of Teeth", 0},
		{"tooth count", "tooth count", 0},
		{"belt teeth", "belt teeth number", 0},
		{"inner spline count", "Inner Spline Count", 0},
		{"outer spline count", "Outer Spline Count", 0},
		{"thread diameter", "Thread Diameter", 1},
		{"bore diameter", "Bore Diameter", 1},
		{"seat type", "Seat Type", 1},
		{"wire count", "Wire Count", 2},
		{"connector", "Connector Type", 2},
		{"height", "Height", 3},
		{"random", "some random spec", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := safetyRank(tc.specName)
			if got != tc.maxRank {
				t.Errorf("safetyRank(%q) = %d, want %d", tc.specName, got, tc.maxRank)
			}
		})
	}
}

// TestSafetyRank_CriticalBeatsAll verifies critical specs (rank 0) sort
// before non-critical specs (rank ≥ 1) when re-ordered together.
func TestSafetyRank_CriticalBeatsAll(t *testing.T) {
	inputs := []string{"Height", "Thread Diameter", "Teeth Count", "Bore Diameter"}
	ranks := make([]int, len(inputs))
	for i, s := range inputs {
		ranks[i] = safetyRank(s)
	}
	// Teeth Count (index 2) must have the lowest rank
	if ranks[2] != 0 {
		t.Fatalf("Teeth Count rank = %d, want 0", ranks[2])
	}
	if ranks[0] <= ranks[2] {
		t.Errorf("Height (%d) must rank higher than Teeth Count (%d)", ranks[0], ranks[2])
	}
	if ranks[1] <= ranks[2] {
		t.Errorf("Thread Diameter (%d) must rank higher than Teeth Count (%d)", ranks[1], ranks[2])
	}
}

// ─── Circuit breaker ───────────────────────────────────────────────────────

// TestCircuitBreaker_OpensAfterThreshold verifies the breaker trips after
// cbFailureThreshold consecutive failures and blocks the strategy.
func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	s := &SmartSearch{}
	name := "test_strategy"

	// Initially closed
	if s.circuitOpen(name) {
		t.Fatal("circuit should start closed")
	}

	// Record enough failures to trip
	for i := int64(0); i < cbFailureThreshold; i++ {
		s.recordStrategyFailure(name)
	}
	if !s.circuitOpen(name) {
		t.Errorf("circuit should be open after %d failures", cbFailureThreshold)
	}
}

// TestCircuitBreaker_ResetsOnSuccess verifies a success clears failure count
// so subsequent failures start counting from zero again.
func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	s := &SmartSearch{}
	name := "test_strategy_reset"

	// Just under threshold
	for i := int64(0); i < cbFailureThreshold-1; i++ {
		s.recordStrategyFailure(name)
	}
	if s.circuitOpen(name) {
		t.Fatal("circuit tripped early — before threshold")
	}

	// Success should reset the counter
	s.recordStrategySuccess(name)

	// One more failure — should NOT trip since counter was reset
	s.recordStrategyFailure(name)
	if s.circuitOpen(name) {
		t.Errorf("circuit tripped after 1 failure post-reset; expected counter to have reset")
	}
}

// TestCircuitBreaker_PerInstance verifies the state is per-SmartSearch,
// not global — two instances must not share breaker state.
func TestCircuitBreaker_PerInstance(t *testing.T) {
	s1 := &SmartSearch{}
	s2 := &SmartSearch{}
	name := "shared_name"

	// Trip s1
	for i := int64(0); i < cbFailureThreshold; i++ {
		s1.recordStrategyFailure(name)
	}
	if !s1.circuitOpen(name) {
		t.Fatal("s1 should be tripped")
	}
	// s2 must be independent
	if s2.circuitOpen(name) {
		t.Errorf("s2 breaker is open — state leaked from s1 (package-global bug)")
	}
}

// ─── Combined-mode helpers ────────────────────────────────────────────────

// TestStrategyPriority_ParsesFirstStrategy verifies the priority-weighted
// sort uses the FIRST strategy in a comma-joined SourceStrategy string.
func TestStrategyPriority_ParsesFirstStrategy(t *testing.T) {
	priorities := map[string]float64{
		"exact_oem":       1.0,
		"cross_reference": 0.9,
		"spec_match":      0.8,
	}
	cases := []struct {
		source string
		want   float64
	}{
		{"exact_oem", 1.0},
		{"exact_oem,cross_reference", 1.0},
		{"cross_reference,spec_match", 0.9},
		{"spec_match", 0.8},
		{"unknown", 0.5}, // fallback
		{"", 0.5},
	}
	for _, tc := range cases {
		if got := strategyPriority(tc.source, priorities); got != tc.want {
			t.Errorf("strategyPriority(%q) = %f, want %f", tc.source, got, tc.want)
		}
	}
}

// TestMax64_Min64_Bounds verifies the helpers behave as expected.
// max64 for the convergence bonus; min64 for the 1.0 cap.
func TestMax64_Min64_Bounds(t *testing.T) {
	if got := max64(0.5, 0.9); got != 0.9 {
		t.Errorf("max64(0.5, 0.9) = %f, want 0.9", got)
	}
	if got := max64(0.9, 0.5); got != 0.9 {
		t.Errorf("max64(0.9, 0.5) = %f, want 0.9", got)
	}
	if got := min64(0.5, 0.9); got != 0.5 {
		t.Errorf("min64(0.5, 0.9) = %f, want 0.5", got)
	}
	// The convergence bonus cap: max(0.98, 0.98) * 1.05 = 1.029, capped to 1.0
	bonus := min64(max64(0.98, 0.98)*1.05, 1.0)
	if bonus != 1.0 {
		t.Errorf("convergence bonus not capped: got %f, want 1.0", bonus)
	}
}

// ─── Enrichment ────────────────────────────────────────────────────────────

// TestEnrichResults_NoneLevel_Passthrough verifies `level=none` is a no-op:
// results returned unchanged, no goroutines spawned, no enrichment fields set.
func TestEnrichResults_NoneLevel_Passthrough(t *testing.T) {
	s := &SmartSearch{}
	input := []SmartResult{
		{Part: model.Part{LegacyArticleId: 100, ArticleNumber: "A", Description: "d"}},
		{Part: model.Part{LegacyArticleId: 200, ArticleNumber: "B", Description: "e"}},
	}
	out := s.enrichResults(input, "none")
	if len(out) != len(input) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(input))
	}
	for i, r := range out {
		if r.LegacyArticleId != input[i].LegacyArticleId {
			t.Errorf("out[%d].LegacyArticleId = %d, want %d", i, r.LegacyArticleId, input[i].LegacyArticleId)
		}
		if len(r.Specifications) != 0 {
			t.Errorf("out[%d] has %d specs, want 0 (level=none)", i, len(r.Specifications))
		}
	}
}

// TestEnrichResults_EmptyLevel_Passthrough verifies `level=""` behaves like "none".
func TestEnrichResults_EmptyLevel_Passthrough(t *testing.T) {
	s := &SmartSearch{}
	input := []SmartResult{{Part: model.Part{LegacyArticleId: 42}}}
	out := s.enrichResults(input, "")
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].LegacyArticleId != 42 {
		t.Errorf("LegacyArticleId = %d, want 42", out[0].LegacyArticleId)
	}
}

// TestEnrichResults_ZeroArticleId_Skipped verifies results with id <= 0 are
// left untouched (no goroutine, no enrichment attempted).
func TestEnrichResults_ZeroArticleId_Skipped(t *testing.T) {
	s := &SmartSearch{}
	input := []SmartResult{
		{Part: model.Part{LegacyArticleId: 0}},
		{Part: model.Part{LegacyArticleId: -5}},
	}
	out := s.enrichResults(input, "basic")
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	for i, r := range out {
		if len(r.Specifications) != 0 || len(r.CompatibleVehicles) != 0 {
			t.Errorf("out[%d] enriched despite id <= 0", i)
		}
	}
}

// TestEnrichResults_NoServices_NoOp verifies that when TecDoc services are nil,
// enrichment is a safe no-op — no panics, no fields populated.
func TestEnrichResults_NoServices_NoOp(t *testing.T) {
	s := &SmartSearch{} // no tecDocSpecs / tecDocVehicle / etc.
	input := []SmartResult{{Part: model.Part{LegacyArticleId: 100}}}
	out := s.enrichResults(input, "full")
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if len(out[0].Specifications) != 0 {
		t.Errorf("Specifications populated without tecDocSpecs: %v", out[0].Specifications)
	}
	if len(out[0].CompatibleVehicles) != 0 {
		t.Errorf("CompatibleVehicles populated without tecDocVehicle: %v", out[0].CompatibleVehicles)
	}
	if out[0].Supersession != nil {
		t.Errorf("Supersession populated without tecDocSuper")
	}
}

// ─── identifyComponentType / deriveChildSpecs ────────────────────────────

func TestIdentifyComponentType_ByCategory(t *testing.T) {
	cases := []struct {
		category string
		want     string
	}{
		{"strut", "strut"},
		{"Shock Absorber", "strut"},
		{"caliper", "caliper"},
		{"radiator", "radiator"},
		{"alternator", "alternator"},
		{"transmission", "transmission"},
		{"turbo", "turbocharger"},
		{"turbocharger housing", "turbocharger"},
		{"exhaust manifold", "exhaust_manifold"},
		{"steering rack", "steering_rack"},
		{"thermostat housing", "thermostat"},
		{"wheel hub", "wheel_hub"},
		{"wheel bearing", "wheel_hub"},
		{"random category", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.category, func(t *testing.T) {
			got := identifyComponentType(nil, tc.category)
			if got != tc.want {
				t.Errorf("identifyComponentType(nil, %q) = %q, want %q", tc.category, got, tc.want)
			}
		})
	}
}

// TestDeriveChildSpecs_UnknownComponent_ReturnsEmpty verifies the registry
// does NOT invent constraints for unknown parent types.
func TestDeriveChildSpecs_UnknownComponent_ReturnsEmpty(t *testing.T) {
	got := deriveChildSpecs("unknown", []model.Specification{
		{Name: "some spec", Value: "42"},
	})
	if len(got) != 0 {
		t.Errorf("deriveChildSpecs(unknown, ...) = %d constraints, want 0", len(got))
	}
}

// TestDeriveChildSpecs_Strut_MapsBodyDiameterToSpringSeat verifies the strut
// → spring-seat-diameter derivation used by AssemblyContextStrategy.
func TestDeriveChildSpecs_Strut_MapsBodyDiameterToSpringSeat(t *testing.T) {
	got := deriveChildSpecs("strut", []model.Specification{
		{Name: "Body Diameter", Value: "46mm"},
	})
	if len(got) == 0 {
		t.Fatal("deriveChildSpecs(strut, body diameter) = no constraints; want spring seat + inner diameter")
	}
	foundSpringSeat := false
	for _, c := range got {
		if c.specName == "spring seat diameter" && c.specValue == "46mm" {
			foundSpringSeat = true
		}
	}
	if !foundSpringSeat {
		t.Errorf("expected spring-seat constraint derived from strut body diameter; got %+v", got)
	}
}

// TestDeriveChildSpecs_Transmission_CriticalReliability verifies the transmission
// case marks spline count as CRITICAL (matches domain-knowledge.md §1.4).
func TestDeriveChildSpecs_Transmission_CriticalReliability(t *testing.T) {
	got := deriveChildSpecs("transmission", []model.Specification{
		{Name: "input shaft - number of teeth", Value: "23"},
		{Name: "input shaft - spline", Value: "23"},
	})
	if len(got) == 0 {
		t.Fatal("deriveChildSpecs(transmission) returned no constraints")
	}
	criticalSeen := false
	for _, c := range got {
		if c.reliability == "critical" {
			criticalSeen = true
			break
		}
	}
	if !criticalSeen {
		t.Errorf("transmission constraints must include CRITICAL reliability for spline; got %+v", got)
	}
}

// TestConfidence_ByReliability verifies the confidence mapping used to
// stamp results from AssemblyContextStrategy.
func TestConfidence_ByReliability(t *testing.T) {
	cases := []struct {
		reliability string
		want        float64
	}{
		{"critical", 0.90},
		{"high", 0.85},
		{"medium", 0.70},
		{"low", 0.70},
		{"", 0.70},
	}
	for _, tc := range cases {
		if got := confidence(tc.reliability); got != tc.want {
			t.Errorf("confidence(%q) = %f, want %f", tc.reliability, got, tc.want)
		}
	}
}

// ─── isSafetyCritical ─────────────────────────────────────────────────────

// TestIsSafetyCritical_DetectsTeethCount verifies the safety-note trigger
// fires for timing belt teeth count and CV axle spline count.
func TestIsSafetyCritical_DetectsTeethCount(t *testing.T) {
	specs := []model.Specification{{Name: "Number of Teeth", Value: "130"}}
	if !isSafetyCritical(specs) {
		t.Errorf("isSafetyCritical(teeth count) = false, want true")
	}
}

func TestIsSafetyCritical_DetectsSplineCount(t *testing.T) {
	specs := []model.Specification{{Name: "Inner spline count", Value: "23"}}
	if !isSafetyCritical(specs) {
		t.Errorf("isSafetyCritical(spline count) = false, want true (via normalizeSpecName)")
	}
}

func TestIsSafetyCritical_IgnoresNonCritical(t *testing.T) {
	specs := []model.Specification{{Name: "Height", Value: "150mm"}}
	if isSafetyCritical(specs) {
		t.Errorf("isSafetyCritical(height) = true, want false")
	}
}

// ─── New strategy wrappers (S4-T2 completion) ─────────────────────────────

// TestOwnedCatalogStrategy_EmptyInput_ReturnsNil verifies short-circuit
// when neither OEM nor Query is provided.
func TestOwnedCatalogStrategy_EmptyInput_ReturnsNil(t *testing.T) {
	strategy := &OwnedCatalogStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{Limit: 10})
	if err != nil {
		t.Errorf("OwnedCatalogStrategy empty input err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("OwnedCatalogStrategy empty input returned %d, want 0", len(results))
	}
}

// TestKeywordGatedStrategy_EmptyQuery_ReturnsNil verifies short-circuit.
func TestKeywordGatedStrategy_EmptyQuery_ReturnsNil(t *testing.T) {
	strategy := &KeywordGatedStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{Limit: 10})
	if err != nil {
		t.Errorf("KeywordGatedStrategy empty query err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("KeywordGatedStrategy empty query returned %d, want 0", len(results))
	}
}

// TestKeywordGatedStrategy_ConfidenceCapped verifies confidence stays at 0.65
// (the tecdoc_keyword sentinel) so higher-confidence strategies always outrank it.
func TestKeywordGatedStrategy_ConfidenceCapped(t *testing.T) {
	s := &SmartSearch{}
	strategy := &KeywordGatedStrategy{search: s}
	if got := strategy.ConfidenceBase(); got != 0.65 {
		t.Errorf("KeywordGated ConfidenceBase = %f, want 0.65 (tecdoc_keyword sentinel)", got)
	}
	if got := strategy.Priority(); got >= (&ExactOEMStrategy{search: s}).Priority() {
		t.Errorf("KeywordGated Priority (%f) must be < ExactOEM (%f)", got, (&ExactOEMStrategy{search: s}).Priority())
	}
}

// TestOwnedCatalogStrategy_PriorityHigherThanKeyword verifies the ordering:
// owned-catalog exact hits outrank the fuzzy keyword path.
func TestOwnedCatalogStrategy_PriorityHigherThanKeyword(t *testing.T) {
	s := &SmartSearch{}
	owned := (&OwnedCatalogStrategy{search: s}).Priority()
	keyword := (&KeywordGatedStrategy{search: s}).Priority()
	if owned <= keyword {
		t.Errorf("owned_catalog priority (%f) must be > keyword_gated (%f)", owned, keyword)
	}
}

// TestAvailableModes_IncludesNewWrappers confirms owned_catalog and keyword_gated
// are exposed to callers (S4-T2 was previously incomplete).
func TestAvailableModes_IncludesNewWrappers(t *testing.T) {
	s := &SmartSearch{}
	keys := map[string]bool{}
	for _, m := range s.AvailableModes() {
		keys[m.Key] = true
	}
	for _, want := range []string{"owned_catalog", "keyword_gated"} {
		if !keys[want] {
			t.Errorf("AvailableModes() missing %q — S4-T2 wrapper not registered", want)
		}
	}
}

// TestStrategyForMode_ReturnsNewWrappers verifies the mode-router maps to
// the actual struct types.
func TestStrategyForMode_ReturnsNewWrappers(t *testing.T) {
	s := &SmartSearch{}
	if _, ok := s.strategyForMode("owned_catalog").(*OwnedCatalogStrategy); !ok {
		t.Errorf("strategyForMode('owned_catalog') did not return *OwnedCatalogStrategy")
	}
	if _, ok := s.strategyForMode("keyword_gated").(*KeywordGatedStrategy); !ok {
		t.Errorf("strategyForMode('keyword_gated') did not return *KeywordGatedStrategy")
	}
}


// ─── isMostlyDigits guard ─────────────────────────────────────────────────

// TestIsMostlyDigits verifies the guard that stops KeywordGated from
// polluting results with digit-substring matches on OEM numbers.
func TestIsMostlyDigits(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"82460-2T010", true},   // OEM — mostly digits, should be blocked
		{"26350-2J001", true},   // OEM
		{"97133-D3000", true},   // OEM
		{"18855-10080", true},   // OEM
		{"oil filter", false},   // Free text — should keyword search
		{"cabin air filter", false},
		{"MANN W712/4", false},  // Aftermarket brand+number, mixed
		{"BOSCH 0451103314", false}, // Brand name dominates
		{"", false},             // Empty
		{"12345", true},         // Pure digits
		{"abc", false},          // Pure letters
	}
	for _, tc := range cases {
		if got := isMostlyDigits(tc.in); got != tc.want {
			t.Errorf("isMostlyDigits(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestKeywordGatedStrategy_SkipsOEMShapedQuery verifies the pollution fix:
// a real OEM like "82460-2T010" must NOT return keyword matches like
// "Pressure Hose, air compressor" whose article number just happens to
// share the "82460" digit substring.
func TestKeywordGatedStrategy_SkipsOEMShapedQuery(t *testing.T) {
	s := &SmartSearch{}
	strategy := &KeywordGatedStrategy{search: s}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// OEM-shaped input should short-circuit and return nothing.
	results, err := strategy.Search(ctx, StrategyRequest{Query: "82460-2T010", Limit: 10})
	if err != nil {
		t.Errorf("Search err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("KeywordGated returned %d results for OEM-shaped query, want 0 (would be digit-substring pollution)", len(results))
	}
}

// ─── LegacyCascadeStrategy — S4-T2 completion ─────────────────────────────

// TestLegacyCascadeStrategy_NoInputReturnsNil verifies short-circuit when
// neither Query nor LinkageTargetId is provided.
func TestLegacyCascadeStrategy_NoInputReturnsNil(t *testing.T) {
	strategy := &LegacyCascadeStrategy{search: &SmartSearch{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{Limit: 10})
	if err != nil {
		t.Errorf("empty input err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("empty input returned %d results, want 0", len(results))
	}
}

// TestLegacyCascadeStrategy_PriorityRanking verifies legacy outranks
// everything except exact_oem in the combined merge.
func TestLegacyCascadeStrategy_PriorityRanking(t *testing.T) {
	s := &SmartSearch{}
	legacy := (&LegacyCascadeStrategy{search: s}).Priority()
	exact := (&ExactOEMStrategy{search: s}).Priority()
	crossRef := (&CrossReferenceStrategy{search: s}).Priority()
	keyword := (&KeywordGatedStrategy{search: s}).Priority()
	if legacy > exact {
		t.Errorf("legacy priority (%f) must be <= exact_oem (%f)", legacy, exact)
	}
	if legacy <= crossRef {
		t.Errorf("legacy priority (%f) must be > cross_reference (%f) — legacy is the widest net", legacy, crossRef)
	}
	if legacy <= keyword {
		t.Errorf("legacy priority (%f) must be > keyword_gated (%f)", legacy, keyword)
	}
}

// TestStrategyForMode_ReturnsLegacyCascade verifies /?mode=legacy dispatches
// to LegacyCascadeStrategy.
func TestStrategyForMode_ReturnsLegacyCascade(t *testing.T) {
	s := &SmartSearch{}
	if _, ok := s.strategyForMode("legacy").(*LegacyCascadeStrategy); !ok {
		t.Errorf("strategyForMode('legacy') did not return *LegacyCascadeStrategy")
	}
}
