# M4 Sprint Backlog — Beyond TecDoc

**Milestone exit gate.** TecDoc coverage plateaus around specific categories (body, glass, dealer accessories) with structural zeros. Adding independent data sources is the only path to full coverage.

- `AvgAM_correct` gains ≥ 3 from non-TecDoc sources on wear parts
- `F1_correct ≥ 0.90` on body / glass slices (currently 0.00-0.15)
- At least 2 alternative data sources integrated (RockAuto + one regional supplier)

---

## Sprint M4.S1 — RockAuto scraper

### Task M4.S1.T1 — Playwright/Chromium-driven scraper

**Goal.** Build a headless-browser scraper against `rockauto.com` that walks the parts catalog for HK vehicles and captures `(OEM, brand, partNumber, category, priceUsd, url)` per part.

**Files to touch:**
- New `scripts/scrapers/rockauto/main.go` — Go binary using `chromedp` or Playwright-Go
- New `scripts/scrapers/rockauto/README.md`

**Approach:**
1. Prototype: `chromedp` navigates to `/en/parts/hyundai/tucson/2018/2.0L L4 Gas DOHC`, waits for the parts tree JS to render, walks each expandable node, captures the OEM cross-reference table.
2. Persistence: emit newline-delimited JSON to stdout with fields `{oem, brand, partNumber, category, priceUsd, url, scrapedAt}`.
3. Rate limiting: 1 req / 2s to avoid detection.
4. Retry with exponential backoff on 429 / connection reset.

**Acceptance criteria:**
- [ ] Scraper produces valid NDJSON for 5 test vehicles: Elantra 2015 (2.0L), Tucson 2018 (2.0L Gas), Sonata 2020 (2.5L), Kia Rio 2016 (1.6L), Kia Sorento 2017 (3.3L).
- [ ] Each vehicle produces ≥ 200 parts (RockAuto typically shows 300-500 per vehicle).
- [ ] Handles the anti-bot page gracefully (retries with fresh session).

**Effort:** L

**Risk:** anti-bot detection. Budget 3 sprints max before switching to a paid catalog.

**Dependencies:** none

---

### Task M4.S1.T2 — Import pipeline + `articlecrosses`-style merge

**Goal.** Ingest scraper output into a new Postgres table and merge into `FindAftermarketForOEM`.

**Files to touch:**
- New `db/migrations/000020_aftermarket_rockauto.sql`
- `internal/service/tecdoc.go` — add a fourth path to `FindAftermarketForOEM_MultiPath`
- New `scripts/scrapers/rockauto/import.go`

**Approach:**
1. Migration:
   ```sql
   CREATE TABLE IF NOT EXISTS aftermarket_rockauto (
       id            BIGSERIAL PRIMARY KEY,
       oem_normalized TEXT NOT NULL,
       brand         TEXT NOT NULL,
       part_number   TEXT NOT NULL,
       category      TEXT,
       price_usd_cents INT,
       source_url    TEXT,
       scraped_at    TIMESTAMPTZ NOT NULL,
       created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       UNIQUE (oem_normalized, brand, part_number)
   );
   CREATE INDEX idx_aftermarket_rockauto_oem ON aftermarket_rockauto(oem_normalized);
   ```
2. Importer reads NDJSON stdin, `INSERT ... ON CONFLICT DO UPDATE SET scraped_at = EXCLUDED.scraped_at`.
3. `FindAftermarketForOEM` (from M2.S1.T1) adds a fourth goroutine that queries this table.

**Acceptance criteria:**
- [ ] Migration applies cleanly; index built.
- [ ] Importer handles 100k rows without OOM.
- [ ] Audit re-run against 5 test-vehicle OEMs: `AvgAM_correct` climbs by ≥ 3 for those specific OEMs.

**Effort:** M

**Dependencies:** M4.S1.T1, M2.S1.T1

---

### Task M4.S1.T3 — Continuous refresh cron

**Goal.** Weekly re-scrape of the top-500 most-queried OEMs. Invalidate `scraped_at > 30 days`.

**Files to touch:**
- New `cmd/rockauto_refresher/main.go`
- New `.github/workflows/rockauto-nightly.yml`

**Approach:**
1. Query Postgres for top-500 OEMs from `search_query_log` (assumed table — verify or create in prerequisite task).
2. For each, run the scraper via subprocess with a targeted URL.
3. Upsert into `aftermarket_rockauto`.
4. Report metrics: rows added / rows updated / rows failed.

**Acceptance criteria:**
- [ ] Binary builds and runs against staging.
- [ ] Dry-run mode outputs planned URLs without hitting the scraper.
- [ ] GitHub Actions workflow scheduled for weekly.

**Effort:** M

**Dependencies:** M4.S1.T2

---

## Sprint M4.S2 — Regional supplier catalog

### Task M4.S2.T1 — Survey report

**Goal.** Identify the 5 largest regional Hyundai/Kia parts distributors serving KSA / UAE / Oman. Machine-readable catalog? Data-feed access? Cost?

**Files to touch:**
- New `docs/data-sources/regional-catalog-survey.md`

**Approach:**
1. Candidate list:
   - Ali Al-Ghanim Auto (Kuwait/GCC)
   - Al-Futtaim Motors (UAE/wider)
   - Petromin (KSA)
   - Al-Ghazlain Auto Parts (KSA)
   - Aljazirah Vehicles (KSA)
2. For each: check if they publish an OEM-cross-ref catalog, whether there's a partner API, what the pricing model looks like, whether the data is fresh.

**Acceptance criteria:**
- [ ] Written report with a comparison matrix (supplier × API? × cost × freshness × coverage).
- [ ] At least 2 suppliers marked "feasible to integrate".

**Effort:** M

**Dependencies:** none

---

### Task M4.S2.T2 — Regional importer (per supplier)

**Goal.** For each feasible regional supplier, build a data importer.

**Files to touch:** (per supplier)
- New `scripts/scrapers/regional/{supplier}/main.go`
- New `db/migrations/000021_aftermarket_regional.sql`
- `internal/service/tecdoc.go` — fifth path in `FindAftermarketForOEM_MultiPath`

**Approach:**
1. Migration:
   ```sql
   CREATE TABLE aftermarket_regional (
       id             BIGSERIAL PRIMARY KEY,
       oem_normalized TEXT NOT NULL,
       supplier       TEXT NOT NULL,
       brand          TEXT,
       part_number    TEXT NOT NULL,
       stock_status   TEXT,
       region         TEXT,
       url            TEXT,
       created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       UNIQUE (oem_normalized, supplier, part_number)
   );
   CREATE INDEX idx_aftermarket_regional_oem ON aftermarket_regional(oem_normalized);
   ```
2. Per-supplier importer honours their data-feed shape (CSV / JSON / paginated HTML).

**Acceptance criteria:**
- [ ] At least 2 regional suppliers integrated.
- [ ] Audit re-run: `AvgAM_correct` gains ≥ 1 from regional sources on HK OEMs.

**Effort:** L (per supplier)

**Dependencies:** M4.S2.T1

---

## Sprint M4.S3 — Dealer parts catalog

### Task M4.S3.T1 — Survey report: Hyundai GSW + Kia World

**Goal.** Determine whether we can access Hyundai Global Service Way (GSW) or Kia World Online (KWO) parts catalog APIs. Dealer partnership requirement? Cost? Data shape?

**Files to touch:**
- New `docs/data-sources/dealer-catalog-survey.md`

**Acceptance criteria:**
- [ ] Written report with API endpoints, auth model, expected data shape, contract cost estimate.

**Effort:** M

**Dependencies:** business / legal follow-up outside code

---

### Task M4.S3.T2 — Live proxy (conditional on partnership)

**Goal.** If GSW/KWO become accessible, build a rate-limited live proxy that queries on-demand + caches for 24h.

**Files to touch:**
- New `internal/service/dealer_catalog.go`
- `internal/service/enrichment.go` — plug in as a sixth path

**Approach:**
1. HTTP client with 5 req/s rate limit (per assumed contract).
2. In-memory LRU + Postgres cache with 24h TTL.
3. Circuit breaker — if the dealer API returns errors > 10% for 5 min, disable for 15 min.

**Acceptance criteria:**
- [ ] Enabled when the `DEALER_CATALOG_ENABLED` config is set.
- [ ] Dealer-catalog data populates `OEMNumbers` for ≥ 90% of seeded HK OEMs when enabled.
- [ ] Circuit breaker verified via integration test.

**Effort:** L

**Dependencies:** M4.S3.T1 + partnership agreement

---

## Sprint M4.S4 — Community contribution system

### Task M4.S4.T1 — Contribution form + storage

**Goal.** Frontend form: "know an aftermarket alternative we missed?"

**Files to touch:**
- `frontend/src/components/AftermarketContribute.tsx` — form
- New `internal/handler/contribute.go` — POST endpoint
- New `db/migrations/000022_aftermarket_community.sql`

**Schema:**
```sql
CREATE TABLE aftermarket_community (
    id             BIGSERIAL PRIMARY KEY,
    oem_normalized TEXT NOT NULL,
    brand          TEXT NOT NULL,
    part_number    TEXT NOT NULL,
    source_url     TEXT,
    notes          TEXT,
    contributor    TEXT,
    status         TEXT NOT NULL DEFAULT 'pending', -- pending / approved / rejected
    submitted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at    TIMESTAMPTZ,
    reviewed_by    TEXT
);
CREATE INDEX idx_aftermarket_community_status ON aftermarket_community(status);
```

**Acceptance criteria:**
- [ ] Form submits to `/api/aftermarket/contribute`.
- [ ] Contribution stored with `status='pending'`.
- [ ] Rate limit: 10 contributions per IP per day.

**Effort:** L

**Dependencies:** none

---

### Task M4.S4.T2 — Admin moderation UI

**Files to touch:**
- `frontend/src/pages/admin/moderation.tsx`
- `internal/handler/admin_moderation.go`

**Approach:**
1. Admin list view showing pending contributions.
2. One-click approve / reject / edit-and-approve.
3. On approve, `FindAftermarketForOEM_MultiPath` queries this table via a seventh path.

**Acceptance criteria:**
- [ ] Admin can approve; the entry surfaces in the next search that touches its OEM.
- [ ] Admin can reject with a reason; rejected entries hidden from search.

**Effort:** L

**Dependencies:** M4.S4.T1

---

## Milestone M4 exit criteria

- [ ] RockAuto scraper live, populating for top-500 OEMs
- [ ] At least 2 regional suppliers integrated
- [ ] Community-contribution form live
- [ ] `AvgAM_correct` gains ≥ 3 from non-TecDoc sources on wear parts
- [ ] `F1_correct ≥ 0.90` on body / glass slices
- [ ] Audit re-run diff attached
