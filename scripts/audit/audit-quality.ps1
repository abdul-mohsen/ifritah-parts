# audit-quality.ps1 - Search quality audit runner
#
# Reads an input corpus CSV, hits qa.ifritah.com per query with enrichmentLevel=full
# so we capture aftermarket alternatives, OEM cross-refs, specs, vehicles, and
# supersession alongside the primary result. Emits ONE dated raw CSV.
#
# Analyze with analyze-quality.ps1 to get per-category / per-slice F1 tables.
#
# Two corpus shapes are supported:
#
#   OEM corpus (default):
#     Columns required: OEM, Slice, GroundTruth, ExpectedCategory, GoodTokens
#     Optional: ExpectedSystem, ExpectedMake, Chassis, Prefix5
#     Run with defaults (no -QueryColumn override).
#
#   Keyword corpus (M0.T5, for keyword_gated strategy):
#     Columns required: Query, ExpectedCategory, GoodTokens
#     Run with: -InputCorpus scripts/audit/corpus-keywords-v1.csv -QueryColumn Query -Modes keyword_gated
#
# Parameters:
#   -InputCorpus     Path to the corpus CSV.
#   -OutputDir       Where the dated raw CSV + log are written.
#   -Endpoint        Base URL (default https://qa.ifritah.com).
#   -Mode            Single mode. Ignored when -Modes is set.
#   -Modes           One or more strategy modes to run (comma-joined string or array).
#                    Example: -Modes keyword_gated  or  -Modes "cache,legacy,combined".
#                    Every corpus row is fanned out across every mode.
#   -EnrichmentLevel enrichmentLevel query param passed to /api/search (default "full").
#   -ThrottleLimit   Max parallel workers.
#   -MaxTimeoutS     Per-request curl timeout.
#   -InterRequestMs  Post-request sleep per worker (jitter for the API).
#   -MaxRetries      429 retry ceiling.
#   -QueryColumn     Which corpus column provides the `q=` value (default "OEM").
#                    Set to "Query" when running against a keyword corpus.
#   -DryRun          Resolve and print URLs then exit; no API calls, no output CSV.
#                    Use this to sanity-check invocations quickly.

param(
  [string]$InputCorpus    = "C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\corpus-1500-v2.csv",
  [string]$OutputDir      = "C:\Users\ALMAAB~1\AppData\Local\Temp\opencode",
  [string]$Endpoint       = "https://qa.ifritah.com",
  [string]$Mode           = "combined",
  [string[]]$Modes        = @(),
  [string]$EnrichmentLevel= "full",
  [int]$ThrottleLimit     = 4,
  [int]$MaxTimeoutS       = 25,
  [int]$InterRequestMs    = 750,
  [int]$MaxRetries        = 5,
  [string]$QueryColumn    = "OEM",
  [switch]$DryRun
)

Add-Type -AssemblyName System.Web

if (-not (Test-Path $InputCorpus)) {
  Write-Error "Corpus not found: $InputCorpus"
  exit 1
}

# If -Modes is empty, fall back to the single -Mode value. Multi-mode means
# every OEM x every mode = big cartesian; useful for per-strategy F1 matrix.
# Accept either a proper array (-Modes a,b,c) OR a comma-joined single
# string (-Modes "a,b,c") — Start-Process -ArgumentList sometimes passes
# it as one string; split defensively.
$modesToRun = if ($Modes.Count -gt 0) {
  $flat = @()
  foreach ($m in $Modes) {
    if ($m -match ",") {
      $flat += ($m -split "," | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    } else {
      $flat += $m.Trim()
    }
  }
  $flat
} else {
  @($Mode)
}

$dateStamp = Get-Date -Format "yyyy-MM-dd_HHmm"
$outputFile = Join-Path $OutputDir "qa-quality-raw-$dateStamp.csv"
$logFile    = Join-Path $OutputDir "qa-quality-raw-$dateStamp.log"

# UTF-8 so keyword corpora with Arabic transliterations round-trip cleanly.
$corpus = Import-Csv $InputCorpus -Encoding utf8
$totalOems = $corpus.Count
$totalReq  = $totalOems * $modesToRun.Count

# Small helper: pull the query value from whichever column -QueryColumn names.
# Returns "" when the column is missing so callers can skip cleanly.
function Get-QueryValue($row, [string]$col) {
  if ($row.PSObject.Properties.Match($col).Count -gt 0) {
    return [string]$row.PSObject.Properties[$col].Value
  }
  return ""
}

Write-Host "Corpus:      $InputCorpus ($totalOems rows)"
Write-Host "QueryColumn: $QueryColumn"
Write-Host "Endpoint:    $Endpoint"
Write-Host "Modes:       $($modesToRun -join ', ') ($($modesToRun.Count) modes -> $totalReq requests)"
Write-Host "Enrichment:  $EnrichmentLevel"
Write-Host "Workers:     $ThrottleLimit (throttle+delay tuned for full enrichment)"
Write-Host "Timeout:     ${MaxTimeoutS}s"
Write-Host "Output:      $outputFile"
Write-Host ""

# Fail loud if the requested QueryColumn isn't present on the corpus. Better
# to bail here than emit 200 rows of blank q= URLs.
$firstRow = $corpus | Select-Object -First 1
if ($firstRow -and $firstRow.PSObject.Properties.Match($QueryColumn).Count -eq 0) {
  $cols = ($firstRow.PSObject.Properties | ForEach-Object { $_.Name }) -join ", "
  Write-Error "Column '$QueryColumn' not found in corpus. Available columns: $cols"
  exit 1
}

# -DryRun: resolve every (row x mode) into a URL and print. No API calls, no
# worker files, no output CSV. Meant to sanity-check invocations quickly.
if ($DryRun) {
  Write-Host "DRY RUN - resolving URLs (no API calls will be made)"
  Write-Host ""
  $shown = 0
  foreach ($row in $corpus) {
    $queryVal = Get-QueryValue $row $QueryColumn
    if (-not $queryVal) {
      Write-Host "SKIP row (empty '$QueryColumn'): $row"
      continue
    }
    $encoded = [System.Web.HttpUtility]::UrlEncode($queryVal)
    foreach ($m in $modesToRun) {
      Write-Host "  $Endpoint/api/search?q=$encoded&mode=$m&enrichmentLevel=$EnrichmentLevel"
      $shown++
    }
  }
  Write-Host ""
  Write-Host "DRY RUN complete: $shown URL(s) resolved across $($corpus.Count) row(s) x $($modesToRun.Count) mode(s)."
  exit 0
}


$workerDir = Join-Path $env:TEMP "qa-quality-workers-$(Get-Random)"
New-Item -ItemType Directory -Path $workerDir -Force | Out-Null

$counter = [System.Collections.Concurrent.ConcurrentDictionary[string,int]]::new()
foreach ($k in @("done","hits","timeouts","429","parse_err","enriched_am","enriched_specs","enriched_veh")) {
  [void]$counter.TryAdd($k, 0)
}

$sw_total = [System.Diagnostics.Stopwatch]::StartNew()

# Fan out corpus x modes into one flat request stream so per-worker parallelism
# is uniform across modes (avoids one mode monopolising the workers).
$requests = foreach ($row in $corpus) {
  foreach ($m in $modesToRun) {
    [PSCustomObject]@{
      Row  = $row
      Mode = $m
    }
  }
}

$requests | ForEach-Object -Parallel {
  $reqWrap   = $_
  $row       = $reqWrap.Row
  $mode      = $reqWrap.Mode
  $endpoint  = $using:Endpoint
  $enrichLvl = $using:EnrichmentLevel
  $timeoutS  = $using:MaxTimeoutS
  $delayMs   = $using:InterRequestMs
  $maxRetry  = $using:MaxRetries
  $counter   = $using:counter
  $totalReq  = $using:totalReq
  $workerDir = $using:workerDir
  $queryCol  = $using:QueryColumn

  Add-Type -AssemblyName System.Web -EA SilentlyContinue
  $threadId = [System.Threading.Thread]::CurrentThread.ManagedThreadId
  $workerFile = Join-Path $workerDir "worker-$threadId.csv"

  # Pull the query value from whichever column -QueryColumn names. The corpus
  # was validated up front so the property exists; still guard for empty cells.
  $queryVal = if ($row.PSObject.Properties.Match($queryCol).Count -gt 0) {
    [string]$row.PSObject.Properties[$queryCol].Value
  } else { "" }
  $encoded = [System.Web.HttpUtility]::UrlEncode($queryVal)
  $url = "$endpoint/api/search?q=$encoded&mode=$mode&enrichmentLevel=$enrichLvl"

  $status = 0
  $body = ""
  $retry = 0
  $sw = [System.Diagnostics.Stopwatch]::StartNew()

  while ($retry -le $maxRetry) {
    try {
      $lines = @(curl.exe -sk --max-time $timeoutS -w "`n%{http_code}" $url 2>&1)
      $status = try { [int]$lines[-1] } catch { 0 }
      $body = if ($lines.Count -gt 1) { ($lines[0..($lines.Count - 2)] -join "`n") } else { "" }
    } catch {
      $status = -1; $body = ""
    }
    if ($status -eq 429) {
      [void]$counter.AddOrUpdate("429", 1, { param($k,$v) $v + 1 })
      Start-Sleep -Seconds ([Math]::Pow(2, $retry))
      $retry++
      continue
    }
    break
  }
  $sw.Stop()
  $elapsed = [Math]::Round($sw.Elapsed.TotalSeconds, 2)
  $isTimeout = ($elapsed -ge ($timeoutS - 1) -and $status -ne 429)

  # Parse response
  $obj = $null
  if ($body) {
    try { $obj = $body | ConvertFrom-Json -EA Stop } catch {
      [void]$counter.AddOrUpdate("parse_err", 1, { param($k,$v) $v + 1 })
    }
  }

  $total      = 0
  $srchStrat  = ""
  $srcStrat   = ""
  $conf       = 0.0
  $firstDesc  = ""
  $firstBrand = ""
  $firstCat   = ""
  $amCount    = 0
  $amSample   = ""
  $specsCount = 0
  $vehCount   = 0
  $docsCount  = 0
  $hasSuper   = "false"
  $oemNumCount= 0
  $oemNumSamp = ""
  $warnCount  = 0
  $warnJoined = ""

  if ($obj) {
    if ($obj.total) { $total = [int]$obj.total }
    if ($obj.searchStrategy) { $srchStrat = [string]$obj.searchStrategy }
    if ($obj.warnings) {
      $warnCount = $obj.warnings.Count
      $warnJoined = ($obj.warnings | ForEach-Object { $_ }) -join " | "
    }
    if ($obj.results -and $obj.results.Count -gt 0) {
      $r = $obj.results[0]
      if ($r.sourceStrategy)    { $srcStrat = [string]$r.sourceStrategy }
      if ($r.confidence)        { $conf = [double]$r.confidence }
      if ($r.description)       { $firstDesc = ($r.description -replace '"', "'" -replace '[\r\n]',' ') }
      if ($r.brand -or $r.brandName) { $firstBrand = if ($r.brand) { [string]$r.brand } else { [string]$r.brandName } }
      if ($r.category)          { $firstCat = [string]$r.category }

      if ($r.aftermarketAlternatives) {
        $amCount = $r.aftermarketAlternatives.Count
        if ($amCount -gt 0) {
          $amSample = ($r.aftermarketAlternatives | Select-Object -First 3 | ForEach-Object {
            "$($_.brand):$($_.partNumber)"
          }) -join ";"
          [void]$counter.AddOrUpdate("enriched_am", 1, { param($k,$v) $v + 1 })
        }
      }
      if ($r.specifications) {
        $specsCount = $r.specifications.Count
        if ($specsCount -gt 0) { [void]$counter.AddOrUpdate("enriched_specs", 1, { param($k,$v) $v + 1 }) }
      }
      if ($r.compatibleVehicles) {
        $vehCount = $r.compatibleVehicles.Count
        if ($vehCount -gt 0) { [void]$counter.AddOrUpdate("enriched_veh", 1, { param($k,$v) $v + 1 }) }
      }
      if ($r.documents) { $docsCount = $r.documents.Count }
      if ($r.supersession) { $hasSuper = "true" }
      if ($r.oemNumbers) {
        $oemNumCount = $r.oemNumbers.Count
        if ($oemNumCount -gt 0) {
          $oemNumSamp = ($r.oemNumbers | Select-Object -First 3 | ForEach-Object {
            "$($_.brand):$($_.number)"
          }) -join ";"
        }
      }
    }
  }

  # Escape CSV
  function CsvEsc($v) {
    $s = [string]$v
    if ($s -match ',|"|`n|`r') { '"' + ($s -replace '"','""') + '"' } else { $s }
  }

  # Corpus fields (defensive - may be missing)
  $expSys   = if ($row.PSObject.Properties.Match('ExpectedSystem').Count -gt 0)   { $row.ExpectedSystem }   else { "" }
  $expMake  = if ($row.PSObject.Properties.Match('ExpectedMake').Count -gt 0)     { $row.ExpectedMake }     else { "" }
  $chassis  = if ($row.PSObject.Properties.Match('Chassis').Count -gt 0)          { $row.Chassis }          else { "" }
  $prefix5  = if ($row.PSObject.Properties.Match('Prefix5').Count -gt 0)          { $row.Prefix5 }          else { "" }

  $fields = @(
    (CsvEsc $queryVal),
    (CsvEsc $row.Slice),
    (CsvEsc $row.GroundTruth),
    (CsvEsc $expSys),
    (CsvEsc $row.ExpectedCategory),
    (CsvEsc $row.GoodTokens),
    (CsvEsc $expMake),
    (CsvEsc $chassis),
    (CsvEsc $prefix5),
    $mode,
    $enrichLvl,
    $status,
    $elapsed,
    $isTimeout,
    $total,
    (CsvEsc $srchStrat),
    (CsvEsc $srcStrat),
    $conf,
    (CsvEsc $firstDesc),
    (CsvEsc $firstBrand),
    (CsvEsc $firstCat),
    $amCount,
    (CsvEsc $amSample),
    $specsCount,
    $vehCount,
    $docsCount,
    $hasSuper,
    $oemNumCount,
    (CsvEsc $oemNumSamp),
    $warnCount,
    (CsvEsc $warnJoined)
  )
  [System.IO.File]::AppendAllText($workerFile, ($fields -join ",") + "`n")

  [void]$counter.AddOrUpdate("done", 1, { param($k,$v) $v + 1 })
  if ($total -gt 0) { [void]$counter.AddOrUpdate("hits", 1, { param($k,$v) $v + 1 }) }
  if ($isTimeout)   { [void]$counter.AddOrUpdate("timeouts", 1, { param($k,$v) $v + 1 }) }

  $done = $counter["done"]
  if ($done % 100 -eq 0) {
    $pct = [Math]::Round(100.0 * $done / $totalReq, 1)
    Write-Host "[$done/$totalReq $pct%] hits=$($counter['hits']) TO=$($counter['timeouts']) 429=$($counter['429']) AM=$($counter['enriched_am']) specs=$($counter['enriched_specs'])"
  }
  Start-Sleep -Milliseconds $delayMs
} -ThrottleLimit $ThrottleLimit

$sw_total.Stop()

# Merge worker files
$header = "OEM,Slice,GroundTruth,ExpectedSystem,ExpectedCategory,GoodTokens,ExpectedMake,Chassis,Prefix5,Mode,EnrichmentLevel,HttpStatus,ElapsedS,IsTimeout,Total,SearchStrategy,SourceStrategy,Confidence,FirstDesc,FirstBrand,FirstCategory,AftermarketCount,AftermarketSample,SpecsCount,VehiclesCount,DocsCount,HasSupersession,OEMNumbersCount,OEMNumbersSample,WarningsCount,Warnings"
$header | Out-File $outputFile -Encoding utf8
foreach ($f in (Get-ChildItem "$workerDir\worker-*.csv")) {
  Get-Content $f.FullName | Add-Content $outputFile -Encoding utf8
}
Remove-Item -Recurse -Force $workerDir

$merged = (Get-Content $outputFile | Measure-Object -Line).Lines
Write-Host ""
Write-Host "DONE in $([Math]::Round($sw_total.Elapsed.TotalMinutes,1)) min"
Write-Host "Rows written: $merged (incl. header)"
Write-Host "Hits: $($counter['hits'])  Timeouts: $($counter['timeouts'])  429s: $($counter['429'])  ParseErr: $($counter['parse_err'])"
Write-Host "Enrichment coverage: AM=$($counter['enriched_am']) Specs=$($counter['enriched_specs']) Vehicles=$($counter['enriched_veh'])"
Write-Host ""
Write-Host "Output CSV: $outputFile"
Write-Host "Next step:  pwsh -File analyze-quality.ps1 -InputCSV `"$outputFile`""
