#!/bin/sh
set -e

PASSWD_FILE="/mosquitto/data/passwd"

if [ ! -f "$PASSWD_FILE" ]; then
    if [ -z "$MQTT_USERNAME" ] || [ -z "$MQTT_PASSWORD" ]; then
        echo "ERROR: MQTT_USERNAME and MQTT_PASSWORD must be set in .env" >&2
        exit 1
    fi
    echo "[mosquitto] creating password file for user '$MQTT_USERNAME'"
    mosquitto_passwd -c -b "$PASSWD_FILE" "$MQTT_USERNAME" "$MQTT_PASSWORD"
fi

# ✅ Чиним владельца и права КАЖДЫЙ раз:
# - при первом создании mosquitto_passwd пишет от root → файл root:root
# - при повторных запусках файл уже есть, но владелец может остаться неверным
chown mosquitto:mosquitto "$PASSWD_FILE"
chmod 600 "$PASSWD_FILE"

exec mosquitto -c /mosquitto/config/mosquitto.conf
