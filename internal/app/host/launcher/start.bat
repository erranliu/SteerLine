@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "STEERLINE_HOME=%~dp0"
"%~dp0bin\steerline.exe" start %*
set "RESULT=%ERRORLEVEL%"
if not "%RESULT%"=="0" pause
exit /b %RESULT%
