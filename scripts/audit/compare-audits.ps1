# compare-audits.ps1 - Compares two by-slice.csv files from analyze-quality.ps1
# and returns whether F1_correct regressed >= threshold on a chosen slice.
#
# Used by .github/workflows/pr-quality-gate.yml (M6.S1.T2) to block PRs that
# touch internal/service/** and cause the search-quality canary to drop.
#
# Input format: the CSVs produced by scripts/audit/analyze-quality.ps1's
# Per-slice section. Columns required:
#   Slice, N, F1_hit, F1_correct, AvgRepl_correct, AvgAM_correct,
#   AvgOEMxRef_correct, F1_rich3, F1_rich5, F1_rich10
#
# Exit codes:
#   0 = F1_correct held or improved (delta >= -threshold)
#   1 = F1_correct regressed by more than threshold (block the PR)
#   2 = corpus mismatch (missing files, missing slice row, unparseable CSV)
#
# Usage:
#   pwsh -File compare-audits.ps1 -BaselineCsv baseline.csv -CandidateCsv pr.csv
#   pwsh -File compare-audits.ps1 -BaselineCsv b.csv -CandidateCsv p.csv -Slice real_hk_seeded
#   pwsh -File compare-audits.ps1 -BaselineCsv b.csv -CandidateCsv p.csv -VerboseOutput
#
# Pure PowerShell. No external tools.

param(
  [Parameter(Mandatory=$true)]
  [string]$BaselineCsv,

  [Parameter(Mandatory=$true)]
  [string]$CandidateCsv,

  [double]$RegressionThreshold = 0.02,

  # Which slice to gate on. Default matches the canary corpus's
  # "real_hk_seeded" slice — the 100 known-good OEMs the seller cares
  # about most. Other slices in the corpus: real_hk_coarse,
  # real_hk_unseeded, plausible_hk, non_hk.
  [string]$Slice = "real_hk_seeded",

  [switch]$VerboseOutput
)

$ErrorActionPreference = 'Stop'

function Write-Info($msg)  { Write-Host $msg }
function Write-Warn($msg)  { Write-Host "WARN: $msg" -ForegroundColor Yellow }
function Write-Err($msg)   { Write-Host "ERROR: $msg" -ForegroundColor Red }

# ---------- Load CSVs (exit 2 on any structural problem) ----------
function Load-SliceCsv {
  param([string]$Path, [string]$Label)

  if (-not (Test-Path -LiteralPath $Path)) {
    Write-Err "$Label CSV not found: $Path"
    return $null
  }
  # Zero-byte or /dev/null-style file → treat as corpus-mismatch, not crash.
  $len = 0
  try { $len = (Get-Item -LiteralPath $Path).Length } catch { $len = 0 }
  if ($len -eq 0) {
    Write-Err "$Label CSV is empty: $Path"
    return $null
  }
  try {
    $rows = Import-Csv -LiteralPath $Path
  } catch {
    Write-Err "$Label CSV could not be parsed: $Path ($($_.Exception.Message))"
    return $null
  }
  if ($null -eq $rows -or $rows.Count -eq 0) {
    Write-Err "$Label CSV has no rows: $Path"
    return $null
  }
  # Structural check: must have Slice + F1_correct columns.
  $first = $rows[0]
  $props = @($first.PSObject.Properties.Name)
  foreach ($required in @('Slice','F1_correct')) {
    if ($props -notcontains $required) {
      Write-Err "$Label CSV missing required column '$required': $Path"
      Write-Err "  Actual columns: $($props -join ', ')"
      return $null
    }
  }
  return $rows
}

$baselineRows  = Load-SliceCsv -Path $BaselineCsv  -Label 'Baseline'
$candidateRows = Load-SliceCsv -Path $CandidateCsv -Label 'Candidate'

if ($null -eq $baselineRows -or $null -eq $candidateRows) {
  Write-Err "corpus mismatch - cannot compare (see errors above)"
  exit 2
}

# ---------- Extract the row for -Slice from each ----------
function Get-SliceRow {
  param($Rows, [string]$SliceName)
  return @($Rows | Where-Object { $_.Slice -eq $SliceName })[0]
}

$baseRow = Get-SliceRow -Rows $baselineRows  -SliceName $Slice
$candRow = Get-SliceRow -Rows $candidateRows -SliceName $Slice

if ($null -eq $baseRow -or $null -eq $candRow) {
  Write-Err "corpus mismatch - slice '$Slice' missing in one or both CSVs"
  Write-Err "  Baseline slices:  $(($baselineRows.Slice  | Sort-Object -Unique) -join ', ')"
  Write-Err "  Candidate slices: $(($candidateRows.Slice | Sort-Object -Unique) -join ', ')"
  exit 2
}

# Parse F1_correct as double (analyze-quality.ps1 writes 3-decimal strings).
$baseF1 = 0.0
$candF1 = 0.0
try { $baseF1 = [double]$baseRow.F1_correct } catch {
  Write-Err "corpus mismatch - baseline F1_correct not numeric: '$($baseRow.F1_correct)'"
  exit 2
}
try { $candF1 = [double]$candRow.F1_correct } catch {
  Write-Err "corpus mismatch - candidate F1_correct not numeric: '$($candRow.F1_correct)'"
  exit 2
}

$delta = [Math]::Round($candF1 - $baseF1, 4)
$deltaSign = if ($delta -ge 0) { '+' } else { '' }

# ---------- Optional verbose full-comparison table ----------
if ($VerboseOutput) {
  Write-Info ""
  Write-Info "=== Full per-slice comparison ==="
  Write-Info ("{0,-20} {1,-8} {2,-10} {3,-10} {4,-10}" -f 'Slice','Metric','Baseline','Candidate','Delta')
  Write-Info ("-" * 60)

  # Which metrics we surface in verbose mode. Ordered by the seller's
  # priorities: correctness first, then richness bars.
  $metrics = @('F1_hit','F1_correct','AvgRepl_correct','AvgAM_correct',
               'AvgOEMxRef_correct','F1_rich3','F1_rich5','F1_rich10')

  $allSlices = @(($baselineRows.Slice + $candidateRows.Slice) | Sort-Object -Unique)
  foreach ($s in $allSlices) {
    $bR = Get-SliceRow -Rows $baselineRows  -SliceName $s
    $cR = Get-SliceRow -Rows $candidateRows -SliceName $s
    foreach ($m in $metrics) {
      $bVal = if ($bR -and $bR.PSObject.Properties.Match($m).Count -gt 0) { $bR.$m } else { 'n/a' }
      $cVal = if ($cR -and $cR.PSObject.Properties.Match($m).Count -gt 0) { $cR.$m } else { 'n/a' }
      $d = ''
      try {
        if ($bVal -ne 'n/a' -and $cVal -ne 'n/a') {
          $dv = [Math]::Round([double]$cVal - [double]$bVal, 4)
          $ds = if ($dv -ge 0) { '+' } else { '' }
          $d = "$ds$dv"
        }
      } catch { $d = '' }
      Write-Info ("{0,-20} {1,-8} {2,-10} {3,-10} {4,-10}" -f $s, $m, $bVal, $cVal, $d)
    }
    Write-Info ""
  }
}

# ---------- Verdict ----------
Write-Info ""
Write-Info "=== PR quality gate: slice '$Slice' ==="
Write-Info ("  baseline F1_correct  = {0}" -f $baseF1)
Write-Info ("  candidate F1_correct = {0}" -f $candF1)
Write-Info ("  delta                = {0}{1}" -f $deltaSign, $delta)
Write-Info ("  threshold            = {0}" -f $RegressionThreshold)

if ($delta -lt (-1 * $RegressionThreshold)) {
  Write-Info ""
  Write-Err "REGRESSION: F1_correct on '$Slice' dropped by $([Math]::Abs($delta)) (threshold $RegressionThreshold)"
  Write-Err "  baseline  = $baseF1"
  Write-Err "  candidate = $candF1"
  Write-Err "  delta     = $delta"
  exit 1
}

Write-Info ""
if ($delta -ge 0) {
  Write-Info "PASS: F1_correct held or improved (delta=$deltaSign$delta)"
} else {
  Write-Info "PASS: F1_correct within tolerance (delta=$delta, threshold=$RegressionThreshold)"
}
exit 0
