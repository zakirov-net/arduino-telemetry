package serial

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

const (
	maxLineLen = 4096
	// byteReadTimeout — таймаут одного Read: позволяет ReadLine регулярно
	// проверять общий дедлайн, не блокируясь навсегда.
	byteReadTimeout = 200 * time.Millisecond
)

var (
	// ErrReadTimeout возвращается, если строка не пришла за ReadTimeout.
	// Порт при этом остаётся рабочим (устройство просто не ответило).
	ErrReadTimeout = errors.New("serial read timeout")
	// ErrClosed возвращается при обращении к незакрытому/закрытому порту.
	ErrClosed = errors.New("serial port is closed")
)

// IsTimeout сообщает, что ошибка — таймаут чтения, а не потеря связи.
func IsTimeout(err error) bool { return errors.Is(err, ErrReadTimeout) }

type Client struct {
	port        serial.Port
	readTimeout time.Duration
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
	if err := port.SetReadTimeout(byteReadTimeout); err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("set read timeout: %w", err)
	}
	if readTimeout <= 0 {
		readTimeout = 2 * time.Second
	}
	return &Client{port: port, readTimeout: readTimeout}, nil
}

// Close закрывает порт. Ошибка закрытия не возвращается: после физического
// отключения USB порт уже невалиден, и это не должно мешать переподключению.
func (c *Client) Close() error {
	if c.port == nil {
		return nil
	}
	_ = c.port.Close()
	c.port = nil
	return nil
}

// WriteLine отправляет строку, добавляя '\n', если её нет.
func (c *Client) WriteLine(s string) error {
	if c.port == nil {
		return ErrClosed
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := c.port.Write([]byte(s))
	return err
}

// ReadLine читает строку до '\n'. Если строка не пришла за ReadTimeout,
// возвращает ErrReadTimeout; прочие ошибки означают потерю связи с портом.
func (c *Client) ReadLine() (string, error) {
	if c.port == nil {
		return "", ErrClosed
	}
	deadline := time.Now().Add(c.readTimeout)
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		if !time.Now().Before(deadline) {
			return "", ErrReadTimeout
		}
		n, err := c.port.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			// Порт жив, но данных пока нет — ждём до дедлайна.
			continue
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
	if c.port == nil {
		return ErrClosed
	}
	return c.port.ResetInputBuffer()
}

// AvailablePorts возвращает список портов, видимых системой. Используется
// в диагностических сообщениях, когда нужный порт открыть не удалось.
func AvailablePorts() []string {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil
	}
	return ports
}
