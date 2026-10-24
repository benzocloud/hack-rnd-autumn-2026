#!/usr/bin/env bash
set -euo pipefail

cd /opt/saldo-frontend
certbot renew --non-interactive \
    --config-dir /opt/saldo-frontend/letsencrypt \
    --work-dir /opt/saldo-frontend/certbot-work \
    --logs-dir /opt/saldo-frontend/certbot-logs \
    --deploy-hook 'docker compose --env-file /opt/saldo-frontend/.env -f /opt/saldo-frontend/compose.yaml exec -T nginx nginx -s reload'
