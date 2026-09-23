@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem ---------------------------------------------------------------------------
rem SteerLine viewer launcher.
rem
rem   viewer.bat [config.json] [run-id|task-id]
rem
rem The binary, configs and per-repo state are shared across source worktrees
rem under STEERLINE_HOME (default C:\SteerlineRuntime):
rem   %STEERLINE_HOME%\bin\steerline.exe   the one binary that runs
rem   %STEERLINE_HOME%\etc\*.json          configs
rem   %STEERLINE_HOME%\state\<repo>\       runtime.json + runs
rem
rem   set STEERLINE_BUILD=1   force a rebuild into the shared bin
rem ---------------------------------------------------------------------------

if "%~1"=="-h" goto :help
if "%~1"=="--help" goto :help

set "SHOME=%STEERLINE_HOME%"
if "%SHOME%"=="" set "SHOME=C:\SteerlineRuntime"
set "BIN=%SHOME%\bin\steerline.exe"

set "CONFIG=%~1"
if "%CONFIG%"=="" set "CONFIG=%STEERLINE_CONFIG%"
if "%CONFIG%"=="" set "CONFIG=%SHOME%\etc\merge-h-308-code.json"
if not exist "%CONFIG%" if exist "%SHOME%\merge-h-308-code\service.orca.json" set "CONFIG=%SHOME%\merge-h-308-code\service.orca.json"

set "RUNID=%~2"
set "RUNARG="
if not "%RUNID%"=="" set "RUNARG=-run-id %RUNID%"

if exist "%BIN%" if not "%STEERLINE_BUILD%"=="1" goto :run

rem Build this checkout into the shared home. Installing over a binary that is
rem currently running the service will fail; use `steerline install` after stop.
set "REPO=%~dp0"
where go >nul 2>&1
if errorlevel 1 (
  if not exist "%BIN%" (
    echo [viewer] go not on PATH and no installed %BIN%
    echo [viewer] run: go build -o "%BIN%" ./cmd/steerline
    pause
    exit /b 1
  )
  goto :run
)
echo [viewer] building %BIN% ...
pushd "%REPO%"
go build -o "%BIN%" ./cmd/steerline
set "BUILD_RC=%ERRORLEVEL%"
popd
if not "%BUILD_RC%"=="0" (
  echo [viewer] build failed
  pause
  exit /b 1
)

:run
if not exist "%CONFIG%" (
  echo [viewer] config not found: %CONFIG%
  echo [viewer] usage: viewer.bat [config.json] [run-id^|task-id]
  pause
  exit /b 1
)
mode con: cols=170 lines=52 >nul 2>&1
title SteerLine viewer

"%BIN%" watch -config "%CONFIG%" %RUNARG%

echo.
echo [viewer] exited.
pause
exit /b 0

:help
echo SteerLine viewer
echo   viewer.bat [config.json] [run-id^|task-id]
echo.
echo STEERLINE_HOME: %SHOME%
echo binary:         %BIN%
echo.
echo Config resolution: 1^) first argument, 2^) %%STEERLINE_CONFIG%%, 3^) %%STEERLINE_HOME%%\etc, 4^) legacy per-repo config.
echo Set STEERLINE_BUILD=1 to force a rebuild. See `steerline home`.
exit /b 0
