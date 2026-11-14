package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/zakirov-net/arduino-telemetry/internal/config"
	"github.com/zakirov-net/arduino-telemetry/internal/models"
	"github.com/zakirov-net/arduino-telemetry/internal/mqttclient"
	"github.com/zakirov-net/arduino-telemetry/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, err := config.LoadIngest()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// 1. Postgres с ретраями: контейнер может стартовать дольше, чем мы.
	store, err := storage.OpenWithRetry(cfg.PostgresDSN, 30*time.Second)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer store.Close()

	// 2. MQTT.
	mqttClient, err := mqttclient.NewClient(mqttclient.Options{
		Broker:   cfg.Broker,
		ClientID: cfg.ClientID,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		log.Fatalf("mqtt: %v", err)
	}
	defer mqttClient.Disconnect(250)

	// 3. Подписка на все устройства: home/arduino/+/data
	filter := cfg.DataTopicFilter()
	err = mqttclient.Subscribe(mqttClient, filter, 1, func(_ mqtt.Client, msg mqtt.Message) {
		handleMessage(store, msg)
	})
	if err != nil {
		log.Fatalf("subscribe %s: %v", filter, err)
	}
	log.Printf("subscribed to %s", filter)

	// 4. Ждём Ctrl+C или SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	log.Println("shutting down")
}

func handleMessage(store *storage.Store, msg mqtt.Message) {
	var r models.Reading
	if err := json.Unmarshal(msg.Payload(), &r); err != nil {
		log.Printf("bad payload on %s: %v", msg.Topic(), err)
		return
	}
	if r.RecordedAt.IsZero() {
		r.RecordedAt = time.Now().UTC()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.InsertReading(ctx, r); err != nil {
		log.Printf("insert: %v", err)
		return
	}
	log.Printf("stored device=%d temp=%s at=%s",
		r.DeviceID, fmtFloat(r.Temperature),
		r.RecordedAt.Format(time.RFC3339))
}

func fmtFloat(p *float64) string {
	if p == nil {
		return "NULL"
	}
	return strconv.FormatFloat(*p, 'f', 2, 64)
}
