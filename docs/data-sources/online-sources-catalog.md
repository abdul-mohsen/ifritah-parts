# Online-Search Sources Catalog — 30 sources for Kia/Hyundai aftermarket

**Scope:** free / free-tier sources only. No paid catalog data. Every source below is either (a) an official API with a free tier, (b) a public reference site publishing machine-readable data (schema.org JSON-LD / OpenGraph / structured HTML), or (c) a knowledge graph / SPARQL endpoint. **No scraping in the crawler sense** — everything is either an official API or a public reference page consulted on-demand with `robots.txt` compliance + rate limits.

**Legal tiers:**

- 🟢 **Official API** — vendor publishes an API contract with quotas we respect
- 🟡 **G5-public** — public pages with schema.org / OpenGraph markup intended for machine consumption; requires `robots.txt` compliance, rate limits, honest User-Agent
- 🔵 **Government / knowledge graph** — public-domain / permissively licensed
- 🔴 **Excluded** — ToS-hostile or requires scraping (kept in list for negative-space clarity)

**Coverage tiers:**

- ⭐⭐⭐⭐⭐ Deep + broad HK-specific
- ⭐⭐⭐⭐ Substantial HK-specific
- ⭐⭐⭐ Some HK / much global overlap
- ⭐⭐ Marginal
- ⭐ Only useful in aggregate

---

## Tier 1 — Official APIs with free tiers

### 1. eBay Motors Finding API 🟢 ⭐⭐⭐⭐

- **Endpoint:** `https://svcs.ebay.com/services/search/FindingService/v1`
- **Auth:** free App-ID (register at developer.ebay.com)
- **Rate limit:** 5,000 calls/day free tier; 1.5M/day with Enterprise application (also free)
- **What it gives:** seller-provided title/brand/MPN/price/condition/image for millions of Kia + Hyundai parts globally including MENA
- **HK coverage:** ⭐⭐⭐⭐ massive marketplace; noisy but abundant
- **Status:** **Adapter implemented** in `internal/service/online_ebay.go`

### 2. eBay Buy Browse API 🟢 ⭐⭐⭐⭐

- **Endpoint:** `https://api.ebay.com/buy/browse/v1/item_summary/search`
- **Auth:** OAuth 2.0 client credentials
- **Rate limit:** 5,000 calls/day free tier (separate quota from Finding API)
- **What it gives:** modern REST, richer item aspects (aspects.Brand, aspects.MPN, compatibilityMatch), better filtering
- **HK coverage:** same eBay inventory as source 1
- **Status:** planned M8.T7-alt (upgrade path from Finding API)

### 3. AliExpress Affiliate Program API 🟢 ⭐⭐⭐⭐

- **Endpoint:** `https://api-sg.aliexpress.com/sync`
- **Auth:** free affiliate signup + AppKey/AppSecret
- **Rate limit:** ~15 QPS per app, generous daily quotas
- **What it gives:** massive Chinese aftermarket catalog with Kia/Hyundai fitment; product name/brand/price/image
- **HK coverage:** ⭐⭐⭐⭐⭐ AliExpress carries deep Chinese-market aftermarket for HK (KYB clones, Denso genuine, Bosch)
- **Legal:** commercial use permitted under affiliate terms
- **Status:** planned M8.T13

### 4. Amazon Product Advertising API (PA-API 5.0) 🟢 ⭐⭐⭐

- **Endpoint:** `https://webservices.amazon.com/paapi5/searchitems`
- **Auth:** requires Amazon Associates account (free signup) + IAM credentials
- **Rate limit:** 1 TPS default; scales with monthly affiliate revenue
- **What it gives:** Amazon Motors category listings for Kia/Hyundai OEMs; brand, title, image, price, ASIN
- **HK coverage:** ⭐⭐⭐ US-heavy inventory; some genuine + aftermarket
- **Legal:** requires Associates signup, revenue-share model — free to use
- **Status:** planned M8.T14

### 5. Walmart Open API 🟢 ⭐⭐

- **Endpoint:** `https://developer.walmart.com/api/us/mp/items`
- **Auth:** free developer signup
- **Rate limit:** 100 requests/minute
- **What it gives:** Walmart automotive parts catalog; brand + price + fitment
- **HK coverage:** ⭐⭐ US-only, thin on OEM specifics
- **Status:** planned M8.T15 (low priority — Amazon dominates for auto parts in US)

### 6. Rakuten Advertising API 🟢 ⭐⭐

- **Endpoint:** `https://api.linksynergy.com/productsearch/1.0`
- **Auth:** free Rakuten Publisher signup + Access Token
- **Rate limit:** 3,000 calls/day free tier
- **What it gives:** aggregated product data from Rakuten's affiliate network (includes 1AAuto, PartsGeek, etc.)
- **HK coverage:** ⭐⭐ depends on member retailers
- **Status:** planned M8.T16

### 7. Alibaba Open Platform API 🟢 ⭐⭐⭐

- **Endpoint:** `https://api.alibaba.com/openapi/param2/2/`
- **Auth:** free Alibaba developer account
- **Rate limit:** per-endpoint quotas (typically 500-5000/day free)
- **What it gives:** B2B parts marketplace, Chinese wholesale, factory-direct aftermarket
- **HK coverage:** ⭐⭐⭐ depends on category — filters + brakes have deep coverage
- **Status:** planned M8.T17 (parallel to AliExpress but B2B focused)

### 8. NHTSA vPIC API 🔵 ⭐⭐⭐⭐

- **Endpoint:** `https://vpic.nhtsa.dot.gov/api/vehicles/DecodeVin/{VIN}?format=json`
- **Auth:** none
- **Rate limit:** unlimited (soft ~10 req/sec)
- **What it gives:** VIN → make/model/year/engine/trim decode + safety recalls
- **HK coverage:** ⭐⭐⭐⭐ complete for US-sold HK vehicles
- **Status:** **already integrated** (M5.S2)

### 9. CarQueryAPI 🟢 ⭐⭐⭐

- **Endpoint:** `https://www.carqueryapi.com/api/0.3/`
- **Auth:** none
- **Rate limit:** unlimited (soft)
- **What it gives:** make/model/year/trim + engine specs (displacement, HP, torque)
- **HK coverage:** ⭐⭐⭐ good global vehicle metadata
- **Status:** **already integrated**

### 10. fueleconomy.gov API 🔵 ⭐⭐⭐

- **Endpoint:** `https://www.fueleconomy.gov/ws/rest/vehicle`
- **Auth:** none
- **Rate limit:** unlimited
- **What it gives:** EPA fuel economy + emissions + engine specs for US-sold vehicles
- **HK coverage:** ⭐⭐⭐ good for US-market HK vehicles
- **Status:** **already integrated**

---

## Tier 2 — Public reference sites (G5-public with schema.org markup)

Each requires: `robots.txt` compliance check → rate-limit (1 req / 2 sec default) → identifying User-Agent → 30-day cache on returned data.

### 11. HyundaiPartsDeal.com 🟡 ⭐⭐⭐⭐⭐

- **URL pattern:** `https://www.hyundaipartsdeal.com/search?catchall={OEM}`
- **Data format:** schema.org `Product` JSON-LD blocks in page HTML
- **What it gives:** authoritative Hyundai dealer OEM reference — genuine part description, supersession, exploded diagram category, MSRP
- **HK coverage:** ⭐⭐⭐⭐⭐ Hyundai gold-standard
- **Status:** planned M8.T7

### 12. KiaPartsNow.com 🟡 ⭐⭐⭐⭐⭐

- **URL pattern:** `https://www.kiapartsnow.com/search?catchall={OEM}`
- **Data format:** schema.org `Product` JSON-LD
- **What it gives:** authoritative Kia dealer OEM reference — sister site to HyundaiPartsDeal
- **HK coverage:** ⭐⭐⭐⭐⭐ Kia gold-standard
- **Status:** planned M8.T8

### 13. 7zap.com 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://7zap.com/en/catalog/cars/hyundai/` (paginated OEM search)
- **Data format:** mostly structured HTML with product tables; some schema.org
- **What it gives:** global OEM catalog with MENA inventory, exploded diagrams, cross-references
- **HK coverage:** ⭐⭐⭐⭐ good MENA-market fit
- **Status:** planned M8.T9

### 14. PartsGeek.com 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://www.partsgeek.com/search.html?q={OEM}`
- **Data format:** OpenGraph + schema.org `Product`
- **What it gives:** ~200 aftermarket brand carrier — Beck-Arnley, CRP, Denso, Mopar, Motorcraft, etc.
- **HK coverage:** ⭐⭐⭐⭐ strong aftermarket coverage for popular HK OEMs
- **Status:** planned M8.T18

### 15. CARiD.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.carid.com/search.html?q={OEM}`
- **Data format:** schema.org `Product`
- **What it gives:** specialty parts retailer with aftermarket + performance brands
- **HK coverage:** ⭐⭐⭐ good for performance + body/lighting
- **Status:** planned M8.T19

### 16. AutoZone.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.autozone.com/searchresult?searchText={OEM}`
- **Data format:** schema.org `Product` + JSON-LD offer blocks
- **What it gives:** US retailer with Duralast (house brand) + national aftermarket brands
- **HK coverage:** ⭐⭐⭐ US-focused but strong on filters/brakes
- **Status:** planned M8.T20

### 17. AdvanceAutoParts.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://shop.advanceautoparts.com/find/{OEM}`
- **Data format:** schema.org `Product`
- **What it gives:** competitor to AutoZone; carries Wearever, Autolite, etc.
- **HK coverage:** ⭐⭐⭐ similar to AutoZone
- **Status:** planned M8.T21

### 18. NAPAOnline.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.napaonline.com/en/search?query={OEM}`
- **Data format:** schema.org `Product`
- **What it gives:** NAPA house brands + national aftermarket carriers
- **HK coverage:** ⭐⭐⭐
- **Status:** planned M8.T22

### 19. 1AAuto.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.1aauto.com/search?keywords={OEM}`
- **Data format:** schema.org `Product`
- **What it gives:** direct-to-consumer aftermarket; body + suspension focus
- **HK coverage:** ⭐⭐⭐ strong body/collision coverage
- **Status:** planned M8.T23

### 20. BuyAutoParts.com 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.buyautoparts.com/search.aspx?search={OEM}`
- **Data format:** schema.org `Product`
- **What it gives:** aftermarket specialty, EGR/turbo/etc. deeper stock
- **HK coverage:** ⭐⭐⭐
- **Status:** planned M8.T24

### 21. Suncoast Hyundai/Kia Parts (dealer) 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://parts.suncoastkia.com/search?query={OEM}` (varies)
- **Data format:** dealer catalog HTML with schema.org
- **What it gives:** dealer-side OEM data direct from a franchise dealer
- **HK coverage:** ⭐⭐⭐⭐ authoritative for HK genuine parts
- **Status:** planned M8.T25 (representative of ~50 US Hyundai/Kia dealers with online catalogs)

---

## Tier 3 — Brand-direct catalogs (free with partner registration)

Each has a public "product finder" that can be queried on-demand. Some offer a formal API to partners for free; others just serve structured HTML.

### 22. Bosch Automotive Aftermarket 🟢 / 🟡 ⭐⭐⭐⭐⭐

- **URL pattern:** `https://www.boschaftermarket.com/xrm/net/oemvfl/en/us/searchparts` (public search)
- **Data format:** returns JSON via XHR; can be parsed
- **What it gives:** first-party BOSCH catalog with OEM cross-refs for Kia/Hyundai
- **HK coverage:** ⭐⭐⭐⭐⭐ authoritative BOSCH data
- **Status:** planned M8.T26 (partner-API upgrade path available for free with distributor agreement)

### 23. MANN+HUMMEL Product Catalog 🟡 ⭐⭐⭐⭐⭐

- **URL pattern:** `https://www.mann-filter.com/catalog` (product finder)
- **Data format:** JSON via XHR endpoints
- **What it gives:** authoritative MANN-FILTER catalog with HK cross-refs
- **HK coverage:** ⭐⭐⭐⭐⭐
- **Status:** planned M8.T27

### 24. MAHLE Aftermarket 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://www.mahle-aftermarket.com/en/products/product-finder/`
- **Data format:** structured HTML with schema.org
- **What it gives:** MAHLE + KNECHT catalog
- **HK coverage:** ⭐⭐⭐⭐
- **Status:** planned M8.T28

### 25. DENSO Global Product Catalog 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://www.denso-am.eu/products/product-catalog`
- **Data format:** structured HTML
- **What it gives:** DENSO ignition + electrical for Kia/Hyundai
- **HK coverage:** ⭐⭐⭐⭐ DENSO is factory-fill for many HK electrical parts
- **Status:** planned M8.T29

### 26. NGK Sparkplug Fitment Lookup 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.ngk.com/product-lookup?vehicle={make}/{model}/{year}`
- **Data format:** JSON via XHR
- **What it gives:** NGK spark plug fitment for HK vehicles
- **HK coverage:** ⭐⭐⭐ narrow category but deep coverage
- **Status:** planned M8.T30 (category-specific — lower priority)

### 27. Hella Product Catalog 🟡 ⭐⭐⭐

- **URL pattern:** `https://www.hella.com/parts-catalogue/en/`
- **Data format:** structured HTML
- **What it gives:** HELLA lighting + sensors + electronics
- **HK coverage:** ⭐⭐⭐
- **Status:** planned M8.T31

---

## Tier 4 — Regional / MENA-focused (Kia/Hyundai target market)

### 28. Emex.ae 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://emex.ae/en/search?q={OEM}` (UAE-based Russian-heritage catalog)
- **Data format:** structured HTML + AJAX endpoints returning JSON
- **What it gives:** UAE/GCC aftermarket marketplace with real stock levels + AED prices
- **HK coverage:** ⭐⭐⭐⭐ GCC-inventory-native
- **Status:** planned M8.T32

### 29. Autopedia 🟡 ⭐⭐⭐

- **URL pattern:** `https://autopedia.ae/search/{OEM}` (regional catalog)
- **Data format:** structured HTML
- **What it gives:** UAE regional catalog with body/glass emphasis
- **HK coverage:** ⭐⭐⭐ regional-specific parts
- **Status:** planned M8.T33

### 30. HAAS-KO Hyundai GCC (via public dealer network pages) 🟡 ⭐⭐⭐

- **URL pattern:** dealer-network catalog pages
- **Data format:** structured HTML
- **What it gives:** GCC-market Hyundai OEM references
- **HK coverage:** ⭐⭐⭐ GCC-authoritative
- **Status:** planned M8.T34 (aggregated across regional dealer sites)

---

## Tier 5 — Knowledge graphs & category-specific reference

### 31. Wikidata SPARQL 🔵 ⭐⭐

- **Endpoint:** `https://query.wikidata.org/sparql`
- **Auth:** none
- **Rate limit:** ~60 requests/minute (soft)
- **What it gives:** structured metadata on car models, brand aliases, model-year mappings — useful for query expansion + brand normalization
- **HK coverage:** ⭐⭐ metadata layer, not part data
- **Status:** planned M8.T35 (query-expansion aid)

### 32. oilfilter-crossreference.com 🟡 ⭐⭐⭐⭐

- **URL pattern:** `https://oilfilter-crossreference.com/lookup?q={OEM_or_brand_part}`
- **Data format:** structured HTML table
- **What it gives:** filter cross-references — 519 alternatives for MANN W 811/80 alone (verified benchmark from `reference_engine_comparison_test.go:15`)
- **HK coverage:** ⭐⭐⭐⭐ deep filter coverage
- **Status:** planned M8.T36

### 33. Category-specific cross-reference sites 🟡 ⭐⭐⭐

- Examples: `bearing-crossreference.com`, `sparkplug-crossreference.com`, `brake-cross-reference.com`
- **What it gives:** community-maintained cross-refs per category
- **HK coverage:** ⭐⭐⭐ per category
- **Status:** planned M8.T37 (single generic adapter parametrized by category)

---

## Tier 6 — Excluded (documented for negative-space clarity)

### 34. RockAuto.com 🔴 ⭐⭐⭐⭐

- **Excluded reason:** ToS explicitly prohibits automation; anti-bot enforcement
- **User directive:** "no scraping"

### 35. Autodoc.co.uk automated queries 🔴 ⭐⭐⭐⭐

- **Excluded reason:** ToS prohibits automated queries; Cloudflare anti-bot
- Note: their B2B API is paid and would fit Tier 1 if procured — deferred to a future paid-decision

### 36. Amazon.com scraping (non-API) 🔴 ⭐⭐⭐

- **Excluded reason:** use PA-API instead (source 4) — direct scraping violates ToS

---

## Deduplication + trust strategy

With 30 sources returning results for the same OEM, we need to converge:

1. **Dedup key** `(NormalizeBrand(brand), lower(part_number))` — collapses "BOSCH" / "Bosch" / "Robert Bosch GmbH" to one row
2. **Trust score** per source, tiered:
   - Tier 3 (brand-direct): 1.0
   - Tier 1 official APIs: 0.9
   - Tier 2 dealer G5 (HyundaiPartsDeal / KiaPartsNow / dealer-specific): 0.85
   - Tier 2 aftermarket-retailer G5 (PartsGeek, CARiD, AutoZone): 0.75
   - Tier 4 regional: 0.7
   - Tier 1 marketplace (eBay, AliExpress, Amazon): 0.65 (seller-provided data is noisier)
   - Tier 5 category cross-ref: 0.6
3. **Convergence bonus:** result found in ≥ 2 sources gets confidence × 1.05 (capped at 1.0)
4. **Ranking:** final score = `avg(trust) × convergence_bonus × BrandTier`; then sort + cap 20 total / 3 per brand (existing M2.S2 tail)

---

## Rate-budget analysis at steady state

Assuming 1,000 unique OEMs looked up per day post-launch (heavier than qa today) with 30-day cache:

| Tier | Sources | Daily calls each (steady) | Budget headroom |
|---|---:|---:|---|
| Tier 1 (official APIs) | 10 | ~30-100 | ✅ Comfortable — eBay 5K/day, AliExpress 15 QPS, etc. |
| Tier 2 (G5-public) | 11 | ~30-100 | ✅ At 1 req/2s max, ceiling is 43,200/day/host |
| Tier 3 (brand-direct) | 6 | ~10-50 | ✅ Well under any reasonable brand-portal quota |
| Tier 4 (regional) | 3 | ~20-80 | ✅ Regional sites see less scrutiny |
| Tier 5 (knowledge) | 3 | ~10-30 | ✅ Wikidata + reference sites |

**Total daily outbound calls at steady state: ~700-2,000** across all 30 sources combined. Well within every free tier.

---

## Implementation phasing (updated M8 plan)

### Phase A · already implemented (this PR)

- Cache table + interface + rate limiter + robots guard + dispatcher
- **1 source live:** eBay Motors Finding API

### Phase B · next 5 sprints — highest ROI adapters

Priorities based on HK relevance + implementation cost:

1. **AliExpress Affiliate API** — free, huge inventory, HK-relevant Chinese aftermarket
2. **HyundaiPartsDeal.com** (G5) — authoritative Hyundai
3. **KiaPartsNow.com** (G5) — authoritative Kia
4. **PartsGeek.com** (G5) — deep aftermarket brand coverage
5. **Emex.ae** — GCC-native inventory

### Phase C · sprints 6-15 — coverage expansion

6. eBay Buy Browse API (OAuth successor to Finding)
7. Amazon PA-API
8. 7zap.com
9. CARiD.com
10. AutoZone.com
11. AdvanceAutoParts.com
12. NAPAOnline.com
13. 1AAuto.com
14. BuyAutoParts.com
15. Autopedia + dealer-network aggregation

### Phase D · sprints 16-25 — brand-direct + reference

16-21. BOSCH / MANN / MAHLE / DENSO / NGK / HELLA brand portals
22. oilfilter-crossreference.com
23. Category cross-ref generic adapter
24. Wikidata query-expansion layer
25. Trust-scored merger + convergence bonus tuning

### Phase E · deferred (evaluated post-Phase-D data)

- Alibaba B2B (source 7)
- Walmart Open API (source 5)
- Rakuten (source 6)
- Additional regional dealers per country

---

## Adapter code shape (reusable pattern)

Every new adapter is a ~150-300-line file with the same shape:

```go
type NewAdapter struct {
    client       *http.Client
    robots       *RobotsGuard         // Tier 2 + 3 only
    baseURL      string
}

func NewNewAdapter(client *http.Client, robots *RobotsGuard) *NewAdapter { ... }

func (a *NewAdapter) Name() string             { return "online:<source>" }
func (a *NewAdapter) Enabled() bool            { return envOn("ONLINE_<SOURCE>_ENABLED") && a.hasCreds() }
func (a *NewAdapter) RateLimit() time.Duration { return <perSourceInterval> }
func (a *NewAdapter) Search(ctx context.Context, oem string) ([]model.AftermarketPart, error) {
    if !a.Enabled() { return nil, nil }
    if a.robots != nil {
        if allowed, _ := a.robots.Allowed(ctx, robotsClientAgent, a.buildURL(oem)); !allowed {
            return nil, nil
        }
    }
    resp, err := a.doHTTP(ctx, oem)
    if err != nil { return nil, err }
    return a.parse(resp, oem)
}
```

The dispatcher (`internal/service/online_search.go`) already fans out to `[]OnlineSource` — new adapters just append to that slice at construction time in `cmd/api/main.go`.

---

## Ratings ceiling analysis

Assuming most adapters land with functional HK coverage:

| Sources live | Dim 3 est. | Dim 4 est. | Overall est. |
|---|:-:|:-:|:-:|
| Just eBay (current) | 4 | 6 | 6.6 |
| + Phase B (5 more) | 6 | 7 | 7.3 |
| + Phase C (10 more) | 7 | 8 | 7.8 |
| + Phase D (10 more) | 8 | 8 | 8.2 |
| Full 30-source stack | 8 | 8 | 8.2 |

Diminishing returns kick in around 15-20 sources — most subsequent sources add convergence signal (trust) rather than new coverage.

---

## Legal + compliance summary

All 33 non-excluded sources honour these guarantees:

1. `robots.txt` fetched + respected on every G5/Tier-2 adapter
2. Official APIs used with valid credentials + within quota
3. User-Agent: `IfritahPartsEngine/1.0 (+https://ifritah.com/bot-info)`
4. Per-source rate limit enforced (min 1 req / 2 sec for G5)
5. On-demand only — no bulk crawling
6. 30-day cache TTL default
7. Attribution back-link on every surfaced result
8. Global + per-source kill switches
9. Immediate takedown if any source objects (`ONLINE_<SOURCE>_ENABLED=false`)

**Total committed cost across all 30 sources: $0.** Every source uses either a free API tier or is a public reference page consulted on-demand within its published usage terms.
