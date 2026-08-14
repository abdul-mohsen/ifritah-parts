$ErrorActionPreference = "Stop"

Write-Host "=== Parts Engine API Test ===" -ForegroundColor Cyan

# 1. Health check
Write-Host "`n--- Health Check ---" -ForegroundColor Yellow
try {
    $health = [System.Net.WebClient]::new().DownloadString("http://localhost:8080/health")
    Write-Host "Health: $health" -ForegroundColor Green
} catch {
    Write-Host "Server not running! Start server.exe first." -ForegroundColor Red
    exit 1
}

# 2. VIN Decode - Hyundai Tucson 2015
Write-Host "`n--- VIN Decode: Hyundai Tucson 2015 ---" -ForegroundColor Yellow
$vin = "KM8JT3AF5FU057722"
$body = "{`"vin`":`"$vin`"}"
try {
    $wc = [System.Net.WebClient]::new()
    $wc.Headers.Add("Content-Type", "application/json")
    $result = $wc.UploadString("http://localhost:8080/api/vin/decode", $body)
    $json = $result | ConvertFrom-Json
    Write-Host "VIN: $($json.vin)" -ForegroundColor Green
    Write-Host "Make: $($json.nhtsaRaw.make)"
    Write-Host "Model: $($json.nhtsaRaw.model)"
    Write-Host "Year: $($json.nhtsaRaw.modelYear)"
    Write-Host "Body: $($json.nhtsaRaw.bodyClass)"
    Write-Host "Drive: $($json.nhtsaRaw.driveType)"
    Write-Host "Fuel: $($json.nhtsaRaw.fuelType)"
    Write-Host "Engine CC: $($json.nhtsaRaw.engineDisplacementCC)"
    Write-Host "Cylinders: $($json.nhtsaRaw.engineNumberOfCylinders)"
    Write-Host "Plant: $($json.nhtsaRaw.plantCountry)"
    if ($json.vehicle) {
        Write-Host "TecDoc Vehicle: $($json.vehicle | ConvertTo-Json -Depth 3)" -ForegroundColor Cyan
    } else {
        Write-Host "TecDoc Vehicle: (none - DB not connected)" -ForegroundColor DarkYellow
    }
    if ($json.dbWarning) {
        Write-Host "DB Warning: $($json.dbWarning)" -ForegroundColor DarkYellow
    }
    if ($json.recalls) {
        Write-Host "Recalls: $($json.recalls.Count)" -ForegroundColor Red
        $json.recalls | ForEach-Object { Write-Host "  - $($_.NHTSACampaignNumber): $($_.Component)" }
    }
    Write-Host "`nFull JSON:" -ForegroundColor Gray
    Write-Host ($result | ConvertFrom-Json | ConvertTo-Json -Depth 5)
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

# 3. VIN Decode - Kia Sportage 2020
Write-Host "`n--- VIN Decode: Kia Sportage 2020 ---" -ForegroundColor Yellow
$vin2 = "KNDPN3AC0L7777777"
$body2 = "{`"vin`":`"$vin2`"}"
try {
    $wc2 = [System.Net.WebClient]::new()
    $wc2.Headers.Add("Content-Type", "application/json")
    $result2 = $wc2.UploadString("http://localhost:8080/api/vin/decode", $body2)
    $json2 = $result2 | ConvertFrom-Json
    Write-Host "VIN: $($json2.vin)" -ForegroundColor Green
    Write-Host "Make: $($json2.nhtsaRaw.make)"
    Write-Host "Model: $($json2.nhtsaRaw.model)"
    Write-Host "Year: $($json2.nhtsaRaw.modelYear)"
    Write-Host "Body: $($json2.nhtsaRaw.bodyClass)"
    Write-Host "Fuel: $($json2.nhtsaRaw.fuelType)"
    Write-Host "Engine CC: $($json2.nhtsaRaw.engineDisplacementCC)"
    if ($json2.recalls) {
        Write-Host "Recalls: $($json2.recalls.Count)" -ForegroundColor Red
    }
    Write-Host "`nFull JSON:" -ForegroundColor Gray
    Write-Host ($result2 | ConvertFrom-Json | ConvertTo-Json -Depth 5)
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

# 4. Invalid VIN
Write-Host "`n--- Invalid VIN ---" -ForegroundColor Yellow
try {
    $wc3 = [System.Net.WebClient]::new()
    $wc3.Headers.Add("Content-Type", "application/json")
    $result3 = $wc3.UploadString("http://localhost:8080/api/vin/decode", "{`"vin`":`"INVALID`"}")
    Write-Host "Result: $result3"
} catch {
    $reader = [System.IO.StreamReader]::new($_.Exception.InnerException.Response.GetResponseStream())
    $errBody = $reader.ReadToEnd()
    Write-Host "Expected error: $errBody" -ForegroundColor Green
}

Write-Host "`n=== Done ===" -ForegroundColor Cyan
