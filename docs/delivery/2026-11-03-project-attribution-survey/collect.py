"""Read-only survey of indexed unattributed requests; emits paths, never prompt text."""
import collections
import hashlib
import json
import os
import re
import subprocess
import sys
import time
from urllib.parse import urlsplit, urlunsplit

START = "2026-11-02 00:00:00+08"
END = "2026-11-03 00:00:00+08"
ARCHIVES = "/opt/sub2api/backend/data/request-archive"


def query(sql):
    wrapped = "BEGIN READ ONLY; SET LOCAL statement_timeout='30s'; " + sql + "; ROLLBACK;"
    p = subprocess.run(["sudo", "-u", "postgres", "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-d", "sub2api"],
                       input=wrapped, text=True, capture_output=True, check=True, cwd="/tmp")
    return json.loads(p.stdout)


def strings(value, context=""):
    if isinstance(value, str):
        yield context, value
    elif isinstance(value, list):
        for item in value:
            yield from strings(item, context)
    elif isinstance(value, dict):
        role = value.get("role", context)
        for key in ("system", "instructions", "input", "messages", "content", "text", "arguments", "output", "result"):
            if key in value:
                yield from strings(value[key], str(role) + "/" + key)


PATH = re.compile(r"(?<![A-Za-z0-9])(?:[A-Za-z]:[\\/]|/(?:Users|home|root|workspace|workspaces|work|app|opt|mnt|data|code|projects|tmp)/)[^\s\"'<>|*?`\x00]{2,350}")
TAGGED = re.compile(r"<tagged_files>(.*?)</tagged_files>", re.S | re.I)
CWD = re.compile(r"<cwd>([^<\n]{1,350})</cwd>|(?:Primary working|Working) director(?:y|ies):\s*([^\n]{1,350})", re.I)
URL = re.compile(r"(?:https?://|ssh://|git://|git@)[A-Za-z0-9_.:@/-]{5,250}")


def clean_path(p):
    p = re.split(r"\\[nr]", p)[0]
    p = p.replace("\\", "/")
    p = re.sub(r"/{2,}", "/", p)
    p = re.split(r" \(was |[，。；）、（\[\]]|/n(?=[*+\s/])|/r/n", p)[0]
    p = p.strip(" -*`\"'")
    if p.startswith("file:"):
        p = p[5:]
    p = re.sub(r"/+$", "", p.rstrip(".,;:)]}"))
    p = re.sub(r":\d+(?::\d+)?$", "", p)
    p = re.sub(r"(/Users/|/home/)[^/]+/", r"\1<user>/", p)
    p = re.sub(r"(?i)([a-z]:/Users/)[^/]+/", r"\1<user>/", p)
    if len(p) > 300 or "\x00" in p:
        return None
    if re.search(r"(?i)(?:sk-|ghp_|github_pat_|token=|password=|secret=)", p):
        return None
    return p


def line_before(f, end):
    pos = end
    chunks = []
    total = 0
    while pos > 0 and total < 128 * 1024 * 1024:
        size = min(pos, 1024 * 1024)
        pos -= size
        f.seek(pos)
        chunk = f.read(size)
        total += len(chunk)
        boundary = chunk.rfind(b"\n", 0, len(chunk) - 1 if not chunks else len(chunk))
        if boundary >= 0:
            chunks.append(chunk[boundary + 1:])
            return b"".join(reversed(chunks)), total
        chunks.append(chunk)
    if pos == 0:
        return b"".join(reversed(chunks)), total
    raise ValueError("line_exceeds_128MiB")


def run():
    where = "event_time >= '%s' AND event_time < '%s'" % (START, END)
    stats = query("SELECT row_to_json(x) FROM (SELECT count(*) total, count(*) FILTER (WHERE client_project='') unattributed, count(DISTINCT user_id) users FROM token_analysis_request_summaries WHERE " + where + ") x")
    rows = query("SELECT coalesce(json_agg(x),'[]') FROM (SELECT archive_id,source_file,source_offset,user_id,event_time,user_agent,endpoint,client_project FROM token_analysis_request_summaries WHERE " + where + " AND client_project='' ORDER BY source_file,source_offset) x")
    roots = query("SELECT coalesce(json_agg(x),'[]') FROM (SELECT client_project,client_workdir,count(*) requests FROM token_analysis_request_summaries WHERE " + where + " AND client_project<>'' GROUP BY 1,2 ORDER BY 3 DESC) x")
    files = {}
    inventories = {}
    result = []
    counts = collections.Counter()
    began = time.monotonic()
    last = began
    for row in rows:
        hints = set()
        aid = row["archive_id"]
        try:
            name = row["source_file"]
            if name != os.path.basename(name):
                raise ValueError("unexpected_source_path")
            path = os.path.join(ARCHIVES, name)
            if path not in files:
                files[path] = open(path, "rb")
                s = os.stat(path)
                inventories[path] = {"size": s.st_size, "mtime_ns": s.st_mtime_ns}
            line, read_bytes = line_before(files[path], row["source_offset"])
            counts["io_bytes"] += read_bytes
            counts["request_line_bytes"] += len(line)
            event = json.loads(line)
            if event.get("archive_id") != aid or event.get("event") != "request":
                raise ValueError("identity_mismatch")
            body = event.get("body", "")
            if isinstance(body, str):
                try:
                    body = json.loads(body)
                except ValueError:
                    counts["body_json_invalid"] += 1
            for context, text in strings(body):
                for tagged in TAGGED.findall(text):
                    for m in PATH.finditer(tagged):
                        p = clean_path(m.group())
                        if p:
                            hints.add(("tagged_files", p, context))
                for m in CWD.finditer(text):
                    p = clean_path(m.group(1) or m.group(2))
                    if p:
                        hints.add(("cwd_marker", p, context))
                for m in PATH.finditer(text):
                    p = clean_path(m.group())
                    if p:
                        hints.add(("path_mention", p, context))
                for m in URL.finditer(text):
                    raw = m.group().rstrip(".,;:)/")
                    if not re.search(r"(?i)(github|gitlab|gitee|git\.|svn|192\.168\.|\.git$)", raw):
                        continue
                    if re.search(r"(?i)(sk-|ghp_|github_pat_|token|password|secret)", raw):
                        continue
                    if raw.startswith("git@"):
                        safe = raw
                    else:
                        parts = urlsplit(raw)
                        if not parts.hostname:
                            continue
                        safe = urlunsplit((parts.scheme, parts.hostname + (":" + str(parts.port) if parts.port else ""), parts.path, "", ""))
                    hints.add(("repository_url_mention", safe, context))
            counts["verified_requests"] += 1
            counts["requests_with_hints"] += bool(hints)
            counts["requests_with_tagged_files"] += any(h[0] == "tagged_files" for h in hints)
            counts["body_truncated"] += bool(event.get("body_truncated"))
            result.append({**row, "body_sha256": event.get("body_sha256"), "line_sha256": hashlib.sha256(line).hexdigest(), "hints": sorted(hints)})
        except (ValueError, OSError, TypeError) as e:
            counts["errors"] += 1
            result.append({**row, "error": type(e).__name__ + ":" + str(e)[:60]})
        if time.monotonic() - last > 20:
            print(json.dumps({"done": len(result), "total": len(rows), "seconds": round(time.monotonic() - began)}), file=sys.stderr, flush=True)
            last = time.monotonic()
    for path, before in inventories.items():
        s = os.stat(path)
        before["unchanged"] = (before["size"], before["mtime_ns"]) == (s.st_size, s.st_mtime_ns)
    for r in roots:
        r["client_workdir"] = clean_path(r["client_workdir"])
    print(json.dumps({"window": [START, END], "stats": stats, "scan": counts, "seconds": round(time.monotonic() - began), "files": inventories, "attributed_roots": roots, "requests": result}, ensure_ascii=True))


if __name__ == "__main__":
    run()
