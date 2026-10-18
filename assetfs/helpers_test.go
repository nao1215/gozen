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

// defaultManifest は差し替えの道具が書く記録（report.json の file と source）。
var defaultManifest = Manifest{Name: "report.json", File: "file", Sum: "source"}

// useManifest は置き場の manifest で、重ねる絵が今の元の絵から作った物かを確かめるようにする。
func (o *OverlayFS) useManifest(m Manifest) *OverlayFS {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.man = m
	o.loaded, o.sums, o.hashes = false, nil, nil
	return o
}
