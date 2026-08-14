# probe-harsh.ps1
# Harsh Kia/Hyundai audit — fires a realistic HK query matrix at the local
# parts-engine API and writes a structured JSON test log plus a compact
# Markdown findings table.
#
# Categories:
#   - VIN decode          (5 cases; 2 golden, 3 broader)
#   - OEM search          (12 cases; 4 golden hits, 8 real-world misses)
#   - Text search         (10 cases; common consumer terms)
#   - Vehicle-scoped      (4 cases; 2016 Tucson etc.)
#   - Recall              (4 cases; HK)
#   - Catalog browse      (3 cases; models/vehicles/groups)
#   - Detail + placement  (2 cases; from golden case results)
#
# Each probe records: url, ok, http_status, elapsed_ms, results_count,
# first_result_number (if any), warnings, error, notes.
#
# Output:
#   qa/harsh-probe.json      (machine-readable)
#   qa/harsh-probe.md        (top-line results table)

param(
    [string]$BaseUrl = 'http://127.0.0.1:8080',
    [int]$TimeoutSec = 20,
    [string]$OutJson = (Join-Path (Split-Path -Parent $PSScriptRoot) 'qa\harsh-probe.json'),
    [string]$OutMd   = (Join-Path (Split-Path -Parent $PSScriptRoot) 'qa\harsh-probe.md')
)

$ErrorActionPreference = 'Stop'

function New-Probe {
    param(
        [string]$Id,
        [string]$Category,
        [string]$Description,
        [string]$Url,
        [hashtable]$Expect = @{}
    )
    [pscustomobject]@{
        id           = $Id
        category     = $Category
        description  = $Description
        url          = $Url
        expect       = $Expect
        ok           = $false
        httpStatus   = 0
        elapsedMs    = 0
        rawResponse  = $null
        resultCount  = 0
        firstArticle = ''
        warnings     = @()
        error        = ''
        verdict      = ''
        notes        = ''
    }
}

function Invoke-Probe {
    param(
        [pscustomobject]$Probe,
        [string]$Method = 'GET',
        [string]$Body = ''
    )
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $params = @{
            Uri            = $Probe.url
            TimeoutSec     = $script:TimeoutSec
            ErrorAction    = 'Stop'
            UseBasicParsing = $true
            Method         = $Method
        }
        if ($Body) {
            $params['Body']        = $Body
            $params['ContentType'] = 'application/json'
        }
        $resp = Invoke-WebRequest @params
        $Probe.httpStatus = [int]$resp.StatusCode
        $ct = "$($resp.Headers['Content-Type'])"
        if ($ct -notmatch 'json' -and $ct -match 'html') {
            # /api/* silently falling through to SPA
            $Probe.rawResponse = "SPA_FALLBACK_HTML"
            $Probe.notes = "content-type=$ct — likely SPA fallback for unmatched route"
        } else {
            try {
                $Probe.rawResponse = $resp.Content | ConvertFrom-Json -ErrorAction Stop
            } catch {
                $Probe.rawResponse = $resp.Content
            }
        }
        $Probe.ok = $true
    } catch {
        $Probe.error = $_.Exception.Message
        if ($_.Exception.Response) {
            $Probe.httpStatus = [int]$_.Exception.Response.StatusCode.value__
        }
    } finally {
        $stopwatch.Stop()
        $Probe.elapsedMs = [int]$stopwatch.Elapsed.TotalMilliseconds
    }
    $Probe
}

function Normalize-Article {
    param([string]$s)
    if (-not $s) { return '' }
    ($s -replace '[-\.\s/]', '').ToUpper()
}

function Score-SearchProbe {
    param([pscustomobject]$Probe)
    $body = $Probe.rawResponse
    if (-not $body) { $Probe.verdict = 'ERROR'; return }

    $results = @()
    if ($body.PSObject.Properties.Name -contains 'results' -and $body.results) {
        $results = @($body.results)
    }
    $Probe.resultCount = $results.Count
    if ($results.Count -gt 0) {
        $Probe.firstArticle = "$($results[0].articleNumber)"
    }
    if ($body.PSObject.Properties.Name -contains 'warnings' -and $body.warnings) {
        $Probe.warnings = @($body.warnings)
    }

    # Detect junk placeholder descriptions (scraped garbage sold as a real hit)
    $junkDescriptions = @('sign up with','sign in','create an account','login','captcha','403','404','not found','no results')
    $firstDescription = ''
    if ($results.Count -gt 0) {
        $firstDescription = "$($results[0].description)".ToLower()
        foreach ($j in $junkDescriptions) {
            if ($firstDescription -match [regex]::Escape($j)) {
                $Probe.verdict = 'FAIL_JUNK_DESC'
                $Probe.notes = "first result description looks like scrape junk: '$firstDescription'"
                return
            }
        }
    }

    $expectMin = $Probe.expect['minResults']
    if ($null -eq $expectMin) { $expectMin = 1 }
    $expectFirst = $Probe.expect['expectedFirstArticle']
    $expectAny   = $Probe.expect['expectedAnyArticle']
    $mustFail    = $Probe.expect['expectFail']

    if ($mustFail) {
        if ($results.Count -eq 0) { $Probe.verdict = 'PASS_ZERO' } else { $Probe.verdict = 'FAIL_LEAK' }
        return
    }

    if ($results.Count -lt $expectMin) {
        $Probe.verdict = 'FAIL_UNDERFILL'
        return
    }
    if ($expectFirst) {
        if ((Normalize-Article $Probe.firstArticle) -eq (Normalize-Article $expectFirst)) {
            $Probe.verdict = 'PASS_FIRST'
        } else {
            $Probe.verdict = 'FAIL_RANK'
        }
        return
    }
    if ($expectAny) {
        $normExpect = Normalize-Article $expectAny
        $hit = $results | Where-Object { (Normalize-Article "$($_.articleNumber)") -match [regex]::Escape($normExpect) }
        if ($hit) { $Probe.verdict = 'PASS_ANY' } else { $Probe.verdict = 'FAIL_MISSING' }
        return
    }
    $Probe.verdict = 'PASS_ANY'
}

function Score-VinProbe {
    param([pscustomobject]$Probe)
    $body = $Probe.rawResponse
    if (-not $body) { $Probe.verdict = 'ERROR'; return }

    $variants = 0
    if ($body.PSObject.Properties.Name -contains 'allVariants') {
        $variants = @($body.allVariants).Count
    } elseif ($body.PSObject.Properties.Name -contains 'variants') {
        $variants = @($body.variants).Count
    }
    $Probe.resultCount = $variants

    $decodedMake  = ''
    $decodedModel = ''
    $decodedYear  = ''
    if ($body.PSObject.Properties.Name -contains 'vehicle' -and $body.vehicle) {
        $decodedMake  = "$($body.vehicle.make)"
        $decodedModel = "$($body.vehicle.model)"
        $decodedYear  = "$($body.vehicle.modelYear)"
    }
    if (-not $decodedMake -and $body.PSObject.Properties.Name -contains 'nhtsaRaw' -and $body.nhtsaRaw) {
        $decodedMake  = "$($body.nhtsaRaw.make)"
        $decodedModel = "$($body.nhtsaRaw.model)"
        $decodedYear  = "$($body.nhtsaRaw.modelYear)"
    }
    $Probe.notes = "make=$decodedMake model=$decodedModel year=$decodedYear variants=$variants"

    $expectMake  = $Probe.expect['expectedMake']
    $expectModel = $Probe.expect['expectedModel']
    $expectYear  = $Probe.expect['expectedModelYear']

    $verdict = 'PASS'
    if ($expectMake -and $decodedMake -notmatch [regex]::Escape($expectMake)) { $verdict = 'FAIL_MAKE' }
    elseif ($expectModel -and $decodedModel -notmatch [regex]::Escape($expectModel)) { $verdict = 'FAIL_MODEL' }
    elseif ($expectYear -and $decodedYear -notmatch [regex]::Escape($expectYear)) { $verdict = 'FAIL_YEAR' }
    $Probe.verdict = $verdict
}

function Score-RecallProbe {
    param([pscustomobject]$Probe)
    $body = $Probe.rawResponse
    if (-not $body) { $Probe.verdict = 'ERROR'; return }
    $recalls = @()
    if ($body.PSObject.Properties.Name -contains 'recalls') {
        $recalls = @($body.recalls)
    }
    $Probe.resultCount = $recalls.Count
    $expectMin = $Probe.expect['minResults']
    if ($null -eq $expectMin) { $expectMin = 1 }
    if ($recalls.Count -lt $expectMin) { $Probe.verdict = 'FAIL_UNDERFILL' } else { $Probe.verdict = 'PASS' }
}

function Score-CatalogProbe {
    param([pscustomobject]$Probe)
    $body = $Probe.rawResponse
    if (-not $body) { $Probe.verdict = 'ERROR'; return }
    $count = 0
    foreach ($key in @('models','vehicles','groups','parts','makes')) {
        if ($body.PSObject.Properties.Name -contains $key -and $body.$key) {
            $count = @($body.$key).Count
            break
        }
    }
    $Probe.resultCount = $count
    if ($count -gt 0) { $Probe.verdict = 'PASS' } else { $Probe.verdict = 'FAIL_EMPTY' }
}

# --------------------------------------------------------------------
# Test matrix
# --------------------------------------------------------------------

$probes = @()

# VIN cases (golden + broader HK). Real endpoint is POST /api/vin/decode.
$probes += New-Probe 'vin-01' 'vin' 'Hyundai Tucson 2016 (golden)' `
    "$BaseUrl/api/vin/decode" `
    @{ expectedMake='HYUNDAI'; expectedModel='TUCSON'; expectedModelYear='2016'; vinBody='KM8J33A46GU123456' }
$probes += New-Probe 'vin-02' 'vin' 'Kia Sportage 2018 (golden)' `
    "$BaseUrl/api/vin/decode" `
    @{ expectedMake='KIA'; expectedModel='SPORTAGE'; expectedModelYear='2018'; vinBody='KNDPMCAC4J7412345' }
$probes += New-Probe 'vin-03' 'vin' 'Hyundai Elantra 2017' `
    "$BaseUrl/api/vin/decode" `
    @{ expectedMake='HYUNDAI'; vinBody='5NPE24AF6HH123456' }
$probes += New-Probe 'vin-04' 'vin' 'Kia K5 2021' `
    "$BaseUrl/api/vin/decode" `
    @{ expectedMake='KIA'; vinBody='5XXG14J20MG123456' }
$probes += New-Probe 'vin-05' 'vin' 'Hyundai Palisade 2022' `
    "$BaseUrl/api/vin/decode" `
    @{ expectedMake='HYUNDAI'; vinBody='KM8R7DHE4NU123456' }
$probes += New-Probe 'vin-99' 'vin' 'BUG: GET /api/vin/:vin falls through to SPA' `
    "$BaseUrl/api/vin/KM8J33A46GU123456" `
    @{ apiShouldNotBeHtml=$true }

# OEM search — golden hits
$probes += New-Probe 'oem-golden-01' 'oem' 'Golden: 26300-35505 oil filter' `
    "$BaseUrl/api/search?q=26300-35505" `
    @{ minResults=1; expectedFirstArticle='26300-35505' }
$probes += New-Probe 'oem-golden-02' 'oem' 'Golden: 97133-D3000 cabin filter' `
    "$BaseUrl/api/search?q=97133-D3000" `
    @{ minResults=1; expectedAnyArticle='97133-D3000' }
$probes += New-Probe 'oem-golden-03' 'oem' 'Golden article-number prefix 97133' `
    "$BaseUrl/api/search?q=97133" `
    @{ minResults=1; expectedAnyArticle='97133-D3000' }
$probes += New-Probe 'oem-golden-04' 'oem' 'Golden category text: cabin air filter' `
    "$BaseUrl/api/search?q=cabin%20air%20filter" `
    @{ minResults=1; expectedAnyArticle='97133-D3000' }

# OEM search — real HK parts that a real user would type. These probe whether
# the tiny 127-article dataset can survive contact with reality.
$realHkOems = @(
    @{ oem='46321-3B650'; label='Hyundai auto-trans mount' },
    @{ oem='54528-4A100'; label='Kia lower ball joint' },
    @{ oem='55700-3S000'; label='Hyundai Sonata rear axle beam' },
    @{ oem='92101-3S050'; label='Hyundai Sonata headlight' },
    @{ oem='25100-25000'; label='Hyundai water pump' },
    @{ oem='58101-3SA00'; label='Hyundai Sonata front brake pad' },
    @{ oem='51712-2S000'; label='Hyundai Tucson strut' },
    @{ oem='55311-2S000'; label='Hyundai Tucson rear coil spring' }
)
foreach ($p in $realHkOems) {
    $probes += New-Probe "oem-real-$($p.oem)" 'oem' "Real HK OEM: $($p.label) [$($p.oem)]" `
        "$BaseUrl/api/search?q=$($p.oem)" `
        @{ minResults=1; expectedAnyArticle=$p.oem }
}

# Text search — common consumer terms
$textQueries = @(
    'oil filter','brake pad','brake disc','air filter','spark plug',
    'wiper blade','headlight','tail light','shock absorber','ball joint'
)
foreach ($t in $textQueries) {
    $probes += New-Probe "text-$([Uri]::EscapeDataString($t))" 'text' "Text: '$t'" `
        "$BaseUrl/api/search?q=$([Uri]::EscapeDataString($t))" `
        @{ minResults=1 }
}

# Vehicle-scoped queries (linkageTargetId = 10001 = 2016 Tucson per golden)
$probes += New-Probe 'veh-01' 'vehicle' 'Tucson 2016: cabin air filter' `
    "$BaseUrl/api/search?q=cabin%20air%20filter&vehicleId=10001" `
    @{ minResults=1; expectedAnyArticle='97133' }
$probes += New-Probe 'veh-02' 'vehicle' 'Tucson 2016: oil filter' `
    "$BaseUrl/api/search?q=oil%20filter&vehicleId=10001" `
    @{ minResults=1 }
$probes += New-Probe 'veh-03' 'vehicle' 'Tucson 2016: brake pad' `
    "$BaseUrl/api/search?q=brake%20pad&vehicleId=10001" `
    @{ minResults=1 }
$probes += New-Probe 'veh-04' 'vehicle' 'Tucson 2016: wiper' `
    "$BaseUrl/api/search?q=wiper&vehicleId=10001" `
    @{ minResults=1 }

# Recalls
$probes += New-Probe 'rec-01' 'recall' 'HYUNDAI TUCSON 2016 recalls' `
    "$BaseUrl/api/recalls?make=HYUNDAI&model=TUCSON&year=2016" @{ minResults=1 }
$probes += New-Probe 'rec-02' 'recall' 'KIA SPORTAGE 2018 recalls' `
    "$BaseUrl/api/recalls?make=KIA&model=SPORTAGE&year=2018" @{ minResults=1 }
$probes += New-Probe 'rec-03' 'recall' 'HYUNDAI SANTA FE 2019 recalls' `
    "$BaseUrl/api/recalls?make=HYUNDAI&model=SANTA%20FE&year=2019" @{ minResults=1 }
$probes += New-Probe 'rec-04' 'recall' 'KIA SORENTO 2021 recalls' `
    "$BaseUrl/api/recalls?make=KIA&model=SORENTO&year=2021" @{ minResults=1 }

# Catalog browse
$probes += New-Probe 'cat-01' 'catalog' 'Models list' "$BaseUrl/api/catalog/models" @{}
$probes += New-Probe 'cat-02' 'catalog' 'Vehicles for HYUNDAI TUCSON 2016' `
    "$BaseUrl/api/catalog/vehicles?make=HYUNDAI&model=TUCSON&year=2016" @{}
$probes += New-Probe 'cat-03' 'catalog' 'Groups for vehicle 10001' `
    "$BaseUrl/api/catalog/groups?vehicleId=10001" @{}

# Non-HK boundary probe — should return NOTHING (app is HK-scoped)
$probes += New-Probe 'boundary-01' 'boundary' 'Toyota 90915-YZZE1 (should be zero/warning)' `
    "$BaseUrl/api/search?q=90915-YZZE1" `
    @{ expectFail=$true }

# Detail
$probes += New-Probe 'det-01' 'detail' 'Detail: legacyArticleId 100001 (oil filter)' `
    "$BaseUrl/api/part/100001/detail?vehicleId=10001" @{}
$probes += New-Probe 'det-02' 'detail' 'Detail: legacyArticleId 100307 (cabin filter golden)' `
    "$BaseUrl/api/part/100307/detail?vehicleId=10001" @{}

# --------------------------------------------------------------------
# Run
# --------------------------------------------------------------------

Write-Host "Firing $($probes.Count) probes at $BaseUrl ..."
$i = 0
foreach ($p in $probes) {
    $i++
    Write-Host ("  [{0,2}/{1}] {2,-15} {3}" -f $i, $probes.Count, $p.category, $p.id)
    $method = 'GET'
    $body   = ''
    if ($p.category -eq 'vin' -and $p.expect.ContainsKey('vinBody')) {
        $method = 'POST'
        $body   = @{ vin = $p.expect['vinBody'] } | ConvertTo-Json -Compress
    }
    Invoke-Probe -Probe $p -Method $method -Body $body | Out-Null
    switch ($p.category) {
        'vin' {
            if ($p.expect['apiShouldNotBeHtml']) {
                # This probe is asserting the routing bug we just found
                if ($p.rawResponse -eq 'SPA_FALLBACK_HTML') {
                    $p.verdict = 'FAIL_ROUTING'
                } else {
                    $p.verdict = 'PASS'
                }
            } else {
                Score-VinProbe $p
            }
        }
        'oem'     { Score-SearchProbe $p }
        'text'    { Score-SearchProbe $p }
        'vehicle' { Score-SearchProbe $p }
        'boundary'{ Score-SearchProbe $p }
        'recall'  { Score-RecallProbe $p }
        'catalog' { Score-CatalogProbe $p }
        'detail'  { $p.verdict = if ($p.ok) { 'PASS_LOADED' } else { 'ERROR' } }
    }
}

# --------------------------------------------------------------------
# Persist
# --------------------------------------------------------------------

$outDir = Split-Path -Parent $OutJson
if (-not (Test-Path -LiteralPath $outDir)) { New-Item -ItemType Directory -Path $outDir | Out-Null }

$summary = @{
    baseUrl     = $BaseUrl
    at          = (Get-Date).ToString('o')
    totalProbes = $probes.Count
    passCount   = ($probes | Where-Object { $_.verdict -match '^PASS' }).Count
    failCount   = ($probes | Where-Object { $_.verdict -match '^FAIL' }).Count
    errorCount  = ($probes | Where-Object { $_.verdict -eq 'ERROR' }).Count
    byCategory  = ($probes | Group-Object category | ForEach-Object {
        @{ category=$_.Name; total=$_.Count;
           pass=($_.Group | Where-Object { $_.verdict -match '^PASS' }).Count;
           fail=($_.Group | Where-Object { $_.verdict -match '^FAIL' }).Count } })
}

$out = @{
    summary = $summary
    probes  = $probes | Select-Object id, category, description, url, expect, ok,
                                       httpStatus, elapsedMs, resultCount,
                                       firstArticle, verdict, warnings, error,
                                       notes, rawResponse
}
$out | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $OutJson -Encoding UTF8

# Markdown summary
$md = New-Object System.Text.StringBuilder
$null = $md.AppendLine("# Harsh HK probe results")
$null = $md.AppendLine('')
$null = $md.AppendLine("Base URL: `$BaseUrl`  ")
$null = $md.AppendLine("At: $((Get-Date).ToString('u'))  ")
$null = $md.AppendLine("Total: $($summary.totalProbes) | Pass: $($summary.passCount) | Fail: $($summary.failCount) | Error: $($summary.errorCount)")
$null = $md.AppendLine('')
$null = $md.AppendLine('| id | category | verdict | http | results | first | ms | description |')
$null = $md.AppendLine('| --- | --- | --- | ---: | ---: | --- | ---: | --- |')
foreach ($p in $probes) {
    $desc = $p.description -replace '\|', '\|'
    $null = $md.AppendLine("| $($p.id) | $($p.category) | $($p.verdict) | $($p.httpStatus) | $($p.resultCount) | $($p.firstArticle) | $($p.elapsedMs) | $desc |")
}
Set-Content -LiteralPath $OutMd -Value $md.ToString() -Encoding UTF8

Write-Host ""
Write-Host "== Summary =="
Write-Host "  Total  : $($summary.totalProbes)"
Write-Host "  Pass   : $($summary.passCount)"
Write-Host "  Fail   : $($summary.failCount)"
Write-Host "  Error  : $($summary.errorCount)"
Write-Host ""
Write-Host "  JSON   : $OutJson"
Write-Host "  MD     : $OutMd"
