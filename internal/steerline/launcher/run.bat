@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: run one project's scheduler in this window (Ctrl+C to stop).
rem Run several at once for several projects; each needs its own config file.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
set "CFG=%~1"
if "%CFG%"=="" set "CFG=%SHOME%\etc\merge-h-308-code.json"

if not exist "%BIN%" ( echo [run] missing %BIN% & pause & exit /b 1 )
if not exist "%CFG%" ( echo [run] missing config %CFG% & pause & exit /b 1 )

mode con: cols=170 lines=52 >nul 2>&1
title SteerLine service
"%BIN%" run -config "%CFG%" -console pretty
echo.
echo [run] service exited.
pause
