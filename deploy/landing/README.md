# Публикация лендинга

Next.js слушает 3005 внутри контейнера и на сервере. Основной frontend остаётся
на 3000 внутри своей Docker-сети, существующий HTTPS адрес не меняется.
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
Порт 3005 должен быть свободен. Nginx, TLS и firewall других проектов не меняются.
При закрытом внешнем порте администратор отдельно настраивает доступ к 3005.

Используется существующее защищённое main-only environment `frontend-mock`:
те же SSH key/known-hosts secrets и временный GITHUB_TOKEN для GHCR pull.
Пакет `ghcr.io/benzocloud/saldo-landing` должен разрешать доступ этому репозиторию.
На сервере не хранятся PAT. Ошибка проверки возвращает предыдущий образ;
неудачная первая установка убирает только новый контейнер лендинга.

Ручной запуск после merge:

```sh
gh workflow run landing.yml --repo benzocloud/hack-rnd-autumn-2026 --ref main
```
