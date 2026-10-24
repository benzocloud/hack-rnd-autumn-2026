# Saldo

Монорепозиторий сервиса финансового прогнозирования для малого бизнеса.

- [`frontend/`](frontend/README.md) — Next.js: исходники, моки, тесты, зависимости и Docker-сборка.
- [`deploy/frontend/`](deploy/frontend/README.md) — Nginx, TLS и ограниченный CI deploy демонстрации.
- [`docs/api/`](docs/api/README.md) — REST-контракты.
- [`.agent/`](.agent/instructions.md) — инструкции, решения и планы разработки.

Демонстрация с моками: [saldo.brokebyte.me](https://saldo.brokebyte.me).

```bash
cd frontend
npm ci
cp .env.example .env.local
npm run dev
```

GitHub Actions проверяет frontend в PR и развёртывает его после push в `main`.
