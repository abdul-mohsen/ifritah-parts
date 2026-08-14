# Fix open-vehicle-db download - try main branch
$dataDir = "c:\ssda\chatGPT\parts-engine\data"
$ovdbDir = "$dataDir\open-vehicle-db"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$wc = New-Object System.Net.WebClient

# Try main branch
$baseUrl = "https://raw.githubusercontent.com/plowman/open-vehicle-db/main"
try {
    $wc.DownloadFile("$baseUrl/data/makes.json", "$ovdbDir\makes.json")
    $wc.DownloadFile("$baseUrl/data/models.json", "$ovdbDir\models.json")
    $wc.DownloadFile("$baseUrl/data/styles.json", "$ovdbDir\styles.json")
    Write-Host "Downloaded from main branch"
} catch {
    Write-Host "Main branch failed too, trying ZIP archive..."
    try {
        $wc.DownloadFile("https://github.com/plowman/open-vehicle-db/archive/refs/heads/main.zip", "$dataDir\ovdb.zip")
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [System.IO.Compression.ZipFile]::ExtractToDirectory("$dataDir\ovdb.zip", "$dataDir\ovdb_temp")
        $extracted = Get-ChildItem "$dataDir\ovdb_temp" -Directory | Select-Object -First 1
        Copy-Item "$($extracted.FullName)\data\*" "$ovdbDir\" -Recurse -Force
        Remove-Item "$dataDir\ovdb.zip"
        Remove-Item "$dataDir\ovdb_temp" -Recurse
        Write-Host "Downloaded via ZIP archive"
    } catch {
        Write-Host "ZIP also failed: $_"
        # Try master ZIP
        try {
            $wc.DownloadFile("https://github.com/plowman/open-vehicle-db/archive/refs/heads/master.zip", "$dataDir\ovdb.zip")
            Add-Type -AssemblyName System.IO.Compression.FileSystem
            [System.IO.Compression.ZipFile]::ExtractToDirectory("$dataDir\ovdb.zip", "$dataDir\ovdb_temp")
            $extracted = Get-ChildItem "$dataDir\ovdb_temp" -Directory | Select-Object -First 1
            Copy-Item "$($extracted.FullName)\data\*" "$ovdbDir\" -Recurse -Force
            Remove-Item "$dataDir\ovdb.zip"
            Remove-Item "$dataDir\ovdb_temp" -Recurse
            Write-Host "Downloaded via master ZIP"
        } catch {
            Write-Host "All download methods failed: $_"
        }
    }
}

# Check what we got
if (Test-Path $ovdbDir) {
    Get-ChildItem $ovdbDir | ForEach-Object { Write-Host "$($_.Name) - $([math]::Round($_.Length / 1KB, 1)) KB" }
}
