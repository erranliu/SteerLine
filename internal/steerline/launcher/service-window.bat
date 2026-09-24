@echo off
setlocal EnableExtensions DisableDelayedExpansion
chcp 65001 >nul 2>&1

rem Keep this window open while the service runs. Ctrl+C requests shutdown.
rem Closing the window with X is an abrupt exit.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
if "%~1"=="" (
  set "CFG=%SHOME%\etc\merge-h-308-code.json"
) else (
  set "CFG=%~f1"
)

if not exist "%BIN%" ( echo [service-window] missing "%BIN%" & pause & exit /b 1 )
if not exist "%CFG%" ( echo [service-window] missing config "%CFG%" & pause & exit /b 1 )

mode con: cols=150 lines=44 >nul 2>&1
title SteerLine service window
echo [service-window] Keep this window open or minimized while the service runs.
echo [service-window] Ctrl+C requests shutdown. Closing with X ends it abruptly.
"%BIN%" run -config "%CFG%" -dispatch manual -console summary -service-window
set "RESULT=%ERRORLEVEL%"
echo.
echo [service-window] service exited with code %RESULT%.
echo [service-window] Reopen this launcher to restart with the current config.
pause
exit /b %RESULT%
