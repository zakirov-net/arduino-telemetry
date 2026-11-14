package mqttclient

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Publish — синхронная публикация с ожиданием подтверждения.
func Publish(client mqtt.Client, topic string, payload []byte, qos byte, retain bool) error {
	tok := client.Publish(topic, qos, retain, payload)
	tok.Wait()
	return tok.Error()
}
