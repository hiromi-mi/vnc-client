package vncclient

import (
	"bufio"
	"bytes"
	"crypto/des"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/term"
)

// Swap DES key first bit <-> last bit
func invertbit(buf []byte) {
	l := len(buf)
	for i := 0; i < l; i++ {
		r := buf[i]
		r = (r&0xf0)>>4 | (r&0x0f)<<4
		r = (r&0xcc)>>2 | (r&0x33)<<2
		r = (r&0xaa)>>1 | (r&0x55)<<1
		buf[i] = r
	}
}

func Handshake(conn io.ReadWriteCloser) error {
	_, err := conn.Write([]byte("RFB 003.008\n"))
	if err != nil {
		log.Print(err)
		return err
	}

	buf := make([]byte, 100)

	conn.Read(buf)
	conn.Read(buf)

	_, err = conn.Write([]byte{0x13})
	if err != nil {
		log.Print(err)
		return err
	}
	conn.Read(buf)

	_, err = conn.Write([]byte{0, 2})
	if err != nil {
		log.Print(err)
		return err
	}
	conn.Read(buf)

	_, err = conn.Write([]byte{0, 0, 0, 2})
	if err != nil {
		log.Print(err)
		return err
	}

	i := make([]byte, 16)
	conn.Read(i)
	log.Printf("4.5: %v", i)

	fmt.Printf("Password: ")
	key, err := term.ReadPassword(int(syscall.Stdin))
	invertbit(key)

	block, err := des.NewCipher(key)
	if err != nil {
		log.Print(err)
		return err
	}
	// CFB Mode

	j := make([]byte, 16)
	block.Encrypt(j[0:8], i[0:8])
	log.Printf("5a: %v", j)
	block.Encrypt(j[8:16], i[8:16])
	log.Printf("5: %v", j)
	conn.Write(j)
	conn.Read(buf)
	log.Printf("6: %v", buf)
	if buf[3] != 0 {
		log.Print("Error: Authentification Failed.")
		log.Print(err)
		return err
	}

	serverinit := make([]byte, 50)
	conn.Write([]byte{0}) // clientinit
	conn.Read(serverinit)

	/*
		width := binary.BigEndian.Uint16(serverinit[0:2])
		height := binary.BigEndian.Uint16(serverinit[2:4])
	*/

	//log.Printf("serverinit: %v %v %v", serverinit, width, height)
	return err
}

func WriteRequest(conn net.Conn, u interface{}) {
	err := binary.Write(conn, binary.BigEndian, u)
	if err != nil {
		log.Print("WriteRequest: ", err)
	}
}

func SetEncodings(conn net.Conn) {
	// -27: JPEG
	// -224: LastRect
	// 7: Tight Encoding
	// 1: CopyRect
	encoding := []int32{-27, -308, -223, -224, 7, 1}

	var buf bytes.Buffer
	n, err := buf.Write([]byte{2, 0, 0, uint8(len(encoding))})
	if n != 4 || err != nil {
		log.Println("Error", err)
		return

	}
	// Anonymous Struct struct {len uint32; encoding hoge}{3, 5}
	err = binary.Write(&buf, binary.BigEndian, encoding)
	if err != nil {
		log.Println("Error", err)
		return
	}

	// Raw Encoding and CopyRect Encoding
	written, err := io.Copy(conn, &buf)
	if int(written) != 4+4*len(encoding) || err != nil {
		log.Println("Error", err)
		return
	}
	log.Println("Set up Encoding.")
}

func gradientFilter(gradient []byte, W int, H int) []byte {
	colorbuf := make([]byte, 3*W*H)
	for j := 0; j < H; j++ {
		for i := 0; i < W; i++ {
			// RGB
			for k := 0; k < 3; k++ {
				if i >= 1 && j >= 1 {
					p := int(colorbuf[(i-1)*j+k]) + int(colorbuf[i*(j-1)+k]) - int(colorbuf[(i-1)*(j-1)+k])
					if p < 0 {
						p = 0
					}
					if p > 255 {
						p = 255
					}
					colorbuf[i*j+k] = byte(p) + gradient[i*j+k]
				} else if i == 0 && j >= 1 {
					p := colorbuf[i*(j-1)+k]
					colorbuf[i*j+k] = p + gradient[i*j+k]
				} else if i >= 1 && j == 0 {
					p := colorbuf[(i-1)*j+k]
					colorbuf[i*j+k] = p + gradient[i*j+k]
				} else {
					// i = j = 0
					colorbuf[i*j+k] = gradient[i*j+k]
				}
			}
		}
	}
	return colorbuf
}

func conFrameUpdateDetail(conn *TCPWrapper, pullch PullCh) {
	var rectcnt struct{ Cnt uint16 }
	err := binary.Read(conn, binary.BigEndian, &rectcnt)
	if err != nil && err != io.EOF {
		log.Println("Error conFrameUpdatedetail", err)
		return
	}
	if err != nil {
		log.Print(err)
		return
	}

	init := true
	for i := 0; i < int(rectcnt.Cnt); i++ {
		info := new(struct {
			X        uint16
			Y        uint16
			W        uint16
			H        uint16
			DrawKind int32
		})
		err = binary.Read(conn, binary.BigEndian, info)
		if err != nil {
			log.Println("Error rectinfo: ", err)
			return
		}

		if info.W == 0 && info.H == 0 && info.X == 0 && info.Y == 0 {
			log.Println("LastRect")
			break
		}

		if info.X > 2048 {
			buf := make([]byte, 200)
			conn.Read(buf)
			log.Fatal("Error:  log", rectcnt, " ", buf, info)
		}

		rect := sdl.Rect{X: int32(info.X), Y: int32(info.Y), W: int32(info.W), H: int32(info.H)}

		log.Println("Let's draw: ", info)
		switch info.DrawKind {
		case -308:
			var pad struct {
				NumScreen uint8
				Pad1      uint8
				Pad2      uint8
				Pad3      uint8
			}
			var screen Screen

			err := binary.Read(conn, binary.BigEndian, &pad)
			if err != nil {
				log.Printf("error resize %v", err)
			}
			log.Print("Resize: ", pad)
			err = binary.Read(conn, binary.BigEndian, &screen)
			log.Print("Resize: ", screen)
		case 1:
			var reuserect struct {
				X uint16
				Y uint16
			}
			err := binary.Read(conn, binary.BigEndian, reuserect)
			if err != nil {
				log.Printf("error reuse %v", err)
			}
			reuseev := &PullReuseRect{
				Rect:      rect,
				ReuseRect: sdl.Rect{X: int32(reuserect.X), Y: int32(reuserect.Y), W: int32(info.W), H: int32(info.H)},
			}
			pullch <- reuseev
		case 0:
			buf := make([]byte, 4*int32(info.W)*int32(info.H))
			// TODO should use readatleast
			// TODO should usee as type* and CAPITAL LETTEr to io.read()
			n, err := io.ReadAtLeast(conn, buf, 4*int(info.W)*int(info.H))
			// n, err := conn.Read(buf)
			if n != 4*int(info.W)*int(info.H) || err != nil {
				log.Printf("error %d %v", n, err)
			}
			ev := &PullRaw{Rect: rect, Buf: buf}
			pullch <- ev
		case 7:
			compctrl := []byte{0}
			n, err := conn.Read(compctrl)
			if n != 1 || err != nil {
				log.Printf("error tight encoding type: %v", err)
			}

			// TODO zlib stream reset (?)
			if compctrl[0]&(0b11110000) == 0b10000000 {
				log.Print("FillCompression")
				var clr RGB
				err = binary.Read(conn, binary.BigEndian, &clr)
				if err != nil {
					log.Printf("error tight encoding color: %v %v", err, clr)
				}
				ev := &PullTightFill{
					Rect:  rect,
					Color: sdl.Color{R: clr.R, G: clr.G, B: clr.B, A: 255},
				}
				pullch <- ev
			} else if compctrl[0]&(0b11110000) == 0b10010000 {
				l := z.GetCompressedlen()
				buf := make([]byte, l)
				n, err = conn.Read(buf)
				if err != nil || l != n {
					log.Printf("error tight encoding jpeg len: %v %v", err, n)
				}
				ev := &PullTightJPEG{
					Rect: rect,
					Buf:  buf,
				}
				pullch <- ev
			} else {

				if compctrl[0]&(1<<6) > 0 {
					// may add filter-id
					// RGB 24bits
					filterid := []uint8{0}
					n, err := conn.Read(filterid)
					if n != 1 || err != nil {
						log.Printf("error tight encoding filterid: %v", err)
					}
					log.Print("BasicCompression: ", compctrl[0], " ", filterid)

					// 白黒なら1bit, そうでないなら8bit
					switch filterid[0] {
					case 1:
						log.Printf("Palette filter")

						// TODO buffer overflow palettecnt + 1 == 0
						palettecnt := []uint8{0}
						n, err := conn.Read(palettecnt)
						if n != 1 || err != nil {
							log.Printf("error tight encoding palettecnt: %v", err)
						}
						palettes := int(palettecnt[0]) + 1
						x := make([]RGB, palettes)
						err = binary.Read(conn, binary.BigEndian, x)
						if err != nil || len(x) < 1 {
							log.Print("Error read copied message: ", err, ". palettecnt: ", palettes)
							return
						}
						y := make([]sdl.Color, palettes)
						for i, c := range x {
							y[i] = sdl.Color{R: c.R, B: c.B, G: c.G, A: 255}
						}

						var buf []byte
						if palettecnt[0]+1 == 2 {
							// send 1 pixel per 1-bit

							log.Printf("palettecnt: %d %d %d", info.W-1, info.H, (info.W-1)/8+1)
							b, err := z.ReadBuf(int32((info.W-1)/8+1)*int32(info.H), compctrl, init)
							buf = make([]byte, int32(info.W)*int32(info.H))
							for i := 0; i < int(info.H); i++ {
								for j := 0; j < int(info.W); j++ {
									k := int(info.W-1)/8 + 1
									buf[i*int(info.W)+j] = (b[i*k+j/8] >> (7 - j%8)) & 1
								}
							}

							if err != nil {
								log.Print("Error color palette filter ", b, " extend -> ", buf, " 1-bit: ", err)
							}
						} else {
							//1-byte per 1-pixel
							buf, err = z.ReadBuf(1*int32(info.W)*int32(info.H), compctrl, init)
						}
						if err != nil {
							log.Print("Error color palette filter: ", err)
						}

						ev := &PullColorMap{
							Rect:     rect,
							ColorNum: len(x),
							ColorMap: y,
							Buf:      buf,
						}
						pullch <- ev
					case 2:
						log.Printf("Gradient filter")

						buf, _ := z.ReadBuf(3*int32(info.W)*int32(info.H), compctrl, init)
						bufafter := gradientFilter(buf, int(info.W), int(info.H))

						ev := &PullSmall{
							Rect: rect,
							Buf:  bufafter,
						}
						pullch <- ev

						//V[i,j] is the intensity of a color component
					case 0:
						buf, _ := z.ReadBuf(3*int32(info.H)*int32(info.W), compctrl, init)
						ev := &PullSmall{
							Rect: rect,
							Buf:  buf,
						}
						pullch <- ev
					default:
						log.Printf("error tight encoding funknown ilterid: %v", filterid)
					}

				} else {
					buf, _ := z.ReadBuf(3*int32(info.H)*int32(info.W), compctrl, init)

					ev := &PullSmall{
						Rect: rect,
						Buf:  buf,
					}
					pullch <- ev
				}
			}

		}
		init = false
	}
}

var z ZlibStreamer
var q struct {
	w uint16
	h uint16
}

type TCPWrapper struct {
	Buffer *bufio.Reader
	Conn   net.Conn
}

func (wrapper *TCPWrapper) Read(p []byte) (n int, err error) {
	a := len(p)
	n = 0
	n, err = io.ReadAtLeast(wrapper.Conn, p, a)
	for n < a {
		fmt.Print("hoge ")
		l, err := wrapper.Buffer.Read(p[n:])
		if err != nil && err != io.EOF {
			return n, err
		}
		n += int(l)
	}
	if n != len(p) {
		fmt.Printf("Error loading read: %d, %d", n, len(p))
	}
	// return wrapper.Conn.Read(p)
	return n, err
}

func (wrapper *TCPWrapper) Close() error {
	return wrapper.Conn.Close()
}

func Con(conn net.Conn, bytesbuf *TCPWrapper, pullch PullCh) {
	defer close(pullch)

	z = ZlibStreamer{conn: bytesbuf}
	updatetime := time.Now()
	// first update
	updater := NewUpdater(0, 0, int(q.w), int(q.h))
	WriteRequest(conn, updater)

	for {

		var msgkind struct {
			Kind  uint8
			Dummy uint8
		}
		err := binary.Read(bytesbuf, binary.BigEndian, &msgkind)
		if err == io.EOF {
			sdl.Delay(50)
			continue
		}
		if err != nil {
			log.Print("Error con: ", err)
			break
		}

		switch msgkind.Kind {
		case 150:
			log.Println("Continuous Upgrade Supported!")
		case 1:
			log.Println("Not Implemented to set up new color map")
		case 2:
			log.Println("Bell")
		case 3:
			var x struct {
				Pad    uint16
				Length uint32
			}
			err := binary.Read(bytesbuf, binary.BigEndian, &x)
			if err != nil {
				log.Print("Error copy message: ", err)
			}
			y := make([]byte, x.Length)
			n, err := bytesbuf.Read(y)
			if n != int(x.Length) || err != nil {
				log.Print("Error read copied message: ", err)
				return
			}
			// TODO Support UTF-8 character
			log.Println("string copied: ", string(y))
			sdl.SetClipboardText(string(y))
		case 0:
			conFrameUpdateDetail(bytesbuf, pullch)
		default:
			log.Println("Unknown Event: ", msgkind)
		}

		curtime := time.Now()
		updater := NewUpdater(0, 0, int(q.w), int(q.h))
		log.Println("update time: ", curtime.Sub(updatetime))
		updatetime = curtime

		WriteRequest(conn, updater)
	}
}

type Updater struct {
	kind        byte
	incremental byte
	x           uint16
	y           uint16
	w           uint16
	h           uint16
}

func NewUpdater(x int, y int, w int, h int) Updater {
	u := Updater{
		kind:        3,
		incremental: 1,
		x:           uint16(x),
		y:           uint16(y),
		w:           uint16(w),
		h:           uint16(h),
	}
	return u
}
