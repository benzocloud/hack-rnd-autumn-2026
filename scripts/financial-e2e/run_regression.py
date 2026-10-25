#!/usr/bin/env python3
"""CSV/manual regression and negative HTTP scenarios against an isolated stack."""

from __future__ import annotations

import argparse
import json
import sys
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))

from lib.http_client import ApiClient  # noqa: E402
from lib.journey import (  # noqa: E402
    FIXTURES,
    Journey,
    load_stack_env,
    multipart_import,
    next_demo_email,
)

ARTIFACTS = ROOT / "artifacts"


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def record(
    results: list[dict[str, Any]], scenario_id: str, status: str, detail: str = "", **evidence: Any
) -> None:
    # Keep evidence keys out of reserved names used by the result row.
    evidence.pop("status", None)
    evidence.pop("detail", None)
    evidence.pop("id", None)
    row = {"id": scenario_id, "status": status, "detail": detail or None, "evidence": evidence}
    results.append(row)
    print(f"{scenario_id:8} {status} {detail}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--api-origin", default=__import__("os").environ.get("SALDO_E2E_API_ORIGIN", "")
    )
    parser.add_argument("--ca-file", default=__import__("os").environ.get("SALDO_E2E_CA_FILE", ""))
    parser.add_argument("--fixture", type=Path, default=FIXTURES / "ordinary_90d_c001.csv")
    parser.add_argument("--short", type=Path, default=FIXTURES / "short_history.csv")
    parser.add_argument("--eod-conflict", type=Path, default=FIXTURES / "eod_conflict.csv")
    args = parser.parse_args()
    if not args.api_origin:
        print("blocked: set SALDO_E2E_API_ORIGIN", file=sys.stderr)
        return 2

    state = load_stack_env()
    client = ApiClient(origin=args.api_origin, ca_file=args.ca_file or state.get("ca_file"))
    email = next_demo_email(state)
    results: list[dict[str, Any]] = []
    run: dict[str, Any] = {
        "ts": utc_now(),
        "email_local": email.split("@", 1)[0],
        "results": results,
    }

    journey = Journey.register_login(client, email)
    company_id = journey.create_company()
    run["company_id"] = company_id

    # --- baseline import/commit ---
    first_key = client.new_idempotency_key("import")
    journey.upload_csv(args.fixture, idempotency_key=first_key)
    preview = journey.wait_preview()
    if preview.get("status") != "preview_ready":
        record(
            results,
            "BOOT-import",
            "fail",
            f"preview status {preview.get('status')}",
            preview=preview,
        )
        ARTIFACTS.mkdir(parents=True, exist_ok=True)
        (ARTIFACTS / "last-regression.json").write_text(
            json.dumps(run, ensure_ascii=False, indent=2) + "\n"
        )
        return 1
    committed = journey.commit_import(preview)
    baseline_count = journey.transaction_count()
    account_after_import = journey.account()
    source = account_after_import.get("transaction_source")
    record(
        results,
        "BOOT-import",
        "pass",
        "baseline committed",
        transaction_page_count=baseline_count,
        data_version=account_after_import.get("data_version"),
        transaction_source=source,
    )

    # R-01 re-upload same statement
    try:
        journey.upload_csv(args.fixture)
        preview2 = journey.wait_preview()
        preview_block = preview2.get("preview") if isinstance(preview2.get("preview"), dict) else {}
        already = preview_block.get("already_present_count")
        new_tx = preview_block.get("new_transaction_count")
        if preview2.get("status") == "preview_ready" and (
            (isinstance(already, int) and already > 0) or new_tx == 0
        ):
            # Commit should not double facts when all already present.
            if (
                preview_block.get("can_commit") is False
                and (preview_block.get("blocking_issue_count") or 0) > 0
            ):
                record(
                    results,
                    "R-01",
                    "pass",
                    "re-upload preview without new facts / blocked appropriately",
                    already_present_count=already,
                    new_transaction_count=new_tx,
                )
            else:
                journey.commit_import(preview2)
                after = journey.transaction_count()
                if after == baseline_count or (
                    isinstance(already, int) and already > 0 and new_tx == 0
                ):
                    record(
                        results,
                        "R-01",
                        "pass",
                        "no fact doubling",
                        before=baseline_count,
                        after=after,
                        already_present_count=already,
                    )
                else:
                    record(
                        results,
                        "R-01",
                        "fail",
                        "transaction count grew after identical re-import",
                        before=baseline_count,
                        after=after,
                        already_present_count=already,
                        new_transaction_count=new_tx,
                    )
        else:
            record(results, "R-01", "fail", "unexpected re-upload preview", preview=preview2)
        baseline_count = journey.transaction_count() or baseline_count
    except Exception as exc:  # noqa: BLE001
        record(results, "R-01", "fail", str(exc))

    # R-02 replay same idempotency key
    try:
        again = journey.upload_csv(args.fixture, idempotency_key=first_key)
        # Same key+same body should reuse; exact id may match first import or return equivalent.
        record(
            results,
            "R-02",
            "pass",
            "replay accepted without new enqueue failure",
            replay_id=again.get("id"),
            original_id=committed.get("id"),
        )
    except Exception as exc:  # noqa: BLE001
        # Conflict on different body is N-02; same body should not fail.
        record(results, "R-02", "fail", str(exc))

    # R-08 before manuals: forecast needs reconciled EOD from import alone.
    try:
        old = journey.create_forecast()
        if old.get("status") != "succeeded":
            record(
                results,
                "R-08",
                "fail",
                "baseline forecast not succeeded",
                forecast_status=old.get("status"),
            )
        else:
            old_id = journey.forecast_id
            account = journey.account()
            journey.create_manual(
                operation_date="2026-09-27",
                direction="inflow",
                amount_minor="22222",
                expected_data_version=str(account["data_version"]),
                counterparty_name="E2E Stale Trigger",
                payment_purpose="Trigger forecast stale",
                expected=201,
            )
            refreshed = client.request(
                "GET",
                f"/api/v1/companies/{company_id}/forecasts/{old_id}",
                expected=200,
            ).body
            assert isinstance(refreshed, dict)
            if refreshed.get("is_stale") is True:
                record(results, "R-08", "pass", "old forecast marked stale", forecast_id=old_id)
            else:
                record(
                    results,
                    "R-08",
                    "fail",
                    "expected is_stale=true",
                    is_stale=refreshed.get("is_stale"),
                    forecast_status=refreshed.get("status"),
                )
            baseline_count = journey.transaction_count() or baseline_count
    except Exception as exc:  # noqa: BLE001
        record(results, "R-08", "fail", str(exc))

    # R-03 manual after import
    manual_key = client.new_idempotency_key("manual")
    manual_version: str | None = None
    manual_id: str | None = None
    try:
        account = journey.account()
        manual_version = str(account["data_version"])
        # Use a date inside window but unlikely to collide with exact imported row content.
        manual = journey.create_manual(
            operation_date="2026-09-28",
            direction="outflow",
            amount_minor="12345",
            expected_data_version=manual_version,
            counterparty_name="E2E Manual Counterparty",
            payment_purpose="Unique manual purpose e2e-regression",
            idempotency_key=manual_key,
            expected=201,
        )
        body = manual.body
        assert isinstance(body, dict)
        source_kind = (body.get("source") or {}).get("kind")
        account2 = journey.account()
        source2 = account2.get("transaction_source")
        # External namespace must remain (dataset binding), not flip to manual-only.
        mode = (source2 or {}).get("mode") if isinstance(source2, dict) else None
        system = (source2 or {}).get("source_system") if isinstance(source2, dict) else None
        if source_kind != "manual":
            record(results, "R-03", "fail", "manual provenance missing", body=body)
        elif system not in {None, "dataset_v1"} and mode == "manual":
            record(
                results,
                "R-03",
                "fail",
                "external namespace replaced by manual",
                transaction_source=source2,
            )
        else:
            # Prefer external binding preserved.
            ok_ns = True
            if isinstance(source, dict) and source.get("source_system") == "dataset_v1":
                ok_ns = system == "dataset_v1"
            if not ok_ns:
                record(results, "R-03", "fail", "namespace changed", before=source, after=source2)
            else:
                record(
                    results,
                    "R-03",
                    "pass",
                    "manual allowed; provenance manual; namespace kept",
                    transaction_source=source2,
                    manual_id=body.get("id"),
                )
        manual_id = body.get("id")
        run["manual_id"] = manual_id
        baseline_count = journey.transaction_count() or baseline_count
    except Exception as exc:  # noqa: BLE001
        record(results, "R-03", "fail", str(exc))
        manual_id = None

    # R-04 replay manual — body must match the first request (incl. expected_data_version).
    try:
        if manual_version is None or manual_id is None:
            record(results, "R-04", "blocked:no_manual")
        else:
            replay = journey.create_manual(
                operation_date="2026-09-28",
                direction="outflow",
                amount_minor="12345",
                expected_data_version=manual_version,
                counterparty_name="E2E Manual Counterparty",
                payment_purpose="Unique manual purpose e2e-regression",
                idempotency_key=manual_key,
                expected={200, 201},
            )
            body = replay.body
            assert isinstance(body, dict)
            if body.get("id") == manual_id:
                record(
                    results, "R-04", "pass", "manual replay returns same id", manual_id=manual_id
                )
            else:
                after = journey.transaction_count()
                if after == baseline_count:
                    record(
                        results,
                        "R-04",
                        "pass",
                        "manual replay did not increase count",
                        before=baseline_count,
                        after=after,
                        replay_id=body.get("id"),
                    )
                else:
                    record(
                        results,
                        "R-04",
                        "fail",
                        "manual replay created another fact",
                        before=baseline_count,
                        after=after,
                        replay_id=body.get("id"),
                        original=manual_id,
                    )
    except Exception as exc:  # noqa: BLE001
        record(results, "R-04", "fail", str(exc))

    # R-05 exact imported duplicate as manual
    try:
        # Take one imported transaction and recreate identical economics/date/purpose.
        txs = journey.list_transactions(limit=20)
        items = txs.get("items") or []
        imported = None
        for item in items:
            src = item.get("source") or {}
            if src.get("kind") in {"file", "external"} or src.get("source_system") == "dataset_v1":
                imported = item
                break
        if not imported:
            record(results, "R-05", "blocked:no_imported_sample")
        else:
            account = journey.account()
            amount = imported["amount"]["amount_minor"]
            dup = journey.create_manual(
                operation_date=imported["operation_date"],
                direction=imported["direction"],
                amount_minor=str(amount),
                expected_data_version=str(account["data_version"]),
                counterparty_name=imported.get("counterparty_name") or "",
                payment_purpose=imported.get("payment_purpose") or "",
                counterparty_inn=imported.get("counterparty_inn") or "",
                expected={409, 422},
            )
            code = (
                (dup.body or {}).get("error", {}).get("code")
                if isinstance(dup.body, dict)
                else None
            )
            if dup.status == 409 and code in {
                "TRANSACTION_DUPLICATE_SUSPECTED",
                "FACTUAL_SOURCE_CONFLICT",
                "CONFLICT",
            }:
                record(results, "R-05", "pass", "exact imported duplicate rejected", code=code)
            elif dup.status == 409:
                record(
                    results, "R-05", "pass", "duplicate rejected with 409", code=code, body=dup.body
                )
            else:
                record(
                    results,
                    "R-05",
                    "fail",
                    "expected 409 duplicate",
                    http_status=dup.status,
                    body=dup.body,
                )
    except Exception as exc:  # noqa: BLE001
        # create_manual with expected set raises on unexpected; catch AssertionError from request
        msg = str(exc)
        if "409" in msg and "TRANSACTION_DUPLICATE" in msg:
            record(results, "R-05", "pass", "duplicate rejected", error=msg)
        else:
            record(results, "R-05", "fail", msg)

    # N-01 invalid upload (empty / bad adapter via truncated body)
    try:
        assert journey.account_id
        body, content_type = multipart_import(journey.account_id, b"not-a-csv", "bad.csv")
        # Corrupt adapter field by rebuilding with wrong adapter.
        boundary = f"----saldo{uuid.uuid4().hex}"
        bad = (
            f'--{boundary}\r\nContent-Disposition: form-data; name="adapter"\r\n\r\nbogus\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="source_company_id"\r\n\r\nC001\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="account_id"\r\n\r\n{journey.account_id}\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="currency"\r\n\r\nRUB\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="bad.csv"\r\nContent-Type: text/csv\r\n\r\nx\r\n'
            f"--{boundary}--\r\n"
        ).encode()
        client.refresh_csrf()
        resp = client.request(
            "POST",
            f"/api/v1/companies/{company_id}/imports",
            data=bad,
            content_type=f"multipart/form-data; boundary={boundary}",
            csrf=True,
            idempotency_key=client.new_idempotency_key("bad"),
            expected={400, 422},
        )
        record(results, "N-01", "pass", "invalid upload rejected", http_status=resp.status)
    except Exception as exc:  # noqa: BLE001
        record(results, "N-01", "fail", str(exc))

    # N-02 duplicate key different body
    try:
        key = client.new_idempotency_key("dupbody")
        journey.upload_csv(args.fixture, idempotency_key=key)
        # Different bytes under same key.
        body, content_type = multipart_import(
            journey.account_id or "", args.short.read_bytes(), "short.csv"
        )
        client.refresh_csrf()
        conflict = client.request(
            "POST",
            f"/api/v1/companies/{company_id}/imports",
            data=body,
            content_type=content_type,
            csrf=True,
            idempotency_key=key,
            expected={409, 422},
        )
        record(
            results,
            "N-02",
            "pass",
            "duplicate key different body rejected",
            http_status=conflict.status,
        )
    except Exception as exc:  # noqa: BLE001
        record(results, "N-02", "fail", str(exc))

    # N-04 short history forecast
    try:
        # Fresh company for short history isolation.
        email2 = next_demo_email(state)
        j2 = Journey.register_login(
            ApiClient(origin=args.api_origin, ca_file=args.ca_file or state.get("ca_file")), email2
        )
        j2.create_company()
        j2.upload_csv(args.short)
        p = j2.wait_preview()
        if p.get("status") == "preview_ready":
            j2.commit_import(p)
            # cutoff from short fixture max date — use 2026-09-29 still may fail eligibility
            final = j2.create_forecast(cutoff="2026-09-29")
            status = final.get("status")
            if status in {"failed", "rejected"}:
                record(
                    results,
                    "N-04",
                    "pass",
                    "short history not green-washed",
                    status=status,
                    error=final.get("error"),
                )
            elif status == "succeeded":
                # Must carry explicit limitations, not silent success as certified.
                readiness = final.get("readiness") or {}
                limitations = (
                    (final.get("result") or {}).get("assumptions")
                    or readiness.get("limitations")
                    or []
                )
                record(
                    results,
                    "N-04",
                    "fail",
                    "short history produced succeeded forecast; check limitations",
                    limitations=limitations,
                    readiness=readiness,
                )
            else:
                record(results, "N-04", "fail", f"unexpected status {status}", body=final)
        else:
            record(
                results, "N-04", "pass", "short history blocked at preview", status=p.get("status")
            )
    except Exception as exc:  # noqa: BLE001
        msg = str(exc)
        if any(
            token in msg
            for token in (
                "422",
                "HISTORY_WINDOW_TOO_SHORT",
                "UNAVAILABLE",
                "INSUFFICIENT",
                "UNSUPPORTED",
            )
        ):
            record(results, "N-04", "pass", "short history rejected at create", error=msg)
        else:
            record(results, "N-04", "fail", msg)

    # N-05 EOD conflict
    try:
        email3 = next_demo_email(state)
        j3 = Journey.register_login(
            ApiClient(origin=args.api_origin, ca_file=args.ca_file or state.get("ca_file")), email3
        )
        j3.create_company()
        j3.upload_csv(args.eod_conflict)
        p = j3.wait_preview()
        preview_block = p.get("preview") if isinstance(p.get("preview"), dict) else {}
        blocking = preview_block.get("blocking_issue_count") or 0
        can_commit = preview_block.get("can_commit")
        if p.get("status") == "failed" or (blocking and can_commit is False):
            record(
                results,
                "N-05",
                "pass",
                "EOD conflict blocks commit",
                blocking_issue_count=blocking,
                can_commit=can_commit,
            )
        elif can_commit is False:
            record(results, "N-05", "pass", "can_commit false", preview=preview_block)
        else:
            record(results, "N-05", "fail", "EOD conflict allowed commit", preview=p)
    except Exception as exc:  # noqa: BLE001
        record(results, "N-05", "fail", str(exc))

    # N-08 CSRF missing
    try:
        client.refresh_csrf()
        # Drop csrf intentionally.
        saved = client.csrf_token
        client.csrf_token = "x" * 16
        # Empty token path: call without header by temporary monkeypatch
        client.csrf_token = None
        try:
            client.request(
                "POST",
                f"/api/v1/companies/{company_id}/forecasts",
                csrf=True,
                json_body={"cutoff_date": "2026-09-29", "horizon_days": 30},
                idempotency_key=client.new_idempotency_key("csrf"),
                expected=403,
            )
            record(results, "N-08", "fail", "csrf=None should have raised before request")
        except RuntimeError as exc:
            if "csrf_token is empty" in str(exc):
                # Client guard — also try invalid token.
                client.csrf_token = "invalid-csrf-token!!"
                resp = client.request(
                    "POST",
                    f"/api/v1/companies/{company_id}/forecasts",
                    csrf=True,
                    json_body={"cutoff_date": "2026-09-29", "horizon_days": 30},
                    idempotency_key=client.new_idempotency_key("csrf2"),
                    expected=403,
                )
                code = (
                    (resp.body or {}).get("error", {}).get("code")
                    if isinstance(resp.body, dict)
                    else None
                )
                record(
                    results,
                    "N-08",
                    "pass",
                    "invalid CSRF rejected",
                    code=code,
                    http_status=resp.status,
                )
            else:
                raise
        finally:
            client.csrf_token = saved
    except Exception as exc:  # noqa: BLE001
        record(results, "N-08", "fail", str(exc))

    # N-09 foreign company
    try:
        foreign = "019a0c65-9800-7000-8000-00000000dead"
        resp = client.request(
            "GET",
            f"/api/v1/companies/{foreign}/account",
            expected={403, 404},
        )
        record(results, "N-09", "pass", "foreign company denied", http_status=resp.status)
    except Exception as exc:  # noqa: BLE001
        record(results, "N-09", "fail", str(exc))

    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / "last-regression.json").write_text(
        json.dumps(run, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {"summary": {r["id"]: r["status"] for r in results}}, ensure_ascii=False, indent=2
        )
    )

    failed = any(
        r["status"] == "fail" or str(r["status"]).startswith("blocked:")
        for r in results
        if r["id"] != "BOOT-import"
    )
    boot_fail = any(r["id"] == "BOOT-import" and r["status"] != "pass" for r in results)
    return 1 if failed or boot_fail else 0


if __name__ == "__main__":
    sys.exit(main())
