$ErrorActionPreference = "SilentlyContinue"
# Kill any running server
Get-Process server | Stop-Process -Force
Start-Sleep 1
# Start new server
Start-Process -FilePath "c:\ssda\chatGPT\parts-engine\server.exe" -WorkingDirectory "c:\ssda\chatGPT\parts-engine" -NoNewWindow
Start-Sleep 2
Write-Host "Server restarted" -ForegroundColor Green
