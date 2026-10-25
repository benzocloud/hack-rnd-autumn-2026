#!/usr/bin/env python3
"""Bring up an isolated Auth/API/Core/ML + disposable Postgres for financial E2E.

Does not modify deploy/**. Uses unique project marker, loopback ports, ephemeral
PKI under artifacts/stack-<id>/, and tears down only matching marker containers.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DEFAULT_REPO = ROOT.parents[1]
REPO = DEFAULT_REPO
ARTIFACTS = ROOT / "artifacts"
# Same pin as services/api/integration/company_runtime_test.go
POSTGRES_IMAGE = (
    "postgres:18.6@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6"
)


def log(msg: str) -> None:
    print(msg, flush=True)


def run(
    cmd: list[str],
    *,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    merged = os.environ.copy()
    if env:
        merged.update(env)
    result = subprocess.run(
        cmd, cwd=str(cwd) if cwd else None, env=merged, text=True, capture_output=True
    )
    if check and result.returncode != 0:
        raise RuntimeError(
            f"command failed ({result.returncode}): {' '.join(cmd)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    return result


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def wait_tcp(host: str, port: int, timeout_s: float = 60.0) -> None:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.settimeout(1.0)
            try:
                sock.connect((host, port))
                return
            except OSError:
                time.sleep(0.2)
    raise TimeoutError(f"tcp {host}:{port} not ready in {timeout_s}s")


def build_go(service: str, packages: list[tuple[str, str]], out_dir: Path) -> dict[str, Path]:
    binaries: dict[str, Path] = {}
    service_dir = REPO / "services" / service
    for name, pkg in packages:
        target = out_dir / name
        log(f"build {service}/{pkg} -> {target.name}")
        run(
            ["go", "build", "-o", str(target), pkg],
            cwd=service_dir,
            env={"GOTOOLCHAIN": "go1.27.3", "CGO_ENABLED": "0"},
        )
        binaries[name] = target
    return binaries


def gen_pki(directory: Path) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    run(
        ["go", "run", ".", "-out", str(directory)],
        cwd=ROOT / "genpki",
        env={"GOTOOLCHAIN": "go1.27.3"},
    )


def docker_postgres(name: str) -> tuple[str, str]:
    """Start disposable postgres; return (container_name, host_port)."""
    # Remove leftovers with same marker only.
    existing = run(["docker", "ps", "-aq", "--filter", f"name=^{name}$"], check=False)
    if existing.stdout.strip():
        run(["docker", "rm", "-f", name], check=False)
    result = run(
        [
            "docker",
            "run",
            "-d",
            "--name",
            name,
            "-e",
            "POSTGRES_USER=e2e",
            "-e",
            "POSTGRES_PASSWORD=e2e-only",
            "-e",
            "POSTGRES_DB=postgres",
            "-e",
            "POSTGRES_HOST_AUTH_METHOD=trust",
            "-p",
            "127.0.0.1::5432",
            "--label",
            f"saldo.financial-e2e={name}",
            POSTGRES_IMAGE,
        ]
    )
    cid = result.stdout.strip()
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        ready = run(
            ["docker", "exec", name, "pg_isready", "-U", "e2e", "-d", "postgres"],
            check=False,
        )
        if ready.returncode == 0:
            break
        time.sleep(0.5)
    else:
        raise TimeoutError("postgres not ready")
    port = (
        run(
            ["docker", "port", name, "5432"],
        )
        .stdout.strip()
        .rsplit(":", 1)[-1]
    )
    log(f"postgres {cid[:12]} on 127.0.0.1:{port}")
    return name, port


def psql(container: str, database: str, sql: str) -> str:
    last = ""
    for _ in range(30):
        proc = subprocess.run(
            [
                "docker",
                "exec",
                "-i",
                container,
                "psql",
                "-U",
                "e2e",
                "-d",
                database,
                "-v",
                "ON_ERROR_STOP=1",
                "-At",
                "-c",
                sql,
            ],
            text=True,
            capture_output=True,
        )
        if proc.returncode == 0:
            return proc.stdout.strip()
        last = proc.stderr.strip() or proc.stdout.strip()
        time.sleep(0.5)
    raise RuntimeError(f"psql failed for {database}: {last}")


def write_state(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    path.chmod(0o600)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--keep", action="store_true", help="do not tear down on exit (daemonize children)"
    )
    parser.add_argument("--suffix", default=None, help="stable project suffix; default random")
    parser.add_argument(
        "--skip-ml",
        action="store_true",
        help="start without ML (forecast scenarios will fail explicitly)",
    )
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=None,
        help="build/run service sources from this checkout (e.g. integration merge of #48); harness stays under scripts/financial-e2e",
    )
    args = parser.parse_args()

    global REPO
    REPO = (args.repo_root or DEFAULT_REPO).resolve()
    if not (REPO / "services" / "api").is_dir():
        raise SystemExit(f"--repo-root missing services/api: {REPO}")

    suffix = args.suffix or uuid.uuid4().hex[:8]
    project = f"saldo-financial-e2e-{suffix}"
    work = ARTIFACTS / f"stack-{suffix}"
    if work.exists():
        shutil.rmtree(work)
    work.mkdir(parents=True, mode=0o700)
    bin_dir = work / "bin"
    bin_dir.mkdir()
    pki_dir = work / "pki"
    spool_dir = work / "import-spool"
    spool_dir.mkdir(mode=0o700)
    logs = work / "logs"
    logs.mkdir()

    marker = work / "MARKER"
    marker.write_text(f"project={project}\npid={os.getpid()}\n", encoding="utf-8")

    processes: list[subprocess.Popen[str]] = []
    container = None
    state_path = work / "state.json"

    def cleanup(signum=None, frame=None) -> None:  # noqa: ANN001
        log(f"cleanup project={project}")
        for proc in reversed(processes):
            if proc.poll() is None:
                proc.send_signal(signal.SIGTERM)
        for proc in processes:
            try:
                proc.wait(timeout=8)
            except subprocess.TimeoutExpired:
                proc.kill()
        if container:
            # Only remove if marker label matches.
            inspect = run(
                [
                    "docker",
                    "inspect",
                    "-f",
                    '{{index .Config.Labels "saldo.financial-e2e"}}',
                    container,
                ],
                check=False,
            )
            if inspect.stdout.strip() == container:
                run(["docker", "rm", "-f", container], check=False)
                log(f"removed container {container}")
            else:
                log(f"refusing teardown: marker mismatch for {container!r}")
        if signum is not None:
            sys.exit(128 + (signum if isinstance(signum, int) else 0))

    if not args.keep:
        signal.signal(signal.SIGINT, cleanup)
        signal.signal(signal.SIGTERM, cleanup)

    try:
        if shutil.which("docker") is None:
            raise RuntimeError("docker required")
        gen_pki(pki_dir)
        auth_bins = build_go(
            "auth",
            [("auth", "./cmd/auth"), ("auth-migrate", "./cmd/migrate")],
            bin_dir,
        )
        core_bins = build_go("core", [("core", "./cmd/core")], bin_dir)
        # Core schema: use goose if available via migrations + docker port
        api_bins = build_go("api", [("api", "./cmd/api")], bin_dir)

        container, pg_port = docker_postgres(project + "-pg")
        psql(container, "postgres", "CREATE DATABASE auth_e2e;")
        psql(container, "postgres", "CREATE DATABASE core_e2e;")
        psql(
            container,
            "auth_e2e",
            "ALTER DATABASE auth_e2e SET saldo.test_database = 'saldo-financial-e2e-auth';",
        )
        psql(
            container,
            "core_e2e",
            "ALTER DATABASE core_e2e SET saldo.test_database = 'saldo-financial-e2e-core';",
        )

        auth_url = f"postgres://e2e:e2e-only@127.0.0.1:{pg_port}/auth_e2e?sslmode=disable"
        run(
            [str(auth_bins["auth-migrate"])],
            cwd=REPO / "services" / "auth",
            env={
                "AUTH_ENV": "local",
                "AUTH_DATABASE_URL": auth_url,
                "AUTH_QUERY_TIMEOUT": "5s",
                "AUTH_STARTUP_TIMEOUT": "5s",
            },
        )

        # Apply core migrations with goose
        core_url = f"postgres://e2e:e2e-only@127.0.0.1:{pg_port}/core_e2e?sslmode=disable"
        goose = shutil.which("goose")
        if goose is None:
            raise RuntimeError("goose not found on PATH; required for Core migrations")
        run(
            [goose, "-dir", "migrations", "postgres", core_url, "up"],
            cwd=REPO / "services" / "core",
        )

        auth_http = free_port()
        auth_grpc = free_port()
        core_grpc = free_port()
        ml_grpc = free_port()
        api_port = free_port()
        origin = f"https://127.0.0.1:{api_port}"
        hmac_key = base64.urlsafe_b64encode(secrets.token_bytes(32)).decode().rstrip("=")
        # Exact allowlist only (no wildcards). Pre-generate a pool so the HTTP
        # harness can pick a fresh address per run without restarting Auth.
        demo_emails = [f"owner-{suffix}-{i:02d}@financial-e2e.test" for i in range(1, 21)]
        demo_email = demo_emails[0]

        def start(
            name: str,
            binary: Path,
            cwd: Path,
            env: dict[str, str],
            ready: tuple[str, int] | None = None,
        ) -> None:
            log_path = logs / f"{name}.log"
            handle = open(log_path, "w", encoding="utf-8")  # noqa: SIM115
            merged = os.environ.copy()
            merged.update(env)
            proc = subprocess.Popen(
                [str(binary)],
                cwd=str(cwd),
                env=merged,
                stdout=handle,
                stderr=subprocess.STDOUT,
                text=True,
            )
            processes.append(proc)
            log(f"started {name} pid={proc.pid} log={log_path}")
            if ready:
                wait_tcp(*ready, timeout_s=45)
            time.sleep(0.3)
            if proc.poll() is not None:
                handle.flush()
                raise RuntimeError(f"{name} exited early code={proc.returncode}; see {log_path}")

        # HTTP harness needs TLS1.3 (Homebrew Python). ML needs grpcio (CLT python3).
        ml_python = os.environ.get("SALDO_E2E_ML_PYTHON", "")
        if not args.skip_ml:
            candidates = []
            if ml_python:
                candidates.append(Path(ml_python))
            candidates.extend(
                [
                    Path("/usr/bin/python3"),
                    Path("/Library/Developer/CommandLineTools/usr/bin/python3"),
                    Path("/tmp/saldo-csv-venv/bin/python"),
                    Path.home() / ".venv/saldo-ml/bin/python",
                ]
            )
            ml_python = ""
            for candidate in candidates:
                if not candidate.exists():
                    continue
                probe = subprocess.run(
                    [str(candidate), "-c", "import grpc"],
                    capture_output=True,
                    text=True,
                )
                if probe.returncode == 0:
                    ml_python = str(candidate)
                    break
            if not ml_python:
                raise RuntimeError("no Python with grpcio for ML; set SALDO_E2E_ML_PYTHON")
            ml_script = REPO / "services" / "ml" / "run_server.py"
            log_path = logs / "ml.log"
            handle = open(log_path, "w", encoding="utf-8")  # noqa: SIM115
            ml_env = os.environ.copy()
            gen = ARTIFACTS / "ml-gen"
            ml_env["PYTHONPATH"] = (
                str(gen)
                + os.pathsep
                + str(REPO / "services" / "ml")
                + (os.pathsep + ml_env["PYTHONPATH"] if ml_env.get("PYTHONPATH") else "")
            )
            if not (gen / "ml").exists():
                raise RuntimeError(
                    f"missing generated ML bindings at {gen}/ml; copy/generate forecast_pb2 locally"
                )
            proc = subprocess.Popen(
                [
                    ml_python,
                    "-B",
                    str(ml_script),
                    "--host",
                    "127.0.0.1",
                    "--port",
                    str(ml_grpc),
                    "--certificate",
                    str(pki_dir / "ml.pem"),
                    "--private-key",
                    str(pki_dir / "ml-key.pem"),
                    "--client-ca",
                    str(pki_dir / "ca.pem"),
                    "--allowed-peer",
                    "spiffe://saldo.local/core",
                ],
                cwd=str(REPO / "services" / "ml"),
                env=ml_env,
                stdout=handle,
                stderr=subprocess.STDOUT,
                text=True,
            )
            processes.append(proc)
            log(f"started ml pid={proc.pid}")
            wait_tcp("127.0.0.1", ml_grpc, timeout_s=45)

        core_env = {
            "CORE_POSTGRES_HOST": "127.0.0.1",
            "CORE_POSTGRES_PORT": pg_port,
            "CORE_POSTGRES_USER": "e2e",
            "CORE_POSTGRES_PASSWORD": "e2e-only",
            "CORE_POSTGRES_DATABASE": "core_e2e",
            "CORE_POSTGRES_SSL_MODE": "disable",
            "CORE_GRPC_ADDRESS": f"127.0.0.1:{core_grpc}",
            "CORE_AUTH_ADDRESS": f"127.0.0.1:{auth_grpc}",
            "CORE_AUTH_SERVER_NAME": "localhost",
            "CORE_AUTH_TIMEOUT": "3s",
            "CORE_MTLS_CA_FILE": str(pki_dir / "ca.pem"),
            "CORE_MTLS_CERT_FILE": str(pki_dir / "core.pem"),
            "CORE_MTLS_KEY_FILE": str(pki_dir / "core-key.pem"),
        }
        if not args.skip_ml:
            core_env["CORE_ML_ADDRESS"] = f"127.0.0.1:{ml_grpc}"
            core_env["CORE_ML_SERVER_NAME"] = "saldo-ml"

        start(
            "core",
            core_bins["core"],
            REPO / "services" / "core",
            core_env,
            ("127.0.0.1", core_grpc),
        )

        auth_env = {
            "AUTH_ENV": "local",
            "AUTH_DATABASE_URL": auth_url,
            "AUTH_HTTP_ADDRESS": f"127.0.0.1:{auth_http}",
            "AUTH_GRPC_ADDRESS": f"127.0.0.1:{auth_grpc}",
            "AUTH_GRPC_CERT_FILE": str(pki_dir / "auth.pem"),
            "AUTH_GRPC_KEY_FILE": str(pki_dir / "auth-key.pem"),
            "AUTH_GRPC_CA_FILE": str(pki_dir / "ca.pem"),
            "AUTH_CODE_HMAC_KEY": hmac_key,
            "AUTH_CSRF_HMAC_KEY": hmac_key,
            "AUTH_CORE_ADDRESS": f"127.0.0.1:{core_grpc}",
            "AUTH_CORE_SERVER_NAME": "localhost",
            "AUTH_CORE_TIMEOUT": "2s",
            "AUTH_QUERY_TIMEOUT": "5s",
            "AUTH_STARTUP_TIMEOUT": "5s",
            "AUTH_DEMO_EMAIL_VERIFICATION": "true",
            "AUTH_DEMO_TEST_EMAILS": ",".join(demo_emails),
        }
        start(
            "auth",
            auth_bins["auth"],
            REPO / "services" / "auth",
            auth_env,
            ("127.0.0.1", auth_http),
        )

        api_env = {
            "API_ADDRESS": f"127.0.0.1:{api_port}",
            "API_ORIGIN": origin,
            "API_AUTH_ADDRESS": f"127.0.0.1:{auth_grpc}",
            "API_AUTH_CA": str(pki_dir / "ca.pem"),
            "API_AUTH_CERT": str(pki_dir / "api.pem"),
            "API_AUTH_KEY": str(pki_dir / "api-key.pem"),
            "API_HTTP_CERT": str(pki_dir / "https.pem"),
            "API_HTTP_KEY": str(pki_dir / "https-key.pem"),
            "API_CORE_ADDRESS": f"127.0.0.1:{core_grpc}",
            "API_CORE_SERVER_NAME": "localhost",
            "API_CORE_CA": str(pki_dir / "ca.pem"),
            "API_CORE_CERT": str(pki_dir / "api.pem"),
            "API_CORE_KEY": str(pki_dir / "api-key.pem"),
            "API_RPC_TIMEOUT": "5s",
            "API_SLOW_TIMEOUT": "30s",
            "API_IMPORT_SPOOL_DIR": str(spool_dir),
        }
        start("api", api_bins["api"], REPO / "services" / "api", api_env, ("127.0.0.1", api_port))

        # Trust ephemeral HTTPS CA for harness via REQUESTS? our client uses ssl default —
        # export SSL_CERT_FILE tip in state; run_http needs custom CA. Record ca path.
        state = {
            "project": project,
            "suffix": suffix,
            "marker": str(marker),
            "work_dir": str(work),
            "api_origin": origin,
            "ca_file": str(pki_dir / "ca.pem"),
            "demo_email": demo_email,
            "demo_emails": demo_emails,
            "demo_email_profile": "AUTH_DEMO_EMAIL_VERIFICATION=true allowlisted exact addresses",
            "verify_code": "000000",
            "postgres_container": container,
            "postgres_port": pg_port,
            "ports": {
                "api": api_port,
                "auth_http": auth_http,
                "auth_grpc": auth_grpc,
                "core_grpc": core_grpc,
                "ml_grpc": None if args.skip_ml else ml_grpc,
            },
            "source_sha": run(["git", "rev-parse", "HEAD"], cwd=REPO).stdout.strip(),
            "ml_enabled": not args.skip_ml,
            "ml_python": ml_python if not args.skip_ml else None,
        }
        write_state(state_path, state)
        export_path = work / "env.sh"
        export_path.write_text(
            "\n".join(
                [
                    f"export SALDO_E2E_API_ORIGIN={origin}",
                    f"export SALDO_E2E_CA_FILE={pki_dir / 'ca.pem'}",
                    f"export SALDO_E2E_DEMO_EMAIL={demo_email}",
                    f"export SALDO_E2E_PROJECT={project}",
                    f"export SALDO_E2E_STACK_STATE={state_path}",
                    f"export SSL_CERT_FILE={pki_dir / 'ca.pem'}",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        log(
            json.dumps(
                {
                    "ok": True,
                    "state": str(state_path),
                    "api_origin": origin,
                    "demo_email": demo_email,
                },
                ensure_ascii=False,
            )
        )
        log(f"source env: . {export_path}")

        if args.keep:
            # Detach: leave processes running; caller must down_stack.py
            log("keep=1; processes left running")
            return 0

        log("stack up; Ctrl+C to tear down")
        while True:
            for proc in processes:
                code = proc.poll()
                if code is not None:
                    raise RuntimeError(f"process exited early code={code}")
            time.sleep(1)
    except Exception as exc:  # noqa: BLE001
        log(f"ERROR: {exc}")
        cleanup()
        write_state(
            ARTIFACTS / "stack-last-error.json",
            {"ts": time.time(), "error": str(exc), "project": project, "work": str(work)},
        )
        return 1


if __name__ == "__main__":
    sys.exit(main())
