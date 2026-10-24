# Saldo

Монорепозиторий сервиса финансового прогнозирования для малого бизнеса.

- [`frontend/`](frontend/README.md) — Next.js: исходники, моки, тесты, зависимости и Docker-сборка.
- [`services/core/`](services/core/README.md) — домен, прикладные финансовые команды и PostgreSQL; транспорт ещё не реализован.
- [`services/auth/`](services/auth/README.md) — каркас Auth, схема и health/readiness; бизнес-RPC ещё не реализованы.
- [`services/ml/`](services/ml/README.md) — baseline, исследования и gRPC-прогноз `mean_90`; контракт и Python-сервис реализованы, подключение снимка данных из Core ещё требуется.
- [`deploy/frontend/`](deploy/frontend/README.md) — Nginx, TLS и ограниченный CI deploy демонстрации.
- [`docs/api/`](docs/api/README.md) — REST-контракты.
- [Состояние интеграции](docs/integration-status.md) — готовые части и зависимости для разработчиков.
- [`.agent/`](.agent/instructions.md) — инструкции, решения и планы разработки.

Демонстрация с моками: [saldo.brokebyte.me](https://saldo.brokebyte.me).

```bash
cd frontend
npm ci
cp .env.example .env.local
npm run dev
```

GitHub Actions проверяет компоненты в PR. Для frontend push в `main` публикует
mock-образ в GHCR; deployment выполняется только явным ручным запуском
`Frontend mock CI and deploy` (`workflow_dispatch`) на `main`.
