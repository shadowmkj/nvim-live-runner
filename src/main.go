package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/bep/debounce"
)

var debouncer = debounce.New(time.Millisecond * 250)

const defaultExecTimeout = 2 * time.Second

func execTimeout() time.Duration {
	// Optional override for long-running snippets.
	// Example: NLR_TIMEOUT_MS=5000
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
	// Ensure we can kill any subprocesses the interpreter/compiler spawns.
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

func handleConnection(conn net.Conn, lang string) {
	buffer := make([]byte, 2056)
	var execMu sync.Mutex
	var cancelExec context.CancelFunc

	cancelRunning := func() {
		execMu.Lock()
		defer execMu.Unlock()
		if cancelExec != nil {
			cancelExec()
		}
	}
	defer cancelRunning()

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				cancelRunning()
				conn.Close()
				return
			}
			fmt.Println("Error reading" + err.Error())
			continue
		}

		if n == 0 {
			fmt.Println("Closing connection")
			cancelRunning()
			conn.Close()
			return
		}

		if n > 0 {
			// Copy the read bytes since the debounced function may run after the next Read.
			b := make([]byte, n)
			copy(b, buffer[:n])
			source := string(b)

			f := func() {
				fmt.Print("\033c")

				execMu.Lock()
				if cancelExec != nil {
					cancelExec()
				}
				ctx, cancel := context.WithTimeout(context.Background(), execTimeout())
				cancelExec = cancel
				execMu.Unlock()
				defer cancel()

				out, tmpFile, err := executeBuffer(ctx, source, lang)
				if tmpFile != "" {
					os.Remove(tmpFile)
				}
				if err != nil {
					print("ERROR: ", err.Error())
				}
				println(string(out))
			}
			debouncer(f)
		}
	}
}

func main() {
	lang := os.Args[1]
	ln, err := net.Listen("tcp", ":65432")
	if err != nil {
		log.Fatal("Error creating socket")
		return
	}
	fmt.Println("Listening..")

	for {
		stream, err := ln.Accept()
		if err != nil {
			log.Fatal("Error creating socket")
			continue
		}

		go handleConnection(stream, lang)
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
