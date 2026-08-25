# M1 Sprint Backlog — Correctness first (F1_correct ≥ 0.95)

Every task is a self-contained agent brief. Copy-paste any task ID (e.g. `M1.S1.T1`) into a fresh agent chat + the current codebase and it should have enough context to execute.

Milestone exit gate: `F1_correct ≥ 0.95` overall and `≥ 0.98` on the seeded slice. Baseline: 0.30 and 0.71.

---

## Sprint M1.S1 — Ranker cross-family penalty

### Task M1.S1.T1 — Introduce `strategyCategoryPenalty` helper

**Goal.** Add a helper that returns a confidence multiplier when a search result's inferred category-system differs from the OEM prefix's expected system. Preserves the hit but sinks it below same-system matches during ranking.

**Files to touch:**
- `internal/service/oem_prefix.go` — expose or extend the existing `DecodeOEMPrefix` so callers can get `.System` cheaply
- `internal/service/strategy.go` — add the helper near `strategyPriority`
- `internal/service/strategy_test.go` — regression tests

**Approach:**
1. Read `internal/service/oem_prefix.go`. Confirm `DecodeOEMPrefix(oem) *OEMCategory` returns a struct with `System` field. It already does.
2. Add a helper in `strategy.go`:
   ```go
   // strategyCategoryPenalty returns a multiplier in (0, 1] to apply to a
   // result's confidence when the queried OEM prefix decodes to a system
   // that DOES NOT match the returned category's parent system.
   //
   // Returns 1.0 (no penalty) when:
   //   - the queried OEM does not decode (partial stem, non-HK)
   //   - the result has no category or its category-system matches
   //     the queried system
   //
   // Returns 0.2 for cross-family mismatches (mirror OEM returning a
   // headlight — the search still surfaces the hit for observability
   // but rank-drops it below any same-system alternative).
   func strategyCategoryPenalty(queryOEM string, resultCategory string) float64 {
       queried := DecodeOEMPrefix(queryOEM)
       if queried == nil || queried.System == "" {
           return 1.0
       }
       // Map the result's category string to a system by looking up which
       // prefixMap entry has that Category. Cache the map.
       resultSystem := categoryToSystem(resultCategory)
       if resultSystem == "" || resultSystem == queried.System {
           return 1.0
       }
       return 0.2
   }
   ```
3. Add the reverse lookup `categoryToSystem` built once at package init from `prefixMap`:
   ```go
   var categorySystemMap = func() map[string]string {
       m := make(map[string]string, 64)
       for _, cat := range prefixMap {
           m[cat.Category] = cat.System
       }
       return m
   }()

   func categoryToSystem(category string) string {
       return categorySystemMap[category]
   }
   ```

**Acceptance criteria:**
- [ ] Unit test `TestStrategyCategoryPenalty_CrossFamily` in `strategy_test.go` covers ≥ 4 known collisions from `docs/reports/2026-08-23-quality-audit/failures.csv` (grep for `wrong-part` failures where `FirstCategory` differs from the ExpectedCategory's system). Each returns `0.2`.
- [ ] Unit test `TestStrategyCategoryPenalty_SameFamily` covers 4 same-family cases (Brake Pad category on `58*` prefix — same "Brakes" system). Each returns `1.0`.
- [ ] Unit test `TestStrategyCategoryPenalty_UnknownPrefix` covers 3 non-decoding inputs (`""`, `"XYZ"`, `"999"`). Each returns `1.0`.
- [ ] `go test ./internal/service/` all passes.

**Effort:** M

**Dependencies:** none

**PR title suggestion:** `feat(ranker): strategyCategoryPenalty for cross-family mismatches`

---

### Task M1.S1.T2 — Apply the penalty in `searchCombined` merge

**Goal.** Wire `strategyCategoryPenalty` into the search-result ranking so cross-family results still surface but sort last. Only apply to fallback strategies (`cache`, `legacy`, `prefix_inference`) — the strategies most likely to surface wrong-family results.

**Files to touch:**
- `internal/service/strategy.go` — `searchCombined`, specifically the dedupe/rank block after the collect loop
- `internal/service/strategy_test.go` — new test

**Approach:**
1. In `searchCombined`, after the `sort.Slice(results, …)` block at line ~380, apply the penalty **before** sorting so it affects the sort order. Pseudo-code:
   ```go
   for i := range results {
       primary := firstStrategyOf(results[i].SourceStrategy) // "cache" from "cache,prefix_inference"
       fallbackStrategy := primary == "cache" || primary == "legacy" || primary == "prefix_inference"
       if fallbackStrategy {
           penalty := strategyCategoryPenalty(req.OEM, results[i].Category)
           results[i].Confidence *= penalty
           if penalty < 1.0 {
               log.Printf("[Combined] category-penalty applied oem=%q result_cat=%q penalty=%.2f",
                   req.OEM, results[i].Category, penalty)
           }
       }
   }
   ```
2. Then run the existing sort.

**Acceptance criteria:**
- [ ] Unit test `TestSearchCombined_CrossFamilyResultDemoted` — synthesise two mock results for the same OEM query (`86391-D3000`, prefix=`86` → Body/Mirrors):
  - Result A: `Category=Mirrors`, `SourceStrategy=cache`, `Confidence=0.9`
  - Result B: `Category=Headlight Assembly`, `SourceStrategy=cache`, `Confidence=0.95`
  - Post-search, result A sorts FIRST despite lower base confidence.
- [ ] Re-run `scripts/audit/audit-quality.ps1` against dev, verify `F1_correct` on `real_hk_coarse` slice climbs from 0.04 to ≥ 0.30. Attach the diff to the PR.

**Effort:** S

**Dependencies:** M1.S1.T1

**Log-line contract:** the `[Combined] category-penalty applied` log line is used by the audit script's `wrong_category_dropped_by_penalty_pct` diagnostic (added in M1.S3.T3).

---

### Task M1.S1.T3 — Same-system tiebreak in the sort

**Goal.** When two results have equal `Confidence * priority`, prefer the one whose category-system matches the queried prefix.

**Files to touch:**
- `internal/service/strategy.go` — the `sort.Slice` less function

**Approach:**
1. Extend the less function:
   ```go
   sort.Slice(results, func(i, j int) bool {
       pi := strategyPriority(results[i].SourceStrategy, priorities)
       pj := strategyPriority(results[j].SourceStrategy, priorities)
       si := results[i].Confidence * pi
       sj := results[j].Confidence * pj
       if si != sj {
           return si > sj
       }
       // Tiebreak: prefer same-system match to the queried OEM
       if req.OEM != "" {
           queried := DecodeOEMPrefix(req.OEM)
           if queried != nil {
               iSys := categoryToSystem(results[i].Category)
               jSys := categoryToSystem(results[j].Category)
               iMatch := iSys == queried.System
               jMatch := jSys == queried.System
               if iMatch != jMatch {
                   return iMatch
               }
           }
       }
       return false
   })
   ```

**Acceptance criteria:**
- [ ] Unit test `TestSearchCombined_TiebreakSameSystem` — two results with identical score, one same-system, one cross-system. Same-system wins.
- [ ] No regression in existing tests.

**Effort:** S

**Dependencies:** M1.S1.T1

---

## Sprint M1.S2 — Non-HK deny-list widening + strict format gate

### Task M1.S2.T1 — Reorder `IsHKOEM` predicate: deny-list first

**Goal.** Currently the `nonHKMakeHints` deny-list only runs when format is unknown OR when format matches but prefix doesn't decode. Some non-HK OEMs (Ford `AL3Z-*`, Peugeot `9803*`) fail the regex and don't decode → they can still slip through `searchCombined` via the `SuggestedMake != ""` branch, but there are gaps. Widen the deny-list and check it FIRST.

**Files to touch:**
- `internal/service/hk_scope.go` — reorder `IsHKOEM`
- `internal/service/hk_scope_test.go` — new regression tests
- Optionally: `internal/service/nonhk_denylist.go` (extract if the deny-list is going to grow ≥ 100 entries)

**Approach:**
1. Move the `nonHKMakeHints` check to the top of `IsHKOEM`, right after `NormalizeOEM`. If any entry matches, return `IsHK=false + SuggestedMake` immediately.
2. Expand the deny-list — target ≥ 100 entries covering:
   - Ford: `AL3Z`, `BR3Z`, `EL3Z`, `F5EX`, `9L`
   - GM/Chevy: `12345678`-style, `19*`, `88970680`-family
   - Peugeot/Citroen: `9803*`, `9804*`
   - Renault/Dacia: `77 00 *`, `8200*`
   - Mitsubishi: `MD*`, `MR*`, `MB*`
   - Mazda: `LF*`, `KL*`, `RF*`
   - Fiat/Jeep/Chrysler: `680*`, `MS-*`
   - Additional Toyota patterns beyond 90915
   - Additional BMW patterns beyond 11-42
   - Additional Nissan patterns beyond 15208 / 22448
3. Extract into `nonhk_denylist.go` if it exceeds ~30 lines.

**Acceptance criteria:**
- [ ] Deny-list has ≥ 100 entries.
- [ ] Every prefix in `hk_scope_test.go:TestIsHKOEM_MultiDashNonHK_ViaDenyList` still returns `IsHK=false + SuggestedMake`.
- [ ] New test `TestIsHKOEM_ExpandedDenyList` covers 20 new deny-list entries.
- [ ] Audit script's non_hk slice leaks drop from 38 to ≤ 2. Attach re-run diff.

**Effort:** M

**Dependencies:** none

---

### Task M1.S2.T2 — Confidence floor for cache-only results

**Goal.** A single cache-only hit with confidence < 0.5 and no corroboration from another strategy is almost always stale garbage. Drop it.

**Files to touch:**
- `internal/service/strategy.go` — final filter in `searchCombined` before sort

**Approach:**
1. Before the sort, filter:
   ```go
   filtered := results[:0]
   for _, r := range results {
       primary := firstStrategyOf(r.SourceStrategy)
       isSoloCache := primary == "cache" && !strings.Contains(r.SourceStrategy, ",")
       if isSoloCache && r.Confidence < 0.5 {
           log.Printf("[Combined] dropping solo-cache low-conf hit oem=%q conf=%.2f desc=%q",
               req.OEM, r.Confidence, r.Description)
           continue
       }
       filtered = append(filtered, r)
   }
   results = filtered
   ```

**Acceptance criteria:**
- [ ] Unit test `TestSearchCombined_DropsSoloCacheLowConf` synthesises a solo-cache result at confidence 0.3; it's dropped.
- [ ] Unit test `TestSearchCombined_KeepsSoloCacheHighConf` synthesises a solo-cache result at confidence 0.9; it's kept.
- [ ] Unit test `TestSearchCombined_KeepsMultiStrategyLowConf` synthesises `cache,legacy` at confidence 0.3; kept.
- [ ] Audit re-run: `F1_correct` overall should climb by ≥ 0.05 (fewer FP-cat rows).

**Effort:** S

**Dependencies:** none

---

## Sprint M1.S3 — Category-consistency validation

### Task M1.S3.T1 — Build `categoryTokens[prefix]` reverse index

**Goal.** For every prefix in `oem_prefix.go`, precompute the list of description tokens that a valid result's description SHOULD contain (extracted from the `OEMCategory.Category` label).

**Files to touch:**
- New `internal/service/category_tokens.go`
- New `internal/service/category_tokens_test.go`

**Approach:**
1. Build a package-level map at init time:
   ```go
   // categoryTokensMap maps every prefix in prefixMap to the set of
   // lower-case tokens that a valid description for that prefix SHOULD
   // contain. Extracted from OEMCategory.Category with common stopwords
   // stripped (a, the, of, /, -, &, and).
   var categoryTokensMap = func() map[string][]string {
       stop := map[string]bool{"a":true,"an":true,"the":true,"of":true,"and":true,"or":true}
       out := make(map[string][]string, len(prefixMap))
       for prefix, cat := range prefixMap {
           text := strings.ToLower(cat.Category)
           text = strings.NewReplacer("/", " ", "-", " ", "&", " ", "(", " ", ")", " ").Replace(text)
           toks := strings.Fields(text)
           filtered := toks[:0]
           for _, t := range toks {
               if len(t) < 2 || stop[t] { continue }
               filtered = append(filtered, t)
           }
           out[prefix] = filtered
       }
       return out
   }()

   // CategoryTokensForOEM returns the tokens a valid description should
   // contain for the given OEM number. Returns nil when the prefix does
   // not decode.
   func CategoryTokensForOEM(oem string) []string {
       cat := DecodeOEMPrefix(oem)
       if cat == nil { return nil }
       // Prefer longest-prefix match — check 3-digit then 2-digit
       if len(cat.Prefix) >= 3 {
           if toks, ok := categoryTokensMap[cat.Prefix]; ok { return toks }
       }
       return categoryTokensMap[cat.Prefix]
   }
   ```

**Acceptance criteria:**
- [ ] `CategoryTokensForOEM("26350-2J001")` returns tokens including `oil`, `filter` (from `263 → "Oil Filter"`)
- [ ] `CategoryTokensForOEM("58101-3XA00")` returns tokens including `brake`, `pad` (from `581 → "Front Brake Pad / Disc"`)
- [ ] `CategoryTokensForOEM("86391-D3000")` returns tokens including `mirror` (from `86 → "Mirrors"`)
- [ ] `CategoryTokensForOEM("XYZ")` returns `nil`
- [ ] Table-driven test covers ≥ 20 known-good mappings.

**Effort:** M

**Dependencies:** none

---

### Task M1.S3.T2 — Drop results with zero category-token overlap

**Goal.** In `searchCombined`, after ranking, drop any result whose `Description` contains ZERO tokens from `CategoryTokensForOEM(queried)`. Emits a warning for observability.

**Files to touch:**
- `internal/service/strategy.go`
- `internal/service/strategy_test.go`

**Approach:**
1. After the confidence-floor filter from M1.S2.T2, add:
   ```go
   if req.OEM != "" {
       expectedTokens := CategoryTokensForOEM(req.OEM)
       if len(expectedTokens) > 0 {
           kept := results[:0]
           for _, r := range results {
               descLower := strings.ToLower(r.Description)
               hasOverlap := false
               for _, tok := range expectedTokens {
                   if strings.Contains(descLower, tok) {
                       hasOverlap = true
                       break
                   }
               }
               if !hasOverlap {
                   log.Printf("[Combined] category-mismatch DROPPED oem=%q desc=%q expected_tokens=%v",
                       req.OEM, r.Description, expectedTokens)
                   continue
               }
               kept = append(kept, r)
           }
           results = kept
       }
   }
   ```

**Acceptance criteria:**
- [ ] Unit test `TestSearchCombined_DropsCategoryMismatch` — query `86391-D3000` (mirror), synthesise a result with `Description="HEADLIGHT ASSY - FRONT LH"`; it's dropped.
- [ ] Unit test `TestSearchCombined_KeepsCategoryMatch` — query `86391-D3000`, synthesise a result with `Description="OUTSIDE MIRROR ASSY LH"`; it's kept.
- [ ] Audit re-run: `F1_correct` on `real_hk_coarse` climbs from 0.04 to ≥ 0.60.

**Effort:** M

**Dependencies:** M1.S3.T1

**Log-line contract:** `[Combined] category-mismatch DROPPED` is scraped by the audit script's diagnostics (M1.S3.T3).

---

### Task M1.S3.T3 — Add diagnostic column to audit output

**Goal.** Extend `analyze-quality.ps1` so `by-category.csv` has a new column `wrong_cat_penalized_pct` — for each category, what percentage of the returned results were confidence-penalized or dropped by the M1.S1/M1.S3 guards.

**Files to touch:**
- `scripts/audit/audit-quality.ps1` — capture the `Warnings` from the response (already present) and count occurrences of `category-mismatch` / `category-penalty` markers
- `scripts/audit/analyze-quality.ps1` — surface as a new column

**Approach:**
1. The response already carries a `Warnings` array. When the guards fire, they add `"category-penalty applied"` or `"category-mismatch dropped N"` strings to `resp.Warnings`.
2. Update `audit-quality.ps1` line ~118 (Warnings capture) to also count how many warnings mention `category-`. Emit as a new column `CategoryGuardHits`.
3. In `analyze-quality.ps1`, sum per-category and compute a percentage of hits.

**Acceptance criteria:**
- [ ] New CSV column present in the by-category output.
- [ ] For known cross-family collision categories (from M1.S1.T2 fix), the percentage is nonzero.

**Effort:** S

**Dependencies:** M1.S1.T2 must emit the warning to `resp.Warnings` (add it in that task)

---

## Sprint exit criteria (whole M1 milestone)

- [ ] `F1_correct` overall ≥ 0.95 (from 0.30)
- [ ] `F1_correct` seeded slice ≥ 0.98 (from 0.71)
- [ ] `F1_correct` coarse slice ≥ 0.60 (from 0.04)
- [ ] Non-HK guard leaks ≤ 2 of 100 (from 38)
- [ ] 37 categories move above `F1_correct = 0.95` (fewer than 3 remain below)
- [ ] All new tests pass; existing tests untouched
- [ ] `pwsh scripts/audit/audit-quality.ps1` + `analyze-quality.ps1` re-run diff attached to the milestone-close PR

## Merge order for M1

1. M1.S1.T1 (helper) — merge first, no user-facing change
2. M1.S1.T2 (apply penalty) — merge; audit-diff shows small improvement
3. M1.S1.T3 (tiebreak) — merge; audit-diff shows further improvement
4. M1.S2.T1 (deny-list widening) — merge; non-HK leaks drop
5. M1.S2.T2 (confidence floor) — merge; F1_correct climbs
6. M1.S3.T1 (tokens map) — merge; no user-facing change
7. M1.S3.T2 (mismatch drop) — merge; F1_correct crosses 0.95
8. M1.S3.T3 (diagnostic) — merge; audit tooling updated

Each PR is ~50-150 lines of code + tests. Reviewable in an hour each.
