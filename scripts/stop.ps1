# stop.ps1 — kill server.exe
Get-Process server -EA 0 | Stop-Process -Force -EA 0
"stopped"
