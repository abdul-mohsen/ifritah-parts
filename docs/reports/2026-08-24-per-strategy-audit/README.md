# 2026-08-24 Per-Strategy Audit — 1490 OEMs × 10 strategies

Deployment audited: qa.ifritah.com bundle `2026-08-21 22:51 UTC` — PR #19 merged, PR #20 code (M1.S1 penalty + M1.S2 deny-list widening + M1.S2.T2 solo-cache floor) **NOT deployed yet**. Baseline for comparing the impact once PR #20 merges + deploys.

## Bottom line — `combined` is worse than `cache` alone

**Overall F1_correct on the audit corpus:**

| Rank | Strategy | F1_hit | **F1_correct** | AvgRepl | AvgAM | AvgOEMxRef |
|---|---|---:|---:|---:|---:|---:|
| 1 | `cache` | 0.76 | **0.46** | 0.00 | 0.00 | 0.00 |
| 2 | `legacy` | 0.66 | **0.45** | 0.27 | 0.18 | 0.09 |
| 3 | `prefix_inference` | 0.61 | 0.33 | 0.00 | 0.00 | 0.00 |
| 4 | **`combined`** | 0.68 | **0.32** ⚠ | 0.02 | 0.02 | 0.00 |
| 5 | `exact_oem` | 0.47 | 0.18 | **0.69** | **0.45** | **0.24** |
| 6 | `cross_reference` | 0.05 | 0.05 | 0.00 | 0.00 | 0.00 |
| 7 | `cross_brand` | 0.05 | 0.05 | 0.00 | 0.00 | 0.00 |
| 8 | `keyword_gated` | 0.00 | 0.00 | — | — | — |
| 9 | `owned_catalog` | 0.00 | 0.00 | — | — | — |
| 10 | `supersession` | 0.00 | 0.00 | — | — | — |

**On the `real_hk_seeded` slice (390 OEMs — ideal case):**

| Strategy | F1_correct | AvgRepl |
|---|---:|---:|
| `cache` | 0.75 | 0.00 |
| `prefix_inference` | 0.75 | 0.00 |
| `combined` | **0.74** ⚠ | 0.02 |
| `exact_oem` | 0.47 | 0.70 |
| `legacy` | 0.47 | 0.69 |
| `cross_brand`, `cross_reference` | 0.14 | 0.00 |
| `keyword_gated`, `owned_catalog`, `supersession` | 0.00 | — |

## Findings

### 1. **`combined` is losing to its own components** (F1_correct = 0.32 vs cache = 0.46)

`combined` fans out cache + legacy + prefix_inference + exact_oem in parallel and merges. But the merge is picking lower-quality results — the F1_correct actually DROPS by 0.14 vs running cache alone.

**Root cause hypothesis (pre-PR-#20 audit):** the merge ranker preferred whichever result had the highest raw confidence. When cache and prefix_inference disagreed on category, the wrong-category result won on confidence alone. This is exactly what **M1.S1's cross-family penalty** in PR #20 addresses.

**Expected impact after PR #20 deploys:** combined should climb ABOVE cache alone once the penalty demotes cross-family results. Target: combined F1_correct ≥ 0.55 (from 0.32).

### 2. **`exact_oem` has the best replacement richness** (AvgRepl = 0.69)

But its recall is limited (F1_hit = 0.47) because it only fires when the OEM exists in `oem_number` (TecDoc's OEM catalog — ~5% HK coverage). When it hits, it hits well.

**Implication for M2:** the multi-path aftermarket UNION (M2.S1.T1) needs to run FIRST via `exact_oem`'s path, then fall back to `cross_reference` / `prefix_inference` etc.

### 3. **Five strategies contribute zero to combined** (F1_hit = 0)

- `owned_catalog` — `hk_parts_cache` empty on qa (M0.T1)
- `supersession` — 0 hits everywhere (M0.T2)
- `keyword_gated` — audit uses OEM queries; needs keyword corpus (M0.T5)
- `vin_assembly`, `vehicle_fitment` — need parameter enrichment (M0.T3, M0.T4)

**Combined loses N% recall for every broken strategy that would have hit.** Fixing these is the M0 backlog.

### 4. **`cross_reference` and `cross_brand` are data-sparse** (F1_hit = 0.05 each)

Both use `articlecrosses.oemNumberNormalized` (indexed by PR #16). The 0.05 hit rate on random HK OEMs suggests articlecrosses has HK coverage under ~5% too — worse than expected. Need to sample which categories DO have coverage.

**Followup:** run this audit filtered to `Brake Pad Set`, `Brake Disc`, `Oil Filter` (categories known to have TecDoc aftermarket data) — expect `cross_reference` F1_hit ≥ 0.60 on those subsets.

## Files

- `raw.csv` — 14900 rows (1490 OEMs × 10 strategies)
- `by-strategy.csv` — F1_hit / F1_correct / AvgRepl per strategy
- `by-strategy-slice.csv` — 50 rows (strategy × slice matrix)
- `by-category.csv` — 126 categories × F1s

## Reproducing

```pwsh
pwsh scripts/audit/audit-quality.ps1 `
  -InputCorpus scripts/audit/corpus-1500-v2.csv `
  -Modes cache,legacy,exact_oem,cross_reference,cross_brand,supersession,owned_catalog,keyword_gated,prefix_inference,combined `
  -EnrichmentLevel none `
  -ThrottleLimit 5 -InterRequestMs 400

pwsh scripts/audit/analyze-quality.ps1 -InputCSV <newest raw>
```

`enrichmentLevel=none` because this audit is about SEARCH quality (per-strategy hit rate + correctness), not enrichment. Full-enrichment audit is a separate run — see `docs/reports/2026-08-23-quality-audit/`.

## Next actions (in priority order)

1. **PR #20 merge + deploy** → re-run this audit. Expect `combined` F1_correct to climb from 0.32 to ≥ 0.55 as the M1.S1 penalty demotes cross-family results.
2. **M0.T2 `supersession` fix** → adds ~0.05 to combined F1_correct + enables the M2.S3 chain walker.
3. **M0.T1 `owned_catalog` fix** → adds ~0.05 to combined F1_correct (fast HK path).
4. **M0.T5 `keyword_gated` proper corpus** → separate test of the strategy on its natural input.
5. **M1.S3 category-consistency validation** → drops the ~40% of combined hits that are wrong-category.
