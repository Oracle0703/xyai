#Requires -Version 5.1
<#
.SYNOPSIS
  Rebuild llm-wiki knowledge graph from markdown.

.EXAMPLE
  tools\refresh-understand-wiki.cmd
#>
param(
  [switch]$SkipAnalysisReuse
)

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$WikiRoot = Join-Path $RepoRoot 'llm-wiki'
$Skill = Join-Path $env:USERPROFILE '.agents\skills\understand-knowledge'
$Parse = Join-Path $Skill 'parse-knowledge-base.py'
$Merge = Join-Path $Skill 'merge-knowledge-graph.py'

if (-not (Test-Path $Parse)) { throw "Missing $Parse — install understand-knowledge skill." }
if (-not (Test-Path (Join-Path $WikiRoot 'index.md'))) {
  throw "Missing llm-wiki/index.md — Karpathy index required."
}

$Inter = Join-Path $WikiRoot '.understand-anything\intermediate'
New-Item -ItemType Directory -Force -Path $Inter | Out-Null

Write-Host '[1/3] parse-knowledge-base...'
python $Parse $WikiRoot
if ($LASTEXITCODE -ne 0) { throw "parse failed exit=$LASTEXITCODE" }

$analysis = Join-Path $Inter 'analysis-batch-1.json'
if (-not $SkipAnalysisReuse) {
  $seedPath = Join-Path $PSScriptRoot 'understand-wiki-analysis-seed.json'
  if (Test-Path $seedPath) {
    Copy-Item $seedPath $analysis -Force
  }
}

Write-Host '[2/3] merge-knowledge-graph...'
python $Merge $WikiRoot
if ($LASTEXITCODE -ne 0) { throw "merge failed exit=$LASTEXITCODE" }

Write-Host '[3/3] validate + save...'
$py = @'
import hashlib
import json
import shutil
import subprocess
from datetime import datetime, timezone
from pathlib import Path

wiki = Path(r"""WIKI_ROOT""")
repo = Path(r"""REPO_ROOT""")
inter = wiki / ".understand-anything" / "intermediate"
g = json.loads((inter / "assembled-graph.json").read_text(encoding="utf-8"))
g["kind"] = "knowledge"
g.setdefault("version", "1.0.0")
nodes = {n["id"]: n for n in g.get("nodes", [])}
for n in g["nodes"]:
    n.setdefault("tags", ["untagged"])
    if not n.get("tags"):
        n["tags"] = ["untagged"]
    n.setdefault("summary", "No summary available")
    n.setdefault("complexity", "simple")
    n.setdefault("name", n["id"])
g["edges"] = [e for e in g.get("edges", []) if e.get("source") in nodes and e.get("target") in nodes]
out = wiki / ".understand-anything"
out.mkdir(parents=True, exist_ok=True)
(out / "knowledge-graph.json").write_text(json.dumps(g, ensure_ascii=False, indent=2), encoding="utf-8")

def wiki_source_hash(root: Path) -> str:
    """Hash wiki sources excluding generated .understand-anything artifacts."""
    h = hashlib.sha256()
    files = []
    for p in root.rglob("*"):
        if not p.is_file():
            continue
        rel = p.relative_to(root).as_posix()
        if rel.startswith(".understand-anything/") or "/.understand-anything/" in rel:
            continue
        files.append(p)
    for p in sorted(files, key=lambda x: x.relative_to(root).as_posix().lower()):
        rel = p.relative_to(root).as_posix()
        h.update(rel.encode("utf-8"))
        h.update(b"\0")
        h.update(p.read_bytes())
        h.update(b"\0")
    return h.hexdigest()

try:
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=str(repo), text=True).strip()
except Exception:
    commit = ""

source_hash = wiki_source_hash(wiki)
meta = {
    "lastAnalyzedAt": datetime.now(timezone.utc).isoformat(),
    "gitCommitHash": commit,
    "wikiSourceHash": source_hash,
    "version": "1.0.0",
    "analyzedFiles": sum(1 for n in g["nodes"] if n.get("type") == "article"),
    "kind": "knowledge",
}
(out / "meta.json").write_text(json.dumps(meta, indent=2), encoding="utf-8")
shutil.rmtree(inter, ignore_errors=True)
print(
    f"saved nodes={len(g['nodes'])} edges={len(g['edges'])} "
    f"layers={len(g.get('layers', []))} tour={len(g.get('tour', []))} "
    f"wikiSourceHash={source_hash[:12]}"
)
'@
$py = $py.Replace('WIKI_ROOT', $WikiRoot.Replace('\', '\\')).Replace('REPO_ROOT', $RepoRoot.Replace('\', '\\'))
$tmpPy = Join-Path $env:TEMP 'refresh-understand-wiki-save.py'
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($tmpPy, $py, $utf8)
python $tmpPy
if ($LASTEXITCODE -ne 0) { throw "save failed" }
Write-Host "OK: $(Join-Path $WikiRoot '.understand-anything\knowledge-graph.json')"
