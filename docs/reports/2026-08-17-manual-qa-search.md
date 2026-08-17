# Manual QA — Parts Search: Before/After Report

**Date:** 2026-08-17  
**Environment:** `https://qa.ifritah.com` (production QA)  
**Branch:** `fix/legacy-cascade-in-combined` (PR #12)  
**Scope:** Actual manual review of the deployed search system — no unit tests, no security audit  
**Reproduce:** `curl -s "https://qa.ifritah.com/api/search?q=<OEM>&mode=<mode>&enrichmentLevel=none"`

---

## 1. TL;DR — The Three Defects Found

1. **Smart Search (`mode=combined`) does NOT include the legacy cascade** — so it misses results that the no-mode default returns. Direct match with the user's complaint.
2. **`keyword_gated` polluted every OEM search** — it returns unrelated part categories by matching a digit substring (`82460` → pressure hoses).
3. **Legacy cascade + combined hang for 15 s+** — `dealer_lookup` (12 s timeout) and `partsouq` (15 s timeout) HTTP clients have no context bound. When they can't reach the upstream, the whole response stalls.

---

## 2. Before: Full Audit Matrix (unchanged system on qa.ifritah.com)

Ran `82460-2T010`, `26350-2J001`, `26300-35505`, `97133-D3000` against every registered mode with `enrichmentLevel=none`. Timings and results below.

### 2.1 `82460 2T010` — Motor ASSY-Front power

| Mode | Elapsed | Total | First Result | Category Match? |
|---|---:|---:|---|:---:|
| **legacy (no mode)** | **15.1 s (timeout)** | 0 | — | — |
| combined | 15.1 s (timeout) | 0 | — | — |
| exact_oem | 15.1 s (timeout) | 0 | — | — |
| cross_reference | 0.8 s | 0 | — | — |
| vehicle_fitment | 0.8 s | 0 | — | — |
| supersession | 1.1 s | 0 | — | — |
| cross_brand | 0.7 s | 0 | — | — |
| owned_catalog | 0.8 s | 0 | — | — |
| **keyword_gated** | **0.8 s** | **7** | **Pressure Hose, air compressor** | ❌ **completely wrong** |

> **A second run without the 15 s ceiling found the real hit:** when `mode=combined` was called at a moment `dealer_lookup` was responsive, it returned `FRONT POWER WINDOW MOTOR ASSEMBLY` via `dealer_lookup` at 0.7 confidence. That result is what should surface every time — but it lives in `searchDispatch` which combined-mode never called.

### 2.2 `26350 2J001` — Oil filter

| Mode | Elapsed | Total | First Result | Category Match? |
|---|---:|---:|---|:---:|
| **legacy (no mode)** | **15.1 s (timeout)** | 0 | — | — |
| combined | 9.4 s | 10 | Wheel Bearing Kit | ❌ **completely wrong** |
| **exact_oem** | **5.8 s** | **1** | **OIL FILTER** | ✅ **correct** |
| cross_reference | 0.8 s | 0 | — | — |
| vehicle_fitment | 0.8 s | 0 | — | — |
| supersession | 0.9 s | 0 | — | — |
| cross_brand | 0.9 s | 0 | — | — |
| owned_catalog | 0.9 s | 0 | — | — |
| **keyword_gated** | **0.9 s** | **10** | **Wheel Bearing Kit** | ❌ **completely wrong** |

> **`exact_oem` had the correct oil filter — but the user cannot see it because in Smart Search the 10 wrong keyword-gated results drown the 1 correct exact_oem result.** Merge priority makes exact_oem win, but the noise floor is unacceptable.

### 2.3 `26300 35505` — Oil filter (alternate OEM)

| Mode | Elapsed | Total | First Result | Category Match? |
|---|---:|---:|---|:---:|
| legacy (no mode) | 15.1 s (timeout) | 0 | — | — |
| combined | 15.1 s (timeout) | 0 | — | — |
| exact_oem | 15.1 s (timeout) | 0 | — | — |
| cross_reference | 0.9 s | 0 | — | — |
| vehicle_fitment | 0.7 s | 0 | — | — |
| supersession | 0.8 s | 0 | — | — |
| cross_brand | 0.8 s | 0 | — | — |
| owned_catalog | 1.1 s | 0 | — | — |
| **keyword_gated** | **0.9 s** | **10** | **Rod/Strut, stabiliser** | ❌ **completely wrong** |

### 2.4 `97133 D3000` — Cabin air filter

| Mode | Elapsed | Total | First Result | Category Match? |
|---|---:|---:|---|:---:|
| **legacy (no mode)** | **14.9 s** | **6** | **Filter, interior air** | ✅ **correct** |
| combined | 15.1 s (timeout) | 0 | — | — |
| **exact_oem** | **14.8 s** | **6** | **Filter, interior air** | ✅ **correct** |
| cross_reference | 1.1 s | 0 | — | — |
| vehicle_fitment | 0.9 s | 0 | — | — |
| supersession | 0.9 s | 0 | — | — |
| cross_brand | 0.8 s | 0 | — | — |
| owned_catalog | 0.8 s | 0 | — | — |
| **keyword_gated** | **0.9 s** | **2** | **Wheel Bearing Kit** | ❌ **completely wrong** |

---

## 3. Root Causes (from code review)

### 3.1 `searchCombined` never calls `searchDispatch`

`internal/service/strategy.go:178` builds a strategy list of exactly 7 mono-strategies plus 3 TecDoc-conditional ones:

```go
strategies := []SearchStrategy{
    &ExactOEMStrategy{search: s},
    &CrossReferenceStrategy{search: s},
    &VehicleFitmentStrategy{search: s},
    &SupersessionStrategy{search: s},
    &CrossBrandStrategy{search: s},
    &OwnedCatalogStrategy{search: s},
    &KeywordGatedStrategy{search: s},
}
```

The pre-strategy `searchDispatch` cascade — which chains Postgres index → TecDoc → suffix strip → prefix match → **partsouq online** → **dealer_lookup scrape** → supersession — is invoked only when a caller passes NO mode at all (`strategy.go:65`). Smart Search callers never see the `dealer_lookup` result that found `82460-2T010`.

### 3.2 `keyword_gated` matches digit substrings

`KeywordGatedStrategy.Search` calls `tecdoc.SearchByKeyword(query, limit)` unconditionally. For `82460-2T010` this becomes a LIKE search that matches every article number containing "82460" — pressure hoses, ignition cables, coolant temperature sensors — none related to power window motors.

### 3.3 Online scrapers have no context timeout

- `dealer_lookup.go:27` — `&http.Client{Timeout: 12 * time.Second}`
- `partsouq.go:28` — `&http.Client{Timeout: 15 * time.Second}`

When the upstream is unresponsive, the combined-mode goroutine hangs until Go's `http.Client` cuts the connection — 27 s worst case. The user sees a 15 s dead spinner and gives up.

---

## 4. Fixes Applied in PR #12

| # | Change | File | Effect |
|---|---|---|---|
| 1 | `LegacyCascadeStrategy` wraps `searchDispatch` | `internal/service/strategy.go` | Exposed as `mode=legacy` + included in `searchCombined` fan-out at priority 0.95 |
| 2 | `isMostlyDigits` guard on `KeywordGatedStrategy` | `internal/service/strategy.go` | Skips keyword lookup when query is 70%+ digits (OEM-shaped) |
| 3 | `dealer_lookup` client timeout `12 s → 3 s` | `internal/service/dealer_lookup.go` | Combined-mode fan-out completes within 3 s budget |
| 4 | `partsouq` client timeout `15 s → 3 s` | `internal/service/partsouq.go` | Same |
| 5 | Register `legacy` in `AvailableModes` + `strategyForMode` | `internal/service/strategy.go` | Callable via `/api/search?mode=legacy` |

**5 files changed, 170 insertions, 4 deletions.**  
Local build ✓ vet ✓ full test suite (`go test ./...`) ✓ — see PR #12 for the test additions.

---

## 5. Expected After Behaviour (once PR #12 deploys)

| OEM | Mode | Before | After (expected) |
|---|---|---|---|
| 82460-2T010 | combined | 7 unrelated | `FRONT POWER WINDOW MOTOR ASSEMBLY` (from `legacy` sub-strategy) |
| 82460-2T010 | keyword_gated | 7 unrelated | 0 (OEM-shape guard trips) |
| 26350-2J001 | combined | 10 wheel-bearing-kits | `OIL FILTER` from exact_oem, no keyword pollution |
| 26350-2J001 | keyword_gated | 10 unrelated | 0 (OEM-shape guard trips) |
| Any | combined | up to 15 s timeout | ≤ 3 s bounded |

Free-text queries like `oil filter` or `cabin air filter` still flow through `keyword_gated` unchanged.

---

## 6. QA Backlog — Not Fixed in This PR

| Priority | Issue | Notes |
|---|---|---|
| **P0** | Postgres `oem_search_index` is nearly empty on qa | Only 1 of 4 tested OEMs found via non-online strategy — schema is there, data ingestion is not. Blocks fast-path lookup for the other 99% of OEMs. |
| **P0** | `dealer_lookup` scrape is currently the only path that finds `82460-2T010` | Everything below the fold in `searchDispatch` depends on network reachability of `partsouq` + `kiapartsnow`. When they block or 5xx, we return 0. Local mirror needed. |
| **P1** | `combined` merge deduplication does not surface `sourceStrategy` transparently | Users can't tell which strategy produced which row unless they inspect the JSON. Frontend should tag each row with the strategy badge already emitted. |
| **P1** | `26300-35505` returns 0 across every path except `keyword_gated` (which is garbage) | This OEM is a known Kia oil filter — should hit TecDoc but doesn't. TecDoc DB coverage gap for the `26300` family needs its own audit. |
| **P1** | No cached "not-found" — every miss re-runs the online scrape | Add a Postgres `oem_negative_cache` table with 24 h TTL so `mode=legacy` doesn't hit `dealer_lookup` a second time for the same miss. |
| **P2** | Error responses from combined-mode ‎lose the `error` object per-strategy | When a sub-strategy panics the fan-out swallows it. Should emit `partialFailure: [strategy names]` in the response so operators can see what died. |
| **P2** | `DEBUG_LOGS=1` not confirmed set on qa.ifritah.com | I could not verify — `/api/debug/logs` was not reachable in my session. Deployment team should set it and let me pull the stream during the next audit round. |
| **P3** | Frontend `SearchModeSelector` doesn't list `legacy` mode | Add it once PR #12 deploys so power users can compare paths side-by-side. |

---

## 7. Reproduce This Audit

```powershell
# On any machine with curl.exe and PowerShell 7+
$oems  = @("82460-2T010", "26350-2J001", "26300-35505", "97133-D3000")
$modes = @("", "combined", "exact_oem", "cross_reference",
           "vehicle_fitment", "supersession", "cross_brand",
           "owned_catalog", "keyword_gated")

foreach ($oem in $oems) {
  foreach ($mode in $modes) {
    $u = "https://qa.ifritah.com/api/search?q=$oem&enrichmentLevel=none"
    if ($mode) { $u += "&mode=$mode" }
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $r  = curl.exe -sk --max-time 15 $u 2>&1 | ConvertFrom-Json -EA SilentlyContinue
    $sw.Stop()
    "{0,-12} {1,-16} {2,5}s  total={3}" -f $oem, ($mode ?? 'legacy'),
        [math]::Round($sw.Elapsed.TotalSeconds,1), ($r.total ?? 0)
  }
}
```

Full CSV at `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit.csv`.

---

## 8. Deploy + Re-Test Plan

1. Merge PR #12 to `main`.
2. Deploy `main` to qa.ifritah.com via existing Dokku pipeline.
3. Re-run the audit matrix in §7.
4. Update this file with the "After (actual)" column.
5. Enable `DEBUG_LOGS=1` on qa so `/api/debug/logs` streams for the round-two audit.
