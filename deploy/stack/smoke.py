#!/usr/bin/env python3
"""Private operator credentials exercise public API→Auth/Core→ML, no mocks."""

import http.cookiejar
import json
import time
import urllib.request
import uuid
from pathlib import Path


def main():
    credentials = json.loads(Path("/opt/saldo-stack/smoke-account.json").read_text())
    origin = "https://saldo.brokebyte.me"
    opener = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
    )

    def call(path, body=None, method=None):
        headers = {"Origin": origin, "Content-Type": "application/json"}
        if body is not None or method == "POST":
            headers["X-CSRF-Token"] = call("/auth/csrf")["csrf_token"]
            headers["Idempotency-Key"] = str(uuid.uuid4())
        request = urllib.request.Request(
            origin + "/api/v1" + path,
            data=None if body is None else json.dumps(body).encode(),
            headers=headers,
            method=method,
        )
        with opener.open(request, timeout=15) as response:
            data = response.read()
            return json.loads(data) if data else None

    call("/auth/login", {"email": credentials["email"], "password": credentials["password"]})
    try:
        company = credentials["company_id"]
        companies = call("/companies")
        if company not in [item["id"] for item in companies["items"]]:
            raise RuntimeError("Smoke membership unavailable")
        base = "/companies/" + company
        account = call(base + "/account")
        accepted = call(
            base + "/forecasts",
            {"cutoff_date": account["balance_as_of"], "horizon_days": 30},
        )
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            forecast = call(base + "/forecasts/" + accepted["id"])
            if forecast["status"] == "failed":
                raise RuntimeError("Financial deployment smoke failed")
            if forecast["status"] == "succeeded":
                if len(forecast["result"]["daily_points"]) != 30:
                    raise RuntimeError("Incomplete financial projection")
                print("PASS public Auth→API→Core→PostgreSQL→ML financial smoke")
                return
            time.sleep(1)
        raise RuntimeError("Financial deployment smoke timed out")
    finally:
        call("/auth/logout", method="POST")


if __name__ == "__main__":
    main()
