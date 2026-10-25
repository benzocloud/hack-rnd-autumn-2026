# Fixture UI smoke — НЕ real backend acceptance

Снимки Chromium 25.10.2026 для draft PR #49. Production build (developerMode=dev),
API ответы перехвачены CDP Fetch и заменены фиксированными демонстрационными DTO.
Банковских/личных данных, cookies и tokens в изображениях нет. Каждое изображение
содержит маркировку FIXTURE UI SMOKE. Это проверка UI и payload, не Go API/Core/ML.

Проверено на финансовом source срезе dfa9f505197457366b71db82be1e8181eb5d7e1e:
- desktop: частичный платёж, перенос даты PATCH без amount/direction;
- активная/снятая match history; cancel remaining и запрет кнопок unmatch после cancel;
- ручная сумма 9007199254740993 копейки: после fixture HTTP 503 сохранены поля,
  тот же idempotency key и байтово эквивалентное тело retry даже при новой account revision;
- additional/replacement требуют явного режима и подтверждения; source IDs выбираются явно;
  изменение выбора снимает completeness; создание платежа передаёт group ID;
- category CAS recovery сохраняет выбор и перечитывает transaction version;
- сохранённый прогноз: 30 строк точных сумм, график, null probability, reload;
- 390×844: ширина body=390, календарная сетка и действия доступны.

Скриншоты не доказывают полный import/payment/forecast lifecycle на backend.
Реальная приёмка блокирована: нет проверенного изолированного API кандидата.
Публичный csrf endpoint после временного503 восстановился до200; source SHA/изоляция
общего сервера не подтверждены, финансовые записи на нём не создавались.
Подробности, зависимости и следующий шаг: `.agent/handoffs/checkpoint-financial-frontend.md`.
