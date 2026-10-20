package assetfs

import (
	"io/fs"
	"testing"
)

func overlay(t *testing.T, base fs.FS, prefix string, top fs.FS) *OverlayFS {
	t.Helper()
	o, err := New(base, top, Config{Prefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// defaultManifest is the record (file and source in report.json) written by the replacement tool.
var defaultManifest = Manifest{Name: "report.json", File: "file", Sum: "source"}

// useManifest uses the location's manifest so as to check whether the overlay picture was made
// from the current source picture.
func (o *OverlayFS) useManifest(m Manifest) *OverlayFS {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.man = m
	o.loaded, o.sums, o.hashes = false, nil, nil
	return o
}
