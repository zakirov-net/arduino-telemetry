package serial

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.bug.st/serial"
)

// fakeSerialPort имитирует serial.Port: Read отдаёт данные из очереди,
// а при пустой очереди возвращает (0, nil) — как порт с таймаутом.
type fakeSerialPort struct {
	data    []byte
	written []byte
	closed  bool
}

func (f *fakeSerialPort) SetMode(*serial.Mode) error { return nil }

func (f *fakeSerialPort) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		time.Sleep(2 * time.Millisecond)
		return 0, nil
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

func (f *fakeSerialPort) Write(p []byte) (int, error) {
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *fakeSerialPort) Drain() error { return nil }

func (f *fakeSerialPort) ResetInputBuffer() error {
	f.data = nil
	return nil
}

func (f *fakeSerialPort) ResetOutputBuffer() error           { return nil }
func (f *fakeSerialPort) SetDTR(bool) error                  { return nil }
func (f *fakeSerialPort) SetRTS(bool) error                  { return nil }
func (f *fakeSerialPort) SetReadTimeout(time.Duration) error { return nil }
func (f *fakeSerialPort) Break(time.Duration) error          { return nil }

func (f *fakeSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (f *fakeSerialPort) Close() error {
	f.closed = true
	return nil
}

func TestReadLineReturnsLine(t *testing.T) {
	port := &fakeSerialPort{data: []byte("{\"temperature\":21.5}\r\n")}
	c := &Client{port: port, readTimeout: 100 * time.Millisecond}

	line, err := c.ReadLine()
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if line != `{"temperature":21.5}` {
		t.Fatalf("unexpected line %q", line)
	}
}

func TestReadLineTimeoutIsDetectable(t *testing.T) {
	c := &Client{port: &fakeSerialPort{}, readTimeout: 40 * time.Millisecond}

	start := time.Now()
	_, err := c.ReadLine()
	if !errors.Is(err, ErrReadTimeout) {
		t.Fatalf("expected ErrReadTimeout, got %v", err)
	}
	if !IsTimeout(err) {
		t.Fatal("IsTimeout must report read timeout")
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("read must wait for the whole timeout, elapsed %v", elapsed)
	}
}

func TestIsTimeoutRejectsOtherErrors(t *testing.T) {
	if IsTimeout(nil) {
		t.Fatal("nil must not be a timeout")
	}
	if IsTimeout(errors.New("Access is denied.")) {
		t.Fatal("port I/O error must not be treated as timeout")
	}
	if !IsTimeout(fmt.Errorf("wrapped: %w", ErrReadTimeout)) {
		t.Fatal("wrapped read timeout must be detected")
	}
}

func TestWriteLineAppendsNewlineOnce(t *testing.T) {
	port := &fakeSerialPort{}
	c := &Client{port: port, readTimeout: time.Second}

	if err := c.WriteLine("GET"); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := c.WriteLine("PING\n"); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if got, want := string(port.written), "GET\nPING\n"; got != want {
		t.Fatalf("written %q, want %q", got, want)
	}
}

func TestClosedClientReturnsErrClosed(t *testing.T) {
	c := &Client{}
	if err := c.WriteLine("GET"); !errors.Is(err, ErrClosed) {
		t.Fatalf("WriteLine: expected ErrClosed, got %v", err)
	}
	if _, err := c.ReadLine(); !errors.Is(err, ErrClosed) {
		t.Fatalf("ReadLine: expected ErrClosed, got %v", err)
	}
	if err := c.FlushInput(); !errors.Is(err, ErrClosed) {
		t.Fatalf("FlushInput: expected ErrClosed, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on empty client: %v", err)
	}
}

func TestCloseReleasesPort(t *testing.T) {
	port := &fakeSerialPort{}
	c := &Client{port: port, readTimeout: time.Second}

	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !port.closed {
		t.Fatal("underlying port must be closed")
	}
	if err := c.WriteLine("GET"); !errors.Is(err, ErrClosed) {
		t.Fatalf("write after close: expected ErrClosed, got %v", err)
	}
}
