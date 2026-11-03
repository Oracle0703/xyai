"""Run the isolated helper with the running service's config environment.

Credentials stay on the server and are never printed. Default is SQL read-only;
--apply is the explicit mutation switch after review of plan and audit output.
"""
import os
import subprocess
import sys

ROOT = "/var/tmp/token-analysis-repair-20261102"
assert sys.argv[1:] in ([], ["--apply"])
pid = subprocess.check_output(["systemctl", "show", "sub2api", "-p", "MainPID", "--value"], text=True).strip()
env = dict(item.split(b"=", 1) for item in open("/proc/" + pid + "/environ", "rb").read().split(b"\0") if b"=" in item)
env = {key.decode(): value.decode() for key, value in env.items()}
args = [ROOT + "/token-repair"]
if sys.argv[1:]:
    args.append("-apply")
result = subprocess.run(args, env=env, cwd="/opt/sub2api/backend")
raise SystemExit(result.returncode)
