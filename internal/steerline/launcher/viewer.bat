@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Compatibility shortcut for the current console. Pass a config to focus one project.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
if not exist "%BIN%" ( echo [viewer] missing "%BIN%" & pause & exit /b 1 )

mode con: cols=170 lines=52 >nul 2>&1
title SteerLine Console
if "%~1"=="" (
  "%BIN%" console
) else (
  "%BIN%" console -config "%~1"
)
echo.
echo [viewer] exited.
pause
