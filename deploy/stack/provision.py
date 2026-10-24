#!/usr/bin/env python3
"""Root-only first-time Saldo credentials/PKI; never prints secret values."""

import base64
import os
import secrets
import subprocess
from pathlib import Path


def main():
    if os.geteuid() != 0:
        raise SystemExit("Run as root on the Saldo deployment host")
    os.umask(0o077)
    root = Path("/opt/saldo-stack")
    root.mkdir(mode=0o700, exist_ok=True)
    if (root / "auth.env").exists():
        raise SystemExit("Already provisioned; preserve existing secrets and database")
    authority = root / "pki/authority"
    authority.mkdir(parents=True, mode=0o700)

    def openssl(*args):
        subprocess.run(
            ["openssl", *args], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )

    openssl(
        "req",
        "-x509",
        "-newkey",
        "rsa:3072",
        "-nodes",
        "-keyout",
        str(authority / "ca.key"),
        "-out",
        str(authority / "ca.pem"),
        "-days",
        "365",
        "-subj",
        "/CN=Saldo private services CA",
    )
    for name, dns in {
        "api": "api,saldo-api",
        "auth": "auth",
        "core": "core",
        "ml": "ml",
        "db": "postgres",
    }.items():
        folder = root / "pki" / name
        folder.mkdir(mode=0o750)
        folder.chmod(0o750)
        (folder / "ca.pem").write_bytes((authority / "ca.pem").read_bytes())
        extensions = authority / (name + ".ext")
        extensions.write_text(
            "subjectAltName="
            + ",".join("DNS:" + d for d in dns.split(","))
            + ",URI:spiffe://saldo.local/"
            + name
            + "\nextendedKeyUsage=serverAuth,clientAuth\n"
        )
        openssl(
            "req",
            "-new",
            "-newkey",
            "rsa:2048",
            "-nodes",
            "-keyout",
            str(folder / "server.key"),
            "-out",
            str(authority / (name + ".csr")),
            "-subj",
            "/CN=" + name,
        )
        openssl(
            "x509",
            "-req",
            "-in",
            str(authority / (name + ".csr")),
            "-CA",
            str(authority / "ca.pem"),
            "-CAkey",
            str(authority / "ca.key"),
            "-CAcreateserial",
            "-out",
            str(folder / "server.pem"),
            "-days",
            "90",
            "-extfile",
            str(extensions),
        )
        uid = 70 if name == "db" else 65532
        os.chown(folder, 0, uid)
        for file in folder.iterdir():
            os.chown(file, uid, uid)
            file.chmod(0o600 if file.suffix == ".key" else 0o644)

    def env(name, values):
        (root / name).write_text("".join(f"{k}={v}\n" for k, v in values.items()))

    auth_password, core_password = secrets.token_hex(24), secrets.token_hex(24)
    env(
        "postgres.env",
        {
            "POSTGRES_USER": "saldo_admin",
            "POSTGRES_PASSWORD": secrets.token_hex(32),
            "POSTGRES_DB": "postgres",
            "AUTH_DB_PASSWORD": auth_password,
            "CORE_DB_PASSWORD": core_password,
            "POSTGRES_INITDB_ARGS": "--auth-host=scram-sha-256",
        },
    )
    env(
        "auth.env",
        {
            "AUTH_ENV": "local",
            "AUTH_DATABASE_URL": f"postgres://saldo_auth:{auth_password}@postgres:5432/saldo_auth?sslmode=verify-full&sslrootcert=/tls/ca.pem",
            "AUTH_HTTP_ADDRESS": "0.0.0.0:8082",
            "AUTH_GRPC_ADDRESS": "0.0.0.0:50053",
            "AUTH_GRPC_CERT_FILE": "/tls/server.pem",
            "AUTH_GRPC_KEY_FILE": "/tls/server.key",
            "AUTH_GRPC_CA_FILE": "/tls/ca.pem",
            "AUTH_CODE_HMAC_KEY": base64.urlsafe_b64encode(secrets.token_bytes(32))
            .decode()
            .rstrip("="),
            "AUTH_CSRF_HMAC_KEY": base64.urlsafe_b64encode(secrets.token_bytes(32))
            .decode()
            .rstrip("="),
            "AUTH_QUERY_TIMEOUT": "5s",
            "AUTH_DEMO_EMAIL_VERIFICATION": "true",
            "AUTH_DEMO_TEST_EMAILS": "owner@saldo.test,financier@saldo.test",
            "AUTH_DB_MAX_CONNECTIONS": "5",
        },
    )
    env(
        "core.env",
        {
            "CORE_POSTGRES_HOST": "postgres",
            "CORE_POSTGRES_PORT": "5432",
            "CORE_POSTGRES_USER": "saldo_core",
            "CORE_POSTGRES_PASSWORD": core_password,
            "CORE_POSTGRES_DATABASE": "saldo_core",
            "CORE_POSTGRES_SSL_MODE": "verify-full",
            "PGSSLROOTCERT": "/tls/ca.pem",
            "CORE_POSTGRES_MAX_CONNS": "5",
            "CORE_POSTGRES_MAX_IDLE_CONNS": "1",
            "CORE_POSTGRES_QUERY_TIMEOUT": "5s",
            "CORE_GRPC_ADDRESS": "0.0.0.0:50052",
            "CORE_AUTH_ADDRESS": "auth:50053",
            "CORE_AUTH_SERVER_NAME": "auth",
            "CORE_ML_ADDRESS": "ml:50051",
            "CORE_ML_SERVER_NAME": "ml",
            "CORE_MTLS_CA_FILE": "/tls/ca.pem",
            "CORE_MTLS_CERT_FILE": "/tls/server.pem",
            "CORE_MTLS_KEY_FILE": "/tls/server.key",
        },
    )
    env(
        "core-migrate.env",
        {
            "GOOSE_DRIVER": "postgres",
            "GOOSE_DBSTRING": f"postgres://saldo_core:{core_password}@postgres:5432/saldo_core?sslmode=verify-full&sslrootcert=/tls/ca.pem",
        },
    )
    env(
        "api.env",
        {
            "API_ADDRESS": "0.0.0.0:8080",
            "API_ORIGIN": "https://saldo.brokebyte.me",
            "API_AUTH_ADDRESS": "auth:50053",
            "API_AUTH_CA": "/tls/ca.pem",
            "API_AUTH_CERT": "/tls/server.pem",
            "API_AUTH_KEY": "/tls/server.key",
            "API_HTTP_CERT": "/tls/server.pem",
            "API_HTTP_KEY": "/tls/server.key",
            "API_CORE_ADDRESS": "core:50052",
            "API_CORE_SERVER_NAME": "core",
        },
    )
    print("Saldo private PKI and credentials provisioned; values not printed")


if __name__ == "__main__":
    main()
