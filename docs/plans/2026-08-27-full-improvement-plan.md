# Full Improvement Plan — Rating 5.1 → 8.0 · Kia + Hyundai · No Paid Data

**Date:** 2026-08-27
**Scope:** Kia and Hyundai only. No paid data-source purchase commitment. Aftermarket gap filled via free/public online-source meta-search with local caching.
**Sequence:** Follows PR #22 (diagnostics + `sql/08`) + PR #25 (engine-health runbook).

---

## Constraints locked in

| Constraint | Effect on plan |
|---|---|
| Kia + Hyundai only | Skip SDC/Auto Care/broad-market plays; focus every source on HK OEM patterns (26/58/82/97 prefixes) |
| No paid data (this iteration) | Drop TecAlliance Web API upgrade, HaynesPro, PartsLink24, direct brand-API purchases; use only free/public sources |
| **No scraping** in the crawler sense | ToS-hostile sites are OUT (RockAuto, Autodoc's automated queries, sites that ban bots) |
| Use free public search interfaces as a "search tool" | Federated on-demand meta-search across sources that either (a) have an official API or (b) publish schema.org-marked machine-readable data on public reference pages |
| Cache their results | Postgres cache table with configurable TTL per source; refresh on cache miss |

**Relationship to M7 (AI/ML matching):** This plan targets Dim 3 via M8 (online-search aggregation). M7 remains a longer-term ML play that depends on M0-M6 finishing + M8 producing training-data signal (`aftermarket_online_cache` rows can seed the analogical-inference model in M7.S1). M7 is not part of the $0 5-wave plan below; it stays on the roadmap for post-Wave-5 evaluation.

---

## Target scores (revised for no-paid path)

| Dim | Current | Target | Method |
|---|:-:|:-:|---|
| 1. Primary OEM data | 6 | **8** | Fix `owned_catalog` + catalog endpoint + HK OEM regex filter |
| 2. Alternative-OEM coverage | 7 | **9** | Apply sql/08 + fix `mfrId` bug + supersession-strategy fix |
| 3. Aftermarket replacement | 2 | **6** | eBay Motors API + community contributions + schema.org meta-search + caching |
| 4. Brand catalog breadth | 5 | **7** | Brand normalization audit + lang case-normalize + dragged along by Dim 3 |
| 5. Smart Search reliability | 5 | **9** | Five reliability PRs (transparency, merge, drops, ctx, circuit) |
| 6. Cache confidence correctness | 3 | **9** | Confidence-cap fix (one PR) |
| 7. Plan clarity | 8 | **10** | This document + updated ROADMAP + dashboard |
| **Overall** | **5.1** | **8.0** | ~$0 committed spend |

Realistic target: **7.9 – 8.0**. If Wave 3's online-search sources happen to have deeper HK aftermarket coverage than expected, Dim 3 could hit 7 and overall goes to 8.1.

---

## Free / public data sources — legal green tier

Only sources with either an official API or explicit machine-readable data policy:

### G1 · eBay Motors Finding API — the anchor source

- **Provider:** eBay Inc.
- **Endpoint:** `https://svcs.ebay.com/services/search/FindingService/v1`
- **Cost:** $0 for 5,000 API calls/day (Standard tier). $0 upgrade to 1.5M calls/day possible with Enterprise application.
- **Legal:** official free API. Terms allow commercial use with attribution.
- **Coverage for HK OEMs:** ⭐⭐⭐⭐ — massive marketplace with millions of Kia/Hyundai listings globally including MENA
- **Data returned per OEM lookup:**
  - Seller-provided title (parseable for brand + part number)
  - Brand (from item specifics `Brand` aspect)
  - Manufacturer part number (from item specifics `MPN`)
  - Condition (new / used / remanufactured)
  - Image URL
  - Price + currency
  - Seller feedback score (proxy for reliability)
- **Rate limit:** 5,000 calls/day standard = ~200 unique OEM lookups/day if we query 25 keywords per OEM. With 30-day cache → covers thousands of unique OEMs/month.
- **Data quality:** seller-provided → noisy but abundant. Requires brand normalization (M2.S2 `NormalizeBrand` already exists).

### G2 · NHTSA vPIC API — already integrated

- **Provider:** US National Highway Traffic Safety Administration
- **Cost:** $0 unlimited (soft rate limit ~10 req/sec)
- **Legal:** US Government public data
- **Coverage:** VIN decode + safety recall data for US-market vehicles including HK models
- **Data:** already flowing into `TecDoc.LinkageTargetsForNHTSA` (M5.S2)
- **Not new to this plan** — noted for completeness

### G3 · CarQueryAPI — already integrated

- **Provider:** carqueryapi.com
- **Cost:** $0 unlimited
- **Legal:** open API
- **Coverage:** make/model/year/trim metadata for global vehicles
- **Data:** already flowing
- **Not new to this plan**

### G4 · fueleconomy.gov API — already integrated

- **Provider:** US EPA
- **Cost:** $0 unlimited
- **Legal:** US Government public data
- **Coverage:** EPA fuel-economy + emissions specs for US-market vehicles
- **Data:** already flowing
- **Not new to this plan**

### G5 · Schema.org JSON-LD from publicly-viewable OEM reference pages

**This is the "online search tool" pattern.**

- **Providers:** HyundaiPartsDeal.com, KiaPartsNow.com, 7zap.com — publicly-viewable reference pages
- **Cost:** $0
- **Legal:** these sites publish `Product` schema.org JSON-LD blocks in their public HTML **specifically for machine consumption by search engines**. Extracting the JSON-LD is technically what Google/Bing do to build knowledge cards.
- **Requirements to stay legal:**
  1. Respect `robots.txt` — check `Disallow` directives, skip disallowed paths
  2. Identify our User-Agent clearly: `IfritahPartsEngine/1.0 (+https://ifritah.com/robots-info)`
  3. Rate-limit to 1 request per 2 seconds per source (below Google-Bot rates)
  4. Only query **on user demand** (never bulk crawl)
  5. Cache 30 days minimum per (source, OEM) tuple
  6. Provide back-link attribution in any result surfaced to the user
- **Coverage per source:**
  - **HyundaiPartsDeal.com** ⭐⭐⭐⭐ — authoritative dealer-side Hyundai reference; exploded diagrams; supersession
  - **KiaPartsNow.com** ⭐⭐⭐⭐ — sister site, Kia-side
  - **7zap.com** ⭐⭐⭐ — global OEM catalog including MENA-market Kia/Hyundai
- **Data returned:** OEM part description, list price, superseded-by, exploded-diagram category, cross-references
- **Reference:** your `internal/service/reference_engine_comparison_test.go:11-25` already names these three sites as "authoritative OEM dealer pages used as cross-validation ground truth"

### G6 · oilfilter-crossreference.com (and analogues per category)

- **Provider:** free community reference sites for specific part families
- **Cost:** $0
- **Legal:** public reference data, similar posture to G5
- **Coverage:** ⭐⭐⭐ per part family — verified benchmark data
- **Reference:** your `reference_engine_comparison_test.go:15` names this as a verified benchmark (519 alternatives for MANN W 811/80)

### Excluded — sources we will NOT use

| Source | Reason |
|---|---|
| RockAuto | Explicit ToS prohibition on automation; anti-bot; user constraint "no scraping" |
| Autodoc.co.uk automated queries | ToS restricts automation |
| Amazon product search without API | ToS restricts scraping |
| Google Search results scraping | ToS restricts; use their paid API if needed (not in this plan) |
| Any site with Disallow directive in robots.txt covering our paths | Legal + technical fragility |

---

## Per-dimension plan

### Dim 1 · Primary OEM data (6 → 8)

Free-only path. Same as prior plan.

- **A1.1** Populate `hk_parts_cache` via `derive_worker` (M0.T1)
- **A1.2** Add HK OEM validation regex (10-char alphanumeric) + wire into `oem_number` reads (M0.T5)
- **A1.3** Fix `/api/catalog/vehicles` (returns 0 today) (M0.T4 sub-A)
- **A1.4** Corpus enricher for `linkageTargetIds` (M0.T4 sub-B, M0.T6)

### Dim 2 · Alternative-OEM coverage (7 → 9)

Free-only path.

- **A2.1** Apply `sql/08` hotfix (PR #22, ops task)
- **A2.2** Fix `strategy_spec_match.go` `mfrId → dataSupplierId`
- **A2.3** Fix supersession strategy (M0.T2 — article-id promotion)
- **A2.4** Verify M2.S3 supersession walker end-to-end via tecdoc_diagnostic_full.sql Part B §9

### Dim 3 · Aftermarket replacement (2 → 6)

**The core of this plan.** Free-only path via online-source meta-search.

This becomes a new milestone **M8 — Online search aggregation** with its own sprint file at `docs/sprints/M8-online-search-aggregation.md`. Summary:

1. **Cache table** — `aftermarket_online_cache` with `(source, oem_normalized, brand, part_number, price, currency, condition, image_url, source_url, fetched_at, ttl_days)`
2. **eBay Motors API client** — anchor source for aftermarket volume
3. **Public-reference clients** — HyundaiPartsDeal / KiaPartsNow / 7zap adapters using schema.org JSON-LD extraction
4. **Cross-reference-site clients** — oilfilter-crossreference.com and equivalents per part family
5. **Federated meta-search service** — fan out to all sources in parallel, dedupe + brand-normalize + cache
6. **UNION into `FindAftermarketForOEM_MultiPath`** — online results tagged `source=online:<source_id>` with provenance
7. **UI provenance badge** — "Sourced from eBay Motors" with click-through URL for buyer
8. **Rate + cost monitoring** — dashboard showing calls/day per source

### Dim 4 · Brand catalog breadth (5 → 7)

Free-only path.

- **A4.1** Audit + expand M2.S2 `NormalizeBrand` canonical map — cross-check against G1/G5 source brands
- **A4.2** Case-normalize `ambrand.lang` in all 25 hardcoded `lang='en'` sites
- **A4.3** Verify `BrandTier` ordering keeps genuine OEMs at top

### Dim 5 · Smart Search reliability (5 → 9)

Free-only path. Five agent-sprints, all internal code.

- **A5.1** Expose strategy-skipped/errored state in `SmartSearchResponse.Warnings[]`
- **A5.2** Merge metadata on convergence (union specs + brand) — no more overwrite loss
- **A5.3** Log unstable-ID drops to debug endpoint
- **A5.4** Extend `mode=combined` ctx to 20 s + real-time progress bar
- **A5.5** Circuit-breaker announcement in progress stream

### Dim 6 · Cache confidence correctness (3 → 9)

Free-only path. One agent-sprint.

- **A6.1** In `oem_cache.go:172-180`, change confidence cap: add tier `WHEN corroborating_sources + 1 >= 5 THEN 1.0`. Preserves `verified_by_user = 1.0` semantics while unblocking automatic ≥5-source verification.

### Dim 7 · Plan clarity (8 → 10)

- **A7.1** This plan document
- **A7.2** ROADMAP update to include M8
- **A7.3** Dashboard from nightly-audit CSVs — per-source `AvgAM_correct` contribution

---

## Wave breakdown — 30 agent-sprints, ~10-12 weeks

**Every sprint = one atomic task an agent can complete without waiting on another agent.**

### Wave 1 · $0 fast wins (7 sprints — fully parallel)

Each sprint is independent — an agent can execute any one without waiting for the others.

| # | Sprint | Dim | Size | Files |
|---|---|:-:|:-:|---|
| W1.1 | Cache confidence cap fix (add ≥5-source tier) | 6 | S | `oem_cache.go`, one test file |
| W1.2 | `strategy_spec_match.go` `mfrId → dataSupplierId` | 2 | S | `strategy_spec_match.go`, one test |
| W1.3 | Apply `sql/08` hotfix + verify EXPLAIN | 2 | — | ops task, no code |
| W1.4 | Expose strategy-skipped/errored state in Warnings + progress events | 5 | S | `strategy.go`, `smart_search.go`, progress types |
| W1.5 | Merge metadata on convergence (union specs/brand) | 5 | S | `strategy.go` collect loop |
| W1.6 | Log dropped unstable-ID results to debug endpoint | 5 | S | `strategy.go`, new debug handler |
| W1.7 | Extend combined-mode ctx to 20 s + progress bar | 5 | S | `strategy.go`, frontend progress renderer |

**End of Wave 1 rating projection:** Dim 6 → 9, Dim 2 → 8, Dim 5 → 7. Overall 5.1 → 6.4.

### Wave 2 · Data-layer fixes (8 sprints — mostly parallel)

| # | Sprint | Dim | Size | Files |
|---|---|:-:|:-:|---|
| W2.1 | Populate `hk_parts_cache` via `derive_worker` run + verify | 1 | M | `derive_worker.go`, ops trigger |
| W2.2 | HK-OEM validation regex + wire into `oem_number` reads | 1 | S | `oem_prefix.go`, `strategy.go` |
| W2.3 | Fix `/api/catalog/vehicles` (currently returns 0) | 1 | M | `catalog.go`, test |
| W2.4 | Corpus enricher `enrich_corpus_linkages.go` | 1 | M | new `scripts/audit/enrich_corpus_linkages.go`, corpus CSV column |
| W2.5 | Fix M0.T2 supersession strategy — article-id promotion | 2 | M | `tecdoc_supersession.go`, `SupersessionStrategy` in `strategy.go` |
| W2.6 | Circuit-breaker skip announcement in progress stream | 5 | S | `strategy.go` circuit-open path |
| W2.7 | Brand normalization map audit + expand `NormalizeBrand` | 4 | S | `tecdoc.go` |
| W2.8 | `ambrand.lang` case-normalization sweep across 25 hardcoded sites | 4 | S | multiple `internal/service/*.go` files |

**End of Wave 2:** Dim 1 → 8, Dim 4 → 7, Dim 5 → 9. Overall 6.4 → 7.4.

### Wave 3 · Online-search aggregation for aftermarket (12 sprints)

Full detail in `docs/sprints/M8-online-search-aggregation.md`.

Summary sequence:

| # | Sprint | Size | Files |
|---|---|:-:|---|
| W3.1 | `aftermarket_online_cache` schema migration | S | `db/migrations/000021_aftermarket_online_cache.sql` |
| W3.2 | eBay Motors Finding API client + auth | M | new `internal/service/online_ebay.go` |
| W3.3 | Response normalizer — eBay item → canonical AftermarketResult | S | `online_ebay.go` |
| W3.4 | Federated meta-search service scaffold | M | new `internal/service/online_search.go` |
| W3.5 | Schema.org JSON-LD extractor (`Product` type) | S | new `internal/service/schema_org_parser.go` |
| W3.6 | HyundaiPartsDeal.com adapter using G5 pattern | M | new `internal/service/online_hyundaipartsdeal.go` |
| W3.7 | KiaPartsNow.com adapter | M | new `internal/service/online_kiapartsnow.go` |
| W3.8 | 7zap.com adapter | M | new `internal/service/online_7zap.go` |
| W3.9 | robots.txt fetcher + compliance guard for G5 sources | S | new `internal/service/robots_guard.go` |
| W3.10 | Rate-limiter per source (token bucket) | S | new `internal/service/rate_limiter.go` |
| W3.11 | UNION online results into `FindAftermarketForOEM_MultiPath` | M | `tecdoc.go` |
| W3.12 | Frontend badge for online-sourced results with attribution | S | `frontend/src/components/PartResult.tsx` |

**End of Wave 3:** Dim 3 → 6, Dim 4 → 7. Overall 7.4 → 7.9.

### Wave 4 · Community contributions public UI + polish (3 sprints)

| # | Sprint | Size |
|---|---|:-:|
| W4.1 | Frontend `AftermarketContribute` component | M |
| W4.2 | Admin moderation UI | M |
| W4.3 | Public rate limits + spam protection for community endpoint | S |

**End of Wave 4:** community contributions live — organic long-tail. Overall 7.9 → 8.0.

### Wave 5 · Verification + close-out (3 sprints)

| # | Sprint | Size |
|---|---|:-:|
| W5.1 | Full 1490-corpus audit re-run using PR #25 runbook | S |
| W5.2 | Publish `docs/reports/2026-XX-XX-post-plan-audit/` with delta vs 2026-08-25 baseline | S |
| W5.3 | Update `.github/workflows/pr-quality-gate.yml` with new baseline thresholds | S |

---

## Rating projection table

| Wave | Dim 1 | Dim 2 | Dim 3 | Dim 4 | Dim 5 | Dim 6 | Dim 7 | Overall |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| Start (2026-08-27) | 6 | 7 | 2 | 5 | 5 | 3 | 8 | **5.1** |
| End of Wave 1 | 6 | 8 | 2 | 5 | 7 | 9 | 8 | **6.4** |
| End of Wave 2 | 8 | 9 | 2 | 7 | 9 | 9 | 8 | **7.4** |
| End of Wave 3 | 8 | 9 | 6 | 7 | 9 | 9 | 9 | **7.9** |
| End of Wave 4 | 8 | 9 | 6 | 7 | 9 | 9 | 10 | **8.0** |
| End of Wave 5 | 8 | 9 | 6 | 7 | 9 | 9 | 10 | **8.0** |

**Committed cost across all 5 waves: $0.**

---

## Risks + mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| eBay Motors API rate limit (5K/day) too low | Dim 3 doesn't reach 6 | Cache aggressively (30d TTL); apply for Enterprise tier (still free, higher quota); prioritize top-500 OEMs |
| G5 sites change ToS or add Cloudflare block | Wave 3 partial | Feature-flag each adapter; graceful-degrade; keep eBay as anchor |
| eBay Motors data quality noisy (seller-provided) | Brand attribution imprecise | Reuse existing `NormalizeBrand` + `BrandTier` — same pipeline as TecDoc results |
| tecdoc_diagnostic_full.sql Part C §12 shows existing coverage already sufficient | Wave 3 was unnecessary | Reduce Wave 3 to eBay-only, skip G5 adapters. Not a regression risk. |
| Community contributions attract spam | Data pollution | Rate limit + admin moderation gate (already in PR #20 backend); public UI enforces reCAPTCHA |
| sql/08 apply blocks writes on prod-scale table | 15-min DDL window | Schedule during low-traffic; use `pt-online-schema-change` if needed |

---

## Legal / compliance notes for online-source usage

**Non-negotiable per every G5 adapter:**

1. Fetch and parse `robots.txt` on first query per hostname per day. Skip disallowed paths.
2. Rate-limit each source hostname to 1 request per 2 seconds.
3. Send User-Agent header: `IfritahPartsEngine/1.0 (+https://ifritah.com/bot-info)`
4. Only send outbound requests **in response to a user-initiated search** — never on cron.
5. Cache 30 days minimum before re-fetching same `(source, oem)` pair.
6. Attribute every surfaced result to its source URL with a click-through link.
7. Store `robots.txt` snapshot per host per fetch date for audit trail.
8. Kill switch (`ONLINE_SEARCH_ENABLED=false`) that disables the entire subsystem in one env var.

If any G5 source contacts us objecting to our use, we immediately disable that adapter's feature flag and delete cached data.

---

## Suggested PR sequence — one PR per Wave

| Order | PR | Base |
|---|---|---|
| 1 | PR #22 (already open) — diagnostics + sql/08 | main |
| 2 | PR #25 (already open) — engine-health runbook | #22 |
| 3 | **This plan PR** — docs only, adds `docs/sprints/M8` + ROADMAP + this plan doc | #25 |
| 4 | Wave-1 code PR — 6 code sprints as one PR (W1.1 - W1.7 except ops) | main (after #22-#25 merge) |
| 5 | Wave-2 code PR — 8 sprints | main |
| 6 | Wave-3 online-search PR — 12 sprints; can be split into 3 sub-PRs |     main |
| 7 | Wave-4 community UI PR | main |
| 8 | Wave-5 verification report PR (post-audit `docs/reports/` doc) | main |

---

## Exit gates

**Wave 1 exit:** `oem_resolution_cache` shows fresh + cached returning identical confidence; `strategy_spec_match` results carry non-empty `brandName`; combined mode reports skipped/errored strategies in Warnings.

**Wave 2 exit:** `hk_parts_cache` populated with ≥ 1 hit for 5 seeded OEMs (`26350-2J001`, `58101-3XA00`, `82460-2T010`, `97133-2S000`, `82370-3SA00`); `/api/catalog/vehicles?make=Hyundai&model=Elantra` returns ≥ 5 vehicles.

**Wave 3 exit:** for the 19-OEM audit corpus, `AvgAM_online ≥ 3` (average aftermarket brands from online sources per corpus OEM); cache hit rate ≥ 70% on repeat queries within 30 days.

**Wave 4 exit:** community contribution UI live; end-to-end submission → admin moderation → search-visible pipeline verified with 1 test contribution.

**Wave 5 exit:** full 1490-corpus audit against post-Wave-4 build shows overall `F1_correct` ≥ 0.75 (from 0.25 today) OR published deviation report explains gap. `pr-quality-gate.yml` baseline updated.

---

## Open items requiring decisions

1. Is a follow-up paid data purchase acceptable at end of Wave 5 IF Dim 3 stalls at 6? (This plan defers that decision; user has veto.)
2. What is the ceiling `ONLINE_SEARCH_ENABLED` should default to? Recommend `false` in prod; `true` on qa for verification. Operator toggles when comfortable.
3. Who acts as the moderation queue reviewer for community contributions (W4.2)? Needs at least 1 human hour/day at steady state.
