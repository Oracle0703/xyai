#Requires -Version 5.1
<#
.SYNOPSIS
  Report llm-wiki + knowledge graph readiness (integration health).

.EXAMPLE
  tools\check-understand-status.cmd
  tools\check-understand-status.cmd -AllowDirtyWiki
#>
param(
  # Pre-commit local iteration: allow dirty llm-wiki while still validating JSON/hash tooling.
  [switch]$AllowDirtyWiki
)

$ErrorActionPreference = 'Continue'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Ok([bool]$cond, [string]$msg) {
  if ($cond) {
    Write-Host "[OK] $msg" -ForegroundColor Green
    return 0
  }
  Write-Host "[!!] $msg" -ForegroundColor Yellow
  return 1
}

function Info([string]$msg) {
  Write-Host "[--] $msg" -ForegroundColor DarkGray
}

function Test-GitIgnored([string]$relPath) {
  Push-Location $Root
  try {
    git check-ignore -q -- $relPath 2>$null | Out-Null
    return ($LASTEXITCODE -eq 0)
  } finally {
    Pop-Location
  }
}

function Get-GitHead {
  Push-Location $Root
  try {
    $h = (git rev-parse HEAD 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($h)) { return $null }
    return $h
  } finally {
    Pop-Location
  }
}

function Get-WikiSourceHash {
  $wiki = Join-Path $Root 'llm-wiki'
  $sha = [System.Security.Cryptography.SHA256]::Create()
  try {
    $files = Get-ChildItem -LiteralPath $wiki -Recurse -File -Force | Where-Object {
      $rel = $_.FullName.Substring($wiki.Length).TrimStart('\', '/')
      $rel -notmatch '(^|\\|/)\.understand-anything(\\|/|$)'
    } | Sort-Object { $_.FullName.Substring($wiki.Length).ToLowerInvariant() }

    foreach ($f in $files) {
      $rel = ($f.FullName.Substring($wiki.Length).TrimStart('\', '/') -replace '\\', '/')
      $nameBytes = [System.Text.Encoding]::UTF8.GetBytes($rel)
      $sha.TransformBlock($nameBytes, 0, $nameBytes.Length, $null, 0) | Out-Null
      $nul = [byte[]](0)
      $sha.TransformBlock($nul, 0, 1, $null, 0) | Out-Null
      $bytes = [System.IO.File]::ReadAllBytes($f.FullName)
      if ($bytes.Length -gt 0) {
        $sha.TransformBlock($bytes, 0, $bytes.Length, $null, 0) | Out-Null
      }
      $sha.TransformBlock($nul, 0, 1, $null, 0) | Out-Null
    }
    $sha.TransformFinalBlock([byte[]]::new(0), 0, 0) | Out-Null
    return ([System.BitConverter]::ToString($sha.Hash) -replace '-', '').ToLowerInvariant()
  } finally {
    $sha.Dispose()
  }
}

function Test-GraphJson {
  param(
    [string]$Path,
    [string]$Label,
    [ValidateSet('code', 'knowledge')]
    [string]$Kind
  )
  if (-not (Test-Path $Path)) {
    return (Ok $false "$Label missing: $Path")
  }

  $localFail = 0
  try {
    $raw = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
    if ([string]::IsNullOrWhiteSpace($raw)) {
      return (Ok $false "$Label is empty")
    }
    $g = $raw | ConvertFrom-Json
  } catch {
    return (Ok $false "$Label is not valid JSON: $($_.Exception.Message)")
  }

  $nodeCount = @($g.nodes).Count
  if ($nodeCount -lt 1) {
    $localFail += Ok $false "$Label has no nodes"
  } else {
    $localFail += Ok $true "$Label nodes=$nodeCount"
  }

  if ($null -eq $g.edges) {
    $localFail += Ok $false "$Label missing edges array"
  } else {
    $localFail += Ok $true "$Label edges=$(@($g.edges).Count)"
  }

  if ($Kind -eq 'code') {
    if ($null -eq $g.project -or [string]::IsNullOrWhiteSpace([string]$g.project.name)) {
      $localFail += Ok $false "$Label missing project.name"
    } else {
      $localFail += Ok $true "$Label project=$($g.project.name)"
    }
  } else {
    $kindVal = [string]$g.kind
    if ($kindVal -and $kindVal -ne 'knowledge') {
      $localFail += Ok $false "$Label kind='$kindVal' (expected knowledge or empty)"
    } else {
      $localFail += Ok $true "$Label kind=knowledge-compatible"
    }
  }

  $ids = @{}
  foreach ($n in @($g.nodes)) {
    if ($n.id) { $ids[[string]$n.id] = $true }
  }
  $dangling = 0
  foreach ($e in @($g.edges)) {
    $s = [string]$e.source
    $t = [string]$e.target
    if (-not $ids.ContainsKey($s) -or -not $ids.ContainsKey($t)) { $dangling++ }
  }
  if ($dangling -gt 0) {
    $localFail += Ok $false "$Label has $dangling dangling edge(s)"
  } else {
    $localFail += Ok $true "$Label edges reference existing nodes"
  }

  return $localFail
}

function Get-MetaObject([string]$MetaPath) {
  if (-not (Test-Path $MetaPath)) { return $null }
  try {
    return (Get-Content -LiteralPath $MetaPath -Raw -Encoding UTF8 | ConvertFrom-Json)
  } catch {
    return $null
  }
}

function Test-HistoryDelta {
  param([string]$FromCommit, [string]$Pathspec)
  Push-Location $Root
  try {
    if ([string]::IsNullOrWhiteSpace($FromCommit)) { return @{ Ok = $false; Reason = 'missing commit' } }
    git cat-file -e "$FromCommit^{commit}" 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { return @{ Ok = $false; Reason = "unknown commit $FromCommit" } }
    git merge-base --is-ancestor $FromCommit HEAD 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { return @{ Ok = $false; Reason = "meta commit not ancestor of HEAD ($FromCommit)" } }
    $delta = @(git log --oneline "$FromCommit..HEAD" -- $Pathspec 2>$null | Where-Object { $_ })
    if ($delta.Count -gt 0) {
      return @{
        Ok     = $false
        Reason = "$($delta.Count) commit(s) after meta touch pathspec (e.g. $($delta[0]))"
      }
    }
    $short = $FromCommit.Substring(0, [Math]::Min(12, $FromCommit.Length))
    return @{ Ok = $true; Reason = "no pathspec changes since meta $short" }
  } finally {
    Pop-Location
  }
}

function Test-WikiDirty {
  Push-Location $Root
  try {
    $out = git status --porcelain -- llm-wiki 2>$null
    if ($LASTEXITCODE -ne 0) { return @{ Dirty = $true; Detail = 'git status failed'; Count = -1 } }
    $lines = @($out | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    return @{ Dirty = ($lines.Count -gt 0); Detail = ($lines -join '; '); Count = $lines.Count }
  } finally {
    Pop-Location
  }
}

$fail = 0
$head = Get-GitHead
if ($head) {
  $fail += Ok $true "git HEAD=$head"
} else {
  $fail += Ok $false 'git HEAD unavailable'
}

$fail += Ok (Test-Path (Join-Path $Root 'llm-wiki\wiki\README.md')) 'llm-wiki/wiki/README.md exists'
$fail += Ok (Test-Path (Join-Path $Root 'llm-wiki\index.md')) 'llm-wiki/index.md (Karpathy index) exists'
$fail += Ok (Test-Path (Join-Path $Root 'AGENTS.md')) 'root AGENTS.md exists'
$fail += Ok (Test-Path (Join-Path $Root '.understand-anything\config.json')) 'code graph config.json trackable path exists'
$fail += Ok (Test-Path (Join-Path $Root '.understand-anything\.understandignore')) '.understandignore scope exists'
$fail += Ok (Test-Path (Join-Path $Root 'tools\check-understand-status.cmd')) 'check cmd entrypoint exists'
$fail += Ok (Test-Path (Join-Path $Root 'tools\refresh-understand-wiki.cmd')) 'refresh cmd entrypoint exists'
$fail += Ok (Test-Path (Join-Path $Root 'tools\start-understand-dashboard.cmd')) 'dashboard cmd entrypoint exists'
$fail += Ok (Test-Path (Join-Path $Root 'tools\understand-wiki-analysis-seed.json')) 'wiki analysis seed exists'

$fail += Ok (-not (Test-GitIgnored '.understand-anything/config.json')) 'config.json is NOT gitignored (shareable)'
$fail += Ok (Test-GitIgnored '.understand-anything/knowledge-graph.json') 'code knowledge-graph.json IS gitignored (large)'
$fail += Ok (-not (Test-GitIgnored 'llm-wiki/.understand-anything/knowledge-graph.json')) 'wiki knowledge-graph.json is NOT gitignored (shareable)'

$codeGraph = Join-Path $Root '.understand-anything\knowledge-graph.json'
$wikiGraph = Join-Path $Root 'llm-wiki\.understand-anything\knowledge-graph.json'
$codeMetaPath = Join-Path $Root '.understand-anything\meta.json'
$wikiMetaPath = Join-Path $Root 'llm-wiki\.understand-anything\meta.json'

if (Test-Path $codeGraph) {
  $fail += Test-GraphJson -Path $codeGraph -Label 'code graph' -Kind code
} else {
  Info 'code knowledge-graph.json absent (optional local artifact; run /understand)'
}

$fail += Test-GraphJson -Path $wikiGraph -Label 'wiki graph' -Kind knowledge

# Wiki freshness: content hash (excludes .understand-anything) + optional git baseline
$wikiMeta = Get-MetaObject $wikiMetaPath
if ($null -eq $wikiMeta) {
  $fail += Ok $false 'wiki meta.json missing or invalid JSON'
} else {
  $metaHash = [string]$wikiMeta.wikiSourceHash
  if ([string]::IsNullOrWhiteSpace($metaHash)) {
    $fail += Ok $false 'wiki meta.json missing wikiSourceHash (re-run tools\refresh-understand-wiki.cmd)'
  } else {
    $currentHash = Get-WikiSourceHash
    if ($currentHash -eq $metaHash) {
      $fail += Ok $true "wikiSourceHash matches working tree ($($metaHash.Substring(0,12)))"
    } else {
      $fail += Ok $false "wikiSourceHash mismatch meta=$($metaHash.Substring(0,12)) tree=$($currentHash.Substring(0,12)) — run tools\refresh-understand-wiki.cmd"
    }
  }

  $wikiCommit = [string]$wikiMeta.gitCommitHash
  if ($head -and -not [string]::IsNullOrWhiteSpace($wikiCommit)) {
    # Informational ancestry: graph may be built before commit lands.
    git -C $Root cat-file -e "$wikiCommit^{commit}" 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
      git -C $Root merge-base --is-ancestor $wikiCommit HEAD 2>$null | Out-Null
      if ($LASTEXITCODE -eq 0 -or $wikiCommit -eq $head) {
        Info "wiki meta gitCommitHash=$($wikiCommit.Substring(0,12)) is on current branch history"
      } else {
        $fail += Ok $false "wiki meta gitCommitHash=$wikiCommit is not ancestor of HEAD"
      }
    } else {
      Info "wiki meta gitCommitHash=$wikiCommit not in local object db (ok if graph produced offline)"
    }
  }
}

# Code graph baseline (local): backend/frontend history after meta
if ((Test-Path $codeGraph) -and $head) {
  $codeMeta = Get-MetaObject $codeMetaPath
  $codeCommit = if ($codeMeta) { [string]$codeMeta.gitCommitHash } else { '' }
  if ([string]::IsNullOrWhiteSpace($codeCommit)) {
    Info 'code meta.json missing gitCommitHash (local graph may be incomplete)'
  } else {
    $c = Test-HistoryDelta -FromCommit $codeCommit -Pathspec 'backend frontend'
    if ($c.Ok) {
      $fail += Ok $true "code graph baseline: $($c.Reason)"
    } else {
      $fail += Ok $false "code graph stale vs HEAD: $($c.Reason) — run /understand"
    }
  }
}

$dirty = Test-WikiDirty
if ($dirty.Dirty) {
  if ($AllowDirtyWiki) {
    Info "llm-wiki dirty ($($dirty.Count) paths) allowed via -AllowDirtyWiki"
    Info $dirty.Detail
  } else {
    $fail += Ok $false ("llm-wiki working tree dirty ($($dirty.Count) paths) — commit or refresh before READY")
    Info $dirty.Detail
  }
} else {
  $fail += Ok $true 'llm-wiki working tree clean'
}

foreach ($port in 5173, 5174) {
  $up = [bool](Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)
  if ($up) { Write-Host "[OK] dashboard port $port listening" -ForegroundColor Green }
  else { Info "dashboard port $port not listening (run tools\start-understand-dashboard.cmd)" }
}

$agents = Get-Content (Join-Path $Root 'AGENTS.md') -Raw -ErrorAction SilentlyContinue
if ($agents -match 'understand|知识图谱|knowledge-graph') {
  Write-Host '[OK] AGENTS.md references knowledge graph workflow' -ForegroundColor Green
} else {
  Write-Host '[!!] AGENTS.md missing knowledge graph integration' -ForegroundColor Yellow
  $fail++
}

if ($agents -match 'check-understand-status\.cmd|ExecutionPolicy Bypass') {
  Write-Host '[OK] AGENTS.md documents executable entrypoints' -ForegroundColor Green
} else {
  Write-Host '[!!] AGENTS.md should document tools\*.cmd or Bypass entrypoints' -ForegroundColor Yellow
  $fail++
}

Write-Host ''
if ($fail -gt 0) {
  Write-Host "Status: PARTIAL ($fail checks need attention)" -ForegroundColor Yellow
  exit 1
}
Write-Host 'Status: READY' -ForegroundColor Green
exit 0
