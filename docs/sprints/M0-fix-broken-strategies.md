# M0 Sprint Backlog — Fix all broken strategies (prerequisite for M1-M6)

Before we can improve `combined` mode we need every underlying strategy to actually work in isolation. The 2026-08-24 per-strategy probe against qa found that **5 of 13 registered strategies return 0 hits on known-good OEMs** even in ideal conditions. `combined` mode already benefits from the working ones; making the broken ones contribute unlocks additional recall + replacement richness without touching the merge logic.

## Per-strategy status (2026-08-24 probe against qa.ifritah.com)

Test OEM `26350-2J001` (Hyundai V6 oil filter — in seeded map, in oem_number, no articlecrosses aftermarket):

| Strategy | Hit? | Time | Status | Root cause |
|---|:-:|---:|---|---|
| **cache** | ✅ 1 | 0.4 s | Works | Postgres cache |
| **legacy** | ✅ 1 | 3.2 s | Works | Full cascade |
| **exact_oem** | ✅ 1 | 1.7 s | Works | oem_number direct hit |
| **prefix_inference** | ✅ 1 | 0.8 s | Works | Synthesized description |
| **cross_reference** | ❌ 0 | 0.7 s | Data-dependent | No articlecrosses row for this OEM (works on brake pads) |
| **cross_brand** | ❌ 0 | 0.7 s | Data-dependent | Same as cross_reference — data-sparse |
| **keyword_gated** | ❌ 0 | 0.8 s | Wrong-input | Needs keywords (`q=oil+filter`), not OEM |
| **owned_catalog** | ❌ 0 | 1.0 s | **BROKEN — empty table** | `hk_parts_cache` unpopulated on qa |
| **supersession** | ❌ 0 | 0.7 s | **BROKEN** | 0 hits everywhere; needs investigation |
| **spec_match** | ❌ 0 | 0.8 s | Needs param | Requires `seedArticleId` — audit corpus lacks it |
| **assembly_context** | ❌ 0 | 0.7 s | Needs param | Requires parent-component context |
| **vin_assembly** | ❌ 0 | 0.7 s | **BROKEN with VIN** | Passed `KMHDU4AD6DU100000`, still 0 |
| **vehicle_fitment** | ❌ 0 | 0.7 s | **BROKEN with linkageTargetId** | Passed `linkageTargetId=39843`, still 0 |
| **combined** | ✅ 1 | 2.1 s | Works (via cache + prefix + exact) | — |

For a brake-pad OEM (`58101-3XA00`), `cross_reference` and `cross_brand` DO hit (17 refs each) because that OEM has articlecrosses rows. **They're not broken — the DATA is sparse.**

But **owned_catalog, supersession, vin_assembly, vehicle_fitment return 0 hits on EVERY test** — they're structurally broken and contribute nothing to `combined`.

---

## Tasks — one PR per strategy

### Task M0.T1 — Diagnose + fix `owned_catalog` (empty table)

**Goal.** Get `owned_catalog` to return hits for OEMs known to exist in the source data.

**Files:**
- `internal/service/strategy.go` (OwnedCatalogStrategy)
- Investigate: `db/migrations/000011_*` (or wherever `hk_parts_cache` is created)
- Investigate: `internal/service/derive_worker.go` (populates the cache)

**Approach:**
1. **Diagnose.** Connect to qa Postgres. Run `SELECT COUNT(*) FROM hk_parts_cache;` — likely 0 or very small. If 0, the derive_worker never ran or ran and failed silently.
2. Check derive_worker logs on qa (`journalctl -u parts-engine` or app logs). Is it firing? Is it hitting a query error?
3. If the worker needs TecDoc MySQL configured but it isn't, that's a config drift issue — check the deploy env.
4. Manually trigger a derive run: `POST /api/admin/derive-parts` (if that endpoint exists) or run `scripts/derive_hk_maps/main.go` against qa.
5. Verify `hk_parts_cache` now populates. Re-run `owned_catalog` probe — should now hit.

**Acceptance criteria:**
- [ ] Written diagnosis in `docs/data-sources/owned-catalog-diagnosis.md`.
- [ ] `owned_catalog` returns ≥ 1 hit for `26350-2J001`, `58101-3XA00`, `82460-2T010`.
- [ ] Audit re-run: `owned_catalog` F1_correct climbs from 0 to ≥ 0.60.
- [ ] `hk_parts_cache` row count reported in the diagnosis (baseline + post-fix).

**Effort:** M (diagnosis-heavy)

**Dependencies:** access to qa Postgres

---

### Task M0.T2 — Fix `supersession` strategy

**Goal.** Return the supersession chain for OEMs known to have one.

**Files:**
- `internal/service/strategy.go` (SupersessionStrategy)
- `internal/service/tecdoc_supersession.go`
- `internal/service/tecdoc_supersession_test.go`

**Approach:**
1. **Diagnose.** Find a Hyundai OEM in the TecDoc data that has a supersession chain (`SELECT * FROM articleSuperseded WHERE ... LIMIT 5`). Probe against qa; verify 0 hits.
2. Trace `SupersessionStrategy.Search()` — where does it lose the chain? Likely one of:
   - Article-id promotion fails at entry (same 74% failure as PR #20 fixed for enrichment) — need to apply the same cross-refs fallback here.
   - Query joins are wrong.
   - `articleSuperseded` table is in the MySQL dump but the schema differs from expected.
3. Add proper article-id promotion at the strategy entry (mirror the enrichment fix from PR #20).
4. Fix the query if needed. Add table-driven test with 3 known chains.

**Acceptance criteria:**
- [ ] Regression test with 3 known-good OEM supersession chains — each returns the expected chain length.
- [ ] `supersession` F1_correct climbs from 0 to ≥ 0.40 on chain-relevant OEMs.
- [ ] Written diagnosis in `docs/data-sources/supersession-diagnosis.md`.

**Effort:** M

**Dependencies:** none

---

### Task M0.T3 — Fix `vin_assembly` strategy

**Goal.** Given a VIN, return parts cataloged against that vehicle's linkage target.

**Files:**
- `internal/service/strategy_assembly.go` (VinAssemblyStrategy)
- `internal/service/vin_decoder.go`

**Approach:**
1. **Diagnose.** Probe with a known-good HK VIN (`KMHDU4AD6DU100000` — Hyundai Elantra 2013). Result: 0 hits. Why?
2. Trace:
   - Does `VinAssemblyStrategy.Search()` even parse the input as a VIN or does it expect `q=OEM&vin=...` shape?
   - Does the VIN decoder return a valid `linkageTargetId`?
   - Does the downstream `linkagetargets → articlelinkages` query find rows?
3. Likely gaps:
   - Input is passed as `req.OEM` not `req.Query`; the strategy is looking for OEM shape and rejects VINs.
   - Or the decoder returns nil for HK WMIs.
4. Add strategy-level heuristic: if `req.Query` looks like a VIN (17 chars, alphanumeric, no dashes), invoke the decoder and use the resulting `linkageTargetId` to run the linkage lookup.

**Acceptance criteria:**
- [ ] Table-driven test with 5 known-good HK VINs — each returns ≥ 10 parts across ≥ 5 categories.
- [ ] `vin_assembly` F1_correct on a VIN corpus (new, TBD) ≥ 0.70.
- [ ] Handles invalid input (13-char string, non-alphanumeric) without panic.

**Effort:** M

**Dependencies:** vin_decoder may need widening — verify HK WMI coverage (`KMH*`, `KNA*`, `KND*`, `KNH*`)

---

### Task M0.T4 — Provide `vehicle_fitment` with valid linkage IDs (corpus, not code)

**Diagnosis (2026-08-24).** After tracing `VehicleFitmentStrategy.Search` → `searchByVehicle` → `TecDoc.PartsForVehicle`, the code path is intact. The query is `SELECT ... FROM articlesvehicletrees avt WHERE avt.linkingTargetId = ? AND avt.linkingTargetType = 'P'` — this works when the linkage ID exists. Our probe returned 0 because the arbitrary ID we passed (`39843`) has no rows in the qa MySQL. Additionally, `/api/catalog/vehicles?make=Hyundai&model=Elantra` returned `total=0` on qa, so the frontend has no way to obtain valid IDs either.

The fix is **corpus + catalog wiring**, not strategy code.

**Goal.** Populate the audit corpus with real linkage IDs so `vehicle_fitment` can be tested end-to-end, AND fix the catalog endpoint so users can obtain them.

**Files:**
- `scripts/audit/corpus-1500-v2.csv` — add `LinkageTargetIds` column
- New `scripts/audit/enrich_corpus_linkages.go` — batch tool that queries TecDoc MySQL
- `scripts/audit/audit-quality.ps1` — thread linkage IDs into `vehicle_fitment` mode requests
- Investigate `catalog/vehicles` endpoint (`internal/handler/catalog.go`) — why does it return 0?

**Approach:**
1. **Sub-task A.** Fix `/api/catalog/vehicles` — it returns 0 even for Elantra. Trace the query in `internal/handler/catalog.go:Vehicles`. Likely the join to `modelseries` / `linkagetargets` has a filter that's too strict (e.g. `lang='en'` on a table with no English rows, or a `bodyStyle` filter that eliminates most rows). Fix and add a test.
2. **Sub-task B.** Build the corpus enricher. For every OEM in `corpus-1500-v2.csv`:
   ```sql
   SELECT DISTINCT avt.linkingTargetId
   FROM oem_number on
   JOIN articles a ON a.legacyArticleId = on.articleId
   JOIN articlesvehicletrees avt ON avt.legacyArticleId = a.legacyArticleId
   WHERE on.clean_number = ? AND avt.linkingTargetType = 'P'
   LIMIT 5;
   ```
   Store the top 5 IDs comma-separated in a new `LinkageTargetIds` column.
3. **Sub-task C.** In `audit-quality.ps1`, when `mode == vehicle_fitment` and the corpus row has `LinkageTargetIds`, pick the first one and append `&linkageTargetId={id}` to the URL.

**Acceptance criteria:**
- [ ] Written diagnosis in `docs/data-sources/vehicle-fitment-audit-report.md`.
- [ ] `/api/catalog/vehicles?make=Hyundai&model=Elantra` returns ≥ 5 vehicles (currently 0).
- [ ] ≥ 60% of seeded-slice rows have ≥ 1 linkage ID after enrichment.
- [ ] Re-run `vehicle_fitment` mode against the enriched corpus; expect F1_correct ≥ 0.80.

**Effort:** M (corpus enrichment) + M (catalog fix)

**Dependencies:** access to TecDoc MySQL for the enrichment query

---

### Task M0.T5 — Test `keyword_gated` on keyword corpus

**Goal.** `keyword_gated` was designed for keyword queries (`q=oil filter&category=Oil Filter`), not OEM queries. Our audit tested it with OEMs and got 0 hits — that's the expected shape, not a bug. Give it a proper corpus.

**Files:**
- New `scripts/audit/corpus-keywords-v1.csv` — 200 keyword queries with expected top-category-tokens
- `scripts/audit/audit-quality.ps1` — accept an optional `-QueryColumn` param so `q` can be pulled from a different column when the corpus is keyword-based

**Approach:**
1. Build a 200-query keyword corpus. Rows:
   ```
   Query,ExpectedCategory,GoodTokens
   "oil filter","Oil Filter","oil,filter"
   "brake pad set front","Brake Pad Set - Front","brake,pad,front"
   "cabin air filter tucson","Cabin Air Filter","cabin,filter"
   ...
   ```
2. Extend audit script to run `-Modes keyword_gated -QueryColumn Query -InputCorpus corpus-keywords-v1.csv`.
3. Re-run analyzer; expect `keyword_gated` F1_correct ≥ 0.85 (it should be very good at keyword→category matching, that's its whole purpose).

**Acceptance criteria:**
- [ ] `corpus-keywords-v1.csv` ships in `scripts/audit/` with 200 rows.
- [ ] Audit script's `-QueryColumn` param works.
- [ ] `keyword_gated` F1_correct on keyword corpus ≥ 0.85.

**Effort:** M

**Dependencies:** none

---

### Task M0.T6 — Enrich corpus with linkage-target IDs

**Goal.** For every OEM in `corpus-1500-v2.csv`, look up its `linkageTargetId(s)` so tests for `vehicle_fitment` and `spec_match` can run end-to-end via corpus rows.

**Files:**
- `scripts/audit/corpus-1500-v2.csv` — add columns `LinkageTargetIds` (comma-separated), `SeedArticleIds` (comma-separated)
- New `scripts/audit/enrich_corpus_linkages.go` — batch tool that queries TecDoc MySQL for each OEM's linkages
- `scripts/audit/audit-quality.ps1` — pass linkage IDs as query params when running `vehicle_fitment` mode

**Approach:**
1. For each OEM: `SELECT DISTINCT al.linkageTargetId FROM articlelinkages al JOIN articles a ON a.legacyArticleId = al.legacyArticleId JOIN oem_number on ON on.articleId = a.legacyArticleId WHERE on.clean_number = ?`
2. Write the top 5 IDs into a new column. Empty when no data.
3. `audit-quality.ps1` uses that column to compose the URL when mode is `vehicle_fitment` or `spec_match`.

**Acceptance criteria:**
- [ ] ≥ 60% of seeded-slice rows have ≥ 1 linkage ID after enrichment.
- [ ] Re-run audit with the enriched corpus; `vehicle_fitment` gets a proper baseline number.

**Effort:** L (data + tool + audit-script threading)

**Dependencies:** M0.T4

---

### Task M0.T7 — Per-strategy F1 tracking in CI

**Goal.** Extend the nightly CI audit (M6.S1.T1) to attach a per-strategy CSV as an artifact so we can see which strategies regress independently.

**Files:**
- `.github/workflows/nightly-audit.yml` (once M6.S1.T1 lands) — publish the `by-strategy-*.csv`
- `docs/reports/nightly/README.md` — document how to read the per-strategy artifact

**Acceptance criteria:**
- [ ] Nightly job publishes `by-strategy-{date}.csv` alongside the existing by-category / by-slice outputs.
- [ ] The PR gate (M6.S1.T2) reads the strategy CSV and comments on regressions in individual strategies.

**Effort:** S

**Dependencies:** M6.S1 (parent nightly workflow)

---

## Milestone M0 exit criteria

Run the multi-strategy audit (`-Modes cache,legacy,exact_oem,prefix_inference,cross_reference,cross_brand,supersession,owned_catalog,vehicle_fitment,vin_assembly,keyword_gated,combined`) and confirm:

- [ ] `owned_catalog` F1_correct ≥ 0.60 on seeded slice (from 0.00)
- [ ] `supersession` F1_correct ≥ 0.40 on OEMs with known chains (from 0.00)
- [ ] `vin_assembly` F1_correct ≥ 0.70 on VIN corpus (from 0.00)
- [ ] `vehicle_fitment` F1_correct ≥ 0.80 on linkage corpus (from 0.00)
- [ ] `keyword_gated` F1_correct ≥ 0.85 on keyword corpus (untested until now)
- [ ] Per-strategy CSV attached to the milestone-close PR
- [ ] `combined` mode gains ≥ +0.05 F1_correct from the newly-working strategies

## Priority ordering for M0

Fixing broken strategies has multiplier effects on `combined` mode. Ship in order:

1. **M0.T4** (`vehicle_fitment` — probably a 1-line wiring fix, ~1 day) — unlocks vehicle-scoped searches
2. **M0.T2** (`supersession` — likely article-id promotion, ~2 days) — unlocks the chain that M2.S3 depends on
3. **M0.T1** (`owned_catalog` — likely ops / config, ~2 days) — unlocks fast-path for HK OEMs
4. **M0.T3** (`vin_assembly` — new logic, ~3 days) — unlocks VIN queries
5. **M0.T5** + **M0.T6** (corpus work, ~5 days combined) — unlocks proper audit coverage
6. **M0.T7** (CI wiring, depends on M6.S1) — unlocks continuous per-strategy tracking

Total M0 sprint: ~13 days (~2.5 weeks).

## Strategy value matrix (why fixing each one matters)

| Strategy | Cost to fix | Combined F1 lift when fixed | Standalone value |
|---|---|---:|---|
| owned_catalog | Ops config | +0.05 | Fast-path HK OEMs |
| supersession | Article-id promotion | +0.05 | Enables M2.S3 chain walker |
| vin_assembly | New wiring | +0.02 | Enables VIN queries (M5.S2) |
| vehicle_fitment | Field wiring | +0.03 | Enables vehicle-scoped searches |
| keyword_gated | Corpus only | +0.00 (not in combined) | Enables text-search UI |
| spec_match | Corpus + wiring | +0.00 (not in combined) | Enables spec-based sub-parts |
| assembly_context | Corpus + wiring | +0.00 (not in combined) | Enables parent-component lookup |
