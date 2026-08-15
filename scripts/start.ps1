# start.ps1 — boots server.exe on the port in .env
param([switch]$Force)

$root = Split-Path -Parent $PSScriptRoot
$exe  = Join-Path $root 'server.exe'
$logs = Join-Path $root 'logs'
$out  = Join-Path $logs 'out.log'
$err  = Join-Path $logs 'err.log'
$pid_ = Join-Path $logs 'server.pid'

if ($Force) { Get-Process server -EA 0 | Stop-Process -Force -EA 0 }
if (-not (Test-Path $exe)) {
    Push-Location $root
    & go build -o $exe .\cmd\server
    Pop-Location
}
New-Item -ItemType Directory $logs -Force -EA 0 | Out-Null
New-Item -ItemType File $out, $err -Force -EA 0 | Out-Null
$p = Start-Process $exe -WorkingDirectory $root `
    -RedirectStandardOutput $out `
    -RedirectStandardError  $err `
    -WindowStyle Hidden -PassThru
$p.Id | Out-File $pid_
Start-Sleep 2

$port = (Select-String -Path (Join-Path $root '.env') -Pattern '^PORT=(\d+)').Matches.Groups[1].Value
if (-not $port) { $port = '8080' }
try {
    $r = Invoke-RestMethod "http://127.0.0.1:$port/health" -TimeoutSec 3
    "pid=$($p.Id)  port=$port  health=$($r | ConvertTo-Json -Compress)"
} catch {
    "pid=$($p.Id) port=$port NOT-UP; check logs\err.log"
}
