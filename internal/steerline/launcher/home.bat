@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: show where this machine keeps the SteerLine binary, configs
rem and per-repo state.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"

if not exist "%BIN%" ( echo [home] missing %BIN% & pause & exit /b 1 )
"%BIN%" home -config "%SHOME%\etc\merge-h-308-code.json"
echo.
pause
