"""Shared bootstrap helpers for financial E2E HTTP suites."""

from __future__ import annotations

import json
import secrets
import time
import uuid
from pathlib import Path
from typing import Any

from .http_client import ApiClient, Response

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "fixtures"


def load_stack_env() -> dict[str, Any]:
    state_path = Path(__import__("os").environ.get("SALDO_E2E_STACK_STATE", ""))
    if not state_path.is_file():
        raise RuntimeError("SALDO_E2E_STACK_STATE missing; source artifacts/stack-*/env.sh")
    return json.loads(state_path.read_text(encoding="utf-8"))


def next_demo_email(state: dict[str, Any]) -> str:
    state_path = Path(__import__("os").environ["SALDO_E2E_STACK_STATE"])
    pool = state.get("demo_emails") or []
    used_path = state_path.parent / "used-emails.json"
    used = json.loads(used_path.read_text(encoding="utf-8")) if used_path.is_file() else []
    for candidate in pool:
        if candidate not in used:
            used.append(candidate)
            used_path.write_text(json.dumps(used, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            return candidate
    raise RuntimeError("demo email pool exhausted; restart stack with a new suffix")


def multipart_import(account_id: str, csv_bytes: bytes, filename: str) -> tuple[bytes, str]:
    boundary = f"----saldo{uuid.uuid4().hex}"

    def field(name: str, value: str) -> bytes:
        return (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="{name}"\r\n\r\n'
            f"{value}\r\n"
        ).encode()

    body = b"".join(
        [
            field("adapter", "dataset_v1"),
            field("source_company_id", "C001"),
            field("account_id", account_id),
            field("currency", "RUB"),
            (
                f"--{boundary}\r\n"
                f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
                f"Content-Type: text/csv\r\n\r\n"
            ).encode()
            + csv_bytes
            + b"\r\n",
            f"--{boundary}--\r\n".encode(),
        ]
    )
    return body, f"multipart/form-data; boundary={boundary}"


def poll(client: ApiClient, path: str, *, is_done, timeout_s: float = 120.0, interval_s: float = 0.5) -> Any:
    deadline = time.monotonic() + timeout_s
    last = None
    while time.monotonic() < deadline:
        response = client.request("GET", path, expected={200, 202})
        last = response.body
        if is_done(response.body):
            return response.body
        time.sleep(interval_s)
    raise TimeoutError(f"poll {path} timed out; last={last!r}")


class Journey:
    def __init__(self, client: ApiClient, email: str, password: str) -> None:
        self.client = client
        self.email = email
        self.password = password
        self.company_id: str | None = None
        self.account_id: str | None = None
        self.import_id: str | None = None
        self.forecast_id: str | None = None

    @classmethod
    def register_login(cls, client: ApiClient, email: str) -> Journey:
        password = secrets.token_urlsafe(18) + "aA9!"
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
                "display_name": f"E2E {email.split('@', 1)[0]}",
                "personal_data_consent": {
                    "accepted": True,
                    "document_version": consent["document_version"],
                    "document_sha256": consent["document_sha256"],
                },
            },
            expected=202,
        ).body
        assert isinstance(registered, dict)
        client.refresh_csrf()
        client.request(
            "POST",
            "/api/v1/auth/verify-email",
            csrf=True,
            json_body={"challenge_id": registered["challenge_id"], "code": "000000"},
            expected=200,
        )
        client.refresh_csrf()
        client.request(
            "POST",
            "/api/v1/auth/login",
            csrf=True,
            json_body={"email": email, "password": password},
            expected=200,
        )
        return cls(client, email, password)

    def create_company(self) -> str:
        self.client.refresh_csrf()
        created = self.client.request(
            "POST",
            "/api/v1/companies",
            csrf=True,
            idempotency_key=self.client.new_idempotency_key("company"),
            json_body={
                "name": f"E2E {uuid.uuid4().hex[:8]}",
                "inn": None,
                "currency": "RUB",
                "timezone": "Europe/Moscow",
            },
            expected=201,
        ).body
        assert isinstance(created, dict)
        self.company_id = created["id"]
        account = self.client.request(
            "GET",
            f"/api/v1/companies/{self.company_id}/account",
            expected=200,
        ).body
        assert isinstance(account, dict)
        self.account_id = account["id"]
        return self.company_id

    def upload_csv(self, path: Path, *, idempotency_key: str | None = None) -> dict[str, Any]:
        assert self.company_id and self.account_id
        body, content_type = multipart_import(self.account_id, path.read_bytes(), path.name)
        self.client.refresh_csrf()
        uploaded = self.client.request(
            "POST",
            f"/api/v1/companies/{self.company_id}/imports",
            data=body,
            content_type=content_type,
            csrf=True,
            idempotency_key=idempotency_key or self.client.new_idempotency_key("import"),
            expected={200, 201, 202},
        )
        assert isinstance(uploaded.body, dict)
        self.import_id = uploaded.body["id"]
        return uploaded.body

    def wait_preview(self) -> dict[str, Any]:
        assert self.company_id and self.import_id
        body = poll(
            self.client,
            f"/api/v1/companies/{self.company_id}/imports/{self.import_id}",
            is_done=lambda b: isinstance(b, dict)
            and b.get("status") in {"preview_ready", "failed", "rejected", "applied"},
        )
        assert isinstance(body, dict)
        return body

    def commit_import(self, preview: dict[str, Any] | None = None) -> dict[str, Any]:
        assert self.company_id and self.import_id
        preview = preview or self.wait_preview()
        preview_block = preview.get("preview") if isinstance(preview.get("preview"), dict) else preview
        self.client.refresh_csrf()
        committed = self.client.request(
            "POST",
            f"/api/v1/companies/{self.company_id}/imports/{self.import_id}/commit",
            csrf=True,
            idempotency_key=self.client.new_idempotency_key("commit"),
            json_body={
                "version": str(preview["version"]),
                "expected_data_version": str(preview_block["base_data_version"]),
                "acknowledge_warnings": True,
            },
            expected=200,
        ).body
        assert isinstance(committed, dict)
        return committed

    def account(self) -> dict[str, Any]:
        assert self.company_id
        body = self.client.request(
            "GET",
            f"/api/v1/companies/{self.company_id}/account",
            expected=200,
        ).body
        assert isinstance(body, dict)
        return body

    def list_transactions(self, *, limit: int = 100) -> dict[str, Any]:
        assert self.company_id
        body = self.client.request(
            "GET",
            f"/api/v1/companies/{self.company_id}/transactions?limit={limit}",
            expected=200,
        ).body
        assert isinstance(body, dict)
        return body

    def transaction_count(self) -> int | None:
        body = self.list_transactions(limit=100)
        items = body.get("items") or body.get("transactions")
        if not isinstance(items, list):
            return None
        # Prefer total if present; otherwise page length (bounded).
        for key in ("total", "total_count", "count"):
            if isinstance(body.get(key), int):
                return body[key]
        return len(items)

    def create_manual(
        self,
        *,
        operation_date: str,
        direction: str,
        amount_minor: str,
        expected_data_version: str,
        counterparty_name: str = "E2E Manual",
        payment_purpose: str = "Manual supplement after import",
        counterparty_inn: str = "",
        idempotency_key: str | None = None,
        expected: int | set[int] = 201,
    ) -> Response:
        assert self.company_id
        self.client.refresh_csrf()
        payload: dict[str, Any] = {
            "operation_date": operation_date,
            "direction": direction,
            "amount": {"amount_minor": amount_minor, "currency": "RUB"},
            "counterparty_name": counterparty_name,
            "payment_purpose": payment_purpose,
            "expected_data_version": expected_data_version,
        }
        # Empty string fails OpenAPI validation; omit or send a real INN.
        if counterparty_inn:
            payload["counterparty_inn"] = counterparty_inn
        return self.client.request(
            "POST",
            f"/api/v1/companies/{self.company_id}/transactions",
            csrf=True,
            idempotency_key=idempotency_key or self.client.new_idempotency_key("manual"),
            json_body=payload,
            expected=expected,
        )

    def create_forecast(self, cutoff: str = "2026-09-29") -> dict[str, Any]:
        assert self.company_id
        self.client.refresh_csrf()
        created = self.client.request(
            "POST",
            f"/api/v1/companies/{self.company_id}/forecasts",
            csrf=True,
            idempotency_key=self.client.new_idempotency_key("forecast"),
            json_body={"cutoff_date": cutoff, "horizon_days": 30},
            expected={200, 201, 202},
        ).body
        assert isinstance(created, dict)
        self.forecast_id = created["id"]
        final = poll(
            self.client,
            f"/api/v1/companies/{self.company_id}/forecasts/{self.forecast_id}",
            timeout_s=120,
            is_done=lambda b: isinstance(b, dict)
            and b.get("status") in {"succeeded", "failed", "rejected", "cancelled"},
        )
        assert isinstance(final, dict)
        return final
