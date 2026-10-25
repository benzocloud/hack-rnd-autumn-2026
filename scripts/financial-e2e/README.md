# Financial E2E harness (agent C)

Воспроизводимая приёмка публичного пути Frontend → API → Auth/Core → PostgreSQL → ML.
Не заменяет модульные suites владельцев компонентов и не дублирует
`deploy/stack/smoke.py`.

## Быстрый старт

```sh
cd scripts/financial-e2e
python3 prepare_fixtures.py
# после подъёма собственного isolated stack:
export SALDO_E2E_API_ORIGIN='https://127.0.0.1:<port>'   # не production
python3 run_http.py --dry-probe
python3 run_http.py
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
