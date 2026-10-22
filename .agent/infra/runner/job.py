"""Shared read-only inference lifecycle; receives no GitHub credentials."""

import base64
import fcntl
import gzip
import hashlib
import io
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
from contextlib import contextmanager
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "review"))
from review_policy import safe_path

HOME = Path("/var/lib/hackrnd-review")
MAX_INPUT = 18 * 1024 * 1024


def extract_snapshot(archive, destination):
    total = 0
    # Bound decompression before tar parses PAX/long-name records as well as files.
    with gzip.GzipFile(fileobj=io.BytesIO(archive)) as compressed:
        expanded = compressed.read(40 * 1024 * 1024 + 1)
    if len(expanded) > 40 * 1024 * 1024:
        raise ValueError("Expanded archive exceeds limit")
    with tarfile.open(fileobj=io.BytesIO(expanded), mode="r:") as source:
        for count, member in enumerate(source, 1):
            if count > 20000 or member.size < 0:
                raise ValueError("Archive entry limit exceeded")
            parts = Path(member.name).parts
            if not parts or parts[0] in ("/", ".."):
                raise ValueError("Invalid archive root")
            relative = "/".join(parts[1:])
            if not relative:
                continue
            path = safe_path(relative)
            target = destination / str(path)
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            elif member.isfile():
                total += member.size
                if total > 32 * 1024 * 1024:
                    raise ValueError("Expanded source exceeds 32 MiB")
                target.parent.mkdir(parents=True, exist_ok=True)
                # No symlinks, hardlinks, device files, ownership or executable modes.
                with source.extractfile(member) as src, target.open("xb") as out:
                    shutil.copyfileobj(src, out)
                target.chmod(0o400)
            else:
                raise ValueError("Non-regular archive entry is not supported")


@contextmanager
def workspace(package):
    os.umask(0o077)
    with (HOME / "review.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with tempfile.TemporaryDirectory(prefix="agent-", dir=HOME / "jobs") as temporary:
            job = Path(temporary)
            source = job / "source"
            source.mkdir()
            extract_snapshot(base64.b64decode(package["archive"], validate=True), source)
            for name, content in package["policies"].items():
                if name not in {
                    "AGENTS.md",
                    ".agent/instructions.md",
                    ".agent/decisions.md",
                    ".agent/workflows/automated-review.md",
                    ".agent/workflows/context-and-memory.md",
                    ".agent/workflows/issue-planning.md",
                    ".agent/prompts/pr-review.md",
                    ".agent/workflows/git-conventions.md",
                    ".agent/references/go-services.md",
                }:
                    raise ValueError("Unexpected trusted policy path")
                path = job / str(safe_path(name))
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content)
            (job / "context.json").write_text(json.dumps(package["context"]))
            yield job


def load_skills(job, required):
    # Native skill discovery may not emit shell-read events. Load the pinned,
    # immutable entrypoints explicitly and retain content hashes as receipts.
    texts, receipts = [], []
    for name in required:
        path = (HOME / ".agents/skills" / name / "SKILL.md").resolve(strict=True)
        if (
            not path.is_relative_to(Path("/opt/hackrnd-agent/skill-sources"))
            or path.stat().st_uid != 0
        ):
            raise ValueError("Required skill must come from root-owned pinned sources")
        data = path.read_bytes()
        if len(data) > 128 * 1024:
            raise ValueError("Skill entrypoint exceeds limit")
        texts.append("\nRequired skill: " + name + "\n" + data.decode())
        receipts.append({"name": name, "sha256": hashlib.sha256(data).hexdigest()})
    (job / "required-skills.md").write_text("\n".join(texts))
    return receipts


def infer(job, schema, prompt, ml=False, model=None, reasoning=None, question=False):
    (job / "report.schema.json").write_text(json.dumps(schema))
    command = [
        "/opt/hackrnd-agent/bin/codex",
        "exec",
        "-p",
        "workflow",
        "--skip-git-repo-check",
        "--ephemeral",
        "--json",
        "--output-schema",
        str(job / "report.schema.json"),
        "--output-last-message",
        str(job / "report.json"),
    ]
    if model is not None:
        command += ["--model", model]
    if reasoning is not None:
        command += ["-c", "model_reasoning_effort=" + json.dumps(reasoning)]
    if question:
        command += ["--disable", "shell_tool", "-c", 'default_permissions="agent-question"']
    if ml:
        if not question:
            command += ["-c", 'default_permissions="agent-review-ml"']
        paths = [p for p in (HOME / ".agents/skills").iterdir() if p.name.startswith("golang-")]
        entries = ",".join(
            "{path=" + json.dumps(str(p / "SKILL.md")) + ",enabled=false}" for p in paths
        )
        command += ["-c", "skills.config=[" + entries + "]"]
    command.append("-")
    env = {
        "HOME": str(HOME),
        "USER": "hackrnd-review",
        "LOGNAME": "hackrnd-review",
        "PATH": "/opt/hackrnd-agent/bin:/usr/bin:/bin",
        "LANG": "C.UTF-8",
    }
    with (job / "events.jsonl").open("w") as events, (job / "stderr.log").open("w") as errors:
        result = subprocess.run(
            command,
            input=prompt,
            text=True,
            cwd=job,
            env=env,
            stdout=events,
            stderr=errors,
            timeout=1050,
        )
    if result.returncode:
        shutil.copyfile(job / "stderr.log", HOME / "checks/last-workflow-error.log")
        raise RuntimeError("Codex failed; no valid report was produced")
    if question:
        for line in (job / "events.jsonl").read_text().splitlines():
            event = json.loads(line)
            item = event.get("item", {})
            if event.get("type") in ("item.started", "item.completed") and item.get("type") not in (
                "agent_message",
                "reasoning",
                "plan",
            ):
                raise ValueError("Question invoked a tool; refusing an agentic answer")
    return json.loads((job / "report.json").read_text())
