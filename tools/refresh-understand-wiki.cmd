@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0refresh-understand-wiki.ps1" %*
exit /b %ERRORLEVEL%
