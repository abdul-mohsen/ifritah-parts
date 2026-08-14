# Build server and run it
cd c:\ssda\chatGPT\parts-engine
go build -o server.exe ./cmd/server
Write-Host "Build complete, starting server..."
Start-Process .\server.exe
