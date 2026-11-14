# AGENTS.md

Проект: Arduino Telemetry (см. README.md для архитектуры и быстрого старта).

## Сборка и запуск
- Всё сразу: `.\scripts\build.ps1 all`
- Только poller (Windows): `.\scripts\build.ps1 poller-windows`
- Только ingest (Linux, для Docker): `.\scripts\build.ps1 ingest-linux`
- Только web (React + Go, для Docker): `.\scripts\build.ps1 web-linux`
- Docker-сервисы: `docker compose up -d --build`
- Остановка: `docker compose down` (данные БД сохраняются; `-v` удаляет тома)

## Границы (не трогать без явного запроса)
- `build/` — только артефакты, не коммитить
- `data/backups/` — дампы БД
- `.env`, `poller.env` — секреты, не перезаписывать и не выводить в чат
- `infra/postgres/init/001_init.sql` — выполняется только при пустом томе; для существующей БД правки не применятся автоматически

## Конвенции
- Go: `CGO_ENABLED=0` для всех бинарников; `poller` — windows, `ingest`/`web` — linux
- Serial-протокол: 9600 8N1, строки `\n`, JSON-ответы (см. README)
- `device_id` — SMALLINT, справочник `devices`; не менять на TEXT
- Агрегация графиков: только `date_bin` (не `date_trunc`) для интервалов вроде `5m`
- React-статика собирается в `cmd/web/static/` через Vite; в git — только плейсхолдер `index.html`

## Проверка после изменений
- Go: `go build ./...`
- Frontend: `cd web && npm run build`
- Docker: `docker compose up -d --build` + `docker compose logs -f <service>`
