# Saldo frontend: mock deployment

Цель: публичная демонстрация на `https://saldo.brokebyte.me` с локальными
моками, без обращений к API/ML. DNS: A `saldo` → `77.90.183.11`.

## Состав

- `/opt/saldo-frontend`: отдельный Compose project `saldo-frontend`.
- `frontend`: Next.js standalone, `developerMode=mock` **при сборке**.
  Порт 3000 доступен только внутри Docker network, наружу не публикуется.
- `nginx`: единственные опубликованные порты — 80/443. HTTP перенаправляется
  на HTTPS; `/.well-known/acme-challenge/` обслуживается локально.
- Let's Encrypt: отдельные config/work/log директории только этого проекта.
  Timer `saldo-frontend-cert-renew.timer` проверяет сертификат дважды в день
  и перезагружает только Nginx Saldo после успешного продления.
- SSH user `saldo-deploy` не входит в Docker group. Его единственный ключ
  ограничен forced command, без shell/PTY/forwarding; sudo разрешает только
  root-owned `saldo-frontend-deploy`. Принимается конкретный SHA-256 digest
  фиксированного образа `ghcr.io/benzocloud/saldo-frontend-mock`.

Существующие контейнеры, сертификаты, сети, firewall и SSH-ключи других
сервисов не меняются. На старте нужно убедиться, что порты 80/443 свободны.

## Первичная установка администратором

На сервере нужны Docker Compose, certbot, sudo, curl и разрешённые порты
80/443. Отдельный SSH-ключ генерируется для CI; приватная часть не попадает
в репозиторий. Сборка выполняется вне сервера, чтобы не занимать его CPU/RAM:

```bash
docker build --platform linux/amd64 --build-arg developerMode=mock \
  --label me.brokebyte.saldo.mode=mock -t saldo-frontend-mock:bootstrap -f frontend/Dockerfile frontend
docker save saldo-frontend-mock:bootstrap | gzip > /tmp/saldo-frontend.tar.gz
# Передать архив образа, deploy/frontend и только публичный SSH-ключ на сервер.
# На сервере, от root:
gunzip -c /tmp/saldo-frontend.tar.gz | docker load
image=$(docker image inspect saldo-frontend-mock:bootstrap --format '{{.Id}}')
bash /path/to/deploy/frontend/install.sh "$image" /path/to/ci-key.pub
```

Installer использует отдельную ACME-регистрацию без email; при желании контакт
можно добавить в её account через certbot. Нужен уже разрешающийся DNS.
Installer не делает `docker compose down`, prune, reboot или host-wide reload.
Изменения конфигурации `deploy/frontend/` устанавливаются администратором;
CI не заменяет root-owned shell scripts, Nginx config и Compose config.

## GitHub Actions

Workflow `.github/workflows/frontend.yml`:

1. PR: lint/typecheck, отдельные tests и Docker build/smoke-test. Без deployment.
2. `main`: те же проверки, публикация `sha-<commit>` в GHCR и deployment
   **по digest**, без `latest`.
3. `workflow_dispatch` разрешает повторный deployment только на `main`.

Environment `frontend-mock` разрешает deployment только с ветки `main`.
Два Actions secrets хранятся только в environment `frontend-mock`,
доступном для deployment из `main` (repository-wide копий нет):

- `FRONTEND_DEPLOY_SSH_KEY`: отдельный приватный ключ `saldo-deploy`.
- `FRONTEND_DEPLOY_KNOWN_HOSTS`: проверенный ED25519 host key для
  `[brokebyte.me]:8321`.

Временный `GITHUB_TOKEN` с `packages:read` передаётся в stdin SSH и используется
только для pull приватного GHCR-образа. Docker login хранится в временной
root-only директории и удаляется при завершении; PAT на сервере не сохраняется.
Для первого GHCR push организация должна разрешать создание packages;
package привязывается к этому репозиторию через OCI source label.

CI начинает автоматический deployment только после merge workflow и frontend
в `main`. Первичный административный запуск до merge не доказывает прохождение
этого будущего CI deployment.

## Проверки и откат

```bash
cd /opt/saldo-frontend
docker compose --env-file .env -f compose.yaml ps
curl -I https://saldo.brokebyte.me/login
systemctl list-timers saldo-frontend-cert-renew.timer
certbot renew --dry-run --config-dir /opt/saldo-frontend/letsencrypt \
  --work-dir /opt/saldo-frontend/certbot-work \
  --logs-dir /opt/saldo-frontend/certbot-logs
```

Deploy ожидает healthcheck Next.js, проверяет Nginx config, перезагружает
его upstream и проверяет HTTPS. При неуспехе восстанавливает предыдущий образ.
Последний успешный предыдущий image reference сохраняется в `previous-image`.
Для ручного отката администратор записывает его как `FRONTEND_IMAGE` в `.env`,
выполняет `docker compose up -d --no-deps --wait frontend` и
`docker compose exec -T nginx nginx -s reload`. Не удалять предыдущий образ
до подтверждения успешного deployment.
