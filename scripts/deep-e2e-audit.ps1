# deep-e2e-audit.ps1
# Runs the deep Playwright audit against both local (127.0.0.1:8080) and
# production (qa.ifritah.com), collects artifacts into qa/e2e-report/,
# then renders a compact Markdown comparison so we can see the delta.
#
# Usage:
#   pwsh scripts/deep-e2e-audit.ps1                   # both targets
#   pwsh scripts/deep-e2e-audit.ps1 -Target local     # local only
#   pwsh scripts/deep-e2e-audit.ps1 -Target prod      # prod only

param(
    [ValidateSet('both', 'local', 'prod')]
    [string]$Target = 'both',
    [ValidateSet('chromium', 'firefox')]
    [string]$Browser = 'chromium'
)

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$frontend    = Join-Path $projectRoot 'frontend'
$reportRoot  = Join-Path $projectRoot "qa\e2e-report"
$mdPath      = Join-Path $projectRoot "docs\E2E_DEEP_REPORT.md"

if (-not (Test-Path -LiteralPath $reportRoot)) {
    New-Item -ItemType Directory -Path $reportRoot | Out-Null
}

function Run-Target {
    param(
        [string]$Label,
        [string]$BaseUrl
    )
    Write-Host ""
    Write-Host "========================================================"
    Write-Host " Running deep audit against [$Label] $BaseUrl"
    Write-Host "========================================================"

    $targetDir = Join-Path $reportRoot $Label
    if (Test-Path -LiteralPath $targetDir) {
        Remove-Item -LiteralPath $targetDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    New-Item -ItemType Directory -Path $targetDir | Out-Null

    $env:E2E_BASE_URL     = $BaseUrl
    $env:E2E_TARGET_LABEL = $Label

    # Give production a bigger per-test budget; local should be fast.
    $env:PW_TEST_TIMEOUT = if ($Label -eq 'prod') { '180000' } else { '60000' }

    Push-Location $frontend
    try {
        & npx --no-install playwright test tests/e2e/deep-qa-audit.spec.ts `
                --config=playwright.config.ts `
                --project=$Browser `
                --timeout=$($env:PW_TEST_TIMEOUT) `
                --reporter=list `
                --output=$targetDir\pw-test-output `
                --trace=on 2>&1 | Tee-Object -FilePath (Join-Path $targetDir 'pw.log')
        $exit = $LASTEXITCODE
    } finally {
        Pop-Location
    }

    Write-Host "  [$Label] finished. exit=$exit"
    # Return a single object as the ONLY pipeline output from this function.
    [pscustomobject]@{
        label   = $Label
        baseUrl = $BaseUrl
        exit    = $exit
        dir     = $targetDir
    } | Write-Output
}

function Summarize-Target {
    param([string]$Label, [string]$Dir)
    $findingsPath = Join-Path $Dir 'findings.json'
    if (-not (Test-Path -LiteralPath $findingsPath)) {
        [pscustomobject]@{ label = $Label; missing = $true } | Write-Output
        return
    }
    $data = @(Get-Content -LiteralPath $findingsPath -Raw | ConvertFrom-Json)

    $groups = $data | Group-Object category
    $byCategory = @()
    foreach ($g in $groups) {
        $byCategory += [pscustomobject]@{
            category = $g.Name
            total    = $g.Count
            passed   = ($g.Group | Where-Object { $_.ok }).Count
            failed   = ($g.Group | Where-Object { -not $_.ok }).Count
        }
    }

    [pscustomobject]@{
        label      = $Label
        missing    = $false
        total      = $data.Count
        passed     = ($data | Where-Object { $_.ok }).Count
        failed     = ($data | Where-Object { -not $_.ok }).Count
        byCategory = $byCategory
        findings   = $data
    } | Write-Output
}

$runResults = @()

if ($Target -eq 'both' -or $Target -eq 'local') {
    $runResults += @(Run-Target -Label 'local' -BaseUrl 'http://127.0.0.1:8080')
}
if ($Target -eq 'both' -or $Target -eq 'prod') {
    $runResults += @(Run-Target -Label 'prod' -BaseUrl 'https://qa.ifritah.com')
}

# --------------------------------------------------------------------
# Aggregate → Markdown
# --------------------------------------------------------------------
$summaries = @()
foreach ($r in $runResults) {
    if ($null -eq $r) { continue }
    if ($null -eq $r.dir) { continue }
    $summaries += @(Summarize-Target -Label $r.label -Dir $r.dir)
}

$md = New-Object System.Text.StringBuilder
$null = $md.AppendLine('# Parts Engine — Deep E2E QA Report (Playwright)')
$null = $md.AppendLine('')
$null = $md.AppendLine("Generated: $((Get-Date).ToString('u'))")
$null = $md.AppendLine('')
$null = $md.AppendLine('Real user journeys were exercised through a real Chromium (Playwright), not stubbed. Screenshots, HAR files and traces are stored per-target under `qa/e2e-report/<target>/`.')
$null = $md.AppendLine('')
$null = $md.AppendLine('## Headline')
$null = $md.AppendLine('')
$null = $md.AppendLine('| Target | Total steps | Passed | Failed | Artifacts |')
$null = $md.AppendLine('| --- | ---: | ---: | ---: | --- |')
foreach ($s in $summaries) {
    if ($s.missing) {
        $null = $md.AppendLine("| $($s.label) | – | – | – | **NO FINDINGS JSON** |")
        continue
    }
    $rel = "qa/e2e-report/$($s.label)/"
    $null = $md.AppendLine("| $($s.label) | $($s.total) | $($s.passed) | $($s.failed) | [$rel]($rel) |")
}
$null = $md.AppendLine('')

$null = $md.AppendLine('## By category')
$null = $md.AppendLine('')
$null = $md.AppendLine('| Target | Category | Total | Pass | Fail |')
$null = $md.AppendLine('| --- | --- | ---: | ---: | ---: |')
foreach ($s in $summaries) {
    if ($s.missing) { continue }
    foreach ($c in $s.byCategory) {
        $null = $md.AppendLine("| $($s.label) | $($c.category) | $($c.total) | $($c.passed) | $($c.failed) |")
    }
}
$null = $md.AppendLine('')

foreach ($s in $summaries) {
    if ($s.missing) { continue }
    $null = $md.AppendLine("## [$($s.label)] step-by-step findings")
    $null = $md.AppendLine('')
    $null = $md.AppendLine('| id | category | step | ok | elapsed ms | message |')
    $null = $md.AppendLine('| --- | --- | --- | :-: | ---: | --- |')
    foreach ($f in $s.findings) {
        $msg = $f.message
        if ($msg) { $msg = ($msg -replace '\|', '\|') -replace '\r?\n', ' ' }
        if ($msg -and $msg.Length -gt 200) { $msg = $msg.Substring(0, 200) + '…' }
        $ok = if ($f.ok) { 'x' } else { 'FAIL' }
        $ms = if ($f.elapsedMs) { $f.elapsedMs } else { '' }
        $null = $md.AppendLine("| $($f.id) | $($f.category) | $($f.step) | $ok | $ms | $msg |")
    }
    $null = $md.AppendLine('')
}

$md.ToString() | Set-Content -LiteralPath $mdPath -Encoding UTF8
Write-Host ""
Write-Host "== Report written =="
Write-Host "  $mdPath"
foreach ($s in $summaries) {
    if ($s.missing) { continue }
    $rel = Join-Path $reportRoot $s.label
    Write-Host "  [$($s.label)] $rel  ($($s.passed)/$($s.total) passed, $($s.failed) failed)"
}
