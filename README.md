# Arduino Telemetry

Учебный программно-аппаратный комплекс: Arduino с датчиком температуры LM35 → Go-сервисы → PostgreSQL → React-веб-интерфейс с графиками и управлением LED.

## Содержание

- [Архитектура](#архитектура)
- [Структура репозитория](#структура-репозитория)
- [Требования](#требования)
- [Быстрый старт](#быстрый-старт)
- [Разработка](#разработка)
- [Эксплуатация](#эксплуатация)
- [Диагностика](#диагностика)

## Архитектура

### Схема взаимодействия

```
┌──────────────┐   USB Serial   ┌──────────────┐
│   Arduino    │◄──────────────►│    poller    │   (запускается на хосте,
│  LM35 на A0  │  GET, LED:ON   │  (Go, хост)  │    рядом с Arduino)
│  LED  на D7  │  JSON-ответ    └──────┬───────┘
└──────────────┘                       │ MQTT publish
                                       │   home/arduino/<dev>/data
                                       │   home/arduino/<dev>/status
                                       ▼
                               ┌───────────────┐
                               │   Mosquitto   │   (Docker, порт 1883,
                               │  (MQTT-брокер)│    логин/пароль)
                               └───┬───────┬───┘
                    subscribe      │       │      publish
              /+/data              │       │   /+/command
                                   ▼       ▲
                          ┌────────────┐   │   ┌──────────────┐
                          │   ingest   │   │   │     web      │
                          │ (Go, Docker)│  │   │ (Go, Docker) │
                          └─────┬──────┘   │   └──────┬───────┘
                                │ INSERT   │          │ SELECT
                                ▼          │          ▼
                          ┌────────────────────────────────┐
                          │          PostgreSQL            │   (Docker, порт 5432,
                          │  devices(id, name, location)   │    named volume)
                          │  readings(device_id, temp, ts) │
                          └────────────────────────────────┘
                                                ▲
                                                │ HTTP /api/*
                                                │
                                       ┌────────┴────────┐
                                       │    Браузер      │
                                       │ (React + Vite)  │
                                       │  встроен в web  │
                                       └─────────────────┘
```

### Потоки данных

| Направление | Путь | Формат |
|---|---|---|
| **Телеметрия** | Arduino → poller → MQTT → ingest → Postgres → web → браузер | `{"device_id":1,"temperature":23.50,"recorded_at":"..."}` |
| **Управление** | браузер → web (`POST /api/command`) → MQTT → poller → Arduino | `{"device_name":"arduino-01","command":"LED:ON"}` |
| **Статус** | poller → MQTT `.../status` (retain + LWT) | `online` / `offline` |

### Ключевые решения

- **poller работает на хосте**, не в Docker: ему нужен прямой доступ к COM-порту (на Windows проброс USB в контейнер требует `usbipd-win` и WSL2 — лишняя сложность).
- **ingest и web — статические Go-бинарники в Alpine-контейнерах**, сборка локальная (кросс-компиляция `CGO_ENABLED=0 GOOS=linux`), Docker только упаковывает.
- **React встроен в `web`-бинарник** через `embed.FS` — один контейнер отдаёт и API, и статику, без nginx и CORS.
- **Postgres — named volume** (не bind mount): на Windows bind mount ломает права POSIX. Резервная копия — `pg_dump` в `./data/backups/` раз в сутки.
- **`device_id` — `SMALLINT`** со справочником `devices`: 2 байта в «горячей» таблице вместо текстового `'arduino-01'`.
- **`date_bin` вместо `date_trunc`** для агрегации графиков: поддерживает произвольные интервалы (5m, 15m, 6h…).

### Протокол Arduino ↔ poller

Serial: **9600 8N1**, строки, разделитель `\n`. Команда → JSON-ответ:

| Команда | Ответ |
|---|---|
| `GET` | `{"temperature":23.50,"led":false}` |
| `PING` | `{"pong":true}` |
| `LED:ON` | `{"ok":true,"led":true}` |
| `LED:OFF` | `{"ok":true,"led":false}` |
| иное | `{"error":"unknown_command","cmd":"..."}` |

## Структура репозитория

```
arduino-telemetry/
├── .env.example / .env             # конфиг docker-compose (ingest, web, postgres, mosquitto)
├── poller.env.example / poller.env # конфиг poller (serial, MQTT, device)
├── docker-compose.yml
├── go.mod / go.sum
├── Makefile                        # для Linux/macOS
│
├── arduino/
│   └── temperature_sensor/
│       └── temperature_sensor.ino  # LM35 на A0, LED на D7, Serial.begin(9600)
│
├── cmd/
│   ├── poller/                     # Arduino(serial) ↔ MQTT, на хосте
│   ├── ingest/                     # MQTT → Postgres, в Docker
│   │   └── Dockerfile
│   └── web/                        # REST API + статика React, в Docker
│       ├── Dockerfile
│       ├── static/                 # embed.FS (сборка React, gitignore)
│       │   └── index.html          # плейсхолдер
│       └── ...
│
├── internal/
│   ├── config/                     # LoadPoller, LoadIngest, LoadWeb
│   ├── models/                     # Reading, Device, AggregatedReading
│   ├── serial/                     # обёртка go.bug.st/serial
│   ├── mqttclient/                 # publisher, subscriber, NewClient
│   └── storage/                    # Postgres: OpenWithRetry, InsertReading, ReadingsRange, ListDevices
│
├── web/                            # React + Vite + Ant Design + Recharts
│   ├── src/
│   ├── package.json
│   └── vite.config.ts              # outDir → ../cmd/web/static
│
├── infra/
│   ├── mosquitto/config/
│   │   ├── mosquitto.conf
│   │   └── docker-entrypoint.sh    # создаёт passwd из MQTT_USERNAME/MQTT_PASSWORD
│   └── postgres/init/
│       └── 001_init.sql            # таблицы + справочник devices
│
├── scripts/
│   ├── build.ps1                   # Windows
│   └── postgres-backup.sh          # pg_dump каждые 24 ч в /backups
│
├── build/                          # локальные бинарники (gitignore)
└── data/backups/                   # дампы Postgres (gitignore)
```

## Требования

| Инструмент | Версия | Зачем |
|---|---|---|
| **Go** | 1.22+ | сборка `poller`, `ingest`, `web` |
| **Node.js + npm** | 20 LTS | сборка React (`web/`) |
| **Docker Desktop** | 24+ | postgres, mosquitto, ingest, web |
| **Arduino IDE** | 2.x | прошивка скетча |

Windows дополнительно: **PowerShell 5.1+** (уже есть), **LF**-переводы строк для `*.sh` (в VS Code: правый нижний угол → `LF`).

## Быстрый старт

### 1. Клонировать и настроить окружение

```powershell
git clone <repo> arduino-telemetry
cd arduino-telemetry

# Конфиг для Docker-сервисов
Copy-Item .env.example .env

# Конфиг для poller (запускается на хосте)
Copy-Item poller.env.example poller.env
```

Откройте оба файла и **задайте пароли** (`change_me_*` — заменить). Пароли MQTT в `.env` и `poller.env` **должны совпадать**.

Проверьте, что в `poller.env` стоят ваши значения:

```dotenv
SERIAL_PORT=COM5       # порт, к которому подключена Arduino
SERIAL_BAUD=9600       # должно совпадать с Serial.begin() в скетче
DEVICE_ID=1
DEVICE_NAME=arduino-01
```

### 2. Прошить Arduino

Откройте `arduino/temperature_sensor/temperature_sensor.ino` в Arduino IDE и прошейте. Убедитесь:

- `Serial.begin(9600)` в скетче (совпадает с `SERIAL_BAUD`);
- LM35 подключён к A0, LED — к D7 (через резистор 220–470 Ом);
- закройте Serial Monitor после проверки — он держит порт монопольно.

Проверка: Serial Monitor на **9600**, NL → отправить `GET` → должен прийти `{"temperature":23.50,"led":false}`.

### 3. Собрать бинарники

```powershell
# Linux-бинарники для Docker + Windows-бинарник poller для хоста
.\scripts\build.ps1 all
```

Появится:

- `build/ingest-linux`
- `build/web-linux` (включая React-сборку из `web/`)
- `build/poller.exe`

Если фронт ещё не собирался — `build.ps1` сам выполнит `npm ci` (при отсутствии `node_modules`) и `npm run build`.

### 4. Запустить Docker-сервисы

```powershell
docker compose up -d --build
docker compose ps
```

Ожидаемо пять сервисов: `mosquitto`, `postgres`, `postgres-backup`, `ingest`, `web` — все `Up`.

Проверка API:

```powershell
curl http://localhost:8080/api/health      # {"status":"ok"}
curl http://localhost:8080/api/devices     # [{"id":1,"name":"arduino-01",...}]
```

Веб-интерфейс: http://localhost:8080

### 5. Запустить poller на хосте

```powershell
cd build
.\poller.exe
```

Ожидаемые логи:

```
loaded config from poller.env
published {"device_id":1,"temperature":23.50,"recorded_at":"..."}
```

Через 5 секунд (`POLL_INTERVAL`) — следующая строка. В браузере на графике появятся точки, а переключатель LED в UI начнёт включать/выключать диод.

## Разработка

### Остановка и запуск

```powershell
docker compose down                    # остановить, данные БД СОХРАНЯЮТСЯ
docker compose down -v                 # остановить + удалить тома (сброс БД и пароля Mosquitto)
docker compose restart ingest          # перезапустить один сервис
docker compose logs -f web             # смотреть логи
```

`poller.exe` останавливается `Ctrl+C`.

### Обновление отдельного сервиса

| Что поменяли | Что делать |
|---|---|
| `cmd/ingest/`, `internal/storage`, `internal/models` | `.\scripts\build.ps1 ingest-linux` → `docker compose up -d --build ingest` |
| `cmd/web/`, `web/src/` | `.\scripts\build.ps1 web-linux` → `docker compose up -d --build web` |
| `cmd/poller/` | `.\scripts\build.ps1 poller-windows` → перезапустить `build\poller.exe` |
| `arduino/*.ino` | Прошить через Arduino IDE |

`internal/config` и `internal/mqttclient` используются всеми Go-сервисами — после их правки пересобирайте **всё** (`.\scripts\build.ps1 all`).

### Frontend dev-режим (HMR без Docker)

Два терминала:

```powershell
# Терминал 1 — Go API
.\scripts\build.ps1 web-windows
cd build; .\web.exe

# Терминал 2 — Vite с hot-reload
cd web
npm run dev
```

Открыть http://localhost:5173 — Vite проксирует `/api/*` на `:8080`. Правки в `App.tsx` применяются мгновенно.

### Сборка вручную (без PowerShell-скриптов)

```powershell
# poller (Windows)
$env:CGO_ENABLED="0"; $env:GOOS="windows"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/poller.exe ./cmd/poller

# ingest и web (Linux, для контейнеров)
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/ingest-linux ./cmd/ingest
go build -trimpath -ldflags="-s -w" -o build/web-linux ./cmd/web
```

Linux/macOS: `make all`, `make up`, `make down`.

## Эксплуатация

### Просмотр БД

```powershell
docker compose exec postgres psql -U arduino -d arduino_telemetry

# внутри psql:
\dt                                    # таблицы
SELECT * FROM devices;                 # справочник
SELECT * FROM readings ORDER BY id DESC LIMIT 10;
SELECT COUNT(*), MIN(recorded_at), MAX(recorded_at) FROM readings;
```

### Резервные копии

Сервис `postgres-backup` делает `pg_dump` раз в 24 часа в `./data/backups/arduino_YYYYMMDD_HHMMSS.dump` и удаляет дампы старше 7 дней.

Восстановление в отдельную БД для проверки:

```powershell
docker compose exec postgres createdb -U arduino -T template0 restore_test
docker compose cp data/backups/<файл>.dump postgres:/tmp/r.dump
docker compose exec postgres pg_restore -U arduino -d restore_test /tmp/r.dump
docker compose exec postgres psql -U arduino -d restore_test -c "SELECT COUNT(*) FROM readings;"
docker compose exec postgres dropdb -U arduino restore_test
```

### Смена пароля MQTT

Файл `passwd` в томе Mosquitto создаётся **один раз**, поэтому простая правка `MQTT_PASSWORD` в `.env` не подействует. Два способа:

```powershell
# Точечно
docker compose exec mosquitto mosquitto_passwd -b /mosquitto/data/passwd arduino новый_пароль
docker compose restart mosquitto

# Или полный сброс (потеряются данные БД!)
docker compose down -v
docker compose up -d
```

Не забудьте синхронизировать `MQTT_PASSWORD` в `poller.env` и перезапустить `poller.exe`.

### Добавление второго устройства

1. В `infra/postgres/init/001_init.sql` — новая строка в `INSERT INTO devices` (или вручную через `psql`, если БД уже создана).
2. В `poller.env` — `DEVICE_ID=2`, `DEVICE_NAME=arduino-02`, свой `SERIAL_PORT` (например, `COM7`).
3. Запустить второй экземпляр `poller.exe` с этим env (из другой папки с собственным `poller.env`).
4. В UI появится `arduino-02` в выпадающем списке автоматически — `web` читает `devices` из БД.

## Диагностика

### `poller.exe`: `serial read timeout`

| Проверить | Как |
|---|---|
| Serial Monitor не открыт | Закрыть Arduino IDE / PuTTY / RealTerm — порт монопольный |
| Baudrate совпадает | `Serial.begin(9600)` в скетче = `SERIAL_BAUD=9600` в `poller.env` |
| Порт правильный | Диспетчер устройств → «Порты (COM и LPT)» → цифра после `COM` (по умолчанию `COM5`) |
| Прошит актуальный скетч | Serial Monitor (**9600**, NL) → `GET` → должен прийти JSON |
| `poller.env` подхватился | В логах строка `loaded config from poller.env`; запускать из `build\` |
| Таймаут мал для CH340-клона | В `internal/serial/client.go`: `SetReadTimeout(500 * time.Millisecond)` + пересобрать |

### `poller.exe`: `open serial COM5: Access denied`

Порт держит другой процесс — Arduino IDE (Serial Monitor), PuTTY, RealTerm, второй экземпляр poller. Закройте всё, что может обращаться к COM5, проверьте в Диспетчере задач (`Serial Monitor` / `java.exe` от Arduino IDE).

### Mosquitto рестартует: `Unable to open pwfile`

Файл `passwd` принадлежит `root`, а Mosquitto работает как `mosquitto`. `docker-entrypoint.sh` должен содержать **вне** блока `if`:

```sh
chown mosquitto:mosquitto "$PASSWD_FILE"
chmod 600 "$PASSWD_FILE"
```

После правки: `docker compose down && docker compose up -d`. Если файл уже «испорчен» в томе — `down -v`.

### `ingest` падает: `Connection Refused: not authorised`

Пароль в `.env` (`MQTT_PASSWORD`) не совпадает с тем, что в `passwd`-файле Mosquitto. Либо `down -v` для пересоздания, либо `mosquitto_passwd -b` вручную (см. выше).

### Веб: график пустой, в консоли `null.length`

Go вернул `null` вместо `[]` для пустой коллекции. В `internal/storage/postgres.go` должно быть `out := make([]T, 0)` (не `var out []T`). После правки — пересобрать `web-linux` и `docker compose up -d --build web`.

### Веб: 500 `unit "5m" not recognized`

В `ReadingsRange` используется `date_trunc` вместо `date_bin`. Должно быть:

```sql
date_bin($4::interval, recorded_at, timestamptz '2000-01-01 00:00:00+00')
```

А `handlers.go` маппит `1m`/`5m`/`1h` → `1 minute`/`5 minutes`/`1 hour`.

### `docker compose up` не пересобирает образ

Docker кэширует слои. Если бинарник пересобран, но Docker не видит изменений:

```powershell
docker compose up -d --build --force-recreate web
# или
docker compose build --no-cache web && docker compose up -d web
```

### `exec format error` / `bad interpreter` в entrypoint

CRLF-переводы строк в `.sh`-файле. В VS Code — правый нижний угол → `LF` → сохранить. Касается `docker-entrypoint.sh` и `postgres-backup.sh`.

### Очистка «под ноль»

```powershell
docker compose down -v              # контейнеры + тома (БД, Mosquitto passwd)
Remove-Item -Recurse data           # бэкапы
Remove-Item -Recurse build\*        # бинарники (кроме .gitkeep)
```

После этого — с шага 3 «Быстрый старт».