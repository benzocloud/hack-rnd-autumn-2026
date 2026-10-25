"""Minimal cookie+CSRF HTTP client for Saldo public API (stdlib only)."""

from __future__ import annotations

import http.cookiejar
import json
import ssl
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any
from uuid import uuid4


@dataclass
class Response:
    status: int
    headers: dict[str, str]
    body: Any
    raw: bytes


@dataclass
class ApiClient:
    origin: str
    timeout_s: float = 30.0
    user_agent: str = "saldo-financial-e2e/0.1"
    jar: http.cookiejar.CookieJar = field(default_factory=http.cookiejar.CookieJar)
    csrf_token: str | None = None
    _opener: urllib.request.OpenerDirector = field(init=False, repr=False)

    def __post_init__(self) -> None:
        self.origin = self.origin.rstrip("/")
        context = ssl.create_default_context()
        self._opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar),
            urllib.request.HTTPSHandler(context=context),
        )

    def _url(self, path: str) -> str:
        if path.startswith("http://") or path.startswith("https://"):
            return path
        if not path.startswith("/"):
            path = "/" + path
        return self.origin + path

    def request(
        self,
        method: str,
        path: str,
        *,
        json_body: Any | None = None,
        data: bytes | None = None,
        content_type: str | None = None,
        idempotency_key: str | None = None,
        csrf: bool = False,
        expected: int | set[int] | None = None,
    ) -> Response:
        headers = {
            "Accept": "application/json",
            "Origin": self.origin,
            "User-Agent": self.user_agent,
        }
        body = data
        if json_body is not None:
            body = json.dumps(json_body, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if content_type is not None:
            headers["Content-Type"] = content_type
        if csrf:
            if not self.csrf_token:
                raise RuntimeError("CSRF required but csrf_token is empty; call refresh_csrf() first")
            headers["X-CSRF-Token"] = self.csrf_token
        if idempotency_key is not None:
            headers["Idempotency-Key"] = idempotency_key

        request = urllib.request.Request(self._url(path), data=body, headers=headers, method=method.upper())
        try:
            with self._opener.open(request, timeout=self.timeout_s) as handle:
                raw = handle.read()
                status = handle.getcode() or 0
                resp_headers = {k.lower(): v for k, v in handle.headers.items()}
        except urllib.error.HTTPError as err:
            raw = err.read()
            status = err.code
            resp_headers = {k.lower(): v for k, v in err.headers.items()} if err.headers else {}

        parsed: Any
        if raw:
            try:
                parsed = json.loads(raw.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                parsed = {"_non_json": True, "bytes": len(raw)}
        else:
            parsed = None

        response = Response(status=status, headers=resp_headers, body=parsed, raw=raw)
        if expected is not None:
            allowed = {expected} if isinstance(expected, int) else set(expected)
            if status not in allowed:
                raise AssertionError(
                    f"{method.upper()} {path} expected {sorted(allowed)}, got {status}: {parsed!r}"
                )
        return response

    def refresh_csrf(self) -> str:
        response = self.request("GET", "/api/v1/auth/csrf", expected=200)
        token = response.body.get("csrf_token") if isinstance(response.body, dict) else None
        if not isinstance(token, str) or not token:
            raise AssertionError(f"csrf_token missing: {response.body!r}")
        self.csrf_token = token
        return token

    def new_idempotency_key(self, prefix: str = "e2e") -> str:
        return f"{prefix}-{uuid4()}"
