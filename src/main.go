package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bep/debounce"
)

var (
	debouncer  = debounce.New(time.Millisecond * 250)
	execMu     sync.Mutex
	cancelExec context.CancelFunc
)

const defaultExecTimeout = 2 * time.Second

func execTimeout() time.Duration {
	if v := os.Getenv("NLR_TIMEOUT_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultExecTimeout
}

func runCombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
		if cmd.Process != nil {
			if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil && pgid > 0 {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		}
		if ctx.Err() == context.DeadlineExceeded {
			return out, errors.New("execution timed out")
		}
		return out, errors.New("execution cancelled")
	}

	return out, err
}

func handleConnection(conn net.Conn, defaultLang string) {
	defer conn.Close()

	// Read all incoming payload data until EOF.
	// Neovim sends the entire code buffer over TCP and closes the client connection write-side.
	data, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		fmt.Println("Error reading from TCP connection:", err.Error())
		return
	}

	if len(data) == 0 {
		return
	}

	payload := string(data)
	lang := defaultLang
	source := payload

	// Extract dynamic language header if provided (format: .py\n<code>)
	if idx := strings.IndexByte(payload, '\n'); idx != -1 {
		firstLine := strings.TrimSpace(payload[:idx])
		if strings.HasPrefix(firstLine, ".") {
			lang = firstLine
			source = payload[idx+1:]
		}
	}

	// Schedule execution with debouncing.
	f := func() {
		fmt.Print("\033c")

		execMu.Lock()
		if cancelExec != nil {
			cancelExec()
		}
		ctx, cancel := context.WithTimeout(context.Background(), execTimeout())
		cancelExec = cancel
		execMu.Unlock()

		defer func() {
			execMu.Lock()
			cancel()
			cancelExec = nil
			execMu.Unlock()
		}()

		out, tmpFile, err := executeBuffer(ctx, source, lang)
		if tmpFile != "" {
			_ = os.Remove(tmpFile)
		}
		if err != nil {
			fmt.Print("ERROR: ", err.Error(), "\n")
		}
		if len(out) > 0 {
			fmt.Println(string(out))
		}
	}
	debouncer(f)
}

func parseConfig(args []string) (port int, lang string, err error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	portFlag := fs.Int("port", 65432, "TCP port for the server to listen on")
	langFlag := fs.String("lang", "", "Default file extension / language (e.g., .py, .go, .lua, .js)")

	if err := fs.Parse(args); err != nil {
		return 0, "", err
	}

	lang = *langFlag
	if lang == "" && fs.NArg() > 0 {
		lang = fs.Arg(0)
	}

	return *portFlag, lang, nil
}

func runServer(ln net.Listener, defaultLang string, stopCh <-chan struct{}) error {
	defer ln.Close()
	fmt.Printf("Listening on %s...\n", ln.Addr().String())

	if stopCh != nil {
		go func() {
			<-stopCh
			_ = ln.Close()
		}()
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if stopCh != nil {
				select {
				case <-stopCh:
					return nil
				default:
				}
			}
			log.Println("Error accepting TCP connection:", err)
			return err
		}

		go handleConnection(conn, defaultLang)
	}
}

func runApp(args []string, stopCh <-chan struct{}) error {
	port, lang, err := parseConfig(args)
	if err != nil {
		return fmt.Errorf("error parsing arguments: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("error listening on %s: %w", addr, err)
	}

	return runServer(ln, lang, stopCh)
}

func main() {
	if err := runApp(os.Args[1:], nil); err != nil {
		log.Fatal(err)
	}
}

func executeBuffer(ctx context.Context, source string, lang string) ([]byte, string, error) {
	switch lang {
	case ".py":
		out, err := runCombinedOutput(ctx, "python3", "-c", source)
		if err != nil {
			return nil, "", errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, "", nil
	case ".lua":
		out, err := runCombinedOutput(ctx, "lua", "-e", source)
		if err != nil {
			return nil, "", errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, "", nil
	case ".js":
		out, err := runCombinedOutput(ctx, "node", "-e", source)
		if err != nil {
			return nil, "", errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, "", nil
	case ".go":
		tmpFile, err := os.CreateTemp("", "temp-*.go")
		if err != nil {
			return nil, "", errors.New("Error creating temp file: " + err.Error())
		}
		defer tmpFile.Close()

		_, err = tmpFile.WriteString(source)
		if err != nil {
			return nil, tmpFile.Name(), errors.New("Error writing to temp file: " + err.Error())
		}
		out, err := runCombinedOutput(ctx, "go", "run", tmpFile.Name())
		if err != nil {
			return nil, tmpFile.Name(), errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, tmpFile.Name(), nil
	}
	return nil, "", errors.New("Unsupported language: " + lang)
}
