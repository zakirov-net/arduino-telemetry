package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/zakirov-net/arduino-telemetry/internal/models"
)

type Store struct {
	db *sql.DB
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open pgx: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Close() error                   { return s.db.Close() }

func (s *Store) InsertReading(ctx context.Context, r models.Reading) error {
	const q = `
		INSERT INTO readings (device_id, temperature, recorded_at)
		VALUES ($1, $2, $3)`
	_, err := s.db.ExecContext(ctx, q, r.DeviceID, r.Temperature, r.RecordedAt)
	return err
}

func (s *Store) ListDevices(ctx context.Context) ([]models.Device, error) {
	const q = `SELECT id, name, location FROM devices ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Device, 0)
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.Name, &d.Location); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ReadingsRange(ctx context.Context, deviceID int16, from, to time.Time, bucketInterval string) ([]models.AggregatedReading, error) {
	const q = `
		SELECT date_bin($4::interval, recorded_at, timestamptz '2000-01-01 00:00:00+00') AS recorded_at,
		       AVG(temperature)                                                          AS temperature
		FROM readings
		WHERE device_id = $1 AND recorded_at BETWEEN $2 AND $3
		GROUP BY 1
		ORDER BY 1`
	rows, err := s.db.QueryContext(ctx, q, deviceID, from, to, bucketInterval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.AggregatedReading, 0)
	for rows.Next() {
		var r models.AggregatedReading
		if err := rows.Scan(&r.RecordedAt, &r.Temperature); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
