@echo off
setlocal EnableExtensions DisableDelayedExpansion
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-deploy.ps1" %*
set "deploy_exit=%ERRORLEVEL%"
echo.
if "%deploy_exit%"=="0" (echo Build and deployment succeeded.) else (echo Deployment failed with exit code %deploy_exit%.)
for %%A in (%*) do if /I "%%~A"=="-NoPause" goto done
pause
:done
exit /b %deploy_exit%
