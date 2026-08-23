# analyze-quality.ps1 - Search-engine quality analyzer
#
# Reads the raw CSV from audit-quality.ps1, emits FOUR dated output files:
#
#   qa-quality-by-category-<date>.csv - one row per ExpectedCategory, ALL of them
#   qa-quality-by-system-<date>.csv   - one row per ExpectedSystem
#   qa-quality-by-slice-<date>.csv    - one row per corpus slice
#   qa-quality-failures-<date>.csv    - every OEM that failed, with reason
#   qa-quality-summary-<date>.txt     - human-readable overview
#
# Classification per row (GroundTruth-aware):
#   TP  - exists row + hit + description contains >=2 GoodTokens
#   FPc - exists row + hit + description missed GoodTokens (wrong category)
#   FPe - not_hk_format row returned results (should have been empty)
#   FPn - non_hk row returned results (guard leak - critical)
#   FN  - exists row returned zero results (missed real part)
#   TN  - non-existent OEM returned zero results (correct rejection)
#
# Also reports enrichment coverage per group:
#   Aftermarket%, Specs%, Vehicles%, Supersession%, OEMNumbers%

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
$failFile  = Join-Path $OutputDir "qa-quality-failures-$dateStamp.csv"
$summFile  = Join-Path $OutputDir "qa-quality-summary-$dateStamp.txt"

$rows = Import-Csv $InputCSV
Write-Host "Loaded $($rows.Count) rows from $InputCSV"

function Get-F1($tp,$fp,$fn) {
  if ($tp -eq 0) { return @{P=0.0; R=0.0; F1=0.0} }
  $p = $tp / ($tp + $fp)
  $r = $tp / ($tp + $fn)
  $f1 = if (($p + $r) -gt 0) { 2 * $p * $r / ($p + $r) } else { 0.0 }
  return @{P=[Math]::Round($p,3); R=[Math]::Round($r,3); F1=[Math]::Round($f1,3)}
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
  $hit = [int]$r.Total -gt 0
  $tokensOK = Test-Tokens $r.FirstDesc $r.GoodTokens $MinTokensMatch
  $isTimeout = ($r.IsTimeout -eq "True") -or ([double]$r.ElapsedS -ge 14)

  $class = switch ($r.GroundTruth) {
    "exists"        { if ($hit -and $tokensOK) { "TP" } elseif ($hit) { "FPc" } else { "FN" } }
    "not_hk_format" { if ($hit) { "FPe" } else { "TN" } }
    "non_hk"        { if ($hit) { "FPn" } else { "TN" } }
    default         { "UNK" }
  }

  $failReason = switch ($class) {
    "TP"  { "" }
    "FPc" { "wrong-category: got '$($r.FirstCategory)' / desc '$($r.FirstDesc)' but expected tokens '$($r.GoodTokens)'" }
    "FPe" { "not_hk_format leaked (n=$($r.Total))" }
    "FPn" { "NON-HK LEAK: '$($r.OEM)' returned $($r.Total) results (expected empty)" }
    "FN"  {
      if ($isTimeout) { "TIMEOUT at ${$r.ElapsedS}s" }
      elseif ([int]$r.HttpStatus -ne 200) { "HTTP $($r.HttpStatus)" }
      else { "no results" }
    }
    "TN"  { "" }
    default { "unclassified" }
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
    TokensOK         = $tokensOK
    Class            = $class
    FailReason       = $failReason
    AftermarketCount = [int]$r.AftermarketCount
    AftermarketSample= $r.AftermarketSample
    SpecsCount       = [int]$r.SpecsCount
    VehiclesCount    = [int]$r.VehiclesCount
    HasSupersession  = $r.HasSupersession -eq "true"
    OEMNumbersCount  = [int]$r.OEMNumbersCount
    OEMNumbersSample = $r.OEMNumbersSample
    Warnings         = $r.Warnings
  }
}

# ------------ Per-category CSV (ALL categories, no top-N cut) ------------
$catRows = $classified | Group-Object ExpectedCategory | ForEach-Object {
  $g = $_.Group
  $existsOnly = $g | Where-Object { $_.GroundTruth -eq "exists" }
  $tp  = ($g | Where-Object { $_.Class -eq "TP" }).Count
  $fpc = ($g | Where-Object { $_.Class -eq "FPc" }).Count
  $fpe = ($g | Where-Object { $_.Class -eq "FPe" }).Count
  $fpn = ($g | Where-Object { $_.Class -eq "FPn" }).Count
  $fp  = $fpc + $fpe + $fpn
  $fn  = ($g | Where-Object { $_.Class -eq "FN" }).Count
  $tn  = ($g | Where-Object { $_.Class -eq "TN" }).Count
  $prf = Get-F1 $tp $fp $fn

  $amHits    = ($g | Where-Object { $_.AftermarketCount -gt 0 }).Count
  $specsHits = ($g | Where-Object { $_.SpecsCount -gt 0 }).Count
  $vehHits   = ($g | Where-Object { $_.VehiclesCount -gt 0 }).Count
  $supHits   = ($g | Where-Object { $_.HasSupersession }).Count
  $oemHits   = ($g | Where-Object { $_.OEMNumbersCount -gt 0 }).Count
  $hitCount  = ($g | Where-Object { $_.Total -gt 0 }).Count

  [PSCustomObject]@{
    ExpectedCategory = if ($_.Name) { $_.Name } else { "<blank>" }
    N                = $g.Count
    N_exists         = $existsOnly.Count
    TP               = $tp
    FP_wrong_cat     = $fpc
    FP_empty_leak    = $fpe
    FP_non_hk_leak   = $fpn
    FN               = $fn
    TN               = $tn
    Precision        = $prf.P
    Recall           = $prf.R
    F1               = $prf.F1
    Hits             = $hitCount
    Aftermarket_pct  = if ($hitCount -gt 0) { [Math]::Round(100.0 * $amHits / $hitCount, 1) } else { 0 }
    Specs_pct        = if ($hitCount -gt 0) { [Math]::Round(100.0 * $specsHits / $hitCount, 1) } else { 0 }
    Vehicles_pct     = if ($hitCount -gt 0) { [Math]::Round(100.0 * $vehHits / $hitCount, 1) } else { 0 }
    Supersession_pct = if ($hitCount -gt 0) { [Math]::Round(100.0 * $supHits / $hitCount, 1) } else { 0 }
    OEMNumbers_pct   = if ($hitCount -gt 0) { [Math]::Round(100.0 * $oemHits / $hitCount, 1) } else { 0 }
  }
} | Sort-Object N -Descending
$catRows | Export-Csv $catFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-category CSV: $catFile ($($catRows.Count) categories)"

# ------------ Per-system CSV ------------
$sysRows = $classified | Group-Object ExpectedSystem | ForEach-Object {
  $g = $_.Group
  $tp = ($g | Where-Object { $_.Class -eq "TP" }).Count
  $fp = ($g | Where-Object { $_.Class -match "^FP" }).Count
  $fn = ($g | Where-Object { $_.Class -eq "FN" }).Count
  $tn = ($g | Where-Object { $_.Class -eq "TN" }).Count
  $prf = Get-F1 $tp $fp $fn
  $hitCount  = ($g | Where-Object { $_.Total -gt 0 }).Count
  $amHits    = ($g | Where-Object { $_.AftermarketCount -gt 0 }).Count
  $specsHits = ($g | Where-Object { $_.SpecsCount -gt 0 }).Count
  [PSCustomObject]@{
    ExpectedSystem   = if ($_.Name) { $_.Name } else { "<blank>" }
    N                = $g.Count
    TP               = $tp
    FP               = $fp
    FN               = $fn
    TN               = $tn
    Precision        = $prf.P
    Recall           = $prf.R
    F1               = $prf.F1
    Aftermarket_pct  = if ($hitCount -gt 0) { [Math]::Round(100.0 * $amHits / $hitCount, 1) } else { 0 }
    Specs_pct        = if ($hitCount -gt 0) { [Math]::Round(100.0 * $specsHits / $hitCount, 1) } else { 0 }
  }
} | Sort-Object N -Descending
$sysRows | Export-Csv $sysFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-system CSV:   $sysFile ($($sysRows.Count) systems)"

# ------------ Per-slice CSV ------------
$sliceRows = $classified | Group-Object Slice | ForEach-Object {
  $g = $_.Group
  $tp = ($g | Where-Object { $_.Class -eq "TP" }).Count
  $fp = ($g | Where-Object { $_.Class -match "^FP" }).Count
  $fn = ($g | Where-Object { $_.Class -eq "FN" }).Count
  $tn = ($g | Where-Object { $_.Class -eq "TN" }).Count
  $prf = Get-F1 $tp $fp $fn
  $hitCount  = ($g | Where-Object { $_.Total -gt 0 }).Count
  $amHits    = ($g | Where-Object { $_.AftermarketCount -gt 0 }).Count
  $specsHits = ($g | Where-Object { $_.SpecsCount -gt 0 }).Count
  $vehHits   = ($g | Where-Object { $_.VehiclesCount -gt 0 }).Count
  [PSCustomObject]@{
    Slice            = $_.Name
    N                = $g.Count
    TP               = $tp
    FP               = $fp
    FN               = $fn
    TN               = $tn
    Precision        = $prf.P
    Recall           = $prf.R
    F1               = $prf.F1
    Aftermarket_pct  = if ($hitCount -gt 0) { [Math]::Round(100.0 * $amHits / $hitCount, 1) } else { 0 }
    Specs_pct        = if ($hitCount -gt 0) { [Math]::Round(100.0 * $specsHits / $hitCount, 1) } else { 0 }
    Vehicles_pct     = if ($hitCount -gt 0) { [Math]::Round(100.0 * $vehHits / $hitCount, 1) } else { 0 }
  }
} | Sort-Object N -Descending
$sliceRows | Export-Csv $sliceFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-slice CSV:    $sliceFile"

# ------------ Failures CSV (every FN, FPc, FPe, FPn) ------------
$failures = $classified | Where-Object { $_.Class -in @("FN","FPc","FPe","FPn") } |
  Select-Object OEM, Slice, GroundTruth, ExpectedCategory, ExpectedSystem, GoodTokens, Class, FailReason, Total, FirstDesc, FirstCategory, FirstBrand, ElapsedS, IsTimeout
$failures | Export-Csv $failFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote failures CSV:     $failFile ($($failures.Count) failures)"

# ------------ Summary ------------
$totalRows = $classified.Count
$totalTP  = ($classified | Where-Object { $_.Class -eq "TP" }).Count
$totalFPc = ($classified | Where-Object { $_.Class -eq "FPc" }).Count
$totalFPe = ($classified | Where-Object { $_.Class -eq "FPe" }).Count
$totalFPn = ($classified | Where-Object { $_.Class -eq "FPn" }).Count
$totalFN  = ($classified | Where-Object { $_.Class -eq "FN" }).Count
$totalTN  = ($classified | Where-Object { $_.Class -eq "TN" }).Count
$totalFP  = $totalFPc + $totalFPe + $totalFPn
$overall  = Get-F1 $totalTP $totalFP $totalFN

$hits     = ($classified | Where-Object { $_.Total -gt 0 }).Count
$amHits   = ($classified | Where-Object { $_.AftermarketCount -gt 0 }).Count
$specsHits= ($classified | Where-Object { $_.SpecsCount -gt 0 }).Count
$vehHits  = ($classified | Where-Object { $_.VehiclesCount -gt 0 }).Count
$supHits  = ($classified | Where-Object { $_.HasSupersession }).Count
$oemHits  = ($classified | Where-Object { $_.OEMNumbersCount -gt 0 }).Count
$timeouts = ($classified | Where-Object { $_.IsTimeout }).Count

$catsPerfect = ($catRows | Where-Object { $_.F1 -ge 0.95 -and $_.N_exists -ge 5 }).Count
$catsBroken  = ($catRows | Where-Object { $_.F1 -eq 0 -and $_.N_exists -ge 3 }).Count

$summary = @"
=====================================================================
QA SEARCH QUALITY REPORT - $dateStamp
=====================================================================
Source: $InputCSV
Rows:   $totalRows

--- Ground truth distribution ---
$($classified | Group-Object GroundTruth | ForEach-Object { "  {0,-18} n={1}" -f $_.Name, $_.Count } | Out-String)
--- Overall (all rows, all slices) ---
  TP = $totalTP  FN = $totalFN
  FP (wrong-category) = $totalFPc
  FP (empty-leak)     = $totalFPe
  FP (non-HK leak)    = $totalFPn
  TN = $totalTN
  Precision = $($overall.P)   Recall = $($overall.R)   F1 = $($overall.F1)

--- Enrichment coverage (of $hits hits) ---
  Aftermarket populated:   $amHits   ($([Math]::Round(100.0 * $amHits / [Math]::Max(1,$hits), 1))%)
  Specs populated:         $specsHits   ($([Math]::Round(100.0 * $specsHits / [Math]::Max(1,$hits), 1))%)
  CompatibleVehicles:      $vehHits   ($([Math]::Round(100.0 * $vehHits / [Math]::Max(1,$hits), 1))%)
  Supersession chain:      $supHits   ($([Math]::Round(100.0 * $supHits / [Math]::Max(1,$hits), 1))%)
  OEM cross-references:    $oemHits   ($([Math]::Round(100.0 * $oemHits / [Math]::Max(1,$hits), 1))%)

--- Timeouts ---
  Rows >=14s: $timeouts   ($([Math]::Round(100.0 * $timeouts / $totalRows, 1))%)

--- Category coverage ---
  Total distinct categories: $($catRows.Count)
  F1 >= 0.95 (n_exists >= 5):  $catsPerfect
  F1 = 0    (n_exists >= 3):   $catsBroken

--- Per-slice F1 ---
$($sliceRows | Format-Table -AutoSize | Out-String)

--- Top 15 broken categories (F1=0, n_exists >= 3) ---
$($catRows | Where-Object { $_.F1 -eq 0 -and $_.N_exists -ge 3 } | Sort-Object N_exists -Descending | Select-Object -First 15 ExpectedCategory, N_exists, FP_wrong_cat, FN | Format-Table -AutoSize | Out-String)

--- Top 15 working categories (F1 >= 0.90, n_exists >= 5) ---
$($catRows | Where-Object { $_.F1 -ge 0.90 -and $_.N_exists -ge 5 } | Sort-Object N_exists -Descending | Select-Object -First 15 ExpectedCategory, N_exists, TP, F1, Aftermarket_pct, Specs_pct | Format-Table -AutoSize | Out-String)

--- Files ---
  Full per-category CSV: $catFile
  Full per-system CSV:   $sysFile
  Full per-slice CSV:    $sliceFile
  All failures CSV:      $failFile
"@

$summary | Out-File $summFile -Encoding utf8
Write-Host ""
Write-Host $summary
Write-Host ""
Write-Host "Summary saved: $summFile"
