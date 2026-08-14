# Run test_all_brands.ps1 and capture results summary
powershell -ExecutionPolicy Bypass -File c:\ssda\chatGPT\parts-engine\scripts\test_all_brands.ps1 2>&1 | Select-String "FAIL|Results:" | ForEach-Object { $_.Line }
