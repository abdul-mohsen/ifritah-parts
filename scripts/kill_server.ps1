# Kill existing server and restart in foreground for debugging
Get-Process server -ErrorAction SilentlyContinue | Where-Object { $_.Path -like "*parts-engine*" } | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Write-Host "Starting server in foreground..."
