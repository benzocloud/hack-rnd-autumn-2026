#!/usr/bin/env bash
set -euo pipefail
umask 077

[[ $EUID == 0 ]]
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test -x /usr/local/sbin/saldo-frontend-deploy
id saldo-deploy >/dev/null

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
