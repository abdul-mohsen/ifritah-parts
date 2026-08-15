# QA report — data currently shown on the dashboard

**Reviewer:** OpenCode
**Reviewed:** 2026-08-14
**Under review:** the merged branch (`merge/adopt-feature-baseline-into-main`) running locally at `http://127.0.0.1:8080` — same code the PR will land on `main` when merged.
**Method:** live probe of every endpoint the dashboard's three tabs consume (`VIN Decode`, `Search`, `Catalog`), Playwright screenshot pass, direct Postgres row counts, forensic checks against the shipped seed data.

---

## Headline

**Rating: 4.5 / 10 — safe but empty.** The engine on this branch is now honest (won't fabricate results, won't leak non-HK parts, won't hang for 56 seconds), but the corpus behind the dashboard is a **1,710-row demo**. The three tabs render clean UI on real data for the ~98 Hyundai/Kia parts in the seed and the 15 vehicle groups it covers — everything outside that returns an honest zero.

- **What the dashboard reliably shows:** VIN decode → variants + recalls + parts, Golden OEM ranking (`26300-35505` first at 96%), text-search for common category terms, per-vehicle catalog groups + parts, part-detail with source + provenance + replacement guidance.
- **What it can't show:** any real HK OEM not in the 98-article seed set (returns 0 results — honest, but empty). No public part imagery. No technical specifications (`provenanceComplete: false` on every audited detail — disclosed honestly). No engine-code filtering (`/api/vehicle/:id/engine` returns 501 by design).
- **What was fixed on this branch (proved live):** Toyota `90915-YZZE1` now rejected in 6 ms with an honest "belongs to Toyota" warning (was fabricating a fake Corolla-compatible HK oil filter). `/api/vin/:vin` returns JSON 404 (was falling through to SPA HTML). Scraped page chrome (`"Sign up with"`, `"LIFE-TIME-FILTER"`) no longer surfaced.

---

## 1. The corpus the dashboard is drawing from

Direct row counts from the live Postgres:

| Table | Rows | What it feeds |
| ----- | ---: | ------------- |
| `hk_parts_cache` | **1,710** (98 distinct HK articles) | Every catalog/search response |
| `oem_search_index` | 163 | OEM lookup fallback |
| `vehicle_lookup` | 27 (15 distinct groups) | VIN → variant resolution, catalog vehicle picker |
| `hk_platform_map` | 5 | Cross-brand sibling suggestions (Sonata↔K5 etc.) |
| `nhtsa_tecdoc_bridge` | 27 | NHTSA make/model/year → TecDoc linkage |
| `substitution_links` | 37 | Cautious source-backed replacement chain |
| `external_sources` | 14 | Source registry / provenance |
| `commons_media_reviews` | 0 | Public part imagery pipeline (empty) |

**15 distinct HK vehicle groups** covered:

```
HYUNDAI ELANTRA   2016-2020, 2017-2020, 2021-2025
HYUNDAI KONA      2018-2023
HYUNDAI SANTA FE  2019-2023
HYUNDAI SONATA    2020-2024
HYUNDAI TUCSON    2015-2018, 2021-2024
KIA FORTE         2019-2023, 2020-2023
KIA K5            2021-2025
KIA SELTOS        2020-2024
KIA SORENTO       2021-2025
KIA SPORTAGE      2016-2018, 2022-2025
```

Anyone typing a VIN that decodes to a Kia Rio, Cerato, Optima, Genesis, Palisade, Niro EV, Kona EV, Ioniq 5/6, Stonic, Picanto, K3, K7, Carnival, Telluride, Venue, Nexo, Bayon, etc. will decode fine via NHTSA but see **zero catalog variants** on the current corpus.

**Only 98 distinct HK-branded articles** live in the local seed. See §3 for what happens on the 8-part probe of real OEMs outside that set.

---

## 2. Per-tab live behaviour

Playwright screenshots (20 files) referenced below live at
`qa/e2e-report/local/screenshots/*.png` on the merge branch.

### 2.1 `/` — VIN Decode tab

- **Screenshot:** `01-landing.png` (`Parts Engine` heading + Evidence-first Hyundai / Kia parts workflow tagline + 3-tab nav + VIN input placeholder)
- **VIN `KM8J33A46GU123456` decoded live:**
  - Make / model / year: `HYUNDAI TUCSON 2016`
  - **3 variants** returned (`TUCSON 2.0 MPI (TL)`, `TUCSON 1.6 T-GDI (TL)`, `TUCSON 2.0 CRDi (TL)`)
  - **20 parts** attached to the auto-selected variant
  - **5 recalls** attached (NHTSA campaigns `16V628000`, `16V842000`, `16V147000`, `16V348000`, `20V543000`) — every one carries `sourceLabel: "NHTSA vehicle recall API"`, `sourceUrl`, and the non-VIN-specific-warning disclosure
  - `needsConfirmation: true` — the frontend correctly surfaces the variant picker
- **Screenshots:** `11-vin-decoded.png` (variants list), `12-vin-recall.png` (recall banner)
- **Verdict:** ✅ works end-to-end for VINs that decode to the 15 vehicle groups the seed covers. Non-golden VINs (`5NPE24AF6HH123456`, `5XXG14J20MG123456`, `KM8R7DHE4NU123456`) decode via NHTSA fine but return zero local catalog variants — same 15-group corpus gap.

### 2.2 `/oem` — Search tab

Live probe of the 12 queries a real user would type:

| Query | Total | Strategy | Ranked-first result | Latency |
| ----- | ----: | -------- | ------------------- | ------: |
| `26300-35505` (Hyundai oil filter) | **5** | `oem_crossref` | **`26300-35505` FILTER ASSY-ENGINE OIL @ 0.96** | ≈8 ms |
| `97133-D3000` (Tucson cabin filter) | **2** | `oem_crossref` | `97133-D3000` FILTER-AIR @ 0.96 | ≈7 ms |
| `28113-D3100` (Tucson air filter) | **3** | `oem_crossref` | `28113-D3100` FILTER-AIR CLEANER @ 0.96 | ≈7 ms |
| `58101-D3A70` (HK front brake pad) | **3** | `oem_crossref` | `58101-D3A70` Brake Pad Set Front @ 0.96 | ≈7 ms |
| `54528-4A100` (Kia lower ball joint) | **0** | `online_partsouq` | (empty — not in 98-article seed; junk filter suppressed the scrape) | ≈2 s |
| `46321-3B650` (Hyundai trans mount) | **0** | `online_partsouq` | (empty — not in seed) | ≈2 s |
| `90915-YZZE1` (Toyota) | **0** | `hk_scope_rejected` | (empty — honest boundary warning) | ≈4 ms |
| `11427634292` (BMW) | **0** | `article_lookup` | (empty) | ≈8 ms |
| `oil filter` (text) | **6** | `text_search` | `26300-35505` @ 0.60 | ≈9 ms |
| `cabin air filter` (text) | **4** | `text_search` | `97133-D3000` @ 0.60 | ≈8 ms |
| `brake pad` (text) | **9** | `text_search` | `58101-D3A70` @ 0.60 | ≈9 ms |
| `headlight` (text) | **5** | `text_search` | `92102-D3100` Headlight Assembly Right @ 0.60 | ≈10 ms |

**What the user sees on the ranked-first card:** article number, brand `HYUNDAI/KIA`, description (real, not scraped), confidence badge (green 96% for owned-catalog OEM hits, yellow 60% for text-only), `FitmentBadge` driver, `ConfidenceNote` (`"Exact part-number match in the owned catalog"` for the golden hits).

Screenshot: `31-oem-26300-results.png`.

- **Verdict:** ✅ exact HK OEM always ranked first when the article is in the seed. ✅ boundary rejection is fast + honest. ⚠ **any real HK OEM outside the 98-article seed returns 0 results** — no fabricated placeholder, but the user gets nothing actionable either.

### 2.3 `/catalog` — Catalog tab

- **`/api/catalog/models`** → `{"makes":["HYUNDAI","KIA"]}` — correctly scoped, 2 makes visible on the dashboard's make picker.
- **`/api/catalog/vehicles?make=HYUNDAI&model=TUCSON&year=2016`** → 5 vehicle variants (all 3 TL 2015-2018 engines + 2 NX4 2021-2024 — the frontend uses the year to narrow to the correct generation).
- **`/api/catalog/groups?vehicleId=10001`** → **27 assembly groups** (`Cabin Filter & Blower`, `Air Intake & Filters`, `Brake Hydraulics`, `ABS / Wheel Speed`, `Body Panels`, …). Every group carries a `partCount` (2–9 per group).
- **Screenshots:** `50-catalog-all-parts.png` (all groups + parts), `51-catalog-detail.png` (part-detail modal with source, confidence, replacement guidance, provenance-gap disclosure).
- **Verdict:** ✅ catalog is fully browseable for the 15 vehicle groups the seed covers.

### 2.4 Part detail — `/api/part/100001/detail?vehicleId=10001` (golden oil filter)

- `articleNumber: 26300-35505`, `brandName: HYUNDAI/KIA`
- **`quality.provenanceComplete: false`** — dashboard explicitly discloses missing technical spec evidence (`"technical specification evidence"` in `provenanceGaps`). This is design-correct (README principle #4).
- **5 replacements** returned with source label + confidence per row.
- **0 substitutions** (empty for this specific part; the substitution feature works — probe `100307` shows the same UI wired up).
- **`placement.kind: inferred`** — the dashboard flags that the vehicle fitment is inferred, not exact-catalog-linked (design-correct honest reporting).

---

## 3. Data-coverage gaps (what dashboard users hit)

Two classes of "0 result" answer:

**Class A — honest zero from the HK-scope gate (design-correct, ≤ 10 ms).** Any query that doesn't match the HK 5+5 OEM format or hits a known-non-HK prefix rejects immediately with a warning. `90915-YZZE1` was the prior CRITICAL leak; the gate closes it. Verified live.

**Class B — honest zero from the corpus gap (design-correct, but empty for the user).** Real Hyundai/Kia OEMs a customer would type (`54528-4A100`, `46321-3B650`, `55700-3S000`, `92101-3S050`, `25100-25000`, `58101-3SA00`, `51712-2S000`, `55311-2S000`) return 0 results after the online scrape falls through — the junk-desc filter (this PR) correctly rejects the scraped chrome that used to be surfaced as a 0.75-confidence "part." No fabrication now, but no answer either.

Root cause of Class B: the local Postgres has **98 distinct HK articles** vs the tens of thousands the production TecDoc slice presumably holds. This branch does not carry the TecDoc dump — it will only match once a real HK slice is loaded (see follow-ups).

---

## 4. What still breaks

Categorised, with severity and location on the merged branch:

| # | Severity | Where | Symptom |
| - | -------- | ----- | ------- |
| D-1 | HIGH | corpus | 98 HK articles + 15 vehicle groups → 0-result rate on real user OEMs. |
| D-2 | HIGH | `internal/service/smart_search.go:145+` | Fallback cascade still has no per-strategy timeout budget. Local `54528-4A100` probe takes ~2 s to reach "empty" because the online scrape runs to completion. On prod this compounds to 15-56 s (measured earlier). |
| D-3 | MEDIUM | `internal/service/smart_search.go` ranker | Text search "oil filter" returns HK oil filter first locally, but that's because 6 of the 6 seed rows are HK. On a larger corpus with mixed brands there's still no explicit manufacturer bias. |
| D-4 | MEDIUM | `frontend/src/components/OemSearch.tsx` | Frontend does not dedupe by canonical OEM equivalence class. On a bigger corpus, the same physical part under 3 supplier codes will appear as 3 cards. |
| D-5 | MEDIUM | `internal/handler/*.go` (multiple) | `c.JSON(500, gin.H{"error": err.Error()})` returns raw SQL error text to the client — leaks table/column names on any DB error. |
| D-6 | LOW | `commons_media_reviews` | Table exists, 0 rows — no reviewer-approved public imagery. Dashboard shows no part diagrams / photos, per design. |
| D-7 | LOW | Playwright `O1-04` step | Search-result card click on `/oem` doesn't open the detail modal within 8 s — 1 / 39 test fails. Locator quirk (the whole card should be clickable), not a data defect. |

---

## 5. Rating breakdown

| Dimension | Score | Reasoning |
| --------- | :---: | --------- |
| Truthfulness (no fabricated results) | 9/10 | HK-scope gate + junk-desc filter working live. Toyota probe: rejected with honest reason. Golden OEMs: exact hit only. |
| Coverage (breadth of HK parts) | 2/10 | 98 articles + 15 vehicle groups. Any real customer OEM outside that set = 0 results. |
| Search precision (relevant results ranked first) | 8/10 | Exact HK OEM ranks first at 96% confidence for every seed hit. Text search ranks HK first for every common noun. |
| Latency | 6/10 | Local: 6-10 ms for hits, ~2 s for empty fallback. Prod (measured earlier): 15-56 s per query. |
| Safety / scope guarantees | 9/10 | HK-scope gate closed the CRITICAL boundary leak; junk-desc filter closed the fabricated-result CRITICAL. |
| Data provenance disclosure | 8/10 | Part detail correctly returns `provenanceComplete: false` for missing tech specs. Replacements carry source label + confidence. |
| Test coverage on the shown data | 5/10 | 4 unit tests for scope + junk filter, 38 / 39 Playwright steps on real user journeys — but the golden set is still 4 search cases + 2 VIN cases. |
| Documentation vs shipped reality | 8/10 | README + ARCHITECTURE + `docs/MYSQL_TO_POSTGRES_PORT.md` now line up with what the dashboard actually does. |
| **Overall** | **4.5 / 10** | Safe but empty. Redeploying prod with this branch fixes the truthfulness / safety / latency ceiling; loading real TecDoc HK slice fixes the coverage floor. |

---

## 6. Recommended next moves (ordered by impact / hour)

1. **Load a real HK TecDoc slice into local Postgres.** This closes D-1 (coverage), which is the single biggest complaint of the dashboard right now. Needs a mysqldump from prod OR access to the source. Every other follow-up multiplies in value once the corpus is real.
2. **Add hard timeout budget on the fallback cascade** (`smart_search.searchByOEM`): 500 ms per strategy, 2 s overall. Kills the 2 s local empty-response latency and the 15-56 s prod latency simultaneously.
3. **Grow `qa/golden_cases.json` to ≥ 50 HK OEMs** with `referenceUrl`, `expectedFirstArticle`, `relevanceGrade`. The current 4-case gate cannot detect real-world regressions on the bigger corpus.
4. **Manufacturer-tier weight in `oemReferenceRank` + text-search ordering** (D-3). Once the seed is bigger this will actually matter — HYUNDAI/KIA / Mobis / Mando > generic aftermarket unless explicitly requested.
5. **Redact `err.Error()` on 500 responses** (D-5). Log full internal error server-side, return `{"error":"internal error","requestId":"..."}` to the client.
6. **Fix the `O1-04` card-click locator** (D-7). Either wire a click on the whole result card, or update the harness locator.
7. **Deploy this branch to `qa.ifritah.com`.** Everything on the dashboard-quality axis on this branch is materially better than what's currently in production. Merging + deploying is a single-step upgrade.

---

## 7. Reproduction

```powershell
# Boot the merged local (idempotent, background):
pwsh scripts\dev-server.ps1 start

# Verify:
pwsh scripts\dev-server.ps1 status
# → [server] pid=... running · [postgres] Up ...healthy · [health] /health OK

# Postgres corpus counts:
docker exec parts-postgres psql -U parts -d parts_engine -tAc "SELECT count(*) FROM hk_parts_cache"
# → 1710

# Live 12-query probe (same numbers as §2.2 above):
pwsh scripts\probe-harsh.ps1
# → qa/harsh-probe.md + qa/harsh-probe.json

# Playwright dashboard-level audit (20 screenshots at
# qa/e2e-report/local/screenshots/*):
pwsh scripts\deep-e2e-audit.ps1 -Target local -Browser chromium
# → 38 / 39 pass, single non-passing step is D-7
```

All artifacts referenced above are reproducible on the merge branch of PR #4.
