# Search-Quality Audit Scripts

Reusable PowerShell scripts for measuring end-to-end search quality against any
deployed environment. Two-phase workflow:

1. **`audit-quality.ps1`** - runs a corpus of OEMs through the search API and
   captures the raw response (hit / miss, first result description + brand +
   category, enrichment coverage) into a dated CSV.

2. **`analyze-quality.ps1`** - reads the raw CSV and emits four dated slices
   (per-category, per-system, per-slice, per-failure) plus a human-readable
   summary. Computes TP / FP / FN / F1 with a `GoodTokens` category match.

Every run stamps its outputs with `yyyy-MM-dd_HHmm` so results are cumulative
and comparable across deploys.

## Quick start

```pwsh
# 1. Run the audit (default corpus, qa.ifritah.com, combined + full enrichment)
pwsh scripts/audit/audit-quality.ps1

# 2. Analyze the newest raw CSV
$raw = Get-ChildItem scripts/audit/qa-quality-raw-*.csv |
       Sort-Object LastWriteTime -Desc | Select-Object -First 1
pwsh scripts/audit/analyze-quality.ps1 -InputCSV $raw.FullName
```

## Corpus format (`corpus-1500-v2.csv`)

Required columns:

| Column             | Meaning                                                   |
|--------------------|-----------------------------------------------------------|
| `OEM`              | Part number to search (e.g. `26350-2J001`)                |
| `Slice`            | Corpus slice tag (real_hk_seeded / real_hk_coarse / …)   |
| `GroundTruth`      | `exists`, `not_hk_format`, or `non_hk`                    |
| `ExpectedCategory` | Human-readable category for the OEM                       |
| `GoodTokens`       | Comma-separated tokens the correct description must contain (case-insensitive) |

Optional (used by grouping/reports):

`ExpectedSystem`, `ExpectedMake`, `Chassis`, `Prefix5`

## Outputs (all dated)

| File                                            | What                                                    |
|-------------------------------------------------|---------------------------------------------------------|
| `qa-quality-raw-<date>.csv`                     | one row per OEM: response fields + enrichment counts    |
| `qa-quality-by-category-<date>.csv`             | **every category**, TP/FP/FN/F1 + enrichment coverage % |
| `qa-quality-by-system-<date>.csv`               | grouped by `ExpectedSystem`                             |
| `qa-quality-by-slice-<date>.csv`                | grouped by corpus `Slice`                               |
| `qa-quality-failures-<date>.csv`                | every failing OEM with reason                           |
| `qa-quality-summary-<date>.txt`                 | human-readable overview                                 |

## Classification logic

For each response row:

| GroundTruth      | Hit + tokens match | Hit + tokens don't match | Zero results |
|------------------|--------------------|--------------------------|--------------|
| `exists`         | **TP**             | **FP** (wrong-category)  | **FN**       |
| `not_hk_format`  | **FP** (empty-leak)| —                        | **TN**       |
| `non_hk`         | **FP** (non-HK leak) | —                      | **TN**       |

Token match = ≥ 2 of the `GoodTokens` (comma-separated) appear in the
returned description (case-insensitive).

## Parameters (`audit-quality.ps1`)

| Param              | Default                          | Notes                                             |
|--------------------|----------------------------------|---------------------------------------------------|
| `-InputCorpus`     | `corpus-1500-v2.csv`             | Path to CSV corpus                                |
| `-OutputDir`       | `<repo>/scripts/audit`           | Where to write the dated raw CSV                  |
| `-Endpoint`        | `https://qa.ifritah.com`         | Full URL of the search endpoint                   |
| `-Mode`            | `combined`                       | `combined` / `cache` / `legacy` / `exact_oem` / … |
| `-EnrichmentLevel` | `full`                           | `none` / `basic` / `full`                         |
| `-ThrottleLimit`   | `4`                              | Concurrent workers                                |
| `-MaxTimeoutS`     | `25`                             | Per-request timeout                               |
| `-InterRequestMs`  | `750`                            | Delay between requests per worker (429 mitigation)|
| `-MaxRetries`      | `5`                              | Exponential-backoff retries on 429                |

## Historical results

See `docs/reports/2026-08-23-quality-audit/` for the first full-enrichment run
against qa (bundle `2026-08-21 22:51 UTC`). Overall F1 = 0.30 on 1490 OEMs.
Follow-up PRs will re-run and diff against that baseline.
