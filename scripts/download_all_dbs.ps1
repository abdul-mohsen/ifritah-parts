# Download all public vehicle databases
$dataDir = "c:\ssda\chatGPT\parts-engine\data"
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$wc = New-Object System.Net.WebClient

# 1. EPA FuelEconomy.gov CSV (40K+ vehicles, 1984-2026)
$epaFile = "$dataDir\vehicles.csv"
if (-not (Test-Path $epaFile)) {
    Write-Host "Downloading EPA FuelEconomy.gov vehicles CSV..."
    $wc.DownloadFile("https://www.fueleconomy.gov/feg/epadata/vehicles.csv.zip", "$dataDir\vehicles.csv.zip")
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory("$dataDir\vehicles.csv.zip", $dataDir)
    Remove-Item "$dataDir\vehicles.csv.zip"
    $fi = Get-Item $epaFile
    Write-Host "EPA CSV: $([math]::Round($fi.Length / 1MB, 1)) MB"
} else {
    Write-Host "EPA CSV already exists"
}

# 2. open-vehicle-db (9,730 styles, JSON)
$ovdbDir = "$dataDir\open-vehicle-db"
if (-not (Test-Path $ovdbDir)) {
    Write-Host "Downloading open-vehicle-db data..."
    New-Item -ItemType Directory -Force -Path $ovdbDir | Out-Null
    $wc.DownloadFile("https://raw.githubusercontent.com/plowman/open-vehicle-db/master/data/makes.json", "$ovdbDir\makes.json")
    $wc.DownloadFile("https://raw.githubusercontent.com/plowman/open-vehicle-db/master/data/models.json", "$ovdbDir\models.json")
    $wc.DownloadFile("https://raw.githubusercontent.com/plowman/open-vehicle-db/master/data/styles.json", "$ovdbDir\styles.json")
    Write-Host "open-vehicle-db downloaded"
} else {
    Write-Host "open-vehicle-db already exists"
}

# 3. arthurkao vehicle-make-model-data (19,722 models, JSON)
$arthurFile = "$dataDir\vehicle_make_model.json"
if (-not (Test-Path $arthurFile)) {
    Write-Host "Downloading arthurkao vehicle-make-model-data..."
    $wc.DownloadFile("https://raw.githubusercontent.com/arthurkao/vehicle-make-model-data/master/json_data.json", $arthurFile)
    $fi = Get-Item $arthurFile
    Write-Host "arthurkao JSON: $([math]::Round($fi.Length / 1MB, 1)) MB"
} else {
    Write-Host "arthurkao JSON already exists"
}

Write-Host "All downloads complete!"
