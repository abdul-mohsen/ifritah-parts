# dev-server.ps1
# Idempotent background launcher for the local parts-engine dev stack.
# Usage:
#   pwsh scripts/dev-server.ps1 start
#   pwsh scripts/dev-server.ps1 stop
#   pwsh scripts/dev-server.ps1 restart
#   pwsh scripts/dev-server.ps1 status
#   pwsh scripts/dev-server.ps1 logs    # tail last 60 lines
#   pwsh scripts/dev-server.ps1 rebuild # go build then restart
#
# Boot order:
#   1. docker compose up -d postgres (only if not already healthy)
#   2. go build -o server.exe .\cmd\server (only for 'rebuild')
#   3. Start server.exe with all env vars, redirect stdout/stderr to logs
#
# The server is launched with Start-Process so this script exits fast.
# Never blocks the caller.

param(
    [Parameter(Position = 0)]
    [ValidateSet('start', 'stop', 'restart', 'status', 'logs', 'rebuild')]
    [string]$Action = 'status'
)

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$serverExe   = Join-Path $projectRoot 'server.exe'
$logDir      = Join-Path $projectRoot 'logs'
$stdoutLog   = Join-Path $logDir 'server.out.log'
$stderrLog   = Join-Path $logDir 'server.err.log'
$pidFile     = Join-Path $logDir 'server.pid'

if (-not (Test-Path -LiteralPath $logDir)) {
    New-Item -ItemType Directory -Path $logDir | Out-Null
}

function Get-ServerPid {
    if (-not (Test-Path -LiteralPath $pidFile)) { return $null }
    $storedPid = Get-Content -LiteralPath $pidFile -ErrorAction SilentlyContinue
    if (-not $storedPid) { return $null }
    $proc = Get-Process -Id $storedPid -ErrorAction SilentlyContinue
    if ($proc) { return $proc } else {
        Remove-Item -LiteralPath $pidFile -ErrorAction SilentlyContinue
        return $null
    }
}

function Test-Health {
    param([int]$Retries = 8, [int]$DelaySec = 1)
    for ($i = 0; $i -lt $Retries; $i++) {
        try {
            $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:8080/health' -TimeoutSec 3 -ErrorAction Stop
            if ($resp.StatusCode -eq 200) { return $true }
        } catch { }
        Start-Sleep -Seconds $DelaySec
    }
    return $false
}

function Ensure-Postgres {
    $status = docker ps --filter 'name=parts-postgres' --format '{{.Status}}' 2>$null
    if ($status -match 'healthy') {
        Write-Host "[postgres] already healthy: $status"
        return
    }
    Write-Host '[postgres] booting via docker compose ...'
    Push-Location $projectRoot
    try {
        docker compose up -d postgres 2>&1 | Out-Null
    } finally {
        Pop-Location
    }
    for ($i = 0; $i -lt 24; $i++) {
        $status = docker ps --filter 'name=parts-postgres' --format '{{.Status}}' 2>$null
        if ($status -match 'healthy') {
            Write-Host "[postgres] healthy after $i x5s: $status"
            return
        }
        Start-Sleep -Seconds 5
    }
    throw "postgres did not become healthy in time"
}

function Set-EnvForServer {
    $env:BIND_ADDR         = '127.0.0.1'
    $env:PORT              = '8080'
    $env:PGHOST            = '127.0.0.1'
    $env:PGPORT            = '55432'
    $env:PGUSER            = 'parts'
    $env:PGPASSWORD        = 'parts_engine_pw'
    $env:PGDATABASE        = 'parts_engine'
    $env:PGSSLMODE         = 'disable'
    $env:DATA_DIR          = Join-Path $projectRoot 'data'
    $env:FRONTEND_DIR      = Join-Path $projectRoot 'frontend\dist'
    $env:CORS_ORIGINS      = 'http://localhost:5173,http://localhost:3000,http://localhost:5175'
    $env:NHTSA_URL         = 'https://vpic.nhtsa.dot.gov/api'
    $env:NHTSA_RECALLS_URL = 'https://api.nhtsa.gov/recalls'
}

function Start-Server {
    $existing = Get-ServerPid
    if ($existing) {
        Write-Host "[server] already running (pid=$($existing.Id))"
        if (Test-Health -Retries 3) { Write-Host '[server] /health OK' } else { Write-Host '[server] /health FAILED' }
        return
    }

    Ensure-Postgres

    if (-not (Test-Path -LiteralPath $serverExe)) {
        Write-Host '[server] server.exe missing, building ...'
        Push-Location $projectRoot
        try {
            $env:CGO_ENABLED = '0'
            & go build -o $serverExe .\cmd\server
            if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
        } finally {
            Pop-Location
        }
    }

    Set-EnvForServer
    Remove-Item -LiteralPath $stdoutLog, $stderrLog -ErrorAction SilentlyContinue

    $proc = Start-Process -FilePath $serverExe `
                          -WorkingDirectory $projectRoot `
                          -RedirectStandardOutput $stdoutLog `
                          -RedirectStandardError $stderrLog `
                          -WindowStyle Hidden `
                          -PassThru
    Set-Content -LiteralPath $pidFile -Value $proc.Id
    Write-Host "[server] started pid=$($proc.Id)"

    if (Test-Health -Retries 15 -DelaySec 1) {
        Write-Host '[server] /health OK'
    } else {
        Write-Host '[server] /health did NOT respond in time — check logs:'
        Get-Content -LiteralPath $stderrLog -Tail 25 -ErrorAction SilentlyContinue
    }
}

function Stop-Server {
    $proc = Get-ServerPid
    if (-not $proc) {
        # Fallback: kill any server.exe we may have leaked
        Get-Process -Name 'server' -ErrorAction SilentlyContinue |
            Where-Object { $_.Path -like "$projectRoot*" } |
            Stop-Process -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $pidFile -ErrorAction SilentlyContinue
        Write-Host '[server] not running'
        return
    }
    try { Stop-Process -Id $proc.Id -Force } catch {}
    Remove-Item -LiteralPath $pidFile -ErrorAction SilentlyContinue
    Write-Host "[server] stopped pid=$($proc.Id)"
}

function Show-Status {
    $proc = Get-ServerPid
    if ($proc) {
        Write-Host "[server] pid=$($proc.Id) running"
    } else {
        Write-Host '[server] not running'
    }
    $status = docker ps --filter 'name=parts-postgres' --format '{{.Status}}' 2>$null
    if ($status) { Write-Host "[postgres] $status" } else { Write-Host '[postgres] not running' }
    if (Test-Health -Retries 1) { Write-Host '[health] http://127.0.0.1:8080/health OK' } else { Write-Host '[health] FAIL' }
}

function Show-Logs {
    if (Test-Path -LiteralPath $stdoutLog) {
        Write-Host '=== stdout (last 60) ==='
        Get-Content -LiteralPath $stdoutLog -Tail 60
    }
    if (Test-Path -LiteralPath $stderrLog) {
        Write-Host '=== stderr (last 60) ==='
        Get-Content -LiteralPath $stderrLog -Tail 60
    }
}

function Rebuild {
    Stop-Server
    Push-Location $projectRoot
    try {
        $env:CGO_ENABLED = '0'
        & go build -o $serverExe .\cmd\server
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
        Write-Host '[server] rebuilt'
    } finally {
        Pop-Location
    }
    Start-Server
}

switch ($Action) {
    'start'   { Start-Server }
    'stop'    { Stop-Server }
    'restart' { Stop-Server; Start-Sleep -Seconds 1; Start-Server }
    'status'  { Show-Status }
    'logs'    { Show-Logs }
    'rebuild' { Rebuild }
}
