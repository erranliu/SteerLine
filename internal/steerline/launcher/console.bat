@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Open the multi-project console, or one explicit config passed as argument.
set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "STEERLINE_HOME=%SHOME%"
set "BIN=%SHOME%\bin\steerline.exe"

if not exist "%BIN%" ( echo [console] missing "%BIN%" & pause & exit /b 1 )

rem Open a dedicated interactive console and keep startup errors visible.
if /i "%~1"=="--steerline-console-child" goto child
if "%~1"=="" (
  start "SteerLine Console" cmd.exe /c ""%~f0" --steerline-console-child"
) else (
  start "SteerLine Console" cmd.exe /c ""%~f0" --steerline-console-child "%~1""
)
exit /b %ERRORLEVEL%

:child
if "%~2"=="" (
  "%BIN%" console
) else (
  "%BIN%" console -config "%~2"
)
set "RESULT=%ERRORLEVEL%"
if not "%RESULT%"=="0" (
  echo.
  echo [console] exited with code %RESULT%.
  pause
)
exit /b %RESULT%
