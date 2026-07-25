Get-Process server -ErrorAction SilentlyContinue | ForEach-Object { $_.Kill() }
Start-Sleep -Seconds 1
cd c:\ssda\chatGPT\parts-engine
Start-Process .\server.exe
Start-Sleep -Seconds 2
powershell -ExecutionPolicy Bypass -File .\scripts\test_all_brands.ps1 > test_results.txt 2>&1
Get-Content .\test_results.txt | Select-Object -Last 5
