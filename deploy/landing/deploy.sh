#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ $# != 1 || ! $1 =~ ^deploy-landing\ (sha256:[a-f0-9]{64})\ ([a-zA-Z0-9_-]+)$ ]]; then
    echo "Expected: deploy-landing sha256:<digest> <GitHub actor>" >&2
    exit 2
fi
digest=${BASH_REMATCH[1]}
actor=${BASH_REMATCH[2]}
IFS= read -r registry_token
[[ -n $registry_token ]]

cd /opt/saldo-landing
exec 9>/run/lock/saldo-landing-deploy.lock
flock -n 9
export DOCKER_CONFIG
DOCKER_CONFIG=$(mktemp -d /opt/saldo-landing/.docker-login.XXXXXX)
trap 'rm -rf "$DOCKER_CONFIG"' EXIT
printf '%s' "$registry_token" | docker login ghcr.io -u "$actor" --password-stdin
unset registry_token
image="ghcr.io/benzocloud/saldo-landing@$digest"
docker pull "$image"
previous=$(sed -n 's/^LANDING_IMAGE=//p' .env)

compose() { docker compose --env-file .env -f compose.yaml "$@"; }
write_image() {
    printf 'LANDING_IMAGE=%s\n' "$1" > .env.next
    mv .env.next .env
}

write_image "$image"
if compose up -d --no-deps --wait --wait-timeout 120 landing && \
    curl --fail --silent --show-error --retry 5 --retry-connrefused http://127.0.0.1:3005/ >/dev/null; then
    printf '%s\n' "$previous" > previous-image
    echo "Deployed $image"
else
    echo "Landing deployment failed; restoring previous state" >&2
    if [[ -n $previous ]]; then
        write_image "$previous"
        compose up -d --no-deps --wait --wait-timeout 120 landing
    else
        compose rm --stop --force landing
        write_image ""
    fi
    exit 1
fi
