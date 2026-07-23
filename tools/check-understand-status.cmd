@echo off
setlocal
REM Bypass local ExecutionPolicy so tools work under RemoteSigned/Restricted.
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0check-understand-status.ps1" %*
exit /b %ERRORLEVEL%
