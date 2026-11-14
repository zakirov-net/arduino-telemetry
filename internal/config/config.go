package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MQTT — общая часть конфигурации для poller и ingest.
type MQTT struct {
	Broker    string
	Username  string
	Password  string
	ClientID  string
	BaseTopic string // без завершающего '/'
}

type Poller struct {
	MQTT
	DeviceID     int16
	DeviceName   string
	SerialPort   string
	SerialBaud   int
	PollInterval time.Duration
	ReadTimeout  time.Duration
}

func (p Poller) DataTopic() string    { return p.BaseTopic + "/" + p.DeviceName + "/data" }
func (p Poller) CommandTopic() string { return p.BaseTopic + "/" + p.DeviceName + "/command" }
func (p Poller) StatusTopic() string  { return p.BaseTopic + "/" + p.DeviceName + "/status" }

type Ingest struct {
	MQTT
	PostgresDSN string
}

func (i Ingest) DataTopicFilter() string { return i.BaseTopic + "/+/data" }

// LoadPoller читает poller.env.
func LoadPoller() (Poller, error) {
	m, err := loadMQTT("MQTT_CLIENT_ID_POLLER")
	if err != nil {
		return Poller{}, err
	}

	rawID, err := mustEnv("DEVICE_ID")
	if err != nil {
		return Poller{}, err
	}
	id64, err := strconv.ParseInt(rawID, 10, 16)
	if err != nil {
		return Poller{}, fmt.Errorf("DEVICE_ID must fit into SMALLINT: %w", err)
	}

	baud, err := getEnvInt("SERIAL_BAUD", 115200)
	if err != nil {
		return Poller{}, err
	}
	poll, err := getEnvDuration("POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return Poller{}, err
	}
	readTO, err := getEnvDuration("SERIAL_READ_TIMEOUT", 2*time.Second)
	if err != nil {
		return Poller{}, err
	}

	devName, err := mustEnv("DEVICE_NAME")
	if err != nil {
		return Poller{}, err
	}
	serialPort, err := mustEnv("SERIAL_PORT")
	if err != nil {
		return Poller{}, err
	}

	return Poller{
		MQTT:         m,
		DeviceID:     int16(id64),
		DeviceName:   devName,
		SerialPort:   serialPort,
		SerialBaud:   baud,
		PollInterval: poll,
		ReadTimeout:  readTO,
	}, nil
}

// LoadIngest читает .env (файл используется docker-compose).
func LoadIngest() (Ingest, error) {
	m, err := loadMQTT("MQTT_CLIENT_ID_INGEST")
	if err != nil {
		return Ingest{}, err
	}
	dsn, err := buildPostgresDSN()
	if err != nil {
		return Ingest{}, err
	}
	return Ingest{MQTT: m, PostgresDSN: dsn}, nil
}

func loadMQTT(clientIDEnv string) (MQTT, error) {
	broker, err := mustEnv("MQTT_BROKER")
	if err != nil {
		return MQTT{}, err
	}
	username, err := mustEnv("MQTT_USERNAME")
	if err != nil {
		return MQTT{}, err
	}
	password, err := mustEnv("MQTT_PASSWORD")
	if err != nil {
		return MQTT{}, err
	}
	baseTopic, err := mustEnv("MQTT_BASE_TOPIC")
	if err != nil {
		return MQTT{}, err
	}
	clientID, err := mustEnv(clientIDEnv)
	if err != nil {
		return MQTT{}, err
	}
	return MQTT{
		Broker:    broker,
		Username:  username,
		Password:  password,
		ClientID:  clientID,
		BaseTopic: strings.TrimRight(baseTopic, "/"),
	}, nil
}

// buildPostgresDSN собирает DSN из отдельных переменных (или берёт POSTGRES_DSN как override).
func buildPostgresDSN() (string, error) {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn, nil
	}
	user, err := mustEnv("POSTGRES_USER")
	if err != nil {
		return "", err
	}
	pass, err := mustEnv("POSTGRES_PASSWORD")
	if err != nil {
		return "", err
	}
	host, err := mustEnv("POSTGRES_HOST")
	if err != nil {
		return "", err
	}
	port, err := mustEnv("POSTGRES_PORT")
	if err != nil {
		return "", err
	}
	db, err := mustEnv("POSTGRES_DB")
	if err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   host + ":" + port,
		Path:   "/" + db,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func mustEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required env %s is empty", key)
	}
	return v, nil
}

func getEnvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be int: %w", key, err)
	}
	return n, nil
}

func getEnvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be duration (e.g. 5s): %w", key, err)
	}
	return d, nil
}

type Web struct {
	MQTT
	ListenAddr  string
	PostgresDSN string
}

func LoadWeb() (Web, error) {
	m, err := loadMQTT("MQTT_CLIENT_ID_WEB")
	if err != nil {
		return Web{}, err
	}
	dsn, err := buildPostgresDSN()
	if err != nil {
		return Web{}, err
	}
	return Web{
		MQTT:        m,
		ListenAddr:  getEnvStr("WEB_LISTEN_ADDR", ":8080"),
		PostgresDSN: dsn,
	}, nil
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
