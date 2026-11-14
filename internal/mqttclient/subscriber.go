package mqttclient

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Will struct {
	Topic   string
	Payload string
	QoS     byte
	Retain  bool
}

type Options struct {
	Broker   string
	ClientID string
	Username string
	Password string
	Will     *Will
}

// NewClient создаёт клиента и сразу подключается (с ретраями).
func NewClient(opts Options) (mqtt.Client, error) {
	o := mqtt.NewClientOptions().
		AddBroker(opts.Broker).
		SetClientID(opts.ClientID).
		SetUsername(opts.Username).
		SetPassword(opts.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetConnectTimeout(5 * time.Second).
		SetCleanSession(true)

	if opts.Will != nil {
		o.SetWill(opts.Will.Topic, opts.Will.Payload, opts.Will.QoS, opts.Will.Retain)
	}

	c := mqtt.NewClient(o)
	tok := c.Connect()
	tok.Wait()
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect: %w", err)
	}
	return c, nil
}

// Subscribe — синхронная подписка.
func Subscribe(client mqtt.Client, filter string, qos byte, handler mqtt.MessageHandler) error {
	tok := client.Subscribe(filter, qos, handler)
	tok.Wait()
	return tok.Error()
}
