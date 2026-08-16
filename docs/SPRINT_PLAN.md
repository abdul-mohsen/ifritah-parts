# ifritah-parts — Sprint Plan & Agent Rules

## AGENT RULES (mandatory — all agents must follow)
1. NEVER print file contents into the chat. Save all reports, plans, outputs to files.
2. All reports go to: docs/reports/YYYY-MM-DD_<topic>.md
3. All generated test files go to: internal/service/ or internal/handler/
4. Confirm work with one-line summary only: "Saved X to Y"
5. If you need to show a status, write max 5 lines. No walls of text.
6. Every agent task returns: files created/modified, test count delta, pass/fail count.

## Working directory
C:\ssda\chatGPT\ifritah\ifritah-parts.git\parts-engine-baseline

## Current state (as of session end)
- Test runs: 45,786
- Live API accuracy: P=56.9% R=69.8% F1=62.7% Acc=45.7%
- Bugs confirmed: BUG-1 through BUG-12
- Root cause: 11 of 14 TecDoc MySQL tables never queried
- TecDoc IS deployed and connected (tecdoc:true on /health)
- Implementation plan: docs/search-modes-implementation-plan.html

---

## SPRINT 1 — Database Foundation (4 days)
**Goal:** Every TecDoc table gets a dedicated Go query method.
**Outcome:** go test ./internal/service/... -run TestTecDoc passes ≥95% coverage.

### Agent A — TecDoc Cross-Reference methods
Files to create:
- internal/service/tecdoc_crossref.go — SearchCrossReferences(oemNumber) from articlecrosses (30M)
- internal/service/tecdoc_crossref_test.go — table-driven tests, ≥20 real OEM inputs

### Agent B — TecDoc Specification + Documents methods
Files to create:
- internal/service/tecdoc_specifications.go — FindSpecifications(legacyArticleId) from articlecriteria (27M)
- internal/service/tecdoc_documents.go — FindDocuments(legacyArticleId) from articledocs (8.2M)
- internal/service/tecdoc_specifications_test.go
- internal/service/tecdoc_documents_test.go

### Agent C — TecDoc Supersession + Functional Equivalents methods
Files to create:
- internal/service/tecdoc_supersession.go — FindSupersession(legacyArticleId) recursive CTE on replacedbyarticles/replacesarticles
- internal/service/tecdoc_functional.go — FindFunctionalEquivalents(legacyArticleId, vehicleId?) via legacy2generic
- internal/service/tecdoc_supersession_test.go
- internal/service/tecdoc_functional_test.go

### Agent D — TecDoc Vehicle Compatibility + Cross-Brand methods
Files to create:
- internal/service/tecdoc_vehicle.go — FindCompatibleVehicles(legacyArticleId) from articlesvehicletrees (651M)
- internal/service/tecdoc_crossbrand.go — FindCrossBrandEquivalents(oemNumber) Hyundai<->Kia platform
- internal/service/tecdoc_vehicle_test.go
- internal/service/tecdoc_crossbrand_test.go

### Sprint 1 models (create these first)
- internal/model/specification.go — {Name, Value, Unit, CriteriaType}
- internal/model/compatible_vehicle.go — {LinkageTargetId, Vehicle, Make, Model, Years, FuelType}
- internal/model/supersession_chain.go — {Current, ReplacedBy, Replaces, Depth}
- internal/model/document.go — {URL, FileName, DocType, Language}

---

## SPRINT 2 — Search Strategy Layer (8 days)
**Goal:** 8 search strategies implement SearchStrategy interface. Tests pass per strategy.
**Outcome:** Each strategy independently queryable, F1 target per strategy met.

### Agent A — Strategy interface + Exact OEM + Cross-Reference strategies
Files to create:
- internal/service/strategy.go — SearchStrategy interface, SearchRequest, SearchResult, registry
- internal/service/strategy_exact_oem.go
- internal/service/strategy_crossref.go
- internal/service/strategy_test.go — interface contract tests

### Agent B — Vehicle Fitment + Specification strategies
Files to create:
- internal/service/strategy_vehicle.go
- internal/service/strategy_specification.go
- internal/service/strategy_vehicle_test.go
- internal/service/strategy_specification_test.go

### Agent C — Functional Equivalents + Supersession + Cross-Brand strategies
Files to create:
- internal/service/strategy_functional.go
- internal/service/strategy_supersession.go
- internal/service/strategy_crossbrand.go
- internal/service/strategy_functional_test.go
- internal/service/strategy_supersession_test.go
- internal/service/strategy_crossbrand_test.go

### Agent D — Keyword (gated) strategy + SmartSearch dispatcher
Files to modify/create:
- internal/service/strategy_keyword.go — category-token gate to prevent BUG-1
- internal/service/smart_search.go — add SearchMode dispatch, kill tecdoc_keyword fallback for OEM queries
- internal/service/strategy_keyword_test.go
- internal/service/smart_search_dispatch_test.go

---

## SPRINT 3 — API + Response Schema (3 days)
**Goal:** ?mode= param works, response includes specs/compat/images/supersession.
**Outcome:** All 9 modes queryable via API, response schema matches TecDoc detail level.

### Agent A — API mode parameter + modes endpoint
Files to modify:
- internal/handler/search.go — add ?mode= param parsing, delegate to strategy registry
- internal/handler/search_test.go — test all 9 modes return correct shape

### Agent B — SmartResult enrichment + response schema
Files to modify:
- internal/service/smart_search.go — add compatibility, specifications, images, supersession, functionalEquivalents to SmartResult
- internal/model/part.go — extend SmartResult struct
- internal/handler/search.go — add enrichmentLevel param (none|basic|full)

### Agent C — Golden cases update + qa_gate per-mode support
Files to modify:
- qa/golden_cases.json — update 60 existing cases to use mode param, add expectedSearchStrategy
- cmd/qa_gate/main.go — add --mode flag, per-mode pass/fail tracking
- docs/reports/ — qa_gate run report saved here

---

## SPRINT 4 — Smart Search Combined Mode (4 days)
**Goal:** Fan-out over 7 strategies in parallel, merge/rank/dedupe.
**Outcome:** Smart Search P95 < 3s, F1 ≥ 91%.

### Agent A — Fan-out engine + deduplication
Files to create:
- internal/service/smart_search_combined.go — parallel fan-out, context timeout budget, result merge
- internal/service/smart_search_combined_test.go

### Agent B — Weighted ranking + circuit breaker
Files to create:
- internal/service/smart_search_ranker.go — combinedScore(), priority weights per strategy
- internal/service/smart_search_breaker.go — circuit breaker, skip failing strategies
- internal/service/smart_search_ranker_test.go

---

## SPRINT 5 — Frontend (3 days)
**Goal:** Dropdown with 9 modes, strategy badge on each result card, new fields rendered.
**Outcome:** Users can switch modes, see strategy source, view specs/compat/images.

### Agent A — SearchModeSelector + StrategyBadge components
Files to create:
- rontend/src/components/SearchModeSelector.tsx
- rontend/src/components/StrategyBadge.tsx

### Agent B — New result card fields
Files to create:
- rontend/src/components/SpecificationTable.tsx
- rontend/src/components/CompatibilityChips.tsx
- rontend/src/components/ImagesCarousel.tsx
- rontend/src/components/SupersessionChain.tsx
Files to modify:
- rontend/src/components/ResultCard.tsx
- rontend/src/components/SearchBox.tsx

---

## SPRINT 6 — Testing & QA (4 days)
**Goal:** 800+ golden cases covering all 9 modes. Per-mode scorecard in CI.
**Outcome:** All mode F1 targets met. Report saved to docs/reports/.

### Agent A — Golden case expansion (100 cases per mode)
Files to modify:
- qa/golden_cases.json — grow to 800+ cases
Save report to: docs/reports/golden_cases_expansion.md

### Agent B — Per-mode qa_gate run + accuracy report
Run: go run ./cmd/qa_gate --all-modes
Save report to: docs/reports/per_mode_accuracy.md

### Agent C — Performance benchmarks
Run: latency P50/P95/P99 per mode
Save report to: docs/reports/performance_benchmarks.md

### Agent D — Load test
Run: 100 concurrent Smart Search queries
Save report to: docs/reports/load_test_results.md

---

## SPRINT 7 — Rollout (2 days)
**Goal:** Feature flags, canary, dashboards, alerts.
**Outcome:** All 9 modes deployed, 7 days green metrics.

### Agent A — Feature flags
Files to create:
- internal/config/features.go — per-mode enable/disable flags via env vars

### Agent B — Monitoring + runbook
Files to create:
- docs/runbook.md — how to disable a broken mode without redeploy
- docs/monitoring.md — dashboard queries, alert thresholds

---

## Success targets
| Metric | Current | Sprint 7 target |
|--------|---------|-----------------|
| F1 | 62.7% | ≥91% |
| Precision | 56.9% | ≥92% |
| Recall | 69.8% | ≥90% |
| FP Rate | 100% | <10% |
| Aftermarket alts/part | 3-5 | 30-50 |
| Compatibility populated | 0% | 100% |
| Specs populated | 0% | 100% |
| Images populated | 0% | 100% |
| P95 latency (Smart) | >20s | <2s |
| Test runs | 45,786 | ≥50,000 |

---
