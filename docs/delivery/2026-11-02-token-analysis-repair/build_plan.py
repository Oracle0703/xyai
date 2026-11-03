"""Build a bounded repair plan from a completed private scan and read-only DB snapshot."""
import collections
import csv
import datetime
import json
import os
import re

ROOT = "/var/tmp/token-analysis-repair-20261102"
scan = json.load(open(ROOT + "/scan-result.json"))
stat = os.stat(scan["path"])
assert (stat.st_size, stat.st_mtime_ns) == (scan["size"], scan["mtime_ns"])
known = {r["archive_id"]: r for r in csv.DictReader(open(ROOT + "/summaries-before.csv"))}
events = [json.loads(line) for line in open(ROOT + "/manifest.jsonl")]
invalid = [r for r in events if "invalid" in r]
salvaged = json.load(open(ROOT + "/salvaged-events.json"))
fragments = json.load(open(ROOT + "/unrecoverable-fragments.json"))
assert {r["start"] for r in invalid} == {r["start"] for r in fragments}, "unreviewed damaged lines"
events = sorted([r for r in events if "invalid" not in r] + salvaged, key=lambda r: r["start"])
requests = [r for r in events if r.get("event") == "request"]
responses = [r for r in events if r.get("event") == "response"]
for group in (requests, responses):
    assert len({r["archive_id"] for r in group}) == len(group), "duplicate archive event needs review"


def parse_timestamp(value):
    value = value.replace("Z", "+00:00")
    value = re.sub(r"\.([0-9]+)", lambda m: "." + (m[1] + "000000")[:6], value)
    if re.search(r"[+-][0-9]{2}$", value):
        value += ":00"
    return datetime.datetime.fromisoformat(value)


def same_response(row, saved):
    if not saved or not saved["completed_at"]:
        return False
    delta = abs((parse_timestamp(row["timestamp"]) - parse_timestamp(saved["completed_at"])).total_seconds())
    return (delta <= 0.000001 and str(row.get("status", "")) == saved["response_status"] and
            row.get("response_id", "") == saved["response_id"])


request_plan = [r for r in requests if r["archive_id"] not in known or known[r["archive_id"]]["has_input"] == "f"]
response_plan = [r for r in responses if not same_response(r, known.get(r["archive_id"]))]
keys = ("archive_id", "event", "start", "length", "end", "sha256")
plan = {k: scan[k] for k in ("path", "size", "mtime_ns")}
plan["rows"] = [{k: r[k] for k in keys} for r in request_plan + response_plan]
with open(ROOT + "/plan.json", "w") as out:
    json.dump(plan, out)
request_ids = {r["archive_id"] for r in requests}
missing_requests = [r for r in requests if r["archive_id"] not in known]
missing_responses = [r for r in response_plan if r["archive_id"] not in known or not known[r["archive_id"]]["completed_at"]]
summary = {
    "scan_counts": scan["counts"],
    "salvaged_suffix_events": len(salvaged),
    "unrecoverable_request_fragments": len(fragments),
    "missing_request_summaries": len(missing_requests),
    "request_input_candidates": len(request_plan) - len(missing_requests),
    "response_candidates": len(response_plan),
    "missing_response_facts": len(missing_responses),
    "response_conflicts": len(response_plan) - len(missing_responses),
    "response_without_request_in_same_file": sum(r["archive_id"] not in request_ids for r in responses),
    "response_without_summary_in_snapshot": sum(r["archive_id"] not in known for r in responses),
    "missing_response_time_range": [min((r["timestamp"] for r in missing_responses), default=None), max((r["timestamp"] for r in missing_responses), default=None)],
    "plan_rows": len(plan["rows"]),
}
json.dump(summary, open(ROOT + "/plan-summary.json", "w"), indent=2)
print(json.dumps(summary, indent=2))
