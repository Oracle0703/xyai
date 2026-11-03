"""Read-only full metadata reconciliation after replay; writes only local evidence."""
import collections
import csv
import io
import json
import re
import subprocess

ROOT = "/var/tmp/token-analysis-repair-20261102"


def query_csv(sql):
    result = subprocess.run(
        ["sudo", "-u", "postgres", "psql", "-X", "-q", "-d", "sub2api", "-c",
         "BEGIN READ ONLY; SET LOCAL statement_timeout='45s'; COPY (" + sql +
         ") TO STDOUT WITH CSV HEADER; COMMIT;"], capture_output=True, text=True, check=True)
    return list(csv.DictReader(io.StringIO(result.stdout)))


summaries = query_csv("""SELECT s.archive_id,s.source_file,s.source_offset,s.completed_at,
 s.response_status,s.response_id,s.body_sha256,s.usage_log_id,s.usage_match_method,
 (u.archive_id IS NOT NULL) AS has_input FROM token_analysis_request_summaries s
 LEFT JOIN token_analysis_user_inputs u ON u.archive_id=s.archive_id
 WHERE s.event_time>='2026-10-30T00:00:00+08:00' AND s.event_time<'2026-11-02T00:00:00+08:00'""")
known = {r["archive_id"]: r for r in summaries}
pending = {r["archive_id"]: r for r in query_csv("SELECT archive_id,completed_at,response_status,response_id FROM token_analysis_pending_responses")}
events = [json.loads(line) for line in open(ROOT + "/manifest.jsonl")]
events += json.load(open(ROOT + "/salvaged-events.json"))
requests = [r for r in events if r.get("event") == "request"]
responses = [r for r in events if r.get("event") == "response"]
missing_requests = [r for r in requests if r["archive_id"] not in known]
source_conflicts = [r for r in requests if r["archive_id"] in known and (
    known[r["archive_id"]]["source_file"] != "2026-10-31.jsonl" or
    int(known[r["archive_id"]]["source_offset"]) != r["end"] or
    known[r["archive_id"]]["body_sha256"] != r.get("body_sha256", ""))]
missing_responses = [r for r in responses if not known.get(r["archive_id"], {}).get("completed_at")
                     and r["archive_id"] not in pending]
plan = json.load(open(ROOT + "/plan.json"))
new_ids = {r["archive_id"] for r in plan["rows"] if r["event"] == "request"}
repaired = [known[i] for i in new_ids if i in known]
summary = {
    "readable_requests": len(requests), "readable_responses": len(responses),
    "missing_request_summaries": len(missing_requests), "source_position_or_hash_conflicts": len(source_conflicts),
    "missing_response_facts": len(missing_responses),
    "new_summaries_present": len(repaired),
    "new_summaries_with_input": sum(r["has_input"] == "t" for r in repaired),
    "new_summaries_linked": sum(bool(r["usage_log_id"]) for r in repaired),
    "new_summary_link_methods": dict(collections.Counter(r["usage_match_method"] for r in repaired)),
    "new_summaries_without_completion": sum(not r["completed_at"] for r in repaired),
    "readable_responses_only_in_pending": sum(not known.get(r["archive_id"], {}).get("completed_at") and r["archive_id"] in pending for r in responses),
}
json.dump(summary, open(ROOT + "/verification.json", "w"), indent=2)
print(json.dumps(summary, indent=2))
assert not missing_requests and not source_conflicts and not missing_responses
