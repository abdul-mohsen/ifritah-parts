$ErrorActionPreference = "Stop"
Write-Host "=== Comprehensive VIN Decoder Test ===" -ForegroundColor Cyan
Write-Host ""

try {
    $h = [System.Net.WebClient]::new().DownloadString("http://localhost:8080/health")
    Write-Host "Server: OK" -ForegroundColor Green
} catch {
    Write-Host "Server not running! Start server.exe first." -ForegroundColor Red
    exit 1
}

$tests = @(
    @("KM8JT3AF5FU057722", "HYUNDAI",       "Tucson",              2015),
    @("MHFBT9F34G6006150", "TOYOTA",        "Avanza/Yaris",        2016),
    @("5NPE34AF7FH123456", "HYUNDAI",       "Sonata",              2015),
    @("KNDPN3AC0L7777777", "KIA",           "Sportage",            2020),
    @("5XYP34A53GP000001", "KIA",           "Sorento",             2016),
    @("KMTG34LA1LU000001", "GENESIS",       "G70",                2020),
    @("4T1BF1FK5HU000001", "TOYOTA",        "Camry",               2017),
    @("JTDKN3DU5A0000001", "TOYOTA",        "Prius",              2010),
    @("2T1BURHE5HC000001", "TOYOTA",        "Corolla",             2017),
    @("5TFJX4GN8LX000001", "TOYOTA",        "Tacoma",             2020),
    @("JTJ77AHZ5H2000001", "LEXUS",         "NX",                 2017),
    @("1HGBH41JXMN100001", "HONDA",         "Civic/Accord",        2021),
    @("5J6RW2H85KA000001", "HONDA",         "CR-V",                2019),
    @("19UUB2F34LA000001", "ACURA",         "TLX",                2020),
    @("1N4AL3AP8EC000001", "NISSAN",        "Altima",             2014),
    @("JN8AT2MV5HW000001", "NISSAN",        "Rogue",              2017),
    @("5N3AA58C57N000001", "INFINITI",      "Unknown",             2007),
    @("JM5TCAWY8K0317954", "MAZDA",         "CX-9/CX-50",         2019),
    @("JM1BN1V73M1000001", "MAZDA",         "Mazda3/Mazda6",       2021),
    @("JF2SJABC5GH000001", "SUBARU",        "Forester",           2016),
    @("4S4BSANC7L3000001", "SUBARU",        "Forester/Outback",    2020),
    @("JA4AZ3A35HZ000001", "MITSUBISHI",    "Outlander",          2017),
    @("4A4AR3AU5FE000001", "MITSUBISHI",    "Outlander Sport",   2015),
    @("JS3TD942574000001", "SUZUKI",        "Unknown",             2007),
    @("1FTFW1ET5EKE00001", "FORD",          "F-150",              2014),
    @("1FM5K8D82FGA00001", "FORD",          "Explorer",           2015),
    @("1FA6P8CF1L5000001", "FORD",          "Mustang",            2020),
    @("5LMJJ2JT9LEL00001", "LINCOLN",       "Navigator/Aviator",   2020),
    @("1G1YY22G465000001", "CHEVROLET",     "Corvette",           2006),
    @("1GCUYDED5LZ000001", "CHEVROLET",     "Silverado",           2020),
    @("1GKS1AEJ8FR000001", "GMC",           "Yukon",              2015),
    @("1GYS3BKJ5FR000001", "CADILLAC",      "Escalade",            2015),
    @("1G4HP54K174000001", "BUICK",         "Lucerne",            2007),
    @("1C6RR7LT7HS000001", "RAM",           "1500",               2017),
    @("1B3CC5FB5AN000001", "DODGE",         "Avenger",            2010),
    @("1J4GA59167L000001", "JEEP",          "Wrangler",           2007),
    @("5YJ3E1EA1LF000001", "TESLA",         "Model 3",            2020),
    @("WBAPH5C55BA000001", "BMW",           "328i",               2011),
    @("5UXKR0C58E0000001", "BMW",           "X5",                 2014),
    @("WMWXP5C55DT000001", "MINI",          "Unknown",             2013),
    @("WDDHF5KB1EA000001", "MERCEDES-BENZ", "E-Class",            2014),
    @("4JGDF7CE5FA000001", "MERCEDES-BENZ", "GL-Class",           2015),
    @("WAUDFAFC2DN000001", "AUDI",          "A6",                 2013),
    @("WA1LFAFP5EA000001", "AUDI",          "Q5",                 2014),
    @("WVWZZZ3CZWE000001", "VOLKSWAGEN",    "Unknown",             2028),
    @("3VW2B7AJ8DM000001", "VOLKSWAGEN",    "Jetta",              2013),
    @("WP0AB2A95ES000001", "PORSCHE",       "911",                2014),
    @("WP1AA2A27HLA00001", "PORSCHE",       "Cayenne",            2017),
    @("YV1RS592582000001", "VOLVO",         "S60",                2008),
    @("SAJWA0ES7DPS00001", "JAGUAR",        "XF",                 2013),
    @("SALGS2RE7LA000001", "LAND ROVER",    "Range Rover",        2020),
    @("SCBFR7ZA5FC000001", "BENTLEY",       "Continental",        2015),
    @("SCA664S57LUX00001", "ROLLS-ROYCE",   "Unknown",             2020),
    @("ZFF67NFA1E0000001", "FERRARI",       "458 Italia",         2014),
    @("ZHWUC1ZF7HLA00001", "LAMBORGHINI",   "Huracan",            2017),
    @("ZAM57RTA1F1000001", "MASERATI",      "Ghibli",             2015),
    @("ZFA31200011000001", "FIAT",          "Unknown",             2001),
    @("ZARFAEBN5L7000001", "ALFA ROMEO",    "Giulia (952)",       2020),
    @("VF3LCBHZ6ES000001", "PEUGEOT",       "Unknown",             2014),
    @("VF77J5FK8DV000001", "CITROEN",       "Unknown",             2013),
    @("VF1RFB00856000001", "RENAULT",       "Unknown",             2005),
    @("W0LPE6EJ3H7000001", "OPEL",          "Unknown",             2017),
    @("VSSZZZ5PZHR000001", "SEAT",          "Unknown",             2017),
    @("TMBGE61Z582000001", "SKODA",         "Unknown",             2008),
    @("LGXCE4CB5P0000001", "BYD",           "Han/Seal/Dolphin",    2023),
    @("JAACL14L760000001", "ISUZU",         "Unknown",             2006),
    @("JF1GD67697S000001", "SUBARU",        "Impreza",            2007),
    # --- New WMI coverage tests ---
    # Hyundai global plants
    @("NLHB251AAP1000001", "HYUNDAI",       "Santa Fe",            2023),
    @("MF3AA00A0P0000001", "HYUNDAI",       "Accent",              2023),
    @("LBEA000A0N0000001", "HYUNDAI",       "Accent",              2022),
    @("MB2AA00A0M0000001", "HYUNDAI",       "Accent",              2021),
    @("Z94AA00A0L0000001", "HYUNDAI",       "Accent",              2020),
    @("AC5AA00A0K0000001", "HYUNDAI",       "Accent",              2019),
    @("7YAAA00A0S0000001", "HYUNDAI",       "Accent",              2025),
    @("2HMAA00A0R0000001", "HYUNDAI",       "Accent",              2024),
    @("TMCAA00A0N0000001", "HYUNDAI",       "Accent",              2022),
    # Kia global plants
    @("U6YAA00A0R0000001", "KIA",           "Forte",               2024),
    @("3KPAA00A0P0000001", "KIA",           "Forte",               2023),
    @("3KMAA00A0N0000001", "KIA",           "Forte",               2022),
    @("MZBAA00A0S0000001", "KIA",           "Forte",               2025),
    # Toyota global plants
    @("MR0AA0000H0000001", "TOYOTA",        "Hilux/Fortuner",      2017),
    @("MR2AA0000K0000001", "TOYOTA",        "Yaris/Corolla",       2019),
    @("NMTAA0000R0000001", "TOYOTA",        "Unknown",             2024),
    @("VNKAA0000N0000001", "TOYOTA",        "Unknown",             2022),
    @("SB1AA0000M0000001", "TOYOTA",        "Unknown",             2021),
    @("LVGA000A0K0000001", "TOYOTA",        "Unknown",             2019),
    @("LTVA000A0L0000001", "TOYOTA",        "Unknown",             2020),
    @("5TBAA00A0R0000001", "TOYOTA",        "Tundra/Tacoma",       2024),
    @("3TMAA00A0S0000001", "TOYOTA",        "Unknown",             2025),
    # Lexus additional
    @("JT6AA00A0R0000001", "LEXUS",         "Unknown",             2024),
    @("JT8AA00A0N0000001", "LEXUS",         "Unknown",             2022),
    @("2T2AA00A0M0000001", "LEXUS",         "RX/NX",               2021),
    @("58AAA00A0S0000001", "LEXUS",         "Unknown",             2025),
    # Honda global
    @("JHLRD7840CA000001", "HONDA",         "CR-V/Pilot/HR-V",    2012),
    @("19XAA00A0R0000001", "HONDA",         "Unknown",             2024),
    @("5FPAA00A0S0000001", "HONDA",         "Ridgeline/Passport", 2025),
    @("5KBAA00A0N0000001", "HONDA",         "Civic/Accord",       2022),
    @("7FAAA00A0R0000001", "HONDA",         "Unknown",             2024),
    @("3CZAA00A0M0000001", "HONDA",         "Unknown",             2021),
    @("LHGAA00A0L0000001", "HONDA",         "Unknown",             2020),
    # Acura additional
    @("19VAA00A0N0000001", "ACURA",         "Unknown",             2022),
    @("5J8AA00A0R0000001", "ACURA",         "Unknown",             2024),
    @("5FRAA00A0M0000001", "ACURA",         "Unknown",             2021),
    # Nissan additional
    @("LGBA000A0K0000001", "NISSAN",        "Unknown",             2019),
    @("3N8AA00A0R0000001", "NISSAN",        "Unknown",             2024),
    @("LJNAA00A0N0000001", "NISSAN",        "Unknown",             2022),
    # Infiniti additional
    @("JNRAA00A0R0000001", "INFINITI",      "Unknown",             2024),
    @("SJKAA00A0M0000001", "INFINITI",      "Unknown",             2021),
    # Mazda additional
    @("JMZAA00A0R0000001", "MAZDA",         "Unknown",             2024),
    @("JM0AA00A0N0000001", "MAZDA",         "Unknown",             2022),
    @("7MMAA00A0S0000001", "MAZDA",         "Unknown",             2025),
    # Subaru additional
    @("JF3AA00A0R0000001", "SUBARU",        "Unknown",             2024),
    # Mitsubishi additional
    @("ML3AA00A0K0000001", "MITSUBISHI",    "Mirage",             2019),
    @("MK2AA00A0N0000001", "MITSUBISHI",    "Unknown",             2022),
    @("JMAAA00A0R0000001", "MITSUBISHI",    "Unknown",             2024),
    # Suzuki additional
    @("JSAAA00A0R0000001", "SUZUKI",        "Unknown",             2024),
    @("MBHAA00A0N0000001", "SUZUKI",        "Unknown",             2022),
    # Ford additional
    @("2FAAA00A0R0000001", "FORD",          "Unknown",             2024),
    @("3FTAA00A0S0000001", "FORD",          "F-150/Maverick",      2025),
    @("NM0AA00A0N0000001", "FORD",          "Unknown",             2022),
    @("MPBAA00A0R0000001", "FORD",          "Unknown",             2024),
    # Lincoln additional
    @("2LMAA00A0R0000001", "LINCOLN",       "Navigator/Aviator",   2024),
    # Chevrolet additional
    @("2GCAA00A0N0000001", "CHEVROLET",     "Unknown",             2022),
    @("2GNAA00A0R0000001", "CHEVROLET",     "Unknown",             2024),
    # GMC additional
    @("2GKAA00A0R0000001", "GMC",           "Unknown",             2024),
    @("3GKAA00A0N0000001", "GMC",           "Unknown",             2022),
    # Cadillac additional
    @("3GYAA00A0R0000001", "CADILLAC",      "Escalade",            2024),
    # Buick additional
    @("2G4AA00A0R0000001", "BUICK",         "Unknown",             2024),
    @("5GAAA00A0N0000001", "BUICK",         "Unknown",             2022),
    # Dodge additional
    @("1D3AA00A0R0000001", "DODGE",         "Unknown",             2024),
    @("1D4AA00A0N0000001", "DODGE",         "Unknown",             2022),
    # Jeep additional
    @("ZACAA00A0R0000001", "JEEP",          "Unknown",             2024),
    # Tesla additional
    @("XP7AA00A0R0000001", "TESLA",         "Model Y",             2024),
    @("7G2AA00A0S0000001", "TESLA",         "Unknown",             2025),
    # Rivian
    @("7FCAA00A0R0000001", "RIVIAN",        "R1T",                 2024),
    @("7PDAA00A0S0000001", "RIVIAN",        "R1S",                 2025),
    # Lucid
    @("50EAA00A0R0000001", "LUCID",         "Unknown",             2024),
    @("7UUAA00A0S0000001", "LUCID",         "Unknown",             2025),
    # BMW additional
    @("WBXAA00A0R0000001", "BMW",           "Unknown",             2024),
    @("5UMAA00A0N0000001", "BMW",           "M3/M4/M5",            2022),
    @("4USAA00A0R0000001", "BMW",           "Unknown",             2024),
    @("3MWAA00A0S0000001", "BMW",           "Unknown",             2025),
    @("LBVAA00A0N0000001", "BMW",           "Unknown",             2022),
    # MINI additional
    @("WMZAA00A0R0000001", "MINI",          "Unknown",             2024),
    # Mercedes additional
    @("W1KAA00A0R0000001", "MERCEDES-BENZ", "Unknown",             2024),
    @("W1NAA00A0N0000001", "MERCEDES-BENZ", "GLC/GLE/GLS",         2022),
    @("55SAA00A0S0000001", "MERCEDES-BENZ", "Unknown",             2025),
    @("NLEAA00A0R0000001", "MERCEDES-BENZ", "Unknown",             2024),
    @("NMBAA00A0N0000001", "MERCEDES-BENZ", "Unknown",             2022),
    # Smart
    @("WMEAA00A0R0000001", "SMART",         "Unknown",             2024),
    @("W1AAA00A0N0000001", "SMART",         "Unknown",             2022),
    # Audi additional
    @("WUAAA00A0R0000001", "AUDI",          "Unknown",             2024),
    @("WU1AA00A0N0000001", "AUDI",          "Unknown",             2022),
    # VW additional
    @("WVGAA00A0R0000001", "VOLKSWAGEN",    "Tiguan/Atlas",        2024),
    @("3VVAA00A0N0000001", "VOLKSWAGEN",    "Taos/Atlas",          2022),
    @("LFVAA00A0R0000001", "VOLKSWAGEN",    "Unknown",             2024),
    @("LSVAA00A0N0000001", "VOLKSWAGEN",    "Unknown",             2022),
    # Volvo additional
    @("7JDAA00A0R0000001", "VOLVO",         "Unknown",             2024),
    @("LYVAA00A0N0000001", "VOLVO",         "XC90",               2022),
    # Polestar
    @("LPSAA00A0R0000001", "POLESTAR",      "Unknown",             2024),
    @("YSMAA00A0N0000001", "POLESTAR",      "Unknown",             2022),
    @("7SYAA00A0S0000001", "POLESTAR",      "Unknown",             2025),
    # Fiat additional
    @("ZFBAA00A0R0000001", "FIAT",          "Unknown",             2024),
    # Alfa Romeo additional
    @("ZASAA00A0R0000001", "ALFA ROMEO",    "Unknown",             2024),
    # Maserati additional
    @("ZN6AA00A0R0000001", "MASERATI",      "Unknown",             2024),
    # Lamborghini additional
    @("ZPBAA00A0R0000001", "LAMBORGHINI",   "Unknown",             2024),
    # Aston Martin additional
    @("SD7AA00A0R0000001", "ASTON MARTIN",  "Unknown",             2024),
    # Bentley additional
    @("SJAAA00A0R0000001", "BENTLEY",       "Unknown",             2024),
    # Rolls-Royce additional
    @("SLAAA00A0R0000001", "ROLLS-ROYCE",   "Unknown",             2024),
    # INEOS
    @("SC6AA00A0R0000001", "INEOS",         "Unknown",             2024),
    # Peugeot additional
    @("VR3AA00A0R0000001", "PEUGEOT",       "Unknown",             2024),
    # Citroen additional
    @("VR7AA00A0R0000001", "CITROEN",       "Unknown",             2024),
    # DS
    @("VR1AA00A0R0000001", "DS",            "Unknown",             2024),
    # Renault additional
    @("UU1AA00A0R0000001", "RENAULT",       "Unknown",             2024),
    # Opel additional
    @("W0VAA00A0R0000001", "OPEL",          "Unknown",             2024),
    # BYD additional
    @("LC0AA00A0R0000001", "BYD",           "K7M",                2024),
    @("LPEAA00A0N0000001", "BYD",           "Unknown",             2022),
    # Geely
    @("L6TAA00A0R0000001", "GEELY",         "Unknown",             2024),
    @("LB3AA00A0N0000001", "GEELY",         "Unknown",             2022),
    # NIO
    @("LJ1AA00A0R0000001", "NIO",           "Unknown",             2024),
    # XPeng
    @("L1NAA00A0R0000001", "XPENG",         "Unknown",             2024),
    # Li Auto
    @("LW4AA00A0R0000001", "LI AUTO",       "Unknown",             2024),
    # Great Wall
    @("LGWAA00A0R0000001", "GREAT WALL",    "Unknown",             2024),
    # Chery
    @("LNNAA00A0R0000001", "CHERY",         "Unknown",             2024),
    # VinFast
    @("RLLAA00A0R0000001", "VINFAST",       "Unknown",             2024),
    @("RLNAA00A0S0000001", "VINFAST",       "Unknown",             2025),
    # Mahindra
    @("MABAA00A0R0000001", "MAHINDRA",      "Unknown",             2024),
    # Tata
    @("MATAA00A0R0000001", "TATA",          "Unknown",             2024),
    # Maruti Suzuki
    @("MA3AA00A0R0000001", "MARUTI SUZUKI", "Unknown",             2024),
    # Daihatsu
    @("JDAAA00A0R0000001", "DAIHATSU",      "Unknown",             2024),
    # Saab
    @("YS3AA00A0R0000001", "SAAB",          "Unknown",             2024),
    # Isuzu additional
    @("MPAAA00A0R0000001", "ISUZU",         "Unknown",             2024)
)

$pass = 0
$fail = 0
$fmt = "{0,-20} {1,-16} {2,-16} {3,-8} {4,-8} {5}"
Write-Host ($fmt -f "VIN", "Exp Make", "Got Make", "ExpYr", "GotYr", "Result") -ForegroundColor Yellow
Write-Host ("-" * 100) -ForegroundColor DarkGray

foreach ($t in $tests) {
    $vin = $t[0]
    $expMake = $t[1]
    $expModel = $t[2]
    $expYear = $t[3]

    try {
        $wc = [System.Net.WebClient]::new()
        $wc.Headers.Add("Content-Type", "application/json")
        $body = '{"vin":"' + $vin + '"}'
        $result = $wc.UploadString("http://localhost:8080/api/vin/decode", $body) | ConvertFrom-Json
        $gotMake = $result.nhtsaRaw.make
        $gotModel = $result.nhtsaRaw.model
        $gotYear = [int]$result.nhtsaRaw.modelYear

        $makeOk = $gotMake -eq $expMake
        $yearOk = $gotYear -eq $expYear
        $modelOk = $gotModel -eq $expModel

        if ($makeOk -and $yearOk -and $modelOk) {
            $status = "PASS"
            $color = "Green"
            $pass++
        } else {
            $status = "FAIL"
            $color = "Red"
            $fail++
            if (-not $makeOk) { $status += " make=$gotMake" }
            if (-not $yearOk) { $status += " year=$gotYear" }
            if (-not $modelOk) { $status += " model=$gotModel" }
        }
        Write-Host ($fmt -f $vin.Substring(0,[Math]::Min(17,$vin.Length)), $expMake, $gotMake, $expYear, $gotYear, $status) -ForegroundColor $color
    } catch {
        $fail++
        Write-Host ($fmt -f $vin.Substring(0,[Math]::Min(17,$vin.Length)), $expMake, "ERROR", $expYear, "?", "FAIL: $_") -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "=== Results: $pass PASSED, $fail FAILED out of $($tests.Count) ===" -ForegroundColor $(if ($fail -eq 0) { "Green" } else { "Yellow" })
