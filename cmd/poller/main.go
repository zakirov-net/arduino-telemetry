package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"

	"github.com/zakirov-net/arduino-telemetry/internal/config"
	"github.com/zakirov-net/arduino-telemetry/internal/models"
	"github.com/zakirov-net/arduino-telemetry/internal/mqttclient"
	"github.com/zakirov-net/arduino-telemetry/internal/serial"
)

type poller struct {
	sc  *serial.Client
	mc  mqtt.Client
	cfg config.Poller
	mu  sync.Mutex // сериализует транзакции по serial: GET-цикл и команды MQTT
}

// transact отправляет строку и читает ответ. Держит mutex на всю транзакцию.
// FlushInput перед записью отбрасывает «просроченный» ответ от предыдущей
// операции, если та завершилась по таймауту.
func (p *poller) transact(cmd string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.sc.FlushInput(); err != nil {
		return "", err
	}
	if err := p.sc.WriteLine(cmd); err != nil {
		return "", err
	}
	return p.sc.ReadLine()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if _, err := os.Stat("poller.env"); err == nil {
		if err := godotenv.Load("poller.env"); err != nil {
			log.Fatalf("load poller.env: %v", err)
		}
		log.Println("loaded config from poller.env")
	}

	cfg, err := config.LoadPoller()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	mc, err := mqttclient.NewClient(mqttclient.Options{
		Broker:   cfg.Broker,
		ClientID: cfg.ClientID,
		Username: cfg.Username,
		Password: cfg.Password,
		Will: &mqttclient.Will{
			Topic:   cfg.StatusTopic(),
			Payload: "offline",
			QoS:     1,
			Retain:  true,
		},
	})
	if err != nil {
		log.Fatalf("mqtt: %v", err)
	}
	defer mc.Disconnect(250)

	if err := mqttclient.Publish(mc, cfg.StatusTopic(), []byte("online"), 1, true); err != nil {
		log.Fatalf("publish online: %v", err)
	}

	sc, err := serial.Open(cfg.SerialPort, cfg.SerialBaud, cfg.ReadTimeout)
	if err != nil {
		log.Fatalf("serial: %v", err)
	}
	defer sc.Close()

	p := &poller{sc: sc, mc: mc, cfg: cfg}

	if err := mqttclient.Subscribe(mc, cfg.CommandTopic(), 1, p.handleCommand); err != nil {
		log.Fatalf("subscribe %s: %v", cfg.CommandTopic(), err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	p.pollAndPublish()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			_ = mqttclient.Publish(mc, cfg.StatusTopic(), []byte("offline"), 1, true)
			return
		case <-ticker.C:
			p.pollAndPublish()
		}
	}
}

func (p *poller) handleCommand(_ mqtt.Client, msg mqtt.Message) {
	cmd := strings.TrimSpace(string(msg.Payload()))
	if cmd == "" {
		return
	}
	log.Printf("cmd from mqtt: %q", cmd)
	resp, err := p.transact(cmd)
	if err != nil {
		log.Printf("cmd %q failed: %v", cmd, err)
		return
	}
	log.Printf("cmd %q -> %s", cmd, resp)
}

func (p *poller) pollAndPublish() {
	line, err := p.transact("GET")
	if err != nil {
		log.Printf("GET: %v", err)
		return
	}

	var raw struct {
		Temperature *float64 `json:"temperature"`
		Error       string   `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		log.Printf("bad arduino payload %q: %v", line, err)
		return
	}
	if raw.Error != "" {
		log.Printf("arduino error: %s", raw.Error)
		return
	}

	reading := models.Reading{
		DeviceID:    p.cfg.DeviceID,
		Temperature: raw.Temperature,
		RecordedAt:  time.Now().UTC(),
	}
	payload, err := json.Marshal(reading)
	if err != nil {
		log.Printf("marshal: %v", err)
		return
	}
	if err := mqttclient.Publish(p.mc, p.cfg.DataTopic(), payload, 0, false); err != nil {
		log.Printf("publish data: %v", err)
		return
	}
	log.Printf("published %s", payload)
}
