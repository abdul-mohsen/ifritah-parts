# Start server, wait, then run tests
cd c:\ssda\chatGPT\parts-engine
$proc = Start-Process -FilePath ".\server.exe" -PassThru
Start-Sleep -Seconds 5

# Quick health check
try {
    $wc = [System.Net.WebClient]::new()
    $health = $wc.DownloadString("http://localhost:8080/health")
    Write-Host "Server OK: $health"
} catch {
    Write-Host "Server not responding: $_"
    exit 1
}

# Run test suite
$pass = 0; $fail = 0; $total = 0
$failList = @()

# Same test data from test_all_brands.ps1
$tests = @(
    # Hyundai
    @("KM8JT3AF5FU057722","HYUNDAI","2015"),
    @("MHFBT9F34G6006150","TOYOTA","2016"),
    @("5NPE34AF7FH123456","HYUNDAI","2015"),
    @("KNDPN3AC0L7777777","KIA","2020"),
    @("5XYP34A53GP000001","KIA","2016"),
    @("KMTG34LA1LU000001","GENESIS","2020"),
    # Toyota/Lexus/Honda/Acura/Nissan/Infiniti
    @("4T1BF1FK5HU000001","TOYOTA","2017"),
    @("JTDKN3DU5A0000001","TOYOTA","2010"),
    @("2T1BURHE5HC000001","TOYOTA","2017"),
    @("5TFJX4GN8LX000001","TOYOTA","2020"),
    @("JTJ77AHZ5H2000001","LEXUS","2017"),
    @("1HGBH41JXMN100001","HONDA","2021"),
    @("5J6RW2H85KA000001","HONDA","2019"),
    @("19UUB2F34LA000001","ACURA","2020"),
    @("1N4AL3AP8EC000001","NISSAN","2014"),
    @("JN8AT2MV5HW000001","NISSAN","2017"),
    @("5N3AA58C57N000001","INFINITI","2007"),
    # Mazda/Subaru/Mitsubishi/Suzuki
    @("JM5TCAWY8K0317954","MAZDA","2019"),
    @("JM1BN1V73M1000001","MAZDA","2021"),
    @("JF2SJABC5GH000001","SUBARU","2016"),
    @("4S4BSANC7L3000001","SUBARU","2020"),
    @("JA4AZ3A35HZ000001","MITSUBISHI","2017"),
    @("4A4AR3AU5FE000001","MITSUBISHI","2015"),
    @("JS3TD942574000001","SUZUKI","2007"),
    # Ford/Lincoln
    @("1FTFW1ET5EKE00001","FORD","2014"),
    @("1FM5K8D82FGA00001","FORD","2015"),
    @("1FA6P8CF1L5000001","FORD","2020"),
    @("5LMJJ2JT9LEL00001","LINCOLN","2020"),
    # GM
    @("1G1YY22G465000001","CHEVROLET","2006"),
    @("1GCUYDED5LZ000001","CHEVROLET","2020"),
    @("1GKS1AEJ8FR000001","GMC","2015"),
    @("1GYS3BKJ5FR000001","CADILLAC","2015"),
    @("1G4HP54K174000001","BUICK","2007"),
    # Chrysler/Ram/Dodge/Jeep
    @("1C6RR7LT7HS000001","RAM","2017"),
    @("1B3CC5FB5AN000001","DODGE","2010"),
    @("1J4GA59167L000001","JEEP","2007"),
    # Tesla
    @("5YJ3E1EA1LF000001","TESLA","2020"),
    # European
    @("WBAPH5C55BA000001","BMW","2011"),
    @("5UXKR0C58E0000001","BMW","2014"),
    @("WMWXP5C55DT000001","MINI","2013"),
    @("WDDHF5KB1EA000001","MERCEDES-BENZ","2014"),
    @("4JGDF7CE5FA000001","MERCEDES-BENZ","2015"),
    @("WAUDFAFC2DN000001","AUDI","2013"),
    @("WA1LFAFP5EA000001","AUDI","2014"),
    @("WVWZZZ3CZWE000001","VOLKSWAGEN","2028"),
    @("3VW2B7AJ8DM000001","VOLKSWAGEN","2013"),
    @("WP0AB2A95ES000001","PORSCHE","2014"),
    @("WP1AA2A27HLA00001","PORSCHE","2017"),
    @("YV1RS592582000001","VOLVO","2008"),
    @("SAJWA0ES7DPS00001","JAGUAR","2013"),
    @("SALGS2RE7LA000001","LAND ROVER","2020"),
    @("SCBFR7ZA5FC000001","BENTLEY","2015"),
    @("SCA664S57LUX00001","ROLLS-ROYCE","2020"),
    @("ZFF67NFA1E0000001","FERRARI","2014"),
    @("ZHWUC1ZF7HLA00001","LAMBORGHINI","2017"),
    @("ZAM57RTA1F1000001","MASERATI","2015"),
    @("ZFA31200011000001","FIAT","2001")
)

foreach ($t in $tests) {
    $total++
    $vin = $t[0]; $expMake = $t[1]; $expYear = $t[2]
    try {
        $body = @{vin=$vin} | ConvertTo-Json
        $wc2 = [System.Net.WebClient]::new()
        $wc2.Headers.Add("Content-Type", "application/json")
        $json = $wc2.UploadString("http://localhost:8080/api/vin/decode", "POST", $body)
        $data = $json | ConvertFrom-Json
        $raw = $data.nhtsaRaw
        $makeOk = $raw.make -eq $expMake
        $yearOk = $raw.modelYear -eq $expYear
        if ($makeOk -and $yearOk) { $pass++ }
        else {
            $fail++
            $failList += "$vin exp=$expMake/$expYear got=$($raw.make)/$($raw.modelYear)"
        }
    } catch {
        $fail++
        $failList += "$vin ERROR: $_"
    }
}

Write-Host ""
Write-Host "Results: $pass passed, $fail failed out of $total"
if ($failList.Count -gt 0) {
    Write-Host ""
    Write-Host "Failures:"
    foreach ($f in $failList) { Write-Host "  $f" }
}

# Stop server
$proc | Stop-Process -Force -ErrorAction SilentlyContinue
