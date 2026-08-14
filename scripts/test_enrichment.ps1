# Test VIN decode with enrichment data
$baseUrl = "http://localhost:8080"

# Test VINs with expected enrichment
$tests = @(
    @{vin="1N4AA6AP9HC406410"; make="NISSAN"; model="Maxima"; desc="2017 Nissan Maxima"},
    @{vin="KMHD35LH5GU200001"; make="HYUNDAI"; model="Elantra"; desc="2016 Hyundai Elantra"},
    @{vin="5YJ3E1EA1JF000001"; make="TESLA"; model="Model 3"; desc="2018 Tesla Model 3"},
    @{vin="4T1BF1FK5HU000001"; make="TOYOTA"; model="Camry"; desc="2017 Toyota Camry"},
    @{vin="1HGCV1F34JA000001"; make="HONDA"; model="Accord"; desc="2018 Honda Accord"}
)

$pass = 0; $fail = 0

foreach ($t in $tests) {
    try {
        $body = @{vin=$t.vin} | ConvertTo-Json
        $resp = [System.Net.WebClient]::new()
        $resp.Headers.Add("Content-Type", "application/json")
        $json = $resp.UploadString("$baseUrl/api/vin/decode", "POST", $body)
        $data = $json | ConvertFrom-Json

        $nhtsaRaw = $data.nhtsaRaw
        $makeOk = $nhtsaRaw.make -eq $t.make
        $modelOk = $nhtsaRaw.model -like "*$($t.model)*"
        $hasMPG = $nhtsaRaw.combinedMPG -gt 0
        $hasTrans = $nhtsaRaw.transmission -ne $null -and $nhtsaRaw.transmission -ne ""
        $hasSources = $nhtsaRaw.dataSources -ne $null -and $nhtsaRaw.dataSources.Count -gt 0

        $status = if ($makeOk -and $modelOk) { "PASS" } else { "FAIL" }
        if ($status -eq "PASS") { $pass++ } else { $fail++ }

        Write-Host "$status $($t.desc)"
        Write-Host "  Make: $($nhtsaRaw.make) | Model: $($nhtsaRaw.model) | Year: $($nhtsaRaw.modelYear)"
        if ($hasTrans) { Write-Host "  Transmission: $($nhtsaRaw.transmission)" }
        if ($hasMPG) { Write-Host "  MPG: $($nhtsaRaw.cityMPG)/$($nhtsaRaw.highwayMPG)/$($nhtsaRaw.combinedMPG)" }
        if ($nhtsaRaw.vehicleClass) { Write-Host "  Class: $($nhtsaRaw.vehicleClass)" }
        if ($hasSources) { Write-Host "  DataSources: $($nhtsaRaw.dataSources -join ', ')" }
        Write-Host ""
    } catch {
        $fail++
        Write-Host "FAIL $($t.desc): $_"
    }
}

Write-Host "Results: $pass passed, $fail failed"
