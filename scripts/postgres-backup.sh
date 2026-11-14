#!/bin/sh
# Периодический pg_dump в /backups с ротацией.
# Запускается в контейнере postgres-backup.
set -eu

INTERVAL=${BACKUP_INTERVAL:-86400}       # секунд между дампами (сутки по умолчанию)
RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-7}

mkdir -p /backups

# Ждём, пока postgres начнёт принимать подключения
until pg_isready -h postgres -U "$POSTGRES_USER" -q; do
    echo "[backup] waiting for postgres..."
    sleep 2
done

while true; do
    TS=$(date +%Y%m%d_%H%M%S)
    FILE="/backups/arduino_${TS}.dump"
    echo "[backup] dumping to $FILE"
    if PGPASSWORD="$POSTGRES_PASSWORD" pg_dump \
        -h postgres -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
        -Fc -f "$FILE"; then
        echo "[backup] ok ($(du -h "$FILE" | cut -f1))"
    else
        echo "[backup] FAILED" >&2
        rm -f "$FILE"
    fi

    echo "[backup] pruning files older than ${RETENTION_DAYS} days"
    find /backups -name 'arduino_*.dump' -mtime "+${RETENTION_DAYS}" -delete

    sleep "$INTERVAL"
done