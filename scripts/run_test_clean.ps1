$output = powershell -ExecutionPolicy Bypass -File .\scripts\test_all_brands.ps1 2>$null
$output | Where-Object { $_ -match "Results:|FAIL" }
