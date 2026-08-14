# Main branch — thorough code review

**Reviewer:** OpenCode
**Date:** 2026-08-14
**Branch reviewed:** `main` @ `6663f0c init (#2)` (the current production/`qa.ifritah.com` codebase)
**Compared to:** `feature/parts-engine-baseline` (the newer Postgres migration)
**Method:** static reading of every file in `cmd/`, `internal/`, `frontend/src/`, `docker-compose.yml`, `Dockerfile`, `go.mod`, plus grep-driven security sweeps. This PR also carries **runnable fixes** for the highest-severity items; the rest are catalogued below as follow-ups.

> Every finding has a code (`M-*`), a severity, a file:line reference, and a concrete remediation. Codes are permanent identifiers usable in PR titles and issues.

---

## Executive summary

The `main` branch is the codebase currently serving `qa.ifritah.com` (per `/health`: `mode: mysql, tecdoc: true`). It is functionally rich (MySQL + full TecDoc, SQLite offline fallback, VIN decode via NHTSA vPIC, PartsOuq scraper) but carries several concrete defects that map 1:1 to the symptoms my earlier probes measured on production:

- 15–56 s search latency
- Bosch / Mann aftermarket ranked ahead of the exact Hyundai/Kia OEM
- `recalls: null` on every recall response
- Toyota/BMW/Ford OEMs surfaced as fake HK parts with Corolla compatibility
- `qa.ifritah.com` returns HTTP 200 + SPA HTML for unmatched `/api/*` GETs
- Scraped page chrome (`"Sign up with"`, `"LIFE-TIME-FILTER"`) shown as 0.75-confidence "parts"

Additionally I found:

- **A nil-pointer panic** in `internal/service/crossref.go:56` — any SQL error crashes the process (silent error swallow, then `defer rows.Close()` on nil `rows`).
- **A CORS misconfiguration** in `Dockerfile:33` — `CORS_ORIGINS=*` combined with `AllowCredentials: true` in `cmd/server/main.go:158-163` is a spec-violating and browser-rejected combination that gin-contrib/cors will echo the request origin for → any origin gets credentialed CORS.
- **Zero unit tests** anywhere in the repo (`go test ./...` finds nothing to run).
- **A stub `RecallsClient`** in `internal/service/recalls.go` that unconditionally returns `(nil, nil)`. The API advertises a recall endpoint that always returns null.

**Overall rating: 3.5 / 10.** Same rating as the feature branch — because the class of problems is the same and the mitigations shipped on the feature branch are not in main.

---

## Findings

### Severity-CRITICAL

#### M-CRITICAL-1 · CORS wildcard with credentials

**File:** `Dockerfile:33` + `cmd/server/main.go:158-163`
**Weight:** CRITICAL. Any origin can make credentialed requests to the API.

```dockerfile
ENV BIND_ADDR=0.0.0.0 \
    PORT=8080 \
    DATA_DIR=/app/data \
    CORS_ORIGINS=*         # ← default
```

```go
r.Use(cors.New(cors.Config{
    AllowOrigins:     cfg.CORSOrigins,   // ["*"] by default
    AllowMethods:     []string{"GET", "POST", "OPTIONS"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
    AllowCredentials: true,               // ← Dangerous with wildcard
}))
```

The CORS spec forbids `Access-Control-Allow-Origin: *` with credentials. `gin-contrib/cors` handles this by **echoing the request `Origin` header** — so any site making a `credentials: 'include'` fetch gets that fetch's origin back and full access.

**Remediation (shipped in this PR):**

- Remove the `CORS_ORIGINS=*` default from `Dockerfile`.
- `cmd/server/main.go` now refuses to enable `AllowCredentials` when any wildcard is present in the origins list, and logs a warning. Deployers must set an explicit `CORS_ORIGINS=https://ifritah.com,https://qa.ifritah.com` (or equivalent).

#### M-CRITICAL-2 · Nil-pointer panic in `CrossRef.FindOEMNumbers`

**File:** `internal/service/crossref.go:52-58`
**Weight:** CRITICAL. Any SQL error crashes the goroutine.

```go
rows, err := logQuery(s.db, "CrossRef.FindOEMNumbers", query, legacyArticleId)
if err != nil {
}                       // ← empty block, error silently swallowed
defer rows.Close()      // ← panics with nil-pointer if rows is nil (i.e. err != nil)
```

Any SQL error (network drop, table missing, malformed placeholder, dropped connection, TecDoc container restart, etc.) leaves `rows == nil` while `err != nil`. The `defer rows.Close()` then panics.

**Remediation (shipped in this PR):**

```go
rows, err := logQuery(s.db, "CrossRef.FindOEMNumbers", query, legacyArticleId)
if err != nil {
    return nil, fmt.Errorf("crossref OEM lookup: %w", err)
}
defer rows.Close()
```

#### M-CRITICAL-3 · Non-HK OEMs surfaced as HK parts

**File:** `internal/service/smart_search.go` (the whole `searchByOEM` cascade)
**Weight:** CRITICAL. Product safety: an app that claims to be scoped to Hyundai/Kia returns fake HK parts for Toyota / BMW / Nissan / Honda / Ford OEMs, with Corolla compatibility.

Reproduced against local + prod: typing Toyota `90915-YZZE1` (an oil filter) returns:

```json
{"query":"90915-YZZE1","results":[{
  "articleNumber":"90915YZZE1","description":"FILTER OIL",
  "confidence":0.75,"confidenceNote":"Online lookup from PartsOuq",
  "compatibility":["Corolla","Corolla Sed/Lb","Sprinter","Tercel"]}],
 "total":1,"searchStrategy":"article_to_oem_fallback"}
```

Root cause: the fallback cascade calls PartsOuq / dealer lookup / supersession without checking whether the OEM even matches the HK numbering format.

**Remediation (shipped in this PR):** new `internal/service/hk_scope.go` `IsHKOEM(oem)` classifier + wired into `smart_search.searchByOEM`. Toyota, BMW, Nissan, Honda, Ford OEMs are now rejected in ~4 ms with:

```json
{"results":null,"total":0,"searchStrategy":"hk_scope_rejected",
 "warnings":[
   "This app searches Hyundai/Kia parts only. This OEM prefix belongs to Toyota.",
   "Try the parts distributor for Toyota instead."]}
```

Includes 4 unit tests covering golden HK OEMs, boundary rejects, and suggested-make hints.

#### M-CRITICAL-4 · Scraped page chrome surfaced as parts

**File:** `internal/service/smart_search.go` online-lookup / dealer-lookup / supersession result builders
**Weight:** CRITICAL. Real HK OEMs (`46321-3B650`, `54528-4A100`, `55700-3S000`, `25100-25000`, `58101-3SA00`, `51712-2S000`) returned `description: "Sign up with"` — literally the text of a partsouq sign-up-button — as 0.75-confidence "parts". Prod also surfaces `"LIFE-TIME-FILTER"` for text queries like "cabin air filter".

Root cause: the online lookup accepts and displays any description PartsOuq returns without validating it isn't UI chrome.

**Remediation (shipped in this PR):** new `internal/service/junk_desc_filter.go` `IsJunkDescription(s)` deny-list. Wired into every online / dealer / supersession result builder in `smart_search.go`. Deny-list covers `sign up`, `sign in`, `log in`, `login`, `create an account`, `captcha`, `403 forbidden`, `504 gateway timeout`, `cookie preferences`, `life-time-filter`, `click here`, plus empty/whitespace-only strings.

### Severity-HIGH

#### M-HIGH-1 · Recall endpoint is a stub

**File:** `internal/service/recalls.go` (17 lines, whole file)
**Weight:** HIGH. The recall endpoint exists in the router, has a handler, and the frontend renders a `RecallBanner` component — but the service is:

```go
type RecallsClient struct{}

func NewRecallsClient() *RecallsClient { return &RecallsClient{} }

func (c *RecallsClient) GetRecalls(make, model string, year int) ([]model.Recall, error) {
    return nil, nil
}
```

That is why `qa.ifritah.com/api/recalls?make=HYUNDAI&model=TUCSON&year=2016` returns `{"recalls": null, "total": 0}` in 297 ms. The API is not silently broken — it's silently absent. Users see "no recalls" whether or not there are any.

**Remediation (NOT in this PR — separate follow-up):** port the working NHTSA client from `feature/parts-engine-baseline/internal/service/recalls.go` (Postgres branch has a full working implementation with `sourceLabel`, `sourceUrl`, non-VIN-specific warning). Add `NHTSA_RECALLS_URL=https://api.nhtsa.gov/recalls` to `.env.example` and `config.go`.

#### M-HIGH-2 · Unbounded fallback cascade — 15–56 s search latency

**File:** `internal/service/smart_search.go:145-467` (`searchByOEM` calls 8 fallback strategies serially)
**Weight:** HIGH. Every strategy runs to completion before the next is tried:

1. `crossRef.FindByOEM` (indexed local DB)
2. `oem.Search` (oem_search_index)
3. TecDoc `SearchByOEM` (21 M row table)
4. suffix-strip + retry
5. prefix fuzzy match (`prefixOEMSearch`)
6. **PartsOuq HTML scrape** (up to 15 s per fetch — see `partsouq.go:28`)
7. Dealer lookup (hyundaipartsdeal.com / kiapartsnow.com)
8. Reverse supersession
9. Aftermarket crossref fallback

Measured on `qa.ifritah.com`: golden OEM `26300-35505` takes **56.1 s**, `97133-D3000` takes 51.4 s, Toyota boundary probe times out after 60 s. This is a broken product, not a slow one.

**Remediation (NOT in this PR — separate follow-up):**

- Add a per-strategy deadline: 500 ms for local queries, 2 s for online/scrape.
- Add an overall search deadline: 3 s.
- On overall deadline exceeded, return the best partial result with `warnings: ["Search timed out; showing best partial results"]`.
- Kill the "run every strategy and then return the first non-empty" pattern — the HK-scope gate (shipped here) short-circuits at least the boundary-probe hangs.

#### M-HIGH-3 · JSON 404 for unmatched `/api/*`

**File:** `cmd/server/main.go:221-223` (`NoRoute` handler)
**Weight:** HIGH. Any unmatched `/api/*` GET currently serves the SPA `index.html` with status 200. A JSON client (mobile app, monitoring, curl) gets HTML + a JSON-parse error, never a clean 404.

Reproduced against local: `GET /api/vin/KM8J33A46GU123456` returns 200 + `text/html; charset=utf-8` + the React shell.

**Remediation (shipped in this PR):** `NoRoute` now branches on path prefix: `/api/*` returns `{"error":"not_found","path":...,"method":...}` with HTTP 404; everything else keeps falling through to `frontend/dist/index.html` for React Router.

#### M-HIGH-4 · Zero unit tests

**Signal:** `Get-ChildItem *_test.go -Recurse` returns nothing across the entire repo.
**Weight:** HIGH. No regression contract. A one-character edit to `smart_search.go` could silently break the ranker.

**Remediation (partially in this PR):** the 4 new unit tests in `internal/service/hk_scope_test.go` establish the contract for the HK-scope + junk-filter behavior. Every future change to those files must keep the tests green.

Follow-ups:

- Bring over the `TestPartsHandler`, `TestSmartSearch`, `TestRecalls` tests from the Postgres branch.
- Add an integration-test suite that runs against the MySQL container (Docker Compose profile).

### Severity-MEDIUM

#### M-MED-1 · Ranker has no source-quality / manufacturer bias

**File:** `internal/service/smart_search.go` — implicit ordering by DB row + confidence
**Weight:** MEDIUM. Text search "oil filter" on prod returned `A52-9618` "Cap, oil filter housing" (ACKOJA, Fiat context) as result #1. There is no explicit rule "if brand is `HYUNDAI/KIA` or `Mobis`, rank first".

**Follow-up (not in this PR):** add a manufacturer-tier weight to `oemReferenceRank` and to the text-search sort — HYUNDAI/KIA / Mobis / Mando should always outrank generic aftermarket unless the user has explicitly asked for aftermarket.

#### M-MED-2 · `InterpolateParams: true` in MySQL DSN

**File:** `internal/config/config.go:73`
**Weight:** MEDIUM. This disables the driver's prepared-statement round trip and inlines parameters client-side. Not a defect by itself — the code correctly uses `?` placeholders and `db.Query(sql, arg1, arg2)`. But it removes a defense-in-depth layer: if anyone in the future writes `db.Query("... WHERE x = " + userInput)`, the injection is immediate. Two places I flagged (`nhtsa/decoder.go:103`, `tecdoc.go:522`) use `fmt.Sprintf` with hardcoded table names — safe today but brittle.

**Follow-up:** drop `InterpolateParams: true`. Keep prepared statements. Cost: one extra network round trip per query on the first call; negligible after `db.Prepare` cache warms.

#### M-MED-3 · No rate limiting, no auth

**File:** `cmd/server/main.go` route setup
**Weight:** MEDIUM. Every `/api/*` route is unauthenticated and unrate-limited. `qa.ifritah.com` is public. A single hostile client can:
- exhaust the PartsOuq scrape rate (partsouq.go rate-limits itself to 1/s, so any attacker floods a queue),
- exhaust the MySQL connection pool (only 20 open per `db.SetMaxOpenConns(20)`),
- rack up NHTSA API cost (whenever M-HIGH-1 is fixed).

**Follow-up:** either put behind a reverse proxy that rate-limits (nginx `limit_req`), or add middleware (`gin-contrib/limits`).

#### M-MED-4 · Error messages leak internal detail

**File:** `internal/handler/vin.go:57`, `parts.go:64`, etc.
**Weight:** MEDIUM. `c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})` sends the raw error string. For SQL errors that string includes table names, column names, and sometimes the failed query. Fine in dev, leakage in prod.

**Follow-up:** log the full error server-side, return `{"error":"internal error","requestId":"..."}` to the client.

#### M-MED-5 · Docker Compose exposes obsolete `version` key + no volume-mounted secrets

**File:** `docker-compose.yml:1`
**Weight:** LOW. Prints an obsolescence warning on every boot. Also no MySQL service is defined — the app expects to reach an external MySQL. That's OK for prod, but there's no docker-compose target that boots a full stack for local dev.

**Follow-up:** drop `version:`, add an optional `mysql` service on a `full-stack` compose profile.

#### M-MED-6 · Frontend does not dedupe results

**File:** `frontend/src/components/OemSearch.tsx`
**Weight:** MEDIUM. Compared to `feature/parts-engine-baseline`, this branch's `OemSearch.tsx` (15 KB) is missing the `dedupeSmartResults` + `dedupeOEMResults` functions that the Postgres branch (27 KB) has. On prod I observed the same `A52-0438` article appearing 3 times in the top of `oil filter` results — that's the missing dedupe.

**Follow-up:** either port the Postgres-branch OemSearch, or add dedupe on the backend so every client benefits.

### Severity-LOW

#### M-LOW-1 · `.gitignore` too narrow

**File:** `.gitignore` (3 lines total)
**Weight:** LOW. Doesn't ignore `logs/`, `dist/`, `.vscode/`, `node_modules/`, `*.log`. That said, most of those are handled by `frontend/.gitignore` in practice. Recommend expanding.

#### M-LOW-2 · 47 experimental scripts checked in

**Directory:** `scripts/` (47 `.ps1`, `.py`, `.bat`, `.sh` files — `test_28310.ps1`, `test_all_brands.ps1`, `test_mazda.ps1`, `debug_epa.py`, …)
**Weight:** LOW. Most look like one-shot dev scripts, unclear which are load-bearing. Recommend either promoting the useful ones (build_epa_db.py, download_vpic_db.ps1) to documented tooling, or moving the rest to a separate `scratch/` folder.

#### M-LOW-3 · `.env.example` incomplete

**File:** `.env.example`
**Weight:** LOW. Missing `FRONTEND_DIR` (referenced by `cmd/server/main.go:213`) and `NHTSA_RECALLS_URL` (needed once M-HIGH-1 is fixed).

#### M-LOW-4 · Silent-mode determination could be clearer

**File:** `cmd/server/main.go:37-59`. The `offline` flag is only set when MySQL is unavailable AND SQLite is available. If both fail, `offline` stays false but `activeDB` is nil. Downstream code that does `if s.offline` misinterprets that as "MySQL mode". Small trap.

**Follow-up:** introduce an enum `dbmode.None | dbmode.SQLiteOnly | dbmode.MySQLOnly | dbmode.Both` and pass that around.

---

## What this PR ships (code)

| File | Change | Weight closed |
| ---- | ------ | ------------- |
| `internal/service/hk_scope.go` (new) | `IsHKOEM(oem)` classifier | M-CRITICAL-3 |
| `internal/service/hk_scope_test.go` (new) | 4 unit tests: golden HK cases, boundary rejects, suggested-make, junk-desc | M-CRITICAL-3, M-CRITICAL-4, M-HIGH-4 |
| `internal/service/junk_desc_filter.go` (new) | `IsJunkDescription(s)` deny-list | M-CRITICAL-4 |
| `internal/service/smart_search.go` (patched) | Wires the gate + filter into `searchByOEM`, online lookup, dealer lookup, supersession | M-CRITICAL-3, M-CRITICAL-4 |
| `internal/service/crossref.go` (patched) | Returns the SQL error instead of dropping it on the floor | M-CRITICAL-2 |
| `cmd/server/main.go` (patched) | `/api/*` returns JSON 404; CORS refuses wildcard + credentials with warn | M-CRITICAL-1, M-HIGH-3 |
| `Dockerfile` (patched) | Removes `CORS_ORIGINS=*` default | M-CRITICAL-1 |
| `docs/MAIN_BRANCH_REVIEW.md` (new) | This document | — |

## Follow-ups (NOT in this PR)

| Weight | Follow-up |
| ------ | --------- |
| M-HIGH-1 | Port a real `RecallsClient` that calls `api.nhtsa.gov/recalls/recallsByVehicle`; expose `sourceLabel`, `sourceUrl`, non-VIN-specific warning. Wire `NHTSA_RECALLS_URL` in config. |
| M-HIGH-2 | Hard per-strategy timeout budget (500 ms local, 2 s online) + overall 3 s cap in `smart_search.searchByOEM`. |
| M-MED-1 | Manufacturer-tier weight in `oemReferenceRank` — HK / Mobis / Mando outrank generic aftermarket. |
| M-MED-2 | Drop `InterpolateParams: true` from MySQL DSN. |
| M-MED-3 | Add rate-limit middleware (per-IP) or delegate to nginx. |
| M-MED-4 | Redact `err.Error()` in prod responses; return `requestId` for log correlation. |
| M-MED-6 | Backend-side dedupe on `SmartSearchResponse.Results` by canonical OEM equivalence. |
| Corpus  | Load a real HK TecDoc slice into local dev (needs prod MySQL dump). Same as `W-DATA-1..5` in the workspace QA report. |

## Reproduction / verification

**Unit tests:**

```powershell
cd C:\ssda\chatGPT\parts-engine   # your existing checkout
git fetch origin
git checkout review/thorough-audit-and-critical-fixes
go test ./internal/service/ -run 'TestIsHKOEM|TestIsJunkDescription' -v
```

Expected:

```
--- PASS: TestIsHKOEM_GoldenCases (0.00s)
--- PASS: TestIsHKOEM_BoundaryRejects (0.00s)
--- PASS: TestIsHKOEM_SuggestsMake (0.00s)
--- PASS: TestIsJunkDescription (0.00s)
PASS
```

**Behavioural — Toyota boundary:**

```powershell
# BEFORE this PR:
$ Invoke-RestMethod http://.../api/search?q=90915-YZZE1
  → 1 result, description "FILTER OIL", 0.75 confidence, compatibility ["Corolla",...]

# AFTER this PR:
$ Invoke-RestMethod http://.../api/search?q=90915-YZZE1
  → 0 results, strategy "hk_scope_rejected",
     warnings ["This app searches Hyundai/Kia parts only. This OEM prefix belongs to Toyota.",
               "Try the parts distributor for Toyota instead."]
```

**Behavioural — junk descriptions:**

```powershell
# BEFORE:
$ Invoke-RestMethod .../api/search?q=54528-4A100
  → 1 result, description "Sign up with", 0.75 confidence

# AFTER:
$ Invoke-RestMethod .../api/search?q=54528-4A100
  → 0 results (honest — this OEM is not in the local seed cache)
```

**Behavioural — JSON 404:**

```powershell
# BEFORE:
$ Invoke-WebRequest http://.../api/vin/KM8J33A46GU123456
  → 200 OK, Content-Type: text/html, body = React SPA HTML

# AFTER:
$ Invoke-WebRequest http://.../api/vin/KM8J33A46GU123456
  → 404 Not Found, Content-Type: application/json,
     body {"error":"not_found","path":"/api/vin/KM8J33A46GU123456","method":"GET"}
```

**CORS:**

```powershell
# BEFORE (Dockerfile default was CORS_ORIGINS=*):
$ curl -H "Origin: https://evil.example.com" .../api/search?q=foo
  → Access-Control-Allow-Origin: https://evil.example.com
     Access-Control-Allow-Credentials: true

# AFTER:
$ # No CORS_ORIGINS set → server logs a warning, credentials disabled.
$ CORS_ORIGINS=https://qa.ifritah.com,https://ifritah.com → strict allowlist.
```
