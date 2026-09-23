@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem ---------------------------------------------------------------------------
rem SteerLine viewer launcher.
rem
rem   viewer.bat [config.json] [run-id|task-id]
rem
rem   config.json  optional; defaults to %STEERLINE_CONFIG% or the path below.
rem   run-id       optional; opens the viewer focused on that run/task.
rem
rem   set STEERLINE_NO_BUILD=1   skip `go build` and use the existing binary.
rem ---------------------------------------------------------------------------

set "REPO=%~dp0"
set "BIN=%REPO%bin\steerline.exe"

if "%~1"=="-h" goto :help
if "%~1"=="--help" goto :help

if "%STEERLINE_CONFIG%"=="" set "STEERLINE_CONFIG=C:\SteerlineRuntime\merge-h-308-code\service.orca.json"

set "CONFIG=%~1"
if "%CONFIG%"=="" set "CONFIG=%STEERLINE_CONFIG%"

set "RUNID=%~2"
set "RUNARG="
if not "%RUNID%"=="" set "RUNARG=-run-id %RUNID%"

if not exist "%CONFIG%" (
  echo [viewer] config not found: %CONFIG%
  echo [viewer] usage: viewer.bat [config.json] [run-id^|task-id]
  pause
  exit /b 1
)

if "%STEERLINE_NO_BUILD%"=="1" goto :run

where go >nul 2>&1
if errorlevel 1 (
  if not exist "%BIN%" (
    echo [viewer] go not on PATH and no prebuilt %BIN%
    pause
    exit /b 1
  )
  echo [viewer] go not on PATH; using existing binary
  goto :run
)

echo [viewer] building...
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
rem Best-effort enlarge the console; ignore if the screen is smaller.
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
echo Config resolution: 1^) first argument, 2^) %%STEERLINE_CONFIG%%, 3^) built-in default.
echo Set STEERLINE_NO_BUILD=1 to skip rebuilding.
exit /b 0
