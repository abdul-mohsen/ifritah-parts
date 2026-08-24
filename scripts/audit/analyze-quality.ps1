# analyze-quality.ps1 - Search-engine quality analyzer
#
# Reads the raw CSV from audit-quality.ps1, emits FIVE dated output files
# with a metric set built around what a parts seller actually cares about:
#
# TIER 1 - CORRECTNESS (hard requirement, must hit >= 95%)
#   F1_correct = hit + description matches ExpectedCategory tokens.
#                Zero tolerance for wrong-category returns. This is the
#                "I don't get wrong parts" bar the user prioritizes.
#
# TIER 2 - REPLACEMENT RICHNESS (graded - more is better, one is weak)
#   AvgRepl_correct = average total replacements per correct-part hit.
#                     Higher is better. Target: >= 5 on wear parts.
#   AvgAM_correct   = same, aftermarket only.
#   AvgOEMx_correct = same, OEM cross-refs only.
#
# TIER 3 - RICHNESS BARS (F1s at replacement-count thresholds)
#   F1_rich3  = correct part + >= 3 total replacements
#   F1_rich5  = correct part + >= 5 total replacements
#   F1_rich10 = correct part + >= 10 total replacements
#
# Definitions:
#   Total replacements = AftermarketCount + OEMNumbersCount
#                        + (1 if HasSupersession else 0)
#
# Why not one F1? Because a parts seller getting "correct part + 1 mediocre
# alternative" is very different from "correct part + 12 aftermarket brands
# they can choose from on price / stock". The old binary F1_repl treated
# them the same. This layered set makes the richness gap visible so the
# next PR can target it directly.
#
# Outputs (all dated yyyy-MM-dd_HHmm):
#   qa-quality-by-category-<date>.csv   - ALL categories, all metrics
#   qa-quality-by-system-<date>.csv     - per ExpectedSystem
#   qa-quality-by-slice-<date>.csv      - per corpus slice
#   qa-quality-failures-<date>.csv      - every OEM that failed F1_correct
#   qa-quality-summary-<date>.txt       - human-readable overview

param(
  [Parameter(Mandatory=$true)]
  [string]$InputCSV,
  [string]$OutputDir = "C:\Users\ALMAAB~1\AppData\Local\Temp\opencode",
  [int]$MinTokensMatch = 2
)

if (-not (Test-Path $InputCSV)) {
  Write-Error "Input CSV not found: $InputCSV"
  exit 1
}

$dateStamp = Get-Date -Format "yyyy-MM-dd_HHmm"
$catFile   = Join-Path $OutputDir "qa-quality-by-category-$dateStamp.csv"
$sysFile   = Join-Path $OutputDir "qa-quality-by-system-$dateStamp.csv"
$sliceFile = Join-Path $OutputDir "qa-quality-by-slice-$dateStamp.csv"
$stratFile = Join-Path $OutputDir "qa-quality-by-strategy-$dateStamp.csv"
$stratSliceFile = Join-Path $OutputDir "qa-quality-by-strategy-slice-$dateStamp.csv"
$failFile  = Join-Path $OutputDir "qa-quality-failures-$dateStamp.csv"
$summFile  = Join-Path $OutputDir "qa-quality-summary-$dateStamp.txt"

$rows = Import-Csv $InputCSV
Write-Host "Loaded $($rows.Count) rows from $InputCSV"

function Get-F1($tp, $fp, $fn) {
  if ($tp -eq 0) { return @{ P=0.0; R=0.0; F1=0.0 } }
  $p = $tp / ($tp + $fp)
  $r = $tp / ($tp + $fn)
  $f1 = if (($p + $r) -gt 0) { 2 * $p * $r / ($p + $r) } else { 0.0 }
  return @{ P = [Math]::Round($p, 3); R = [Math]::Round($r, 3); F1 = [Math]::Round($f1, 3) }
}

function Test-Tokens($desc, $goodTokens, $minMatch) {
  if ([string]::IsNullOrWhiteSpace($goodTokens)) { return $true }
  if ([string]::IsNullOrWhiteSpace($desc)) { return $false }
  $tokens = $goodTokens -split "," | ForEach-Object { $_.Trim().ToLower() } | Where-Object { $_ }
  if ($tokens.Count -eq 0) { return $true }
  $descLower = $desc.ToLower()
  $matches = ($tokens | Where-Object { $descLower.Contains($_) }).Count
  return $matches -ge [Math]::Min($minMatch, $tokens.Count)
}

# Classify every row
$classified = $rows | ForEach-Object {
  $r = $_
  $hasResult = [int]$r.Total -gt 0
  $partOK    = $hasResult -and (Test-Tokens $r.FirstDesc $r.GoodTokens $MinTokensMatch)
  $amCount   = [int]$r.AftermarketCount
  $oemCount  = [int]$r.OEMNumbersCount
  $hasSup    = $r.HasSupersession -eq "true"
  $replTotal = $amCount + $oemCount + ($(if ($hasSup) { 1 } else { 0 }))
  $isTimeout = ($r.IsTimeout -eq "True") -or ([double]$r.ElapsedS -ge 14)

  # F1_hit
  $hitClass = switch ($r.GroundTruth) {
    "exists"        { if ($hasResult) { "TP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }
  # F1_correct - hit + right category (the hard requirement)
  $correctClass = switch ($r.GroundTruth) {
    "exists"        { if ($partOK) { "TP" } elseif ($hasResult) { "FP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }
  # F1_richN - correct part + >= N total replacements
  function RichClass($needCorrect, $needRepl) {
    if ($r.GroundTruth -eq "exists") {
      if ($needCorrect -and $replTotal -ge $needRepl) { return "TP" }
      elseif ($hasResult) { return "FP" }
      else { return "FN" }
    } elseif ($hasResult) {
      return "FP"
    } else {
      return "TN"
    }
  }
  # PowerShell can't create nested closures cleanly - inline the three tiers.
  $rich3Class = if ($r.GroundTruth -eq "exists") {
                   if ($partOK -and $replTotal -ge 3)  { "TP" } elseif ($hasResult) { "FP" } else { "FN" }
                 } elseif ($hasResult) { "FP" } else { "TN" }
  $rich5Class = if ($r.GroundTruth -eq "exists") {
                   if ($partOK -and $replTotal -ge 5)  { "TP" } elseif ($hasResult) { "FP" } else { "FN" }
                 } elseif ($hasResult) { "FP" } else { "TN" }
  $rich10Class= if ($r.GroundTruth -eq "exists") {
                   if ($partOK -and $replTotal -ge 10) { "TP" } elseif ($hasResult) { "FP" } else { "FN" }
                 } elseif ($hasResult) { "FP" } else { "TN" }

  $failReason = ""
  if ($correctClass -eq "FP") {
    $failReason = "WRONG-PART: expected '$($r.GoodTokens)' but got '$($r.FirstDesc)'"
  } elseif ($correctClass -eq "FN") {
    if ($isTimeout)                       { $failReason = "TIMEOUT at $($r.ElapsedS)s" }
    elseif ([int]$r.HttpStatus -ne 200)   { $failReason = "HTTP $($r.HttpStatus)" }
    else                                  { $failReason = "no results" }
  } elseif ($correctClass -eq "TP" -and $replTotal -lt 3) {
    $failReason = "correct-but-poor: only $replTotal replacements (AM=$amCount OEMxRef=$oemCount sup=$hasSup)"
  }

  [PSCustomObject]@{
    OEM              = $r.OEM
    Slice            = $r.Slice
    GroundTruth      = $r.GroundTruth
    ExpectedSystem   = $r.ExpectedSystem
    ExpectedCategory = $r.ExpectedCategory
    GoodTokens       = $r.GoodTokens
    Mode             = $r.Mode
    ElapsedS         = [double]$r.ElapsedS
    IsTimeout        = $isTimeout
    Total            = [int]$r.Total
    FirstDesc        = $r.FirstDesc
    FirstBrand       = $r.FirstBrand
    FirstCategory    = $r.FirstCategory
    HitClass         = $hitClass
    CorrectClass     = $correctClass
    Rich3Class       = $rich3Class
    Rich5Class       = $rich5Class
    Rich10Class      = $rich10Class
    HasHit           = $hasResult
    PartOK           = $partOK
    AftermarketCount = $amCount
    OEMNumbersCount  = $oemCount
    HasSupersession  = $hasSup
    ReplTotal        = $replTotal
    AftermarketSample= $r.AftermarketSample
    OEMNumbersSample = $r.OEMNumbersSample
    SpecsCount       = [int]$r.SpecsCount
    VehiclesCount    = [int]$r.VehiclesCount
    FailReason       = $failReason
  }
}

function Compute-Metrics($group) {
  $hitTP = ($group | Where-Object { $_.HitClass -eq "TP" }).Count
  $hitFP = ($group | Where-Object { $_.HitClass -eq "FP" }).Count
  $hitFN = ($group | Where-Object { $_.HitClass -eq "FN" }).Count

  $corrTP = ($group | Where-Object { $_.CorrectClass -eq "TP" }).Count
  $corrFP = ($group | Where-Object { $_.CorrectClass -eq "FP" }).Count
  $corrFN = ($group | Where-Object { $_.CorrectClass -eq "FN" }).Count

  $r3TP = ($group | Where-Object { $_.Rich3Class -eq "TP" }).Count
  $r3FP = ($group | Where-Object { $_.Rich3Class -eq "FP" }).Count
  $r3FN = ($group | Where-Object { $_.Rich3Class -eq "FN" }).Count
  $r5TP = ($group | Where-Object { $_.Rich5Class -eq "TP" }).Count
  $r5FP = ($group | Where-Object { $_.Rich5Class -eq "FP" }).Count
  $r5FN = ($group | Where-Object { $_.Rich5Class -eq "FN" }).Count
  $r10TP= ($group | Where-Object { $_.Rich10Class -eq "TP" }).Count
  $r10FP= ($group | Where-Object { $_.Rich10Class -eq "FP" }).Count
  $r10FN= ($group | Where-Object { $_.Rich10Class -eq "FN" }).Count

  # Averages on correct-part hits only (that's when replacement richness is meaningful)
  $correctHits = $group | Where-Object { $_.CorrectClass -eq "TP" }
  $avgRepl = 0.0; $avgAM = 0.0; $avgOEMx = 0.0
  if ($correctHits.Count -gt 0) {
    $avgRepl  = [Math]::Round(($correctHits.ReplTotal        | Measure-Object -Average).Average, 2)
    $avgAM    = [Math]::Round(($correctHits.AftermarketCount | Measure-Object -Average).Average, 2)
    $avgOEMx  = [Math]::Round(($correctHits.OEMNumbersCount  | Measure-Object -Average).Average, 2)
  }

  return @{
    Hit     = Get-F1 $hitTP  $hitFP  $hitFN
    Correct = Get-F1 $corrTP $corrFP $corrFN
    Rich3   = Get-F1 $r3TP   $r3FP   $r3FN
    Rich5   = Get-F1 $r5TP   $r5FP   $r5FN
    Rich10  = Get-F1 $r10TP  $r10FP  $r10FN
    AvgRepl = $avgRepl
    AvgAM   = $avgAM
    AvgOEMx = $avgOEMx
    CorrectTP = $corrTP
  }
}

# Per-category
$catRows = $classified | Group-Object ExpectedCategory | ForEach-Object {
  $g = $_.Group
  $existsCount = ($g | Where-Object { $_.GroundTruth -eq "exists" }).Count
  $m = Compute-Metrics $g
  [PSCustomObject]@{
    ExpectedCategory  = if ($_.Name) { $_.Name } else { "<blank>" }
    N                 = $g.Count
    N_exists          = $existsCount
    F1_hit            = $m.Hit.F1
    F1_correct        = $m.Correct.F1
    AvgRepl_correct   = $m.AvgRepl
    AvgAM_correct     = $m.AvgAM
    AvgOEMxRef_correct= $m.AvgOEMx
    F1_rich3          = $m.Rich3.F1
    F1_rich5          = $m.Rich5.F1
    F1_rich10         = $m.Rich10.F1
    correct_TP        = $m.CorrectTP
  }
} | Sort-Object N -Descending
$catRows | Export-Csv $catFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-category CSV: $catFile ($($catRows.Count) categories)"

# Per-system
$sysRows = $classified | Group-Object ExpectedSystem | ForEach-Object {
  $g = $_.Group
  $m = Compute-Metrics $g
  [PSCustomObject]@{
    ExpectedSystem    = if ($_.Name) { $_.Name } else { "<blank>" }
    N                 = $g.Count
    F1_hit            = $m.Hit.F1
    F1_correct        = $m.Correct.F1
    AvgRepl_correct   = $m.AvgRepl
    AvgAM_correct     = $m.AvgAM
    AvgOEMxRef_correct= $m.AvgOEMx
    F1_rich3          = $m.Rich3.F1
    F1_rich5          = $m.Rich5.F1
    F1_rich10         = $m.Rich10.F1
  }
} | Sort-Object N -Descending
$sysRows | Export-Csv $sysFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-system CSV:   $sysFile ($($sysRows.Count) systems)"

# Per-slice
$sliceRows = $classified | Group-Object Slice | ForEach-Object {
  $g = $_.Group
  $m = Compute-Metrics $g
  [PSCustomObject]@{
    Slice             = $_.Name
    N                 = $g.Count
    F1_hit            = $m.Hit.F1
    F1_correct        = $m.Correct.F1
    AvgRepl_correct   = $m.AvgRepl
    AvgAM_correct     = $m.AvgAM
    AvgOEMxRef_correct= $m.AvgOEMx
    F1_rich3          = $m.Rich3.F1
    F1_rich5          = $m.Rich5.F1
    F1_rich10         = $m.Rich10.F1
  }
} | Sort-Object N -Descending
$sliceRows | Export-Csv $sliceFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-slice CSV:    $sliceFile"

# Per-strategy — treats Mode column as the strategy identifier. Enables
# per-strategy F1 matrix across the corpus so we can see which strategies
# are pulling their weight (cache/legacy/exact_oem) vs which are broken
# (owned_catalog/supersession/vin_assembly at time of writing).
$stratRows = $classified | Group-Object Mode | ForEach-Object {
  $g = $_.Group
  $m = Compute-Metrics $g
  [PSCustomObject]@{
    Strategy          = $_.Name
    N                 = $g.Count
    F1_hit            = $m.Hit.F1
    F1_correct        = $m.Correct.F1
    AvgRepl_correct   = $m.AvgRepl
    AvgAM_correct     = $m.AvgAM
    AvgOEMxRef_correct= $m.AvgOEMx
    F1_rich5          = $m.Rich5.F1
    F1_rich10         = $m.Rich10.F1
  }
} | Sort-Object F1_correct -Descending
$stratRows | Export-Csv $stratFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-strategy CSV: $stratFile ($($stratRows.Count) strategies)"

# Per-strategy × per-slice matrix. Answers "which strategy handles which
# corpus segment best?" — the map that guides per-strategy improvement
# tasks in M0.
$stratSliceRows = $classified | Group-Object Mode, Slice | ForEach-Object {
  $g = $_.Group
  $m = Compute-Metrics $g
  $mode = $g[0].Mode
  $slice = $g[0].Slice
  [PSCustomObject]@{
    Strategy         = $mode
    Slice            = $slice
    N                = $g.Count
    F1_hit           = $m.Hit.F1
    F1_correct       = $m.Correct.F1
    AvgRepl_correct  = $m.AvgRepl
    F1_rich5         = $m.Rich5.F1
  }
} | Sort-Object Strategy, Slice
$stratSliceRows | Export-Csv $stratSliceFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-strategy x per-slice CSV: $stratSliceFile"

# Failures = anything that missed F1_correct OR was correct but replTotal < 3
$failures = $classified | Where-Object {
  $_.CorrectClass -in @("FN","FP") -or ($_.CorrectClass -eq "TP" -and $_.ReplTotal -lt 3)
} | Select-Object OEM, Slice, GroundTruth, ExpectedCategory, ExpectedSystem, GoodTokens,
                  CorrectClass, FailReason, Total, FirstDesc, FirstBrand, FirstCategory,
                  AftermarketCount, OEMNumbersCount, HasSupersession, ReplTotal,
                  ElapsedS, IsTimeout
$failures | Export-Csv $failFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote failures CSV:     $failFile ($($failures.Count) rows)"

# Summary
$overall = Compute-Metrics $classified

$topByRepl = $catRows | Where-Object { $_.N_exists -ge 5 } |
             Sort-Object { [double]$_.AvgRepl_correct } -Descending |
             Select-Object -First 15 ExpectedCategory, N_exists, F1_correct, AvgRepl_correct, AvgAM_correct, AvgOEMxRef_correct, F1_rich5, F1_rich10

$correctBelow95 = $catRows | Where-Object { $_.F1_correct -lt 0.95 -and $_.N_exists -ge 5 }
$correctAt95 = $catRows | Where-Object { $_.F1_correct -ge 0.95 -and $_.N_exists -ge 5 }

$summary = @"
=====================================================================
QA SEARCH QUALITY REPORT - $dateStamp
=====================================================================
Source: $InputCSV
Rows:   $($rows.Count)

--- Ground truth distribution ---
$($classified | Group-Object GroundTruth | ForEach-Object { "  {0,-18} n={1}" -f $_.Name, $_.Count } | Out-String)

--- OVERALL ---
  TIER 1 - CORRECTNESS (target: F1_correct >= 0.95)
    F1_hit      = $($overall.Hit.F1)   (returned any result)
    F1_correct  = $($overall.Correct.F1)   (returned RIGHT-category part - hard requirement)

  TIER 2 - RICHNESS (target: AvgRepl_correct >= 5 on wear parts)
    AvgRepl_correct  = $($overall.AvgRepl) replacements per correct hit (aftermarket + OEMxRef + supersession)
    AvgAM_correct    = $($overall.AvgAM) aftermarket brands per correct hit
    AvgOEMxRef_correct = $($overall.AvgOEMx) TecDoc OEM cross-refs per correct hit

  TIER 3 - RICHNESS BARS
    F1_rich3   = $($overall.Rich3.F1)   (correct part + >= 3 replacements)
    F1_rich5   = $($overall.Rich5.F1)   (correct part + >= 5 replacements)
    F1_rich10  = $($overall.Rich10.F1)  (correct part + >= 10 replacements)

--- Per-slice ---
$($sliceRows | Format-Table -AutoSize | Out-String)

--- Per-strategy (all rows, all slices) ---
$($stratRows | Format-Table -AutoSize | Out-String)

--- Per-strategy x per-slice (top interactions) ---
$($stratSliceRows | Sort-Object F1_correct -Descending | Select-Object -First 25 | Format-Table -AutoSize | Out-String)

--- Per-category, top 15 by average replacements (n_exists >= 5) ---
$($topByRepl | Format-Table -AutoSize | Out-String)

--- Categories BELOW F1_correct = 0.95 (n_exists >= 5, top 20) ---
$($correctBelow95 | Sort-Object N_exists -Descending | Select-Object -First 20 ExpectedCategory, N_exists, F1_correct, AvgRepl_correct | Format-Table -AutoSize | Out-String)

--- Category counts ---
  Total distinct categories:                                 $($catRows.Count)
  Meeting F1_correct >= 0.95 (n_exists >= 5):               $($correctAt95.Count)
  Below F1_correct 0.95 (the "wrong parts" problem):         $($correctBelow95.Count)

--- Files ---
  by-category: $catFile
  by-system:   $sysFile
  by-slice:    $sliceFile
  failures:    $failFile
"@

$summary | Out-File $summFile -Encoding utf8
Write-Host ""
Write-Host $summary
Write-Host ""
Write-Host "Summary saved: $summFile"
