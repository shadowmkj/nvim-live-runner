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

func main() {
	portFlag := flag.Int("port", 65432, "TCP port for the server to listen on")
	langFlag := flag.String("lang", "", "Default file extension / language")
	flag.Parse()

	lang := *langFlag
	if lang == "" && flag.NArg() > 0 {
		lang = flag.Arg(0)
	}

	addr := fmt.Sprintf(":%d", *portFlag)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Error listening on %s: %v", addr, err)
	}
	defer ln.Close()

	fmt.Printf("Listening on %s...\n", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Error accepting TCP connection:", err)
			continue
		}

		go handleConnection(conn, lang)
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
