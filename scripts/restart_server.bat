@echo off
REM Kill old server and restart with database
for /f "tokens=5" %%a in ('netstat -aon ^| findstr :8080 ^| findstr LISTENING') do (
    echo Killing PID %%a on port 8080...
    taskkill /pid %%a /f >nul 2>&1
)
echo Waiting for port to release...
timeout /t 3 /nobreak >nul
echo Starting ssda_server.exe...
cd /d c:\ssda\chatGPT\parts-engine
del /q server_stderr.log >nul 2>&1
del /q server_stdout.log >nul 2>&1
start "" ssda_server.exe
timeout /t 3 /nobreak >nul
type server_stderr.log 2>nul
echo.
echo Server started.
