package vncclient

import (
	"log"
	"net"
	"unsafe"

	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
)

type KP struct {
	kind   byte
	keyint byte
	ph     uint16
	key    uint32
}

type Click struct {
	kind   byte
	button byte
	x      uint16
	y      uint16
}

type DesktopSize struct {
	Kind      uint8
	Padding   byte
	Width     uint16
	Height    uint16
	Screennum uint8
	Padding2  byte
}

type Screen struct {
	Id   uint32
	X    uint16
	Y    uint16
	W    uint16
	H    uint16
	Flag uint32
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

func mouseMotionEventDetail(conn net.Conn, ev *sdl.MouseMotionEvent) {
	WriteRequest(conn, Click{kind: 5, button: 0, x: uint16(ev.X), y: uint16(ev.Y)})
}

func WindowResizedEventDetail(conn net.Conn, ev *sdl.WindowEvent, winw, winh int32) {
	log.Println("Resize Event Begin!")
	//cursurface, err = window.GetSurface()
	WriteRequest(conn, DesktopSize{Kind: 251, Padding: 0, Width: uint16(winw), Height: uint16(winh), Screennum: 1, Padding2: 0})
	WriteRequest(conn, Screen{Id: 0, X: 0, Y: 0, W: uint16(winw), H: uint16(winh), Flag: 0})

	updater := NewUpdater(0, 0, int(winw), int(winh))
	WriteRequest(conn, updater)
}

func run(conn net.Conn, ch PullCh) {

	SetEncodings(conn)
	var window *sdl.Window

	winTitle := "VNC Client"
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
				mouseMotionEventDetail(conn, ev)
			case *sdl.WindowEvent:
				ev := event.(*sdl.WindowEvent)

				switch ev.Event {
				case sdl.WINDOWEVENT_RESIZED:
					winw, winh := window.GetSize()

					q.w = uint16(winw)
					q.h = uint16(winh)
					WindowResizedEventDetail(conn, ev, winw, winh)
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
