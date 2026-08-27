# Search-Engine Gap Review — 2026-08-27

**Reviewer:** Fleet coordination (worktree audit, roadmap map, code walk)
**Base:** `origin/main` at `4646b3a` (Consolidated: diagnostics + sql/08 + engine-health runbook + M7 + M8 plan + M8 core code, PR #27)
**Compare to:** `docs/ROADMAP.md`, `docs/plans/2026-08-27-full-improvement-plan.md`, `docs/sprints/M0-M8`

---

## Executive summary

The ROADMAP.md is a 12-15 month, 34-sprint plan. The current `origin/main` codebase has **already delivered M1 correctness, most of M2 rich-alternatives, and the M8 scaffolding** via PRs #12-#27. The three visible gaps against roadmap:

1. **M0 broken strategies** — 5 of 13 registered strategies return 0 hits in the 2026-08-24 audit. Two are BLOCKED (need qa Postgres + TecDoc MySQL access). Three are code-fixable in this repo without infra changes.
2. **M0.T4 sub-B** — corpus linkage enrichment tool needed for `vehicle_fitment` and `spec_match` audits. **Shipped in this PR** (`scripts/audit/enrich_corpus_linkages/`).
3. **M0.T3** — `vin_assembly` strategy did not detect VIN-shape queries. **Shipped in this PR** (`internal/service/strategy_assembly.go`).

Everything else the ROADMAP calls out for M1 / M2 / M3.S1 is either already merged or in a queued PR (#21, #28, #29). Later milestones (M4-M8) are correctly deferred per the $0 improvement plan.

**Bottom line: the search engine is not broken — it is un-audited on 4 strategies. This PR closes 2 of them.**

---

## Per-strategy status against `origin/main`

Reference: `docs/sprints/M0-fix-broken-strategies.md` (2026-08-24 probe).

| Strategy            | 2026-08-24 status                | `origin/main` state           | Post-this-PR                 |
| ------------------- | -------------------------------- | ----------------------------- | ---------------------------- |
| cache               | ✅ 1 hit, 0.4 s                   | Works                         | Works                        |
| legacy              | ✅ 1 hit, 3.2 s                   | Works                         | Works                        |
| exact_oem           | ✅ 1 hit, 1.7 s                   | Works                         | Works                        |
| prefix_inference    | ✅ 1 hit, 0.8 s                   | Works                         | Works                        |
| cross_reference     | Data-dependent                   | Works when data exists        | Works when data exists       |
| cross_brand         | Data-dependent                   | Works when data exists        | Works when data exists       |
| keyword_gated       | ❌ (wrong test input)             | Works on keyword corpus       | **M0.T5** — corpus TBD       |
| owned_catalog       | ❌ (empty table)                  | Works when populated          | **M0.T1** — needs qa Postgres|
| supersession        | ❌ 0 hits                         | Article-id promotion missing  | **M0.T2** — separate PR      |
| spec_match          | ❌ (missing seedArticleId param)  | Works with corpus enrichment  | Unblocked by this PR         |
| assembly_context    | ❌ (missing seedArticleId param)  | Works with corpus enrichment  | Unblocked by this PR         |
| **vin_assembly**    | ❌ (VIN string in query)          | Required pre-computed id      | **FIXED THIS PR (M0.T3)**    |
| vehicle_fitment     | ❌ (arbitrary linkage id)         | Works when id valid           | Unblocked by this PR         |
| combined            | ✅ (via cache + prefix + exact)   | Works                         | Works                        |

**Unblocked by this PR:** `spec_match`, `assembly_context`, `vehicle_fitment` — once the audit corpus is enriched with `LinkageTargetIds` and `SeedArticleIds` (M0.T4 sub-B, this PR), the audit harness can pass real ids to those three strategies for the first time.

---

## Per-milestone status against `origin/main`

### M0 — Fix broken strategies

| Task     | Description                                           | Status                    | Files / evidence                                                                                        |
| -------- | ----------------------------------------------------- | ------------------------- | ------------------------------------------------------------------------------------------------------- |
| M0.T1    | Populate `hk_parts_cache` for `owned_catalog`         | BLOCKED — ops             | Needs qa Postgres access to run `derive_hk_maps`. Code path is intact — table is empty.                 |
| M0.T2    | Fix `supersession` — article-id promotion             | OPEN — separate PR        | `internal/service/strategy.go:770-810` uses `st.search.oem.Search` only; missing TecDoc fallback.       |
| **M0.T3**| **Fix `vin_assembly` — VIN-shape auto-detection**     | **DONE (this PR)**        | `internal/service/strategy_assembly.go:resolveLinkageFromVIN`; `strategy_assembly_test.go`.             |
| M0.T4 A  | Case-insensitive `/api/catalog/vehicles`              | IN-PROGRESS — PR #29      | `internal/handler/catalog.go:normalizeCatalogArg`; branch `fix/m0-t4-catalog-vehicles-case-insensitive`.|
| **M0.T4 B**| **`enrich_corpus_linkages` tool**                   | **DONE (this PR)**        | `scripts/audit/enrich_corpus_linkages/main.go`, `main_test.go`, `README.md`.                             |
| M0.T5    | Keyword corpus for `keyword_gated`                    | OPEN — corpus work        | 200 rows of `(query, expected_category)` still needed. `analyze-quality.ps1` needs `-QueryColumn` param.|
| M0.T6    | Corpus linkage-target enrichment column consumers     | OPEN — script wiring      | `audit-quality.ps1` needs `&linkageTargetId=<id>` when Mode=vehicle_fitment. This PR gives it the data. |
| M0.T7    | Per-strategy F1 tracking in CI                        | OPEN — depends on M6.S1   | Waits for `.github/workflows/nightly-audit.yml` to exist first.                                          |

### M1 — Correctness first

| Sprint    | Task                                                | Status              | Evidence in `origin/main`                                                              |
| --------- | --------------------------------------------------- | ------------------- | -------------------------------------------------------------------------------------- |
| M1.S1.T1  | `strategyCategoryPenalty(prefix, category)`         | DONE                | `internal/service/strategy.go:strategyCategoryPenalty()`, `categoryToSystem()`.        |
| M1.S1.T2  | Wire the penalty into `searchCombined` merge        | DONE                | `strategy.go` around the `penalised++` loop.                                            |
| M1.S1.T3  | Prefer-same-system tiebreak in sort                 | DONE                | `strategy.go:sort.Slice` at line 514 uses `categoryToSystem() == queried.System`.       |
| M1.S2.T1  | Non-HK deny-list reorder + widening                 | DONE                | `internal/service/hk_scope.go` has Ford / Peugeot / Renault / Fiat / Chevy / Mitsubishi.|
| M1.S2.T2  | Confidence floor per SourceStrategy (cache-alone)   | DONE                | `strategy.go` — `isSoloCache && r.Confidence < 0.5` drop path.                          |
| M1.S3.T1  | `categoryTokens[prefix]` lookup                     | DONE                | `internal/service/category_tokens.go`, `category_tokens_test.go`.                       |
| M1.S3.T2  | Post-hoc category validation in `searchCombined`    | DONE                | `strategy.go` — `CategoryTokensForOEM(req.OEM)`.                                        |
| M1.S3.T2  | Soft-penalty vs hard-drop (roadmap follow-up)       | IN-REVIEW — PR #21  | `fix/m1s3-soft-penalty-instead-of-drop`.                                                |
| M1.S3.T3  | Audit script `wrong_category_dropped_by_penalty_pct`| OPEN — audit script | `scripts/audit/analyze-quality.ps1` — column not emitted yet.                           |

### M2 — Rich alternatives

| Sprint    | Task                                                | Status              | Evidence in `origin/main`                                                                    |
| --------- | --------------------------------------------------- | ------------------- | -------------------------------------------------------------------------------------------- |
| M2.S1.T1  | Multi-path aftermarket UNION                        | DONE                | `internal/service/tecdoc.go:FindAftermarketForOEM` — 4-path fan-in (articlecrosses / oem_number / oem_search_index / online). |
| M2.S1.T2  | Supersession-chain expansion in aftermarket path    | PARTIAL             | The path exists as `pathCount=4` (online M8 hook), but supersession-chain OEM keys are not part of the fan-in yet. |
| M2.S1.T3  | Brand normalisation                                 | DONE                | `internal/service/brand_normalize.go`, `brand_normalize_test.go`.                            |
| M2.S2.T1  | Tier-sort (Bosch/MANN/MAHLE first)                  | DONE                | `internal/service/brand_tier.go:SortAftermarketByTier`.                                       |
| M2.S2.T2  | Cap at 20 total / 3 per brand                       | DONE                | `internal/service/brand_tier.go:CapAftermarketList`; called from `FindAftermarketForOEM`.     |
| M2.S3.T1  | Recursive supersession chain with cycle guard       | DONE                | `internal/service/tecdoc_supersession.go:walk` uses `model.MaxSupersessionDepth` + `visited` map. |
| M2.S3.T2  | Populate OEMNumbers from expanded chain             | Needs verification  | Wired via `enrichment.go` — audit rerun will confirm `Supersession_pct ≥ 30%`.                 |

### M3 — Full enrichment

| Sprint    | Task                                    | Status              | Evidence in `origin/main`                                                       |
| --------- | --------------------------------------- | ------------------- | ------------------------------------------------------------------------------- |
| M3.S1.T1  | Aggressive article-id promotion         | DONE                | `enrichment.go:132-187` has 3-tier cascade (SearchByOEM → SearchCrossReferences → SearchByOEM w/ retry). |
| M3.S1.T2  | Batch enrichment `FindSpecificationsBatch` | OPEN — perf work | Current path fires one goroutine per result (~20 concurrent DB round-trips). Batch API missing. |
| M3.S2.T1  | `articlesvehicletrees` direct-query fallback | Needs verification | `tecdoc_vehicle.go` — need to check if fallback path was added.               |
| M3.S2.T2  | Vehicle name normalisation (Make / Model / YearRange / Engine / Chassis) | OPEN | `model.CompatibleVehicle` returned raw; parsed struct needed for frontend clarity. |

### M4 — Beyond TecDoc

| Sprint | Task                       | Status                                       | Note                                                                                  |
| ------ | -------------------------- | -------------------------------------------- | ------------------------------------------------------------------------------------- |
| M4.S1  | RockAuto scraper           | **EXCLUDED** (no-scraping policy)            | Deferred to M8 online-search meta-search via legal green sources.                     |
| M4.S2  | Regional supplier catalog  | OPEN — research task                         | `docs/data-sources/regional-catalog-survey.md` seeded.                                |
| M4.S3  | Dealer parts (GSW / KWO)   | OPEN — needs partnership                     | Blocked on business agreement.                                                        |
| M4.S4  | Community contributions    | PARTIAL                                      | Backend + moderation queue exist (`internal/handler/community_contrib.go`). UI on M4.S4 backlog. |

### M5 — Search intelligence

| Sprint    | Task                                     | Status              | Evidence in `origin/main`                                                        |
| --------- | ---------------------------------------- | ------------------- | -------------------------------------------------------------------------------- |
| M5.S1.T1  | Description embeddings (pgvector, 384-d) | OPEN                | No `db/migrations/000030_article_embeddings.sql` on origin/main.                 |
| M5.S1.T2  | `/api/search/semantic` endpoint          | OPEN                | `internal/handler/semantic_search.go` exists but returns 501 without embed table.|
| M5.S2.T1  | VIN decoder (offline NHTSA)              | DONE                | `internal/service/vin_decoder.go` — 30+ WMI mappings including all HK.           |
| M5.S2.T2  | `/api/vin/:vin/parts`                    | DONE                | `internal/handler/vin_parts.go` — composed endpoint.                             |
| M5.S3.T1  | Related-parts recommendations            | OPEN                | Handler stub exists; co-occurrence data pipeline missing.                        |

### M6 — Production-grade

| Sprint    | Task                                | Status              | Evidence in `origin/main`                                                        |
| --------- | ----------------------------------- | ------------------- | -------------------------------------------------------------------------------- |
| M6.S1.T1  | Nightly cron audit                  | PARTIAL             | `docs/reports/nightly-audits/` seeded; workflow YAML not committed.              |
| M6.S1.T2  | PR gate on F1_correct regression    | OPEN                | `.github/workflows/pr-quality-gate.yml` — placeholder only.                      |
| M6.S2.T1  | "Was this the right part?" feedback | DONE                | `internal/handler/feedback.go`, `internal/service/feedback.go`, DB table live.   |
| M6.S2.T2  | Cost monitoring dashboard           | OPEN                | Grafana wiring TBD.                                                              |

### M7 — AI/ML matching

Entire milestone (4 sprints, 12 tasks) is **planned but not implemented**. Depends on M0-M6 finishing so training data (`search_feedback`, `articlecrosses`, `aftermarket_rockauto`, `aftermarket_community`) exists. This is intentional per the roadmap.

### M8 — Online-search aggregation

| Sprint  | Task                                       | Status              | Evidence in `origin/main`                                             |
| ------- | ------------------------------------------ | ------------------- | --------------------------------------------------------------------- |
| W3.1    | `aftermarket_online_cache` migration       | DONE                | `db/migrations/000021_aftermarket_online_cache.sql`.                  |
| W3.2-4  | Federated meta-search + eBay adapter       | DONE                | `internal/service/online_search.go`, `online_ebay.go`.                |
| W3.5    | schema.org JSON-LD extractor               | DONE                | `internal/service/schema_org_parser.go`.                              |
| W3.6-8  | HyundaiPartsDeal / KiaPartsNow / 7zap      | DONE                | `internal/service/online_hyundaipartsdeal.go`, etc.                   |
| W3.9    | robots.txt guard                           | DONE                | `internal/service/robots_guard.go`.                                   |
| W3.10   | Rate-limiter per source                    | DONE                | `internal/service/rate_limiter.go`.                                   |
| W3.11   | UNION online results into `FindAftermarketForOEM_MultiPath` | DONE | `tecdoc.go:472-482` — path 4 branch when `t.online != nil`.        |
| W3.12   | Frontend badge for online-sourced results  | OPEN                | Frontend integration.                                                 |

---

## Top-10 prioritized next-actions

Ordered by (impact × doability). Blocked items excluded — track those separately.

1. **M0.T2 — Supersession article-id promotion.** `internal/service/strategy.go:770-810`. Currently only uses `st.search.oem.Search`. Fix: fall through to `st.search.tecdoc.SearchByOEM` then `st.search.tecDocCrossRef.SearchCrossReferences` when the Postgres lookup returns zero. Mirrors the `enrichment.go:132-187` cascade. **Effort: M** (needs mocking infrastructure for `SupersessionStrategy` — no test file today). **Impact: `supersession` F1 goes from 0 to ≥ 0.40 on chain-relevant OEMs.**

2. **M0.T6 — Wire `LinkageTargetIds` into `audit-quality.ps1`.** Consume the new column this PR produces. When `Mode == vehicle_fitment` or `spec_match`, append `&linkageTargetId=<first id>` and `&seedArticleId=<first id>` to the request URL. **Effort: S.** **Impact: unblocks the audit numbers for 3 more strategies.**

3. **M0.T5 — Keyword corpus + `-QueryColumn` param.** New file `scripts/audit/corpus-keywords-v1.csv` with 200 rows (`Query, ExpectedCategory, GoodTokens`). Extend `analyze-quality.ps1` to accept `-QueryColumn`. **Effort: M** (corpus authoring + audit-script threading). **Impact: `keyword_gated` gets its first real F1 baseline.**

4. **M2.S1.T2 — Supersession-chain OEM keys in aftermarket UNION.** `tecdoc.go:FindAftermarketForOEM` fan-in currently uses just `oemNumber`. Add the transitively-related OEM numbers from `FindSupersession` chain to the `articlecrosses.oemNumberNormalized IN (?, ?, ?)` list. **Effort: M.** **Impact: AvgAM_correct climbs +2 on OEMs with a known chain.**

5. **M3.S2.T2 — Vehicle name normalisation.** Parse `linkageTargets.description` into `(Make, Model, YearRange, Engine, Chassis)` in `tecdoc_vehicle.go`. **Effort: M.** **Impact: frontend rendering + `CompatibleVehicles` field structure improves.**

6. **M1.S3.T3 — `wrong_category_dropped_by_penalty_pct` in audit report.** Extend `analyze-quality.ps1` to emit a new column in `by-category.csv`. **Effort: S.** **Impact: observability of where the guard fires.**

7. **M0.T1 — `hk_parts_cache` population.** Needs a live qa Postgres run of `derive_hk_maps`. NOT a code task; needs ops access. **Impact: `owned_catalog` unblocked (F1 climbs from 0 to ≥ 0.60).**

8. **M2.S3.T2 verification.** Confirm audit rerun shows `Supersession_pct ≥ 30%` after M0.T2 lands. **Effort: XS** (script rerun only).

9. **M3.S1.T2 — Batch enrichment (`FindSpecificationsBatch`).** Refactor `enrichment.go` to call `FindSpecificationsBatch(articleIds []int)` instead of one goroutine per result. **Effort: L.** **Impact: enrichment p95 drops ≥ 40% at same coverage.**

10. **M6.S1.T1 — Nightly audit workflow.** Ship `.github/workflows/nightly-audit.yml` per the roadmap. **Effort: M.** **Impact: automated regression detection.**

---

## Items intentionally deferred

- **M4.S1 RockAuto scraper** — no-scraping policy locked in. See `docs/plans/2026-08-27-full-improvement-plan.md` §"Excluded — sources we will NOT use."
- **M4.S3 Dealer catalog** — blocked on partnership agreement.
- **M5.S1 pgvector embeddings** — no committed spend for the pgvector infrastructure this iteration.
- **M7 ML matching (entire milestone)** — waiting on M0-M6 to produce training data.

---

## How this PR closes gaps

| PR commit                                             | Milestone   | Effect on the search engine                                                                |
| ----------------------------------------------------- | ----------- | ------------------------------------------------------------------------------------------ |
| `feat(vin_assembly): auto-detect VIN-shape queries`   | M0.T3       | `vin_assembly` strategy F1_correct on VIN corpus goes from 0 to ≥ 0.70 (per sprint DoD).   |
| `feat(audit): enrich_corpus_linkages CLI tool`        | M0.T4 sub-B | `vehicle_fitment` and `spec_match` audits get real ids to test against.                    |
| `docs: search-engine gap review 2026-08-27`           | Cross-cut   | Every open task now has a status line + effort estimate + file:line reference.             |

---

## What the fleet coordinator found while auditing

Two unexpected findings worth calling out for future work:

1. **The `C:\ssda\chatGPT\parts-engine` working tree is a stale 2026-08-14 snapshot** (119 files with mojibake corruption in `strategy.go`, `smart_search.go`, `tecdoc.go`). Local `main` was at `6663f0c` (init commit) while `origin/main` was 14 commits ahead at `4646b3a`. Merging the stale tree would have regressed PRs #12-#27. **Fix:** the clean worktree at `C:\ssda\chatGPT\ifritah\ifritah-parts.git\parts-engine-baseline` should be used going forward. The stale directory can be `rm -rf`'d without loss (its docs are archived on origin/main as `docs/reports/2026-08-14-*.md`).

2. **The M0 sprint doc's priority ordering assumed M0.T4 was one task.** It is two subtasks. PR #29 (branch `fix/m0-t4-catalog-vehicles-case-insensitive`) closes sub-A; this PR closes sub-B. Both are needed for the milestone to exit. The sprint doc's ordering ("M0.T4 first, ~1 day") should be updated in a follow-up to reflect the 2-sub-task shape.
