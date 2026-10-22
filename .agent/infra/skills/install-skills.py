#!/usr/bin/env python3
"""Fetch pinned skill files only; never execute upstream installers or hooks."""
import json
import os
from pathlib import Path
import pwd
import subprocess


def main():
    if os.geteuid() != 0:
        raise SystemExit("Requires root")
    specs = json.loads(Path(__file__).with_name("skills.json").read_text())
    root = Path("/opt/hackrnd-agent/skill-sources")
    root.mkdir(parents=True, exist_ok=True)
    links = Path("/var/lib/hackrnd-review/.agents/skills")
    links.mkdir(parents=True, mode=0o755, exist_ok=True)
    for name, spec in specs.items():
        source = root / name
        if not source.exists():
            subprocess.run(["git", "init", str(source)], check=True, capture_output=True)
            subprocess.run(["git", "-C", str(source), "remote", "add", "origin", spec["repository"]], check=True)
        subprocess.run(["git", "-C", str(source), "fetch", "--depth=1", "origin", spec["commit"]], check=True, timeout=120)
        subprocess.run(["git", "-C", str(source), "checkout", "--detach", "FETCH_HEAD"], check=True)
        actual = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
        if actual != spec["commit"]:
            raise RuntimeError(f"Skill {name} differs from pinned commit")
        installed = []
        for item in spec["paths"]:
            candidate = source / item
            directories = [candidate] if (candidate / "SKILL.md").is_file() else sorted(candidate.iterdir())
            for folder in directories:
                if not (folder / "SKILL.md").is_file():
                    continue
                link = links / folder.name
                if link.is_symlink() and link.resolve() == folder:
                    pass
                elif link.exists() or link.is_symlink():
                    raise RuntimeError(f"Existing skill {link} differs; refusing to overwrite")
                else:
                    link.symlink_to(folder, target_is_directory=True)
                installed.append(folder.name)
        print(f"{name}: {actual}; installed {len(installed)} skills", flush=True)
    user = pwd.getpwnam("hackrnd-review")
    os.chown(links.parent, user.pw_uid, user.pw_gid)
    os.chown(links, user.pw_uid, user.pw_gid)
    print("Skill files installed. Invocation still needs a real Codex review.", flush=True)


if __name__ == "__main__":
    main()
