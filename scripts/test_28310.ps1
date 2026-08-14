try {
    $r = Invoke-WebRequest -Uri 'http://localhost:8080/api/search?q=2831004950&limit=10' -UseBasicParsing -TimeoutSec 10
    Write-Host ('Status: ' + $r.StatusCode)
    Write-Host $r.Content
} catch {
    Write-Host ('ERROR: ' + $_.Exception.Message)
}
