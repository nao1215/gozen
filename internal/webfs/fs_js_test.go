//go:build js

package webfs

import (
	"errors"
	"io/fs"
	"syscall/js"
	"testing"
	"testing/fstest"
)

func TestStatDoesNotFetch(t *testing.T) {
	// No fetch queues or cache: Open of the listed voice would try network I/O.
	f := &httpFS{
		entries: fstest.MapFS{"assets/voice/host/line.ogg": &fstest.MapFile{}},
		served:  map[string]file{"assets/voice/host/line.ogg": {Later: true}},
	}
	if st, err := fs.Stat(f, "assets/voice/host/line.ogg"); err != nil || st.IsDir() {
		t.Fatalf("listed voice: %v, %v", st, err)
	}
	if _, err := fs.Stat(f, "assets/voice/host/missing.ogg"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent voice: %v", err)
	}
	if st, err := fs.Stat(f, "assets/voice/host"); err != nil || !st.IsDir() {
		t.Fatalf("manifest directory: %v, %v", st, err)
	}
}

// A picture the game opens before Load has come to it is fetched at once, not after the
// media queued before it; Load then passes over it, and whoever waited for it is woken.
func TestOpenFetchesAQueuedPictureAtOnce(t *testing.T) {
	var asked []string
	fake := js.FuncOf(func(_ js.Value, args []js.Value) any {
		asked = append(asked, args[0].String())
		buf := js.Global().Get("Uint8Array").New(3)
		js.CopyBytesToJS(buf, []byte{1, 2, 3})
		resp := js.Global().Get("Object").New()
		resp.Set("ok", true)
		read := js.FuncOf(func(js.Value, []js.Value) any {
			return js.Global().Get("Promise").Call("resolve", buf.Get("buffer"))
		})
		resp.Set("arrayBuffer", read)
		return js.Global().Get("Promise").Call("resolve", resp)
	})
	defer fake.Release()
	old := js.Global().Get("fetch")
	js.Global().Set("fetch", fake)
	defer js.Global().Set("fetch", old)
	name := "assets/portraits/a.png"
	coming := make(chan struct{})
	f := &httpFS{
		base: "game/", entries: fstest.MapFS{name: &fstest.MapFile{}}, served: map[string]file{name: {Name: name, File: name + ".webp", Size: 3}},
		cache: map[string][]byte{}, coming: map[string]chan struct{}{name: coming}, fetching: map[string]*fetchCall{},
		fetchers: make(chan struct{}, workers), started: map[string]bool{}, taken: map[string]bool{},
	}
	b, err := fs.ReadFile(f, name)
	if err != nil || len(b) != 3 {
		t.Fatalf("read %v, %v", b, err)
	}
	if len(asked) != 1 || asked[0] != "game/"+name+".webp" || !f.taken[name] {
		t.Fatalf("fetched %v, taken %v", asked, f.taken[name])
	}
	select {
	case <-coming:
	default:
		t.Fatal("those waiting for the picture were not woken")
	}
	if _, err := fs.ReadFile(f, name); err != nil || len(asked) != 1 {
		t.Fatalf("a second read fetched again: %v, %v", asked, err)
	}
}
