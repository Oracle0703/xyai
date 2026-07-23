#Requires -Version 5.1
<#
.SYNOPSIS
  Start Understand Anything dashboards for code graph and/or llm-wiki knowledge graph.

.EXAMPLE
  .\tools\start-understand-dashboard.ps1
  .\tools\start-understand-dashboard.ps1 -Target wiki
  .\tools\start-understand-dashboard.ps1 -Target both -CodePort 5173 -WikiPort 5174
#>
param(
  [ValidateSet('code', 'wiki', 'both')]
  [string]$Target = 'both',
  [int]$CodePort = 5173,
  [int]$WikiPort = 5174,
  [string]$CodeToken = 'codegraph',
  [string]$WikiToken = 'wikigraph'
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$DashCandidates = @(
  (Join-Path $env:USERPROFILE '.understand-anything\repo\understand-anything-plugin\packages\dashboard'),
  (Join-Path $env:USERPROFILE '.understand-anything-plugin\packages\dashboard')
)
$Dash = $DashCandidates | Where-Object { Test-Path (Join-Path $_ 'package.json') } | Select-Object -First 1
if (-not $Dash) {
  throw "Dashboard package not found. Install understand-anything plugin first."
}

$LogDir = Join-Path $RepoRoot '.ua-dashboard-logs'
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

function Test-PortListen([int]$Port) {
  return [bool](Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
}

function Start-OneDashboard {
  param(
    [string]$Name,
    [string]$GraphDir,
    [int]$Port,
    [string]$Token
  )
  $graph = Join-Path $GraphDir '.understand-anything\knowledge-graph.json'
  if (-not (Test-Path $graph)) {
    Write-Warning "[$Name] missing graph: $graph (run /understand or refresh script first)"
    return $null
  }
  if (Test-PortListen $Port) {
    Write-Host "[$Name] already listening on $Port"
    return "http://127.0.0.1:$Port/?token=$Token"
  }
  $out = Join-Path $LogDir "$Name.out.log"
  $err = Join-Path $LogDir "$Name.err.log"
  $cmd = "set GRAPH_DIR=$GraphDir&& set UNDERSTAND_ACCESS_TOKEN=$Token&& node_modules\.bin\vite.cmd --host 127.0.0.1 --port $Port --strictPort"
  $p = Start-Process -FilePath 'cmd.exe' -ArgumentList @('/c', $cmd) -WorkingDirectory $Dash `
    -WindowStyle Hidden -RedirectStandardOutput $out -RedirectStandardError $err -PassThru
  Start-Sleep -Seconds 2
  if (-not (Test-PortListen $Port)) {
    Write-Warning "[$Name] failed to bind port $Port. See $err"
    if (Test-Path $err) { Get-Content $err -Tail 20 | ForEach-Object { Write-Host $_ } }
    return $null
  }
  Write-Host "[$Name] pid=$($p.Id) ready"
  return "http://127.0.0.1:$Port/?token=$Token"
}

$urls = @()
if ($Target -eq 'code' -or $Target -eq 'both') {
  $u = Start-OneDashboard -Name 'code' -GraphDir $RepoRoot -Port $CodePort -Token $CodeToken
  if ($u) { $urls += $u }
}
if ($Target -eq 'wiki' -or $Target -eq 'both') {
  $u = Start-OneDashboard -Name 'wiki' -GraphDir (Join-Path $RepoRoot 'llm-wiki') -Port $WikiPort -Token $WikiToken
  if ($u) { $urls += $u }
}

Write-Host ''
Write-Host 'Dashboard URLs (token required):'
foreach ($u in $urls) { Write-Host "  $u" }
if (-not $urls) { exit 1 }
