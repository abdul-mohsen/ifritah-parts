$endpoints = @(
    @{ Name = "Health"; Uri = "http://localhost:8080/health" }
    @{ Name = "SmartSearch OEM"; Uri = "http://localhost:8080/api/search?q=26300-35505" }
    @{ Name = "SmartSearch Text"; Uri = "http://localhost:8080/api/search?q=oil+filter" }
    @{ Name = "SmartSearch article"; Uri = "http://localhost:8080/api/search?q=0+986+AF1+014" }
    @{ Name = "Catalog Models Hyundai"; Uri = "http://localhost:8080/api/catalog/models?make=HYUNDAI" }
    @{ Name = "Catalog Models Kia"; Uri = "http://localhost:8080/api/catalog/models?make=KIA" }
    @{ Name = "Catalog Vehicles"; Uri = "http://localhost:8080/api/catalog/vehicles?make=HYUNDAI&model=TUCSON" }
    @{ Name = "Catalog Groups"; Uri = "http://localhost:8080/api/catalog/groups?vehicleId=1" }
    @{ Name = "Catalog Parts"; Uri = "http://localhost:8080/api/catalog/parts?vehicleId=1&groupId=10100" }
    @{ Name = "OEM Lookup"; Uri = "http://localhost:8080/api/oem/26300-35505" }
)

foreach ($ep in $endpoints) {
    try {
        $r = Invoke-WebRequest -Uri $ep.Uri -UseBasicParsing -TimeoutSec 5
        $json = $r.Content | ConvertFrom-Json
        Write-Host "PASS $($ep.Name) ($($r.StatusCode))" -ForegroundColor Green
        Write-Host "  -> $($r.Content.Substring(0, [Math]::Min(200, $r.Content.Length)))"
    } catch {
        Write-Host "FAIL $($ep.Name): $_" -ForegroundColor Red
    }
}
