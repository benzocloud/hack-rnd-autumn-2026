#!/usr/bin/env python3
"""Real-HTTP financial journey harness against a public API origin.

Does not use Auth/predictor doubles or financial mocks. Exit status is non-zero
when a selected mandatory scenario fails. Skipped mandatory scenarios also fail
the run unless --allow-skip-mandatory is set (never for CI greenwashing).
"""

from __future__ import annotations

import argparse
import json
import secrets
import sys
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))

from lib.http_client import ApiClient  # noqa: E402

ARTIFACTS = ROOT / "artifacts"
MANDATORY = {
    "P0-01",
    "P0-02",
    "P0-03",
    "P0-04",
    "P0-05",
    "P0-06",
    "P0-07",
    "P0-08",
    "P0-09",
    "P0-10",
}


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def write_json(path: Path, payload: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


class ScenarioResult:
    def __init__(self, scenario_id: str) -> None:
        self.scenario_id = scenario_id
        self.status = "todo"
        self.detail: str | None = None
        self.evidence: dict[str, Any] = {}

    def pass_(self, detail: str = "", **evidence: Any) -> None:
        self.status = "pass"
        self.detail = detail or None
        self.evidence.update(evidence)

    def fail(self, detail: str, **evidence: Any) -> None:
        self.status = "fail"
        self.detail = detail
        self.evidence.update(evidence)

    def blocked(self, detail: str, **evidence: Any) -> None:
        self.status = f"blocked:{detail}" if not detail.startswith("blocked:") else detail
        self.detail = detail
        self.evidence.update(evidence)

    def skip(self, detail: str) -> None:
        self.status = "skip"
        self.detail = detail

    def as_dict(self) -> dict[str, Any]:
        return {
            "id": self.scenario_id,
            "status": self.status,
            "detail": self.detail,
            "evidence": self.evidence,
        }


def poll_until(
    client: ApiClient,
    path: str,
    *,
    is_done,
    timeout_s: float,
    interval_s: float,
) -> Any:
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while time.monotonic() < deadline:
        response = client.request("GET", path, expected={200, 202})
        last = response.body
        if is_done(response.body):
            return response.body
        time.sleep(interval_s)
    raise TimeoutError(f"polling {path} timed out after {timeout_s}s; last={last!r}")


def require_origin(origin: str) -> None:
    parsed = urlparse(origin)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise SystemExit(f"invalid SALDO_E2E_API_ORIGIN: {origin!r}")
    host = parsed.hostname or ""
    if host in {"saldo.brokebyte.me"}:
        raise SystemExit(
            "refusing public production host; use an isolated local API origin "
            "(set SALDO_E2E_API_ORIGIN to loopback/test stack)"
        )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--api-origin",
        default=__import__("os").environ.get("SALDO_E2E_API_ORIGIN", ""),
        help="Public API origin, e.g. https://127.0.0.1:8443",
    )
    parser.add_argument(
        "--fixture",
        type=Path,
        default=ROOT / "fixtures" / "ordinary_90d_c001.csv",
    )
    parser.add_argument("--manifest", type=Path, default=ROOT / "fixtures" / "manifest.json")
    parser.add_argument("--email-prefix", default="owner")
    parser.add_argument(
        "--email-domain",
        default=__import__("os").environ.get("SALDO_E2E_EMAIL_DOMAIN", "financial-e2e.test"),
    )
    parser.add_argument("--verify-code", default="000000", help="Demo allowlist code when enabled")
    parser.add_argument("--poll-timeout", type=float, default=120.0)
    parser.add_argument("--poll-interval", type=float, default=1.0)
    parser.add_argument("--only", nargs="*", default=None, help="Optional scenario id filter")
    parser.add_argument(
        "--allow-skip-mandatory",
        action="store_true",
        help="Dangerous: do not use for claimed green acceptance",
    )
    parser.add_argument(
        "--dry-probe",
        action="store_true",
        help="Only GET /auth/csrf; record connectivity without mutating state",
    )
    args = parser.parse_args()

    if not args.api_origin:
        write_json(
            ARTIFACTS / "last-run.json",
            {
                "ts": utc_now(),
                "status": "blocked:missing_api_origin",
                "hint": "Export SALDO_E2E_API_ORIGIN to the isolated stack public API base",
            },
        )
        print("blocked: set SALDO_E2E_API_ORIGIN", file=sys.stderr)
        return 2

    require_origin(args.api_origin)
    client = ApiClient(origin=args.api_origin)
    results: dict[str, ScenarioResult] = {}
    run: dict[str, Any] = {
        "ts": utc_now(),
        "api_origin_host": urlparse(args.api_origin).hostname,
        "fixture": str(args.fixture),
        "results": [],
    }

    def result(scenario_id: str) -> ScenarioResult:
        item = ScenarioResult(scenario_id)
        results[scenario_id] = item
        return item

    selected = set(args.only) if args.only else None

    def wanted(scenario_id: str) -> bool:
        return selected is None or scenario_id in selected

    # --- connectivity ---
    if wanted("P0-01") or args.dry_probe:
        probe = result("PROBE-csrf" if args.dry_probe and not wanted("P0-01") else "P0-01")
        try:
            client.refresh_csrf()
            if args.dry_probe and not wanted("P0-01"):
                probe.pass_("csrf ok", csrf_present=True)
            else:
                # Registration continues below; mark csrf portion later.
                probe.status = "todo"
        except Exception as exc:  # noqa: BLE001 - surface as blocked/fail for harness
            probe.blocked(f"api_unreachable:{exc}")
            run["results"] = [item.as_dict() for item in results.values()]
            write_json(ARTIFACTS / "last-run.json", run)
            print(json.dumps(run, ensure_ascii=False, indent=2))
            return 2

    if args.dry_probe and selected is None:
        run["results"] = [item.as_dict() for item in results.values()]
        write_json(ARTIFACTS / "last-run.json", run)
        print(json.dumps(run, ensure_ascii=False, indent=2))
        return 0 if all(not r.status.startswith("blocked") and r.status != "fail" for r in results.values()) else 2

    unique = uuid.uuid4().hex[:12]
    email = f"{args.email_prefix}+{unique}@{args.email_domain}"
    password = secrets.token_urlsafe(18) + "aA9!"
    run["account_email_domain"] = args.email_domain
    run["account_email_local_suffix"] = unique

    # P0-01 register
    if wanted("P0-01"):
        s = results.get("P0-01") or result("P0-01")
        try:
            client.refresh_csrf()
            consent = client.request("GET", "/api/v1/auth/personal-data-consent", expected=200).body
            assert isinstance(consent, dict)
            client.refresh_csrf()
            registered = client.request(
                "POST",
                "/api/v1/auth/register",
                csrf=True,
                json_body={
                    "email": email,
                    "password": password,
                    "display_name": f"E2E {unique}",
                    "personal_data_consent": {
                        "accepted": True,
                        "document_version": consent["document_version"],
                        "document_sha256": consent["document_sha256"],
                    },
                },
                expected={201, 200},
            ).body
            challenge_id = registered.get("challenge_id") if isinstance(registered, dict) else None
            if not challenge_id:
                s.fail("missing challenge_id", body=registered)
            else:
                s.pass_("registered", challenge_id_present=True)
                run["challenge_id_present"] = True
                run["_challenge_id"] = challenge_id
        except Exception as exc:  # noqa: BLE001
            s.fail(str(exc))

    # P0-02 verify
    if wanted("P0-02"):
        s = result("P0-02")
        challenge_id = run.get("_challenge_id")
        if not challenge_id:
            s.blocked("missing_challenge_from_P0-01")
        else:
            try:
                client.refresh_csrf()
                verified = client.request(
                    "POST",
                    "/api/v1/auth/verify-email",
                    csrf=True,
                    json_body={"challenge_id": challenge_id, "code": args.verify_code},
                    expected={200, 204},
                )
                # Verify must not silently establish a financial session as login.
                me = client.request("GET", "/api/v1/auth/me", expected={200, 401})
                auto_login = me.status == 200 and isinstance(me.body, dict) and me.body.get("user")
                if auto_login:
                    s.fail("verify-email appeared to auto-login", me=me.body)
                else:
                    s.pass_("verified_without_autologin", verify_status=verified.status)
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

    # P0-03 login
    if wanted("P0-03"):
        s = result("P0-03")
        try:
            client.refresh_csrf()
            session = client.request(
                "POST",
                "/api/v1/auth/login",
                csrf=True,
                json_body={"email": email, "password": password},
                expected=200,
            ).body
            client.refresh_csrf()
            me = client.request("GET", "/api/v1/auth/me", expected=200).body
            s.pass_("logged_in", user_present=isinstance(me, dict) and "user" in me, session_keys=sorted(session) if isinstance(session, dict) else [])
        except Exception as exc:  # noqa: BLE001
            s.fail(str(exc))

    # P0-04 create company
    company_id = None
    if wanted("P0-04"):
        s = result("P0-04")
        try:
            client.refresh_csrf()
            created = client.request(
                "POST",
                "/api/v1/companies",
                csrf=True,
                idempotency_key=client.new_idempotency_key("company"),
                json_body={
                    "name": f"E2E Co {unique}",
                    "inn": None,
                    "currency": "RUB",
                    "timezone": "Europe/Moscow",
                },
                expected={200, 201},
            ).body
            if not isinstance(created, dict) or not created.get("id"):
                # Some deployments may omit onboarding; record blocker without inventing SQL membership.
                s.blocked("company_create_unexpected_body", body=created)
            else:
                company_id = created["id"]
                s.pass_("company_created", company_id_present=True)
                run["company_id"] = company_id
        except Exception as exc:  # noqa: BLE001
            s.blocked(f"company_create:{exc}")

    # Remaining P0 import/forecast steps need company + fixture.
    if company_id and args.fixture.is_file() and any(wanted(x) for x in ("P0-05", "P0-06", "P0-07", "P0-08", "P0-09", "P0-10")):
        # Upload is multipart; keep implementation minimal and explicit.
        csv_bytes = args.fixture.read_bytes()
        if wanted("P0-05"):
            s = result("P0-05")
            try:
                client.refresh_csrf()
                # Build multipart manually for urllib.
                boundary = f"----saldo{uuid.uuid4().hex}"
                filename = args.fixture.name
                body = (
                    f"--{boundary}\r\n"
                    f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
                    f"Content-Type: text/csv\r\n\r\n"
                ).encode() + csv_bytes + f"\r\n--{boundary}--\r\n".encode()
                uploaded = client.request(
                    "POST",
                    f"/api/v1/companies/{company_id}/imports",
                    data=body,
                    content_type=f"multipart/form-data; boundary={boundary}",
                    csrf=True,
                    idempotency_key=client.new_idempotency_key("import"),
                    expected={200, 201, 202},
                )
                import_id = uploaded.body.get("id") if isinstance(uploaded.body, dict) else None
                if not import_id:
                    s.fail("missing import id", body=uploaded.body, status=uploaded.status)
                else:
                    s.pass_("upload_accepted", status=uploaded.status, import_id_present=True)
                    run["import_id"] = import_id
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

        import_id = run.get("import_id")
        if import_id and wanted("P0-06"):
            s = result("P0-06")
            try:
                preview = poll_until(
                    client,
                    f"/api/v1/companies/{company_id}/imports/{import_id}",
                    timeout_s=args.poll_timeout,
                    interval_s=args.poll_interval,
                    is_done=lambda body: isinstance(body, dict)
                    and body.get("status") in {"ready", "failed", "rejected", "preview_ready", "awaiting_ack"},
                )
                s.pass_("preview_terminal", status=preview.get("status"), can_commit=preview.get("can_commit"))
                run["preview"] = {
                    "status": preview.get("status"),
                    "can_commit": preview.get("can_commit"),
                    "issue_count": preview.get("issue_count"),
                }
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

        if import_id and wanted("P0-07"):
            s = result("P0-07")
            try:
                # Acknowledge issues if endpoint lists them; commit with CAS fields from preview.
                preview = client.request(
                    "GET",
                    f"/api/v1/companies/{company_id}/imports/{import_id}",
                    expected=200,
                ).body
                assert isinstance(preview, dict)
                commit_body = {
                    "preview_version": preview.get("preview_version"),
                    "base_data_version": preview.get("base_data_version"),
                    "acknowledged_issue_ids": preview.get("blocking_issue_ids")
                    or preview.get("acknowledgement_required_issue_ids")
                    or [],
                }
                # Drop nulls to avoid spurious validation errors on optional fields.
                commit_body = {k: v for k, v in commit_body.items() if v is not None}
                client.refresh_csrf()
                committed = client.request(
                    "POST",
                    f"/api/v1/companies/{company_id}/imports/{import_id}/commit",
                    csrf=True,
                    idempotency_key=client.new_idempotency_key("commit"),
                    json_body=commit_body,
                    expected={200, 201},
                ).body
                s.pass_("committed", body_keys=sorted(committed) if isinstance(committed, dict) else [])
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

        if wanted("P0-08"):
            s = result("P0-08")
            try:
                txs = client.request(
                    "GET",
                    f"/api/v1/companies/{company_id}/transactions?limit=100",
                    expected=200,
                ).body
                account = client.request(
                    "GET",
                    f"/api/v1/companies/{company_id}/account",
                    expected=200,
                ).body
                count = None
                if isinstance(txs, dict):
                    items = txs.get("items") or txs.get("transactions")
                    if isinstance(items, list):
                        count = len(items)
                s.pass_("ledger_readable", transaction_page_count=count, account_keys=sorted(account) if isinstance(account, dict) else [])
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

        if wanted("P0-09"):
            s = result("P0-09")
            try:
                account = client.request(
                    "GET",
                    f"/api/v1/companies/{company_id}/account",
                    expected=200,
                ).body
                assert isinstance(account, dict)
                client.refresh_csrf()
                created = client.request(
                    "POST",
                    f"/api/v1/companies/{company_id}/forecasts",
                    csrf=True,
                    idempotency_key=client.new_idempotency_key("forecast"),
                    json_body={
                        "horizon_days": 30,
                        "account_data_version": account.get("data_version") or account.get("version"),
                    },
                    expected={200, 201, 202},
                ).body
                forecast_id = created.get("id") if isinstance(created, dict) else None
                if not forecast_id:
                    s.fail("missing forecast id", body=created)
                else:
                    run["forecast_id"] = forecast_id

                    def forecast_done(body: Any) -> bool:
                        return isinstance(body, dict) and body.get("status") in {
                            "succeeded",
                            "failed",
                            "rejected",
                            "cancelled",
                        }

                    final = poll_until(
                        client,
                        f"/api/v1/companies/{company_id}/forecasts/{forecast_id}",
                        timeout_s=args.poll_timeout,
                        interval_s=args.poll_interval,
                        is_done=forecast_done,
                    )
                    points = final.get("points") or final.get("result", {}).get("points") if isinstance(final, dict) else None
                    n = len(points) if isinstance(points, list) else None
                    if final.get("status") != "succeeded":
                        s.fail("forecast_not_succeeded", status=final.get("status"), body=final)
                    elif n != 30:
                        s.fail("expected_30_points", point_count=n, status=final.get("status"))
                    else:
                        s.pass_("forecast_succeeded", point_count=n)
            except Exception as exc:  # noqa: BLE001
                s.fail(str(exc))

        if wanted("P0-10"):
            s = result("P0-10")
            forecast_id = run.get("forecast_id")
            if not forecast_id:
                s.blocked("missing_forecast_id")
            else:
                try:
                    first = client.request(
                        "GET",
                        f"/api/v1/companies/{company_id}/forecasts/{forecast_id}",
                        expected=200,
                    ).raw
                    second = client.request(
                        "GET",
                        f"/api/v1/companies/{company_id}/forecasts/{forecast_id}",
                        expected=200,
                    ).raw
                    if first != second:
                        s.fail("reload_changed_bytes", first_len=len(first), second_len=len(second))
                    else:
                        s.pass_("reload_stable", bytes=len(first))
                except Exception as exc:  # noqa: BLE001
                    s.fail(str(exc))
    else:
        for scenario_id in ("P0-05", "P0-06", "P0-07", "P0-08", "P0-09", "P0-10"):
            if wanted(scenario_id) and scenario_id not in results:
                s = result(scenario_id)
                if not company_id:
                    s.blocked("company_unavailable")
                elif not args.fixture.is_file():
                    s.blocked(f"missing_fixture:{args.fixture}")

    # Mark unimplemented selected scenarios explicitly.
    for scenario_id in MANDATORY:
        if wanted(scenario_id) and scenario_id not in results:
            result(scenario_id).blocked("not_executed")

    run["results"] = [results[key].as_dict() for key in sorted(results)]
    write_json(ARTIFACTS / "last-run.json", run)
    print(json.dumps(run, ensure_ascii=False, indent=2))

    failed = False
    for item in results.values():
        if item.status == "fail" or item.status.startswith("blocked:"):
            if item.scenario_id in MANDATORY or (selected and item.scenario_id in selected):
                failed = True
        if item.status == "skip" and item.scenario_id in MANDATORY and not args.allow_skip_mandatory:
            failed = True
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
