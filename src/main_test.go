package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type errConn struct {
	net.Conn
}

func (e *errConn) Read(b []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (e *errConn) Close() error {
	return nil
}

func TestExecTimeout(t *testing.T) {
	// Default
	os.Unsetenv("NLR_TIMEOUT_MS")
	if to := execTimeout(); to != defaultExecTimeout {
		t.Errorf("Expected default timeout %v, got %v", defaultExecTimeout, to)
	}

	// Valid override
	os.Setenv("NLR_TIMEOUT_MS", "4500")
	if to := execTimeout(); to != 4500*time.Millisecond {
		t.Errorf("Expected 4500ms, got %v", to)
	}

	// Invalid override (negative / NaN)
	os.Setenv("NLR_TIMEOUT_MS", "invalid")
	if to := execTimeout(); to != defaultExecTimeout {
		t.Errorf("Expected default timeout on invalid input, got %v", to)
	}
	os.Setenv("NLR_TIMEOUT_MS", "-100")
	if to := execTimeout(); to != defaultExecTimeout {
		t.Errorf("Expected default timeout on negative input, got %v", to)
	}
	os.Unsetenv("NLR_TIMEOUT_MS")
}

func TestRunCombinedOutput(t *testing.T) {
	ctx := context.Background()
	out, err := runCombinedOutput(ctx, "echo", "hello", "world")
	if err != nil {
		t.Fatalf("runCombinedOutput failed: %v", err)
	}
	if !strings.Contains(string(out), "hello world") {
		t.Errorf("Expected output to contain 'hello world', got %q", string(out))
	}

	// Test deadline exceeded
	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelTimeout()

	_, err = runCombinedOutput(timeoutCtx, "sleep", "2")
	if err == nil || !strings.Contains(err.Error(), "execution timed out") {
		t.Errorf("Expected execution timed out error, got %v", err)
	}

	// Test context canceled
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancelFunc()
	}()
	_, err = runCombinedOutput(cancelCtx, "sleep", "2")
	if err == nil || !strings.Contains(err.Error(), "execution cancelled") {
		t.Errorf("Expected execution cancelled error, got %v", err)
	}
}

func TestExecuteBuffer(t *testing.T) {
	ctx := context.Background()

	// Python
	pyOut, pyTmp, pyErr := executeBuffer(ctx, "print('test python')", ".py")
	if pyErr != nil {
		t.Logf("Python execution note: %v", pyErr)
	} else {
		if !strings.Contains(string(pyOut), "test python") {
			t.Errorf("Unexpected python output: %q", string(pyOut))
		}
	}
	if pyTmp != "" {
		t.Errorf("Expected empty tmpFile for python, got %q", pyTmp)
	}

	// Python syntax error
	_, _, pyErrSyntax := executeBuffer(ctx, "def bad_syntax(:", ".py")
	if pyErrSyntax == nil {
		t.Errorf("Expected error on invalid python syntax")
	}

	// Lua
	luaOut, luaTmp, luaErr := executeBuffer(ctx, "print('test lua')", ".lua")
	if luaErr != nil {
		t.Logf("Lua execution note: %v", luaErr)
	} else {
		if !strings.Contains(string(luaOut), "test lua") {
			t.Errorf("Unexpected lua output: %q", string(luaOut))
		}
	}
	if luaTmp != "" {
		t.Errorf("Expected empty tmpFile for lua, got %q", luaTmp)
	}

	// Lua syntax error
	_, _, luaErrSyntax := executeBuffer(ctx, "function bad(:", ".lua")
	if luaErrSyntax == nil {
		t.Errorf("Expected error on invalid lua syntax")
	}

	// Node.js
	jsOut, jsTmp, jsErr := executeBuffer(ctx, "console.log('test js')", ".js")
	if jsErr != nil {
		t.Logf("Node execution note: %v", jsErr)
	} else {
		if !strings.Contains(string(jsOut), "test js") {
			t.Errorf("Unexpected js output: %q", string(jsOut))
		}
	}
	if jsTmp != "" {
		t.Errorf("Expected empty tmpFile for js, got %q", jsTmp)
	}

	// Node.js syntax error
	_, _, jsErrSyntax := executeBuffer(ctx, "const x = ;", ".js")
	if jsErrSyntax == nil {
		t.Errorf("Expected error on invalid js syntax")
	}

	// Go
	goCode := `package main
import "fmt"
func main() {
	fmt.Println("test go")
}
`
	goOut, goTmp, goErr := executeBuffer(ctx, goCode, ".go")
	if goTmp != "" {
		os.Remove(goTmp)
	}
	if goErr != nil {
		t.Fatalf("executeBuffer(.go) failed: %v", goErr)
	}
	if !strings.Contains(string(goOut), "test go") {
		t.Errorf("Unexpected go output: %q", string(goOut))
	}

	// Go syntax error
	badGoCode := `package main
func main() { syntax error }`
	_, badGoTmp, badGoErr := executeBuffer(ctx, badGoCode, ".go")
	if badGoTmp != "" {
		os.Remove(badGoTmp)
	}
	if badGoErr == nil {
		t.Errorf("Expected error for invalid go code")
	}

	// Unsupported language
	_, _, unsupErr := executeBuffer(ctx, "puts 'hello'", ".rb")
	if unsupErr == nil {
		t.Errorf("Expected error for unsupported language")
	}
}

func TestHandleConnection(t *testing.T) {
	// 1. Connection with dynamic language header (Go code that generates output)
	serverConn, clientConn := net.Pipe()
	go func() {
		clientConn.Write([]byte(".go\npackage main\nimport \"fmt\"\nfunc main(){ fmt.Println(\"dynamic go\") }"))
		clientConn.Close()
	}()
	handleConnection(serverConn, ".py")

	// 2. Empty connection
	emptyServer, emptyClient := net.Pipe()
	emptyClient.Close()
	handleConnection(emptyServer, ".py")

	// 3. Error connection
	handleConnection(&errConn{}, ".py")

	// 4. Connection with error output execution
	serverConnErr, clientConnErr := net.Pipe()
	go func() {
		clientConnErr.Write([]byte(".py\ndef bad(:"))
		clientConnErr.Close()
	}()
	handleConnection(serverConnErr, ".py")

	// Wait for debouncer to process
	time.Sleep(350 * time.Millisecond)
}

func TestParseConfig(t *testing.T) {
	// Default values
	port, lang, err := parseConfig([]string{})
	if err != nil {
		t.Fatalf("Unexpected error with empty args: %v", err)
	}
	if port != 65432 || lang != "" {
		t.Errorf("Expected port 65432, lang '', got %d, %q", port, lang)
	}

	// Flags
	port, lang, err = parseConfig([]string{"-port", "12345", "-lang", ".py"})
	if err != nil {
		t.Fatalf("Unexpected error with flags: %v", err)
	}
	if port != 12345 || lang != ".py" {
		t.Errorf("Expected port 12345, lang '.py', got %d, %q", port, lang)
	}

	// Positional argument for language
	port, lang, err = parseConfig([]string{"-port", "54321", ".go"})
	if err != nil {
		t.Fatalf("Unexpected error with positional arg: %v", err)
	}
	if port != 54321 || lang != ".go" {
		t.Errorf("Expected port 54321, lang '.go', got %d, %q", port, lang)
	}

	// Error case
	_, _, err = parseConfig([]string{"-unknown-flag"})
	if err == nil {
		t.Errorf("Expected error for unknown flag, got nil")
	}
}

func TestRunServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	stopCh := make(chan struct{})
	doneCh := make(chan error, 1)

	go func() {
		doneCh <- runServer(ln, ".py", stopCh)
	}()

	// Connect a client and send a message
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}
	conn.Write([]byte("print('server test')"))
	conn.Close()

	// Give time for handleConnection
	time.Sleep(100 * time.Millisecond)

	// Stop server gracefully via stopCh
	close(stopCh)
	ln.Close()

	select {
	case sErr := <-doneCh:
		if sErr != nil {
			t.Errorf("runServer returned error: %v", sErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runServer did not terminate within timeout")
	}

	// Test runServer with immediate listener close (error return path)
	lnErr, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	lnErr.Close()
	err = runServer(lnErr, ".py", nil)
	if err == nil {
		t.Errorf("Expected error when running server on closed listener, got nil")
	}
}

func TestRunApp(t *testing.T) {
	// Find a free port
	tempLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind temporary port: %v", err)
	}
	port := tempLn.Addr().(*net.TCPAddr).Port
	tempLn.Close()

	stopCh := make(chan struct{})
	doneCh := make(chan error, 1)

	go func() {
		doneCh <- runApp([]string{"-port", strconv.Itoa(port), ".py"}, stopCh)
	}()

	// Allow server to start
	time.Sleep(100 * time.Millisecond)

	// Connect and send a payload
	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("Failed to dial runApp server: %v", err)
	}
	conn.Write([]byte("print('runApp test')"))
	conn.Close()

	time.Sleep(100 * time.Millisecond)
	close(stopCh)

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("runApp returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runApp did not exit in time")
	}

	// Test runApp with invalid arguments
	err = runApp([]string{"-invalid-flag"}, nil)
	if err == nil {
		t.Errorf("Expected error for invalid flags in runApp, got nil")
	}

	// Test runApp with port collision
	clashLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind clash port: %v", err)
	}
	defer clashLn.Close()
	clashPort := clashLn.Addr().(*net.TCPAddr).Port

	err = runApp([]string{"-port", strconv.Itoa(clashPort)}, nil)
	if err == nil {
		t.Errorf("Expected error when binding occupied port in runApp, got nil")
	}
}

func TestTCPStreamReading(t *testing.T) {
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
