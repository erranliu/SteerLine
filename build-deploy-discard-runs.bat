@echo off
setlocal EnableExtensions DisableDelayedExpansion
call "%~dp0build-deploy.bat" -UpdateRunner -DiscardRuns %*
exit /b %ERRORLEVEL%
