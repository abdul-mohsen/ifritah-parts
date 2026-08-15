# Download the NHTSA vPIC SQLite database (pre-built by Corgi project)
$dbDir = "c:\ssda\chatGPT\parts-engine\data"
$gzFile = "$dbDir\vpic.lite.db.gz"
$dbFile = "$dbDir\vpic.lite.db"

if (Test-Path $dbFile) {
    Write-Host "Database already exists at $dbFile"
    $fi = Get-Item $dbFile
    Write-Host "Size: $([math]::Round($fi.Length / 1MB, 1)) MB"
    exit 0
}

New-Item -ItemType Directory -Force -Path $dbDir | Out-Null

Write-Host "Downloading NHTSA vPIC SQLite database..."
$url = "https://corgi.cardog.io/vpic.lite.db.gz"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$wc = New-Object System.Net.WebClient
$wc.DownloadFile($url, $gzFile)
$fi = Get-Item $gzFile
Write-Host "Downloaded: $([math]::Round($fi.Length / 1MB, 1)) MB"

Write-Host "Decompressing..."
Add-Type -AssemblyName System.IO.Compression
$inStream = [System.IO.File]::OpenRead($gzFile)
$outStream = [System.IO.File]::Create($dbFile)
$gzip = New-Object System.IO.Compression.GZipStream($inStream, [System.IO.Compression.CompressionMode]::Decompress)
$gzip.CopyTo($outStream)
$gzip.Dispose()
$outStream.Dispose()
$inStream.Dispose()

Remove-Item $gzFile
$fi = Get-Item $dbFile
Write-Host "Done! Database: $([math]::Round($fi.Length / 1MB, 1)) MB at $dbFile"
