# Публикация лендинга

Публичный адрес лендинга — **https://saldo.brokebyte.me/landing**. Next.js
собирается с `basePath=/landing` и слушает 3005 внутри контейнера. Порт хоста
`127.0.0.1:3005` доступен только локально. Основной frontend остаётся
на 3000 внутри своей Docker-сети; маршрут `/` и страницы приложения не меняются.
Лендинг публикуется отдельно через `.github/workflows/landing.yml` по digest.
PR проверяет код и контейнер; push main публикует образ; только ручной запуск
workflow на main обновляет сервер. Данные финансовой демонстрации синтетические.

Перед первым deploy администратор выполняет из checkout на сервере:

```sh
sudo bash deploy/landing/install.sh
```

Установщик добавляет отдельный Compose project `/opt/saldo-landing` и строго
ограниченную команду `deploy-landing` к существующему SSH dispatcher. Он не
меняет SSH-ключи, не даёт shell/Docker group и сохраняет команду frontend.
Нужен свободный loopback-порт 3005 и существующая сеть `saldo-frontend_default`.
Лендинг подключается к этой сети; Nginx Saldo направляет `/landing` и вложенные
пути в `landing:3005`, сохраняя префикс. Docker DNS обновляет адрес upstream
после пересоздания контейнера. Installer проверяет и перезагружает только
Nginx Saldo, с восстановлением прежнего конфига при неуспехе проверки `/login`.
TLS, firewall, Grafana на хостовом 3000 и сервисы других проектов не меняются.

Используется существующее защищённое main-only environment `frontend-mock`:
те же SSH key/known-hosts secrets и временный GITHUB_TOKEN для GHCR pull.
Пакет `ghcr.io/benzocloud/saldo-landing` должен разрешать доступ этому репозиторию.
На сервере не хранятся PAT. Ошибка проверки возвращает предыдущий образ;
неудачная первая установка убирает только новый контейнер лендинга.

Ручной запуск после merge:

```sh
gh workflow run landing.yml --repo benzocloud/hack-rnd-autumn-2026 --ref main
```

Контейнерный smoke-test проверяет страницу `/landing`, JavaScript/CSS, логотипы,
иконки и шрифты с префиксом. После deployment проверяются публичные HTTPS
`/landing` и `/login`; compose healthcheck обращается к `/landing` на 3005.
