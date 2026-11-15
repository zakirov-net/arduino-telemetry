package main

import (
	"context"
	"encoding/json"
	"fmt"
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

const (
	// serialRetryDelay — пауза перед повторной попыткой открыть порт после
	// потери связи: Windows не всегда отдаёт порт сразу после переподключения.
	serialRetryDelay = 3 * time.Second
	// serialTimeoutLimit — сколько подряд таймаутов чтения терпим, прежде чем
	// переоткрыть порт: часть USB-переходников «залипает» без явной ошибки.
	serialTimeoutLimit = 3
	// serialErrLogInterval — как часто повторять в лог одинаковую ошибку порта.
	serialErrLogInterval = time.Minute
)

// serialPort — операции над портом, нужные poller'у (интерфейс для тестов).
type serialPort interface {
	FlushInput() error
	WriteLine(string) error
	ReadLine() (string, error)
	Close() error
}

// openPortFunc открывает порт; подменяется в тестах.
type openPortFunc func(name string, baud int, readTimeout time.Duration) (serialPort, error)

func openSerial(name string, baud int, readTimeout time.Duration) (serialPort, error) {
	sc, err := serial.Open(name, baud, readTimeout)
	if err != nil {
		return nil, err
	}
	return sc, nil
}

type poller struct {
	sc  serialPort
	mc  mqtt.Client
	cfg config.Poller
	mu  sync.Mutex // сериализует транзакции по serial: GET-цикл и команды MQTT

	openPort openPortFunc // nil => openSerial

	nextSerialAttempt time.Time // раньше этого момента порт не переоткрываем
	readTimeouts      int       // подряд идущие таймауты чтения
	lastSerialErr     string    // для rate-limit логов о недоступном порте
	lastSerialErrLog  time.Time
}

// transact отправляет строку и читает ответ. Держит mutex на всю транзакцию.
// FlushInput перед записью отбрасывает «просроченный» ответ от предыдущей
// операции, если та завершилась по таймауту.
//
// Любая ошибка порта (кроме таймаута чтения) означает потерю USB-соединения:
// порт закрывается, а следующая транзакция откроет его заново.
func (p *poller) transact(cmd string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureSerial(); err != nil {
		return "", err
	}
	if err := p.sc.FlushInput(); err != nil {
		p.dropSerial(err)
		return "", err
	}
	if err := p.sc.WriteLine(cmd); err != nil {
		p.dropSerial(err)
		return "", err
	}
	line, err := p.sc.ReadLine()
	if err != nil {
		if serial.IsTimeout(err) {
			p.noteReadTimeout()
			return "", err
		}
		p.dropSerial(err)
		return "", err
	}
	p.readTimeouts = 0
	return line, nil
}

// ensureSerial открывает порт, если он ещё не открыт или был потерян.
// В пределах backoff порт не дёргаем, чтобы не тормозить цикл опроса.
func (p *poller) ensureSerial() error {
	if p.sc != nil {
		return nil
	}
	now := time.Now()
	if now.Before(p.nextSerialAttempt) {
		return fmt.Errorf("serial %s offline, next attempt at %s",
			p.cfg.SerialPort, p.nextSerialAttempt.Format("15:04:05"))
	}

	open := p.openPort
	if open == nil {
		open = openSerial
	}
	sc, err := open(p.cfg.SerialPort, p.cfg.SerialBaud, p.cfg.ReadTimeout)
	if err != nil {
		p.nextSerialAttempt = now.Add(serialRetryDelay)
		p.logSerialErr(err)
		return err
	}
	p.sc = sc
	p.nextSerialAttempt = time.Time{}
	p.readTimeouts = 0
	p.lastSerialErr = ""
	log.Printf("serial %s connected", p.cfg.SerialPort)
	return nil
}

// dropSerial закрывает потерянный порт; следующая транзакция попробует
// открыть его заново не раньше чем через serialRetryDelay.
func (p *poller) dropSerial(cause error) {
	p.readTimeouts = 0
	if p.sc == nil {
		return
	}
	log.Printf("serial %s: connection lost: %v", p.cfg.SerialPort, cause)
	_ = p.sc.Close()
	p.sc = nil
	p.nextSerialAttempt = time.Now().Add(serialRetryDelay)
}

// noteReadTimeout считает подряд идущие таймауты: если устройство молчит
// слишком долго, порт переоткрывается (лечит «залипшие» USB-переходники).
func (p *poller) noteReadTimeout() {
	p.readTimeouts++
	if p.readTimeouts < serialTimeoutLimit {
		return
	}
	p.dropSerial(fmt.Errorf("device stopped answering (%d read timeouts)", p.readTimeouts))
}

// logSerialErr логирует проблему с портом, не спамя одинаковыми сообщениями.
func (p *poller) logSerialErr(err error) {
	msg := err.Error()
	now := time.Now()
	if msg == p.lastSerialErr && now.Sub(p.lastSerialErrLog) < serialErrLogInterval {
		return
	}
	p.lastSerialErr = msg
	p.lastSerialErrLog = now

	ports := serial.AvailablePorts()
	if len(ports) == 0 {
		log.Printf("serial %s unavailable: %v (no serial ports detected)", p.cfg.SerialPort, err)
		return
	}
	log.Printf("serial %s unavailable: %v (available ports: %s)",
		p.cfg.SerialPort, err, strings.Join(ports, ", "))
}

// close закрывает порт при остановке.
func (p *poller) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sc != nil {
		_ = p.sc.Close()
		p.sc = nil
	}
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

	// Порт открывается лениво (см. ensureSerial): poller продолжает работать,
	// даже если Arduino отключена или появилась в системе позже.
	p := &poller{mc: mc, cfg: cfg, openPort: openSerial}
	defer p.close()

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
