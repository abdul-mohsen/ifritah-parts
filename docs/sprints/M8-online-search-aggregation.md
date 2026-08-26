# M8 — Online-Search Aggregation for Aftermarket Coverage

**Milestone owner:** search-quality
**Depends on:** PR #22 (`sql/08` applied), PR #25 (audit runbook), Wave 1 code fixes (spec-match + cache confidence)
**Cost:** $0 committed spend. All sources free or free-tier.
**Estimated agent-sprints:** 12 sprints
**Goal:** Close the Dim 3 (aftermarket replacement data) gap from 2 → 6 without paid data or scraping. Federated meta-search across free/public sources with local caching.

---

## Problem statement

The 2026-08-25 post-deploy audit measured `AvgAM_correct = 0.09` for HK OEMs. The 2026-08-26 TecDoc-health diagnostic (PR #22) showed the top-30 brands on HK-prefix `articlecrosses` are all car OEMs — no BOSCH, MANN, MAHLE, DENSO, etc. Even after the Wave 1 `mfrId → dataSupplierId` fix unlocks whatever's hidden in the current dump, the underlying data is likely structurally sparse for HK aftermarket brands.

Rather than commit to paid data (TecDoc Web API, HaynesPro, etc.), M8 fills the gap by treating the internet as a federated aftermarket catalog: on-demand queries to free public sources, aggressive local caching, no scraping in the crawler sense.

## Approach — five parts

1. **Cache table** — `aftermarket_online_cache` — one row per (source, OEM, part) tuple with TTL
2. **Source adapters** — one Go package per source implementing a common interface
3. **Meta-search dispatcher** — parallel fan-out to enabled sources
4. **UNION into existing aftermarket path** — online results appear alongside TecDoc results, tagged with provenance
5. **Compliance layer** — robots.txt + rate limits + kill switch

## Green-tier sources (free, legal, this milestone)

| # | Source | Type | Rate limit | Data quality |
|---|---|---|---|---|
| G1 | eBay Motors Finding API | Official free API | 5,000 calls/day | Massive volume, seller-noisy |
| G5a | HyundaiPartsDeal.com | Schema.org JSON-LD on public pages | 1 req / 2 sec | Authoritative OEM reference |
| G5b | KiaPartsNow.com | Schema.org JSON-LD on public pages | 1 req / 2 sec | Authoritative OEM reference |
| G5c | 7zap.com | Schema.org JSON-LD on public pages | 1 req / 2 sec | Global OEM catalog |
| G6 | Category-specific reference sites (e.g. oilfilter-crossreference.com) | Public HTML | 1 req / 5 sec | Cross-reference gold-standard |

## Non-goals

- No scraping of ToS-hostile sites (RockAuto, Amazon, Autodoc automated queries)
- No paid API commitment (defer until Wave 5 evidence permits it)
- No live merging into primary path when online source is DOWN — graceful degradation
- No bulk-crawling — every outbound call is user-search-triggered

---

## Sprints

Each sprint is an atomic agent task, self-contained, reviewable in ≤ 30 min.

### Sprint M8.T1 — `aftermarket_online_cache` schema

**Goal:** ship the persistent cache table for all online-sourced results.

**Files:**
- `db/migrations/000021_aftermarket_online_cache.sql`

**Schema:**

```sql
CREATE TABLE aftermarket_online_cache (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(32) NOT NULL,           -- 'ebay' / 'hyundaipartsdeal' / 'kiapartsnow' / '7zap' / etc.
    oem_normalized VARCHAR(64) NOT NULL,   -- LOWER(REPLACE(REPLACE(REPLACE(oem, '-', ''), ' ', ''), '.', ''))
    brand VARCHAR(128),                    -- Normalized via NormalizeBrand
    part_number VARCHAR(128),
    description TEXT,
    price_cents BIGINT,
    currency CHAR(3),
    condition VARCHAR(32),                 -- 'new' / 'used' / 'reman' / 'oem_genuine' / 'unknown'
    image_url TEXT,
    source_url TEXT NOT NULL,              -- click-through attribution URL
    raw_payload JSONB,                     -- full source response for debugging
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ttl_seconds INTEGER NOT NULL DEFAULT 2592000,  -- 30 days default
    UNIQUE (source, oem_normalized, brand, part_number)
);

CREATE INDEX idx_aftermarket_online_cache_oem  ON aftermarket_online_cache (oem_normalized);
CREATE INDEX idx_aftermarket_online_cache_fresh ON aftermarket_online_cache (source, fetched_at);
```

**Acceptance:**
- Migration applies cleanly on qa
- Golang model + repository written with basic upsert + read-by-OEM
- 3 unit tests: insert, upsert (deduplication), TTL-based freshness check

**Effort:** S

---

### Sprint M8.T2 — eBay Motors Finding API client

**Goal:** hit eBay Motors Finding API for a given OEM query; return normalized results.

**Files:**
- `internal/service/online_ebay.go` (new)
- `internal/service/online_ebay_test.go` (new)

**Approach:**

1. Add env var `EBAY_APP_ID` (secret; read from `os.Getenv` with fallback to `""`).
2. Implement `EbayFinder` struct with `Search(oem string) ([]OnlineResult, error)`.
3. Query the Finding Service:
   ```
   GET https://svcs.ebay.com/services/search/FindingService/v1
       ?OPERATION-NAME=findItemsAdvanced
       &SERVICE-VERSION=1.0.0
       &SECURITY-APPNAME=<EBAY_APP_ID>
       &RESPONSE-DATA-FORMAT=JSON
       &keywords=<OEM>+Hyundai
       &categoryId=6028              // Motors > Parts & Accessories
       &itemFilter(0).name=Condition
       &itemFilter(0).value=New
       &paginationInput.entriesPerPage=25
   ```
4. Parse response; extract `title`, `productId`, `sellerInfo.feedbackScore`, `galleryURL`, `sellingStatus.currentPrice`, `viewItemURL`, `condition.conditionDisplayName`.
5. Extract brand via `itemSpecifics.aspect[name=Brand]` if present; fallback to regex-match on title against `NormalizeBrand`'s canonical set.
6. Return `[]OnlineResult` — same shape as any other adapter (see M8.T5).

**Acceptance:**
- Given `EBAY_APP_ID` set + OEM `263202G000` → returns ≥ 3 results with non-empty `brand`
- Given no `EBAY_APP_ID` → returns `(nil, nil)` gracefully (feature-disabled)
- Test uses `httptest.NewServer` with a captured eBay JSON response fixture
- Timeout 5 s per request

**Effort:** M

---

### Sprint M8.T3 — `OnlineResult` common type + adapter interface

**Goal:** define the shared interface every source adapter implements.

**Files:**
- `internal/service/online_source.go` (new)

**Interface:**

```go
type OnlineSource interface {
    Name() string                                        // "ebay" / "hyundaipartsdeal" / etc.
    Enabled() bool                                       // reads env / config
    Search(ctx context.Context, oem string) ([]OnlineResult, error)
    RateLimit() time.Duration                            // min interval between requests
}

type OnlineResult struct {
    Source        string
    OEM           string
    Brand         string
    PartNumber    string
    Description   string
    PriceCents    int64
    Currency      string
    Condition     string
    ImageURL      string
    SourceURL     string
    RawPayload    json.RawMessage
    FetchedAt     time.Time
}
```

**Acceptance:**
- Interface compiles
- Trivial `NoOpSource` test implementation used by 3 tests

**Effort:** S

---

### Sprint M8.T4 — `robots.txt` compliance guard

**Goal:** for any G5/G6 source, ensure we obey `robots.txt` before fetching.

**Files:**
- `internal/service/robots_guard.go` (new)

**Approach:**

1. Use `github.com/temoto/robotstxt` (BSD-2 licensed, already-common Go lib).
2. `RobotsGuard{cache map[string]*robotstxt.RobotsData}` with 24 h TTL per hostname.
3. `Allowed(userAgent string, targetURL string) (bool, error)` — returns `false` if robots.txt disallows.
4. On the FIRST call per hostname per day, `HEAD /robots.txt`, parse, cache.
5. On failure to fetch robots.txt (5xx), FAIL CLOSED (return `false, err`) — do not access if we cannot verify.

**Acceptance:**
- Given `robots.txt: Disallow: /product/` → path `/product/foo` returns `false`
- Given no `robots.txt` (404) → returns `true` (per RFC, absence = allowed)
- Given fetch error → returns `false`
- 5 unit tests covering the state machine

**Effort:** S

---

### Sprint M8.T5 — Rate-limiter per source

**Goal:** enforce per-source rate limits.

**Files:**
- `internal/service/rate_limiter.go` (new)

**Approach:**

1. Token-bucket per `source` name, sized from `OnlineSource.RateLimit()`.
2. `Wait(ctx context.Context) error` — blocks up to `ctx` deadline; returns `nil` when a token is available.
3. Shared across all requests to the same source.

**Acceptance:**
- Given 1 req / 2s limiter → 3 rapid calls take ≥ 4 s cumulative
- Ctx cancel during wait returns immediately with `ctx.Err()`
- Unit tests use `time.NewTicker` with short intervals

**Effort:** S

---

### Sprint M8.T6 — Schema.org JSON-LD extractor

**Goal:** extract `Product` schema.org JSON-LD blocks from HTML pages.

**Files:**
- `internal/service/schema_org_parser.go` (new)

**Approach:**

1. Given HTML bytes, use `golang.org/x/net/html` to find every `<script type="application/ld+json">` block.
2. Parse each block; look for entries with `@type == "Product"` or `@type == "Offer"`.
3. Return normalized `[]OnlineResult` with fields mapped from schema.org properties:
   - `Product.name` → `Description`
   - `Product.brand.name` → `Brand`
   - `Product.mpn` → `PartNumber`
   - `Product.image` → `ImageURL`
   - `Offer.price` → `PriceCents`
   - `Offer.priceCurrency` → `Currency`

**Acceptance:**
- Given fixture HTML from HyundaiPartsDeal.com (checked into `testdata/`) → returns ≥ 1 Product
- Handles nested `@graph` structure
- Handles pages with zero JSON-LD → returns `[]` empty
- 5 unit tests covering shape variants

**Effort:** S

---

### Sprint M8.T7 — HyundaiPartsDeal.com adapter (G5a)

**Goal:** given a Hyundai OEM, fetch its public reference page and extract structured data.

**Files:**
- `internal/service/online_hyundaipartsdeal.go` (new)
- `internal/service/online_hyundaipartsdeal_test.go`

**Approach:**

1. Env-gated: `ONLINE_HYUNDAIPARTSDEAL_ENABLED=true` required (default false).
2. URL pattern: `https://www.hyundaipartsdeal.com/search?catchall=<OEM>`.
3. Use `RobotsGuard` first — abort if disallowed.
4. Use `RateLimiter` from M8.T5 — wait for token.
5. GET with User-Agent `IfritahPartsEngine/1.0 (+https://ifritah.com/bot-info)`.
6. Extract JSON-LD via M8.T6 parser.
7. Return `[]OnlineResult` with `Source="hyundaipartsdeal"` and `SourceURL` = the search result page.

**Acceptance:**
- Given `ONLINE_HYUNDAIPARTSDEAL_ENABLED=false` → returns `(nil, nil)`
- Given the flag on + real OEM → returns ≥ 1 result with non-empty brand
- HTTP timeout 8 s per request
- robots.txt disallow → returns `(nil, ErrRobotsDisallowed)` — logged as warning, not error
- Test uses `httptest.Server` with checked-in HTML fixture

**Effort:** M

---

### Sprint M8.T8 — KiaPartsNow.com adapter (G5b)

**Goal:** same as M8.T7 but for KiaPartsNow.com.

**Files:**
- `internal/service/online_kiapartsnow.go` (new)
- `internal/service/online_kiapartsnow_test.go`

**Approach:** identical to M8.T7 substituting the Kia hostname; may share code with M8.T7 via a `GenericJsonLdAdapter` base.

**Acceptance:** same shape as M8.T7 test suite.

**Effort:** M

---

### Sprint M8.T9 — 7zap.com adapter (G5c)

**Goal:** global OEM catalog adapter.

**Files:**
- `internal/service/online_7zap.go` (new)
- `internal/service/online_7zap_test.go`

**Approach:** 7zap uses a different HTML structure — may not have JSON-LD; fall back to structured HTML extraction. Otherwise identical shape.

**Acceptance:** same shape.

**Effort:** M

---

### Sprint M8.T10 — Federated meta-search dispatcher

**Goal:** given an OEM, fan out to all enabled sources in parallel; return aggregated + deduplicated results; persist to cache.

**Files:**
- `internal/service/online_search.go` (new)
- `internal/service/online_search_test.go`

**Approach:**

1. `OnlineSearchService{sources []OnlineSource, cache OnlineCacheRepo, cb *CircuitBreaker}` constructor.
2. `Search(ctx context.Context, oem string) ([]OnlineResult, error)`:
   - First: read from cache by `oem_normalized`. If ≥ 1 fresh (within TTL) row exists → return cached, no external calls.
   - Otherwise: `sync.WaitGroup` fan-out across all `sources` where `Enabled() == true` and circuit-open == false.
   - Per source: apply `RateLimit()` wait, run `Search(ctx, oem)`, collect, or fail-fast on ctx expiry.
   - Merge + `NormalizeBrand` + dedupe by `(brand, part_number)`.
   - Persist to `aftermarket_online_cache` via async goroutine (fire-and-forget).
   - Return merged list.
3. Ctx deadline default 10 s; individual source timeout 5 s.
4. Circuit breaker per source — 3 consecutive failures → 60 s cooldown.

**Acceptance:**
- Cache hit → 0 external calls, results returned in < 50 ms
- Cache miss → all enabled sources queried in parallel; slowest source is the tail latency
- One source failing → other sources' results still returned
- Ctx expiry → returns partial results
- Unit test with 3 mock sources verifying merge / dedupe / cache-write

**Effort:** M

---

### Sprint M8.T11 — UNION online results into `FindAftermarketForOEM_MultiPath`

**Goal:** online results appear alongside TecDoc results in existing aftermarket path.

**Files:**
- `internal/service/tecdoc.go` — extend `FindAftermarketForOEM_MultiPath`

**Approach:**

1. Existing multi-path UNION already merges TecDoc paths (`articlecrosses` + `oem_number` + `oem_search_index`).
2. Add a fourth path: `OnlineSearchService.Search(ctx, oem)`.
3. Wrap in `go func()` for parallelism; feed results into the same tier-sort pipeline.
4. Online results tagged `Source = "online:<source_id>"` in the result struct so downstream ranking can prefer / demote.
5. `BrandTier` unchanged; `SortAftermarketByTier` already prioritizes premium OEM brands (which online sources may return alongside cheap sellers).
6. `CapAftermarketList` caps at 20/3-per-brand — no change; online results just contribute to the pool.

**Acceptance:**
- For a test OEM with cache pre-populated → results include online rows
- For an OEM with online source down → results = TecDoc-only (no failure)
- Test asserts online rows appear in output with `Source="online:ebay"` etc.

**Effort:** M

---

### Sprint M8.T12 — Frontend provenance badge + attribution

**Goal:** results sourced from online sources render with a visible "Sourced from X" badge and click-through URL.

**Files:**
- `frontend/src/components/PartResult.tsx`

**Approach:**

1. If `result.source` starts with `online:` → render a small badge:
   ```
   [ Sourced from eBay ]  ↗
   ```
2. Badge clickable → `window.open(result.sourceUrl, "_blank")`.
3. `rel="nofollow sponsored"` on the anchor (SEO neutrality).

**Acceptance:**
- Visual: badge appears on the correct rows in the Playwright snapshot test
- Click event opens new tab
- No badge on non-online results (regression check)

**Effort:** S

---

## Compliance / kill switch

Every adapter must respect:

- **Global kill switch:** env `ONLINE_SEARCH_ENABLED=false` → dispatcher returns `[]` without fanning out
- **Per-source kill switch:** env `ONLINE_<SOURCE>_ENABLED=false` → that adapter returns `(nil, nil)`
- **robots.txt:** compulsory for G5 adapters
- **User-Agent:** `IfritahPartsEngine/1.0 (+https://ifritah.com/bot-info)` — must be honest
- **Rate limits:** enforced by shared `RateLimiter` — no adapter bypasses
- **Attribution:** every surfaced result carries `SourceURL` back-link

## Exit gate for M8

Run the min-report (PR #22) after M8 lands. Additionally:

| Metric | Target |
|---|---|
| `AvgAM_online` on 19-OEM corpus | ≥ 3 aftermarket brands per corpus OEM (average) |
| Cache hit rate on repeat queries | ≥ 70% within 30 days |
| p95 latency impact on `FindAftermarketForOEM_MultiPath` | < 500 ms (cache hits are < 50 ms; misses parallelize) |
| Zero ToS violations reported | 0 |
| eBay API daily call ceiling not breached | < 5,000 calls/day at steady state |

## Risks

| Risk | Mitigation |
|---|---|
| G5 source changes HTML structure | Feature-flag per adapter; graceful degrade to eBay-only |
| eBay rate limit hit on high-traffic day | Cache 30d default; consider Enterprise tier if breached |
| G5 site sends cease-and-desist | Kill switch flips within 5 minutes; delete cached rows for that source; log incident |
| False-brand attribution from eBay seller titles | Existing `NormalizeBrand` + confidence threshold; if brand unresolvable, drop the result |
| Cache table grows unbounded | Nightly cron `DELETE FROM aftermarket_online_cache WHERE fetched_at < NOW() - INTERVAL '90 days'` |

---

## Extended source list (Phase B onwards — 25+ additional sources)

See `docs/data-sources/online-sources-catalog.md` for the full 30-source catalog. Summary of sprint IDs for the additional adapters:

### Phase B — highest-ROI adapters (5 sprints)

| # | Sprint | Source | Rationale |
|---|---|---|---|
| M8.T13 | AliExpress Affiliate API client | Chinese aftermarket | HUGE inventory, HK-relevant, free tier |
| M8.T7 | HyundaiPartsDeal.com adapter | Hyundai dealer OEM | Already scoped above; now Phase-B priority |
| M8.T8 | KiaPartsNow.com adapter | Kia dealer OEM | Sister site; Phase-B priority |
| M8.T18 | PartsGeek.com adapter | Multi-brand aftermarket retailer | Deep BOSCH / MANN / MAHLE listings for HK |
| M8.T32 | Emex.ae adapter | UAE / GCC marketplace | Regional inventory + AED prices |

### Phase C — coverage expansion (10 sprints)

| # | Sprint | Source |
|---|---|---|
| M8.T13-alt | eBay Buy Browse API (OAuth successor) | Modernize eBay integration |
| M8.T14 | Amazon PA-API 5.0 client | Amazon Motors category |
| M8.T9 | 7zap.com adapter | Global OEM + MENA |
| M8.T19 | CARiD.com adapter | Specialty performance + body |
| M8.T20 | AutoZone.com adapter | US retailer, Duralast + national brands |
| M8.T21 | AdvanceAutoParts.com adapter | US retailer |
| M8.T22 | NAPAOnline.com adapter | NAPA house brands |
| M8.T23 | 1AAuto.com adapter | Body / suspension DTC |
| M8.T24 | BuyAutoParts.com adapter | Specialty aftermarket |
| M8.T33 | Autopedia + regional-dealer aggregator | GCC regional coverage |

### Phase D — brand-direct + reference (10 sprints)

| # | Sprint | Source |
|---|---|---|
| M8.T26 | BOSCH Automotive Aftermarket adapter | First-party BOSCH cross-refs |
| M8.T27 | MANN+HUMMEL catalog adapter | First-party MANN-FILTER |
| M8.T28 | MAHLE Aftermarket adapter | MAHLE + KNECHT |
| M8.T29 | DENSO catalog adapter | DENSO electrical |
| M8.T30 | NGK sparkplug fitment adapter | Spark plugs |
| M8.T31 | HELLA catalog adapter | Lighting + sensors |
| M8.T36 | oilfilter-crossreference.com adapter | Filter cross-refs |
| M8.T37 | Generic category cross-reference adapter | Bearings / spark plugs / brakes / etc. |
| M8.T35 | Wikidata SPARQL query-expansion aid | Metadata for brand normalization |
| M8.T25 | Dealer-network aggregator (Suncoast + 50 US Hyundai/Kia dealers) | Deep dealer OEM data |

### Phase E — deferred (evaluated post-Phase-D)

| # | Sprint | Source |
|---|---|---|
| M8.T15 | Walmart Open API adapter | US retail |
| M8.T16 | Rakuten Advertising API adapter | Affiliate aggregator |
| M8.T17 | Alibaba B2B API adapter | Wholesale Chinese aftermarket |
| — | Additional country-specific regional adapters (KSA / Kuwait / Oman) | Regional |

### Ratings ceiling per phase

| Sources live | Dim 3 est. | Dim 4 est. | Overall est. |
|---|:-:|:-:|:-:|
| Just eBay (current) | 4 | 6 | 6.6 |
| + Phase B (5 more, 6 total) | 6 | 7 | 7.3 |
| + Phase C (10 more, 16 total) | 7 | 8 | 7.8 |
| + Phase D (10 more, 26 total) | 8 | 8 | 8.2 |
| Full stack (all 30+ live) | 8 | 8 | 8.2 |

Diminishing returns kick in around 15-20 sources — subsequent adapters add convergence signal (trust) rather than new coverage.

### Trust scoring in the merged dispatcher

Every source declares its trust tier when constructed:

```go
type OnlineSource interface {
    Name() string
    Enabled() bool
    RateLimit() time.Duration
    TrustScore() float64                                          // 0.6 - 1.0
    Search(ctx context.Context, oem string) ([]AftermarketPart, error)
}
```

Trust score guidelines (from `docs/data-sources/online-sources-catalog.md`):

- **1.00** — brand-direct catalog (BOSCH / MANN / MAHLE / DENSO / NGK / HELLA)
- **0.90** — official APIs (eBay / AliExpress / Amazon / Walmart / Rakuten)
- **0.85** — dealer G5 (HyundaiPartsDeal / KiaPartsNow / Suncoast dealers)
- **0.75** — aftermarket-retailer G5 (PartsGeek / CARiD / AutoZone / NAPA / 1AAuto)
- **0.70** — regional (Emex / Autopedia)
- **0.65** — marketplace with seller-provided data (eBay / AliExpress noisy tier)
- **0.60** — category cross-reference sites

Convergence bonus: results found by ≥2 sources multiply confidence by 1.05, capped at 1.0. Final rank = `trust × convergence × BrandTier`.

### Reusable adapter code shape

Adding a new source is one file (~150-300 lines):

```go
package service

type NewAdapter struct {
    client  *http.Client
    robots  *RobotsGuard // Tier 2 + 3 only — official APIs don't need it
    baseURL string
}

func NewNewAdapter(client *http.Client, robots *RobotsGuard) *NewAdapter { ... }

func (a *NewAdapter) Name() string             { return "online:<source>" }
func (a *NewAdapter) Enabled() bool            { return envOn("ONLINE_<SOURCE>_ENABLED") && a.hasCreds() }
func (a *NewAdapter) RateLimit() time.Duration { return perSourceInterval }
func (a *NewAdapter) TrustScore() float64      { return trustTierN }
func (a *NewAdapter) Search(ctx context.Context, oem string) ([]model.AftermarketPart, error) {
    if !a.Enabled() { return nil, nil }
    if a.robots != nil {
        allowed, _ := a.robots.Allowed(ctx, robotsClientAgent, a.buildURL(oem))
        if !allowed { return nil, nil }
    }
    resp, err := a.doHTTP(ctx, oem)
    if err != nil { return nil, err }
    return a.parseResponse(resp, oem)
}
```

The dispatcher (`internal/service/online_search.go`) already fans out to `[]OnlineSource`; adding a new adapter is just appending to that slice at construction in `cmd/api/main.go`.

### Feature-flag matrix (env vars)

Each source gets its own kill switch. Recommended defaults for a fresh deploy:

| Variable | qa default | prod default |
|---|:-:|:-:|
| `ONLINE_SEARCH_ENABLED` | `true` | `false` (opt-in per-source) |
| `ONLINE_EBAY_ENABLED` | `true` | `true` after 1 week of qa data |
| `ONLINE_ALIEXPRESS_ENABLED` | `true` | `true` after quota verification |
| `ONLINE_HYUNDAIPARTSDEAL_ENABLED` | `true` | `true` after ToS review |
| `ONLINE_KIAPARTSNOW_ENABLED` | `true` | `true` after ToS review |
| `ONLINE_PARTSGEEK_ENABLED` | `true` | `true` after ToS review |
| `ONLINE_EMEX_ENABLED` | `true` | `true` |
| … all other adapters | `false` | `false` (enable one at a time) |

This lets you ramp up sources progressively while measuring the per-source contribution to `AvgAM_correct` in the nightly audit.
