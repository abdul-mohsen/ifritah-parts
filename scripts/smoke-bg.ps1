# scripts/smoke-bg.ps1 — background Playwright smoke, write to logs/smoke.log
$root = Split-Path -Parent $PSScriptRoot
$logs = Join-Path $root 'logs'
$out  = Join-Path $logs 'smoke.out.log'
$err  = Join-Path $logs 'smoke.err.log'
New-Item -ItemType Directory $logs -Force -EA 0 | Out-Null
Set-Content $out ''
Set-Content $err ''

# Use npm.cmd on Windows so Start-Process finds it
$fe = Join-Path $root 'frontend'
$args = @('playwright', 'test',
    'tests/e2e/story1-regression.spec.ts',
    '--project=chromium',
    '--reporter=list',
    '--timeout=30000')
$p = Start-Process 'npx.cmd' -ArgumentList $args `
    -WorkingDirectory $fe `
    -RedirectStandardOutput $out `
    -RedirectStandardError  $err `
    -WindowStyle Hidden -PassThru
$p.Id | Set-Content (Join-Path $logs 'smoke.pid')
"pid=$($p.Id) — tail with: Get-Content $out -Wait"
