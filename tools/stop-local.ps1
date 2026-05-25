$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot ".localdev"
$PidsDir = Join-Path $LocalDir "pids"

function Stop-ManagedProcess([string]$PidFile) {
    if (-not (Test-Path $PidFile)) {
        return
    }

    $pidText = Get-Content $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($pidText -and $pidText -match '^\d+$') {
        $proc = Get-Process -Id ([int]$pidText) -ErrorAction SilentlyContinue
        if ($proc) {
            cmd /d /c "taskkill /PID $($proc.Id) /T /F >nul 2>nul" | Out-Null
        }
    }

    Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
}

function Stop-ProcessByPort([int]$Port) {
    $connections = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue
    foreach ($connection in ($connections | Select-Object -Unique OwningProcess)) {
        if ($connection.OwningProcess) {
            cmd /d /c "taskkill /PID $($connection.OwningProcess) /T /F >nul 2>nul" | Out-Null
        }
    }
}

function Stop-ProcessByCommandPattern([string]$Pattern) {
    $targets = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.CommandLine -and $_.CommandLine -like "*$Pattern*"
    }
    foreach ($target in $targets) {
        cmd /d /c "taskkill /PID $($target.ProcessId) /T /F >nul 2>nul" | Out-Null
    }
}

Write-Host "==> Stopping local frontend/backend" -ForegroundColor Cyan
Stop-ManagedProcess (Join-Path $PidsDir "backend.pid")
Stop-ManagedProcess (Join-Path $PidsDir "frontend.pid")
Stop-ProcessByPort 8080
Stop-ProcessByPort 3000
Stop-ProcessByCommandPattern "go run ./cmd/server"
Stop-ProcessByCommandPattern "vite.js"
Stop-ProcessByCommandPattern "pnpm run dev --host 127.0.0.1"
Write-Host "Local services stopped."
