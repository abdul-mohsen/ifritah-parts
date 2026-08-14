# Kill existing server, restart, and run quick VIN test
$procs = Get-Process -Name server -ErrorAction SilentlyContinue
if ($procs) {
    $procs | Stop-Process -Force
    Start-Sleep -Seconds 1
}

# Start server in background
Start-Process -FilePath ".\server.exe" -WorkingDirectory "c:\ssda\chatGPT\parts-engine"
Start-Sleep -Seconds 3

function DecodeVIN($vin) {
    $body = @{ vin = $vin } | ConvertTo-Json
    $r = Invoke-RestMethod -Method POST -Uri "http://localhost:8080/api/vin/decode" -Body $body -ContentType "application/json"
    return $r
}

# Test the Maxima VIN
Write-Host "=== TEST 1: 1N4AA6AP9HC406410 (should be 2017 NISSAN Maxima) ==="
$r = DecodeVIN "1N4AA6AP9HC406410"
Write-Host "Make: $($r.nhtsaRaw.make)"
Write-Host "Model: $($r.nhtsaRaw.model)"
Write-Host "Year: $($r.nhtsaRaw.modelYear)"
Write-Host "Body: $($r.nhtsaRaw.bodyClass)"
Write-Host "Fuel: $($r.nhtsaRaw.fuelType)"
Write-Host "Engine CC: $($r.nhtsaRaw.engineDisplacementCC)"
Write-Host "Cylinders: $($r.nhtsaRaw.engineNumberOfCylinders)"
Write-Host ""

# Test MHF VIN (should fallback to WMI since MHF is Thai/Indonesian)
Write-Host "=== TEST 2: MHFBT9F34G6006150 (should be TOYOTA, WMI fallback) ==="
$r2 = DecodeVIN "MHFBT9F34G6006150"
Write-Host "Make: $($r2.nhtsaRaw.make)"
Write-Host "Model: $($r2.nhtsaRaw.model)"
Write-Host "Year: $($r2.nhtsaRaw.modelYear)"
Write-Host ""

# Test Hyundai
Write-Host "=== TEST 3: KMHDN46D38U409756 (should be 2008 HYUNDAI Elantra) ==="
$r3 = DecodeVIN "KMHDN46D38U409756"
Write-Host "Make: $($r3.nhtsaRaw.make)"
Write-Host "Model: $($r3.nhtsaRaw.model)"
Write-Host "Year: $($r3.nhtsaRaw.modelYear)"
Write-Host ""

# Test Toyota Camry
Write-Host "=== TEST 4: 4T1BE46K19U356780 (should be 2009 TOYOTA Camry) ==="
$r4 = DecodeVIN "4T1BE46K19U356780"
Write-Host "Make: $($r4.nhtsaRaw.make)"
Write-Host "Model: $($r4.nhtsaRaw.model)"
Write-Host "Year: $($r4.nhtsaRaw.modelYear)"
Write-Host ""

# Test Honda CR-V
Write-Host "=== TEST 5: 5J6RM4H33EL012345 (should be 2014 HONDA CR-V) ==="
$r5 = DecodeVIN "5J6RM4H33EL012345"
Write-Host "Make: $($r5.nhtsaRaw.make)"
Write-Host "Model: $($r5.nhtsaRaw.model)"
Write-Host "Year: $($r5.nhtsaRaw.modelYear)"
