$ErrorActionPreference = "Stop"
Write-Host "Testing VIN: JM5TCAWY8K0317954" -ForegroundColor Cyan
Start-Sleep 2
try {
    $wc = [System.Net.WebClient]::new()
    $wc.Headers.Add("Content-Type", "application/json")
    $result = $wc.UploadString("http://localhost:8080/api/vin/decode", '{"vin":"JM5TCAWY8K0317954"}')
    $json = $result | ConvertFrom-Json
    Write-Host "Make:    $($json.nhtsaRaw.make)" -ForegroundColor Green
    Write-Host "Model:   $($json.nhtsaRaw.model)" -ForegroundColor Green
    Write-Host "Year:    $($json.nhtsaRaw.modelYear)" -ForegroundColor Green
    Write-Host "Country: $($json.nhtsaRaw.plantCountry)" -ForegroundColor Green
    Write-Host "`nFull JSON:" -ForegroundColor Gray
    Write-Host ($json | ConvertTo-Json -Depth 5)
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
