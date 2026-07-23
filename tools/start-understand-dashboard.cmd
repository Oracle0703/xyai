@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-understand-dashboard.ps1" %*
exit /b %ERRORLEVEL%
