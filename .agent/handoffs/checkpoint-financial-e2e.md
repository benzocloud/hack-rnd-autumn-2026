# Handoff: financial E2E checkpoint (агент C)

Время: 25.10.2026 ~10:54 Europe/Moscow.

## Публикация

| Поле | Значение |
| --- | --- |
| PR URL | https://github.com/benzocloud/hack-rnd-autumn-2026/pull/50 |
| Branch | `globalarray/test/verify_financial_user_journey` |
| Base | `origin/main` `71f2a86ffbcf218d7fff517830b7aa41f0a97c95` |
| Local HEAD | `971a1b79dca53ef87122b7637283961f2d3ed163` |
| Remote HEAD | `971a1b79dca53ef87122b7637283961f2d3ed163` (match=yes) |
| Worktree | `/Users/globalarray/hack-rnd-autumn-2026-financial-e2e` |
| Nickname | `globalarray` |

## GBrain

`unavailable`: в Cursor-сессии нет MCP namespace/tools для
`https://gbrain.brokebyte.me/mcp` (GetDynamicTools по gbrain/recall — пусто).
Продолжаем по Git/ADR/OpenAPI/PR handoff агента B.

## GitHub API / PR CLI

- `git fetch` / `git push --dry-run` к origin работают через osxkeychain.
- `gh` не залогинен (`gh auth status`: not logged in). Публичный REST к private
  repo даёт 404 без token. ConnectScm Cursor завершился успешно, но CLI token
  не появился.
- Подхват PR, если `gh pr create` падает:
  1. `gh auth login` (scopes: `repo`, `read:org`) на машине владельца/агента.
  2. `cd /Users/globalarray/hack-rnd-autumn-2026-financial-e2e`
  3. `gh pr create --draft --title "test: verify the real financial user journey" --body-file ...`
  4. Либо открыть compare URL после push:
     `https://github.com/benzocloud/hack-rnd-autumn-2026/compare/main...globalarray/test/verify_financial_user_journey?expand=1`

## Зависимости кандидата (не в этом PR)

| Компонент | Источник | SHA | Примечание |
| --- | --- | --- | --- |
| Base | `origin/main` `71f2a86ffbcf218d7fff517830b7aa41f0a97c95` |
| CSV/manual (#47) | main | `f686ca8` | merged |
| Onboarding (#41) | main | ancestor | 0 commits ahead |
| Payments (#48) | `globalarray/feat/payments_frozen_calendar` | ~`ac90fa3` | open draft; нужно для lifecycle |
| Frontend (#49) | `globalarray/feat/financial_user_journey` | ~`7d6d749` | open; агент B |
| ML train (#45) | `god4k1ta/feat/ml_trained_forecast` | ~`462344f` | open; не закрывает calibration |

Интеграционный прогон: отдельный checkout = merge опубликованных SHA.
Чужие product-коммиты **не** включаются в тестовый PR.

## Env variable names (без значений)

`SALDO_E2E_PROJECT`, `SALDO_E2E_API_ORIGIN`, `SALDO_E2E_FRONTEND_ORIGIN`,
`SALDO_DATASET_DIR`, `SALDO_E2E_EMAIL_DOMAIN`, `AUTH_DEMO_EMAIL_VERIFICATION`,
`AUTH_DEMO_TEST_EMAILS`, `AUTH_ENV`, `COMPOSE_PROJECT_NAME`,
`SALDO_E2E_KEEP_STACK`, `SALDO_E2E_ARTIFACT_DIR`.

Secrets/cookies/private keys не коммитятся; артефакты — под
`scripts/financial-e2e/artifacts/` (gitignore).

## Fixture hashes

| Файл | SHA-256 | Примечание |
| --- | --- | --- |
| source `dataset/statements/C001.csv` | `945fe67e8418882bf7bd5e9bdcda570c613fc7777113d55424ae34b052dfa92a` | полный; gitignored |
| generated `ordinary_90d_c001.csv` | `8780e7806a705f1c2898acf234c301ff6c1391f88fbb99bd27bb55f7c4e4bf85` | window 2026-07-02..2026-09-29, 151 ops |
| generated `short_history.csv` | `a25c0d3ccf4c2823a7740c2617889f0cad02a3bda67eaa1fe94be6f9445f4df3` | 30d negative |
| generated `eod_conflict.csv` | `24581aafd2bc76283e61092d121a408af74200afd75c4061720404fc276998bc` | EOD conflict negative |

Наблюдение: C001 не имеет строк на `2026-09-30`; последний EOD `2026-09-29`.

## Запрос к агенту B (frontend manifests)

Если browser harness потребует прямой `@playwright/test` в `frontend/package.json`
и npm script `test:e2e:financial`, точный diff опишем здесь до правки.
Пока: сценарии в `frontend/tests/e2e/financial/**`; запуск через
`scripts/financial-e2e` / npx без изменения shared manifests, если хватит
существующих зависимостей.

## Результаты / skips

Этап 1 в публикации: матрица + skeleton harness. Real HTTP/browser ещё не
прогонялись в этом worktree.

## Blockers

1. **gh CLI auth** — osxkeychain token проходит `git push`, но `gh auth login --with-token` отклоняет его (`missing required scope 'read:org'`). Draft PR создаём через REST `POST /repos/.../pulls` тем же token (repo) либо после `gh auth login` с полными scopes.
2. **Images `86c1374`** — локальный GHCR/cache старше main; для real stack нужен
   build/run binaries кандидата.
3. **Public demo** — не использовать как proof (503 CSRF по handoff #49).
4. **PR #48 не merged** — payment lifecycle на чистом main будет blocked;
   проверять на integration merge.

## Что сохранено на GitHub

*(обновить после push)* plan, handoff, scenarios matrix, harness skeleton,
verification stub.

## Следующий шаг подхвата

1. Проверить remote SHA == local.
2. Довести `scripts/financial-e2e/run_http.py` happy-path против isolated stack.
3. Поднять disposable stack (отдельный project/ports) или зафиксировать точный
   blocker команд.
4. Не ждать апрува этапов; статусы — «На ревью».


## Checkpoint 10:58 МСК — этап 1 На ревью

- Draft PR: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/50
- Pushed SHA matched remote at publication: `971a1b79dca53ef87122b7637283961f2d3ed163`
- Current local before this docs commit: `971a1b79dca53ef87122b7637283961f2d3ed163`
- Base main: `71f2a86ffbcf218d7fff517830b7aa41f0a97c95`
- Payments tip (#48): `1f10b6e615296a01d64957730ac2ce38a2e2b11c`
- Frontend tip (#49): `7d6d749c4dcd0547e11d48162527c38021dbb003`
- ML tip (#45): `462344f9a11ae563c7d8d104cd6844270f64983d`
- Fixtures prepared; real stack not yet started.
- Next: isolated stack under `scripts/financial-e2e` (no `deploy/**` edits).
