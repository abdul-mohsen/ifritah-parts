# Start server and capture output
$proc = Start-Process -FilePath "c:\ssda\chatGPT\parts-engine\server.exe" -WorkingDirectory "c:\ssda\chatGPT\parts-engine" -NoNewWindow -PassThru -RedirectStandardOutput "c:\ssda\chatGPT\parts-engine\server_stdout.log" -RedirectStandardError "c:\ssda\chatGPT\parts-engine\server_stderr.log"
Start-Sleep -Seconds 5

Write-Host "=== STDOUT ==="
Get-Content "c:\ssda\chatGPT\parts-engine\server_stdout.log" -Raw
Write-Host ""
Write-Host "=== STDERR ==="
Get-Content "c:\ssda\chatGPT\parts-engine\server_stderr.log" -Raw
Write-Host ""
Write-Host "Server PID: $($proc.Id)"
