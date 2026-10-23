# Референсы Go-сервисов

Подключены по запросу разработчика из `references/maxhack`.
Проверка локальных файлов: **2026-10-22**. Источники списка:
`maxhack/.agent/instructions.md` и `maxhack/.agent/instructions.local.md`, HEAD
`06e680152ea6b62ea023551498018d7e7b08d877` (оба файла без локальных правок).
Ниже — проверенные примеры и применимые приёмы, а не обязательный для копирования
каркас или новые архитектурные ADR.

Сначала соблюдать ADR-002/003 текущего проекта: HTTP/JSON между браузером и Go
API, gRPC/protobuf на согласованных границах Go/ML, PostgreSQL по необходимости.
Референсы не разрешают менять эти границы, переносить чужую предметную модель,
контракты, схемы БД, зависимости, версии Go или инфраструктуру MAX.
Для `ML:` PR Go-skills по-прежнему запрещены; маршрутизация — ADR-012.

## Проверенные источники

| Референс | Файлы для сверки | Что брать как ориентир | Ограничения |
| --- | --- | --- | --- |
| `references/maxhack/services/gateway/` | `cmd/gateway/main.go`, `internal/app/app.go`, `internal/handler/http/{router,server}.go`, `internal/config/config.go`, соседние тесты | Тонкий entrypoint; явная сборка приложения; разделение server/router/feature handler; отдельные timeout-ы; readiness; ограниченное по времени завершение | В Gateway есть локальные незакоммиченные правки. Здесь проверена рабочая копия, а не только HEAD; тесты референса в рамках переноса не запускались. MAX webhook/Kafka не переносятся |
| `references/hack-rnd-2026-spring/services/engine-go/` | `internal/app/app.go`, `internal/service/`, `internal/domain/`, `internal/delivery/grpc/`, `internal/infrastructure/postgres/repository/` | Ручная сборка зависимостей; transport → service → repository; предметные модели/ошибки; lifecycle ресурсов | В этом источнике транспорт называется `delivery`, а не `handler`. Не переносить чужие proto, сертификаты и бизнес-логику |
| `references/centralservice/` | `internal/app/{app,container,shutdown}.go`, `internal/handler/`, `internal/service/`, `internal/domain/`, `internal/repository/minecraft/trust/{repository,queries}.go` и `sql/` | Интерфейсы рядом с потребителем; ручная сборка; repository с DB/pool; SQL в отдельных файлах через `//go:embed`; явное владение ресурсами | Игровая предметная область и SQL/драйверы другого проекта не определяют PostgreSQL-контракты нашего проекта. Крупный container не является требованием для маленького сервиса |
| `references/golang-arch/` | `internal/handler/router.go`, `internal/handler/part/handler.go`, `internal/service/`, `internal/repository/` | Центральная регистрация маршрутов; consumer-side interfaces; feature packages; приватный handler с явным `NewHandler` | Фактический путь здесь `internal/handler/router.go`, без `http/`. В примере есть глобальный `log` и выдача `err.Error()` наружу — это не рекомендации для переноса |
| `references/auth-service/` | `internal/config/config.go`, `internal/lifecycle/readiness.go`, `internal/app/http/app.go`, `internal/delivery/http/router.go` | Типизированные секции конфигурации; разделение lifecycle и router; readiness reader/writer; владение `http.Server` на уровне приложения | Не переносить глобальное состояние logger, auth/JWT-зависимости и `sslmode=disable`. Defaults до файла, затем env/Secrets и валидация — целевой подход из замечаний maxhack; поведение готового loader нельзя считать эталонным без проверки |

Все пути указаны от корня текущего репозитория. `references/` содержит
локальные символические ссылки и исключён из Git через `/references/` в
`.gitignore`; исходники референсов не копируются и не коммитятся. На runner
и у других разработчиков ссылки/checkout могут отсутствовать. Тогда использовать зафиксированное здесь описание, текущие ADR и
исходники проекта; явно сообщать, что оригинал недоступен. Не подключать чужие
checkout к runner и не копировать их целиком ради ссылок.

## Как применять

Дополнительно для плана первого Core-этапа проверен 2026-10-23
`references/maxhack/services/gateway/pkg/log/logger.go:13–41` (HEAD maxhack выше,
сам logger без локальных изменений): конструктор возвращает caller-owned
`*slog.Logger`, использует JSON handler или tint, без global state. Владелец выбрал
адаптацию в локальный `services/core/pkg/log` (ADR-022). Детали writer/валидации,
применимое MIT notice и другие проверенные приёмы перечислены в
[плане Go Core](../plans/go-core-foundation.md); адаптация выполнена в подэтапе 2.1.
Наш первый этап использует стандартные `slog.NewJSONHandler`/`NewTextHandler`;
внешняя зависимость tint из исходника не переносится.

1. Выбрать конкретный приём под задачу и проверить исходный файл/его текущий diff.
2. В плане указать референс, выбранный приём, затрагиваемые файлы и отличия от
   источника. Отделять существующие файлы от предлагаемых новых.
3. Создавать только нужные слои; не вводить repository, DI framework или отдельный
   пакет исключительно ради сходства с референсом. SQL-библиотека не выбрана
   самим фактом наличия `sqlx` в примере.
4. После адаптации проверить ошибки, context/deadline, закрытие ресурсов,
   transport mapping, тесты и совместимость наших protobuf. Локальный пример
   не заменяет тесты и не доказывает корректность заимствованного фрагмента.

Кандидаты на дополнительные правила и предостережения:
`.agent/plans/maxhack-lessons.md`. Статус «предложено» там не означает принятого ADR.
