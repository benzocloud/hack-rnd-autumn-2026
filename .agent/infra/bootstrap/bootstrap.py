#!/usr/bin/env python3
"""Install pinned runtimes on the agent host. Does not operate Docker or Fadwix."""

import argparse
import hashlib
import json
import os
import platform
import pwd
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile
from pathlib import Path

ROOT = Path("/opt/hackrnd-agent")
HERE = Path(__file__).resolve().parent


def run(*args):
    subprocess.run(args, check=True)


def directory(path, mode=0o755, owner=None):
    path.mkdir(parents=True, exist_ok=True)
    path.chmod(mode)
    if owner:
        account = pwd.getpwnam(owner)
        os.chown(path, account.pw_uid, account.pw_gid)


def account(name, home):
    try:
        current = pwd.getpwnam(name)
        if current.pw_dir != str(home):
            raise RuntimeError(f"Existing {name} has unexpected home; refusing to adopt")
    except KeyError:
        run(
            "useradd",
            "--system",
            "--user-group",
            "--home-dir",
            str(home),
            "--shell",
            "/usr/sbin/nologin",
            name,
        )
    directory(home, 0o700, name)


def install_binary(name, spec, verify_version=True):
    destination = ROOT / "releases" / f"{name}-{spec['version']}" / name
    receipt = destination.with_suffix(".sha256")
    if destination.exists() and receipt.exists():
        digest = hashlib.sha256(destination.read_bytes()).hexdigest()
        if digest != receipt.read_text().strip():
            raise RuntimeError(f"Installed {name} was changed; refusing to overwrite")
        print(f"{name} {spec['version']} already installed", flush=True)
    else:
        directory(destination.parent)
        with tempfile.TemporaryDirectory(prefix="hackrnd-download-") as work:
            archive = Path(work) / "download"
            request = urllib.request.Request(spec["url"], headers={"User-Agent": "hackrnd-infra"})
            digest = hashlib.sha256()
            print(f"Downloading {name} {spec['version']}", flush=True)
            with (
                urllib.request.urlopen(request, timeout=120) as response,
                archive.open("wb") as out,
            ):
                while chunk := response.read(1024 * 1024):
                    digest.update(chunk)
                    out.write(chunk)
            if digest.hexdigest() != spec["sha256"]:
                raise RuntimeError(f"Checksum mismatch for {name}")
            staged = Path(work) / name
            if spec["url"].endswith(".zip"):
                with (
                    zipfile.ZipFile(archive) as zipped,
                    staged.open("wb") as out,
                    zipped.open(spec["member"]) as src,
                ):
                    shutil.copyfileobj(src, out)
            elif "member" in spec:
                with tarfile.open(archive, "r:gz") as tar:
                    member = tar.getmember(spec["member"])
                    if not member.isfile():
                        raise RuntimeError("Binary archive member is not a regular file")
                    with tar.extractfile(member) as src, staged.open("wb") as out:
                        shutil.copyfileobj(src, out)
            else:
                archive.rename(staged)
            shutil.copyfile(staged, destination)
            destination.chmod(0o755)
            receipt.write_text(hashlib.sha256(destination.read_bytes()).hexdigest() + "\n")
            receipt.chmod(0o644)
    link = ROOT / "bin" / name
    if link.exists() and not link.is_symlink():
        raise RuntimeError(f"Refusing to replace unmanaged file {link}")
    temporary = link.with_suffix(".new")
    temporary.unlink(missing_ok=True)
    temporary.symlink_to(destination)
    temporary.replace(link)
    if verify_version:
        run(str(link), "--version")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--with-gbrain", action="store_true", help="Install deferred Bun/GBrain runtimes explicitly"
    )
    options = parser.parse_args()
    if os.geteuid() != 0 or platform.system() != "Linux" or platform.machine() != "x86_64":
        raise SystemExit("Requires root on Linux x86_64")
    directory(ROOT)
    directory(ROOT / "bin")
    account("hackrnd-gbrain", Path("/var/lib/hackrnd-gbrain"))
    account("hackrnd-review", Path("/var/lib/hackrnd-review"))
    directory(Path("/var/backups/hackrnd-gbrain"), 0o700, "hackrnd-gbrain")
    directory(Path("/etc/hackrnd-agent"), 0o700)
    specs = json.loads((HERE / "versions.json").read_text())
    install_binary("codex", specs["codex"])
    install_binary("codex-code-mode-host", specs["codex-code-mode-host"], verify_version=False)
    companion = ROOT / "releases" / f"codex-{specs['codex']['version']}" / "codex-code-mode-host"
    if not companion.exists():
        companion.symlink_to(ROOT / "bin/codex-code-mode-host")
    install_binary("bwrap", specs["bwrap"])
    sandbox_directory = ROOT / "releases" / f"bwrap-{specs['bwrap']['version']}"
    os.chown(sandbox_directory, 0, pwd.getpwnam("hackrnd-review").pw_gid)
    sandbox_directory.chmod(0o750)
    resources = ROOT / "releases" / f"codex-{specs['codex']['version']}" / "codex-resources"
    directory(resources)
    bwrap_link = resources / "bwrap"
    if not bwrap_link.exists():
        bwrap_link.symlink_to(ROOT / "bin/bwrap")
    shutil.copyfile(HERE / "versions.json", ROOT / "versions.json")
    if not options.with_gbrain:
        print(
            "Codex runtimes installed. GBrain deferred (ADR-010); no CPU change required.",
            flush=True,
        )
        return
    if "sse4_2" not in Path("/proc/cpuinfo").read_text().split():
        raise SystemExit(
            "GBrain blocked: Bun requires SSE4.2; change the VM CPU model or use another host. Codex is installed."
        )
    install_binary("bun", specs["bun"])
    source = ROOT / "releases" / f"gbrain-{specs['gbrain']['version']}-source"
    if not source.exists():
        run("git", "init", str(source))
        run(
            "git",
            "-C",
            str(source),
            "remote",
            "add",
            "origin",
            "https://github.com/garrytan/gbrain.git",
        )
    marker = source / ".hackrnd-installed"
    if not marker.exists():
        run("chown", "-R", "root:root", str(source))
        run("git", "-C", str(source), "fetch", "--depth=1", "origin", specs["gbrain"]["commit"])
        run("git", "-C", str(source), "checkout", "--detach", "FETCH_HEAD")
        actual = subprocess.check_output(
            ["git", "-C", str(source), "rev-parse", "HEAD"], text=True
        ).strip()
        if actual != specs["gbrain"]["commit"]:
            raise RuntimeError("GBrain checkout differs from pinned commit")
        run("chown", "-R", "hackrnd-gbrain:hackrnd-gbrain", str(source))
        try:
            subprocess.run(
                [
                    "runuser",
                    "-u",
                    "hackrnd-gbrain",
                    "--",
                    str(ROOT / "bin/bun"),
                    "install",
                    "--production",
                    "--frozen-lockfile",
                    "--ignore-scripts",
                ],
                cwd=source,
                check=True,
                timeout=300,
            )
        finally:
            run("chown", "-R", "root:root", str(source))
        marker.write_text(actual + "\n")
    wrapper = ROOT / "bin/gbrain"
    wrapper.unlink(missing_ok=True)
    wrapper.write_text(
        "#!/bin/sh\nexec /opt/hackrnd-agent/bin/bun --no-env-file "
        + str(source / "src/cli.ts")
        + ' "$@"\n'
    )
    wrapper.chmod(0o755)
    run(str(wrapper), "--version")
    print("Runtimes installed. No existing services or containers changed.", flush=True)


if __name__ == "__main__":
    main()
