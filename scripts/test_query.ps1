$queries = @(
    @{ Name = "529331P000 (no dash)"; Uri = "http://localhost:8080/api/search?q=529331P000&limit=50" }
    @{ Name = "52933-1P000 (with dash)"; Uri = "http://localhost:8080/api/search?q=52933-1P000&limit=50" }
    @{ Name = "TPMS text search"; Uri = "http://localhost:8080/api/search?q=TPMS&limit=10" }
    @{ Name = "OEM lookup 52933-1P000"; Uri = "http://localhost:8080/api/oem/52933-1P000" }
)

foreach ($q in $queries) {
    try {
        $r = Invoke-WebRequest -Uri $q.Uri -UseBasicParsing -TimeoutSec 5
        $json = $r.Content | ConvertFrom-Json
        $total = $json.total
        $strategy = $json.searchStrategy
        Write-Host "PASS $($q.Name) — $total results (strategy: $strategy)" -ForegroundColor Green
        Write-Host "  -> $($r.Content.Substring(0, [Math]::Min(250, $r.Content.Length)))"
    } catch {
        Write-Host ('FAIL ' + $q.Name + ': ' + $_) -ForegroundColor Red
    }
    Write-Host ""
}
