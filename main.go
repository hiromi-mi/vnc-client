package vncclient

import (
	"bufio"
	"log"
	"net"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Ldate | log.Ltime)

	conn, err := net.Dial("tcp", "localhost:5900")
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close()

	pull := make(PullCh, 1000)

	buffer := &TCPWrapper{
		Buffer: bufio.NewReaderSize(conn, 1024*768),
		Conn:   conn,
	}
	go con(conn, buffer, pull)

	run(conn, pull)

	return
}
