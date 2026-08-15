# scripts/watch-ci.ps1 — polls PR 5 checks, appends to logs/ci.log
$root = Split-Path -Parent $PSScriptRoot
$log  = Join-Path $root 'logs\ci.log'
New-Item -ItemType Directory (Split-Path $log) -Force -EA 0 | Out-Null
"[$(Get-Date -Format s)]" | Out-File $log -Append
Push-Location $root
& gh pr checks 5 2>&1 | Out-File $log -Append
Pop-Location
Get-Content $log -Tail 6
