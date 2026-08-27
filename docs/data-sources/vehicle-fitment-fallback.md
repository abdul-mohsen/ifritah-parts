# Vehicle Fitment Fallback — schema comparison notes

**Task:** M3.S2.T1 (spike + implementation).
**Files affected:** `internal/service/tecdoc_vehicle.go`, `internal/service/tecdoc_vehicle_test.go`.
**Goal:** When `TecDocVehicle.FindCompatibleVehicles(legacyArticleId, limit)` returns 0 rows through the strict primary join, fall back to a looser query against `articlesvehicletrees + linkagetargets` only, so wear parts (filters, brakes, etc.) that are legitimately shared across manufacturers/model-series gaps still light up the `vehicle_fitment` strategy.

---

## 1. TecDoc tables involved

All names/columns are verbatim from the raw MySQL dump (camelCase, no underscores). Row counts are approximate as of 2026-08.

| Table | Rows | Role |
|---|---|---|
| `articlesvehicletrees` (`avt`) | ~651M | Part ↔ Vehicle linkage table (`legacyArticleId`, `linkingTargetId`, `linkingTargetType`, `assemblyGroupNodeId`) |
| `linkagetargets` (`lt`) | ~5M (across langs) | The vehicle-variant descriptor keyed by `linkageTargetId`. Multi-lingual — one row per `(linkageTargetId, lang)`. Holds `description`, `beginYearMonth`, `endYearMonth`, `fuelType`, `capacityCC`, `horsePowerFrom`, `vehicleModelSeriesId` |
| `modelseries` (`ms`) | ~50k | `(modelId, manuId, modelname, linkingTargetType)` — the model dimension |
| `manufacturers` (`m`) | ~500 | `(manuId, manuName, linkingTargetType)` — the make dimension |
| `assemblygroupnodenames` (`agn`) | ~200k | Category-hint text keyed by `(assemblyGroupNodeId, lang)` |

Column-name notes (they trip people up):

- `articlesvehicletrees.linkingTargetId` — camelCase, join-key column.
- `linkagetargets.linkageTargetId` — same value space, but the **column** here is spelled `linkageTargetId` (with a `k`). Both spellings live in the same DB. All primary joins use `lt.linkageTargetId = avt.linkingTargetId`.
- Year fields on `linkagetargets` are `beginYearMonth` / `endYearMonth` (YYYYMM integer, e.g. `201503`). The M3.S2.T1 task spec sketch used `yearFrom` / `yearTo` — those don't exist in this schema. We use the real names and continue to run them through `yearFromYearMonth()`.
- HP is `lt.horsePowerFrom` (there is also a `horsePowerTo`).

---

## 2. Primary path (current)

`internal/service/tecdoc_vehicle.go` → `sqlVehicleFitmentRepo.QueryCompatibleVehicles`:

```
FROM articlesvehicletrees avt
JOIN linkagetargets       lt  ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
JOIN modelseries          ms  ON ms.modelId         = lt.vehicleModelSeriesId
JOIN manufacturers        m   ON m.manuId           = ms.manuId
LEFT JOIN assemblygroupnodenames agn
    ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
WHERE avt.legacyArticleId = ? AND avt.linkingTargetType = 'P'
```

Four inner joins wide. Returns richly-decorated rows: `linkingTargetId`, `lt.description` (English), `m.manuName`, `ms.modelname`, year range, fuel, cc, hp, and a category hint that drives `FitmentDriver` classification.

### Why it loses rows

Every one of these conditions is a silent row-dropper:

1. **`lt.lang = 'en'`** — a linkage target that only has non-English rows is invisible.
2. **`INNER JOIN modelseries`** — some `linkagetargets` rows have `vehicleModelSeriesId` values that don't resolve in `modelseries` (dump gaps, or the row predates a model-series consolidation).
3. **`INNER JOIN manufacturers`** — same class of dump gap on the manufacturer side.
4. **`linkingTargetType = 'P'` on `avt`** — HK dumps have been observed to use `V` or blank in a minority of rows (see `scripts/diagnostics/vehicle_fitment_mysql.sql` §6). We keep this filter — the strategy is only interested in passenger fitment — but note it as a loss channel.

Empirically, wear parts (oil filters, cabin filters, brake pads) hit conditions 2 and 3 most often: the article's `avt` linkages point at vehicle variants whose `modelseries` / `manufacturers` denormalization is incomplete, so the whole row is dropped even though the fitment is real.

---

## 3. Fallback path (this task)

Two-way join only. No `modelseries`, no `manufacturers`, no `lang` filter:

```
FROM articlesvehicletrees avt
JOIN linkagetargets       lt  ON lt.linkageTargetId = avt.linkingTargetId
WHERE avt.legacyArticleId = ? AND avt.linkingTargetType = 'P'
ORDER BY (lt.lang = 'en') DESC, lt.beginYearMonth DESC, avt.linkingTargetId
LIMIT ?
```

Key design decisions:

- **Drop `modelseries` + `manufacturers`.** This is the primary coverage win: rows survive when either denormalization side has a gap.
- **Drop `lt.lang='en'`.** Adds coverage for TecDoc dumps whose language mix skips English on a linkage target. The `ORDER BY (lt.lang='en') DESC` prefix causes the English row to sort first when it exists, so downstream `seen[linkingTargetId]` dedup keeps the English description; non-English is only surfaced when English is genuinely absent.
- **Skip `assemblygroupnodenames`.** The fallback trades category-hint richness for coverage — `FitmentDriver` on fallback rows is the generic classification (empty `CategoryHint` → default driver). This is acceptable: the point of the fallback is to make the vehicle_fitment strategy fire at all.
- **Make / Model empty.** Without the manufacturer/model-series joins we cannot denormalize `Make` / `Model`. The full description in `VehicleName` still starts with `HYUNDAI TUCSON (TL) …`, so downstream renderers have the raw label; existing `parseVehicleDescription` still extracts Chassis and EngineSpec unchanged.

### Coverage expectation

- On articles whose primary hits, the fallback is never called — no regression risk.
- On articles where primary is empty because of the 4-join strictness, the fallback typically resolves 5–50 vehicles (bounded by `LIMIT`).
- On articles that have zero linkages in `articlesvehicletrees` at all (some accessories, some newly-added ranges), both paths are empty; we return `nil, nil` and let downstream strategies (spec deduction, cross-brand) handle it.

The audit metric `Vehicles_pct` is measured over "wear-parts queries" — filter / brake / suspension entries against seeded Hyundai/Kia articles. Because those articles are exactly the ones that fail on `modelseries`/`manufacturers` gaps (they're widely-shared across trims), we expect the fallback to cover ≥ 60% of the currently-empty set.

---

## 4. Fast-return + error semantics

`FindCompatibleVehicles(legacyArticleId, limit)` after this change:

| Primary result | Fallback | Return |
|---|---|---|
| rows > 0 | *not called* | primary rows |
| rows = 0, error = nil | called | fallback rows (may be empty) |
| rows = 0, error ≠ nil | *not called* | primary error |
| rows = 0, fallback error ≠ nil | called | empty slice (fallback error is logged as WARN, not surfaced) |

Rules:

1. **Fail-loud on true errors.** A primary DB error is a real system fault (index gone, connection broken) — surface it.
2. **Fail-quiet on empty.** An article legitimately without fitment is not an error.
3. **Fallback errors are best-effort.** If the fallback query itself fails, we log a warning and return the empty primary result. Reason: at this point the primary is already known empty (not an error). A fallback-only error should not turn a "no fitment" answer into a hard failure.

Observability: on the fallback-hit path we emit
`[vehicle_fitment] primary empty, fallback returned N rows for legacyArticleId=X`
so audit reruns can quantify the coverage lift from log grep alone.

---

## 5. KISS invariants

- **One fallback level only.** primary → articlesvehicletrees-only → empty. No further chained fallbacks in this task. If a broader search is needed later (e.g. `linkingTargetType` = `P` or `V`), that lands as M3.S2.T3+ with its own audit.
- **Same return shape.** `[]model.CompatibleVehicle`; the fallback path fills the fields it can (LinkageTargetId, VehicleName, YearFrom/To, FuelType, CapacityCC, HorsePower, Chassis, EngineSpec) and leaves Make/Model empty rather than inventing values.
- **Repo interface additive.** `vehicleFitmentRepo` grows one method; the existing stub in tests grows one method. No signature changes on the public `FindCompatibleVehicles`.

---

## 6. Post-merge validation

After merge, run the seeded-article audit and record `Vehicles_pct` before / after:

```
scripts/audit/run-per-strategy-audit.sh --strategy vehicle_fitment
```

Expected: from 0% → ≥ 60% on the wear-parts subset. If less, the loss channel is likely `linkingTargetType`, not the joins — file M3.S2.T3 to loosen that filter with its own spike.
