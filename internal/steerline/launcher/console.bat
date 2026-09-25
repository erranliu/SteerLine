@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Open the multi-project console, or one explicit config passed as argument.
set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"

if not exist "%BIN%" ( echo [console] missing "%BIN%" & pause & exit /b 1 )
mode con: cols=170 lines=52 >nul 2>&1
title SteerLine Console
if "%~1"=="" (
  "%BIN%" console
) else (
  "%BIN%" console -config "%~1"
)
echo.
echo [console] exited.
pause
