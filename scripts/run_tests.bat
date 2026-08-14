@echo off
cd /d c:\ssda\chatGPT\parts-engine

echo Building test binary...
go build -o partsouq_test.exe .\scripts\partsouq_test_v2\main.go
if errorlevel 1 (
    echo BUILD FAILED
    exit /b 1
)

echo Running 100-test PartsOuq verification suite...
partsouq_test.exe
echo.
echo Exit code: %ERRORLEVEL%
