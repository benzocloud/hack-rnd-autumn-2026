#!/usr/bin/env python3
"""Two real loopback processes: HTTPS API -> mTLS gRPC test Auth. No external services."""

import argparse
import http.cookiejar
import json
import os
import signal
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def stop(process):
    if process is not None and process.poll() is None:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=7)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=2)
            raise AssertionError("process did not shut down gracefully") from None


def smoke(output):
    root = Path(__file__).resolve().parents[1]
    with tempfile.TemporaryDirectory(prefix="saldo-api-smoke-") as temporary:
        temp = Path(temporary)
        for name in ("api", "testauth"):
            subprocess.run(
                ["go", "build", "-o", str(temp / name), f"./cmd/{name}"], cwd=root, check=True
            )
        api_port, auth_port = port(), port()
        origin = f"https://127.0.0.1:{api_port}"
        pki = temp / "pki"
        processes = []
        results = []
        with (temp / "api.log").open("w+") as api_log, (temp / "auth.log").open("w+") as auth_log:
            try:
                auth = subprocess.Popen(
                    [
                        str(temp / "testauth"),
                        "-address",
                        f"127.0.0.1:{auth_port}",
                        "-pki",
                        str(pki),
                    ],
                    stdout=auth_log,
                    stderr=auth_log,
                )
                processes.append(auth)
                deadline = time.monotonic() + 10
                while not (pki / "https-key.pem").exists():
                    assert auth.poll() is None and time.monotonic() < deadline, (
                        "test Auth startup failed"
                    )
                    time.sleep(0.02)
                env = os.environ | {
                    "API_ADDRESS": f"127.0.0.1:{api_port}",
                    "API_ORIGIN": origin,
                    "API_AUTH_ADDRESS": f"127.0.0.1:{auth_port}",
                    "API_AUTH_CA": str(pki / "ca.pem"),
                    "API_AUTH_CERT": str(pki / "api.pem"),
                    "API_AUTH_KEY": str(pki / "api-key.pem"),
                    "API_HTTP_CERT": str(pki / "https.pem"),
                    "API_HTTP_KEY": str(pki / "https-key.pem"),
                }
                api = subprocess.Popen([str(temp / "api")], env=env, stdout=api_log, stderr=api_log)
                processes.append(api)
                tls = ssl.create_default_context(cafile=str(pki / "ca.pem"))
                jar = http.cookiejar.CookieJar()
                opener = urllib.request.build_opener(
                    urllib.request.HTTPSHandler(context=tls),
                    urllib.request.HTTPCookieProcessor(jar),
                )
                csrf = ""

                def call(method, path, body=None, expected=200, reason=None, record=True):
                    data = None if body is None else json.dumps(body).encode()
                    headers = {"Content-Type": "application/json"}
                    if method == "POST":
                        headers |= {"Origin": origin, "X-CSRF-Token": csrf}
                    request = urllib.request.Request(
                        origin + path, data=data, headers=headers, method=method
                    )
                    try:
                        response = opener.open(request, timeout=5)
                    except urllib.error.HTTPError as failure:
                        response = failure
                    with response:
                        raw = response.read()
                        value = json.loads(raw) if raw else None
                        assert response.status == expected, (path, response.status, value, expected)
                        assert response.headers["Cache-Control"] == "no-store"
                        assert response.headers["X-Request-ID"]
                        if reason:
                            assert value["error"]["code"] == reason, value
                            assert isinstance(value["error"]["field_errors"], list)
                        if expected == 204:
                            assert not raw
                        for cookie in jar:
                            assert (
                                cookie.secure
                                and cookie.path == "/"
                                and cookie.has_nonstandard_attr("HttpOnly")
                            )
                            assert cookie.value.encode() not in raw, "cookie leaked to JSON"
                        if record:
                            results.append(
                                {
                                    "method": method,
                                    "path": path,
                                    "status": response.status,
                                    "body": value,
                                }
                            )
                            print(
                                method, path, response.status, json.dumps(value, ensure_ascii=False)
                            )
                        return value

                def refresh():
                    nonlocal csrf
                    csrf = call("GET", "/api/v1/auth/csrf", record=False)["csrf_token"]

                deadline = time.monotonic() + 10
                while True:
                    assert api.poll() is None, "API exited during startup"
                    try:
                        call("GET", "/health/ready", record=False)
                        break
                    except (urllib.error.URLError, AssertionError):
                        assert time.monotonic() < deadline, "API readiness deadline"
                        time.sleep(0.05)
                consent = call("GET", "/api/v1/auth/personal-data-consent")
                token = call("GET", "/api/v1/auth/csrf")
                csrf = token["csrf_token"]
                call("GET", "/api/v1/me", expected=401, reason="AUTH_REQUIRED")
                register = {
                    "email": "demo@example.test",
                    "password": "example-password-only",
                    "display_name": "API Demo",
                    "personal_data_consent": {
                        "accepted": True,
                        "document_version": consent["document_version"],
                        "document_sha256": consent["document_sha256"],
                    },
                }
                changed = register | {
                    "personal_data_consent": register["personal_data_consent"]
                    | {"document_version": "old-version"}
                }
                call("POST", "/api/v1/auth/register", changed, 409, "CONSENT_DOCUMENT_CHANGED")
                challenge = call("POST", "/api/v1/auth/register", register, 202)
                call(
                    "POST",
                    "/api/v1/auth/verify-email",
                    {"challenge_id": challenge["challenge_id"], "code": "999999"},
                    422,
                    "CODE_INVALID_OR_EXPIRED",
                )
                call(
                    "POST",
                    "/api/v1/auth/verify-email",
                    {"challenge_id": challenge["challenge_id"], "code": "001234"},
                )
                call(
                    "POST",
                    "/api/v1/auth/resend-verification",
                    {"email": "bad"},
                    422,
                    "VALIDATION_ERROR",
                )
                call(
                    "POST", "/api/v1/auth/resend-verification", {"email": "demo@example.test"}, 202
                )
                call(
                    "POST",
                    "/api/v1/auth/forgot-password",
                    {"email": "bad"},
                    422,
                    "VALIDATION_ERROR",
                )
                call("POST", "/api/v1/auth/forgot-password", {"email": "demo@example.test"}, 202)
                call(
                    "POST",
                    "/api/v1/auth/login",
                    {"email": "demo@example.test", "password": "wrong-password-only"},
                    401,
                    "INVALID_CREDENTIALS",
                )
                call(
                    "POST",
                    "/api/v1/auth/login",
                    {"email": "demo@example.test", "password": "example-password-only"},
                )
                refresh()
                me = call("GET", "/api/v1/me")
                assert me["email"] == "demo@example.test" and me["email_verified"] is True
                call("POST", "/api/v1/auth/logout", expected=204)
                call("POST", "/api/v1/auth/logout", expected=403, reason="CSRF_INVALID")
                refresh()
                call(
                    "POST",
                    "/api/v1/auth/reset-password",
                    {"token": "invalid-one-time-token", "new_password": "changed-password-only"},
                    422,
                    "TOKEN_INVALID_OR_EXPIRED",
                )
                call(
                    "POST",
                    "/api/v1/auth/reset-password",
                    {"token": "example-one-time-token", "new_password": "changed-password-only"},
                )
                stop(auth)
                call(
                    "GET",
                    "/api/v1/auth/personal-data-consent",
                    expected=503,
                    reason="DEPENDENCY_UNAVAILABLE",
                )
                call("GET", "/api/v1/auth/csrf", expected=503, reason="DEPENDENCY_UNAVAILABLE")
                stop(api)
                assert api.returncode == 0 and auth.returncode == 0, "ungraceful process exit"
                api_log.seek(0)
                logs = api_log.read()
                for secret in (
                    "example-password-only",
                    "example-one-time-token",
                    "001234",
                    token["csrf_token"],
                ):
                    assert secret not in logs, "secret appeared in API logs"
                for operation in {(r["method"], r["path"]) for r in results}:
                    codes = [r["status"] for r in results if (r["method"], r["path"]) == operation]
                    assert any(code < 300 for code in codes) and any(
                        code >= 400 for code in codes
                    ), operation
                assert len({(r["method"], r["path"]) for r in results}) == 10
                if output:
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
                print(
                    "PASS: 10 endpoints, success+error, separate HTTPS/mTLS processes, graceful shutdown"
                )
            finally:
                for process in reversed(processes):
                    stop(process)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path)
    smoke(parser.parse_args().output)
