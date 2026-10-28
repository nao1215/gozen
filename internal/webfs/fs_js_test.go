//go:build js

package webfs

import (
	"errors"
	"io/fs"
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
