#!/usr/bin/env python3
"""One-time operator-only synthetic fixture, never a public onboarding shortcut."""

import datetime
import http.cookiejar
import json
import os
import secrets
import subprocess
import urllib.request
from pathlib import Path


def main():
    os.umask(0o077)
    target = Path("/opt/saldo-stack/smoke-account.json")
    if os.geteuid() != 0 or target.exists():
        raise SystemExit("Root-only first provisioning; existing fixture preserved")
    origin = "https://saldo.brokebyte.me"
    opener = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
    )

    def call(path, body=None):
        headers = {"Origin": origin, "Content-Type": "application/json"}
        if body is not None:
            headers["X-CSRF-Token"] = call("/auth/csrf")["csrf_token"]
        request = urllib.request.Request(
            origin + "/api/v1" + path,
            data=None if body is None else json.dumps(body).encode(),
            headers=headers,
        )
        with opener.open(request, timeout=15) as response:
            return json.load(response)

    password = secrets.token_urlsafe(24) + "aA9!"
    credentials = {"email": "owner@saldo.test", "password": password}
    document = call("/auth/personal-data-consent")
    challenge = call(
        "/auth/register",
        {
            **credentials,
            "display_name": "Демонстрационный владелец",
            "personal_data_consent": {
                "accepted": True,
                "document_version": document["document_version"],
                "document_sha256": document["document_sha256"],
            },
        },
    )
    call("/auth/verify-email", {"challenge_id": challenge["challenge_id"], "code": "000000"})
    session = call("/auth/login", credentials)
    user = session["user"]["id"]

    def sql(database, query):
        return subprocess.check_output(
            [
                "docker",
                "compose",
                "exec",
                "-T",
                "postgres",
                "psql",
                "-U",
                "saldo_admin",
                "-d",
                database,
                "-v",
                "ON_ERROR_STOP=1",
                "-At",
            ],
            input=query,
            text=True,
            cwd="/opt/saldo-stack",
        ).strip()

    company, account = sql("saldo_core", "SELECT uuidv7(),uuidv7();").split("|")
    cutoff = (
        datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=3))).date()
        - datetime.timedelta(days=1)
    ).isoformat()
    sql(
        "saldo_core",
        f"""BEGIN;
INSERT INTO core_companies(id) VALUES('{company}');
INSERT INTO core_accounts(id,company_id,currency,data_version,factual_kind) VALUES('{account}','{company}','RUB',183,'manual');
INSERT INTO core_company_profiles VALUES('{company}','{account}','Демо: Малый бизнес — синтетические данные','7707083893','RUB','Europe/Moscow',1,now());
INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,source_kind,counterparty_inn,counterparty_name,purpose,version,created_data_version)
SELECT '{company}','{account}',uuidv7(),'{cutoff}'::date-n,dir,amount,'RUB','manual','','Синтетический контрагент','Демонстрационная операция, не реальные данные',1,1 FROM generate_series(0,89)n CROSS JOIN (VALUES('inflow',100000),('outflow',400000))v(dir,amount);
INSERT INTO core_balances(company_id,account_id,balance_date,amount_minor,currency,source_kind,recorded_at,note,created_data_version) VALUES('{company}','{account}','{cutoff}',5000000,'RUB','manual',now(),'Синтетический наблюдаемый EOD',183);
INSERT INTO core_history_coverage VALUES('{company}','{account}',183,'{cutoff}'::date-89,'{cutoff}','Complete synthetic demo fixture: 180 known operations and explicit zero-missing source days');
COMMIT;""",
    )
    sql(
        "saldo_auth",
        f"INSERT INTO memberships(company_id,user_id,role) VALUES('{company}','{user}','owner');",
    )
    credentials["company_id"] = company
    target.write_text(json.dumps(credentials))
    print("Synthetic demo fixture prepared; credentials stored root-only, not printed")


if __name__ == "__main__":
    main()
