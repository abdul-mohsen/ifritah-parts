# Targeted enrichment test - start server, test, cleanup
$p = Start-Process -FilePath "c:\ssda\chatGPT\parts-engine\server.exe" -PassThru
Start-Sleep -Seconds 5

$results = @()
$vins = @(
    @{vin="1N4AA6AP9HC406410"; make="NISSAN"},
    @{vin="4T1BF1FK5HU000001"; make="TOYOTA"},
    @{vin="KMHD35LH5GU200001"; make="HYUNDAI"},
    @{vin="5YJ3E1EA1JF000001"; make="TESLA"},
    @{vin="1HGCV1F34JA000001"; make="HONDA"}
)

foreach ($v in $vins) {
    try {
        $wc = [System.Net.WebClient]::new()
        $wc.Headers.Add("Content-Type","application/json")
        $json = $wc.UploadString("http://localhost:8080/api/vin/decode","POST", "{`"vin`":`"$($v.vin)`"}")
        $raw = ($json | ConvertFrom-Json).nhtsaRaw
        $results += "$($raw.make) $($raw.model) MPG=$($raw.combinedMPG) Trans=$($raw.transmission) Src=$($raw.dataSources -join ',')"
    } catch {
        $results += "ERROR $($v.vin): $_"
    }
}

$results | ForEach-Object { Write-Host $_ }

# cleanup
taskkill /F /PID $p.Id 2>$null | Out-Null
