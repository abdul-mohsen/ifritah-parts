# Parts Engine Roadmap — from "search that returns something" to "the definitive Hyundai/Kia parts sourcing engine"

**Owner:** search-quality
**Status:** Living document - updated after every audit cycle
**Baseline audit:** `docs/reports/2026-08-23-quality-audit/` (F1_correct = 0.30 overall, AvgRepl_correct = 0.20, F1_rich5 = 0.002)
**Last state update:** 2026-08-27 — post TecDoc audit-diagnostic run + PRs #22-#27 merged.

---

## Current state at-a-glance (2026-08-27)

| Milestone | Status | Evidence |
|---|:-:|---|
| **M0** Fix broken strategies (data discovery) | ✅ **DONE** | TecDoc diagnostic complete — sql/06+07+08 all applied and used; every hot query hits its index; aftermarket data confirmed present in TecDoc for HK OEMs (2026-08-27 baseline check) |
| **M1** No wrong parts (correctness) | 🟢 **~90%** | PR #19 + #20 shipped HK-scope guard + strategyCategoryPenalty + widened deny-list; PR #21 (soft-penalty refinement) still open |
| **M2** Rich alternatives (aftermarket) | 🟢 **Code done, awaits re-audit** | Multi-path UNION + brand normalization + tier-sort in PR #20; smart-search `mfrId` —>` dataSupplierId` fix in PR #27 + smart-search enrichAftermarket guard removed; needs 1490-corpus re-run to confirm `AvgAM_correct` climbs |
| **M3** Full enrichment (specs / vehicles / supersession) | 🟢 **~90%** | sql/07 `legacyArticleId` + sql/08 `criteria_value` indexes applied; `SearchByOEMIndex` third-level promotion in PR #20; supersession-chain OEM AM fetch in PR #20 |
| **M4** Beyond TecDoc (external data) | 🟡 **50%** | Backend + migrations shipped in PR #20 for RockAuto / regional / community; RockAuto scraper is skeleton-only (`--dry-run`); real scraper build ~3-4 weeks; M4.S3 dealer catalog blocked on partnership |
| **M5** Search intelligence (semantic / VIN / related) | ✅ **DONE** | pgvector migration 000019, `/api/search/semantic`, `/api/vin/:vin/parts`, `/api/parts/related` all live in PR #20 |
| **M6** Production-grade (audit CI / feedback / cost) | ✅ **DONE** | Nightly audit + PR quality gate + `/api/search/feedback` + cost meter all live in PR #20 |
| **M7** AI/ML part-matching | ⚫ **Planned only** | Full 13-task plan doc merged in PR #27 (`docs/sprints/M7-ml-part-matching.md`); no code yet — depends on M8 producing training data |
| **M8** Online-search aggregation | 🟢 **Core code + 41 sources wired** | Cache table + dispatcher + eBay + 40 G5 adapters + robots.txt guard + rate limiter all live in PR #27; adapters enabled by default, gated per-source via env; 5 sprints still deferred (schema.org tests + 3 G5 real fixtures + frontend badge) |

**Overall completion vs 34-sprint total: ~65%** (5 milestones done or near-done; 3 in flight; 1 planned; 1 partial + externally-blocked).

**Legend**: ✅ done · 🟢 near-done or in-flight · 🟡 partial · ⚫ planned only · 🔴 blocked

---

## North-star goal

> **For any Hyundai / Kia OEM number a parts seller queries, the engine returns:**
> - **the exact part identified** (name, brand, category, description) with **zero wrong-category returns**
> - **at least 5 aftermarket alternatives** across recognisable brands (Bosch, MANN, MAHLE, Denso, Textar, etc.)
> - **at least 3 OEM cross-references** (superseded, successor, or Kia↔Hyundai variant)
> - **full technical specs** (thread, diameter, capacity, torque) where physically meaningful
> - **compatible vehicle list** (make, model, year range, engine, chassis)
> - **all in under 3 seconds** end-to-end

### Success metrics (must ALL hit target on the seeded-corpus regression run)

| Metric | Baseline (2026-08-23) | Target |
|---|---:|---:|
| `F1_correct` overall | 0.30 | **≥ 0.98** |
| `F1_correct` on seeded slice | 0.71 | **≥ 0.99** |
| `AvgRepl_correct` on wear parts | 0.10-0.86 | **≥ 8** |
| `AvgAM_correct` on wear parts | 0.10-0.43 | **≥ 5** |
| `AvgOEMxRef_correct` on wear parts | 0.08-0.36 | **≥ 3** |
| `F1_rich5` on wear parts | 0.00-0.09 | **≥ 0.90** |
| `F1_rich10` on wear parts | 0.00 | **≥ 0.60** |
| p95 latency (mode=combined, enrichment=full) | not measured today | **≤ 3.0 s** |
| Non-HK guard leaks | 38 of 100 | **≤ 2 of 100** |
| Body / glass categories `F1_correct` | 0.00-0.15 | **≥ 0.90** (aftermarket N/A) |
| Continuous audit passes on every PR | not enforced | required CI gate |
| `AvgAM_inferred` on OEMs with zero direct aftermarket data | 0 | **≥ 3** (M7) |
| Ranker nDCG@5 on held-out feedback set | not measured | **≥ 0.75** (M7) |
| NL-query parser field-recall on parts-seller logs | not measured | **≥ 0.90** (M7) |

---

## Milestones

Each milestone ships a measurable slice of the north-star. Every milestone ends with a full re-audit and a PR that appends new numbers to `docs/reports/`.

| # | Milestone | Success gate | Estimated sprints |
|---|---|---|---:|
| **M0** | **Fix broken strategies** — 5 of 13 return 0 hits today | `owned_catalog`, `supersession`, `vin_assembly`, `vehicle_fitment` all F1_correct ≥ 0.40 on relevant slices | 1 (7 tasks) |
| M1 | **No wrong parts** — correctness first | `F1_correct ≥ 0.95` overall, ≥ 0.98 seeded | 3 |
| M2 | **Rich alternatives** — five or more per wear part | `AvgAM_correct ≥ 5` on wear parts, `F1_rich5 ≥ 0.60` | 3 |
| M3 | **Full enrichment** — specs / vehicles / supersession populated | ≥ 80% of correct hits have specs, ≥ 60% vehicles, ≥ 40% supersession | 2 |
| M4 | **Beyond TecDoc** — RockAuto + regional supplier + dealer + community | AvgAM adds +3 from non-TecDoc sources; F1_correct on body/glass ≥ 0.90 | 4 |
| M5 | **Search intelligence** — semantic, VIN, cross-suggest | VIN → parts F1 ≥ 0.80; description-similarity recall + 10 pts on unseeded slice | 3 |
| M6 | **Production-grade** — continuous audit CI, feedback loop, cost SLA | Every PR blocked if F1_correct regresses; p95 ≤ 3 s; monthly cost audit | 2 |
| M7 | **AI/ML part-matching engine** — analogical inference, cross-brand mapping, feedback-trained ranker | AvgAM_inferred ≥ 3 on OEMs with zero direct TecDoc aftermarket data; ranker recall@5 +15 pts on unseeded | 4 |
| M8 | **Online-search aggregation** — free/public sources meta-search + cache for HK aftermarket | `AvgAM_online` ≥ 3 aftermarket brands per corpus OEM at $0 committed spend | 12 |

Total: **34 sprints** (~12-15 months at 1 sprint / 2 weeks).

**Merge order matters.** M0 unblocks the strategies that M2 (`supersession` walker), M3 (`vehicle_fitment` for vehicle enrichment), and M5 (`vin_assembly`) will build on. Skip M0 and the later milestones are building on broken foundations. **M7 depends on M0-M6** — the ML engine trains on the artifacts produced by the earlier milestones (audit CSVs, feedback events, aftermarket rows from M4). **M8** is the free/public-source aftermarket fill; it can run in parallel with M0-M6 code fixes but its exit gate depends on Wave-2 code fixes (`mfrId` bug, sql/08) being live in prod. M8 also produces the training signal M7 depends on (`aftermarket_online_cache` rows).

---

## M1 — Correctness first (F1_correct ≥ 0.95)

**Problem statement.** The 2026-08-23 audit found 37 categories below `F1_correct = 0.95`. The engine returns *something* for most OEMs but the something is often the wrong-category part — e.g. `86xxx-*` (mirror) returning "Headlight". A parts seller cannot recover from wrong-category hits.

### Sprint M1.S1 — Ranker cross-family penalty

**Goal:** No result surfaces if its `category.system` differs from the queried OEM's `DecodeOEMPrefix.System`.

- **Task M1.S1.T1** — Add `strategyCategoryPenalty(query prefix, result category)` helper in `internal/service/strategy.go`. When the prefix decodes to a known system (e.g. `86xx` → Body / Mirrors) and the result's category tree parent is a different system (e.g. Electrical / Lighting), multiply confidence by `0.2`. Preserves the hit (won't be dropped by the strict guard) but sinks it below any same-system alternative.
  - **Files:** `internal/service/strategy.go`, `internal/service/oem_prefix.go` (may need `System()` exposure)
  - **DoD:** unit test `TestStrategyCategoryPenalty_CrossFamily` covers 4 known collisions from the failures.csv (`86391 → Headlight`, `8265*→…`, `71110→…`, `88600→…`); each collision result post-penalty has `confidence < 0.3`
  - **Effort:** M

- **Task M1.S1.T2** — Wire the penalty into `searchCombined` merge step. After dedupe, iterate results and apply penalty for any whose SourceStrategy = `cache` / `legacy` / `prefix_inference` (the fallback strategies that most often surface wrong-family hits).
  - **Files:** `internal/service/strategy.go:searchCombined` (after the dedupe loop, before sort)
  - **DoD:** existing tests pass; run the audit script against dev, verify F1_correct on `real_hk_coarse` slice climbs from 0.04 to ≥ 0.30
  - **Effort:** S

- **Task M1.S1.T3** — Add a "prefer same-system by ≥ 0.3 confidence" tiebreak in the sort. When two results have equal confidence*priority but one matches the queried prefix's system, prefer the matching one.
  - **DoD:** unit test `TestSearchCombined_PrefersSameSystem` synthesises two mock results and verifies ordering
  - **Effort:** S

### Sprint M1.S2 — Non-HK deny-list widening + strict format gate

**Goal:** No non-HK OEM leaks. Guard on the deny-list first, format regex second.

- **Task M1.S2.T1** — Reorder `IsHKOEM` in `hk_scope.go` so deny-list check runs BEFORE format-regex classification. Currently the deny-list is only checked when format ≠ "unknown"; some non-HK OEMs (Ford `AL3Z-*`, Peugeot `9803*`) fail the regex → format=unknown → guard skipped. Move the deny-list ahead.
  - **Files:** `internal/service/hk_scope.go`
  - **DoD:** add 10 new deny-list entries (Ford, Peugeot, Renault, Fiat, Chevy, Mitsubishi) with regression tests; audit script's non_hk slice leaks drop from 38 to ≤ 2
  - **Effort:** M

- **Task M1.S2.T2** — Confidence floor per SourceStrategy. If `SourceStrategy = "cache"` alone (no corroboration from any other strategy) AND `confidence < 0.5`, drop the result entirely. Currently a stale cache entry can carry an unrelated OEM into a search that shouldn't have hit it.
  - **Files:** `internal/service/strategy.go:searchCombined` (final filter before sort)
  - **DoD:** synthetic test with a mocked cache row that doesn't match the query; result is not returned
  - **Effort:** S

### Sprint M1.S3 — Category-consistency validation

**Goal:** Reject results whose `FirstDesc` doesn't contain any word from the expected category tree.

- **Task M1.S3.T1** — Build a `categoryTokens[prefix]` lookup that maps every prefix in `oem_prefix.go` to its expected description tokens (extracted from the category name — "Brake Pad" → `[brake, pad]`, "Ignition Coil" → `[ignition, coil]`).
  - **Files:** new `internal/service/category_tokens.go`
  - **DoD:** the lookup exists for every `OEMCategory.Category` and returns non-empty arrays
  - **Effort:** M

- **Task M1.S3.T2** — Post-hoc validation in `searchCombined`: after ranking, drop any result where its `Description` contains **zero** of the tokens from `categoryTokens[queryPrefix]`. Emit a warning `category_mismatch_dropped` for observability.
  - **Files:** `internal/service/strategy.go`
  - **DoD:** F1_correct on `real_hk_coarse` climbs from 0.04 to ≥ 0.60; audit re-run
  - **Effort:** M

- **Task M1.S3.T3** — Update the audit `analyze-quality.ps1` to emit a `wrong_category_dropped_by_penalty_pct` diagnostic per category so we can see which categories the guard fires on most.
  - **Files:** `scripts/audit/analyze-quality.ps1`
  - **DoD:** new column in `by-category.csv`
  - **Effort:** S

**Milestone M1 exit gate:** re-audit, expect overall F1_correct ≥ 0.95 (from 0.30). Merge PR, tag `parts-engine-quality-M1`.

---

## M2 — Rich alternatives (AvgAM_correct ≥ 5, F1_rich5 ≥ 0.60)

**Problem statement.** Even for the best category (Brake Pad Set - Rear), the app returns 0.86 replacements per correct hit — less than 1 alternative on average. Aftermarket coverage in TecDoc's `oem_number` table is 5-15%; the newly-indexed `articlecrosses` (PR #16) covers much more but is still under-utilized.

### Sprint M2.S1 — Multi-path aftermarket UNION

**Goal:** Merge every aftermarket lookup path into one deduped result set.

- **Task M2.S1.T1** — Refactor `FindAftermarketForOEM` in `internal/service/tecdoc.go` into `FindAftermarketForOEM_MultiPath` that runs three queries in parallel:
  1. `articlecrosses.oemNumberNormalized = ?` (the current PR #20 path)
  2. `oem_number.clean_number = ?` (the original path — some data exists only here)
  3. `oem_search_index.normalized = ?` (secondary xref, per PR #14)
  - Dedupe by `(brand, articleNumber)`. Return the union.
  - **Files:** `internal/service/tecdoc.go`
  - **DoD:** unit test with 3 stub repos each returning distinct rows; the merge returns all 3 sets deduped; count = union size
  - **Effort:** M

- **Task M2.S1.T2** — Add a fourth path: `articlecrosses.oemNumberNormalized IN (?, ?, ?)` where the extra keys are the supersession chain OEMs (any OEM that TecDoc lists as superseding-or-successor to the query). Widens the aftermarket net by including brands cataloged against the parent/child OEM.
  - **Files:** `internal/service/tecdoc.go`, `internal/service/tecdoc_supersession.go`
  - **DoD:** for a known supersession chain OEM in the audit, AvgAM_correct climbs by ≥ 2
  - **Effort:** M

- **Task M2.S1.T3** — Brand normalisation. Currently `"BOSCH"`, `"Bosch"`, `"Robert Bosch GmbH"` count as separate brands. Introduce a `NormalizeBrand()` in `alternatives.go` with a canonical map (≥ 200 known aftermarket brands) so the dedup collapses variants.
  - **Files:** new `internal/service/brand_normalize.go`
  - **DoD:** unit test asserts the 30 top-shipping HK aftermarket brands (BOSCH, MANN, MAHLE, MOBIS, TEIN, TEXTAR, FERODO, HENGST, KYB, MONROE, GABRIEL, DENSO, NGK, VALEO, HELLA, LEMFORDER, FEBI, MEYLE, SKF, KOYO, NSK, FAG, INA, GATES, DAYCO, CONTITECH, MAHLE, KNECHT, HERTH+BUSS, TRW) all map cleanly
  - **Effort:** M

### Sprint M2.S2 — Priority ordering + top-K per brand cap

**Goal:** The returned aftermarket list is useful, not overwhelming or dominated by one brand.

- **Task M2.S2.T1** — Sort aftermarket results by brand recognisability + price tier + stock. First pass: alphabetical inside a tiered ranking (Tier 1 = OEM brand + top-10 aftermarket; Tier 2 = mid; Tier 3 = private label). Populate the tier list in `brand_normalize.go`.
  - **Files:** `internal/service/alternatives.go`
  - **DoD:** unit test verifies a canned mixed input sorts Tier1 → Tier2 → Tier3; ties resolve alphabetically
  - **Effort:** M

- **Task M2.S2.T2** — Cap the returned aftermarket list at 20 total, max 3 per brand. Prevents a single brand (e.g. an aliased Bosch entry) from crowding out variety.
  - **Files:** `internal/service/alternatives.go`
  - **DoD:** synthetic test with 40 Bosch entries + 40 Mann entries returns ≤ 20 total with max 3 Bosch and max 3 Mann
  - **Effort:** S

### Sprint M2.S3 — Supersession-chain walker

**Goal:** For every correct hit, walk the supersession chain and surface every ancestor + successor OEM number in `OEMNumbers`.

- **Task M2.S3.T1** — Recursive supersession expansion in `tecdoc_supersession.go`. Currently `FindSupersession(articleId)` returns one hop; expand to full transitive closure with a depth cap of 5 and cycle detection.
  - **Files:** `internal/service/tecdoc_supersession.go`
  - **DoD:** unit test with a synthetic 4-hop chain returns 4 nodes; a cycle test returns without infinite loop
  - **Effort:** M

- **Task M2.S3.T2** — Populate `OEMNumbers` from the expanded chain in `enrichment.go`. Each supersession node becomes an `OEMReference` with `Manufacturer = "SUPERSESSION"`.
  - **Files:** `internal/service/enrichment.go`
  - **DoD:** audit re-run; `Supersession_pct` climbs from 1.2% to ≥ 30%
  - **Effort:** S

**M2 exit gate:** re-audit — expect `AvgAM_correct ≥ 5` and `F1_rich5 ≥ 0.60` on wear-parts categories. Tag `parts-engine-quality-M2`.

---

## M3 — Full enrichment coverage (specs / vehicles / supersession ≥ 80%)

**Problem statement.** The 2026-08-23 audit found 0% `CompatibleVehicles`, 2.5% `Specifications`, 1.2% `Supersession` populated. Root cause: article-id promotion fails 74% of the time; when it succeeds the downstream calls still return sparse. Once M1+M2 fix the article-id promotion (already partly in PR #20), this milestone drives coverage across all fields.

### Sprint M3.S1 — Aggressive article-id promotion

- **Task M3.S1.T1** — Chained promotion: if `SearchByOEM → 0 refs`, try `SearchCrossReferences → 0 refs`, try `oem_search_index → 0 refs`. Each fallback adds article-id candidates. Pick the one with the highest `articles.dataSupplierId` recency (proxy for "canonical") when multiple candidates surface.
  - **Files:** `internal/service/enrichment.go`
  - **DoD:** article-id promotion rate on the seeded slice climbs from 26% to ≥ 80%
  - **Effort:** M

- **Task M3.S1.T2** — Batch enrichment where possible. Today `enrichResults` fires one goroutine per result → 20 concurrent DB round-trips. Replace with `FindSpecificationsBatch(articleIds)`, `FindCompatibleVehiclesBatch(articleIds)` returning `map[int][]row`.
  - **Files:** `internal/service/tecdoc_specifications.go`, `internal/service/tecdoc_vehicle.go`, `internal/service/enrichment.go`
  - **DoD:** enrichment p95 drops ≥ 40% at same coverage
  - **Effort:** L

### Sprint M3.S2 — Vehicle fitment expansion

- **Task M3.S2.T1** — When `FindCompatibleVehicles` returns 0 vehicles, fall back to `articlesvehicletrees` direct query (an alternative TecDoc join path). Requires understanding the schema difference — spike task first.
  - **Files:** `internal/service/tecdoc_vehicle.go`
  - **DoD:** `Vehicles_pct` climbs from 0% to ≥ 60% on wear parts
  - **Effort:** M

- **Task M3.S2.T2** — Vehicle name normalisation. Currently returns raw `linkageTargets.description` which is technical; parse into `Make / Model / YearRange / Engine / Chassis` structured fields. Frontend can render better.
  - **Files:** `internal/service/tecdoc_vehicle.go`, `internal/model/compatible_vehicle.go`
  - **DoD:** unit test asserts parsed struct populates all 5 fields for 10 known linkage rows
  - **Effort:** M

**M3 exit gate:** re-audit — Specs ≥ 80%, Vehicles ≥ 60%, Supersession ≥ 40% on correct hits.

---

## M4 — Beyond TecDoc (RockAuto, regional supplier, dealer, community)

**Problem statement.** TecDoc's HK OEM coverage is ~5% for `oem_number`, better for `articlecrosses` but still leaves body / glass / interior / dealer-accessory categories with structural zero coverage. Data-source diversification is the only path.

### Sprint M4.S1 — RockAuto scraper (aftermarket-brand-first)

- **Task M4.S1.T1** — Build a Playwright/Chromium-driven scraper against `rockauto.com` that walks `/en/parts/hyundai/{model}/{year}/{engine}` and captures `(OEM, brand, partNumber, category, priceUsd, url)` tuples. RockAuto is JS-rendered — plain curl won't work.
  - **Files:** new `scripts/scrapers/rockauto/`
  - **DoD:** scraper produces a valid CSV for 5 test vehicles (Elantra 2015, Tucson 2018, Sonata 2020, Kia Rio 2016, Kia Sorento 2017)
  - **Effort:** L
  - **Risk:** anti-bot; may need rotating proxies. Budget 3 sprints max before switching to a paid catalog.

- **Task M4.S1.T2** — Import pipeline. Ingest the scraper CSV into a new Postgres table `aftermarket_rockauto` with columns `(oem_normalized, brand, part_number, category, price_usd_cents, source_url, scraped_at)`. Add a merge step in `FindAftermarketForOEM_MultiPath` that unions this table alongside the TecDoc paths.
  - **Files:** `db/migrations/000020_aftermarket_rockauto.sql`, `internal/service/tecdoc.go`
  - **DoD:** for 5 seeded OEMs known to have RockAuto coverage, `AvgAM_correct` climbs by ≥ 3
  - **Effort:** M

- **Task M4.S1.T3** — Continuous refresh. Cron job that re-scrapes the top-500 most-queried OEMs weekly, invalidates entries > 30 days old.
  - **Files:** new `cmd/rockauto_refresher/`
  - **DoD:** container-image builds, dry-run mode against staging works
  - **Effort:** M

### Sprint M4.S2 — Regional supplier catalog (Saudi Arabia + Gulf)

- **Task M4.S2.T1** — Research task: identify the 5 largest regional parts distributors serving Hyundai/Kia in KSA/UAE/Oman (Ali Al-Ghanim Auto, Al-Futtaim Motors, Petromin, others). Which publish structured catalogs? Which offer bulk data feeds?
  - **DoD:** written report in `docs/data-sources/regional-catalog-survey.md`
  - **Effort:** M

- **Task M4.S2.T2** — For every supplier with a machine-readable catalog, build an importer. Landing table: `aftermarket_regional` with `(oem_normalized, supplier, brand, part_number, stock_status, region, url)`.
  - **Files:** new `scripts/scrapers/regional/{supplier}/`
  - **DoD:** at least 2 regional suppliers integrated, adding ≥ 1 avg replacement per correct HK OEM
  - **Effort:** L (per supplier)

### Sprint M4.S3 — Dealer parts catalog integration

- **Task M4.S3.T1** — Research task: Hyundai Global Service Way (GSW) + Kia World APIs. Are they accessible with a dealer partnership? Cost? Data shape?
  - **DoD:** written report in `docs/data-sources/dealer-catalog-survey.md`
  - **Effort:** M

- **Task M4.S3.T2** — If GSW/KWO become accessible, build a live proxy that fetches on demand and caches for 24h. Rate-limited to 5 req/s per contract.
  - **Files:** new `internal/service/dealer_catalog.go`
  - **DoD:** dealer-catalog data populates `OEMNumbers` for ≥ 90% of seeded HK OEMs when the source is enabled
  - **Effort:** L
  - **Dependency:** partnership agreement

### Sprint M4.S4 — Community contribution system

- **Task M4.S4.T1** — Frontend contribution form: "know an aftermarket alternative we missed? Add it here". Fields: OEM, aftermarket brand, part number, supplier URL, notes.
  - **Files:** `frontend/src/components/AftermarketContribute.tsx`, new `internal/handler/contribute.go`
  - **DoD:** contribution saved to `aftermarket_community` table; requires admin approval before appearing in search
  - **Effort:** L

- **Task M4.S4.T2** — Admin review UI + moderation queue.
  - **Files:** `frontend/src/pages/admin/moderation.tsx`, `internal/handler/admin_moderation.go`
  - **DoD:** admin can approve / reject / edit contributions; approved entries flow into the same `FindAftermarketForOEM_MultiPath` union
  - **Effort:** L

**M4 exit gate:** re-audit — `F1_correct` on body/glass slices ≥ 0.90; `AvgAM_correct` gains +3 avg from non-TecDoc sources.

---

## M5 — Search intelligence (semantic, VIN, related)

### Sprint M5.S1 — Description embeddings

- **Task M5.S1.T1** — Embed every article's `genericArticleDescription` using a small multilingual model (e.g. `paraphrase-multilingual-MiniLM-L12-v2`) into a `pgvector` column. Store dim=384.
  - **Files:** new `db/migrations/000030_article_embeddings.sql`, new `scripts/embed_articles/`
  - **DoD:** embed table populated for all 27M articles; index built with IVFFlat
  - **Effort:** L

- **Task M5.S1.T2** — Semantic search endpoint: `/api/search/semantic?q=oil filter for Sonata 2020&topK=20`. Uses the embedding index to return top-K candidates by cosine similarity.
  - **Files:** new `internal/handler/search_semantic.go`
  - **DoD:** for 20 natural-language queries, top-3 recall ≥ 0.80
  - **Effort:** M

### Sprint M5.S2 — VIN → parts pipeline

- **Task M5.S2.T1** — VIN decoder. Given a 17-char VIN, return `(make, model, year, engine, trim)` using an offline NHTSA-derived table + Hyundai/Kia WMI/VDS decoding rules.
  - **Files:** `internal/service/vin_decoder.go` (partially exists — expand)
  - **DoD:** unit test with 30 known HK VINs decodes correctly
  - **Effort:** M

- **Task M5.S2.T2** — `/api/vehicle/{vin}/parts?category=filters` endpoint. Returns all HK OEMs cataloged against that vehicle's `linkageTargetId`, grouped by category.
  - **Files:** new `internal/handler/vehicle_parts.go`
  - **DoD:** for a known VIN, returns ≥ 30 OEMs across ≥ 10 categories
  - **Effort:** M

### Sprint M5.S3 — Related parts recommendations

- **Task M5.S3.T1** — "If you're buying oil filter → also change: air filter, cabin filter, spark plugs" logic. Built from co-occurrence in vehicle service intervals + observed cart data.
  - **Files:** new `internal/service/related_parts.go`
  - **DoD:** for 10 known service-bundle categories, related-parts recall ≥ 0.70
  - **Effort:** M

**M5 exit gate:** VIN query F1 ≥ 0.80; description-similarity recall +10 pts on `real_hk_unseeded`.

---

## M6 — Production-grade

### Sprint M6.S1 — Continuous audit CI

- **Task M6.S1.T1** — Nightly cron: run `scripts/audit/audit-quality.ps1` against a small (200-OEM) canary corpus. Publish results to `docs/reports/nightly/{date}/`.
  - **Files:** new `.github/workflows/nightly-audit.yml`
  - **DoD:** workflow runs on schedule; commit-back on green
  - **Effort:** M

- **Task M6.S1.T2** — PR gate: any PR touching `internal/service/` must include an audit-diff. If `F1_correct` on the canary drops ≥ 0.02, block the merge.
  - **Files:** `.github/workflows/pr-quality-gate.yml`
  - **DoD:** synthetic-failure PR is blocked; passing PR merges cleanly
  - **Effort:** M

### Sprint M6.S2 — Feedback loop + cost SLA

- **Task M6.S2.T1** — Frontend "was this the right part?" thumbs-up/down after every search. Aggregate weekly, feed into `docs/reports/user-feedback-{week}.md`.
  - **Files:** `frontend/src/components/ResultFeedback.tsx`, new `internal/handler/feedback.go`
  - **DoD:** feedback stored in Postgres; weekly report generated
  - **Effort:** M

- **Task M6.S2.T2** — Cost monitoring. Per-request cost breakdown (DB queries × cost, external API calls × cost). Alert when p95 cost > $0.003/request.
  - **Files:** `internal/service/cost_meter.go`
  - **DoD:** cost dashboard in Grafana / equivalent
  - **Effort:** L

---

## M7 — AI/ML part-matching engine

**Problem statement.** Even after M0-M4 finish, structural data gaps remain: TecDoc has ~30M cross-refs but essentially zero aftermarket-brand rows for Hyundai/Kia OEMs (the 2026-08-26 TecDoc-health diagnostic confirmed: the top 30 brands on HK cross-refs are all car OEMs, no BOSCH/MANN/MAHLE/DENSO). RockAuto + regional catalogs (M4) close some of the gap but still leave long-tail OEMs unmatched.

M7 uses machine learning to **infer** cross-references and rank aftermarket alternatives that are not stored anywhere as a direct row. It builds on the artifacts already produced by M0-M6:

- **30 M `articlecrosses` rows** — training gold for cross-brand OEM equivalence
- **27 M `articlecriteria` rows** — spec vectors per part
- **340 M `articlesvehicletrees` rows** — vehicle-fitment co-occurrence signal
- **`article_embeddings` from M5.S1** — 384-dim semantic vectors on descriptions
- **`search_feedback` from M6.S2** — thumbs-up / -down implicit relevance signal
- **`aftermarket_rockauto` + `aftermarket_community` from M4** — external ground-truth for HK aftermarket coverage

### Sprint M7.S1 — Analogical OEM inference (nearest-neighbor cross-brand)

**Goal:** For an HK OEM with zero direct aftermarket data, find the "nearest-neighbor" OEM from a brand that DOES have aftermarket coverage (e.g. a Toyota oil filter with the same specs) and surface its aftermarket brands as inferred alternatives.

- **Task M7.S1.T1** — Build a `part_vectors` table: for every article, concatenate `(dataSupplierId, assemblyGroupNodeId, top-N criteria as a sparse vector, description embedding)` into a 512-dim vector. Store in pgvector.
  - **Files:** new `db/migrations/000040_part_vectors.sql`, new `cmd/build_part_vectors/`
  - **DoD:** 6.9 M rows populated; IVFFlat index built; median build time <2 h
  - **Effort:** L
- **Task M7.S1.T2** — `FindAftermarketByAnalogy(oem)` service. When primary + multi-path UNION returns fewer than 3 real aftermarket brands, run: (1) look up the OEM's article vector; (2) k-NN search for top-20 nearest articles across all brands; (3) union their aftermarket cross-refs; (4) tag results as `source=analogy` with a confidence score derived from cosine similarity.
  - **Files:** new `internal/service/analogy_matcher.go`, wire into `tecdoc.go:FindAftermarketForOEM_MultiPath`
  - **DoD:** on 50 seeded HK OEMs known to have zero direct aftermarket rows, at least 30 return ≥ 3 inferred aftermarket brands with cosine ≥ 0.85
  - **Effort:** L
- **Task M7.S1.T3** — UI badge + per-result confidence. Analogy hits render with a `Inferred (85% match)` badge so the parts seller understands provenance.
  - **Files:** `frontend/src/components/PartResult.tsx`
  - **DoD:** badge shows for source=analogy; unit tests cover the render
  - **Effort:** S

### Sprint M7.S2 — Learned ranker (feedback-trained)

**Goal:** Replace the hand-tuned strategy priority weights (`strategy.go:strategyPriority`) with a learned ranker that optimises for user-observed relevance (feedback thumbs + click-throughs).

- **Task M7.S2.T1** — Feature-engineering pipeline. For every `(query_oem, result_article)` pair in the last 90 days of `search_feedback`, compute features: strategy source, confidence, brand tier, category-system match, semantic distance, has-specs flag, has-vehicle-fitment flag, spec-match count. Land in `training_features` table.
  - **Files:** new `cmd/build_training_set/`, new `db/migrations/000041_training_features.sql`
  - **DoD:** ≥ 50 K feature rows generated; feature-store column-completeness > 95%
  - **Effort:** M
- **Task M7.S2.T2** — Train a LightGBM-based ranker. Objective: `LambdaRank` optimising nDCG@5. Hold out 20% for validation. Publish the model + feature-importances to `models/ranker-{date}.txt`.
  - **Files:** new `scripts/train_ranker/train.py` (Python is fine — offline training is decoupled from the Go runtime)
  - **DoD:** nDCG@5 on held-out set ≥ 0.75; feature-importance report attached to PR
  - **Effort:** L
- **Task M7.S2.T3** — In-Go inference. Load the LightGBM model via `leaves` (pure-Go LightGBM inference lib). Score every candidate result post-dedupe, before final sort. Add an A/B toggle `USE_ML_RANKER=true/false`.
  - **Files:** new `internal/service/ml_ranker.go`; wire into `searchCombined`
  - **DoD:** p95 latency impact <50 ms; toggle-off matches current behaviour byte-for-byte
  - **Effort:** M
- **Task M7.S2.T4** — Weekly retrain cron. Rebuild feature set + retrain + hot-swap model file. If nDCG@5 regresses on the held-out set, reject the new model and alert.
  - **Files:** `.github/workflows/ml-ranker-retrain.yml`, `scripts/train_ranker/promote.py`
  - **DoD:** cron runs weekly; regression rollback path exercised in dry-run
  - **Effort:** M

### Sprint M7.S3 — Spec-conditioned equivalence learning

**Goal:** For wear parts (filters, pads, plugs) — the categories where a wrong spec means the part physically won't fit — train a classifier that predicts "these two OEMs are equivalent" using only their `articlecriteria` spec vectors. This lets the engine claim equivalence between an HK OEM and an aftermarket part that shares thread size, diameter, height, filter media, etc.

- **Task M7.S3.T1** — Positive/negative pair mining from `articlecrosses`. For every existing cross-ref row, generate `(oem_A, oem_B, label=1)` positive pairs. Generate negatives by sampling `(oem_A, oem_random_same_category, label=0)` at 4:1 ratio. Land in `equivalence_training_pairs`.
  - **Files:** new `cmd/mine_equivalence_pairs/`, new `db/migrations/000042_equivalence_training_pairs.sql`
  - **DoD:** ≥ 500 K pairs generated; label balance verified
  - **Effort:** M
- **Task M7.S3.T2** — Train a siamese network on `part_vectors` (from M7.S1.T1). Output: probability that two parts are functionally equivalent. Architecture: 2-layer MLP with contrastive loss.
  - **Files:** new `scripts/train_equivalence/train.py`
  - **DoD:** AUROC on held-out pairs ≥ 0.92; false-positive rate <5% at operating threshold
  - **Effort:** L
- **Task M7.S3.T3** — In-Go equivalence-scorer. Given a query OEM + a candidate part, return equivalence probability. Callable from `searchCombined` to filter out low-probability candidates before final sort.
  - **Files:** new `internal/service/equivalence_scorer.go`
  - **DoD:** integrated behind `USE_ML_EQUIVALENCE=true` flag; wear-part F1_correct climbs by ≥ 5 pts on the canary
  - **Effort:** M

### Sprint M7.S4 — LLM query understanding + natural-language search

**Goal:** Support free-text queries like *"oil filter for 2020 Sonata 2.4L"* or *"cheapest aftermarket brake pads for Elantra AD"* — currently the engine only accepts OEM numbers or VIN.

- **Task M7.S4.T1** — LLM-driven query parser. Given free text, extract `(part_type, make, model, year, engine, brand_pref, tier_pref)`. Use a small model (Llama 3 8B or gpt-4o-mini) with structured-output constraint. Cache identical queries for 24h.
  - **Files:** new `internal/service/nl_query_parser.go`, new `internal/service/llm_client.go`
  - **DoD:** for 100 real-world natural-language queries from parts-seller logs, parser recall ≥ 0.90 on all extractable fields
  - **Effort:** M
- **Task M7.S4.T2** — Query-router: after parsing, dispatch to (a) VIN pipeline if VIN detected, (b) semantic search + vehicle-fitment JOIN if part_type + vehicle detected, (c) full-text fallback otherwise.
  - **Files:** `internal/service/query_router.go`
  - **DoD:** for 20 natural-language test queries, correct routing 20/20; nDCG@5 ≥ 0.70 vs a human-curated top-5
  - **Effort:** M
- **Task M7.S4.T3** — Explainability. Every result returned via NL query includes a one-line "why this?" attribution (matched via spec/VIN/analogy/etc.).
  - **Files:** `frontend/src/components/PartResult.tsx`
  - **DoD:** attribution renders for 100% of NL results; A/B trial shows +8 pts thumbs-up rate
  - **Effort:** S

**M7 exit gate:** re-audit — on the 50-OEM canary with zero direct aftermarket data, `AvgAM_inferred ≥ 3`; on the 1490-corpus audit, ranker-vs-heuristic A/B shows recall@5 +15 pts on the unseeded slice; NL-query nDCG@5 ≥ 0.70.

### M7 risk register

| Risk | Mitigation |
|---|---|
| Analogy inference returns wrong-category parts | Gate by `assemblyGroupNodeId` match (once M3 populates it) + cosine threshold ≥ 0.85 + label as "inferred" so seller can dismiss |
| Learned ranker overfits to a narrow demographic of users | Weekly retrain with 90-day sliding window; nDCG regression rollback in the promote workflow |
| LLM query parser hallucinates model years / trims | Structured-output JSON constraint; parser output validated against known NHTSA VIN patterns before dispatch |
| Cost of embedding + LLM inference blows the $0.003/request SLA | Aggressive caching (identical NL queries 24h, embeddings 30d); fall back to keyword search when hot-path exceeds 200ms |
| Serving stale models after data-source refresh | Model version stamped into every result; audit runs pinned to a specific model file |

---

## Cross-cutting concerns

### Data quality gates on every task

Every task that touches search/enrichment must:
1. Have a new or updated regression test that fails without the change
2. Run `pwsh scripts/audit/audit-quality.ps1 -InputCorpus scripts/audit/corpus-1500-v2.csv` and attach the diff to the PR body
3. Not regress `F1_correct`, `AvgRepl_correct`, or `F1_rich5` on the seeded slice

### KISS constraint (from CLAUDE.md)

- No conditional fallback paths in production. Single path, fail loud on migration drift.
- Remove old code when refactoring. No dual-branch queries. No version switches.

### Rebase rule

Every task branches from fresh `main`:
```bash
git checkout main && git pull && git checkout -b {milestone}/{task}
```

### Task template (for agents picking up work)

```markdown
## Task: {milestone}.{sprint}.T{n} — {short title}

**Goal:** 1-2 sentences describing what changes for the user.

**Files to touch:**
- `path/one.go`
- `path/two.go`

**Approach:**
1. …
2. …

**Acceptance criteria (DoD):**
- [ ] Unit test at `path/one_test.go` covers …
- [ ] `go build ./... && go vet ./... && go test ./...` all pass
- [ ] Metric X on the audit script climbs from A to B (attach re-run diff)
- [ ] PR body includes before/after screenshot or CSV row

**Effort:** S / M / L

**Depends on:** {milestone}.{sprint}.T{k} (if any)
```

---

## Progress tracking

Each merged PR appends to `docs/reports/` and stamps the milestone in its commit trailer:

```
Milestone: M1.S1
Task: M1.S1.T2
```

Team can query `git log --grep 'Milestone: M1'` to see everything shipped under the current milestone.

The `by-slice.csv` from `analyze-quality.ps1` is the single source of truth for "are we hitting the north-star?". A weekly review looks at:
1. F1_correct trend on seeded slice
2. AvgRepl_correct trend on wear parts
3. F1_rich5 trend on wear parts
4. Non-HK guard leak count

If any of these regress week-over-week, the roadmap pauses new work and swarms the regression.
