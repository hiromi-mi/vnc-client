package main

import (
	"bufio"
	"log"
	"net"

	vncclient "github.com/hiromi-mi/vnc-client/src"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Ldate | log.Ltime)

	conn, err := net.Dial("tcp", "localhost:5900")
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close()

	err = vncclient.Handshake(conn)
	if err != nil {
		log.Print(err)
		return
	}
	pull := make(vncclient.PullCh, 1000)

	buffer := &vncclient.TCPWrapper{
		Buffer: bufio.NewReaderSize(conn, 1024*768),
		Conn:   conn,
	}
	go vncclient.Con(conn, buffer, pull)

	vncclient.Do(conn, pull)

	return
}
