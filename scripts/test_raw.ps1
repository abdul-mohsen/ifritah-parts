# Raw test - show full JSON response
$wc = [System.Net.WebClient]::new()
$wc.Headers.Add("Content-Type", "application/json")
$body = '{"vin":"1N4AA6AP9HC406410"}'
$json = $wc.UploadString("http://localhost:8080/api/vin/decode", "POST", $body)
$data = $json | ConvertFrom-Json
$data.nhtsaRaw | ConvertTo-Json -Depth 5
