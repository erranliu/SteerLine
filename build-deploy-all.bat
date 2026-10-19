@echo off
setlocal EnableExtensions DisableDelayedExpansion
call "%~dp0build-deploy.bat" -UpdateRunner %*
exit /b %ERRORLEVEL%
