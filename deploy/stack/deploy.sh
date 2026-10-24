#!/usr/bin/env bash
set -euo pipefail
umask 077

# Fixed root-owned operation; CI supplies immutable digests and a short-lived
# packages:read token on stdin. No repository code is executed as root.
if [[ $EUID != 0 || $# != 8 || $1 != deploy-stack || ! $2 =~ ^[a-f0-9]{40}$ || ! $8 =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo 'Expected deploy-stack <SHA> <api> <auth> <core> <ml> <frontend digest> <actor>' >&2
    exit 2
fi
for digest in "${@:3:5}"; do
    [[ $digest =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
done
revision=$2
actor=$8
IFS= read -r registry_token
[[ -n $registry_token ]]
cd /opt/saldo-stack
exec 9>/run/lock/saldo-stack-deploy.lock
flock -n 9
# Serialize frontend image/config changes with its existing deployment command.
exec 8>/run/lock/saldo-frontend-deploy.lock
flock -n 8
export DOCKER_CONFIG
DOCKER_CONFIG=$(mktemp -d /opt/saldo-stack/.docker-login.XXXXXX)
trap 'rm -rf "$DOCKER_CONFIG"' EXIT
printf '%s' "$registry_token" | docker login ghcr.io -u "$actor" --password-stdin >/dev/null
unset registry_token
names=(api auth core ml frontend-real)
digests=("${@:3:5}")
for n in "${!names[@]}"; do
    docker pull "ghcr.io/benzocloud/saldo-${names[n]}@${digests[n]}"
done
backup="releases/$revision-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup"
cp .env "$backup/backend.env"
cp /opt/saldo-frontend/.env "$backup/frontend.env"
cp /opt/saldo-frontend/nginx/https.conf "$backup/https.conf"
# Preserve a consistent pre-migration dump; never rollback data automatically.
docker compose exec -T postgres pg_dumpall -U saldo_admin > "$backup/postgres.sql"
chmod 600 "$backup/"*
compose() { docker compose --env-file .env -f compose.yaml "$@"; }
rollback() {
    cp "$backup/backend.env" .env
    cp "$backup/frontend.env" /opt/saldo-frontend/.env
    cp "$backup/https.conf" /opt/saldo-frontend/nginx/https.conf
    compose up -d auth core ml api
    (cd /opt/saldo-frontend && docker compose up -d --no-deps --wait --wait-timeout 120 frontend && docker compose exec -T nginx nginx -t && docker compose exec -T nginx nginx -s reload)
    echo 'Image/config rollback completed; database preserved for inspection' >&2
}
{
    printf 'API_IMAGE=ghcr.io/benzocloud/saldo-api@%s\n' "$3"
    printf 'AUTH_IMAGE=ghcr.io/benzocloud/saldo-auth@%s\n' "$4"
    printf 'CORE_IMAGE=ghcr.io/benzocloud/saldo-core@%s\n' "$5"
    printf 'ML_IMAGE=ghcr.io/benzocloud/saldo-ml@%s\n' "$6"
} > .env.next
mv .env.next .env
if ! (
    set -e
    compose config --quiet || exit 1
    compose run --rm --no-deps --entrypoint /app/migrate auth || exit 1
    compose run --rm --no-deps --env-from-file core-migrate.env --entrypoint /app/goose core -dir /app/migrations up || exit 1
    compose up -d auth core ml api || exit 1
    # Probe through the existing Saldo nginx with normal public TLS validation.
    for _ in $(seq 1 30); do
        if curl --fail --silent --show-error --resolve saldo.brokebyte.me:443:127.0.0.1 https://saldo.brokebyte.me/api/v1/auth/personal-data-consent >/dev/null; then
            break
        fi
        sleep 2
    done
    curl --fail --silent --show-error --resolve saldo.brokebyte.me:443:127.0.0.1 https://saldo.brokebyte.me/api/v1/auth/personal-data-consent >/dev/null || exit 1
    printf 'FRONTEND_IMAGE=ghcr.io/benzocloud/saldo-frontend-real@%s\n' "$7" > /opt/saldo-frontend/.env.next || exit 1
    mv /opt/saldo-frontend/.env.next /opt/saldo-frontend/.env || exit 1
    cd /opt/saldo-frontend || exit 1
    docker compose up -d --no-deps --wait --wait-timeout 120 frontend || exit 1
    docker compose exec -T nginx nginx -t || exit 1
    docker compose exec -T nginx nginx -s reload || exit 1
    curl --fail --silent --show-error --retry 5 --resolve saldo.brokebyte.me:443:127.0.0.1 https://saldo.brokebyte.me/login >/dev/null || exit 1
    python3 /opt/saldo-stack/smoke.py || exit 1
); then
    rollback
    exit 1
fi
printf '%s\n' "$revision" > deployed-revision
echo "Deployed Saldo release $revision"
