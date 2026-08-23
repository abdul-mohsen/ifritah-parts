# 2026-08-23 Search Quality Audit — 1490 OEMs × combined mode × full enrichment

Deployment audited: bundle `Last-Modified: 2026-08-21 22:51 UTC` — PRs #12/#13/#14/#16/#17/#18 merged, PR #19 **NOT** yet merged.

## Bottom line

**Overall F1 = 0.30.** Search works well only on OEMs whose prefix is in the seeded map. Enrichment is effectively dead: 0% compatible-vehicles, 2.5% specs, 6.7% aftermarket populated across all 756 hits.

| Slice | N | TP | FP | FN | F1 | AM% |
|---|---:|---:|---:|---:|---:|---:|
| `real_hk_seeded` (prefix in seed map) | 390 | 216 | 158 | 16 | **0.71** | 4.8 % |
| `real_hk_coarse` (real HK, sparse coverage) | 400 | 9 | 307 | 84 | 0.04 | 2.2 % |
| `real_hk_unseeded` (real HK, no seed) | 400 | 0 | 0 | **400** | **0.00** | — |
| `plausible_hk` (synthetic HK-format) | 200 | 0 | 28 | — | — | — |
| `non_hk` (Toyota/BMW/Nissan) | 100 | 0 | **38 leaks** | — | — | 68 % |

## Files

- `raw.csv` — one row per OEM, includes description, brand, category, aftermarket sample, specs count, vehicle count, OEM cross-ref sample, warnings.
- `by-category.csv` — **all 126 categories** classified TP / FP / FN + P / R / F1 + enrichment coverage %.
- `by-system.csv` — 15 systems (Engine, Brakes, HVAC, Body, Electrical, etc.).
- `by-slice.csv` — 5 slices.
- `failures.csv` — 1,031 failing OEMs with reason (wrong-category / non-HK-leak / no-results / timeout).

## Reproducing

```pwsh
pwsh scripts/audit/audit-quality.ps1 `
  -InputCorpus scripts/audit/corpus-1500-v2.csv `
  -Endpoint https://qa.ifritah.com `
  -Mode combined `
  -EnrichmentLevel full

pwsh scripts/audit/analyze-quality.ps1 `
  -InputCSV <the raw CSV path printed above>
```

Both scripts write dated outputs. Every run is comparable.

## Categories with F1 = 1.00 (working, all seeded prefixes)

`Brake Pad Set - Front/Rear`, `Brake Disc - Front/Rear`, `Power Window Motor`, `Air Filter`, `Cabin Air Filter`, `Oil Filter`, `Tail Light - Rear`, `Shock Absorber - Front`, `Radiator`

## Categories with F1 = 0 (broken, n ≥ 3) — 45 of them, top 15

| Category | N | Wrong-category FPs | Zero-result FNs |
|---|---:|---:|---:|
| Wiring Harness | 48 | 22 | 26 |
| Exterior Mirror | 46 | 44 | 2 |
| Mirrors | 33 | 24 | 9 |
| Manual Transmission | 17 | 17 | 0 |
| Instrument Panel / Dashboard | 15 | 8 | 7 |
| Weatherstrip & Seal | 13 | 12 | 1 |
| Glass / Windshield | 12 | 12 | 0 |
| Front Differential | 11 | 7 | 4 |
| Compressor A/C | 10 | 9 | 1 |
| Mouldings & Trim | 10 | 9 | 1 |
| Interior Trim | 9 | 4 | 5 |
| Front Body / Hood | 9 | 8 | 1 |
| Sensors & Modules | 9 | 8 | 1 |
| Fender & Side Body | 8 | 7 | 1 |
| Rear Suspension | 7 | 7 | 0 |

## Actionable findings (feed the follow-up PR)

1. **P0 recall — 400 FN on `real_hk_unseeded`.** Real Hyundai/Kia OEMs return zero results because their prefix is missing from `internal/service/oem_prefix.go` `prefixMap`. Biggest single recall win: expand prefix map to cover Body / Interior / Mirror / Glass / Transmission / Differential prefix families.

2. **P0 enrichment — 0 % `compatibleVehicles`, 2.5 % `specifications`.** Even after PR #19's `sql/07_articlecriteria_indexes.sql` unblocks specs, `FindCompatibleVehicles` never fires because the article-id promotion path returns 0 refs for prefix-inferred results 74 % of the time.

3. **P1 aftermarket — 6.7 % coverage.** `FindAftermarketForOEM` queries the `oem_number` table (which stores TecDoc's OEM catalog, not aftermarket cross-refs). Rewrite to use `articlecrosses.oemNumberNormalized` (indexed by PR #16) — same data source the cross-reference strategy already uses.

4. **P1 wrong-category FPs — 45 categories with F1=0.** Search returns *something* but the category is wrong (mirror OEM → headlight result). Ranker prefers seeded-category matches even when they're a poor fit for the OEM's actual family. Consider penalising cross-family matches at the merge step.

5. **P2 non-HK leak — 38 non-HK OEMs still returned results** despite the guard fix in PR #19. The guard fires for OEMs with a `SuggestedMake` from `nonHKMakeHints`, but 38 leaks means: (a) some non-HK OEMs match no deny-list prefix, (b) they pass the guard, (c) then get "matched" via the coarse cache/legacy strategies. Root cause is guard coverage, not code logic.
