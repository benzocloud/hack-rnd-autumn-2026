"""GitHub-hosted API/SSH transport; never installed in model jobs."""
import json
import os
from pathlib import Path
import subprocess
import urllib.request

REPOSITORY = "benzocloud/hack-rnd-autumn-2026"

class GitHub:
    def __init__(self):
        self.base = "https://api.github.com/repos/" + REPOSITORY
        self.token = os.environ["GH_TOKEN"]

    def request(self, path, body=None, raw=False, accept="application/vnd.github+json", method=None):
        req = urllib.request.Request(self.base + path, data=None if body is None else json.dumps(body).encode(),
                                     method=method, headers={"Authorization": "Bearer " + self.token, "Accept": accept,
                                              "User-Agent": "hackrnd-agent-review", "X-GitHub-Api-Version": "2022-11-28"})
        with urllib.request.urlopen(req, timeout=60) as response:
            data = response.read(12 * 1024 * 1024 + 1)
        if len(data) > 12 * 1024 * 1024:
            raise ValueError("GitHub response exceeds review size limit")
        return data if raw else json.loads(data)

    def pages(self, path, key=None):
        result = []
        for page in range(1, 11):
            data = self.request(path + ("&" if "?" in path else "?") + f"per_page=100&page={page}")
            items = data[key] if key else data
            result.extend(items)
            if len(items) < 100:
                return result
        raise ValueError("Context pagination limit exceeded; refusing a partial review")


def event():
    if os.environ["GITHUB_REPOSITORY"] != REPOSITORY:
        raise ValueError("Unexpected repository")
    return json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())


def invoke_json(command):
    if command not in {"agent-review", "agent-scope", "agent-create-plan", "agent-discuss-plan"}:
        raise ValueError("Unknown fixed command")
    key = Path(os.environ["RUNNER_TEMP"]) / "agent-key"
    known = Path(os.environ["RUNNER_TEMP"]) / "agent-known-hosts"
    try:
        key.touch(mode=0o600, exist_ok=False)
        key.write_text(os.environ["AGENT_REVIEW_SSH_KEY"].rstrip() + "\n")
        known.write_text(os.environ["AGENT_REVIEW_KNOWN_HOSTS"].rstrip() + "\n")
        args = ["ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=15",
                "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + str(known),
                "-i", str(key), "hackrnd-review@runner.brokebyte.me", command]
        result = subprocess.run(args, input=Path("request.json").read_bytes(), capture_output=True, timeout=1200)
        if result.returncode:
            raise RuntimeError("Runner failed: " + result.stderr.decode(errors="replace")[-1500:])
        return json.loads(result.stdout)
    finally:
        key.unlink(missing_ok=True)
        known.unlink(missing_ok=True)
        Path("request.json").unlink(missing_ok=True)
