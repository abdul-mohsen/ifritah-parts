# Parts Search Engine — Sprint Plan

**Goal:** Bring `qa.ifritah.com` from **F1 62.7%** to **F1 ≥ 91%** (TecDoc parity)  
**Duration:** 6 sprints × 1 week each = **6 weeks**  
**Team:** Up to 6 agents working in parallel across independent tracks per sprint

---

## Table of Contents

- [Success Definition](#success-definition)
- [Agent Roles](#agent-roles)
- [Sprint Overview](#sprint-overview)
- [Sprint 0 — Setup & Baseline](#sprint-0--setup--baseline)
- [Sprint 1 — TecDoc DB Query Layer](#sprint-1--tecdoc-db-query-layer)
- [Sprint 2 — Search Strategy Framework](#sprint-2--search-strategy-framework)
- [Sprint 3 — API + Response Enrichment](#sprint-3--api--response-enrichment)
- [Sprint 4 — Smart Search + Frontend](#sprint-4--smart-search--frontend)
- [Sprint 5 — Testing & QA Gate](#sprint-5--testing--qa-gate)
- [Sprint 6 — Rollout & Monitoring](#sprint-6--rollout--monitoring)
- [Cross-Sprint Dependencies](#cross-sprint-dependencies)
- [Definition of Done](#definition-of-done)

---

## Success Definition

Sprint work is complete when **all** of these are true, measured by the production QA gate on `qa.ifritah.com`:

| Metric | Baseline (today) | Target |
|--------|-----------------:|-------:|
| Live API Precision | 56.9% | ≥ 92% |
| Live API Recall | 69.8% | ≥ 90% |
| F1 Score | 62.7% | ≥ 91% |
| Accuracy | 45.7% | ≥ 88% |
| FP Rate | 100% | < 10% |
| Expected brand coverage | 7.6% | ≥ 60% |
| Categories at ≥60% brand coverage | 3/57 | ≥ 45/57 |
| Compatibility populated per result | 0% | 100% |
| Specifications populated per result | 0% | 100% |
| Product images populated per result | 0% | 100% |
| Supersession chain populated | 0% | 100% |
| P95 latency (Smart Search) | timeout | < 2s |
| Search modes exposed to user | 1 (hidden) | 9 (dropdown) |

---

## Agent Roles

Each sprint has multiple **independent tracks** that different agents can pick up in parallel.

| Role | Responsibility |
|------|----------------|
| **Backend-DB** | MySQL query methods, schema, connection pooling, indexes |
| **Backend-Service** | Business logic, strategy interfaces, merging, ranking |
| **Backend-API** | HTTP handlers, request/response schemas, OpenAPI docs |
| **Frontend** | React components, dropdown UI, result cards, e2e tests |
| **QA** | Golden cases, accuracy scoring, cross-mode consistency |
| **DevOps** | Feature flags, deployment, dashboards, alerts |

Each task tagged with role. Agents claim tracks per sprint. **No two agents work on the same file simultaneously** — coordinate via git branch names `sprint-N/role-X-taskname`.

---

## Sprint Overview

| Sprint | Focus | Duration | Parallel tracks |
|--------|-------|----------|----------------:|
| 0 | Setup, baseline, tooling | 3 days | 3 |
| 1 | TecDoc query layer (11 tables) | 5 days | 4 |
| 2 | 8 strategies + Smart Search skeleton | 5 days | 4 |
| 3 | API + response enrichment | 5 days | 3 |
| 4 | Smart Search merger + Frontend dropdown | 5 days | 4 |
| 5 | Testing, QA gate, benchmarks | 5 days | 3 |
| 6 | Rollout, monitoring, docs | 3 days | 3 |
| **Total** | | **~31 days** | |

### PR Sprint Label Map

Commit history and PR #9 use granular labels S0–S8. The table below maps those
labels to the plan sprints above so reviewers can navigate without confusion.

| PR label | Plan sprint | What shipped |
|----------|-------------|--------------|
| S0 | Sprint 0 | P0 bug fixes: tecdoc_keyword gate, dedup, vehicle+text fallback, OEM normalisation, internal auth |
| S1 | Sprint 1 | TecDoc query layer — 7 services (crossref, specs, docs, supersession, functional, vehicles, crossbrand), 60 tests |
| S2 | Sprint 2 | Wire articlecrosses (30M rows), fix BUG-4/BUG-6, MySQL connection pooling |
| S3 | Sprint 3 | Enrichment pipeline — SmartResult extended, parallel fan-out, `/api/search/modes` |
| S4 | Sprint 4 | Strategy interface + searchCombined fan-out, circuit breaker per-instance |
| S5 | Sprint 4 | Frontend — SearchModeSelector, StrategyBadge, SpecificationTable, CompatibilityChips, SupersessionChain, OemSearch wired |
| S6 | Sprint 4 | spec_match strategy + SpecMatchStrategy safety ordering |
| S7 | Sprint 4 | assembly_context strategy + AssemblySpec registry |
| S8 | Sprint 4 | vin_assembly strategy — LinkageTargetToSpecs, VehicleConfigurator badge |
| (review) | Sprint 5 (partial) | Review fixes: ConstantTimeCompare, 400 on bad mode/level, safety sorting, enrichment deadline, headers, ~30 tests |

Sprint 5 (full QA gate: 900+ golden cases, per-mode scoring, Playwright e2e) and Sprint 6 (rollout) remain as future work.

---

## Sprint 0 — Setup & Baseline

**Duration:** 3 days  
**Goal:** Every agent has a reproducible dev environment. Baseline metrics captured for later comparison.

### Track A — Environment (Backend-DB agent)

- [ ] **A0.1** Verify MySQL TecDoc connection string, credentials in `.env.local`
- [ ] **A0.2** Confirm all 14 TecDoc tables accessible: `oem_number`, `articles`, `ambrand`, `articlecrosses`, `articlesvehicletrees`, `articlecriteria`, `articledocs`, `articlepdfs`, `replacedbyarticles`, `replacesarticles`, `legacy2generic`, `genericarticlesgroups`, `oemnumbers`, `searchindex`
- [ ] **A0.3** Row-count each table, document in `docs/db-inventory.md`
- [ ] **A0.4** Set up local MySQL mirror for tests (or use test slice)

**Exit criteria:** `go test -tags integration ./...` runs against local DB. Every table returns > 0 rows for a smoke query.

### Track B — CI baseline (QA agent)

- [ ] **B0.1** Run current `qa_gate` against `qa.ifritah.com`, save output as `baseline/qa-report-YYYY-MM-DD.json`
- [ ] **B0.2** Capture per-category scorecard (Precision/Recall/F1/Accuracy)
- [ ] **B0.3** Document baseline in `docs/baseline-metrics.md`
- [ ] **B0.4** Add nightly CI job that runs `qa_gate` and compares to baseline

**Exit criteria:** `docs/baseline-metrics.md` committed with today's snapshot. CI job runs green and fails on regression.

### Track C — Feature flag scaffolding (DevOps agent)

- [ ] **C0.1** Add `FEATURE_FLAGS` env var support to `internal/config/config.go`
- [ ] **C0.2** Add feature flag registry: `SMART_SEARCH_ENABLED`, `MODE_EXACT_OEM`, `MODE_CROSSREF`, `MODE_VEHICLE`, `MODE_SPEC`, `MODE_FUNCTIONAL`, `MODE_SUPERSESSION`, `MODE_CROSSBRAND`, `MODE_KEYWORD`
- [ ] **C0.3** Default all NEW flags to OFF in production
- [ ] **C0.4** Add `GET /api/features` endpoint returning current flag state

**Exit criteria:** `curl /api/features` returns JSON list of flags with default states.

---

## Sprint 1 — TecDoc DB Query Layer

**Duration:** 5 days  
**Goal:** Every one of the 11 unused TecDoc tables has a dedicated Go query method with timeout, logging, and unit tests.

### Track A — OEM cross-reference (Backend-DB agent)

- [ ] **1A.1** Add `TecDoc.SearchCrossReferences(ctx, oemNumber, limit)` — queries `articlecrosses` (30M)
- [ ] **1A.2** Return `[]model.OEMReference` with brand + article number
- [ ] **1A.3** Timeout: 2 seconds
- [ ] **1A.4** Unit test with fixtures: `26300-35505` returns ≥ 6 alternatives
- [ ] **1A.5** Benchmark: single query < 200ms P95

**File:** `internal/service/tecdoc.go` (add method)  
**Exit criteria:** `go test -run TestTecDoc_SearchCrossReferences` passes; latency < 200ms.

### Track B — Vehicle fitment (Backend-DB agent)

- [ ] **1B.1** Add `TecDoc.FindCompatibleVehicles(ctx, legacyArticleId)` — queries `articlesvehicletrees` (651M) → `linkagetargets`
- [ ] **1B.2** Return `[]model.CompatibleVehicle` with `{linkageTargetId, make, model, yearFrom, yearTo, engine}`
- [ ] **1B.3** Add `TecDoc.PartsForVehicleWithCategory(ctx, linkageTargetId, category)` — parts filtered by assembly group
- [ ] **1B.4** Unit tests with real vehicles (Tucson TL 10001)
- [ ] **1B.5** Benchmark: vehicle→parts query < 500ms P95

**Files:** `internal/service/tecdoc.go`, `internal/model/compatible_vehicle.go`  
**Exit criteria:** `TestTecDoc_FindCompatibleVehicles` returns ≥ 3 vehicles for oil filter W 811/80.

### Track C — Specifications + Documents (Backend-DB agent)

- [ ] **1C.1** Add `TecDoc.FindSpecifications(ctx, legacyArticleId)` — queries `articlecriteria` (27M) → `criteria` → `keyvalues`
- [ ] **1C.2** Return `[]model.Specification` with `{name, value, unit, criteriaType}`
- [ ] **1C.3** Add `TecDoc.FindDocuments(ctx, legacyArticleId)` — queries `articledocs` (8.2M) + `articlepdfs`
- [ ] **1C.4** Return `[]model.Document` with `{url, type, filename}`
- [ ] **1C.5** Unit tests: oil filter W 811/80 has thread size, height, diameter specs

**Files:** `internal/service/tecdoc_specifications.go`, `internal/model/specification.go`  
**Exit criteria:** Every seed OEM returns ≥ 3 specs when data exists.

### Track D — Supersession + Functional Equivalents (Backend-DB agent)

- [ ] **1D.1** Add `TecDoc.FindSupersession(ctx, legacyArticleId)` — recursive CTE over `replacedbyarticles`
- [ ] **1D.2** Add `TecDoc.FindReplacesArticles(ctx, legacyArticleId)` — over `replacesarticles`
- [ ] **1D.3** Return `[]model.SupersessionLink` with `{articleNumber, brand, depth, direction}`
- [ ] **1D.4** Add `TecDoc.FindFunctionalEquivalents(ctx, legacyArticleId, vehicleId)` — via `legacy2generic` join
- [ ] **1D.5** Add `TecDoc.FindBySpecification(ctx, criteria...)` — reverse spec lookup
- [ ] **1D.6** Add `TecDoc.FindCrossBrandEquivalents(ctx, oemNumber)` — Hyundai↔Kia platform join

**Files:** `internal/service/tecdoc_supersession.go`, `internal/service/tecdoc_crossbrand.go`  
**Exit criteria:** Recursive supersession terminates at depth ≤ 10; cross-brand returns pairs.

### Sprint 1 Exit Criteria (all tracks)

- All 11 previously-unused TecDoc tables have a dedicated query method
- Every method has unit tests with real fixtures
- Every method has 2-second timeout and structured logging
- Combined test coverage on `tecdoc.go` ≥ 90%
- `go test ./internal/service/... -run TestTecDoc` all green
- No new methods called from application code yet (pure plumbing layer)

---

## Sprint 2 — Search Strategy Framework

**Duration:** 5 days  
**Goal:** 8 individual search modes implemented as hot-swappable `SearchStrategy` interface. Category-token gate prevents BUG-1 style contamination.

### Track A — Strategy interface + registry (Backend-Service agent)

- [ ] **2A.1** Define `SearchStrategy` interface in `internal/service/strategy.go`
- [ ] **2A.2** Define `SearchRequest` struct with all inputs
- [ ] **2A.3** Define `SearchResult` struct with metadata (`strategyUsed`, `latency`, `warnings`)
- [ ] **2A.4** Build strategy registry: `map[string]SearchStrategy` populated at startup
- [ ] **2A.5** Add `SmartSearch.Search(request)` dispatch by request.Mode

**File:** `internal/service/strategy.go`  
**Exit criteria:** Interface compiles; registry serves 8 stub strategies.

### Track B — OEM-based strategies (Backend-Service agent)

- [ ] **2B.1** `ExactOEMStrategy` — wraps `OEMLookup.Search` + `TecDoc.SearchByOEM`
- [ ] **2B.2** `CrossReferenceStrategy` — wraps `TecDoc.SearchCrossReferences`
- [ ] **2B.3** `SupersessionStrategy` — wraps `TecDoc.FindSupersession`
- [ ] **2B.4** `FunctionalEquivalentStrategy` — wraps `TecDoc.FindFunctionalEquivalents`
- [ ] **2B.5** Unit test each with 5 seed OEMs

**Files:** `internal/service/strategy_exact_oem.go`, `strategy_crossref.go`, `strategy_supersession.go`, `strategy_functional.go`

### Track C — Vehicle/Spec strategies (Backend-Service agent)

- [ ] **2C.1** `VehicleFitmentStrategy` — wraps `TecDoc.PartsForVehicle` + category filter
- [ ] **2C.2** `SpecificationStrategy` — parses spec criteria, calls `TecDoc.FindBySpecification`
- [ ] **2C.3** `CrossBrandStrategy` — wraps `TecDoc.FindCrossBrandEquivalents`
- [ ] **2C.4** Unit test each: Tucson 10001 returns ≥ 20 parts; M20×1.5 thread returns oil filters

**Files:** `internal/service/strategy_vehicle.go`, `strategy_specification.go`, `strategy_crossbrand.go`

### Track D — Keyword fallback + gate (Backend-Service agent)

- [ ] **2D.1** `KeywordFulltextStrategy` — wraps `TecDoc.SearchByKeyword` with FULLTEXT
- [ ] **2D.2** Category-token gate: derive expected tokens from query, filter results
- [ ] **2D.3** Confidence sentinel: 0.65 (marks as fallback quality)
- [ ] **2D.4** Regression test: "oil filter" no longer returns "Fuel filter"
- [ ] **2D.5** Regression test: "cabin air filter" no longer returns "WITHOUT CABIN FILTER"

**File:** `internal/service/strategy_keyword.go`  
**Exit criteria:** BUG-1 and BUG-6 no longer reproduce in `strategy_keyword_test.go`.

### Sprint 2 Exit Criteria

- 8 strategies implemented, each independently testable
- `SearchStrategy` interface stable
- Category-token gate eliminates BUG-1 false positives
- `go test ./internal/service/... -run TestStrategy` all green
- Combined test count grows by ≥ 200

---

## Sprint 3 — API + Response Enrichment

**Duration:** 5 days  
**Goal:** API exposes `?mode=` parameter and returns enriched responses with all 5 new fields (compatibility, specifications, images, supersession, functional equivalents).

### Track A — API contract (Backend-API agent)

- [ ] **3A.1** Extend `GET /api/search` to accept `?mode=exact_oem|crossref|vehicle|spec|functional|supersession|crossbrand|keyword|smart`
- [ ] **3A.2** Add `GET /api/search/modes` — returns list of supported modes with description
- [ ] **3A.3** Add `?enrichmentLevel=none|basic|full` param for response weight control
- [ ] **3A.4** Update OpenAPI spec at `docs/openapi.yaml`
- [ ] **3A.5** Backward compat: default mode = `smart`, default enrichment = `basic`

**Files:** `internal/handler/search.go`, `docs/openapi.yaml`  
**Exit criteria:** `curl /api/search/modes` returns 9 entries. Existing clients see richer responses without breaking.

### Track B — Response enrichment (Backend-Service agent)

- [ ] **3B.1** Extend `SmartResult` struct with fields: `Compatibility`, `Specifications`, `Images`, `Supersession`, `FunctionalEquivalents`
- [ ] **3B.2** Post-process every returned `SmartResult`: parallel-fan-out to 5 Stage-1 methods
- [ ] **3B.3** Skip enrichment when `enrichmentLevel=none`
- [ ] **3B.4** Enforce 500ms budget for enrichment layer; partial results OK
- [ ] **3B.5** Wire enrichment into all 8 strategies uniformly

**Files:** `internal/service/smart_search.go`, `internal/model/smart_result.go`  
**Exit criteria:** Sample query returns compat + specs + images + supersession + equivalents.

### Track C — Handler + validation (Backend-API agent)

- [ ] **3C.1** Validate `mode` param — return 400 with error list if invalid
- [ ] **3C.2** Validate `enrichmentLevel` param
- [ ] **3C.3** Rate limit per IP (100 req/min) — reuse existing middleware or add one
- [ ] **3C.4** Handler unit tests for all 9 modes × 3 enrichment levels = 27 combinations
- [ ] **3C.5** Add response header `X-Search-Strategy: <mode>` for observability

**Files:** `internal/handler/search.go`, `internal/handler/search_test.go`  
**Exit criteria:** All 27 mode×enrichment combinations tested; invalid inputs return 400.

### Sprint 3 Exit Criteria

- API accepts `?mode=` and `?enrichmentLevel=` params
- Response schema includes 5 new fields
- Every OEM in seed catalog returns non-empty specs (when data exists)
- Compatibility populated 100% for parts with vehicle linkage
- OpenAPI documented; Swagger UI renders correctly
- No regression in existing client behavior (default mode = smart)

---

## Sprint 4 — Smart Search + Frontend

**Duration:** 5 days  
**Goal:** Smart Search combined mode running with fan-out + merge + rank. Frontend dropdown UI shipped with 9-mode selector.

### Track A — Smart Search merger (Backend-Service agent)

- [ ] **4A.1** Implement `SmartSearchStrategy.Search` with parallel fan-out
- [ ] **4A.2** 3-second hard budget via `context.WithTimeout`
- [ ] **4A.3** Deduplication by `legacyArticleId` (keep highest-confidence)
- [ ] **4A.4** Weighted ranking: `score = strategyConfidence × priorityWeight × matchBonus`
- [ ] **4A.5** Match bonus: +5% if same result appears in ≥ 2 strategies
- [ ] **4A.6** Circuit breaker per strategy (3 fails in 60s = skip for next 60s)
- [ ] **4A.7** Telemetry: `strategyLatencies`, `resultsPerStrategy`, `dedupCount`

**File:** `internal/service/strategy_smart.go`  
**Exit criteria:** Smart Search returns merged results in < 3s P95 for 10 concurrent queries.

### Track B — Frontend dropdown (Frontend agent)

- [ ] **4B.1** `SearchModeSelector` component — fetches `GET /api/search/modes` at load
- [ ] **4B.2** Dropdown persists selection in localStorage
- [ ] **4B.3** URL query-param support: `?mode=crossref` deep-links to mode
- [ ] **4B.4** Tooltip per mode explaining what it does
- [ ] **4B.5** Default: "Smart Search"

**Files:** `frontend/src/components/SearchModeSelector.tsx`, `frontend/src/hooks/useSearchMode.ts`  
**Exit criteria:** Dropdown renders 9 modes; selection persists across reloads.

### Track C — Frontend result card enrichment (Frontend agent)

- [ ] **4C.1** `StrategyBadge` component — pill on each card showing source strategy
- [ ] **4C.2** `SpecificationTable` component — renders specs list
- [ ] **4C.3** `CompatibilityChips` component — clickable vehicle chips
- [ ] **4C.4** `ImagesCarousel` component — thumbnail + lightbox
- [ ] **4C.5** `SupersessionChain` component — arrow visualization of old/new parts
- [ ] **4C.6** Skeleton loaders per section (some fields load slower)

**Files:** `frontend/src/components/*.tsx`  
**Exit criteria:** Every result card renders 5 new sections when data available; skeletons shown while loading.

### Track D — Frontend integration + e2e (Frontend agent)

- [ ] **4D.1** Update `SearchBox.tsx` to pass mode to API
- [ ] **4D.2** Update `ResultCard.tsx` to use new sub-components
- [ ] **4D.3** "Strategies used" summary bar above results
- [ ] **4D.4** Cypress e2e tests for each of 9 modes
- [ ] **4D.5** Accessibility: keyboard nav, aria-labels

**Files:** `frontend/src/components/SearchBox.tsx`, `ResultCard.tsx`, `frontend/cypress/e2e/*.cy.ts`  
**Exit criteria:** All 9 modes return expected UI in Cypress; a11y audit passes.

### Sprint 4 Exit Criteria

- Smart Search runs 7 strategies in parallel, merges results in < 3s
- Frontend dropdown allows selecting any of 9 modes
- Every result card renders specs, compatibility, images, supersession, equivalents
- URL supports `?mode=` deep links
- Cypress e2e green for all 9 modes

---

## Sprint 5 — Testing & QA Gate

**Duration:** 5 days  
**Goal:** Per-mode Precision/Recall/F1/Accuracy scorecards. Cross-mode consistency verified. Performance benchmarks meet targets.

### Track A — Golden case expansion (QA agent)

- [ ] **5A.1** Add 100 golden cases per mode × 8 modes = 800 total
- [ ] **5A.2** Add 100 Smart Search cases
- [ ] **5A.3** Cover all 57 categories with ≥ 15 samples each
- [ ] **5A.4** Include 60 True-Negative cases (Toyota/BMW OEMs, garbage strings)
- [ ] **5A.5** Golden cases in `qa/golden_cases.json` with expected articles, brands, specs

**File:** `qa/golden_cases.json`  
**Exit criteria:** File contains ≥ 900 cases across 57 categories.

### Track B — Per-mode scorecards (QA agent)

- [ ] **5B.1** Extend `qa_gate` to run every case in every mode
- [ ] **5B.2** Compute per-mode Precision/Recall/F1/Accuracy
- [ ] **5B.3** Assert per-mode targets:
  - Exact OEM ≥ 95% precision, ≥ 80% recall
  - Cross-reference ≥ 90% / ≥ 85%
  - Vehicle fitment ≥ 90% / ≥ 85%
  - Specifications ≥ 85% / ≥ 70%
  - Functional equivalents ≥ 85% / ≥ 80%
  - Supersession ≥ 95% / ≥ 70%
  - Cross-brand ≥ 85% / ≥ 70%
  - Keyword (gated) ≥ 70% / ≥ 90%
  - **Smart Search ≥ 92% / ≥ 90% / F1 ≥ 91%**
- [ ] **5B.4** Fail CI if any mode below target

**File:** `cmd/qa_gate/main.go`, `qa/current_impl_quality.json`  
**Exit criteria:** Nightly CI publishes per-mode scorecard; Smart Search meets F1 ≥ 91%.

### Track C — Performance + load (DevOps + QA)

- [ ] **5C.1** Add `hey` or `wrk` benchmark harness to CI
- [ ] **5C.2** Measure P50/P95/P99 latency per mode
- [ ] **5C.3** Load test: 100 concurrent Smart Search queries → no timeouts, P95 < 2s
- [ ] **5C.4** Cross-mode consistency check: same query in 2 modes returns overlapping (not conflicting) results
- [ ] **5C.5** Publish benchmark report to `docs/benchmarks.md`

**Files:** `scripts/bench/*.sh`, `docs/benchmarks.md`  
**Exit criteria:** Load test passes 100 concurrent queries in < 2s P95.

### Sprint 5 Exit Criteria

- 900+ golden cases loaded
- Per-mode scorecard published
- Smart Search hits F1 ≥ 91% target
- Load test proves P95 < 2s for 100 concurrent Smart Search queries
- Cross-mode consistency verified

---

## Sprint 6 — Rollout & Monitoring

**Duration:** 3 days  
**Goal:** All 9 modes live in production. Smart Search rolled out to 100% traffic via canary. Dashboards + alerts operational.

### Track A — Canary rollout (DevOps agent)

- [ ] **6A.1** Enable Smart Search for 5% of traffic via feature flag
- [ ] **6A.2** Monitor error rate, latency, satisfaction metrics for 24 hours
- [ ] **6A.3** Scale to 25% if healthy
- [ ] **6A.4** Scale to 100% after 48 hours of green metrics
- [ ] **6A.5** A/B test: track query-success rate on old cascade vs Smart Search

**Files:** `deployments/config-map.yaml`, `docs/runbook-canary.md`  
**Exit criteria:** 100% traffic on Smart Search with zero P0 alerts for 24 hours.

### Track B — Dashboards + alerts (DevOps agent)

- [ ] **6B.1** Grafana dashboard: per-mode QPS, P95 latency, error rate
- [ ] **6B.2** Grafana dashboard: cache hit rate, DB pool utilization
- [ ] **6B.3** Grafana dashboard: strategy contribution (which strategies match most)
- [ ] **6B.4** Alert: P95 latency > 3s on Smart Search
- [ ] **6B.5** Alert: error rate > 1% on any mode
- [ ] **6B.6** Alert: circuit breaker tripped on any strategy

**Files:** `deployments/grafana/*.json`, `deployments/alerts/*.yaml`  
**Exit criteria:** Dashboards live; test alerts fired successfully.

### Track C — Documentation (Backend-API agent)

- [ ] **6C.1** User-facing help page explaining each mode
- [ ] **6C.2** API reference with curl examples per mode
- [ ] **6C.3** Runbook: how to disable a broken mode via feature flag
- [ ] **6C.4** Runbook: incident response for latency spike
- [ ] **6C.5** Retrospective template for post-launch review

**Files:** `docs/user-guide.md`, `docs/api-reference.md`, `docs/runbook-*.md`  
**Exit criteria:** All docs merged; help page linked from UI.

### Sprint 6 Exit Criteria

- Smart Search on 100% traffic
- 24 hours of green metrics
- All dashboards live
- All docs merged
- Retrospective scheduled

---

## Cross-Sprint Dependencies

```
Sprint 0 (Setup + Baseline)
       │
       ▼
Sprint 1 (DB query layer)  ─────┐
       │                        │
       ▼                        │
Sprint 2 (Strategies)  ─────────┤
       │                        │
       ▼                        │
Sprint 3 (API + Enrichment) ────┤
       │                        │
       ├── Track A backend      │
       │                        │
       ▼                        │
Sprint 4 (Smart + Frontend) ────┤
       │                        │
       ├── Track A: merger ─────┤
       ├── Track B: dropdown ───┤ ← runs in parallel with backend
       ├── Track C: cards ──────┤
       ├── Track D: e2e ────────┤
       │                        │
       ▼                        │
Sprint 5 (Testing + QA gate)  ──┤
       │                        │
       ▼                        │
Sprint 6 (Rollout + Monitor)  ──┘
```

**Parallelism max points:**
- Sprint 1: 4 agents on 4 independent tracks (DB query methods don't depend on each other)
- Sprint 2: 4 agents on 4 strategy groupings
- Sprint 4: Backend Smart Search + 3 frontend agents in parallel
- Sprint 5: QA + DevOps in parallel

**Serial bottlenecks:**
- Sprint 1 must complete before Sprint 2 (strategies need query methods)
- Sprint 2 must complete before Sprint 3 (API needs strategies)
- Sprint 3 must complete before Sprint 4 (Smart Search + UI need API)

---

## Definition of Done

A sprint is **done** when:

1. All tasks in each track marked complete (checked in this file)
2. All exit criteria met and verifiable via commands documented in the sprint
3. Unit tests pass: `go test ./... -count=1`
4. Frontend tests pass: `npm test` and `npx cypress run`
5. QA gate passes on staging: `./cmd/qa_gate -corpus qa/golden_cases.json`
6. Code reviewed by at least one other agent
7. Changes merged to `main` behind feature flags
8. Sprint retro written to `docs/sprint-N-retro.md`

The **project** is done when:

- Sprint 6 exit criteria met
- Live API scorecard on `qa.ifritah.com` shows F1 ≥ 91%
- All 9 modes exposed via UI dropdown
- Zero P0 alerts for 7 consecutive days

---

## Quick-Start for New Agents

Onboarding steps for an agent claiming a track:

1. **Read** `docs/baseline-metrics.md` — know current state
2. **Read** `docs/db-inventory.md` — know table row counts
3. **Pick** a track from the current sprint (only unclaimed ones)
4. **Branch** `git checkout -b sprint-N/role-taskname`
5. **Implement** per the checklist
6. **Test** `go test ./internal/... -run YourTest`
7. **Update** this README — check off completed tasks
8. **PR** with title `[sprint-N] Task 1A.2 — Add SearchCrossReferences`

---

**Version:** 1.0  
**Last updated:** 2026-08-16  
**Owner:** Parts Engine team  
**Repo:** ifritah-parts / qa.ifritah.com
