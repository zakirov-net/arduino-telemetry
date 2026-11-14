package models

import "time"

// Reading — то, что летит в MQTT и затем в таблицу readings.
type Reading struct {
	DeviceID    int16     `json:"device_id"`             // SMALLINT в БД
	Temperature *float64  `json:"temperature,omitempty"` // *float64 → NULL, если датчик молчит
	RecordedAt  time.Time `json:"recorded_at"`           // UTC
}

type Device struct {
	ID       int16   `json:"id"`
	Name     string  `json:"name"`
	Location *string `json:"location,omitempty"`
}

type AggregatedReading struct {
	RecordedAt  time.Time `json:"recorded_at"`
	Temperature *float64  `json:"temperature"`
}
