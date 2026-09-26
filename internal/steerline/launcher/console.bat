@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Open the multi-project console, or one explicit config passed as argument.
set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"

if not exist "%BIN%" ( echo [console] missing "%BIN%" & pause & exit /b 1 )

rem Open a dedicated interactive console. This also works when this launcher
rem was called from a process without an attached terminal (where Bubble Tea
rem would otherwise exit immediately with "console requires an interactive terminal").
if "%~1"=="" (
  start "SteerLine Console" "%BIN%" console
) else (
  start "SteerLine Console" "%BIN%" console -config "%~1"
)
exit /b %ERRORLEVEL%
