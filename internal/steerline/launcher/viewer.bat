@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: open the viewer for one project (default etc\merge-h-308-code.json).
rem Use projects.bat to choose among several; or:  viewer.bat etc\other-repo.json

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
set "CFG=%~1"
if "%CFG%"=="" set "CFG=%SHOME%\etc\merge-h-308-code.json"

if not exist "%BIN%" ( echo [viewer] missing %BIN% & pause & exit /b 1 )
if not exist "%CFG%" ( echo [viewer] missing config %CFG% & pause & exit /b 1 )

mode con: cols=170 lines=52 >nul 2>&1
title SteerLine viewer
"%BIN%" watch -config "%CFG%"
echo.
echo [viewer] exited.
pause
