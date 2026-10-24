#!/usr/bin/env bash
set -euo pipefail
umask 077

[[ $EUID == 0 ]]
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test -x /usr/local/sbin/saldo-frontend-deploy
id saldo-deploy >/dev/null
docker network inspect saldo-frontend_default >/dev/null
test -f /opt/saldo-frontend/nginx/https.conf

install -d -m 755 /opt/saldo-landing
install -m 644 "$source_dir/compose.yaml" /opt/saldo-landing/compose.yaml
install -m 755 "$source_dir/deploy.sh" /usr/local/sbin/saldo-landing-deploy
if [[ ! -f /opt/saldo-landing/.env ]]; then
    printf 'LANDING_IMAGE=\n' > /opt/saldo-landing/.env
fi

cat > /usr/local/libexec/saldo-frontend-ssh <<'WRAPPER'
#!/bin/sh
case "$SSH_ORIGINAL_COMMAND" in
    deploy\ *) exec sudo -n /usr/local/sbin/saldo-frontend-deploy "$SSH_ORIGINAL_COMMAND" ;;
    deploy-landing\ *) exec sudo -n /usr/local/sbin/saldo-landing-deploy "$SSH_ORIGINAL_COMMAND" ;;
    *) exit 2 ;;
esac
WRAPPER
chmod 755 /usr/local/libexec/saldo-frontend-ssh
printf 'saldo-deploy ALL=(root) NOPASSWD: /usr/local/sbin/saldo-landing-deploy\n' > /etc/sudoers.d/saldo-landing
chmod 440 /etc/sudoers.d/saldo-landing
visudo -cf /etc/sudoers.d/saldo-landing

frontend() { docker compose --project-directory /opt/saldo-frontend --env-file /opt/saldo-frontend/.env -f /opt/saldo-frontend/compose.yaml "$@"; }
backup=$(mktemp /opt/saldo-frontend/nginx/.https-backup.XXXXXX)
cp /opt/saldo-frontend/nginx/https.conf "$backup"
install -m 644 "$source_dir/../frontend/nginx-https.conf" /opt/saldo-frontend/nginx/.https-candidate
mv /opt/saldo-frontend/nginx/.https-candidate /opt/saldo-frontend/nginx/https.conf
if frontend exec -T nginx nginx -t && frontend exec -T nginx nginx -s reload && \
    curl --fail --silent --show-error --max-time 15 --retry 3 \
        --resolve saldo.brokebyte.me:443:127.0.0.1 https://saldo.brokebyte.me/login >/dev/null; then
    rm -f "$backup"
else
    mv "$backup" /opt/saldo-frontend/nginx/https.conf
    frontend exec -T nginx nginx -t && frontend exec -T nginx nginx -s reload
    echo 'Restored previous Saldo Nginx configuration' >&2
    exit 1
fi
