# Sprint Plan — Multi-Strategy Parts Search

> **Goal:** Take the ifritah-parts search engine from **F1 62.7% → 91%+** by wiring the 11 currently-unused TecDoc tables and exposing every strategy as a user-selectable mode.

---

## 🚨 AGENT WORKING RULES — READ BEFORE STARTING

Every agent working on this project MUST follow these rules. No exceptions.

### Rule 1 — Save reports to files, don't fill the chat
- All analysis output, review findings, coverage reports, benchmarks, and audit results go into a **file under `docs/reports/`**.
- The chat message should contain only a **one-line pointer** to the saved file (e.g. `Report saved to docs/reports/2026-08-15-brand-coverage.md`).
- Never dump multi-hundred-line tables, JSON blobs, or full HTML into the chat.

### Rule 2 — One file, one topic
- `docs/reports/YYYY-MM-DD-<topic>.md` — dated topic report
- `docs/plans/<name>.md` — plan documents
- `docs/specs/<component>.md` — component specifications
- Never mix report + plan + code in one file.

### Rule 3 — Update this file when a task ships
- When a Sprint task is complete, flip its checkbox from `[ ]` to `[x]` in this file.
- Add the shipping commit SHA next to the checkbox.
- If a task is split further, add sub-tasks with `[ ]` beneath it.

### Rule 4 — All test output is a file, never a chat wall
- `go test ./... > docs/reports/YYYY-MM-DD-test-run.txt`
- Then message: "Test run saved to docs/reports/2026-08-15-test-run.txt · pass=N fail=M"

### Rule 5 — Don't paste code into chat unless asked
- Reference the file path: `see internal/service/tecdoc.go:145`
- Chat is for coordination, not diffs.

### Rule 6 — When spawning parallel agents, give each ONE task
- Every spawned agent gets one task card from this file, referenced by its ID (e.g. `T-1.03`).
- The agent's output goes to `docs/reports/YYYY-MM-DD-T-1.03-<agent-name>.md`.
- Coordinator agent collects one-line summaries only.

---

## 📊 Current Baseline

| Metric | Value | Target | Gap |
|---|--:|--:|--:|
| Precision (live API) | 56.9% | 92% | -35 pp |
| Recall | 69.8% | 90% | -20 pp |
| F1 | 62.7% | 91% | -28 pp |
| Accuracy | 45.7% | 88% | -42 pp |
| Aftermarket alts / part | 3–5 | 30–50 | ~10× |
| Categories ≥60% coverage | 3 / 57 | 45 / 57 | +42 cats |
| Compatibility populated | 0% | 100% | +100 pp |
| Specifications populated | 0% | 100% | +100 pp |
| Product images populated | 0% | 100% | +100 pp |
| Supersession populated | 0% | 100% | +100 pp |

---

## 🗓️ Sprint Overview

| Sprint | Duration | Stage(s) | Deliverable |
|---|---|---|---|
| **Sprint 0** | 3 days | Setup | Repo conventions + agent handbook |
| **Sprint 1** | 2 weeks | Stage 1 | 11 TecDoc tables wired as Go query methods |
| **Sprint 2** | 2 weeks | Stage 2 | 8 search strategies implementing SearchStrategy interface |
| **Sprint 3** | 1 week | Stage 3 | `?mode=` API + enriched response schema |
| **Sprint 4** | 2 weeks | Stages 4 + 5 | Smart Search merger + frontend dropdown |
| **Sprint 5** | 2 weeks | Stages 6 + 7 | Per-mode QA + canary rollout |

**Total: ~9 calendar weeks with 3 parallel agents (backend + frontend + QA)**

---

## Sprint 0 — Setup & Conventions

**Duration:** 3 days
**Goal:** Every agent knows where to write output and what tests to run.
**Parallel-safe:** No — sequential setup.

### Tasks

- [ ] **T-0.01** Create `docs/reports/`, `docs/plans/`, `docs/specs/` directories.
- [ ] **T-0.02** Add `.gitignore` entry for `docs/reports/*.local.md` (agent scratch space).
- [ ] **T-0.03** Write `docs/AGENT-HANDBOOK.md` with the 6 working rules from the top of this file.
- [ ] **T-0.04** Add pre-commit hook: reject commits that add >100 lines to any `*.md` in root (forces reports into `docs/reports/`).
- [ ] **T-0.05** Set up `make test-report` target: runs `go test ./...` and pipes to timestamped file in `docs/reports/`.
- [ ] **T-0.06** Establish task-tracking convention: this file (`SPRINT-PLAN.md`) is the source of truth.

**Sprint 0 outcome:** Any future spawned agent knows where to write, what to run, and how to update task status.

---

## Sprint 1 — Foundation: DB Query Layer

**Duration:** 2 weeks (8 engineer-days)
**Goal:** Every previously-unused TecDoc table has a Go query method with timeout, logging, and unit tests.
**Parallel-safe:** ✅ **YES — split across 4 agents.** Each task is independent.

### Task assignments (parallelize)

**Agent A — Cross-references + Compatibility (~3 days)**
- [ ] **T-1.01** Implement `TecDoc.SearchCrossReferences(oem)` — query `articlecrosses` (30M rows). Return `[]OEMReference` with brand.
  - File: `internal/service/tecdoc.go` (new method)
  - Test: `internal/service/tecdoc_crossref_test.go`
  - Report to: `docs/reports/YYYY-MM-DD-T-1.01-crossref.md`
- [ ] **T-1.02** Implement `TecDoc.FindCompatibleVehicles(legacyArticleId)` — query `articlesvehicletrees` (651M) → `linkagetargets`. Return `[]CompatibleVehicle`.
  - File: `internal/service/tecdoc.go` (new method)
  - Test: `internal/service/tecdoc_compatibility_test.go`

**Agent B — Specifications + Images (~3 days)**
- [ ] **T-1.03** Implement `TecDoc.FindSpecifications(legacyArticleId)` — join `articlecriteria` (27M) → `criteria` → `keyvalues`.
  - New model: `internal/model/specification.go` with `Name`, `Value`, `Unit` fields.
  - Test: `internal/service/tecdoc_specifications_test.go`
- [ ] **T-1.04** Implement `TecDoc.FindDocuments(legacyArticleId)` — query `articledocs` (8.2M) + `articlepdfs`. Return `[]DocumentRef` with URL type.
  - Test: `internal/service/tecdoc_documents_test.go`

**Agent C — Supersession + Functional Equivalents (~3 days)**
- [ ] **T-1.05** Implement `TecDoc.FindSupersession(legacyArticleId)` — recursive CTE over `replacedbyarticles` (229K) + `replacesarticles` (240K). Depth cap 10.
  - New model: `internal/model/supersession_chain.go`
  - Test: `internal/service/tecdoc_supersession_test.go`
- [ ] **T-1.06** Implement `TecDoc.FindFunctionalEquivalents(legacyArticleId, vehicleId?)` — join `legacy2generic` (6.2M) + `articlesvehicletrees`.
  - Test: `internal/service/tecdoc_equivalents_test.go`

**Agent D — Cross-Brand + Specification Reverse Search (~3 days)**
- [ ] **T-1.07** Implement `TecDoc.FindCrossBrandEquivalents(oem, hyundaiVehicleId, kiaVehicleId?)` — Hyundai↔Kia platform join.
  - Also seed a **materialized view** `platform_pairs` populated by an offline job (per `05_hyundai_kia_platform.sql` Query 5).
  - Test: `internal/service/tecdoc_crossbrand_test.go`
- [ ] **T-1.08** Implement `TecDoc.FindBySpecification(criteria...)` — reverse lookup: given `{thread: "M20×1.5", diameter: "76mm"}` find matching parts.
  - Test: `internal/service/tecdoc_spec_search_test.go`

**Coordinator — after A, B, C, D complete**
- [ ] **T-1.09** Add per-method timeout via `context.WithTimeout` (2s each) — pattern from existing `SearchByOEM`.
- [ ] **T-1.10** Add structured logging (`logQueryCtx` pattern) to every new method.
- [ ] **T-1.11** Write `docs/specs/tecdoc-query-layer.md` — reference document listing all query methods.
- [ ] **T-1.12** Run full test suite; produce `docs/reports/YYYY-MM-DD-sprint1-test-run.md`.

### Sprint 1 exit criteria
- 8 new TecDoc query methods, each with ≥95% test coverage on happy path + error path.
- No API changes yet — pure DB-layer plumbing.
- Report saved to `docs/reports/YYYY-MM-DD-sprint1-summary.md`.

---

## Sprint 2 — Individual Strategy Implementations

**Duration:** 2 weeks (8 engineer-days)
**Goal:** 8 hot-swappable search strategies behind a common `SearchStrategy` interface.
**Parallel-safe:** ✅ **YES — split across 4 agents.** Each strategy is independent.

### Prerequisites
- Sprint 1 complete (all TecDoc query methods available).
- `docs/specs/search-strategy-interface.md` finalized (write before Sprint 2 starts).

### Task assignments (parallelize)

**Agent A — Exact + Cross-reference (~2 days)**
- [ ] **T-2.01** Define `SearchStrategy` interface + registry in `internal/service/strategy.go`.
- [ ] **T-2.02** Implement `ExactOEMStrategy` — wraps `OEMLookup.Search` + `TecDoc.SearchByOEM`.
- [ ] **T-2.03** Implement `CrossReferenceStrategy` — wraps `TecDoc.SearchCrossReferences`.

**Agent B — Vehicle + Specification (~2 days)**
- [ ] **T-2.04** Implement `VehicleFitmentStrategy` — wraps `TecDoc.FindCompatibleVehicles` (input: linkageTargetId + optional category).
- [ ] **T-2.05** Implement `SpecificationStrategy` — parses `?spec=` from request, calls `TecDoc.FindBySpecification`.

**Agent C — Functional + Supersession (~2 days)**
- [ ] **T-2.06** Implement `FunctionalEquivalentStrategy` — wraps `TecDoc.FindFunctionalEquivalents` (seed article ID required).
- [ ] **T-2.07** Implement `SupersessionStrategy` — wraps `TecDoc.FindSupersession`.

**Agent D — Cross-brand + Keyword (~2 days)**
- [ ] **T-2.08** Implement `CrossBrandStrategy` — wraps `TecDoc.FindCrossBrandEquivalents`.
- [ ] **T-2.09** Implement `KeywordFulltextStrategy` — wraps `TecDoc.SearchByKeyword` **with category-token gate** (fixes BUG-1). Confidence sentinel 0.65.

**Coordinator — after all 4 agents (~1 day)**
- [ ] **T-2.10** Refactor `SmartSearch.Search` to dispatch to a strategy chosen from `SearchMode` field in request.
- [ ] **T-2.11** Preserve backward compat: no `mode` → run today's cascade.
- [ ] **T-2.12** Sprint 2 test-run report: `docs/reports/YYYY-MM-DD-sprint2-test-run.md`.

### Sprint 2 exit criteria
- All 8 strategies pass table-driven tests with ≥20 real OEM inputs each (~160 new sub-tests).
- Keyword strategy provably rejects BUG-1 style false positives (regression test).
- `docs/specs/search-strategy-interface.md` finalized.

---

## Sprint 3 — API Layer & Response Schema

**Duration:** 1 week (3 engineer-days)
**Goal:** `?mode=` param on `/api/search`; response enriched with specs, compatibility, images, supersession, functional equivalents.
**Parallel-safe:** ⚠ **Partial** — schema first, then handlers.

### Task order (mostly sequential)

**Agent A — Response Schema (~1 day)** — MUST run first
- [ ] **T-3.01** Extend `SmartResult` struct in `internal/service/smart_search.go`:
  - `Compatibility []CompatibleVehicle`
  - `Specifications []Specification`
  - `Images []DocumentRef`
  - `Supersession *SupersessionChain`
  - `FunctionalEquivalents []OEMReference`
  - `SourceStrategy string` (which mode returned this)
  - Docs: `docs/specs/search-response-schema.md`

**Agent B — Search Handler (~1 day)**
- [ ] **T-3.02** Extend `GET /api/search` to accept `?mode=` and `?enrichmentLevel=`.
- [ ] **T-3.03** Add `GET /api/search/modes` endpoint returning list of `{key, name, description}` for UI dropdown.
- [ ] **T-3.04** Backward-compatibility: `?mode=` omitted → runs today's cascade.

**Agent C — Enrichment Pipeline (~1 day)**
- [ ] **T-3.05** Post-process every returned `SmartResult`: parallel calls to `FindCompatibleVehicles`, `FindSpecifications`, `FindDocuments`, `FindSupersession`, `FindFunctionalEquivalents`.
- [ ] **T-3.06** Respect `enrichmentLevel=none|basic|full`:
  - `none` — no enrichment
  - `basic` (default) — specs + compatibility only
  - `full` — everything
- [ ] **T-3.07** Cap enrichment fan-out at 20 parallel goroutines.

**Coordinator**
- [ ] **T-3.08** Update Swagger/OpenAPI doc.
- [ ] **T-3.09** Sprint 3 test-run report: `docs/reports/YYYY-MM-DD-sprint3-test-run.md`.

### Sprint 3 exit criteria
- API supports all 9 modes via `?mode=`.
- Response contains specs, compatibility, images, supersession, functional equivalents when enrichment=full.
- Existing clients unbroken.

---

## Sprint 4 — Smart Search Merger + Frontend

**Duration:** 2 weeks (7 engineer-days)
**Goal:** Smart Search fan-out + user-facing dropdown.
**Parallel-safe:** ✅ **YES — backend + frontend fully independent.**

### Backend track — Agent A (~4 days)

- [ ] **T-4.01** Implement `SmartSearch.SmartCombined(req)`:
  - Fan out 7 strategies as goroutines with 3s hard budget.
  - Buffered result channel; global timeout via `context.WithTimeout`.
- [ ] **T-4.02** Merge/dedupe by `legacyArticleId` (highest-confidence wins; note all matching strategies).
- [ ] **T-4.03** Weighted ranking: `score = strategyConfidence × strategyPriority × matchBonus`.
- [ ] **T-4.04** Match bonus: same article from ≥2 strategies → confidence × 1.05.
- [ ] **T-4.05** Circuit breaker: strategy fails N=3 times in a row → skip for next M=60s.
- [ ] **T-4.06** Structured telemetry: `strategyLatencies`, `resultsFromEachStrategy`.

### Frontend track — Agent B (~3 days, parallel with Agent A)

- [ ] **T-4.07** `SearchModeSelector.tsx` — fetches `/api/search/modes` on mount, renders `<select>` with tooltip descriptions.
- [ ] **T-4.08** Persist selected mode in `localStorage` + URL `?mode=`.
- [ ] **T-4.09** `StrategyBadge.tsx` — colored pill per result card showing `result.sourceStrategy`.
- [ ] **T-4.10** "Strategies used" summary bar above results.

### Frontend track — Agent C (~3 days, parallel with A + B)

- [ ] **T-4.11** `SpecificationTable.tsx` — renders `result.specifications` as key-value grid.
- [ ] **T-4.12** `CompatibilityChips.tsx` — renders `result.compatibility` as vehicle chips.
- [ ] **T-4.13** `ImagesCarousel.tsx` — renders `result.images` with lightbox.
- [ ] **T-4.14** `SupersessionChain.tsx` — visualises replacedBy → current → replaces links.

**Coordinator**
- [ ] **T-4.15** Sprint 4 test-run report.

### Sprint 4 exit criteria
- Smart Search P95 < 3s (verify with load test).
- UI: dropdown works, badge visible, new card sections render.
- Cypress e2e test for each mode selection.

---

## Sprint 5 — QA, Load Test, Rollout

**Duration:** 2 weeks (6 engineer-days)
**Goal:** Verified accuracy per mode + safe production rollout.
**Parallel-safe:** ✅ **Mostly** — QA + DevOps tracks run parallel.

### QA track — Agent A (~4 days)

- [ ] **T-5.01** Extend `qa/golden_cases.json` to 800+ cases (100 × 8 modes).
- [ ] **T-5.02** Extend `cmd/qa_gate/main.go` to run each case in each mode and emit per-mode scorecard.
- [ ] **T-5.03** Target thresholds (fail CI below these):
  - Exact OEM: Prec ≥95%, Rec ≥80%
  - Cross-reference: Prec ≥90%, Rec ≥85%
  - Vehicle: Prec ≥90%, Rec ≥85%
  - Spec: Prec ≥85%, Rec ≥70%
  - Functional: Prec ≥85%, Rec ≥80%
  - Supersession: Prec ≥95%, Rec ≥70%
  - Cross-brand: Prec ≥85%, Rec ≥70%
  - Keyword (gated): Prec ≥70%, Rec ≥90%
  - **Smart Search: Prec ≥92%, Rec ≥90%, F1 ≥91%**
- [ ] **T-5.04** Load test: 100 concurrent Smart Search queries with `hey`/`wrk`. Target: no timeouts, P95 <2s.
- [ ] **T-5.05** Report: `docs/reports/YYYY-MM-DD-per-mode-accuracy.md`.

### DevOps track — Agent B (~2 days)

- [ ] **T-5.06** Feature-flag each mode via env vars (`SEARCH_MODE_EXACT=on`, etc.).
- [ ] **T-5.07** Canary release: enable Smart Search for 5% traffic → 25% → 100% over 3 days.
- [ ] **T-5.08** Dashboards: per-mode QPS, P95 latency, error rate, cache hit rate.
- [ ] **T-5.09** Alerts: P95 > 3s on Smart Search; error rate > 1% any mode.
- [ ] **T-5.10** Runbook: `docs/specs/runbook-search-modes.md` — how to disable a broken mode without redeploy.

**Coordinator**
- [ ] **T-5.11** User-facing help page: `docs/user-search-modes.md` explaining each mode.
- [ ] **T-5.12** Final go/no-go review; report to `docs/reports/YYYY-MM-DD-sprint5-signoff.md`.

### Sprint 5 exit criteria
- CI gate passes with all 9 mode-target thresholds.
- Canary rollout completes to 100% traffic with 7 days of green metrics.
- Runbook + user help doc published.

---

## 🎯 Acceptance Criteria (Overall Project)

Before declaring this project complete, all must be true:

- [ ] Precision (live API, 43 OEM sample) ≥ **92%**
- [ ] Recall ≥ **90%**
- [ ] F1 ≥ **91%**
- [ ] Categories at ≥60% brand coverage: **≥ 45 of 57**
- [ ] Compatibility populated on **100%** of results
- [ ] Specifications populated on **100%** of results
- [ ] Images populated on ≥ **80%** of results (some parts lack docs)
- [ ] Supersession populated when applicable (>0% is a win)
- [ ] Smart Search P95 latency < **2 s**
- [ ] All 9 modes selectable in the UI
- [ ] Zero regressions on existing clients (backward-compat verified)
- [ ] All test reports in `docs/reports/`, all specs in `docs/specs/`

---

## 📚 Where to Find Things

| Document | Path |
|---|---|
| This sprint plan | `docs/plans/SPRINT-PLAN.md` |
| Agent handbook | `docs/AGENT-HANDBOOK.md` |
| Full implementation plan (HTML) | `docs/search-modes-implementation-plan.html` |
| TecDoc SQL schema docs | `C:\ssda\chatGPT\parts\` |
| Golden test cases | `qa/golden_cases.json` |
| Per-agent reports | `docs/reports/YYYY-MM-DD-*.md` |
| Component specs | `docs/specs/*.md` |
| Runbooks | `docs/specs/runbook-*.md` |

---

## 📈 Progress Tracker

Update this section as sprints complete.

| Sprint | Status | Started | Completed | Notes |
|---|---|---|---|---|
| Sprint 0 | ⚪ Not started | — | — | — |
| Sprint 1 | ⚪ Not started | — | — | — |
| Sprint 2 | ⚪ Not started | — | — | — |
| Sprint 3 | ⚪ Not started | — | — | — |
| Sprint 4 | ⚪ Not started | — | — | — |
| Sprint 5 | ⚪ Not started | — | — | — |

Legend: ⚪ Not started · 🟡 In progress · ✅ Complete · 🔴 Blocked
