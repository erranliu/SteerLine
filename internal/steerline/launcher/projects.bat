@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: open the multi-project console or choose a project action.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
if not exist "%BIN%" ( echo [projects] missing %BIN% & pause & exit /b 1 )

"%BIN%" projects
echo.
pause
