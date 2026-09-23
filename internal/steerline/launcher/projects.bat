@echo off
setlocal EnableExtensions
chcp 65001 >nul 2>&1

rem Double-click: pick one of the projects under etc\ and an action
rem (viewer / run service / stop / home). Multiple projects can run at once,
rem each with its own config and its own state\<repo> directory.

set "SHOME=%~dp0"
if "%SHOME:~-1%"=="\" set "SHOME=%SHOME:~0,-1%"
set "BIN=%SHOME%\bin\steerline.exe"
if not exist "%BIN%" ( echo [projects] missing %BIN% & pause & exit /b 1 )

"%BIN%" projects
echo.
pause
