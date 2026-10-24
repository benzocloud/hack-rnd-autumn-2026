#!/bin/sh
set -eu
# Values are generated as ASCII hex, never accepted from HTTP or PR content.
psql --username "$POSTGRES_USER" --dbname postgres --set ON_ERROR_STOP=1 \
    --set=auth_password="$AUTH_DB_PASSWORD" --set=core_password="$CORE_DB_PASSWORD" <<'SQL'
CREATE ROLE saldo_auth LOGIN PASSWORD :'auth_password';
CREATE ROLE saldo_core LOGIN PASSWORD :'core_password';
CREATE DATABASE saldo_auth OWNER saldo_auth;
CREATE DATABASE saldo_core OWNER saldo_core;
REVOKE CONNECT ON DATABASE saldo_auth FROM PUBLIC;
REVOKE CONNECT ON DATABASE saldo_core FROM PUBLIC;
GRANT CONNECT ON DATABASE saldo_auth TO saldo_auth;
GRANT CONNECT ON DATABASE saldo_core TO saldo_core;
SQL
