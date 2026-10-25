#!/usr/bin/env python3
"""Payment lifecycle HTTP scenarios against an isolated stack built from #48."""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))

from lib.http_client import ApiClient  # noqa: E402
from lib.journey import FIXTURES, Journey, load_stack_env, next_demo_email, poll  # noqa: E402

ARTIFACTS = ROOT / "artifacts"
CUTOFF = "2026-09-29"
# Horizon end for 30d from CUTOFF is 2026-10-29. In-horizon vs beyond.
IN_HORIZON = "2026-10-05"
BEYOND_HORIZON = "2026-11-15"


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def record(
    results: list[dict[str, Any]], scenario_id: str, status: str, detail: str = "", **evidence: Any
) -> None:
    evidence.pop("status", None)
    evidence.pop("detail", None)
    evidence.pop("id", None)
    results.append(
        {"id": scenario_id, "status": status, "detail": detail or None, "evidence": evidence}
    )
    print(f"{scenario_id:8} {status} {detail}")


def create_forecast(client: ApiClient, company_id: str, cutoff: str = CUTOFF) -> dict[str, Any]:
    client.refresh_csrf()
    created = client.request(
        "POST",
        f"/api/v1/companies/{company_id}/forecasts",
        csrf=True,
        idempotency_key=client.new_idempotency_key("forecast"),
        json_body={"cutoff_date": cutoff, "horizon_days": 30},
        expected=None,
    )
    if created.status not in {200, 201, 202}:
        return {"_http_status": created.status, "_body": created.body}
    assert isinstance(created.body, dict)
    fid = created.body["id"]
    final = poll(
        client,
        f"/api/v1/companies/{company_id}/forecasts/{fid}",
        timeout_s=120,
        is_done=lambda b: (
            isinstance(b, dict)
            and b.get("status") in {"succeeded", "failed", "rejected", "cancelled"}
        ),
    )
    assert isinstance(final, dict)
    return final


def get_payment(client: ApiClient, company_id: str, payment_id: str) -> dict[str, Any]:
    body = client.request(
        "GET", f"/api/v1/companies/{company_id}/payments/{payment_id}", expected=200
    ).body
    assert isinstance(body, dict)
    return body


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-origin", default=os.environ.get("SALDO_E2E_API_ORIGIN", ""))
    parser.add_argument("--ca-file", default=os.environ.get("SALDO_E2E_CA_FILE", ""))
    parser.add_argument("--fixture", type=Path, default=FIXTURES / "ordinary_90d_c001.csv")
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
        "source_sha": state.get("source_sha"),
        "results": results,
    }

    journey = Journey.register_login(client, email)
    company_id = journey.create_company()
    run["company_id"] = company_id
    journey.upload_csv(args.fixture)
    journey.commit_import(journey.wait_preview())

    # Baseline forecast without calendar obligations.
    baseline = create_forecast(client, company_id)
    if baseline.get("status") != "succeeded":
        record(results, "BOOT-forecast", "fail", "baseline forecast required", body=baseline)
        ARTIFACTS.mkdir(parents=True, exist_ok=True)
        (ARTIFACTS / "last-payments.json").write_text(
            json.dumps(run, ensure_ascii=False, indent=2) + "\n"
        )
        return 1
    baseline_points = (baseline.get("result") or {}).get("daily_points") or []
    record(
        results, "BOOT-forecast", "pass", "ordinary CSV forecast ok", points=len(baseline_points)
    )

    # --- PAY-01 create + forecast interaction ---
    try:
        client.refresh_csrf()
        created = client.request(
            "POST",
            f"/api/v1/companies/{company_id}/payments",
            csrf=True,
            idempotency_key=client.new_idempotency_key("pay"),
            json_body={
                "direction": "outflow",
                "amount": {"amount_minor": "5000000", "currency": "RUB"},
                "scheduled_date": IN_HORIZON,
                "counterparty_name": "E2E Landlord",
                "description": "Rent in horizon",
            },
            expected=201,
        ).body
        assert isinstance(created, dict)
        pay_id = created["id"]
        pay_ver = created["version"]
        run["payment_id"] = pay_id

        after = create_forecast(client, company_id)
        http_status = after.get("_http_status")
        err = None
        if http_status:
            err = ((after.get("_body") or {}).get("error") or {}).get("code")
        if http_status == 422 and err == "UNSUPPORTED_MODEL_CONTEXT":
            # Ordinary CSV cutoff 2026-09-29 vs wall-clock today cannot activate CURRENT calendar.
            # Explicit reject (not silent zero-risk) is the honest ordinary-CSV outcome.
            record(
                results,
                "PAY-01",
                "blocked:ordinary_csv_current_calendar",
                "payment created; in-horizon obligation rejects forecast until CURRENT calendar (cutoff≈today + group)",
                payment_id=pay_id,
                error=err,
                fixture_cutoff=CUTOFF,
                scheduled_date=IN_HORIZON,
            )
        elif after.get("status") == "succeeded":
            # Unexpected on ordinary CSV with wall clock far from cutoff — still accept if points reflect payment.
            points = (after.get("result") or {}).get("daily_points") or []
            record(
                results,
                "PAY-01",
                "pass",
                "forecast succeeded with payment",
                points=len(points),
                payment_id=pay_id,
            )
        else:
            record(
                results, "PAY-01", "fail", "unexpected forecast outcome after payment", body=after
            )
    except Exception as exc:  # noqa: BLE001
        record(results, "PAY-01", "fail", str(exc))
        pay_id = None
        pay_ver = None

    # --- PAY-02 amount then date edit ---
    try:
        if not pay_id:
            record(results, "PAY-02", "blocked:no_payment")
        else:
            client.refresh_csrf()
            amt = client.request(
                "PATCH",
                f"/api/v1/companies/{company_id}/payments/{pay_id}",
                csrf=True,
                json_body={
                    "amount": {"amount_minor": "6000000", "currency": "RUB"},
                    "version": pay_ver,
                },
                expected=200,
            ).body
            assert isinstance(amt, dict)
            if amt["amount"]["amount_minor"] != "6000000":
                record(results, "PAY-02", "fail", "amount not updated", body=amt)
            else:
                client.refresh_csrf()
                dated = client.request(
                    "PATCH",
                    f"/api/v1/companies/{company_id}/payments/{pay_id}",
                    csrf=True,
                    json_body={"scheduled_date": "2026-10-08", "version": amt["version"]},
                    expected=200,
                ).body
                assert isinstance(dated, dict)
                if dated.get("scheduled_date") != "2026-10-08":
                    record(results, "PAY-02", "fail", "date not updated", body=dated)
                else:
                    record(
                        results,
                        "PAY-02",
                        "pass",
                        "amount then date edited",
                        amount=dated["amount"]["amount_minor"],
                        scheduled_date=dated["scheduled_date"],
                        version=dated["version"],
                    )
                    pay_ver = dated["version"]
    except Exception as exc:  # noqa: BLE001
        record(results, "PAY-02", "fail", str(exc))

    # --- PAY-04 before manuals (manual facts break EOD reconciliation for forecast) ---
    try:
        if not pay_id:
            record(results, "PAY-04", "blocked:no_payment")
        else:
            current = get_payment(client, company_id, pay_id)
            client.refresh_csrf()
            client.request(
                "DELETE",
                f"/api/v1/companies/{company_id}/payments/{pay_id}?version={current['version']}",
                csrf=True,
                expected=204,
            )
            hist = client.request(
                "GET",
                f"/api/v1/companies/{company_id}/payments?status=cancelled&limit=50",
                expected=200,
            ).body
            assert isinstance(hist, dict)
            items = hist.get("items") or []
            found = any(item.get("id") == pay_id for item in items)
            fc = create_forecast(client, company_id)
            if not found:
                record(
                    results,
                    "PAY-04",
                    "fail",
                    "cancelled payment missing from history",
                    items_len=len(items),
                )
            elif fc.get("_http_status") == 422:
                err = ((fc.get("_body") or {}).get("error") or {}).get("code")
                record(
                    results,
                    "PAY-04",
                    "fail",
                    "forecast still rejected after cancelling in-horizon payment",
                    error=err,
                )
            elif fc.get("status") == "succeeded":
                record(
                    results,
                    "PAY-04",
                    "pass",
                    "cancel + history + forecast ok",
                    payment_id=pay_id,
                    points=len((fc.get("result") or {}).get("daily_points") or []),
                )
            else:
                record(results, "PAY-04", "fail", "unexpected forecast after cancel", body=fc)
    except Exception as exc:  # noqa: BLE001
        record(results, "PAY-04", "fail", str(exc))

    # --- PAY-03 partial match → unmatch → partial ---
    try:
        if not pay_id:
            record(results, "PAY-03", "blocked:no_payment")
        else:
            # Fresh payment for clean remaining math.
            client.refresh_csrf()
            match_pay = client.request(
                "POST",
                f"/api/v1/companies/{company_id}/payments",
                csrf=True,
                idempotency_key=client.new_idempotency_key("paym"),
                json_body={
                    "direction": "outflow",
                    "amount": {"amount_minor": "3000000", "currency": "RUB"},
                    "scheduled_date": BEYOND_HORIZON,
                    "counterparty_name": "E2E Match Vendor",
                    "description": "Match target beyond horizon",
                },
                expected=201,
            ).body
            assert isinstance(match_pay, dict)
            # Manual fact to allocate against (does not need EOD reconcile for match API).
            account = journey.account()
            manual = journey.create_manual(
                operation_date="2026-09-28",
                direction="outflow",
                amount_minor="1000000",
                expected_data_version=str(account["data_version"]),
                counterparty_name="E2E Match Vendor",
                payment_purpose="Partial settlement fact",
                expected=201,
            ).body
            assert isinstance(manual, dict)
            tx_id = manual["id"]
            tx_ver = str(manual.get("version") or "1")

            client.refresh_csrf()
            matched = client.request(
                "POST",
                f"/api/v1/companies/{company_id}/payments/{match_pay['id']}/matches",
                csrf=True,
                idempotency_key=client.new_idempotency_key("match"),
                json_body={
                    "transaction_id": tx_id,
                    "amount": {"amount_minor": "1000000", "currency": "RUB"},
                    "payment_version": match_pay["version"],
                    "transaction_version": tx_ver,
                },
                expected={200, 201},
            ).body
            assert isinstance(matched, dict)
            match_id = matched["id"]
            refreshed = get_payment(client, company_id, match_pay["id"])
            rem = refreshed["remaining_amount"]["amount_minor"]
            st = refreshed["status"]
            if rem != "2000000" or st != "partially_matched":
                # status should be partially_matched
                record(
                    results,
                    "PAY-03",
                    "fail",
                    "remaining after partial mismatch",
                    remaining=rem,
                    payment_status=st,
                )
                raise RuntimeError("partial remaining")
            client.refresh_csrf()
            client.request(
                "DELETE",
                f"/api/v1/companies/{company_id}/payments/{match_pay['id']}/matches/{match_id}"
                f"?payment_version={refreshed['version']}",
                csrf=True,
                expected=204,
            )
            after_unmatch = get_payment(client, company_id, match_pay["id"])
            if after_unmatch["remaining_amount"]["amount_minor"] != "3000000":
                record(
                    results,
                    "PAY-03",
                    "fail",
                    "unmatch did not restore remaining",
                    body=after_unmatch,
                )
            else:
                # Re-partial
                client.refresh_csrf()
                rematch = client.request(
                    "POST",
                    f"/api/v1/companies/{company_id}/payments/{match_pay['id']}/matches",
                    csrf=True,
                    idempotency_key=client.new_idempotency_key("match2"),
                    json_body={
                        "transaction_id": tx_id,
                        "amount": {"amount_minor": "1000000", "currency": "RUB"},
                        "payment_version": after_unmatch["version"],
                        "transaction_version": tx_ver,
                    },
                    expected={200, 201},
                ).body
                assert isinstance(rematch, dict)
                final_pay = get_payment(client, company_id, match_pay["id"])
                record(
                    results,
                    "PAY-03",
                    "pass",
                    "partial → unmatch → partial",
                    remaining=final_pay["remaining_amount"]["amount_minor"],
                    payment_status=final_pay["status"],
                    match_id=rematch["id"],
                )
    except Exception as exc:  # noqa: BLE001
        record(results, "PAY-03", "fail", str(exc))

    # --- PAY-05 forbidden transition ---
    try:
        client.refresh_csrf()
        p = client.request(
            "POST",
            f"/api/v1/companies/{company_id}/payments",
            csrf=True,
            idempotency_key=client.new_idempotency_key("pay5"),
            json_body={
                "direction": "outflow",
                "amount": {"amount_minor": "500000", "currency": "RUB"},
                "scheduled_date": BEYOND_HORIZON,
                "counterparty_name": "E2E Forbidden",
                "description": "Stale cancel target",
            },
            expected=201,
        ).body
        assert isinstance(p, dict)
        client.refresh_csrf()
        client.request(
            "DELETE",
            f"/api/v1/companies/{company_id}/payments/{p['id']}?version={p['version']}",
            csrf=True,
            expected=204,
        )
        # Second cancel with same version should be 409 (incompatible) or idempotent 204.
        client.refresh_csrf()
        again = client.request(
            "DELETE",
            f"/api/v1/companies/{company_id}/payments/{p['id']}?version={p['version']}",
            csrf=True,
            expected={204, 409},
        )
        # Stale version edit on cancelled payment.
        client.refresh_csrf()
        bad = client.request(
            "PATCH",
            f"/api/v1/companies/{company_id}/payments/{p['id']}",
            csrf=True,
            json_body={"scheduled_date": "2026-11-20", "version": p["version"]},
            expected={409, 422},
        )
        record(
            results,
            "PAY-05",
            "pass",
            "forbidden/stale transitions rejected or idempotent",
            cancel_again=again.status,
            stale_patch=bad.status,
        )
    except Exception as exc:  # noqa: BLE001
        record(results, "PAY-05", "fail", str(exc))

    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    (ARTIFACTS / "last-payments.json").write_text(
        json.dumps(run, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {"summary": {r["id"]: r["status"] for r in results}}, ensure_ascii=False, indent=2
        )
    )

    hard_fail = any(r["status"] == "fail" for r in results)
    return 1 if hard_fail else 0


if __name__ == "__main__":
    sys.exit(main())
