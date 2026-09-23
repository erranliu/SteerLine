@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: ask one project's running service to stop.
rem Optional:  stop.bat etc\other-repo.json

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
set "CFG=%~1"
if "%CFG%"=="" set "CFG=%SHOME%\etc\merge-h-308-code.json"

if not exist "%BIN%" ( echo [stop] missing %BIN% & pause & exit /b 1 )
if not exist "%CFG%" ( echo [stop] missing config %CFG% & pause & exit /b 1 )

"%BIN%" stop -config "%CFG%"
echo.
echo [stop] requested. The service will exit shortly.
pause
