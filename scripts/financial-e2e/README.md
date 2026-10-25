# Financial E2E harness (agent C)

Воспроизводимая приёмка публичного пути Frontend → API → Auth/Core → PostgreSQL → ML.
Не заменяет модульные suites владельцев компонентов и не дублирует
`deploy/stack/smoke.py`.

## Быстрый старт

```sh
cd scripts/financial-e2e
# macOS system Python 3.9/LibreSSL cannot speak TLS 1.3 required by API.
# Prefer Homebrew Python 3.12+ (OpenSSL 3.x), e.g. /opt/homebrew/bin/python3
PY=${SALDO_E2E_PYTHON:-/opt/homebrew/bin/python3}
$PY prepare_fixtures.py
$PY up_stack.py --keep --suffix local
. artifacts/stack-local/env.sh
$PY run_http.py --dry-probe
$PY run_http.py
$PY down_stack.py --suffix local
```

Обязательные переменные (имена без секретов): см. handoff
`.agent/handoffs/checkpoint-financial-e2e.md`.

## Артефакты

- `fixtures/` — детерминированные CSV + `manifest.json` с SHA-256
- `artifacts/last-run.json` — фактический прогон (gitignore)
- `scenarios.md` — матрица ID сценариев

## Browser

Сценарии: `frontend/tests/e2e/financial/`. Общие `frontend/package.json`
scripts/deps — зона агента B; нужный diff сначала в handoff.

## Изолированный stack

Не меняем `deploy/**`. Оборачиваем reference Compose своим project name,
свободными loopback ports и disposable volumes. Marker/identity перед teardown.
Локальные images `saldo-*:86c1374` недостаточны без rebuild кандидата.

## ML bindings (local only)

`up_stack.py` expects generated Python stubs at
`scripts/financial-e2e/artifacts/ml-gen/ml/` (gitignored). Copy from a checkout
that already ran `services/ml/generate_rpc.py`, or generate into that directory.
Do not commit private keys or stack state from `artifacts/`.
