package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"time"

	"github.com/bep/debounce"
)

var debouncer = debounce.New(time.Millisecond * 250)

func handleConnection(conn net.Conn) {
	buffer := make([]byte, 2056)
	for {
		len, err := conn.Read(buffer)
		if err != nil {

			if err == io.EOF {
				// fmt.Println("Closing connection")
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
				cmd := exec.Command("python3", "-c", source)
				out, err := cmd.Output()
				if err != nil {
					print("ERR: ", err.Error())
				}
				println(string(out))
			}
			debouncer(f)
		}
	}
}

func main() {
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

		go handleConnection(stream)
	}

}
