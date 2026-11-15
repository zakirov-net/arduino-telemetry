package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zakirov-net/arduino-telemetry/internal/config"
	"github.com/zakirov-net/arduino-telemetry/internal/serial"
)

// fakePort — заглушка serialPort для проверки переподключения без железа.
type fakePort struct {
	writes   []string
	lines    []string
	writeErr error
	readErr  error
	closed   bool
}

func (f *fakePort) FlushInput() error {
	if f.closed {
		return errors.New("port closed")
	}
	return nil
}

func (f *fakePort) WriteLine(s string) error {
	if f.closed {
		return errors.New("port closed")
	}
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes = append(f.writes, s)
	return nil
}

func (f *fakePort) ReadLine() (string, error) {
	if f.closed {
		return "", errors.New("port closed")
	}
	if f.readErr != nil {
		return "", f.readErr
	}
	if len(f.lines) == 0 {
		return "", serial.ErrReadTimeout
	}
	line := f.lines[0]
	f.lines = f.lines[1:]
	return line, nil
}

func (f *fakePort) Close() error {
	f.closed = true
	return nil
}

type openResult struct {
	port *fakePort
	err  error
}

// fakeOpener отдаёт результаты открытия по очереди и считает попытки.
type fakeOpener struct {
	results []openResult
	calls   int
}

func (o *fakeOpener) open(string, int, time.Duration) (serialPort, error) {
	if o.calls >= len(o.results) {
		o.calls++
		return nil, errors.New("unexpected open call")
	}
	r := o.results[o.calls]
	o.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.port, nil
}

func newTestPoller(opener *fakeOpener) *poller {
	return &poller{
		cfg: config.Poller{
			SerialPort:   "COM3",
			SerialBaud:   9600,
			ReadTimeout:  2 * time.Second,
			PollInterval: 5 * time.Second,
		},
		openPort: opener.open,
	}
}

// Переподключение после «Access is denied» (физическое отключение Arduino).
func TestTransactReopensPortAfterIOError(t *testing.T) {
	broken := &fakePort{writeErr: errors.New("Access is denied.")}
	healthy := &fakePort{lines: []string{`{"temperature":21.5}`}}
	opener := &fakeOpener{results: []openResult{{port: broken}, {port: healthy}}}
	p := newTestPoller(opener)

	_, err := p.transact("GET")
	if err == nil || !strings.Contains(err.Error(), "Access is denied") {
		t.Fatalf("expected access denied from broken port, got %v", err)
	}
	if !broken.closed {
		t.Fatal("broken port must be closed")
	}
	if p.sc != nil {
		t.Fatal("broken port must be dropped")
	}

	// В пределах backoff порт повторно не открываем.
	_, err = p.transact("GET")
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("expected offline error during backoff, got %v", err)
	}
	if opener.calls != 1 {
		t.Fatalf("port must not be reopened during backoff, opens=%d", opener.calls)
	}

	// После backoff — переоткрытие и успешная транзакция.
	p.nextSerialAttempt = time.Time{}
	line, err := p.transact("GET")
	if err != nil {
		t.Fatalf("transact after reconnect: %v", err)
	}
	if line != `{"temperature":21.5}` {
		t.Fatalf("unexpected line %q", line)
	}
	if opener.calls != 2 {
		t.Fatalf("expected 2 open calls, got %d", opener.calls)
	}
	if len(healthy.writes) != 1 || healthy.writes[0] != "GET" {
		t.Fatalf("unexpected writes on reconnected port: %v", healthy.writes)
	}
}

// Одиночный таймаут не рвёт соединение, но серия таймаутов — переоткрывает порт.
func TestTransactReopensAfterRepeatedReadTimeouts(t *testing.T) {
	silent := &fakePort{readErr: serial.ErrReadTimeout}
	healthy := &fakePort{lines: []string{"ok"}}
	opener := &fakeOpener{results: []openResult{{port: silent}, {port: healthy}}}
	p := newTestPoller(opener)

	for i := 0; i < serialTimeoutLimit-1; i++ {
		if _, err := p.transact("GET"); !errors.Is(err, serial.ErrReadTimeout) {
			t.Fatalf("attempt %d: expected read timeout, got %v", i, err)
		}
	}
	if p.sc == nil || silent.closed {
		t.Fatal("port must stay open below the timeout limit")
	}

	if _, err := p.transact("GET"); !errors.Is(err, serial.ErrReadTimeout) {
		t.Fatalf("expected read timeout on limit attempt, got %v", err)
	}
	if p.sc != nil {
		t.Fatal("port must be dropped after repeated read timeouts")
	}
	if !silent.closed {
		t.Fatal("silent port must be closed")
	}

	p.nextSerialAttempt = time.Time{}
	line, err := p.transact("GET")
	if err != nil {
		t.Fatalf("transact after reconnect: %v", err)
	}
	if line != "ok" {
		t.Fatalf("unexpected line %q", line)
	}
}

// Если порт не открывается (Arduino ещё не подключена), poller не падает,
// а повторяет попытки с задержкой.
func TestEnsureSerialRetriesAfterOpenFailure(t *testing.T) {
	healthy := &fakePort{lines: []string{"ok"}}
	opener := &fakeOpener{results: []openResult{
		{err: errors.New("open serial COM3: Access is denied.")},
		{port: healthy},
	}}
	p := newTestPoller(opener)

	if _, err := p.transact("GET"); err == nil {
		t.Fatal("expected open error")
	}
	if p.nextSerialAttempt.IsZero() {
		t.Fatal("backoff must be scheduled after open failure")
	}
	if _, err := p.transact("GET"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("expected offline error during backoff, got %v", err)
	}
	if opener.calls != 1 {
		t.Fatalf("expected single open attempt during backoff, got %d", opener.calls)
	}

	p.nextSerialAttempt = time.Time{}
	if _, err := p.transact("GET"); err != nil {
		t.Fatalf("transact after port appeared: %v", err)
	}
	if opener.calls != 2 {
		t.Fatalf("expected 2 open calls, got %d", opener.calls)
	}
}

// Успешная транзакция сбрасывает счётчик таймаутов.
func TestTransactResetsTimeoutCounterOnSuccess(t *testing.T) {
	port := &fakePort{lines: []string{"", "ok"}}
	opener := &fakeOpener{results: []openResult{{port: port}}}
	p := newTestPoller(opener)

	// Пустая строка — это ответ устройства ('\n'), а не таймаут.
	if _, err := p.transact("GET"); err != nil {
		t.Fatalf("first transact: %v", err)
	}
	p.readTimeouts = serialTimeoutLimit - 1
	if _, err := p.transact("GET"); err != nil {
		t.Fatalf("second transact: %v", err)
	}
	if p.readTimeouts != 0 {
		t.Fatalf("timeout counter must be reset, got %d", p.readTimeouts)
	}
	if p.sc == nil {
		t.Fatal("port must stay open after successful transactions")
	}
}
