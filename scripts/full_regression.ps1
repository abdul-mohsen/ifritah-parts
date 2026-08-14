# Full regression test with enrichment verification
$p = Start-Process -FilePath "c:\ssda\chatGPT\parts-engine\server.exe" -PassThru
Start-Sleep -Seconds 5

$pass = 0; $fail = 0; $enriched = 0
$failList = @()
$tests = @(
    @("KM8JT3AF5FU057722","HYUNDAI","2015"), @("MHFBT9F34G6006150","TOYOTA","2016"),
    @("5NPE34AF7FH123456","HYUNDAI","2015"), @("KNDPN3AC0L7777777","KIA","2020"),
    @("5XYP34A53GP000001","KIA","2016"), @("KMTG34LA1LU000001","GENESIS","2020"),
    @("4T1BF1FK5HU000001","TOYOTA","2017"), @("JTDKN3DU5A0000001","TOYOTA","2010"),
    @("2T1BURHE5HC000001","TOYOTA","2017"), @("5TFJX4GN8LX000001","TOYOTA","2020"),
    @("JTJ77AHZ5H2000001","LEXUS","2017"), @("1HGBH41JXMN100001","HONDA","2021"),
    @("5J6RW2H85KA000001","HONDA","2019"), @("19UUB2F34LA000001","ACURA","2020"),
    @("1N4AL3AP8EC000001","NISSAN","2014"), @("JN8AT2MV5HW000001","NISSAN","2017"),
    @("5N3AA58C57N000001","INFINITI","2007"), @("JM5TCAWY8K0317954","MAZDA","2019"),
    @("JM1BN1V73M1000001","MAZDA","2021"), @("JF2SJABC5GH000001","SUBARU","2016"),
    @("4S4BSANC7L3000001","SUBARU","2020"), @("JA4AZ3A35HZ000001","MITSUBISHI","2017"),
    @("4A4AR3AU5FE000001","MITSUBISHI","2015"), @("JS3TD942574000001","SUZUKI","2007"),
    @("1FTFW1ET5EKE00001","FORD","2014"), @("1FM5K8D82FGA00001","FORD","2015"),
    @("1FA6P8CF1L5000001","FORD","2020"), @("5LMJJ2JT9LEL00001","LINCOLN","2020"),
    @("1G1YY22G465000001","CHEVROLET","2006"), @("1GCUYDED5LZ000001","CHEVROLET","2020"),
    @("1GKS1AEJ8FR000001","GMC","2015"), @("1GYS3BKJ5FR000001","CADILLAC","2015"),
    @("1G4HP54K174000001","BUICK","2007"), @("1C6RR7LT7HS000001","RAM","2017"),
    @("1B3CC5FB5AN000001","DODGE","2010"), @("1J4GA59167L000001","JEEP","2007"),
    @("5YJ3E1EA1LF000001","TESLA","2020"), @("WBAPH5C55BA000001","BMW","2011"),
    @("5UXKR0C58E0000001","BMW","2014"), @("WMWXP5C55DT000001","MINI","2013"),
    @("WDDHF5KB1EA000001","MERCEDES-BENZ","2014"), @("4JGDF7CE5FA000001","MERCEDES-BENZ","2015"),
    @("WAUDFAFC2DN000001","AUDI","2013"), @("WA1LFAFP5EA000001","AUDI","2014"),
    @("WVWZZZ3CZWE000001","VOLKSWAGEN","2028"), @("3VW2B7AJ8DM000001","VOLKSWAGEN","2013"),
    @("WP0AB2A95ES000001","PORSCHE","2014"), @("WP1AA2A27HLA00001","PORSCHE","2017"),
    @("YV1RS592582000001","VOLVO","2008"), @("SAJWA0ES7DPS00001","JAGUAR","2013"),
    @("SALGS2RE7LA000001","LAND ROVER","2020"), @("SCBFR7ZA5FC000001","BENTLEY","2015"),
    @("SCA664S57LUX00001","ROLLS-ROYCE","2020"), @("ZFF67NFA1E0000001","FERRARI","2014"),
    @("ZHWUC1ZF7HLA00001","LAMBORGHINI","2017"), @("ZAM57RTA1F1000001","MASERATI","2015"),
    @("ZFA31200011000001","FIAT","2001"), @("1N4AA6AP9HC406410","NISSAN","2017")
)

foreach ($t in $tests) {
    $vin = $t[0]; $expMake = $t[1]; $expYear = $t[2]
    try {
        $wc = [System.Net.WebClient]::new()
        $wc.Headers.Add("Content-Type","application/json")
        $json = $wc.UploadString("http://localhost:8080/api/vin/decode","POST","{`"vin`":`"$vin`"}")
        $data = ($json | ConvertFrom-Json).nhtsaRaw
        $makeOk = $data.make -eq $expMake
        $yearOk = $data.modelYear -eq $expYear
        if ($makeOk -and $yearOk) {
            $pass++
            if ($data.dataSources -and $data.dataSources.Count -gt 0) { $enriched++ }
        } else {
            $fail++
            $failList += "FAIL $vin exp=$expMake/$expYear got=$($data.make)/$($data.modelYear)"
        }
    } catch { $fail++; $failList += "ERROR $vin $_" }
}

Write-Host "Results: $pass passed, $fail failed out of $($tests.Count) ($enriched enriched)"
foreach ($f in $failList) { Write-Host "  $f" }

taskkill /F /PID $p.Id 2>$null | Out-Null
