# Kill server on port 8080, reseed DB, rebuild and restart
$proc = Get-NetTCPConnection -LocalPort 8080 -ErrorAction SilentlyContinue | Select-Object -First 1
if ($proc) {
    Stop-Process -Id $proc.OwningProcess -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    Write-Host "Killed old server"
}

# Remove old DB
Remove-Item -Path "data\hk_parts.db" -Force -ErrorAction SilentlyContinue
Remove-Item -Path "data\hk_parts.db-wal" -Force -ErrorAction SilentlyContinue
Remove-Item -Path "data\hk_parts.db-shm" -Force -ErrorAction SilentlyContinue
Write-Host "Old DB removed"

# Re-seed
Write-Host "Seeding database..."
go run ./scripts/seed_db/
if ($LASTEXITCODE -ne 0) { Write-Host "SEED FAILED"; exit 1 }
Write-Host "Seed complete"

# Build server
Write-Host "Building server..."
go build -o ssda_server.exe ./cmd/server/
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED"; exit 1 }
Write-Host "Build complete"

# Start server in background
Write-Host "Starting server..."
Start-Process -FilePath ".\ssda_server.exe" -WindowStyle Hidden
Start-Sleep -Seconds 3
Write-Host "Server started"

# Run PartsOuq tests
Write-Host "`nRunning PartsOuq verification tests..."
go run ./scripts/partsouq_test/
