package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/bep/debounce"
)

var debouncer = debounce.New(time.Millisecond * 250)

func handleConnection(conn net.Conn, lang string) {
	buffer := make([]byte, 2056)
	for {
		len, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				conn.Close()
				return
			}
			fmt.Println("Error reading" + err.Error())
			continue
		}

		if len == 0 {
			fmt.Println("Closing connection")
			conn.Close()
		}

		if len > 0 {
			f := func() {
				source := string(buffer[0:len])
				fmt.Print("\033c")
				out, tmpFile, err := executeBuffer(source, lang)
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

func executeBuffer(source string, lang string) ([]byte, string, error) {
	switch lang {
	case ".py":
		cmd := exec.Command("python3", "-c", source)
		out, err := cmd.Output()
		if err != nil {
			return nil, "", errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, "", nil
	case ".lua":
		cmd := exec.Command("lua", "-e", source)
		out, err := cmd.Output()
		if err != nil {
			return nil, "", errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, "", nil
	case ".js":
		cmd := exec.Command("node", "-e", source)
		out, err := cmd.Output()
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
		out, err := exec.Command("go", "run", tmpFile.Name()).CombinedOutput()
		if err != nil {
			return nil, tmpFile.Name(), errors.New("Error executing command: " + err.Error() + "\n" + string(out))
		}
		return out, tmpFile.Name(), nil
	}
	return nil, "", errors.New("Unsupported language: " + lang)
}
