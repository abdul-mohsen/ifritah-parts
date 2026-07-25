# Run test_all_brands and save results to a file
$output = powershell -ExecutionPolicy Bypass -File c:\ssda\chatGPT\parts-engine\scripts\test_all_brands.ps1 2>&1
$output | Out-File c:\ssda\chatGPT\parts-engine\test_results_enriched.txt -Encoding UTF8
$fails = ($output | Select-String "FAIL").Count
$results = $output | Select-String "Results:"
Write-Host "Failures found in output: $fails"
Write-Host $results
