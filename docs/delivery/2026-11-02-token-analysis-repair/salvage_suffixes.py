"""Recover only independently valid JSON objects after damaged request fragments.

Never changes the original file. A valid suffix gets its actual original byte
offset and SHA-256; the truncated prefix remains unresolved evidence.
"""
import hashlib
import json
import re

ROOT = "/var/tmp/token-analysis-repair-20261102"
scan = json.load(open(ROOT + "/scan-result.json"))
bad = [r for r in map(json.loads, open(ROOT + "/manifest.jsonl")) if "invalid" in r]
recovered, fragments = [], []
with open(scan["path"], "rb") as source:
    for row in bad:
        source.seek(row["start"])
        raw = source.read(row["length"])
        assert hashlib.sha256(raw).hexdigest() == row["sha256"]
        starts = [m.start() for m in re.finditer(rb'\{"archive_id":', raw)]
        suffixes = []
        for pos in starts[1:]:
            try:
                event = json.loads(raw[pos:])
            except (ValueError, UnicodeError):
                continue
            assert event.get("event") in ("request", "response")
            item = {
                "line": row["line"], "start": row["start"] + pos,
                "end": row["end"], "length": len(raw) - pos,
                "sha256": hashlib.sha256(raw[pos:]).hexdigest(), "salvaged_suffix": True,
            }
            for key in ("archive_id", "event", "timestamp", "body_sha256", "body_truncated",
                        "client_request_id", "status", "response_id"):
                if key in event:
                    item[key] = event[key]
            suffixes.append(item)
        assert len(suffixes) == 1, "ambiguous damaged line requires manual review"
        recovered.extend(suffixes)
        prefix = raw[:suffixes[0]["start"] - row["start"]]
        header = {}
        for key in ("archive_id", "event", "timestamp"):
            match = re.search(('"' + key + '"\\s*:\\s*"([^"]*)"').encode(), prefix[:1024])
            if match:
                header[key] = match.group(1).decode()
        fragments.append(dict(row, fragment=header))
json.dump(recovered, open(ROOT + "/salvaged-events.json", "w"))
json.dump(fragments, open(ROOT + "/unrecoverable-fragments.json", "w"))
print(json.dumps({"valid_suffixes": len(recovered), "unresolved_fragments": len(fragments)}))
