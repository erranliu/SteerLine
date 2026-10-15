@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "STEERLINE_HOME=%~dp0"
"%~dp0bin\steerline-web.exe" stop
exit /b %ERRORLEVEL%
