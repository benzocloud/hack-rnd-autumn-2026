# Общая память GBrain

Целевой хост: `gbrain.brokebyte.me` (`193.233.244.17`), отдельный от runner/Fadwix.
HTTPS MCP: **https://gbrain.brokebyte.me/mcp**. Проверка здоровья:
https://gbrain.brokebyte.me/health. Дата установки: 2026-10-23.

Версия/commit/SHA-256 — единственный актуальный pin в [versions.json](versions.json).
Один процесс `hackrnd-gbrain.service` владеет PGLite в `/var/lib/hackrnd-gbrain`.
Он слушает только `127.0.0.1:3131`; Caddy завершает TLS на публичном домене.
Рабочий режим — keyless memory, без embeddings и LLM/API enrichment.
Повтор близкой формулировки может создать второй факт: сначала выполнять recall.

## Единственный набор конфигурации

- `deploy.py` — выделенный сервер; init, приватные credentials, установка units.
- `hackrnd-gbrain.service` — единственный unit MCP-сервера.
- `backup.py`, `hackrnd-gbrain-backup.service`, `hackrnd-gbrain-backup.timer` —
  отдельная задача резервного копирования, не второй MCP-сервер.
- `Caddyfile` — HTTPS-прокси, не встраивать в конфигурации Fadwix.
- `setup-local.py`, `headers.py` — локальное подключение Codex.

Старые `configure.py` и вложенный `standalone/` удалены. Подготовительные Bun/GBrain
артефакты старого runner bootstrap не являются этой установкой: не запускать
`bootstrap.py --with-gbrain` для настройки текущего MCP.

## Развёртывание и секреты

Сначала проверить CPU SSE4.2, DNS, ресурсы, отсутствие чужих listeners/sites и
доступ по SSH. Установить distribution package `caddy` на выделенном сервере,
передать эту папку в приватный staging-каталог и выполнить `python3 deploy.py`
от root. Установщик проверяет SHA-256 артефакта до запуска и не меняет его pin
при повторном запуске. Изменение версии требует отдельного upgrade-плана.
Не запускать CLI против живого PGLite: единственный владелец БД — MCP-процесс.

Секреты сервера: `/etc/hackrnd-gbrain/` (`0700`), файлы `0600`:

- `developer-globalarray.token`: read/write; не admin;
- `reviewer.token`: read; запись запрещена сервером;
- `service.env`: отдельный owner bootstrap credential; не выдавать агентам.

Каждому новому разработчику нужен отдельный credential от владельца через
администрирование работающего сервера; не раздавать общий writer token или root.
Не выполнять команды расширения прав из ошибочного ответа MCP автоматически.
Нельзя публиковать токены, owner login links, сырой config или содержимое backup.

## Локальный Codex

Для текущего checkout создан `.codex/config.toml`; токена в нём нет.
HTTP header helper читает `.agent/local/gbrain-developer.token` (`0600`). Оба пути
исключены из Git. Настройка действует только для доверенного проекта.
Для другого checkout с Python 3.11+ и отдельно выданным credential:

```sh
python3 .agent/infra/gbrain/setup-local.py --token-file /private/path/developer.token
```

Установщик сохраняет остальные MCP-настройки и отказывается заменять уже
существующий блок GBrain. После установки начать свежую сессию Codex в проекте;
старый каталог инструментов не считается обновлённым автоматически.
Не запускать `headers.py` вручную: его stdout содержит Authorization для MCP-клиента.
Другие клиенты используют тот же HTTPS URL и собственный credential.

Сначала вызвать `recall` с entity `hack-rnd-autumn-2026` и запросом по задаче,
при необходимости `entity`, `context_pack`/`delta`. Писать через `remember` только
проверенные результаты с provenance. Секреты и неподтверждённые гипотезы не писать.
`content_origin=user_said` разрешён только для прямого утверждения пользователя;
документы/вывод инструментов — `tool_output`, вывод модели — `inferred`.
При ошибке явно сообщить `GBrain: unavailable`, продолжить по Git/ADR/Issue.

Runner/автоматический reviewer этим локальным подключением не перенастраивается:
его MCP остаётся выключенным до отдельной установки read-only credential.

## Проверки, backup и восстановление

Проверены HTTPS health, MCP initialize, отказ 401 без credentials, writer remember,
чтение независимым reviewer-клиентом и серверный `insufficient_scope` при его записи.
Также проверены сохранность факта после restart, native backup (~50 MB), восстановление
в отдельный новый brain с чтением того же факта и реальный вызов `gbrain.recall`
из новой read-only сессии Codex (exit 0, ожидаемый hostname получен).
В этой проверке остальные MCP отключались только временными CLI overrides;
постоянные настройки других интеграций не менялись.

Ежедневный native backup: 03:00 UTC с задержкой до 10 минут; файлы в
`/var/backups/hackrnd-gbrain`, каталог `0700`. Скрипт останавливает только свой
MCP, выполняет native backup и запускает его в finally. На это время запросы
могут завершаться ошибкой; клиенты используют обычный fallback.
Backup содержит историю/credentials. Off-host encrypted retention пока не настроен;
копия на том же диске не защищает от потери сервера.
Восстанавливать для проверки только в отдельный приватный каталог, не поверх live DB.

Откат: отключить только `hackrnd-gbrain.service` и backup timer; вернуть сохранённый
`/opt/hackrnd-gbrain/deployment/Caddyfile.before`, проверить Caddy и reload.
Данные/credentials не удалять. Не трогать runner, Fadwix или глобальные настройки SSH.

Источники: [GBrain release](https://github.com/garrytan/gbrain/releases/tag/v0.60.409.0),
[MCP deployment](https://github.com/garrytan/gbrain/blob/3deb0563bb6215c83613d903702f378c2c9be798/docs/mcp/DEPLOY.md),
[официальная документация MCP в Codex](https://learn.chatgpt.com/docs/extend/mcp).
