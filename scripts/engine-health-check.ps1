#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Unified engine-health check: runs the API quality audit AND surfaces the
  TecDoc MySQL diagnostic in one command, then produces a combined report.

.DESCRIPTION
  Wraps three existing pieces into one workflow:

    1. scripts/audit/audit-quality.ps1        (API-side quality audit)
    2. scripts/audit/analyze-quality.ps1      (per-slice + per-category CSVs)
    3. scripts/diagnostics/tecdoc_health_report_min.sql
       (DB-side diagnostic — this script only PRINTS the mysql command;
        the actual DB run happens outside because we don't ship DB creds
        in the repo)

  After the audit finishes it prints the mysql command the operator needs to
  run for the DB half, then generates a combined markdown report at
  docs/reports/{date}-engine-health/summary.md — filled in with the audit
  numbers automatically and with placeholders for the DB output the operator
  pastes back.

  Post-sql/08 usage:
    pwsh scripts/engine-health-check.ps1 `
      -ApiUrl https://qa.ifritah.com `
      -Corpus scripts/audit/corpus-200-canary.csv

  Post-full-corpus usage:
    pwsh scripts/engine-health-check.ps1 `
      -ApiUrl https://qa.ifritah.com `
      -Corpus scripts/audit/corpus-1500-v2.csv

  Skip the API audit (DB-only day):
    pwsh scripts/engine-health-check.ps1 -SkipAudit
#>

[CmdletBinding()]
param(
    [string] $ApiUrl       = "https://qa.ifritah.com",
    [string] $Corpus       = "scripts/audit/corpus-200-canary.csv",
    [string] $Modes        = "combined",
    [string] $Enrichment   = "full",
    [string] $BaselineCsv  = "",
    [switch] $SkipAudit,
    [switch] $SkipDb,
    [string] $ReportDir    = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot   = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$Timestamp  = Get-Date -Format "yyyy-MM-dd_HHmm"
$DateStamp  = Get-Date -Format "yyyy-MM-dd"

if (-not $ReportDir) {
    $ReportDir = Join-Path $RepoRoot "docs\reports\$DateStamp-engine-health"
}
if (-not (Test-Path -LiteralPath $ReportDir)) {
    New-Item -ItemType Directory -Path $ReportDir | Out-Null
}

Write-Host ""
Write-Host "═══════════════════════════════════════════════════════════════════"
Write-Host "  ENGINE HEALTH CHECK"
Write-Host "  Report dir: $ReportDir"
Write-Host "═══════════════════════════════════════════════════════════════════"
Write-Host ""

# ─── Stage 1: API quality audit ────────────────────────────────────────────

$auditRawCsv     = $null
$auditSummaryMd  = $null
$auditSlicesCsv  = $null

if (-not $SkipAudit) {
    Write-Host "▶ Stage 1/3: API quality audit against $ApiUrl"
    Write-Host "  Corpus:     $Corpus"
    Write-Host "  Modes:      $Modes"
    Write-Host "  Enrichment: $Enrichment"
    Write-Host "  Expected runtime: ~25 min for full corpus, ~3 min for 200-canary"
    Write-Host ""

    $auditScript = Join-Path $RepoRoot "scripts\audit\audit-quality.ps1"
    if (-not (Test-Path -LiteralPath $auditScript)) {
        throw "audit script missing at $auditScript"
    }

    & pwsh -File $auditScript `
        -ApiUrl $ApiUrl `
        -InputCorpus $Corpus `
        -Modes $Modes `
        -Enrichment $Enrichment

    # Locate the freshest raw CSV — audit-quality.ps1 stamps its own timestamp
    $auditRawCsv = Get-ChildItem -Path (Join-Path $RepoRoot "scripts\audit\qa-quality-raw-*.csv") |
                   Sort-Object LastWriteTime -Descending |
                   Select-Object -First 1

    if ($null -eq $auditRawCsv) {
        throw "audit-quality.ps1 completed but produced no qa-quality-raw-*.csv"
    }

    Write-Host ""
    Write-Host "▶ Stage 2/3: analyze audit output"
    Write-Host "  Raw CSV: $($auditRawCsv.Name)"
    Write-Host ""

    $analyzeScript = Join-Path $RepoRoot "scripts\audit\analyze-quality.ps1"
    if (-not (Test-Path -LiteralPath $analyzeScript)) {
        throw "analyzer script missing at $analyzeScript"
    }

    & pwsh -File $analyzeScript -InputCSV $auditRawCsv.FullName

    # Analyze emits summary + per-slice/per-category CSVs alongside the raw
    $auditSummaryMd = Get-ChildItem -Path (Join-Path $RepoRoot "scripts\audit\qa-quality-summary-*.md") |
                      Sort-Object LastWriteTime -Descending | Select-Object -First 1
    $auditSlicesCsv = Get-ChildItem -Path (Join-Path $RepoRoot "scripts\audit\qa-quality-by-slice-*.csv") |
                      Sort-Object LastWriteTime -Descending | Select-Object -First 1
} else {
    Write-Host "▶ Stage 1/3: API audit SKIPPED (-SkipAudit)"
}

# ─── Stage 3: DB diagnostic instructions ───────────────────────────────────

Write-Host ""
Write-Host "▶ Stage 3/3: TecDoc MySQL diagnostic"
Write-Host ""

$dbInstructions = @"
The DB-side diagnostic runs outside this script (creds live in your ops
env, not the repo). Run the two SQL files against your TecDoc MySQL,
in this exact order, and save the outputs into the report dir:

  1. Apply the hotfix migration (idempotent):

     mysql --host=<your-tecdoc-mysql> --user=<user> --password --database=<db> \
           < sql/08_articlecriteria_criteria_value_hotfix.sql

  2. Run the minimal 30-second diagnostic:

     mysql --host=<your-tecdoc-mysql> --user=<user> --password --database=<db> \
           < scripts/diagnostics/tecdoc_health_report_min.sql \
           > $ReportDir\tecdoc-min-$Timestamp.txt

The minimal diagnostic answers 7 questions in ~30 seconds:
  A. Do the 19 real HK corpus OEMs resolve via oem_number?
  B. What REAL aftermarket brands appear per corpus OEM?
     (via articles.dataSupplierId → ambrand.brandId)
  C. Do the corpus articles have supersession chains?
  D. Do they have specs?
  E. HK vehicle catalog (Hyundai/Kia/Genesis linkage IDs)
  F. Language distribution
  G. EXPLAIN plans — G1 confirms sql/08 index is used by the planner
"@

if (-not $SkipDb) {
    Write-Host $dbInstructions
} else {
    Write-Host "  SKIPPED (-SkipDb)"
}

# ─── Combined report generation ────────────────────────────────────────────

Write-Host ""
Write-Host "▶ Generating combined report at $ReportDir\summary.md"

$summaryPath = Join-Path $ReportDir "summary.md"

$report = New-Object System.Text.StringBuilder
[void]$report.AppendLine("# Engine Health Report — $DateStamp")
[void]$report.AppendLine("")
[void]$report.AppendLine("Auto-generated by ``scripts/engine-health-check.ps1`` at ``$Timestamp``.")
[void]$report.AppendLine("")
[void]$report.AppendLine("## Environment")
[void]$report.AppendLine("")
[void]$report.AppendLine("| Setting | Value |")
[void]$report.AppendLine("|---|---|")
[void]$report.AppendLine("| API URL | ``$ApiUrl`` |")
[void]$report.AppendLine("| Corpus | ``$Corpus`` |")
[void]$report.AppendLine("| Modes | ``$Modes`` |")
[void]$report.AppendLine("| Enrichment | ``$Enrichment`` |")
[void]$report.AppendLine("| Timestamp | ``$Timestamp`` |")
[void]$report.AppendLine("")
[void]$report.AppendLine("## Stage 1: API quality audit")
[void]$report.AppendLine("")

if ($SkipAudit) {
    [void]$report.AppendLine("_Skipped via ``-SkipAudit`` flag._")
} elseif ($auditRawCsv) {
    [void]$report.AppendLine("Raw CSV: [``$($auditRawCsv.Name)``]($($auditRawCsv.FullName))")
    if ($auditSummaryMd) {
        [void]$report.AppendLine("Summary: [``$($auditSummaryMd.Name)``]($($auditSummaryMd.FullName))")
    }
    if ($auditSlicesCsv) {
        [void]$report.AppendLine("Per-slice CSV: [``$($auditSlicesCsv.Name)``]($($auditSlicesCsv.FullName))")
    }
    [void]$report.AppendLine("")
    if ($auditSummaryMd -and (Test-Path -LiteralPath $auditSummaryMd.FullName)) {
        [void]$report.AppendLine("### Summary (embedded)")
        [void]$report.AppendLine("")
        Get-Content -LiteralPath $auditSummaryMd.FullName | ForEach-Object {
            [void]$report.AppendLine($_)
        }
    }
} else {
    [void]$report.AppendLine("_Audit ran but no output found — check errors above._")
}

[void]$report.AppendLine("")
[void]$report.AppendLine("## Stage 2: TecDoc MySQL diagnostic")
[void]$report.AppendLine("")

if ($SkipDb) {
    [void]$report.AppendLine("_Skipped via ``-SkipDb`` flag._")
} else {
    [void]$report.AppendLine("Run the SQL files listed in the console output. Paste the output into this")
    [void]$report.AppendLine("report between the fences below (or attach the ``.txt`` file to the PR):")
    [void]$report.AppendLine("")
    [void]$report.AppendLine("<!-- BEGIN tecdoc-min output -->")
    [void]$report.AppendLine('```')
    [void]$report.AppendLine("(paste tecdoc-min-$Timestamp.txt here)")
    [void]$report.AppendLine('```')
    [void]$report.AppendLine("<!-- END tecdoc-min output -->")
}

[void]$report.AppendLine("")
[void]$report.AppendLine("## Stage 3: Delta vs baseline")
[void]$report.AppendLine("")

if ($BaselineCsv -and (Test-Path -LiteralPath $BaselineCsv)) {
    [void]$report.AppendLine("Baseline: ``$BaselineCsv``")
    [void]$report.AppendLine("")
    [void]$report.AppendLine("Compute the deltas manually (or write a comparator) — the audit summary")
    [void]$report.AppendLine("above already contains the current numbers; diff against the baseline for:")
    [void]$report.AppendLine("")
    [void]$report.AppendLine("- ``F1_correct`` overall + seeded slice")
    [void]$report.AppendLine("- ``AvgAM_correct`` on wear parts")
    [void]$report.AppendLine("- ``AvgOEMxRef_correct``")
    [void]$report.AppendLine("- ``F1_rich5`` on wear parts")
    [void]$report.AppendLine("- Non-HK guard leak count")
    [void]$report.AppendLine("- Timeout count per 100")
} else {
    [void]$report.AppendLine("_No baseline provided — pass ``-BaselineCsv path\to\by-slice.csv`` to compare._")
}

[void]$report.AppendLine("")
[void]$report.AppendLine("## Next actions")
[void]$report.AppendLine("")
[void]$report.AppendLine("Based on this run's findings, list the follow-up tasks and their owner:")
[void]$report.AppendLine("")
[void]$report.AppendLine("- [ ] ...")
[void]$report.AppendLine("- [ ] ...")

Set-Content -LiteralPath $summaryPath -Value $report.ToString() -Encoding UTF8

Write-Host ""
Write-Host "═══════════════════════════════════════════════════════════════════"
Write-Host "  ENGINE HEALTH CHECK COMPLETE"
Write-Host "  Combined report: $summaryPath"
Write-Host "═══════════════════════════════════════════════════════════════════"
Write-Host ""
