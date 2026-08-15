# build-frontend.ps1
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'frontend')
if (-not (Test-Path node_modules)) { npm ci --no-audit --no-fund }
npm run build
Pop-Location
