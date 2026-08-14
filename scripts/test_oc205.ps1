try {
    $r = Invoke-WebRequest -Uri 'http://localhost:8080/api/search?q=OC+205&limit=5' -UseBasicParsing -TimeoutSec 10
    Write-Host ('Status: ' + $r.StatusCode)
    Write-Host $r.Content
} catch {
    Write-Host ('ERROR: ' + $_.Exception.Message)
}
