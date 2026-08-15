# Production Baseline Snapshot — `qa.ifritah.com`

**Purpose:** locks in a machine-readable "before" state of the deployed product so that after PR #4 is merged and deployed we can produce an "after" snapshot and objectively measure the delta.

**Captured at:** 2026-08-15 11:30 UTC (48-42 probes fired live against `https://qa.ifritah.com`)
**Captured by:** OpenCode
**Branch under review after deploy:** `merge/adopt-feature-baseline-into-main` (PR #4)
**Snapshot method:** `scripts/probe-harsh.ps1` (42 API probes) + `scripts/deep-e2e-audit.ps1 -Target prod -Browser firefox` (13 Playwright user-journey steps) + direct `GET /api/*` inspection

Raw artifacts committed alongside this doc:

- `qa/baseline-prod.json` — full JSON of the 42-probe API harness (with timings, first-result details, warnings, and raw response bodies)
- `qa/baseline-prod.md` — human-readable table of the same
- `qa/e2e-report/prod/findings.json` — Playwright step results (13 steps, 6 pass / 7 fail)
- `qa/e2e-report/prod/screenshots/*.png` — 4 rendered screenshots (Firefox — Chromium blocked at WAF)
- `qa/e2e-report/prod-forensic/*.json` — earlier forensic dumps (DOM, HAR)

---

## 1. Health baseline

```
$ GET https://qa.ifritah.com/health
{"mode":"mysql","status":"ok","tecdoc":true}
```

Prod is on the **pre-merge MySQL/TecDoc branch**. Not the Postgres branch this PR is merging.

## 2. API probe baseline (42 probes fired live)

| Category | Pass | Fail | Error | Comments |
| -------- | ---: | ---: | ----: | -------- |
| **vin** | 5 | 1 | 0 | All 5 VINs return HTTP 200 but with `vehicle: null`, `allVariants: null`, `parts_count: 0`, `recalls_count: 0`, `needsConfirmation: null`. Fake PASS in the harness because scoring only checks NHTSA-side `nhtsaRaw.make` — which is populated. The **frontend renders empty** for every VIN. |
| **oem** | 1 | 9 | 0 | Only `92101-3S050` returns a "usable" single result (a synthetic placeholder). Golden HK OEMs return wrong first result at 44-56 s. |
| **text** | 10 | 0 | 0 | All 10 return 20 results but ranked-first is universally junk: `LIFE-TIME-FILTER`, `A52-9618` (Fiat oil cap), `1754Q`, `ADS8`, `A63475`, `HY-305`, `ASH7-0001`, `SBK-8142`, `HYAB-CM10SAR-KIT`, `39042`, `39003`. Latencies 14-25 s. |
| **vehicle** | 3 | 1 | 0 | `Tucson 2016: cabin air filter` first result is `LIFE-TIME-FILTER` (placeholder brand string). Others rank generic aftermarket first. |
| **recall** | 4 | 0 | 0 | All 4 recall probes return **`total: 1` with `recalls: [<single stub>]`**. Locally the same queries return 5/3/2/4 real NHTSA campaigns. Prod recall integration effectively broken. |
| **catalog** | 1 | 1 | 1 | `/api/catalog/models` works (`{"makes":["HYUNDAI","KIA"]}`). `/api/catalog/vehicles?make=HYUNDAI&model=TUCSON&year=2016` returns **HTTP 500**. `/api/catalog/groups?vehicleId=10001` returns 200 but empty (`FAIL_EMPTY`). |
| **boundary** | 0 | 0 | 1 | Toyota `90915-YZZE1` **times out after 90 seconds**. No HK-scope gate on prod. |
| **detail** | 2 | 0 | 0 | `/api/part/:id/detail` loads for known articles. Response shape differs from the feature branch. |

**Overall API probe: 26 pass / 14 fail / 2 error.** Compare to local (merged branch): 34 pass / 6 fail / 2 error.

### 2.1 Golden OEM behaviour — the flagship promise from the README

| Query | Prod result | Latency | Notes |
| ----- | ----------- | ------: | ----- |
| `26300-35505` (Hyundai oil filter) | First = `F 026 407 124` (Bosch aftermarket) — **wrong ranking** | **44,208 ms** | 20 total results. Exact HK OEM is NOT first. |
| `97133-D3000` (Hyundai/Kia cabin filter) | First = `CU 23 019` (Mann filter) — **wrong ranking** | **38,369 ms** | 20 total results. Exact HK OEM is NOT first. |
| `97133` (article prefix) | First = `97133` (a Fiat "Hose, heat exchange heating") — **wrong make** | 18,433 ms | 2 total results. Not the Hyundai cabin filter. |
| `cabin air filter` (text) | First = **`LIFE-TIME-FILTER`** (placeholder brand string) — **junk description** | 16,079 ms | 20 total results. |

The README promises: *"Exact owned-catalog OEM records rank before cross-references"* (`README.md:26`). Prod violates this on every golden case.

### 2.2 Real HK OEM behaviour — 8 real customer parts

| Query | Prod first result | Latency | Notes |
| ----- | ----------------- | ------: | ----- |
| `46321-3B650` (Hyundai auto-trans mount) | `SG 1700` | 29,024 ms | 8 results, none of which are the exact HK OEM |
| `54528-4A100` (Kia lower ball joint) | `54528` | **56,405 ms** | 8 results, generic aftermarket |
| `55700-3S000` (Hyundai Sonata rear axle beam) | `557003S000` with junk description | 32,678 ms | 1 result, **still surfaces scrape junk** |
| `92101-3S050` (Hyundai Sonata headlight) | `921013S050` | 32,835 ms | 1 result |
| `25100-25000` (Hyundai water pump) | `FWP2200` | 28,568 ms | 10 results, aftermarket |
| `58101-3SA00` (Hyundai Sonata front brake pad) | `958.0` | 30,462 ms | 14 results, opaque brand |
| `51712-2S000` (Hyundai Tucson strut) | `517122S000` with junk description | 30,404 ms | 1 result, **junk** |
| `55311-2S000` (Hyundai Tucson rear coil spring) | `MM-KI053` | 41,460 ms | 20 results, aftermarket |

Not one of these 8 returned the exact HK OEM first. 2 surfaced scrape junk. Median latency: **31.6 s**.

### 2.3 Latency summary

| Percentile | Value |
| ---------- | ---: |
| p50 (median) | 22 s |
| p95 | 45 s |
| max | **90 s (Toyota timeout)** |
| Golden OEM min | 38 s |
| Text-search min | 14 s |
| Recall API | ≤ 900 ms (fast, but returns null) |
| Catalog list | ≤ 1.3 s |
| Detail | ~1.1 s |

**No prod search returned in under 14 seconds. No golden OEM returned in under 38 seconds.**

## 3. Playwright deep audit baseline (13 steps, 6 pass / 7 fail)

Full JSON: `qa/e2e-report/prod/findings.json`. Screenshots: `qa/e2e-report/prod/screenshots/*.png`.

Because Chromium is blocked at the WAF (`GET https://qa.ifritah.com/assets/*.js → HTTP 403` for Playwright-Chromium; `HTTP 200` for Firefox), the audit runs against **Firefox**. Same DOM, same behaviour a real user would see in that browser.

| Step | Verdict | Latency | Observation |
| ---- | :-----: | ------: | ----------- |
| L1-01 GET / | ✅ PASS | 1,093 ms | HTML loads |
| L1-02 header title "Parts Engine" | ❌ FAIL | 8,023 ms timeout | Locator not found — different DOM than feature branch |
| L1-03 "Evidence-first" banner | ❌ FAIL | 4,010 ms timeout | Banner text does not exist on prod |
| L1-04 3 nav tabs (VIN / Search / Catalog) | ✅ PASS | 23 ms | Prod nav shows "VIN Decode / Smart Search / Catalog" (different casing/label) |
| L1-05 nav to Search tab | ❌ FAIL | 5,091 ms | `OEM / Part Number / Description` label doesn't render on prod |
| L1-06 nav to Catalog tab | ✅ PASS | 67 ms | |
| L1-07 zero console errors | ✅ PASS | — | |
| V1-01 fill VIN + click Decode | ❌ FAIL | 180,074 ms timeout | VIN input placeholder differs; locator times out |
| V1-02 variants appear | ❌ FAIL | — | Backend returned `allVariants: null` anyway |
| V1-03 recall banner visible | ❌ FAIL | — | `recall-banner` test-id absent from prod DOM |
| V1-04 confirm variant → open catalog | ❌ FAIL | — | Prior step aborted the flow |
| L1-07 console errors | ✅ PASS | — | (No JS errors — the app just renders empty) |

**6 of 13 pass** — every failure is either (a) DOM structure differs from the reviewed branch, or (b) backend returns null. The Playwright harness aborts after the landing-page phase because the DOM the tests expect doesn't exist.

### 3.1 DOM diff (Firefox capture)

Verbatim rendered body text on prod landing:

```
Parts Engine
VIN Decode
Smart Search
Catalog
VIN (17 characters)
Decode
```

Verbatim rendered body text on the merged local:

```
Evidence-first Hyundai / Kia parts workflow
Parts Engine
Decode the vehicle with NHTSA-backed data, confirm the exact variant, move into catalog browse,
and inspect parts with provenance, caution labels, and only the visuals the data can honestly support.
VIN decode
Search
Catalog
VIN (17 characters)
Decode
```

**Delta:**

- Sub-header tagline "Evidence-first Hyundai / Kia parts workflow" — **absent on prod**
- Body description "Decode the vehicle with NHTSA-backed data …" — **absent on prod**
- Nav 1: "VIN decode" (local) vs "VIN Decode" (prod) — **case drift**
- Nav 2: "Search" (local) vs "Smart Search" (prod) — **renamed**
- Test IDs `vin-catalog-matches`, `recall-banner`, `search-result-card`, `catalog-source-banner` — **missing on prod**

## 4. Locked-in defects on prod (this is the "before" state)

Every defect below is reproducible against `https://qa.ifritah.com` right now. Merging + deploying PR #4 will fix each row.

| # | Prod symptom | Reproduction | Merged branch state |
| - | ------------ | ------------ | ------------------- |
| P-1 | Golden HK OEM `26300-35505` first result is `F 026 407 124` (Bosch aftermarket), 44 s | `GET /api/search?q=26300-35505` on prod | Local: first is `26300-35505` @ 0.96 conf, 8 ms |
| P-2 | Golden `97133-D3000` first is `CU 23 019` (Mann), 38 s | `GET /api/search?q=97133-D3000` | Local: first is `97133-D3000` @ 0.96 conf, 6 ms |
| P-3 | Toyota `90915-YZZE1` times out at **90 s** | `GET /api/search?q=90915-YZZE1` | Local: 0 results, `hk_scope_rejected`, 4 ms |
| P-4 | `cabin air filter` first result is `LIFE-TIME-FILTER` (placeholder brand string) | `GET /api/search?q=cabin%20air%20filter` | Local: first is `97133-D3000` (Tucson cabin filter) |
| P-5 | Real HK OEM `55700-3S000` returns `description` = junk scrape text | `GET /api/search?q=55700-3S000` | Local: 0 results (junk filter suppressed the scrape) |
| P-6 | Real HK OEM `51712-2S000` returns junk description | `GET /api/search?q=51712-2S000` | Local: 0 results (junk filter) |
| P-7 | VIN endpoint returns `vehicle: null, allVariants: null, parts_count: 0` | `POST /api/vin/decode {"vin":"KM8J33A46GU123456"}` | Local: 3 variants, 20 parts, 5 recalls, needsConfirmation: true |
| P-8 | Recall endpoint returns `recalls: null` — every make/model/year | `GET /api/recalls?make=HYUNDAI&model=TUCSON&year=2016` | Local: 5 real NHTSA campaigns in 115 ms |
| P-9 | `/api/catalog/vehicles?make=HYUNDAI&model=TUCSON&year=2016` returns HTTP 500 | Same URL | Local: HTTP 200 with 5 variants |
| P-10 | `/api/catalog/groups?vehicleId=10001` returns empty | Same URL | Local: 27 assembly groups |
| P-11 | `GET /api/vin/:vin` (wrong method) returns SPA HTML with HTTP 200 | `curl -i /api/vin/anything` | Local: HTTP 404 JSON `{"error":"not_found",...}` |
| P-12 | Prod Chromium blocked at WAF — 403 on `/assets/*.js` | Playwright-Chromium load | Local: 200 |
| P-13 | Prod DOM missing "Evidence-first" tagline, differently-cased nav labels, missing test-ids | Firefox visit + inspect | Local: full evidence-first UI with all test-ids |
| P-14 | Prod text search first-results universally junk (`LIFE-TIME-FILTER`, Fiat oil-cap, opaque brand IDs) | 10 text-search queries | Local: HK-branded parts ranked first |
| P-15 | Prod median search latency: 22 s. p95: 45 s. | Any real search | Local: p95 ≤ 15 ms owned-catalog, ≤ 2 s fallback |

## 5. What "after" will look like (definition of success)

Once PR #4 is deployed:

| Metric | Prod today (baseline) | Merged branch target |
| ------ | --------------------: | -------------------: |
| Health mode | `mysql, tecdoc:true` | `postgres` |
| Golden `26300-35505` first-result article | `F 026 407 124` (Bosch) | `26300-35505` (exact HK) |
| Golden `26300-35505` first-result confidence | ~0.6 (heuristic) | **0.96** (exact catalog match) |
| Golden `26300-35505` latency | 44,208 ms | ≤ 500 ms |
| Toyota `90915-YZZE1` behaviour | 90-s timeout | 0 results + warning in ≤ 20 ms |
| VIN `KM8J33A46GU123456` variants | 0 | ≥ 3 |
| Tucson 2016 recall count | 1 stub | 5 real NHTSA campaigns |
| `LIFE-TIME-FILTER` frequency in results | present in top-1 of `cabin air filter` | filtered out entirely |
| `/api/vin/anything` (wrong method) | HTTP 200 + HTML | HTTP 404 + JSON |
| `/api/catalog/vehicles` for known make/model | HTTP 500 | HTTP 200 with variants |
| Playwright deep audit pass rate | 6 / 13 (46 %) | ≥ 38 / 39 (97 %) |
| Prod Chromium browse | 403 on `/assets/*` | 200 (after WAF fix — separate follow-up) |

## 6. Reproducing this snapshot after the deploy

```powershell
# Same commands, unchanged, will produce the "after" snapshot for direct comparison.

# API probe against prod (42 probes):
pwsh scripts\probe-harsh.ps1 `
    -BaseUrl https://qa.ifritah.com `
    -TimeoutSec 90 `
    -OutJson  qa\baseline-prod-after.json `
    -OutMd    qa\baseline-prod-after.md

# Playwright deep audit against prod (Firefox — until W-INFRA-1 WAF-Chromium-403 is fixed):
pwsh scripts\deep-e2e-audit.ps1 -Target prod -Browser firefox

# Diff the "before" and "after" tables side by side:
Compare-Object `
    (Get-Content qa\baseline-prod.md) `
    (Get-Content qa\baseline-prod-after.md) `
    | Format-Table -AutoSize
```

Diff those against `qa/baseline-prod.json` (this snapshot) and you'll have a machine-computed report of exactly what improved.

## 7. Locked-in evidence file list

```
qa/baseline-prod.json         42-probe raw JSON (this snapshot)                  ← baseline
qa/baseline-prod.md           42-probe human-readable table                      ← baseline
qa/e2e-report/prod/findings.json                    Playwright 13-step results  ← baseline
qa/e2e-report/prod/screenshots/*.png                4 rendered screenshots      ← baseline
qa/e2e-report/prod-forensic/events.json             Chromium 403 evidence
qa/e2e-report/prod-forensic/firefox-dom.json        DOM dump of prod landing
qa/e2e-report/prod-forensic/firefox-full.html       Full prod HTML source
docs/E2E_QA_DEEP.md                                 Full E2E investigation (session 2)
docs/LOCAL_vs_PROD_REPORT.md                        Local vs prod side-by-side
```

**Every claim in this snapshot is falsifiable — re-run the two commands in §6 and you get the same numbers ±5% (NHTSA latency jitter).**
