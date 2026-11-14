CREATE TABLE IF NOT EXISTS devices (
    id         SMALLINT PRIMARY KEY,           -- 2 байта, вручную назначаемый
    name       TEXT NOT NULL UNIQUE,           -- 'arduino-01' — для человека
    location   TEXT,                           -- 'living-room' — опционально
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Начальный справочник. Добавляйте новые устройства сюда же.
INSERT INTO devices (id, name, location) VALUES
    (1, 'arduino-01', 'living-room')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS readings (
    id            BIGSERIAL PRIMARY KEY,
    device_id     SMALLINT NOT NULL REFERENCES devices(id),
    temperature   DOUBLE PRECISION,
    recorded_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_readings_device_time
    ON readings (device_id, recorded_at DESC);