//go:build js

package picdecode

import (
	"errors"
	"fmt"
	"image"
	"sync/atomic"
	"syscall/js"
)

// In a browser the game and everything it runs share one thread: a picture decoded in Go
// (a WebP with alpha) holds it for tens of milliseconds on a desktop and several times
// that on a phone, so the screen stands still and the music, short of its buffer,
// crackles whenever new pictures come in. The browser decodes pictures on threads of its
// own (createImageBitmap), and while it does the game goes on; only reading the pixels
// back takes the game's thread. Where that is missing (Node, an old browser), Go decodes
// as on the desktop.

// Decode decodes the picture b, by the browser when it can. Only WebP and JPEG go to the
// browser: a PNG left as PNG by the web build is one WebP did not suit, such as a 16-bit
// depth map, which a canvas would cut to 8 bits and Go reads as an *image.Gray16.
func Decode(b []byte) (image.Image, error) {
	if browserKind(b) && browserNow() {
		if img, err := browserDecode(b); err == nil {
			return img, nil
		}
	}
	return goDecode(b)
}

// after is closed once the browser may decode (BrowserAfter).
var after atomic.Pointer[<-chan struct{}]

// BrowserAfter lets the browser decode only once ready is closed: until then Go decodes.
// The first frame waits for its pictures, and at that time the music and the sound
// effects are being made in the background and keep the page's thread so busy that the
// browser's decoder answers late, so the screen hands it the pictures after its first
// frame (play.New).
func BrowserAfter(ready <-chan struct{}) { after.Store(&ready) }

func browserNow() bool {
	ready := after.Load()
	if ready == nil {
		return true
	}
	select {
	case <-*ready:
		return true
	default:
		return false
	}
}

// errNoBrowserDecoder is returned where the page cannot decode pictures for Go.
var errNoBrowserDecoder = errors.New("no createImageBitmap or canvas")

// scratchCanvas is the canvas the decoded pictures are drawn on to read their pixels.
var scratchCanvas, scratchCtx js.Value

// await waits for the promise p to settle (other goroutines run meanwhile).
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{v: args[0]}
		return nil
	})
	fail := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: fmt.Errorf("%s", args[0].Call("toString").String())}
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	p.Call("then", ok, fail)
	r := <-ch
	return r.v, r.err
}

// browserDecode decodes raw with the browser's decoders. The canvas gives straight alpha,
// so the picture is an *image.NRGBA: what the puppets keep, and what Ebitengine and the
// dolls read without converting it pixel by pixel.
func browserDecode(raw []byte) (img image.Image, err error) {
	defer func() {
		if r := recover(); r != nil { // a call that throws (syscall/js panics)
			img, err = nil, fmt.Errorf("decoding in the browser: %v", r)
		}
	}()
	global := js.Global()
	if global.Get("createImageBitmap").Type() != js.TypeFunction {
		return nil, errNoBrowserDecoder
	}
	if scratchCtx.IsUndefined() {
		switch {
		case global.Get("OffscreenCanvas").Type() == js.TypeFunction:
			scratchCanvas = global.Get("OffscreenCanvas").New(1, 1)
		case global.Get("document").Type() == js.TypeObject:
			scratchCanvas = global.Get("document").Call("createElement", "canvas")
		default:
			return nil, errNoBrowserDecoder
		}
		scratchCtx = scratchCanvas.Call("getContext", "2d", map[string]any{"willReadFrequently": true})
		if scratchCtx.IsNull() || scratchCtx.IsUndefined() {
			scratchCtx = js.Undefined()
			return nil, errNoBrowserDecoder
		}
	}
	u8 := global.Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(u8, raw)
	blob := global.Get("Blob").New([]any{u8})
	bmp, err := await(global.Call("createImageBitmap", blob, map[string]any{
		"premultiplyAlpha": "none", "colorSpaceConversion": "none",
	}))
	if err != nil {
		return nil, err
	}
	defer bmp.Call("close")
	w, h := bmp.Get("width").Int(), bmp.Get("height").Int()
	// Nothing yields between drawing and reading back (the goroutines of a WebAssembly
	// program switch only where one waits), so one canvas serves every decode.
	scratchCanvas.Set("width", w)
	scratchCanvas.Set("height", h)
	scratchCtx.Call("clearRect", 0, 0, w, h)
	scratchCtx.Call("drawImage", bmp, 0, 0)
	data := scratchCtx.Call("getImageData", 0, 0, w, h).Get("data")
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	js.CopyBytesToGo(dst.Pix, global.Get("Uint8Array").New(data.Get("buffer"), data.Get("byteOffset"), data.Get("byteLength")))
	scratchCanvas.Set("width", 1) // let the canvas's memory go
	scratchCanvas.Set("height", 1)
	return dst, nil
}
