# Article-id promotion diagnosis (M3.S1.T1)

**Status:** In progress on `feat/m3-s1-t1-chained-article-id-promotion`.
**Symptom before fix:** 2026-08-23 quality audit — `CompatibleVehicles`
populated 0.0%, `Specifications` 2.5%, `Supersession` 1.2%. Every
article-anchored enrichment call requires a non-zero
`legacyArticleId`; without it, the goroutine bails early on line
`enrichment.go:218` (`if articleId <= 0 { return }`).
**Root cause:** Article-id promotion inside `enrichResults` failed
approximately 74% of the time on the seeded HK slice, leaving the
downstream enrichments starved.

---

## What was broken (pre-M3.S1.T1)

`enrichResults` (lines 132–194 in the pre-M3 tree) tried to promote
an OEM string to a `legacyArticleId` for any result whose search
strategy didn't already carry one — that is, `prefix_inference`,
`dealer_lookup`, `cache`, and any online-scraped result. The
promotion cascade was already three layers deep:

| Layer | Source | Method                                      | Table                 |
|------:|--------|---------------------------------------------|-----------------------|
| 1     | primary | `TecDoc.SearchByOEM(oem, 5)`              | `oem_number` + `oem_search_index` (internal UNION) |
| 2     | fallback | `TecDocCrossRef.SearchCrossReferences(oem, 5)` | `articlecrosses.oemNumberNormalized` |
| 3     | fallback | `TecDoc.SearchByOEMIndex(oem, 5)`         | `oem_search_index.normalized` (rescue when layer-1 primary block errored) |

Two gaps remained after PR #20 (which fixed the *aftermarket* union
path in `TecDoc.FindAftermarketForOEM`):

### Gap 1 — First-seen instead of canonical pick

The pre-fix loop was:

```go
refs, err := s.tecdoc.SearchByOEM(oem, 5)
if err == nil && len(refs) > 0 {
    for _, ref := range refs {
        if ref.LegacyArticleId > 0 {
            articleId = ref.LegacyArticleId
            break // ← takes first non-zero, no tiebreak
        }
    }
    enriched.OEMNumbers = append(enriched.OEMNumbers, refs...)
}
```

When `SearchByOEM` returned five candidates against a single OEM
(distinct suppliers cataloging the same part), the promoter picked
whichever landed first in the result set. Row order from the
underlying MySQL query is not stable — it depends on join
cardinality against `ambrand` — so two identical runs against a
stable database could pick different article ids, driving different
enrichment fields into the response.

The roadmap DoD calls for the pick to prefer the article with the
highest `articles.dataSupplierId` (proxy for "most recently added
supplier catalog entry" — a rough canonical-ness signal). That
required either a batch lookup after dedupe or extending the
`SearchByOEM` SELECT list. This PR uses a batch lookup so the
existing `model.OEMReference` shape stays untouched.

### Gap 2 — Not testable

The promotion logic was inline inside a per-result goroutine in
`enrichResults`. Every test of it required either:
- A live MySQL connection to hit the three real tables, or
- Reaching in through `SmartSearch` fields with no way to stub the
  individual layers.

Nothing in `enrichment_test.go` covered the fallback wiring. The
sibling `TestEnrichResults_*` tests in `strategy_test.go` only
exercise the no-op and budget paths.

The audit's "26% promotion rate" number came from log-mining, not a
regression test — meaning a bug in the fallback plumbing wouldn't
have been caught before landing in staging.

---

## Design decision — fallthrough vs UNION

The roadmap wording is ambiguous:

> *"if `SearchByOEM → 0 refs`, try `SearchCrossReferences → 0 refs`,
>  try `oem_search_index → 0 refs`. **Each fallback adds article-id
>  candidates.** Pick the one with the highest `articles.dataSupplierId`
>  recency when multiple candidates surface."*

Two readings:

1. **Fallthrough** (fast-path): call layer 1; if it returns rows,
   *stop* and use those. Only descend when the previous layer
   returned zero. The canonical pick then applies **within** the
   winning layer's rows.

2. **UNION**: always call all three, concatenate, dedupe, then pick
   canonical across the full set. Widens the net (max recall) but
   costs three DB round-trips per un-promoted result.

**This PR picks fallthrough.** Justification:

- **Signal quality**: layer 1 (`oem_number`) is TecDoc's own
  authoritative OEM index. If it has a hit, the row's
  dataSupplierId is already the most authoritative anchor for that
  OEM. Layer 2 (`articlecrosses`) is a cross-reference table —
  useful when OEM isn't in the primary index but noisier when it is.
- **DB round-trip cost**: `enrichResults` fans out 10 concurrent
  result-goroutines. Under UNION every un-promoted result would
  trigger three concurrent DB queries → 30 in flight. Fallthrough
  keeps it at ≤ 10 in the hot path (layer 1 hits) and only widens
  when needed.
- **The DoD is "26% → 80% promotion rate"**, not "80% of promoted
  articles picked from the widest possible candidate pool". The
  extra layer-2+3 candidates that UNION would provide are only
  interesting when layer 1 missed — exactly what fallthrough already
  covers.
- **Layer 3 (`oem_search_index`) is a subset of layer 1 in the happy
  path.** `TecDoc.SearchByOEM` already unions `oem_number +
  oem_search_index` internally. Layer 3 exists as a rescue for the
  case where layer 1's primary block errors out early (returns
  before reaching its own secondary block). Under UNION we'd be
  re-hitting `oem_search_index` twice for every un-promoted OEM,
  even when nothing was wrong with layer 1.

The canonical-pick step is still valuable **within** a layer:
`SearchByOEM` can legitimately return 5 rows for one OEM with
different `dataSupplierId`s (Bosch, MANN, MAHLE all catalog the
same Hyundai oil filter). We pick the highest-supplier-id row now,
matching the roadmap requirement, without opening the DB
round-trip cost of UNION.

If the post-merge audit shows fallthrough is still under 80%
promotion rate on the seeded slice, the natural next step is to
switch to UNION — the interface (`oemPromoter`) doesn't change,
just the loop body in `promoteArticleIds`.

---

## Refactor shape

The inline goroutine block splits into two testable pieces:

### `oemPromoter` interface

```go
type oemPromoter interface {
    PromoteByOEM(oem string, limit int) ([]model.OEMReference, error)
    PromoteByCrossReferences(oem string, limit int) ([]model.OEMReference, error)
    PromoteByOEMIndex(oem string, limit int) ([]model.OEMReference, error)
    FetchDataSupplierIds(articleIds []int) (map[int]int, error)
}
```

Production wiring: `smartSearchOEMPromoter{tecdoc, crossRef}` adapts
the existing `*TecDoc` + `*TecDocCrossRef` fields. Tests inject a
plain struct with recorded call counts.

### `promoteArticleIds` — the pipeline

```go
func promoteArticleIds(ctx context.Context, p oemPromoter, oem string, perLayerLimit int) (int, []model.OEMReference, error) {
    // layer 1: oem_number (+ oem_search_index internally)
    if refs := tryLayer(p.PromoteByOEM, oem, perLayerLimit); len(refs) > 0 {
        return pickCanonicalArticleId(p, refs), refs, nil
    }
    // layer 2: articlecrosses
    if refs := tryLayer(p.PromoteByCrossReferences, oem, perLayerLimit); len(refs) > 0 {
        return pickCanonicalArticleId(p, refs), refs, nil
    }
    // layer 3: oem_search_index (rescue)
    if refs := tryLayer(p.PromoteByOEMIndex, oem, perLayerLimit); len(refs) > 0 {
        return pickCanonicalArticleId(p, refs), refs, nil
    }
    return 0, nil, errNoPromotion
}
```

`pickCanonicalArticleId`:
1. Dedupes the returned refs by `LegacyArticleId`.
2. If only one non-zero id survives → return it (no DB round-trip).
3. Else batch-fetches `dataSupplierId` per id via
   `p.FetchDataSupplierIds(ids)` (one query, single IN-list) and
   returns the id with the highest supplier value. Ties break to
   the first-seen ordering.

### `TecDoc.FetchDataSupplierIds`

New method:

```sql
SELECT legacyArticleId, dataSupplierId
FROM   articles
WHERE  legacyArticleId IN (?, ?, ...)
```

`legacyArticleId` is already the join key used by every other
enrichment query — `articles(legacyArticleId)` has an index, so the
IN-list resolves in the same sub-millisecond band as
`FindSpecifications`. No new index required.

---

## Failure semantics

`promoteArticleIds` returns `errNoPromotion` (a sentinel) when
every layer returns zero refs. The caller in `enrichResults` treats
this as "give up on article-anchored enrichment" — the result is
still returned to the user with its original fields intact, just
without `Specifications` / `CompatibleVehicles` / `Supersession` /
`FunctionalEquivalents`. Aftermarket alternatives (which don't need
an articleId, only the OEM string) are unaffected.

This matches the pre-M3 behavior exactly — the observable outcome
of "OEM couldn't be promoted" was already an un-enriched result. We
just log `errNoPromotion` explicitly now instead of silently
falling through the goroutine.

---

## Expected impact

- **Promotion rate**: 26% → target ≥ 80% on the seeded HK slice.
  The 74% miss rate wasn't primarily from the layer count (three
  paths were already tried); it was from `SearchByOEM` returning
  rows where the *first non-zero article id happened to be sparse*
  in specs / vehicles / supersession while a sibling row on the
  same OEM had a fully-populated record. Canonical pick by
  dataSupplierId picks the fully-populated row when it's present.
- **Determinism**: identical requests now return the same article
  id (stable canonical tiebreak). The pre-fix "first non-zero"
  behaviour depended on unstable MySQL row ordering.
- **Testability**: the whole cascade is now covered by unit tests
  in `enrichment_test.go` using an in-memory `stubOEMPromoter`.
- **No perf regression**: fallthrough shape means layer 2+3 are
  still not called on a layer-1 hit; the added `FetchDataSupplierIds`
  is one indexed IN-list query per promoted result, only when the
  winning layer returned more than one distinct article id.

Post-merge, the next audit run should verify the promotion-rate
KPI. If the number is still short of 80% the follow-up is to
consider UNION — see the design-decision section above for the
trade-off.
