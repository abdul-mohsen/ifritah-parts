# test-compare-audits.ps1 - Integration tests for compare-audits.ps1.
#
# Synthesizes small by-slice.csv fixtures, runs the comparator against
# them, and asserts the exit code + a substring in the output.
#
# Run:  pwsh -File scripts/audit/test-compare-audits.ps1
#
# Exit 0 = all tests passed. Exit 1 = at least one test failed.
#
# This is the acceptance-criteria harness for M6.S1.T2 - "exits 0/1/2
# correctly for 4 test inputs". Kept in the repo (not just in CI)
# so contributors can rerun it locally after touching either script.

$ErrorActionPreference = 'Stop'

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$comparator = Join-Path $here 'compare-audits.ps1'
if (-not (Test-Path -LiteralPath $comparator)) {
  Write-Host "ERROR: compare-audits.ps1 not found next to test script: $comparator" -ForegroundColor Red
  exit 1
}

$tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) "compare-audits-test-$(Get-Random)"
New-Item -ItemType Directory -Path $tmpRoot -Force | Out-Null
Write-Host "Fixture dir: $tmpRoot"

$sliceHeader = 'Slice,N,F1_hit,F1_correct,AvgRepl_correct,AvgAM_correct,AvgOEMxRef_correct,F1_rich3,F1_rich5,F1_rich10'

function Write-SliceCsv {
  param([string]$Path, [hashtable[]]$Slices)
  $lines = @($sliceHeader)
  foreach ($s in $Slices) {
    $row = @(
      $s.Slice, $s.N, $s.F1_hit, $s.F1_correct,
      $s.AvgRepl_correct, $s.AvgAM_correct, $s.AvgOEMxRef_correct,
      $s.F1_rich3, $s.F1_rich5, $s.F1_rich10
    )
    $lines += ($row -join ',')
  }
  $lines | Out-File -FilePath $Path -Encoding utf8
}

# Fixture: a "canonical" baseline nightly on real_hk_seeded.
$baselineOk = Join-Path $tmpRoot 'baseline-ok.csv'
Write-SliceCsv -Path $baselineOk -Slices @(
  @{ Slice='real_hk_seeded';  N=100; F1_hit=0.94; F1_correct=0.80; AvgRepl_correct=4.2; AvgAM_correct=1.9; AvgOEMxRef_correct=2.3; F1_rich3=0.71; F1_rich5=0.55; F1_rich10=0.30 },
  @{ Slice='real_hk_coarse';  N=50;  F1_hit=0.88; F1_correct=0.72; AvgRepl_correct=3.1; AvgAM_correct=1.2; AvgOEMxRef_correct=1.9; F1_rich3=0.60; F1_rich5=0.40; F1_rich10=0.18 },
  @{ Slice='non_hk';          N=10;  F1_hit=0.10; F1_correct=0.05; AvgRepl_correct=0.0; AvgAM_correct=0.0; AvgOEMxRef_correct=0.0; F1_rich3=0.00; F1_rich5=0.00; F1_rich10=0.00 }
)

# Case 1: candidate identical to baseline - PASS (delta=0), exit 0.
$candSame = Join-Path $tmpRoot 'cand-same.csv'
Copy-Item -LiteralPath $baselineOk -Destination $candSame

# Case 2: candidate improves F1_correct by +0.05 - PASS, exit 0.
$candBetter = Join-Path $tmpRoot 'cand-better.csv'
Write-SliceCsv -Path $candBetter -Slices @(
  @{ Slice='real_hk_seeded';  N=100; F1_hit=0.95; F1_correct=0.85; AvgRepl_correct=4.5; AvgAM_correct=2.0; AvgOEMxRef_correct=2.4; F1_rich3=0.75; F1_rich5=0.58; F1_rich10=0.33 },
  @{ Slice='real_hk_coarse';  N=50;  F1_hit=0.88; F1_correct=0.72; AvgRepl_correct=3.1; AvgAM_correct=1.2; AvgOEMxRef_correct=1.9; F1_rich3=0.60; F1_rich5=0.40; F1_rich10=0.18 },
  @{ Slice='non_hk';          N=10;  F1_hit=0.10; F1_correct=0.05; AvgRepl_correct=0.0; AvgAM_correct=0.0; AvgOEMxRef_correct=0.0; F1_rich3=0.00; F1_rich5=0.00; F1_rich10=0.00 }
)

# Case 3: candidate regressed F1_correct by 0.05 (>threshold 0.02) - BLOCK, exit 1.
$candWorse = Join-Path $tmpRoot 'cand-worse.csv'
Write-SliceCsv -Path $candWorse -Slices @(
  @{ Slice='real_hk_seeded';  N=100; F1_hit=0.92; F1_correct=0.75; AvgRepl_correct=4.0; AvgAM_correct=1.7; AvgOEMxRef_correct=2.1; F1_rich3=0.65; F1_rich5=0.50; F1_rich10=0.28 },
  @{ Slice='real_hk_coarse';  N=50;  F1_hit=0.88; F1_correct=0.72; AvgRepl_correct=3.1; AvgAM_correct=1.2; AvgOEMxRef_correct=1.9; F1_rich3=0.60; F1_rich5=0.40; F1_rich10=0.18 },
  @{ Slice='non_hk';          N=10;  F1_hit=0.10; F1_correct=0.05; AvgRepl_correct=0.0; AvgAM_correct=0.0; AvgOEMxRef_correct=0.0; F1_rich3=0.00; F1_rich5=0.00; F1_rich10=0.00 }
)

# Case 4: candidate regressed 0.01 (below threshold) - PASS with warning, exit 0.
$candBorderline = Join-Path $tmpRoot 'cand-borderline.csv'
Write-SliceCsv -Path $candBorderline -Slices @(
  @{ Slice='real_hk_seeded';  N=100; F1_hit=0.93; F1_correct=0.79; AvgRepl_correct=4.1; AvgAM_correct=1.8; AvgOEMxRef_correct=2.2; F1_rich3=0.70; F1_rich5=0.54; F1_rich10=0.29 },
  @{ Slice='real_hk_coarse';  N=50;  F1_hit=0.88; F1_correct=0.72; AvgRepl_correct=3.1; AvgAM_correct=1.2; AvgOEMxRef_correct=1.9; F1_rich3=0.60; F1_rich5=0.40; F1_rich10=0.18 },
  @{ Slice='non_hk';          N=10;  F1_hit=0.10; F1_correct=0.05; AvgRepl_correct=0.0; AvgAM_correct=0.0; AvgOEMxRef_correct=0.0; F1_rich3=0.00; F1_rich5=0.00; F1_rich10=0.00 }
)

# Case 5: candidate missing the -Slice row entirely - corpus mismatch, exit 2.
$candMissingSlice = Join-Path $tmpRoot 'cand-missing-slice.csv'
Write-SliceCsv -Path $candMissingSlice -Slices @(
  @{ Slice='real_hk_coarse';  N=50;  F1_hit=0.88; F1_correct=0.72; AvgRepl_correct=3.1; AvgAM_correct=1.2; AvgOEMxRef_correct=1.9; F1_rich3=0.60; F1_rich5=0.40; F1_rich10=0.18 }
)

# Case 6: baseline file missing on disk - corpus mismatch, exit 2.
$missingBaseline = Join-Path $tmpRoot 'does-not-exist.csv'

# Case 7 (bonus, acceptance criterion): both files empty (like /dev/null on Linux)
# - corpus mismatch, exit 2.
$emptyBaseline  = Join-Path $tmpRoot 'empty-baseline.csv'
$emptyCandidate = Join-Path $tmpRoot 'empty-candidate.csv'
New-Item -ItemType File -Path $emptyBaseline  -Force | Out-Null
New-Item -ItemType File -Path $emptyCandidate -Force | Out-Null

# ---------- Runner ----------
$failures = @()
$passed   = 0

function Invoke-Case {
  param(
    [string]$Name,
    [string]$Baseline,
    [string]$Candidate,
    [int]$ExpectedExit,
    [string]$ExpectedSubstring = $null,
    [string[]]$ExtraArgs = @()
  )
  Write-Host ""
  Write-Host "--- CASE: $Name ---"
  $argsList = @(
    '-NoProfile', '-NonInteractive', '-File', $script:comparator,
    '-BaselineCsv',  $Baseline,
    '-CandidateCsv', $Candidate
  ) + $ExtraArgs
  $out = & pwsh @argsList 2>&1
  $actualExit = $LASTEXITCODE
  $outText = ($out | Out-String)
  Write-Host $outText

  $ok = $true
  if ($actualExit -ne $ExpectedExit) {
    Write-Host ("FAIL: expected exit {0}, got {1}" -f $ExpectedExit, $actualExit) -ForegroundColor Red
    $ok = $false
  }
  if ($ExpectedSubstring -and ($outText -notmatch [regex]::Escape($ExpectedSubstring))) {
    Write-Host ("FAIL: expected output to contain '{0}'" -f $ExpectedSubstring) -ForegroundColor Red
    $ok = $false
  }
  if ($ok) {
    Write-Host ("PASS: exit {0}" -f $actualExit) -ForegroundColor Green
    $script:passed++
  } else {
    $script:failures += $Name
  }
}

Invoke-Case -Name 'identical baseline/candidate -> exit 0' `
            -Baseline $baselineOk -Candidate $candSame `
            -ExpectedExit 0 -ExpectedSubstring 'held or improved'

Invoke-Case -Name 'candidate +0.05 better -> exit 0' `
            -Baseline $baselineOk -Candidate $candBetter `
            -ExpectedExit 0 -ExpectedSubstring 'held or improved'

Invoke-Case -Name 'candidate -0.05 regression -> exit 1' `
            -Baseline $baselineOk -Candidate $candWorse `
            -ExpectedExit 1 -ExpectedSubstring 'REGRESSION'

Invoke-Case -Name 'candidate -0.01 within tolerance -> exit 0' `
            -Baseline $baselineOk -Candidate $candBorderline `
            -ExpectedExit 0 -ExpectedSubstring 'within tolerance'

Invoke-Case -Name 'candidate missing slice row -> exit 2' `
            -Baseline $baselineOk -Candidate $candMissingSlice `
            -ExpectedExit 2 -ExpectedSubstring 'corpus mismatch'

Invoke-Case -Name 'baseline file does not exist -> exit 2' `
            -Baseline $missingBaseline -Candidate $candSame `
            -ExpectedExit 2 -ExpectedSubstring 'corpus mismatch'

Invoke-Case -Name 'both files empty -> exit 2' `
            -Baseline $emptyBaseline -Candidate $emptyCandidate `
            -ExpectedExit 2 -ExpectedSubstring 'corpus mismatch'

Invoke-Case -Name 'verbose output includes all slices' `
            -Baseline $baselineOk -Candidate $candBetter `
            -ExpectedExit 0 -ExpectedSubstring 'Full per-slice comparison' `
            -ExtraArgs @('-VerboseOutput')

Invoke-Case -Name 'custom threshold=0.10 masks -0.05 regression -> exit 0' `
            -Baseline $baselineOk -Candidate $candWorse `
            -ExpectedExit 0 -ExpectedSubstring 'within tolerance' `
            -ExtraArgs @('-RegressionThreshold','0.10')

# ---------- Cleanup + summary ----------
Remove-Item -Recurse -Force -LiteralPath $tmpRoot

Write-Host ""
Write-Host "===================================="
Write-Host ("Passed: {0}" -f $passed)
Write-Host ("Failed: {0}" -f $failures.Count)
if ($failures.Count -gt 0) {
  Write-Host "Failing cases:"
  foreach ($f in $failures) { Write-Host "  - $f" -ForegroundColor Red }
  exit 1
}
Write-Host "ALL TESTS PASSED" -ForegroundColor Green
exit 0
