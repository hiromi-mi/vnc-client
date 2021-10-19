package vncclient

import "github.com/veandco/go-sdl2/sdl"

type PullCh chan interface{}

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
