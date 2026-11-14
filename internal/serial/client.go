package serial

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

const maxLineLen = 4096

type Client struct {
	port serial.Port
}

// Open открывает порт и ждёт, пока Arduino перезагрузится после установки DTR.
func Open(name string, baud int, readTimeout time.Duration) (*Client, error) {
	mode := &serial.Mode{BaudRate: baud}
	port, err := serial.Open(name, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial %s: %w", name, err)
	}
	// Arduino Nano/Uno перезагружаются при открытии порта — ждём загрузчик.
	time.Sleep(2 * time.Second)
	_ = port.ResetInputBuffer()
	if err := port.SetReadTimeout(200 * time.Millisecond); err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("set read timeout: %w", err)
	}
	_ = readTimeout // используется на уровне вызова (таймаут чтения строки в poller)
	return &Client{port: port}, nil
}

func (c *Client) Close() error { return c.port.Close() }

// WriteLine отправляет строку, добавляя '\n', если её нет.
func (c *Client) WriteLine(s string) error {
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := c.port.Write([]byte(s))
	return err
}

// ReadLine читает строку до '\n' с таймаутом на каждый байт.
// Если байты не приходят дольше ReadTimeout порта — вернёт ошибку.
func (c *Client) ReadLine() (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := c.port.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", errors.New("serial read timeout")
		}
		b := buf[0]
		if b == '\n' {
			return strings.TrimRight(sb.String(), "\r"), nil
		}
		sb.WriteByte(b)
		if sb.Len() > maxLineLen {
			return "", errors.New("serial line too long")
		}
	}
}

func (c *Client) FlushInput() error {
	return c.port.ResetInputBuffer()
}
