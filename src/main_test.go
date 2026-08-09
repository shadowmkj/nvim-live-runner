package main

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTCPStreamReading(t *testing.T) {
	// Create a large payload > 2056 bytes (e.g., 10 KB of Python code)
	lines := make([]string, 500)
	for i := 0; i < 500; i++ {
		lines[i] = "print('Line " + string(rune('A'+(i%26))) + " output')"
	}
	payload := strings.Join(lines, "\n")

	if len(payload) <= 2056 {
		t.Fatalf("Payload must be larger than 2056 bytes, got %d", len(payload))
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	done := make(chan string, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		data, err := io.ReadAll(conn)
		if err != nil && err != io.EOF {
			t.Errorf("io.ReadAll error: %v", err)
		}
		done <- string(data)
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	_, err = conn.Write([]byte(payload))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	conn.Close()

	select {
	case received := <-done:
		if len(received) != len(payload) {
			t.Fatalf("Expected received length %d, got %d", len(payload), len(received))
		}
		if received != payload {
			t.Fatalf("Received payload content does not match original payload")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Timed out waiting for TCP stream payload")
	}
}

func TestDynamicLanguageHeaderParsing(t *testing.T) {
	rawPayload := ".py\nprint('hello from dynamic Python')"
	idx := strings.IndexByte(rawPayload, '\n')
	if idx == -1 {
		t.Fatalf("Expected newline in payload")
	}

	lang := strings.TrimSpace(rawPayload[:idx])
	source := rawPayload[idx+1:]

	if lang != ".py" {
		t.Errorf("Expected language '.py', got '%s'", lang)
	}
	if source != "print('hello from dynamic Python')" {
		t.Errorf("Expected source 'print(\\'hello from dynamic Python\\')', got '%s'", source)
	}
}
