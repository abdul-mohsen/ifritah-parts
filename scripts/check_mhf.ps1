$body = "DATA=MHFBT9F34G6066150;MHFBT9F34G6006150;KMHBT9F34G6066150&format=json"
$r = Invoke-RestMethod -Uri "https://vpic.nhtsa.dot.gov/api/vehicles/DecodeVINValuesBatch/" -Method POST -Body $body -ContentType "application/x-www-form-urlencoded" -TimeoutSec 30
foreach ($v in $r.Results) {
    Write-Host "$($v.VIN) => Make=$($v.Make) Model=$($v.Model) Year=$($v.ModelYear) Mfr=$($v.Manufacturer)"
}
