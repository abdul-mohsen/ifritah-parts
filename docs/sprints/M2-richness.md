# M2 Sprint Backlog — Rich alternatives (AvgAM_correct ≥ 5, F1_rich5 ≥ 0.60)

Milestone exit gate: on wear-parts categories (Brake Pad, Brake Disc, Oil Filter, Air Filter, Cabin Air Filter, Shock Absorber, Radiator, Water Pump, Ignition Coil, Oxygen Sensor):

- `AvgAM_correct ≥ 5` (currently 0.10-0.43)
- `AvgOEMxRef_correct ≥ 3` (currently 0.08-0.36)
- `AvgRepl_correct ≥ 8` total (currently 0.10-0.86)
- `F1_rich5 ≥ 0.60` (currently 0.00-0.09)

---

## Sprint M2.S1 — Multi-path aftermarket UNION

### Task M2.S1.T1 — `FindAftermarketForOEM_MultiPath`

**Goal.** Refactor `FindAftermarketForOEM` to run three lookup queries in parallel and union the deduped results.

**Files to touch:**
- `internal/service/tecdoc.go` — rewrite `FindAftermarketForOEM`
- New `internal/service/tecdoc_aftermarket_multipath_test.go`

**Approach:**
1. Read the current `FindAftermarketForOEM` (post PR #20 it queries `articlecrosses.oemNumberNormalized`). Change signature to internal `findAftermarketFromArticlecrosses(clean, ctx)`, `findAftermarketFromOemNumber(clean, ctx)`, `findAftermarketFromOemSearchIndex(clean, ctx)`.
2. Public `FindAftermarketForOEM(oemNumber)` fires all three in parallel goroutines, waits with a 3s ctx budget, merges results deduped by `(NormalizeBrand(brand), NormalizePartNumber(partNumber))`.
3. Each path can return `nil, err` independently — union what completed by the deadline.

**Sample structure:**
```go
func (t *TecDoc) FindAftermarketForOEM(oemNumber string) ([]model.AftermarketPart, error) {
    clean := NormalizeOEM(oemNumber)
    if clean == "" { return nil, nil }

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    type pathResult struct {
        parts []model.AftermarketPart
        err   error
        name  string
    }
    resultCh := make(chan pathResult, 3)

    go func() {
        p, err := t.findAftermarketFromArticlecrosses(ctx, clean)
        resultCh <- pathResult{p, err, "articlecrosses"}
    }()
    go func() {
        p, err := t.findAftermarketFromOemNumber(ctx, clean)
        resultCh <- pathResult{p, err, "oem_number"}
    }()
    go func() {
        p, err := t.findAftermarketFromOemSearchIndex(ctx, clean)
        resultCh <- pathResult{p, err, "oem_search_index"}
    }()

    seen := map[string]bool{}
    var out []model.AftermarketPart
    for i := 0; i < 3; i++ {
        select {
        case r := <-resultCh:
            if r.err != nil {
                log.Printf("[FindAftermarketForOEM] path=%s err=%v", r.name, r.err)
                continue
            }
            for _, p := range r.parts {
                key := NormalizeBrand(p.Brand) + "|" + strings.ToLower(p.PartNumber)
                if seen[key] { continue }
                seen[key] = true
                out = append(out, p)
            }
        case <-ctx.Done():
            log.Printf("[FindAftermarketForOEM] ctx deadline exceeded — returning %d partial", len(out))
            return out, nil
        }
    }
    return out, nil
}
```

**Acceptance criteria:**
- [ ] Unit test with three stub repos each returning distinct rows; merge returns union count.
- [ ] Unit test with one stub returning error; others succeed; union is preserved.
- [ ] Unit test with all stubs slow; ctx deadline drains partial results correctly.
- [ ] Audit re-run: `AvgAM_correct` climbs by ≥ 2 on wear-parts categories.

**Effort:** M

**Dependencies:** M2.S1.T3 (brand normalisation) — but can ship without it, deduping will just be less thorough

---

### Task M2.S1.T2 — Supersession-chain aftermarket

**Goal.** Extend `FindAftermarketForOEM` to ALSO query aftermarket for every OEM in the query's supersession chain. Widens the aftermarket net by including brands cataloged against parent/child OEMs.

**Files to touch:**
- `internal/service/tecdoc.go`
- `internal/service/tecdoc_supersession.go` (may need a helper to walk chain by OEM string not article id)

**Approach:**
1. First check: does `tecdoc_supersession.go` expose a "find superseded/successor OEMs by input OEM"? If not, add `FindSupersessionOEMs(cleanOEM) []string` that returns the chain (excluding the input itself, dedupe).
2. In `FindAftermarketForOEM`, after the primary 3-path union, take the chain (up to 5 entries) and issue a batched articlecrosses query:
   ```sql
   SELECT ... FROM articlecrosses ac ... WHERE ac.oemNumberNormalized IN (?, ?, ?, ?, ?)
   LIMIT 200
   ```
3. Dedupe against the primary result set.

**Acceptance criteria:**
- [ ] Unit test with a synthetic supersession chain (`26350-2J001 → 26320-2J001 → 26300-2J001`) — aftermarket lookup returns brands cataloged against ALL three OEMs.
- [ ] Audit re-run: for OEMs known to have a supersession chain (find in `docs/reports/2026-08-23-quality-audit/failures.csv` where `HasSupersession=true`), `AvgAM_correct` climbs by ≥ 1 additional avg.

**Effort:** M

**Dependencies:** M2.S1.T1

---

### Task M2.S1.T3 — Brand normalisation

**Goal.** Collapse `"BOSCH"`, `"Bosch"`, `"Robert Bosch GmbH"`, `"BOSCH GmbH"`, `"bosch"` into a single canonical brand. Prevents deduping from failing on capitalisation / punctuation.

**Files to touch:**
- New `internal/service/brand_normalize.go`
- New `internal/service/brand_normalize_test.go`

**Approach:**
1. Table-driven canonical map for the top 200 aftermarket brands. Load once at init.
2. `NormalizeBrand(rawBrand)` steps:
   1. Trim, uppercase, strip suffixes (`GMBH`, `INC`, `LTD`, `S.A.`, `SPA`, `KG`, `AG`, `CO`)
   2. Look up in canonical map — return canonical form if present
   3. Otherwise return the cleaned-up version

**Sample map (excerpt):**
```go
var brandCanonical = map[string]string{
    "BOSCH": "Bosch", "ROBERTBOSCH": "Bosch", "ROBERT BOSCH": "Bosch",
    "MANN": "MANN-FILTER", "MANNFILTER": "MANN-FILTER", "MANN FILTER": "MANN-FILTER", "MANNHUMMEL": "MANN-FILTER",
    "MAHLE": "MAHLE", "MAHLEBEHR": "MAHLE", "MAHLE ORIGINAL": "MAHLE", "KNECHT": "MAHLE",
    "MOBIS": "Mobis", "HYUNDAIMOBIS": "Mobis",
    "DENSO": "Denso",
    "NGK": "NGK",
    "VALEO": "Valeo",
    "HELLA": "Hella",
    "TEXTAR": "Textar",
    "FERODO": "Ferodo",
    "TRW": "TRW",
    "ATE": "ATE",
    // ...
}
```

**Acceptance criteria:**
- [ ] Table has ≥ 200 entries.
- [ ] Unit test covers the top 30 HK-shipping brands + 5 variants each.
- [ ] `NormalizeBrand("Robert Bosch GmbH") == "Bosch"`

**Effort:** M

**Dependencies:** none

---

## Sprint M2.S2 — Priority ordering + per-brand cap

### Task M2.S2.T1 — Tiered brand ordering

**Goal.** Aftermarket results sort by brand recognisability. Tier 1 (OEM + top-10 aftermarket) before Tier 2 (mid) before Tier 3 (private label).

**Files to touch:**
- `internal/service/brand_normalize.go` — add `BrandTier(canonical) int`
- `internal/service/alternatives.go` — apply the tiered sort

**Approach:**
1. Tier 1 = `{"Mobis", "Hyundai", "Kia", "Bosch", "MANN-FILTER", "MAHLE", "Denso", "NGK", "Valeo", "Hella", "Textar", "Ferodo", "TRW"}`
2. Tier 2 = `{"Febi", "Meyle", "Blue Print", "Contitech", "Gates", "Dayco", "SKF", "FAG", "INA", "Koyo", "NSK", ...}`
3. Tier 3 = everything else
4. Sort with tie-break: `(tier ASC, brand ASC, partNumber ASC)`

**Acceptance criteria:**
- [ ] Unit test with a mixed input of 30 rows (10 per tier) sorts them tier-first.
- [ ] Test covers ties within a tier — alphabetical.

**Effort:** M

**Dependencies:** M2.S1.T3

---

### Task M2.S2.T2 — Cap at 20 total, 3 per brand

**Goal.** Prevent one brand from crowding out variety.

**Files to touch:**
- `internal/service/alternatives.go`

**Approach:**
1. After the tiered sort, run:
   ```go
   const maxTotal = 20
   const maxPerBrand = 3
   perBrand := make(map[string]int, 10)
   out := parts[:0]
   for _, p := range parts {
       if len(out) >= maxTotal { break }
       key := NormalizeBrand(p.Brand)
       if perBrand[key] >= maxPerBrand { continue }
       perBrand[key]++
       out = append(out, p)
   }
   ```

**Acceptance criteria:**
- [ ] Synthetic test: 40 Bosch + 40 Mann returns 20 total, 3 Bosch + 3 Mann + 14 others (if tier permits).
- [ ] Synthetic test: 5 Bosch returns 3 Bosch (not 5).

**Effort:** S

**Dependencies:** M2.S2.T1

---

## Sprint M2.S3 — Supersession-chain walker

### Task M2.S3.T1 — Transitive-closure supersession

**Goal.** `FindSupersession(articleId)` walks the whole chain up and down, not just one hop.

**Files to touch:**
- `internal/service/tecdoc_supersession.go`

**Approach:**
1. Recursive walk with a `visited map[int]bool` cycle guard.
2. Depth cap = 5 (empirically the longest known TecDoc chain).
3. Return the full chain as `[]model.SupersessionLink` in walk order (root → tail).

**Acceptance criteria:**
- [ ] Unit test with a stub that returns a 4-hop synthetic chain. Result has 4 links in order.
- [ ] Unit test with a cyclic chain (A→B→A). Result returns [A, B] without infinite loop.
- [ ] Existing single-hop tests still pass.

**Effort:** M

**Dependencies:** none

---

### Task M2.S3.T2 — Populate `OEMNumbers` from supersession chain

**Goal.** Every OEM in the expanded supersession chain becomes an entry in the enriched result's `OEMNumbers` list, tagged with `Manufacturer = "SUPERSESSION"`.

**Files to touch:**
- `internal/service/enrichment.go` — extend the enrichment cascade

**Approach:**
1. After the specs/vehicles calls, invoke `FindSupersession(articleId)` (post M2.S3.T1 it returns the full chain).
2. Convert each link to `model.OEMReference` with `Manufacturer = "SUPERSESSION"`, append to `enriched.OEMNumbers`.

**Acceptance criteria:**
- [ ] For OEMs with a known supersession chain, `OEMNumbersCount` climbs by the chain length.
- [ ] Audit re-run: `Supersession_pct` climbs from 1.2% to ≥ 30%.

**Effort:** S

**Dependencies:** M2.S3.T1

---

## Milestone M2 exit criteria

- [ ] `AvgAM_correct ≥ 5` on wear parts
- [ ] `AvgOEMxRef_correct ≥ 3` on wear parts
- [ ] `AvgRepl_correct ≥ 8` on wear parts
- [ ] `F1_rich5 ≥ 0.60` on wear parts
- [ ] `F1_rich10 ≥ 0.30` on wear parts
- [ ] `Supersession_pct ≥ 30%` on correct hits
- [ ] Audit re-run diff attached
