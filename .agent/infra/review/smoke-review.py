#!/usr/bin/env python3
"""Trusted fixture review using the deployed default config and optional memory."""

import json
import os
import subprocess
from pathlib import Path


def main():
    if os.geteuid() != 0:
        raise SystemExit("Requires root to prepare a root-owned, read-only fixture")
    home = Path("/var/lib/hackrnd-review")
    fixture = home / "jobs/skill-fixture"
    fixture.mkdir(mode=0o755, exist_ok=True)
    files = {
        "README.md": "Trusted synthetic preflight, not a PR. First returns the first integer, "
        "or zero for empty input. Used once per small request.\n",
        "first.go": "package fixture\n\nfunc First(values []int) int {\n\treturn values[0]\n}\n",
    }
    for name, content in files.items():
        target = fixture / name
        if target.exists() and target.read_text() != content:
            raise RuntimeError(f"Refusing to overwrite changed fixture {target}")
        target.write_text(content)
        target.chmod(0o644)
    prompt = """This is a trusted infrastructure skill preflight, not a PR review.
Use the deployed default Codex configuration. GBrain rollout is deferred under
ADR-010; it is disabled and unavailable. Continue without shared memory.
Include exactly this status line: GBrain: unavailable (deferred).
Apply $caveman-review, $ponytail-review, and $golang-how-to to first.go and
README.md here. Read each installed SKILL.md and relevant Go secondary skills.
Do not modify files, install tools, delegate, contact GitHub, or publish anything.
Report one concrete bug with file and line; deduplicate the skills' findings.
List skills actually read and applied, checks actually run, and unavailable tools.
State that GBrain and end-to-end PR review were NOT tested. Do not claim a PASS.
"""
    command = [
        "runuser",
        "-u",
        "hackrnd-review",
        "--",
        "/opt/hackrnd-agent/bin/codex",
        "exec",
        "--skip-git-repo-check",
        "--ephemeral",
        "--sandbox",
        "read-only",
        "--json",
        "--output-last-message",
        str(home / "checks/skills-smoke.txt"),
        "-",
    ]
    with (
        (home / "checks/skills-smoke.jsonl").open("w") as output,
        (home / "checks/skills-smoke.log").open("w") as errors,
    ):
        result = subprocess.run(
            command,
            input=prompt,
            text=True,
            cwd=fixture,
            stdout=output,
            stderr=errors,
            timeout=300,
        )
    print(f"Fixture preflight exit: {result.returncode}; evidence: {home}/checks/skills-smoke.*")
    if result.returncode:
        raise SystemExit(result.returncode)
    completed_reads = []
    for line in (home / "checks/skills-smoke.jsonl").read_text().splitlines():
        event = json.loads(line)
        item = event.get("item", {})
        if (
            event.get("type") == "item.completed"
            and item.get("type") == "command_execution"
            and item.get("exit_code") == 0
        ):
            completed_reads.append(item.get("aggregated_output", ""))
    for name in ["caveman-review", "ponytail-review", "golang-how-to"]:
        if not any(f"name: {name}" in output for output in completed_reads):
            raise SystemExit(
                f"No successful tool evidence for reading {name}; inspect diagnostic logs"
            )
    if "GBrain: unavailable (deferred)." not in (home / "checks/skills-smoke.txt").read_text():
        raise SystemExit("Review did not disclose unavailable shared memory")
    print(
        "All three skill instructions were read successfully. Inspect findings and limitations in the final text."
    )


if __name__ == "__main__":
    main()
