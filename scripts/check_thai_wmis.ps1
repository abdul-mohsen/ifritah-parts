# Check various Thai manufacturer WMIs
$vins = @(
    "MHFBA1AA0HA000001",  # MHF - unknown
    "MHSBA1AA0HA000001",  # MHS
    "MHKBA1AA0HA000001",  # MHK
    "MR0BA1AA0HA000001",  # MR0 - Toyota Thailand
    "MHRBA1AA0HA000001",  # MHR - Honda Thailand
    "MNBBA1AA0HA000001",  # MNB - Ford Thailand
    "MNTBA1AA0HA000001",  # MNT - Nissan Thailand
    "MMABA1AA0HA000001",  # MMA - Mitsubishi Thailand
    "MMBBA1AA0HA000001",  # MMB - Mitsubishi Thailand
    "MBHBA1AA0HA000001",  # MBH
    "MBJBA1AA0HA000001",  # MBJ - Toyota Thailand?
    "MBKBA1AA0HA000001",  # MBK
    "MHCBA1AA0HA000001",  # MHC
    "MHDBA1AA0HA000001",  # MHD
    "MHEBA1AA0HA000001",  # MHE
    "MHGBA1AA0HA000001",  # MHG
    "MHHBA1AA0HA000001",  # MHH
    "MHJBA1AA0HA000001"   # MHJ
)
$data = ($vins -join ";")
$body = "DATA=$data&format=json"
$r = Invoke-RestMethod -Uri "https://vpic.nhtsa.dot.gov/api/vehicles/DecodeVINValuesBatch/" -Method POST -Body $body -ContentType "application/x-www-form-urlencoded" -TimeoutSec 30
foreach ($v in $r.Results) {
    $wmi = $v.VIN.Substring(0,3)
    if ($v.Make -or $v.Manufacturer) {
        Write-Host "$wmi => Make=$($v.Make) Mfr=$($v.Manufacturer)" -ForegroundColor Green
    } else {
        Write-Host "$wmi => (not recognized)" -ForegroundColor DarkGray
    }
}
