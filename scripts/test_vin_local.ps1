$ErrorActionPreference = "Stop"

Write-Host "=== VIN Decode API Test (Local Decoder) ===" -ForegroundColor Cyan

# 1. Health check
Write-Host "`n--- Health Check ---" -ForegroundColor Yellow
try {
    $health = [System.Net.WebClient]::new().DownloadString("http://localhost:8080/health")
    Write-Host "OK: $health" -ForegroundColor Green
} catch {
    Write-Host "Server not running! Run restart_server.ps1 first." -ForegroundColor Red
    exit 1
}

function Test-VIN($vin, $label) {
    Write-Host "`n--- $label ($vin) ---" -ForegroundColor Yellow
    try {
        $wc = [System.Net.WebClient]::new()
        $wc.Headers.Add("Content-Type", "application/json")
        $result = $wc.UploadString("http://localhost:8080/api/vin/decode", "{`"vin`":`"$vin`"}")
        $json = $result | ConvertFrom-Json
        Write-Host "  Make:    $($json.nhtsaRaw.make)" -ForegroundColor White
        Write-Host "  Model:   $($json.nhtsaRaw.model)" -ForegroundColor White
        Write-Host "  Year:    $($json.nhtsaRaw.modelYear)" -ForegroundColor White
        Write-Host "  Country: $($json.nhtsaRaw.plantCountry)" -ForegroundColor White
        if ($json.dbWarning) {
            Write-Host "  DB:      (no DB)" -ForegroundColor DarkYellow
        }
        Write-Host "`n  Full JSON:" -ForegroundColor Gray
        Write-Host ($json | ConvertTo-Json -Depth 5)
    } catch {
        if ($_.Exception.InnerException.Response) {
            $reader = [System.IO.StreamReader]::new($_.Exception.InnerException.Response.GetResponseStream())
            $errBody = $reader.ReadToEnd()
            Write-Host "  Error: $errBody" -ForegroundColor Red
        } else {
            Write-Host "  Error: $_" -ForegroundColor Red
        }
    }
}

# Hyundai VINs
Test-VIN "KM8JT3AF5FU057722" "Hyundai Tucson 2015"
Test-VIN "5NPE34AF7FH123456" "Hyundai Sonata 2015"
Test-VIN "KMH25412AKA004567" "Hyundai Accent 2019"

# Kia VINs
Test-VIN "KNDPN3AC0L7777777" "Kia Sportage 2020"
Test-VIN "5XYPH4A50GG012345" "Kia Sorento 2016"

# Other brands
Test-VIN "JM3TCAWY8K0317954" "Mazda CX-9 2019"
Test-VIN "WBAJB0C51JB084264" "BMW 5 Series 2018"

# Invalid
Write-Host "`n--- Invalid VIN ---" -ForegroundColor Yellow
try {
    $wc = [System.Net.WebClient]::new()
    $wc.Headers.Add("Content-Type", "application/json")
    $wc.UploadString("http://localhost:8080/api/vin/decode", "{`"vin`":`"BAD`"}") | Out-Null
} catch {
    $reader = [System.IO.StreamReader]::new($_.Exception.InnerException.Response.GetResponseStream())
    $errBody = $reader.ReadToEnd()
    Write-Host "  Correctly rejected: $errBody" -ForegroundColor Green
}

Write-Host "`n=== Done ===" -ForegroundColor Cyan
