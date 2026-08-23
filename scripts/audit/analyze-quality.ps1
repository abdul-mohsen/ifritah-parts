# analyze-quality.ps1 - Search-engine quality analyzer
#
# Reads the raw CSV from audit-quality.ps1, emits FIVE dated output files
# and computes FOUR layered F1 metrics per group:
#
#   F1_hit    = returned any result (basic recall)
#   F1_part   = returned result + description matches ExpectedCategory
#               tokens (correct part identification)
#   F1_repl   = returned result + at least one replacement (aftermarket
#               brand OR TecDoc OEM cross-reference OR supersession chain)
#   F1_full   = returned CORRECT part AND at least one replacement.
#               This is the "app fulfills its promise" metric a parts
#               seller cares about - they need the OEM identified AND
#               at least one alternative sourcing option.
#
# Layered outputs so you can see WHERE the quality gap is: identify part
# vs find replacement vs both. Target for F1_full is >= 0.95 on
# replaceable-parts categories (filters, brakes, shocks, etc.). Body /
# glass / accessory categories have a lower structural ceiling because
# TecDoc has zero aftermarket data for them - track F1_part on those.
#
# Outputs (all dated yyyy-MM-dd_HHmm):
#   qa-quality-by-category-<date>.csv   - ALL categories, all four F1s
#   qa-quality-by-system-<date>.csv     - per ExpectedSystem
#   qa-quality-by-slice-<date>.csv      - per corpus slice
#   qa-quality-failures-<date>.csv      - every failing OEM with reason
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

# Classify every row against ALL FOUR metrics
$classified = $rows | ForEach-Object {
  $r = $_
  $hasResult = [int]$r.Total -gt 0
  $partOK    = $hasResult -and (Test-Tokens $r.FirstDesc $r.GoodTokens $MinTokensMatch)
  $hasAm     = [int]$r.AftermarketCount -gt 0
  $hasOemRef = [int]$r.OEMNumbersCount -gt 0
  $hasSup    = $r.HasSupersession -eq "true"
  $hasRepl   = $hasAm -or $hasOemRef -or $hasSup
  $isTimeout = ($r.IsTimeout -eq "True") -or ([double]$r.ElapsedS -ge 14)

  # HIT: returned any result at all
  $hitClass = switch ($r.GroundTruth) {
    "exists"        { if ($hasResult) { "TP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }
  # PART: hit + correct category
  $partClass = switch ($r.GroundTruth) {
    "exists"        { if ($partOK) { "TP" } elseif ($hasResult) { "FP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }
  # REPL: hit + at least one replacement
  $replClass = switch ($r.GroundTruth) {
    "exists"        { if ($hasResult -and $hasRepl) { "TP" } elseif ($hasResult) { "FP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }
  # FULL: hit + correct category + replacement
  $fullClass = switch ($r.GroundTruth) {
    "exists"        { if ($partOK -and $hasRepl) { "TP" } elseif ($hasResult) { "FP" } else { "FN" } }
    "not_hk_format" { if ($hasResult) { "FP" } else { "TN" } }
    "non_hk"        { if ($hasResult) { "FP" } else { "TN" } }
    default         { "UNK" }
  }

  $failReason = ""
  if ($fullClass -eq "FP") {
    if (-not $partOK -and -not $hasRepl) { $failReason = "wrong-part+no-replacement" }
    elseif (-not $partOK) { $failReason = "wrong-part (has replacement)" }
    elseif (-not $hasRepl) { $failReason = "correct-part / no-replacement (AM=$($r.AftermarketCount) OEMx=$($r.OEMNumbersCount) sup=$($r.HasSupersession))" }
  } elseif ($fullClass -eq "FN") {
    if ($isTimeout) { $failReason = "TIMEOUT at $($r.ElapsedS)s" }
    elseif ([int]$r.HttpStatus -ne 200) { $failReason = "HTTP $($r.HttpStatus)" }
    else { $failReason = "no results" }
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
    PartClass        = $partClass
    ReplClass        = $replClass
    FullClass        = $fullClass
    HasHit           = $hasResult
    PartOK           = $partOK
    HasAftermarket   = $hasAm
    HasOEMRef        = $hasOemRef
    HasSupersession  = $hasSup
    HasReplacement   = $hasRepl
    AftermarketCount = [int]$r.AftermarketCount
    AftermarketSample= $r.AftermarketSample
    SpecsCount       = [int]$r.SpecsCount
    VehiclesCount    = [int]$r.VehiclesCount
    OEMNumbersCount  = [int]$r.OEMNumbersCount
    OEMNumbersSample = $r.OEMNumbersSample
    FailReason       = $failReason
  }
}

function Compute-F1Set($group) {
  $hitTP = ($group | Where-Object { $_.HitClass  -eq "TP" }).Count
  $hitFP = ($group | Where-Object { $_.HitClass  -eq "FP" }).Count
  $hitFN = ($group | Where-Object { $_.HitClass  -eq "FN" }).Count
  $partTP= ($group | Where-Object { $_.PartClass -eq "TP" }).Count
  $partFP= ($group | Where-Object { $_.PartClass -eq "FP" }).Count
  $partFN= ($group | Where-Object { $_.PartClass -eq "FN" }).Count
  $replTP= ($group | Where-Object { $_.ReplClass -eq "TP" }).Count
  $replFP= ($group | Where-Object { $_.ReplClass -eq "FP" }).Count
  $replFN= ($group | Where-Object { $_.ReplClass -eq "FN" }).Count
  $fullTP= ($group | Where-Object { $_.FullClass -eq "TP" }).Count
  $fullFP= ($group | Where-Object { $_.FullClass -eq "FP" }).Count
  $fullFN= ($group | Where-Object { $_.FullClass -eq "FN" }).Count
  $h = Get-F1 $hitTP $hitFP $hitFN
  $p = Get-F1 $partTP $partFP $partFN
  $r = Get-F1 $replTP $replFP $replFN
  $f = Get-F1 $fullTP $fullFP $fullFN
  return @{
    Hit  = @{TP=$hitTP;  FP=$hitFP;  FN=$hitFN;  P=$h.P; R=$h.R; F1=$h.F1}
    Part = @{TP=$partTP; FP=$partFP; FN=$partFN; P=$p.P; R=$p.R; F1=$p.F1}
    Repl = @{TP=$replTP; FP=$replFP; FN=$replFN; P=$r.P; R=$r.R; F1=$r.F1}
    Full = @{TP=$fullTP; FP=$fullFP; FN=$fullFN; P=$f.P; R=$f.R; F1=$f.F1}
  }
}

# ------------ Per-category ------------
$catRows = $classified | Group-Object ExpectedCategory | ForEach-Object {
  $g = $_.Group
  $n = $g.Count
  $existsCount = ($g | Where-Object { $_.GroundTruth -eq "exists" }).Count
  $f1s = Compute-F1Set $g
  $hitCount = ($g | Where-Object { $_.HasHit }).Count

  $amHits = ($g | Where-Object { $_.HasAftermarket }).Count
  $oemHits= ($g | Where-Object { $_.HasOEMRef }).Count
  $supHits= ($g | Where-Object { $_.HasSupersession }).Count

  [PSCustomObject]@{
    ExpectedCategory = if ($_.Name) { $_.Name } else { "<blank>" }
    N                = $n
    N_exists         = $existsCount
    "F1_hit"         = $f1s.Hit.F1
    "F1_part"        = $f1s.Part.F1
    "F1_repl"        = $f1s.Repl.F1
    "F1_full"        = $f1s.Full.F1
    "hit_TP"         = $f1s.Hit.TP
    "part_TP"        = $f1s.Part.TP
    "repl_TP"        = $f1s.Repl.TP
    "full_TP"        = $f1s.Full.TP
    Aftermarket_pct  = if ($hitCount -gt 0) { [Math]::Round(100.0 * $amHits / $hitCount, 1) } else { 0 }
    OEMxRef_pct      = if ($hitCount -gt 0) { [Math]::Round(100.0 * $oemHits / $hitCount, 1) } else { 0 }
    Supersession_pct = if ($hitCount -gt 0) { [Math]::Round(100.0 * $supHits / $hitCount, 1) } else { 0 }
  }
} | Sort-Object N -Descending
$catRows | Export-Csv $catFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-category CSV: $catFile ($($catRows.Count) categories)"

# ------------ Per-system ------------
$sysRows = $classified | Group-Object ExpectedSystem | ForEach-Object {
  $g = $_.Group
  $f1s = Compute-F1Set $g
  [PSCustomObject]@{
    ExpectedSystem = if ($_.Name) { $_.Name } else { "<blank>" }
    N              = $g.Count
    "F1_hit"       = $f1s.Hit.F1
    "F1_part"      = $f1s.Part.F1
    "F1_repl"      = $f1s.Repl.F1
    "F1_full"      = $f1s.Full.F1
  }
} | Sort-Object N -Descending
$sysRows | Export-Csv $sysFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-system CSV:   $sysFile ($($sysRows.Count) systems)"

# ------------ Per-slice ------------
$sliceRows = $classified | Group-Object Slice | ForEach-Object {
  $g = $_.Group
  $f1s = Compute-F1Set $g
  [PSCustomObject]@{
    Slice     = $_.Name
    N         = $g.Count
    "F1_hit"  = $f1s.Hit.F1
    "F1_part" = $f1s.Part.F1
    "F1_repl" = $f1s.Repl.F1
    "F1_full" = $f1s.Full.F1
    "hit_TP"  = $f1s.Hit.TP
    "part_TP" = $f1s.Part.TP
    "repl_TP" = $f1s.Repl.TP
    "full_TP" = $f1s.Full.TP
  }
} | Sort-Object N -Descending
$sliceRows | Export-Csv $sliceFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote per-slice CSV:    $sliceFile"

# ------------ Failures (every FN + FP under FullClass) ------------
$failures = $classified | Where-Object { $_.FullClass -in @("FN","FP") } |
  Select-Object OEM, Slice, GroundTruth, ExpectedCategory, ExpectedSystem, GoodTokens,
                FullClass, FailReason, Total, FirstDesc, FirstBrand, FirstCategory,
                HasAftermarket, HasOEMRef, HasSupersession, ElapsedS, IsTimeout
$failures | Export-Csv $failFile -Encoding utf8 -NoTypeInformation
Write-Host "Wrote failures CSV:     $failFile ($($failures.Count) failures)"

# ------------ Summary ------------
$overall = Compute-F1Set $classified
$catsFullOk = ($catRows | Where-Object { $_.F1_full -ge 0.95 -and $_.N_exists -ge 5 }).Count
$catsFullFail = ($catRows | Where-Object { $_.F1_full -lt 0.5 -and $_.N_exists -ge 3 }).Count

$summary = @"
=====================================================================
QA SEARCH QUALITY REPORT - $dateStamp
=====================================================================
Source: $InputCSV
Rows:   $($rows.Count)

--- Ground truth distribution ---
$($classified | Group-Object GroundTruth | ForEach-Object { "  {0,-18} n={1}" -f $_.Name, $_.Count } | Out-String)
--- FOUR-LAYER F1 (overall) ---
  F1_hit  = $($overall.Hit.F1)   (returned any result - basic recall)
  F1_part = $($overall.Part.F1)   (hit + correct category identification)
  F1_repl = $($overall.Repl.F1)   (hit + >=1 replacement: aftermarket / OEMxref / supersession)
  F1_full = $($overall.Full.F1)   (hit + correct category + >=1 replacement - "app fulfills promise")

Target: F1_full >= 0.95 on replaceable-parts categories.
Body / glass / dealer-accessory categories have a data-source ceiling.

--- Per-slice F1 ---
$($sliceRows | Format-Table -AutoSize | Out-String)

--- Categories at or above F1_full = 0.95 (n_exists >= 5) ---
$($catRows | Where-Object { $_.F1_full -ge 0.95 -and $_.N_exists -ge 5 } | Sort-Object N_exists -Descending | Select-Object ExpectedCategory, N_exists, F1_full, Aftermarket_pct, OEMxRef_pct | Format-Table -AutoSize | Out-String)

--- Categories below F1_full = 0.50 (n_exists >= 3, top 20) ---
$($catRows | Where-Object { $_.F1_full -lt 0.5 -and $_.N_exists -ge 3 } | Sort-Object N_exists -Descending | Select-Object -First 20 ExpectedCategory, N_exists, F1_hit, F1_part, F1_repl, F1_full | Format-Table -AutoSize | Out-String)

--- Category summary ---
  Total distinct categories: $($catRows.Count)
  Meeting 95% F1_full target (n_exists >= 5): $catsFullOk
  Below 50% F1_full (n_exists >= 3):          $catsFullFail

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
