# Use NHTSA WMI decode endpoint (different from VIN decode)
$wmis = @("MHF","MR0","MHR","MNB","MHS","MHK","MBJ","MMA","MMB","MMC","MMT","MBC","TSM","MP1","MAJ","MAL","MHC","MHD","MHE","MHG","MHH","MHJ","MHL","MHM","MHN","MHP")
foreach ($wmi in $wmis) {
    try {
        $r = Invoke-RestMethod -Uri "https://vpic.nhtsa.dot.gov/api/vehicles/decodewmi/$wmi`?format=json" -TimeoutSec 10
        $res = $r.Results
        if ($res.ManufacturerName -or $res.CommonName) {
            Write-Host "$wmi => Mfr=$($res.ManufacturerName) Common=$($res.CommonName) Country=$($res.Country)" -ForegroundColor Green
        } else {
            Write-Host "$wmi => (not in NHTSA WMI DB)" -ForegroundColor DarkGray
        }
    } catch {
        Write-Host "$wmi => ERROR: $_" -ForegroundColor Red
    }
    Start-Sleep -Milliseconds 200
}
