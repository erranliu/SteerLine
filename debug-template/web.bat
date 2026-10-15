@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "STEERLINE_HOME=%~dp0home"
set "STEERLINE_E2E_CONTROLLED_STATE=%~dp0provider\state.json"
"%~dp0bin\steerline-web.exe" open %*
exit /b %ERRORLEVEL%
