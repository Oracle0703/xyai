@echo off
setlocal
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0tools\stop-local.ps1" %*
set "EXIT_CODE=%errorlevel%"
if not "%EXIT_CODE%"=="0" (
  echo.
  echo stop-local failed with exit code %EXIT_CODE%.
  echo Press any key to close...
  pause >nul
)
exit /b %EXIT_CODE%
