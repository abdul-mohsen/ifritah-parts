@echo off
echo Stopping server on port 8080...
for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8080.*LISTENING"') do (
    echo Killing PID %%a
    taskkill /pid %%a /f >nul 2>&1
)
timeout /t 2 /nobreak >nul
echo Re-seeding database...
cd /d c:\ssda\chatGPT\parts-engine
go run ./scripts/seed_db/
if errorlevel 1 (
    echo SEED FAILED
    exit /b 1
)
echo Starting server...
start /b ssda_server.exe >nul 2>&1
timeout /t 3 /nobreak >nul
echo Done. Server should be running.
