#!/usr/bin/env bash
set -euo pipefail
umask 077

# Administrator bootstrap, never invoked by GitHub Actions.
if [[ $EUID != 0 || $# != 2 || ! -f $2 ]]; then
    echo "Usage as root: install.sh <locally available immutable image> <public-key-file>" >&2
    exit 2
fi
image=$1
public_key=$(realpath "$2")
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
for command in docker certbot sudo curl useradd visudo; do
    command -v "$command" >/dev/null
done
docker image inspect "$image" >/dev/null
ssh-keygen -lf "$public_key" >/dev/null

install -d -m 755 /opt/saldo-frontend /opt/saldo-frontend/nginx /opt/saldo-frontend/acme
install -d -m 700 /opt/saldo-frontend/letsencrypt /opt/saldo-frontend/certbot-work /opt/saldo-frontend/certbot-logs
install -m 644 "$source_dir/compose.yaml" /opt/saldo-frontend/compose.yaml
install -m 644 "$source_dir/nginx-http.conf" /opt/saldo-frontend/nginx/http.conf
install -m 755 "$source_dir/deploy.sh" /usr/local/sbin/saldo-frontend-deploy
install -m 755 "$source_dir/renew.sh" /usr/local/sbin/saldo-frontend-cert-renew

if ! id saldo-deploy >/dev/null 2>&1; then
    useradd --system --create-home --home-dir /var/lib/saldo-deploy --shell /bin/sh saldo-deploy
    # Unlock only public-key authentication; password authentication cannot match '*'.
    usermod --password '*' saldo-deploy
fi
install -d -m 755 /usr/local/libexec
cat > /usr/local/libexec/saldo-frontend-ssh <<'WRAPPER'
#!/bin/sh
exec sudo -n /usr/local/sbin/saldo-frontend-deploy "$SSH_ORIGINAL_COMMAND"
WRAPPER
chmod 755 /usr/local/libexec/saldo-frontend-ssh
install -d -o saldo-deploy -g saldo-deploy -m 700 /var/lib/saldo-deploy/.ssh
printf 'restrict,command="/usr/local/libexec/saldo-frontend-ssh" %s\n' "$(cat "$public_key")" \
    > /var/lib/saldo-deploy/.ssh/authorized_keys
chown saldo-deploy:saldo-deploy /var/lib/saldo-deploy/.ssh/authorized_keys
chmod 600 /var/lib/saldo-deploy/.ssh/authorized_keys
printf 'saldo-deploy ALL=(root) NOPASSWD: /usr/local/sbin/saldo-frontend-deploy\n' \
    > /etc/sudoers.d/saldo-frontend
chmod 440 /etc/sudoers.d/saldo-frontend
visudo -cf /etc/sudoers.d/saldo-frontend

cd /opt/saldo-frontend
# Preserve a running deployment when repeating configuration installation.
if [[ ! -f .env ]]; then
    printf 'FRONTEND_IMAGE=%s\n' "$image" > .env
fi
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --wait --wait-timeout 120

# Only our certificate storage is touched; existing host certificates are isolated.
certbot certonly --non-interactive --agree-tos --register-unsafely-without-email \
    --webroot -w /opt/saldo-frontend/acme -d saldo.brokebyte.me \
    --config-dir /opt/saldo-frontend/letsencrypt \
    --work-dir /opt/saldo-frontend/certbot-work \
    --logs-dir /opt/saldo-frontend/certbot-logs
install -m 644 "$source_dir/nginx-https.conf" /opt/saldo-frontend/nginx/https.conf
docker compose --env-file .env -f compose.yaml exec -T nginx nginx -t
docker compose --env-file .env -f compose.yaml exec -T nginx nginx -s reload
install -m 644 "$source_dir/saldo-frontend-cert-renew.service" /etc/systemd/system/
install -m 644 "$source_dir/saldo-frontend-cert-renew.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now saldo-frontend-cert-renew.timer
curl --fail --silent --show-error --retry 5 \
    --resolve saldo.brokebyte.me:443:127.0.0.1 https://saldo.brokebyte.me/login >/dev/null
