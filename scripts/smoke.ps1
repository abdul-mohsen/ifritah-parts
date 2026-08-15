# scripts/smoke.ps1 — quick Playwright smoke against port 8081
$root = Split-Path -Parent $PSScriptRoot
$env:E2E_BASE_URL = 'http://127.0.0.1:8081'
Push-Location (Join-Path $root 'frontend')
npx playwright test tests/e2e/story1-regression.spec.ts `
    --project=chromium `
    --reporter=list `
    --timeout=30000 2>&1 | Select-Object -Last 15
Pop-Location
