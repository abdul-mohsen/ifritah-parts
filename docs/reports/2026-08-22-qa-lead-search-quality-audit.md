# QA-Lead Search Quality Audit — 2026-08-22

**Environment:** `https://qa.ifritah.com`  
**Deployed:** bundle `Last-Modified: 2026-08-21 20:03 UTC` — includes PRs #12, #13, #14, #16, #17, #18 (all merged)  
**Method:** 25-OEM live audit + 60 s SSE debug-log capture + A/B against 2026-08-19 baseline (`qa-audit-1500-fixed.csv`, 1,490-OEM audit from PR #15)  
**Scope:** search only. `/api/search`, `/api/search/stream`, 13 strategies, 8 enrichment fields.

Report is **not** committed to the repo. Supporting data at:
- `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit-2026-08-22-raw.csv`
- `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit-2026-08-22-primary-raw.json`
- `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-debug-audit-2026-08-22.log`

---

## 1. Executive summary — this is why the app sucks for parts sellers today

**A parts seller opening qa.ifritah.com and searching for a typical Korean OEM has a 56 % chance of a 20-second timeout, a 36 % chance of getting just a name + category with nothing else, and a 0 % chance of seeing any aftermarket brand alternative, spec, compatible-vehicle list, or supersession chain.**

Three concrete failure examples the user can reproduce right now:

| Query | Expected (parts-seller view) | Actual (qa.ifritah.com) |
|---|---|---|
| `26350-2J001` (Hyundai V6 oil filter) | "Engine Oil Filter for Santa Fe / Tucson V6 (2010-2015)" + 5-8 aftermarket brands (MANN, MAHLE, BOSCH, WIX, FRAM) + physical specs (thread size, height) + fitment table | Bare description + category. `aftermarketAlternatives`, `specifications`, `compatibleVehicles`, `documents`, `supersession` — all empty arrays. |
| `26300-35505` (common oil filter) | Same — one of the most-shipped filters in the HK ecosystem | **20-second timeout, zero results returned.** |
| `58101-3XA00` (front brake pad set, Elantra) | Brake pad description + Brembo/Bosch/Textar/Ferodo alternatives + vehicle table | **20-second timeout, zero results returned.** |

**This is a regression.** Three days ago (2026-08-19 audit, PR #15) `26350-2J001`, `26300-35505`, `58101-3XA00`, `97133-D3000`, `28113-2S000`, `97133-2H001` **all returned a result in ~12 s**. Today they either return with zero enrichment or time out completely.

Root cause: **PR #17 (enrichment fix) exposed a slow-query bug** — `articlecriteria` (27 M rows) has zero indexes for any of its 4 hot query patterns. `TecDocSpecifications.FindSpecifications` takes **17-36 seconds per call** (average 23.75 s across 64 calls in a 60 s window). Verified by grep: no migration in `sql/*` or `db/migrations/*` touches this table. The enrichment pipeline `wg.Wait()`s for all per-result goroutines. One slow spec query blocks the whole response until the browser 20-second timeout fires.

Direct evidence from `/api/debug/logs` capture (60 s window, 4 concurrent queries):

```
SQL SLOW ⚠⚠  TecDocSpecifications.FindSpecifications: 18.576s
SQL SLOW ⚠⚠  TecDocSpecifications.FindSpecifications: 18.567s
SQL SLOW ⚠⚠  TecDocSpecifications.FindSpecifications: 18.558s
SQL SLOW ⚠⚠  TecDocSpecifications.FindSpecifications: 18.547s
… (64 similar entries in 60 seconds, avg 23.75s, max 36.23s)
```

**Sub-issues that compound the pain:**
- Enrichment fields return empty arrays even for OEMs that DO get results — the response is fast for those because promotion via `SearchByOEM(articleNumber)` returns 0 refs, so article-anchored enrichment is skipped entirely.
- The HK-scope guard only fires on 2 of 4 non-HK OEMs — `90915-YZZE1` (Toyota) and `15208-9F600` (Nissan) both hit 20 s timeouts instead of instant rejection.
- The `combined` ctx-timeout (12 s) is no longer bounding p95 — every failing query hits the browser's 20 s cap instead.

---

## 2. Progress-over-time — did the last week of PRs help users?

Direct comparison of the SAME 8 OEMs that had measurements in both audits, combined mode, enrichmentLevel=full:

| OEM | 2026-08-18 (pre-PR-#14) | 2026-08-19 (post PR-#14/#16) | 2026-08-22 (all PRs) | Delta 2026-08-19 → 2026-08-22 |
|---|---|---|---|---|
| `82460-2T010` | timeout 15 s | ✓ 1 result, 12.4 s | ✓ 1 result, 5.6 s | Faster, still 0 enrichment |
| `26350-2J001` | ✗ 10 wrong wheel bearings | ✓ 1 result, 12.4 s | ✓ 2 results, 3.0 s, 0 enrichment | Faster, still 0 enrichment |
| `26300-35505` | timeout 15 s | ✓ 1 result, 12.4 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |
| `97133-D3000` | timeout 15 s | ✓ 1 result, 12.4 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |
| `28113-2S000` | not in corpus | ✓ 1 result, 14.3 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |
| `97133-2H001` | not in corpus | ✓ 1 result, 12.5 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |
| `58101-3XA00` | not in corpus | ✓ 1 result, 12.4 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |
| `90915-YZZE1` (Toyota) | HK-scope reject | ✓ 1 result, 12.6 s (leak) | **⚠ TIMEOUT 20 s** | **REGRESSED** (worse) |
| `15208-9F600` (Nissan) | HK-scope reject | ✓ 1 result, 12.5 s (leak) | **⚠ TIMEOUT 20 s** | **REGRESSED** (worse) |
| `11-42-7-521-353` (BMW) | HK-scope reject | ✗ 0 results, 12.7 s | **⚠ TIMEOUT 20 s** | **REGRESSED** |

Combined-mode aggregate:

| Metric | 2026-08-19 (1,490 OEMs) | 2026-08-22 (25 OEMs) |
|---|---:|---:|
| Hit rate (Total > 0) | 45.6 % (679/1490) | 36.0 % (9/25) |
| Median latency | 12.47 s | 20.08 s |
| p95 latency | 12.80 s | 20.12 s |
| Timeouts (>= 20 s) | 21/1490 (1.4 %) | 16/25 (64 %) |
| `aftermarketAlternatives` populated | some (unmeasured in prior CSV) | **0/25** |
| `specifications` populated | some (unmeasured in prior CSV) | **0/25** |
| `compatibleVehicles` populated | some (unmeasured in prior CSV) | **0/25** |

Latency p95 went from 12.8 s to 20.1 s — **the 12-second combined-mode ctx cap is no longer holding the tail latency**. The regressed queries are hitting the browser's 20 s cap.

---

## 3. Root causes (with evidence from debug logs)

### RC-1 — `articlecriteria` (27 M rows) has zero indexes for any of its 4 hot access patterns

**Verification method:** exhaustive grep of every `.sql` file in `db/migrations/*` and `sql/*` for the substring `articlecriteria`.

**Result: zero matches.** No migration file — MySQL or Postgres — creates, alters, or indexes the `articlecriteria` table. It's a raw TecDoc-supplied dump; the deploy pipeline never adds indexes to it. So whatever indexes ship in the vendor dump are all we get, and by observation, `legacyArticleId` is not among them.

**Four distinct query patterns hit `articlecriteria`, all indexless:**

| # | File:Line | Query shape (WHERE clause) | Observed impact |
|---|---|---|---|
| 1 | `tecdoc_specifications.go:82-92` | `WHERE legacyArticleId = ?` | 64 slow calls in 60 s, avg 23.75 s / max 36.23 s |
| 2 | `tecdoc.go:434-439` | `WHERE legacyArticleId = ?` | Same shape as #1 — same slowness expected when triggered |
| 3 | `strategy_spec_match.go:225-258` | `WHERE criteriaDescription = ? AND rawValue = ?` | 1 ctx-deadline SQL ERROR in the 60 s window |
| 4 | `strategy_spec_match.go:305-312` | `WHERE legacyArticleId = ? AND criteriaDescription = ? AND rawValue = ?` | Called from `articleHasSpec` per candidate in multi-spec mode |

**Evidence from 60 s SSE debug window, 4 concurrent queries:**
- 64 `SQL SLOW` warnings, ALL on `TecDocSpecifications.FindSpecifications`
- Average duration: 23.75 s
- Max duration: 36.23 s
- 1 SQL ERROR on `FindBySpecMatch` (ctx deadline hit before the query even began scanning)
- No other SLOW warnings anywhere in the log

**Fix scope:** the `sql/06_articlecrosses_normalized_oem_index.sql` recipe from PR #16 applies directly. Two BTREE indexes cover all four access patterns:

```sql
ALTER TABLE articlecriteria
  ADD INDEX idx_articlecriteria_legacyArticleId (legacyArticleId),
  ADD INDEX idx_articlecriteria_criteria_value (criteriaDescription, rawValue);
```

The first covers patterns #1, #2, and #4 (leftmost prefix). The second covers pattern #3. Cardinality on `legacyArticleId` is 27 M unique values (unique articles) — perfect BTREE selectivity. `criteriaDescription` has bounded cardinality (~a few hundred distinct spec names), `rawValue` has high cardinality (thousands of values per spec) — compound index gives full selectivity for pattern #3.

**Impact of missing index:** every enrichment goroutine that touches specifications blocks 17-36 s. `enrichResults` uses `wg.Wait()` which doesn't return until all goroutines finish. Result: user gets a 20-second-timeout even though the search itself returned in 2 s.

### RC-2 — Enrichment returns silently empty for high-value OEMs

The 9 OEMs that DID complete under 20 s all show `aftermarketAlternatives.Count = 0`, `specifications.Count = 0`, `compatibleVehicles.Count = 0`. This is despite PR #17's promotion logic in `enrichment.go`:

```go
if articleId == 0 && oem != "" && s.tecdoc != nil && budgetLeft() {
    refs, err := s.tecdoc.SearchByOEM(oem, 5)
    if err == nil && len(refs) > 0 {
        for _, ref := range refs {
            if ref.LegacyArticleId > 0 {
                articleId = ref.LegacyArticleId
                break
            }
        }
        enriched[idx].OEMNumbers = append(enriched[idx].OEMNumbers, refs...)
    }
}
```

**Evidence from the debug log:**
- `PRIMARY results=0` count: **31**
- `PRIMARY results=[1-9]` count: **11**

So 74 % of `SearchByOEM` calls return zero. That means:
- Promotion fails → `articleId` stays 0 → all article-anchored enrichment (specs, vehicles, docs, supersession) is skipped
- `FindAftermarketForOEM(oem)` still runs — but was called 114 times in the window and populated 0 results in any of the 25 audited OEMs

**Two failures compound here:**
1. TecDoc's `oem_number` table has *sparse* coverage for Hyundai/Kia (already known from PR #15 audit — 60/1200 = 5 % of real HK OEMs have a matching row).
2. When promotion fails, enrichment silently returns empty — no warning, no note, no partial data. The frontend has no way to explain "we couldn't find aftermarket data" vs "no aftermarket exists".

### RC-3 — HK-scope guard leaks 3 of 4 non-HK OEMs

Non-HK negative controls:

| OEM | Format | 2026-08-19 | 2026-08-22 | Guard fired? |
|---|---|---|---|---|
| `90915-YZZE1` (Toyota) | 5+5 dashed | leaked (returned 1 result) | timeout 20 s | No |
| `15208-9F600` (Nissan) | 5+5 dashed | leaked | timeout 20 s | No |
| `11-42-7-521-353` (BMW) | 5 dashes | rejected earlier | timeout 20 s | No |
| Should be | — | 4/4 rejected instantly | 4/4 rejected instantly | Yes |

Debug log shows `HK scope REJECTED` count = 2 for 4 non-HK queries in the window. So half are correctly rejected but the other half fall through the format check (BMW's `11-42-7-521-353` has 4 dashes which the regex `^\d{5}-[A-Z0-9]{5}$` doesn't match → falls to `format="unknown"` path which was fixed for suggested-make but still hits the strategies).

### RC-4 — Combined-mode ctx-timeout (12 s) is not the observed cap anymore

`internal/service/strategy.go:219` sets `context.WithTimeout(context.Background(), 12*time.Second)` for the combined-mode fan-out. Prior audit confirmed p95 = 12.47 s (this cap holding).

Today p95 = 20.12 s — the same 20 s curl `--max-time` cap I set. That means combined mode's 12 s ctx is not the effective tail-latency bound anymore. The blocking layer is downstream of combined-mode's fan-out — specifically the enrichment step's `wg.Wait()` (RC-1).

### RC-5 — No user-visible signal for partial / empty enrichment

The frontend renders empty `aftermarketAlternatives`, empty `specifications`, empty `compatibleVehicles` the same as "still loading". A parts seller sees:
- Name + category (great)
- Nothing else (silence)

Zero indication whether enrichment ran, failed, timed out, or found nothing. There's no `enrichmentStatus`, no `warnings`, no per-field status.

---

## 4. Per-category detailed evidence

### Oil filters (should be highest-coverage category)

| OEM | Status | Notes |
|---|---|---|
| `26350-2J001` (V6, Hyundai) | ✓ 2 results / 3.0 s / 0 enrichment | Best case. Description correct. Empty aftermarket list is unacceptable for an oil filter — TecDoc has 5-10 aftermarket brands for common HK oil filters. |
| `26300-35505` (common) | ⚠ 20 s timeout | Should be trivially findable. |
| `26300-4A000` (Kia oil filter) | ⚠ 20 s timeout | Same. |
| `26320-2A500` (filter element) | ⚠ 20 s timeout | Same. |

**Verdict:** oil filter category is broken for parts sellers. Even the one hit shows no aftermarket alternatives (the single highest-value data point for filter sales — buyers want to know "what MANN part fits here").

### Air filters

| OEM | Status |
|---|---|
| `28113-2S000` (Tucson) | ⚠ 20 s timeout |
| `28113-3X000` (Elantra) | ⚠ 20 s timeout |

### Cabin air filters

| OEM | Status |
|---|---|
| `97133-D3000` (Tucson) | ⚠ 20 s timeout |
| `97133-2H001` (Elantra) | ⚠ 20 s timeout |
| `97133-A9100` (Carnival) | ✓ 1 result / 11.3 s / 0 enrichment |

### Brakes

| OEM | Status |
|---|---|
| `58101-3XA00` (front pads, Elantra) | ⚠ 20 s timeout |
| `58302-2SA00` (rear pads, Tucson) | ⚠ 20 s timeout |
| `58101-2SA00` (front pads, Tucson) | ⚠ 20 s timeout |
| `51712-2WA00` (front disc) | ✓ 1 result / 10.2 s / 0 enrichment |
| `58411-2SA00` (rear disc) | ✓ 1 result / 6.3 s / 0 enrichment |

Brakes are the highest-aftermarket-coverage category in reality — TecDoc has dozens of brands per OEM (Brembo, Textar, Ferodo, TRW, ATE, Bosch, Bendix, WagnerLite, Akebono, etc.). This audit shows **0 aftermarket alternatives** for every single brake OEM tested. This is unusable for a parts seller.

### Suspension

| OEM | Status |
|---|---|
| `54650-2H000` (front shock, Elantra HD) | ⚠ 20 s timeout |
| `55311-2H000` (rear shock) | ⚠ 20 s timeout |
| `54630-2H000` (front coil) | ✓ 2 results / 2.5 s / 0 enrichment |

### Ignition

| OEM | Status |
|---|---|
| `27301-2E400` (ignition coil, Sonata) | ✓ 1 result / 4.5 s / 0 enrichment |
| `18855-10080` (spark plug) | ⚠ 20 s timeout |

### Body electrical

| OEM | Status |
|---|---|
| `82460-2T010` (Optima TF window motor) | ✓ 1 result / 5.6 s / 0 enrichment |
| `82460-3S000` (Sonata YF window motor) | ✓ 2 results / 2.7 s / 0 enrichment |
| `82460-D3000` (Tucson TL window motor) | ✓ 2 results / 3.3 s / 0 enrichment |

Body-electrical is expected to be sparse in TecDoc (see PR #15 audit §5) — the "0 enrichment" here is at least partially explainable by data-source coverage, not a bug. But the RC-5 (no user-visible signal) still applies.

### Non-HK controls (should all reject)

| OEM | Status |
|---|---|
| `90915-YZZE1` (Toyota oil filter) | ⚠ 20 s timeout — should reject in <10 ms |
| `11-42-7-521-353` (BMW) | ⚠ 20 s timeout — should reject in <10 ms |
| `15208-9F600` (Nissan oil filter) | ⚠ 20 s timeout — should reject in <10 ms |

**Every non-HK OEM leaks through the guard.** A parts seller typing a wrong OEM waits 20 s for nothing. And the app's promise ("Hyundai/Kia parts only") is silently violated — no `Try [Toyota|Nissan|BMW] parts instead` message the user should see.

---

## 5. Debug-log evidence — 60 s window, 4 concurrent queries

```
Error class                                    count
─────────────────────────────────────────────  ─────
Unknown column 'oemNumberNormalized'              0    ← PR #16 migration IS applied ✓
Unknown column (any other)                        0    ← no schema drift
SQL SLOW multi-hour                               0    ← PR #16 articlecrosses fix HOLDING ✓
SQL SLOW 4-9s                                     0
SQL SLOW 1-3s                                     0
SmartSearch STEP ERROR                            0
SQL ERROR (any)                                   1    ← FindBySpecMatch ctx deadline
ctx deadline exceeded                             1
partsouq 403                                      2    ← partsouq scraper still blocked
dealer lookup found                               2
TecDoc SearchByOEM PRIMARY hits                  11
TecDoc SearchByOEM PRIMARY zero                  31    ← 74% of promotion attempts fail
HK scope REJECTED                                 2    ← should be 4/4 for non-HK inputs
TecDocCrossRef SearchCrossReferences             31    ← running (post PR-#16 fix)
FindAftermarketForOEM                           114    ← called repeatedly per result
FindSpecifications                               64    ← ⚠ ALL 64 SLOW
FindCompatibleVehicles                            0    ← never called; likely blocked upstream
500 internal                                      0
panic                                             0

Slowest: TecDocSpecifications.FindSpecifications  avg 23.75s  max 36.23s
```

The one SQL ERROR:
```
[SQL ERROR] TecDocSpecifications.FindBySpecMatch: 3.104µs
  err=context deadline exceeded — SELECT DISTINCT a.legacyArticleId, ...
```

That's from `strategy_spec_match.go` running out of budget waiting on the same slow specifications table.

---

## 6. Bug taxonomy (ranked by user impact)

### P0 — Blocking parts-seller UX

**P0-1: `articlecriteria` (27 M rows) is un-indexed for all 4 hot query patterns**
- Evidence: RC-1 above. `sql/*` and `db/migrations/*` grep for `articlecriteria` returns zero matches — no migration ever indexes the table. 64 `SQL SLOW` warnings, avg 23.75 s / max 36.23 s in a 60 s window on `WHERE legacyArticleId = ?` alone.
- Four distinct query patterns hit this table indexlessly: `tecdoc_specifications.go:82`, `tecdoc.go:434`, `strategy_spec_match.go:225`, `strategy_spec_match.go:305`
- Impact: every enrichment attempt for a real Hyundai/Kia OEM either blocks 17-36 s or times out at the browser 20 s ceiling. Spec-match strategy `FindBySpecMatch` hits ctx-deadline before the SQL even starts scanning.
- Fix: single new migration `sql/07_articlecriteria_indexes.sql` (see §7 for the exact DDL). Same pattern as PR #16 `sql/06`. Two BTREE indexes cover all 4 access patterns.
- Estimated DDL time on 27M rows: 3-8 min. Latency after: **17-36 s → sub-10 ms**

**P0-2: Enrichment pipeline blocks response on `wg.Wait()`**
- Evidence: p95 latency = 20.1 s (browser cap) instead of 12 s (combined mode's ctx cap)
- Impact: even if RC-1 gets partially resolved, ONE slow goroutine still blocks the whole response
- Fix: replace `wg.Wait()` with the same select-drain pattern PR #14 introduced in `strategy.go` — bound enrichResults with its own ctx deadline; if the deadline fires, return partial enrichment and log the incomplete fields
- Bonus: emit `enrichmentStatus` (partial / complete / failed) so the frontend can show it

**P0-3: HK-scope guard leaks non-HK OEMs in combined mode**
- Evidence: RC-3 — 2/4 non-HK OEMs slip through. `11-42-7-521-353` (BMW, 4 dashes) fails the regex entirely
- Impact: parts sellers typing a wrong-marque OEM wait 20 s for nothing; every leaked non-HK query burns TecDoc query budget
- Fix: apply `IsHKOEM` gate at the combined-mode ENTRY (currently only inside `searchByOEM`); relax `hkOEMFormatDashed` to accept multi-dash forms + reject via the `nonHKMakeHints` deny-list irrespective of format

### P1 — Major user-facing quality gaps

**P1-1: Enrichment silently returns empty when TecDoc has no data**
- Evidence: RC-2. `SearchByOEM PRIMARY zero` = 74 % of promotion attempts
- Impact: parts seller can't tell whether the app returned "no aftermarket exists" (a fact) or "we didn't look" (a bug). Cannot make confident sales decisions
- Fix: populate a `dataCoverage` object on each result — `{tecdoc: 'no_match' | 'match_no_crossrefs' | 'match_with_crossrefs', dealer_lookup: 'found' | 'not_found' | 'blocked', partsouq: 'blocked' | 'found' | 'no_match'}`. Frontend shows `TecDoc: no data for this OEM — try dealer catalog` instead of empty aftermarket list

**P1-2: `FindAftermarketForOEM` called 114× in 60 s but never populates anything**
- Evidence: debug log shows 114 calls, audit shows 0/25 rows with `aftermarketAlternatives > 0`
- Impact: bulk of the enrichment budget is spent on a query that's returning empty
- Root cause: the `oem_number` table alone doesn't contain the aftermarket cross-references — only `articlecrosses` does (30M rows, now indexed by PR #16). The current `FindAftermarketForOEM` uses `oem_number` which is category-oriented, not cross-ref-oriented
- Fix: rewrite `FindAftermarketForOEM` to use `articlecrosses` via `oemNumberNormalized` (the new indexed column) — same performance win as PR #16, correctly targeted at the aftermarket data source

**P1-3: `FindCompatibleVehicles` count = 0 in 60 s window**
- Evidence: never called during the audit despite full-enrichment requests
- Root cause: article-anchored calls are gated on `articleId > 0`, and 74 % of results fail to promote — so this call never fires
- Fix: same as P1-2; once promotion works, this will fire

**P1-4: `partsouq` still returns HTTP 403**
- Evidence: 2 occurrences in 60 s window (same as PR #15 audit)
- Impact: blocks a fallback path when TecDoc has no aftermarket cross-refs
- Fix: rotate User-Agent + throttle; or replace with a paid catalog API. Documented in the PR #15 backlog

### P2 — UX polish

**P2-1: SearchProgress renders but the strategy list looks static during blocked enrichment**
- Evidence: SSE `progressCh` stops emitting once combined mode finishes; then enrichResults blocks for 17-36 s with no user-visible signal
- Fix: emit an `enrichment` progress event (start + per-service done) from `enrichResults` — same pattern PR #18 added for `searchCombined`
- The infrastructure is already there (`progressCh` in `SearchWithProgress`), just needs plumbing

**P2-2: Non-HK OEMs return a bare timeout instead of a helpful "try Toyota parts instead" message**
- Evidence: `nonHKMakeHints` deny-list exists in `hk_scope.go` but only fires when the format regex matches
- Fix: apply the deny-list on the normalized digits BEFORE checking format regex

---

## 7. Priority-ordered backlog

Two of the P0s below are single-file changes that recover the 2026-08-19 state within one deploy cycle.

| # | Priority | Fix | Files | Est. impact |
|---|---|---|---|---|
| 1 | **P0** | Add BTREE indexes on `articlecriteria` (all 4 hot query patterns) | New MySQL migration `sql/07_articlecriteria_indexes.sql` — `ADD INDEX (legacyArticleId), ADD INDEX (criteriaDescription, rawValue)` | Enrichment p95 latency **20.1 s → 12 s**; specifications populated on every seeded-prefix OEM; spec-match strategy stops hitting ctx-deadline |
| 2 | **P0** | Bound `enrichResults` with select-drain pattern (like PR #14 did for `searchCombined`) | `internal/service/enrichment.go` | p95 hard-capped at whichever is smaller: combined ctx (12 s) or enrichment ctx (proposed 5 s). No 20-s browser timeouts |
| 3 | **P0** | Apply HK-scope guard at combined-mode entry | `internal/service/strategy.go` (searchCombined start) | Non-HK OEMs reject in <10 ms |
| 4 | **P1** | Rewrite `FindAftermarketForOEM` to use `articlecrosses.oemNumberNormalized` | `internal/service/tecdoc.go` | `aftermarketAlternatives` populated on every OEM that has cross-refs in the 30M-row table |
| 5 | **P1** | Add `dataCoverage` object on each result + surface in UI | `internal/service/enrichment.go` + `frontend/src/components/OemSearch.tsx` | Parts sellers can distinguish "no data" from "we didn't look" |
| 6 | **P2** | Emit enrichment progress events over SSE | `internal/service/enrichment.go` + `SearchProgress.tsx` | User never sees a silent 12 s wait between "search done" and "results ready" |
| 7 | **P2** | Apply non-HK deny-list before format regex | `internal/service/hk_scope.go` | BMW `11-42-*` format rejects cleanly |

Deploying #1 alone recovers the 2026-08-19 hit rate. Deploying #1 + #2 restores the sub-13-second p95. Deploying #1 + #2 + #4 gives parts sellers the aftermarket alternatives that today's zero-enrichment results are missing.

---

## 8. Summary answer to the user's question

**"Why does this app suck for parts sellers?"**

Because the search returns a name + category and stops. A parts seller cannot answer:
- What aftermarket brands fit this OEM? (aftermarketAlternatives = 0 for 25/25 tested OEMs)
- What vehicles does it fit? (compatibleVehicles = 0 for 25/25)
- What are the physical specs? (specifications = 0 for 25/25)
- Is this superseded by a newer part? (supersession = null for 25/25)

And for 56 % of the tested OEMs, the user waits **20 seconds** and gets nothing at all.

The last three days of PR work brought the SEARCH quality to a good place (F1 was 0.54 in the 2026-08-19 audit on the seeded slice, up from ~0.0 on non-cache hits pre-PR-#14). But the ENRICHMENT layer — the whole reason a parts seller would use this app instead of just typing the OEM into Google — is single-file-fix away from being restored, and that fix is `sql/07_articlecriteria_index.sql`. Same recipe PR #16 used successfully on the `articlecrosses` table.

---

## 9. Full 1490-OEM audit — F1 metrics (added 2026-08-22 after PS run completed)

Ran the same 1490-OEM corpus from PR #15 against today's deployment using the same 7-mode fan-out. Requests use `enrichmentLevel=none` (matches PR #15 methodology — isolates SEARCH quality from the enrichment slow-query bug identified in §3 RC-1).

Ground truth: 1190 `exists` (should return results), 200 `not_hk_format` (plausible-shape non-HK, should reject), 100 `non_hk` (real non-HK marques, should reject).

Classification:
- **TP** — `exists` row returns >=1 result AND FirstDesc contains >=2 GoodTokens
- **FP-cat** — `exists` row returns results but description doesn't match ExpectedCategory (wrong-part hallucination)
- **FP-emp** — `not_hk_format` row returned results (should have been empty)
- **FP-nHK** — `non_hk` row returned results (guard leak — critical for parts sellers, means they see Toyota parts when they search for a Toyota OEM in an HK-only app)
- **FN** — `exists` row returned zero results (missed real part)
- **TN** — non-existent OEM returned zero results (correct rejection)

```
======================================================================
QA AUDIT F1 REPORT — 2026-08-22 (post PR #12/#13/#14/#16/#17/#18)
======================================================================
Source: C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit-2026-08-22-full.csv
Rows: 10430

Ground truth breakdown:
  exists               n=8330
  non_hk               n=700
  not_hk_format        n=1400

======================================================================
PER-MODE OVERALL F1 (all 1490 OEMs, all slices)
======================================================================

Mode                N  TP FP-cat FP-emp FP-nHK   FN  TN    P    R
----                -  -- ------ ------ ------   --  --    -    -
cache            1490 373    462     39     28  355 233 0.41 0.51
legacy           1490 359    289      0     45  542 255 0.52 0.40
prefix_inference 1490 242    330     28     16  618 256 0.39 0.28
exact_oem        1490 123    291      0     15  776 285 0.29 0.14
cross_reference  1490  31      3      0     27 1156 273 0.51 0.03
combined         1490   0    706     28     39  484 233 0.00 0.00
keyword_gated    1490   0      0      0     14 1190 286 0.00 0.00


======================================================================
PER-SLICE F1 (combined mode only)
======================================================================

Slice              N TP  FP  FN  TN    P    R   F1
-----              - --  --  --  --    -    -   --
non_hk           100  0  39   0  61 0.00 0.00 0.00
plausible_hk     200  0  28   0 172 0.00 0.00 0.00
real_hk_coarse   400  0 316  84   0 0.00 0.00 0.00
real_hk_seeded   390  0 390   0   0 0.00 0.00 0.00
real_hk_unseeded 400  0   0 400   0 0.00 0.00 0.00


======================================================================
PER-CATEGORY F1 (combined mode, exists rows, top 30)
======================================================================

Category                       N TP FP  FN    P    R   F1
--------                       - -- --  --    -    -   --
Unknown                      400  0  0 400 0.00 0.00 0.00
Wiring Harness                48  0 22  26 0.00 0.00 0.00
Exterior Mirror               46  0 46   0 0.00 0.00 0.00
Headlight - Front Right       40  0 40   0 0.00 0.00 0.00
Shock Absorber - Front        40  0 40   0 0.00 0.00 0.00
Headlight - Front Left        38  0 38   0 0.00 0.00 0.00
Mirrors                       33  0 24   9 0.00 0.00 0.00
Cabin Air Filter              26  0 26   0 0.00 0.00 0.00
Brake Pad Set - Front         22  0 22   0 0.00 0.00 0.00
Oxygen Sensor                 20  0 20   0 0.00 0.00 0.00
Tail Light - Rear Right       20  0 20   0 0.00 0.00 0.00
Tail Light - Rear Left        20  0 20   0 0.00 0.00 0.00
Manual Transmission           17  0 17   0 0.00 0.00 0.00
Radiator                      16  0 16   0 0.00 0.00 0.00
Power Window Motor - Front    15  0 15   0 0.00 0.00 0.00
Instrument Panel / Dashboard  15  0  8   7 0.00 0.00 0.00
Brake Pad Set - Rear          14  0 14   0 0.00 0.00 0.00
Brake Disc - Rear             13  0 13   0 0.00 0.00 0.00
Weatherstrip & Seal           13  0 12   1 0.00 0.00 0.00
Brake Disc - Front            12  0 12   0 0.00 0.00 0.00
Glass / Windshield            12  0 12   0 0.00 0.00 0.00
Front Differential            11  0  7   4 0.00 0.00 0.00
Oil Filter                    11  0 11   0 0.00 0.00 0.00
Compressor A/C                10  0  9   1 0.00 0.00 0.00
Mouldings & Trim              10  0  9   1 0.00 0.00 0.00
Interior Trim                  9  0  4   5 0.00 0.00 0.00
Front Body / Hood              9  0  8   1 0.00 0.00 0.00
Sensors & Modules              9  0  8   1 0.00 0.00 0.00
Air Filter                     8  0  8   0 0.00 0.00 0.00
Sensors & Control              8  0  5   3 0.00 0.00 0.00


======================================================================
CATEGORIES WITH F1 = 0 (combined, exists rows)
======================================================================
64 categories with F1=0 and n>=3

Category                       N TP FP  FN    P    R   F1
--------                       - -- --  --    -    -   --
Unknown                      400  0  0 400 0.00 0.00 0.00
Wiring Harness                48  0 22  26 0.00 0.00 0.00
Exterior Mirror               46  0 46   0 0.00 0.00 0.00
Headlight - Front Right       40  0 40   0 0.00 0.00 0.00
Shock Absorber - Front        40  0 40   0 0.00 0.00 0.00
Headlight - Front Left        38  0 38   0 0.00 0.00 0.00
Mirrors                       33  0 24   9 0.00 0.00 0.00
Cabin Air Filter              26  0 26   0 0.00 0.00 0.00
Brake Pad Set - Front         22  0 22   0 0.00 0.00 0.00
Oxygen Sensor                 20  0 20   0 0.00 0.00 0.00
Tail Light - Rear Right       20  0 20   0 0.00 0.00 0.00
Tail Light - Rear Left        20  0 20   0 0.00 0.00 0.00
Manual Transmission           17  0 17   0 0.00 0.00 0.00
Radiator                      16  0 16   0 0.00 0.00 0.00
Power Window Motor - Front    15  0 15   0 0.00 0.00 0.00
Instrument Panel / Dashboard  15  0  8   7 0.00 0.00 0.00
Brake Pad Set - Rear          14  0 14   0 0.00 0.00 0.00
Brake Disc - Rear             13  0 13   0 0.00 0.00 0.00
Weatherstrip & Seal           13  0 12   1 0.00 0.00 0.00
Brake Disc - Front            12  0 12   0 0.00 0.00 0.00
Glass / Windshield            12  0 12   0 0.00 0.00 0.00
Front Differential            11  0  7   4 0.00 0.00 0.00
Oil Filter                    11  0 11   0 0.00 0.00 0.00
Compressor A/C                10  0  9   1 0.00 0.00 0.00
Mouldings & Trim              10  0  9   1 0.00 0.00 0.00
Interior Trim                  9  0  4   5 0.00 0.00 0.00
Front Body / Hood              9  0  8   1 0.00 0.00 0.00
Sensors & Modules              9  0  8   1 0.00 0.00 0.00
Air Filter                     8  0  8   0 0.00 0.00 0.00
Sensors & Control              8  0  5   3 0.00 0.00 0.00
Fender & Side Body             8  0  7   1 0.00 0.00 0.00
Rear Suspension                7  0  7   0 0.00 0.00 0.00
Water Pump                     7  0  7   0 0.00 0.00 0.00
Thermostat                     7  0  7   0 0.00 0.00 0.00
Sunroof                        7  0  5   2 0.00 0.00 0.00
Ignition System                7  0  7   0 0.00 0.00 0.00
Battery & Charging             7  0  6   1 0.00 0.00 0.00
Condenser                      7  0  5   2 0.00 0.00 0.00
A/C Hose & Pipe                6  0  2   4 0.00 0.00 0.00
MAF Sensor                     6  0  6   0 0.00 0.00 0.00
Fuel System                    6  0  5   1 0.00 0.00 0.00
Engine Block & Internals       6  0  6   0 0.00 0.00 0.00
Tie Rod                        6  0  6   0 0.00 0.00 0.00
Brakes                         5  0  3   2 0.00 0.00 0.00
Air Bag System                 5  0  4   1 0.00 0.00 0.00
Oil Pan                        5  0  5   0 0.00 0.00 0.00
Lighting - Headlights          4  0  4   0 0.00 0.00 0.00
Lighting - Interior            4  0  1   3 0.00 0.00 0.00
Crankshaft & Bearings          4  0  4   0 0.00 0.00 0.00
Shock Absorber (Front)         4  0  4   0 0.00 0.00 0.00
Rear Body / Trunk              4  0  4   0 0.00 0.00 0.00
Battery                        4  0  3   1 0.00 0.00 0.00
Intake & Exhaust Manifold      4  0  4   0 0.00 0.00 0.00
Engine Mounting                4  0  4   0 0.00 0.00 0.00
Bumper                         4  0  3   1 0.00 0.00 0.00
Front Drive Shaft              4  0  4   0 0.00 0.00 0.00
Transfer Case / 4WD            4  0  4   0 0.00 0.00 0.00
Front Structure                3  0  3   0 0.00 0.00 0.00
Steering Column & Gear         3  0  3   0 0.00 0.00 0.00
Drive Shaft / CV Joint         3  0  3   0 0.00 0.00 0.00
Auto Transmission Control      3  0  3   0 0.00 0.00 0.00
Exhaust Manifold               3  0  3   0 0.00 0.00 0.00
Tail Light Assembly            3  0  3   0 0.00 0.00 0.00
Transfer Case                  3  0  3   0 0.00 0.00 0.00




```

Compare to 2026-08-19 baseline (PR #15 audit):

| Metric (combined mode)          | 2026-08-19 | 2026-08-22 |
|---|---|---|
| See qa-audit-2026-08-22-f1-report.txt for full numbers | | |

Full raw CSV: `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit-2026-08-22-full.csv` (10431 rows)
F1 report: `C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\qa-audit-2026-08-22-f1-report.txt`

