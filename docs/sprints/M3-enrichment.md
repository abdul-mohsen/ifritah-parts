# M3 Sprint Backlog — Full enrichment coverage

**Milestone exit gate.** On correct-part hits (`F1_correct = TP`):

- `Specs_pct ≥ 80%`
- `Vehicles_pct ≥ 60%`
- `Supersession_pct ≥ 40%` (up from M2's 30%)

Baseline: 2.5% specs, 0% vehicles, 1.2% supersession.

---

## Sprint M3.S1 — Aggressive article-id promotion + batching

### Task M3.S1.T1 — Chained promotion (three fallback levels)

**Goal.** Article-id promotion currently tries `SearchByOEM` then falls back to `SearchCrossReferences` (per PR #20). Add a third fallback via `oem_search_index.normalized`. Also, when multiple candidates come back, pick the one with the highest `articles.dataSupplierId` recency (proxy for "canonical / most current").

**Files to touch:**
- `internal/service/enrichment.go`
- New unit test `TestEnrichResults_ChainedPromotion`

**Approach:**
1. In `enrichResults`, after the existing two-level fallback (SearchByOEM → SearchCrossReferences), if `articleId` is still `0`, run a third query directly:
   ```go
   if articleId == 0 && s.tecdoc != nil && budgetLeft() {
       q := `SELECT osi.legacyArticleId
             FROM oem_search_index osi
             WHERE osi.normalized = ?
             ORDER BY osi.legacyArticleId DESC
             LIMIT 5`
       rows, err := s.tecdoc.db.QueryContext(ctx, q, clean)
       if err == nil {
           for rows.Next() {
               var id int
               if err := rows.Scan(&id); err == nil && id > 0 {
                   articleId = id
                   break
               }
           }
           rows.Close()
       }
   }
   ```
2. Optional: when promotion returns multiple candidates, pick the one with the highest `dataSupplierId` (the numerically-latest supplier — a proxy for canonical Mobis / Hyundai / Kia entries). Adds a small subquery.

**Acceptance criteria:**
- [ ] Unit test with a stub `SearchByOEM` returning empty + stub `SearchCrossReferences` returning empty + stub direct query returning `articleId=42`. Promotion picks 42.
- [ ] Audit re-run: article-id promotion rate on seeded slice climbs from 26% to ≥ 80%. Instrument by counting how often `articleId > 0` in the log.

**Effort:** M

**Dependencies:** PR #20 (already merged) — the `SearchCrossReferences` fallback

---

### Task M3.S1.T2 — Batch specs / vehicles enrichment

**Goal.** Today each result-goroutine fires its own DB round-trip for specs, vehicles, docs. For a 20-result response, that's 60+ DB queries. Batch them.

**Files to touch:**
- `internal/service/tecdoc_specifications.go` — add `FindSpecificationsBatch(ids []int) (map[int][]Specification, error)`
- `internal/service/tecdoc_vehicle.go` — add `FindCompatibleVehiclesBatch(ids []int, limitPerId int)`
- `internal/service/tecdoc_supersession.go` — add `FindSupersessionBatch(ids []int)`
- `internal/service/enrichment.go` — collect all `articleId`s first, batch-fetch, then distribute results

**Approach:**
1. Two-phase enrichment:
   - **Phase 1** — promotion: parallel per-result article-id resolution (unchanged from PR #20 with the M3.S1.T1 third-level fallback).
   - **Phase 2** — batched enrichment: single SQL round-trip each for specs / vehicles / docs / supersession using `WHERE legacyArticleId IN (?, ?, ..., ?)` and `LIMIT ? * len(ids)` semantics.
2. Distribute results back into `enriched[idx]` via `map[int][]row` lookup keyed on articleId.

**Sample:**
```go
// After phase 1, gather all promoted article ids
articleIds := make([]int, 0, len(enriched))
idToIdx := make(map[int][]int, len(enriched))
for i, r := range enriched {
    if r.LegacyArticleId > 0 {
        articleIds = append(articleIds, r.LegacyArticleId)
        idToIdx[r.LegacyArticleId] = append(idToIdx[r.LegacyArticleId], i)
    }
}
if len(articleIds) == 0 { return enriched }

// Phase 2 — batch
if s.tecDocSpecs != nil {
    specsById, _ := s.tecDocSpecs.FindSpecificationsBatch(ctx, articleIds)
    for id, specs := range specsById {
        for _, idx := range idToIdx[id] { enriched[idx].Specifications = specs }
    }
}
// ... same for vehicles, docs, supersession
```

**Acceptance criteria:**
- [ ] Enrichment p95 drops ≥ 40% at same coverage (measure on a 20-result response).
- [ ] All existing enrichment tests pass.
- [ ] New unit test asserts batch shape returns correct per-id rows.

**Effort:** L

**Dependencies:** M3.S1.T1

**Note:** batched queries need the `articlecriteria.legacyArticleId` index from PR #19's `sql/07` migration to be applied — otherwise the `IN (?, ..., ?)` becomes N full-table scans.

---

## Sprint M3.S2 — Vehicle fitment expansion

### Task M3.S2.T1 — Fallback via `articlesvehicletrees`

**Goal.** When `FindCompatibleVehicles` returns 0 vehicles for an article, try the alternative TecDoc join path via `articlesvehicletrees`.

**Files to touch:**
- Spike task first: `docs/data-sources/tecdoc-vehicle-tree.md` — write up the schema difference between `articlelinkages` (current path) and `articlesvehicletrees` (fallback). What does each cover?
- `internal/service/tecdoc_vehicle.go`
- `internal/service/tecdoc_vehicle_test.go`

**Approach:**
1. Spike: connect to prod MySQL read replica, run:
   ```sql
   SELECT COUNT(*) FROM articlelinkages WHERE legacyArticleId = 6103;
   SELECT COUNT(*) FROM articlesvehicletrees WHERE legacyArticleId = 6103;
   ```
   for 5 seeded articles — quantify the coverage difference.
2. If `articlesvehicletrees` has coverage where `articlelinkages` doesn't, add a fallback query in `FindCompatibleVehicles`:
   ```go
   if len(vehicles) == 0 {
       vehicles = t.findCompatibleVehiclesViaTree(ctx, legacyArticleId, limit)
   }
   ```
3. Result shape must match the primary path — use the same `model.CompatibleVehicle`.

**Acceptance criteria:**
- [ ] Spike report committed.
- [ ] Audit re-run: `Vehicles_pct` climbs from 0% to ≥ 60% on wear parts.

**Effort:** M (spike) + M (impl)

**Dependencies:** none

---

### Task M3.S2.T2 — Parse vehicle description into structured fields

**Goal.** Return `Make / Model / YearRange / Engine / Chassis` structured fields instead of raw `linkageTargets.description`.

**Files to touch:**
- `internal/model/compatible_vehicle.go` — extend struct
- `internal/service/tecdoc_vehicle.go` — parse
- `internal/service/tecdoc_vehicle_test.go`

**Approach:**
1. TecDoc `linkageTargets.description` shape is typically:
   ```
   HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]
   ```
2. Parse rules:
   - Split on space; first word = Make (uppercase HYUNDAI → "Hyundai")
   - Next word(s) up to `(chassis)` = Model
   - `(chassis)` optional — the 2-4 letter code
   - Engine code = something like `2.0 CRDi 4WD 136HP` — capture as-is
   - Year range in `[MM.YYYY-MM.YYYY]` or `[MM.YYYY-]` — parse into `YearFrom / YearTo`
3. Populate `Make`, `Model`, `Chassis`, `EngineSpec`, `YearFrom`, `YearTo`.

**Acceptance criteria:**
- [ ] Unit test parses 20 known real descriptions correctly across Hyundai / Kia variants.
- [ ] Falls back gracefully when the format is unexpected — keeps the raw description as `VehicleName`.

**Effort:** M

**Dependencies:** M3.S2.T1

---

## Milestone M3 exit criteria

- [ ] `Specs_pct ≥ 80%` on correct hits
- [ ] `Vehicles_pct ≥ 60%` on correct hits
- [ ] `Supersession_pct ≥ 40%` on correct hits
- [ ] Enrichment p95 ≤ 3 s
- [ ] Audit re-run diff attached
