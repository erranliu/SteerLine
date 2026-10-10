@echo off
setlocal EnableExtensions DisableDelayedExpansion
chcp 65001 >nul 2>&1
"%~dp0bin\debug-helper.exe" debug reset %*
set "RESULT=%ERRORLEVEL%"
if /i "%~1"=="--no-pause" exit /b %RESULT%
if /i "%~2"=="--no-pause" exit /b %RESULT%
pause
exit /b %RESULT%
