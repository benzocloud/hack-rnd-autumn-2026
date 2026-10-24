# Лендинг Saldo на /landing

Дата: 2026-10-24. Основание: прямое поручение владельца настроить production
deployment лендинга из `alites-lite/fix/registration_consent_display`, merge в
main и CI. Ветка уже включена через PR #31, main `01ecfc0`. Это отдельная задача
в checkout `runner/ci/landing_path_deploy`; работа над Core не затрагивается.

Цель: `https://saldo.brokebyte.me` продолжает обслуживать приложение на внутреннем
3000, `/landing` и вложенные ресурсы обслуживает отдельный Next.js на 3005.
Хостовый 3000 занят Grafana: его не освобождать и не менять.

## Подготовка и проверка

- Next.js basePath=/landing при сборке; явные public assets получают префикс.
- Отдельный Compose project saldo-landing, loopback 3005 и существующая сеть
  saldo-frontend_default. Изменяется только Nginx Saldo, с проверкой и откатом.
- Сохраняются ограниченные SSH-команды и main-only environment frontend-mock;
  PAT/новые ключи на сервер не передаются. Образ фиксирован по GHCR digest.
- Локально lint/typecheck, 6 tests, production build — PASS; настоящий standalone
  server: `/landing` и 28 ресурсов — PASS, legal redirect ведёт в приложение.
- Bash syntax, Compose config, actionlint и git diff --check — PASS.
- PR CI должен проверить Docker smoke. Deployment выполняется отдельным
  workflow_dispatch на main после merge; root-приложение не пересоздаётся.

## Публикация и deployment

Статус: подготовлено, ожидаются CI, merge и разрешённый владельцем deployment.
Перед установкой сохранить текущие Saldo config и идентификаторы контейнеров,
проверить свободный 3005. Сначала установить root-owned landing deploy scripts
и точечный Nginx route, затем запустить workflow на main. Проверить публичный
HTTPS, assets, фактический image revision/digest, health и неизменность соседних
контейнеров. Образ собирает GitHub, не production-хост.

Сервис является презентационным лендингом с синтетическими примерами; это
не интеграция финансовых API. Настройка не затрагивает GBrain, Fadwix, Grafana,
прочие Compose projects, сертификаты или firewall.
