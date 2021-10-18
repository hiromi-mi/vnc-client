package vncclient

import (
	"io"
	"log"

	"github.com/4kills/go-zlib"
)

type ZlibStreamer struct {
	ZlibReaders [4]*zlib.Reader
	conn        io.Reader
}

func (s *ZlibStreamer) resetzlib(compctrl byte) {
	for i := 0; i < 4; i++ {
		// NO ZLIb header!  TODO

		if compctrl&(1<<i) != 0 {
			s.ZlibReaders[i].Reset(s.conn, nil)
		}
	}
}

func (s *ZlibStreamer) GetCompressedlen() int {
	pad := []uint8{0}
	s.conn.Read(pad)
	compressedlen := int(pad[0] & 0b01111111)
	if pad[0]&(1<<7) > 0 {
		s.conn.Read(pad)

		compressedlen += int(pad[0]&0b01111111) * (1 << 7)
		if pad[0]&(1<<7) > 0 {
			s.conn.Read(pad)

			compressedlen += int(pad[0]&0b01111111) * (1 << 14)
		}
	}

	return compressedlen
}

func (s *ZlibStreamer) ReadBuf(clen int32, compctrl []byte, init bool) ([]byte, error) {
	ctrl := compctrl[0]
	if clen < 12 {
		b := make([]byte, clen)
		_, err := s.conn.Read(b)
		return b, err
	}
	s.resetzlib(ctrl)

	compressedlen := s.GetCompressedlen()
	//var r io.ReadCloser
	var err error
	i := (compctrl[0] & 0b110000) >> 4
	if s.ZlibReaders[i] == nil {
		s.ZlibReaders[i], err = zlib.NewReader(s.conn)

	}
	if err != nil {
		log.Printf("Error zlib creation %v", err)
	}
	c := make([]byte, compressedlen)
	b2 := make([]byte, clen)

	s.conn.Read(c)
	_, _, err = s.ZlibReaders[i].ReadBuffer(c, b2)

	return b2, err
}
