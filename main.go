package main

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
	"unsafe"

	"github.com/4kills/go-zlib"
	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/term"
)

type EventCh chan interface{}
type PullCh chan interface{}

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

type PullReuseRect struct {
	Rect      sdl.Rect
	ReuseRect sdl.Rect
}

type PullRaw struct {
	Rect sdl.Rect
	Buf  []byte
}

type PullTightFill struct {
	Rect  sdl.Rect
	Color sdl.Color
}

type PullTightJPEG struct {
	Rect sdl.Rect
	Buf  []byte
}

type PullSmall struct {
	Rect sdl.Rect
	Buf  []byte
}

type PullColorMap struct {
	Rect     sdl.Rect
	ColorNum int
	ColorMap []sdl.Color
	Buf      []byte
}

type RGB struct {
	R, G, B uint8
}

type ZlibStreamer struct {
	ZlibReaders [4]*zlib.Reader
	//conn        net.Conn
	conn *TCPWrapper
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

func (s *ZlibStreamer) resetzlib(compctrl byte) {
	for i := 0; i < 4; i++ {
		// NO ZLIb header!  TODO

		if compctrl&(1<<i) != 0 {
			s.ZlibReaders[i].Reset(s.conn, nil)
		}
	}
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
			var screen struct {
				Id   uint32
				X    uint16
				Y    uint16
				W    uint16
				H    uint16
				Flag uint32
			}

			err := binary.Read(conn, binary.BigEndian, &pad)
			if err != nil {
				log.Printf("error resize %v", err)
			}
			log.Print("Resize: ", pad)
			err = binary.Read(conn, binary.BigEndian, &screen)
			log.Print("Resize: ", screen)
		case -223:
			var pad struct {
				NumScreen uint8
				Pad1      uint8
				Pad2      uint8
				Pad3      uint8
				Id        uint32
				X         uint32
				Y         uint32
				W         uint32
				H         uint32
				Flag      uint32
			}

			err := binary.Read(conn, binary.BigEndian, pad)
			if err != nil {
				log.Printf("error resize %v", err)
			}
			log.Print("Resize: ", pad)
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
					// TODO まちがえて compctrl にして意味不明になってた
					n, err := conn.Read(filterid)
					if n != 1 || err != nil {
						log.Printf("error tight encoding filterid: %v", err)
					}
					log.Print("BasicCompression: ", compctrl[0], " ", filterid)

					// 白黒なら1bit, そうでないなら8bitで送ってくる
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
						bufafter := make([]byte, 3*int(info.W)*int(info.H))

						buf, _ := z.ReadBuf(3*int32(info.W)*int32(info.H), compctrl, init)

						for j := 0; j < int(info.H); j++ {
							for i := 0; i < int(info.W); i++ {
								for k := 0; k < 3; k++ {
									if i >= 1 && j >= 1 {
										p := int(bufafter[(i-1)*j+k]) + int(bufafter[i*(j-1)+k]) - int(bufafter[(i-1)*(j-1)+k])
										if p < 0 {
											p = 0
										}
										if p > 255 {
											p = 255
										}
										bufafter[i*j+k] = byte(p) + buf[i*j+k]
									} else if i == 0 && j >= 1 {
										p := bufafter[i*(j-1)+k]
										bufafter[i*j+k] = p + buf[i*j+k]
									} else if i >= 1 && j == 0 {
										p := bufafter[(i-1)*j+k]
										bufafter[i*j+k] = p + buf[i*j+k]
									} else {
										// i = j = 0
										bufafter[i*j+k] = buf[i*j+k]
									}
								}

							}
						}
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

func con(conn net.Conn, bytesbuf *TCPWrapper, pullch PullCh) {
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
			// TODO Japanese?
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

func keyEventDetail(conn net.Conn, ev *sdl.KeyboardEvent) {
	if ev.Keysym.Scancode == sdl.SCANCODE_RETURN {
		WriteRequest(conn, KeyPress(ev.Type == sdl.KEYDOWN, 0xff0d))
	} else if sdl.K_SPACE <= ev.Keysym.Sym && ev.Keysym.Sym <= sdl.K_AT {
		WriteRequest(conn, KeyPress(ev.Type == sdl.KEYDOWN, uint32(ev.Keysym.Sym)))
	} else if sdl.K_a <= ev.Keysym.Sym && ev.Keysym.Sym <= sdl.K_z {
		if ev.Keysym.Mod&sdl.KMOD_SHIFT == 0 {
			WriteRequest(conn, KeyPress(ev.Type == sdl.KEYDOWN, uint32(ev.Keysym.Sym)))
		} else {
			// capital letter
			WriteRequest(conn, KeyPress(ev.Type == sdl.KEYDOWN, uint32(ev.Keysym.Sym)-0x20))
		}
	} else if ev.Keysym.Sym == sdl.K_BACKSPACE {
		WriteRequest(conn, KeyPress(ev.Type == sdl.KEYDOWN, 0xff08))
	}
}

func mouseEventDetail(conn net.Conn, ev *sdl.MouseButtonEvent) {
	button := ev.State
	// Depend on LEFT == 1, RIGHT == 3
	button = button << (ev.Button - 1)
	// Depend on ev.State (Released == 0, Pressed == 1)
	WriteRequest(conn, Click{kind: 5, button: button, x: uint16(ev.X), y: uint16(ev.Y)})
}

func mouseMotionDetail(conn net.Conn, ev *sdl.MouseMotionEvent) {
	WriteRequest(conn, Click{kind: 5, button: 0, x: uint16(ev.X), y: uint16(ev.Y)})
}

type DesktopSize struct {
	Kind    uint8
	Padding byte
	Width   uint16
	Height  uint16
	// TODO Support Multidisplay
	Screennum uint8
	Padding2  byte
	Id        uint32
	X         uint16
	Y         uint16
	W         uint16
	H         uint16
	Flag      uint32
}

func run(conn net.Conn, ch PullCh) {

	SetEncodings(conn)
	var window *sdl.Window

	winTitle := "VNC Client"
	//window, err := sdl.CreateWindow(winTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, 1024, 768, sdl.WINDOW_MAXIMIZED|sdl.WINDOW_RESIZABLE|sdl.WINDOW_SHOWN)
	window, err := sdl.CreateWindow(winTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, 1024, 768, sdl.WINDOW_RESIZABLE|sdl.WINDOW_SHOWN)

	if err != nil {
		log.Print(err)
		return
	}
	defer window.Destroy()

	running := true

	//cursurface, err := window.GetSurface()
	if err != nil {
		log.Print(err)
		running = false
	}

	for running {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch event.(type) {
			case *sdl.QuitEvent:
				running = false
			case *sdl.KeyboardEvent:
				ev := event.(*sdl.KeyboardEvent)
				keyEventDetail(conn, ev)
			case *sdl.MouseButtonEvent:
				ev := event.(*sdl.MouseButtonEvent)
				log.Printf("Mouse Click: %+v\n", ev)
				mouseEventDetail(conn, ev)
			case *sdl.MouseMotionEvent:
				ev := event.(*sdl.MouseMotionEvent)
				mouseMotionDetail(conn, ev)
			case *sdl.WindowEvent:
				ev := event.(*sdl.WindowEvent)

				if ev.Event == sdl.WINDOWEVENT_RESIZED {
					log.Println("Resize Event Begin!")
					//cursurface, err = window.GetSurface()
					winw, winh := window.GetSize()

					q.w = uint16(winw)
					q.h = uint16(winh)
					WriteRequest(conn, DesktopSize{Kind: 251, Padding: 0, Width: uint16(winw), Height: uint16(winh), Screennum: 1, Padding2: 0, Id: 0, X: 0, Y: 0, W: uint16(winw), H: uint16(winh), Flag: 0})

					updater := NewUpdater(0, 0, int(q.w), int(q.h))
					WriteRequest(conn, updater)
				}

			}
		}

		cursurface, err := window.GetSurface()
		for len(ch) > 0 {
			x, ok := <-ch
			if !ok {
				running = false
				break
			}
			switch x.(type) {
			case *PullRaw:
				ev := x.(*PullRaw)

				// not RGB, but GBR (redShift, grennShift)
				buf2 := ev.Buf
				surface, err := sdl.CreateRGBSurfaceWithFormatFrom(unsafe.Pointer(&buf2[0]), ev.Rect.W, ev.Rect.H, 24, ev.Rect.W*4, uint32(sdl.PIXELFORMAT_BGRA32))
				if err != nil {
					log.Print(err)
					running = false
				}
				err = surface.BlitScaled(nil, cursurface, &ev.Rect)
				if err != nil {
					log.Print(err)
					running = false
				}
				log.Println("draw complete: ", ev.Rect)
			case *PullReuseRect:
				ev := x.(*PullReuseRect)
				err = cursurface.BlitScaled(&ev.ReuseRect, cursurface, &ev.Rect)
				if err != nil {
					log.Print(err)
					running = false
				}
			case *PullTightFill:
				ev := x.(*PullTightFill)
				err = cursurface.FillRect(&ev.Rect, sdl.MapRGB(cursurface.Format, ev.Color.R, ev.Color.G, ev.Color.B))
				if err != nil {
					log.Print(err)
					running = false
				}
				log.Println("draw complete (TightFill w/ ", ev.Color, "): ", ev.Rect)
			case *PullTightJPEG:
				ev := x.(*PullTightJPEG)
				rwops, err := sdl.RWFromMem(ev.Buf)
				if err != nil {
					log.Print(err)
					running = false
				}
				surface, err := img.LoadJPGRW(rwops)
				if err != nil {
					log.Print(err)
					running = false
				}
				err = surface.BlitScaled(nil, cursurface, &ev.Rect)
				if err != nil {
					log.Print(err)
					running = false
				}
				log.Println("draw complete (PullTightJPEG): ", ev.Rect)

			case *PullSmall:
				ev := x.(*PullSmall)

				surface, err := sdl.CreateRGBSurfaceWithFormatFrom(unsafe.Pointer(&ev.Buf[0]), ev.Rect.W, ev.Rect.H, 24, ev.Rect.W*3, uint32(sdl.PIXELFORMAT_RGB24))
				if err != nil {
					log.Print(err)
					running = false
				}
				err = surface.BlitScaled(nil, cursurface, &ev.Rect)
				if err != nil {
					log.Print(err)
					running = false
				}
				log.Println("time to draw complete (PullSmall): ", ev.Rect)
			case *PullColorMap:
				ev := x.(*PullColorMap)
				p, err := sdl.AllocPalette(ev.ColorNum)
				if err != nil {
					log.Print(err)
					running = false
				}
				p.SetColors(ev.ColorMap)

				surface, err := sdl.CreateRGBSurfaceWithFormatFrom(unsafe.Pointer(&ev.Buf[0]), ev.Rect.W, ev.Rect.H, 24, ev.Rect.W, uint32(sdl.PIXELFORMAT_INDEX8))
				surface.SetPalette(p)
				if err != nil {
					log.Print(err)
					running = false
				}
				err = surface.BlitScaled(nil, cursurface, &ev.Rect)
				if err != nil {
					log.Print(err)
					running = false
				}
				log.Println("draw complete (PullSmall): ", ev.Rect)
			}
		}

		err = window.UpdateSurface()
		if err != nil {
			log.Print(err)
			running = false
		}

	}

	return
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Ldate | log.Ltime)

	conn, err := net.Dial("tcp", "localhost:5900")
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close()

	_, err = conn.Write([]byte("RFB 003.008\n"))
	if err != nil {
		log.Print(err)
		return
	}

	buf := make([]byte, 100)

	conn.Read(buf)
	conn.Read(buf)

	_, err = conn.Write([]byte{0x13})
	if err != nil {
		log.Print(err)
		return
	}
	conn.Read(buf)

	_, err = conn.Write([]byte{0, 2})
	if err != nil {
		log.Print(err)
		return
	}
	conn.Read(buf)

	_, err = conn.Write([]byte{0, 0, 0, 2})
	if err != nil {
		log.Print(err)
		return
	}

	i := make([]byte, 16)
	conn.Read(i)
	log.Printf("4.5: %v", i)

	// Swap DES key first bit <-> last bit
	fmt.Printf("Password: ")
	key, err := term.ReadPassword(int(syscall.Stdin))
	newkey := make([]byte, 8)
	for i := 0; i < 8; i++ {
		r := key[i]
		r = (r&0xf0)>>4 | (r&0x0f)<<4
		r = (r&0xcc)>>2 | (r&0x33)<<2
		r = (r&0xaa)>>1 | (r&0x55)<<1
		newkey[i] = r
	}

	block, err := des.NewCipher(newkey)
	if err != nil {
		log.Print(err)
		return
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
		return
	}

	serverinit := make([]byte, 50)
	conn.Write([]byte{0}) // clientinit
	conn.Read(serverinit)

	/*
		width := binary.BigEndian.Uint16(serverinit[0:2])
		height := binary.BigEndian.Uint16(serverinit[2:4])
	*/

	//log.Printf("serverinit: %v %v %v", serverinit, width, height)

	pull := make(PullCh, 1000)

	buffer := &TCPWrapper{
		Buffer: bufio.NewReaderSize(conn, 1024*768),
		Conn:   conn,
	}
	go con(conn, buffer, pull)

	run(conn, pull)

	return
}

type Click struct {
	kind   byte
	button byte
	x      uint16
	y      uint16
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

type KP struct {
	kind   byte
	keyint byte
	ph     uint16
	key    uint32
}

func KeyPress(keydown bool, key uint32) KP {
	var keyint byte
	if keydown {
		keyint = 1
	} else {
		keyint = 0
	}
	kp := KP{kind: 4, keyint: keyint, ph: 0, key: key}
	return kp
}
