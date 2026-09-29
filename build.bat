@echo off
setlocal

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1"
set "build_exit=%ERRORLEVEL%"

echo.
if "%build_exit%"=="0" (
    echo Build succeeded.
) else (
    echo Build failed with exit code %build_exit%.
)

if /I not "%~1"=="--no-pause" pause
exit /b %build_exit%
