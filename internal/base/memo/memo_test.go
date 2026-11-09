package memo

import (
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecordRoundTripAndOptOut(t *testing.T) {
	root := t.TempDir()
	m := &Memo{Source: Human, Kind: "note", Text: "The gate opens.", Time: "2026-01-02T03:04:05Z"}
	p, err := Write(root, m, image.NewNRGBA(image.Rect(0, 0, 4, 4)), []byte("saved-state"))
	if err != nil {
		t.Fatal(err)
	}
	got, b, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != m.Text || string(b) != "saved-state" || got.Version != "" || Version(false) != "" {
		t.Fatalf("record %+v %q", got, b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), got.Image)); err != nil {
		t.Fatal(err)
	}
}

func TestFailedRecordRollsBack(t *testing.T) {
	root := t.TempDir()
	m := &Memo{Data: map[string]any{"invalid": make(chan int)}}
	if _, err := Write(root, m, image.NewNRGBA(image.Rect(0, 0, 1, 1)), []byte("save")); err == nil {
		t.Fatal("accepted non-JSON value")
	}
	files, err := filepath.Glob(filepath.Join(root, "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("partial record remains: %v", files)
	}
}

func TestConcurrentReservations(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	paths := make(chan string, 12)
	for range 12 {
		wg.Go(func() {
			p, err := Write(root, &Memo{Text: "A neutral observation."}, nil, nil)
			if err != nil {
				t.Error(err)
				return
			}
			paths <- p
		})
	}
	wg.Wait()
	close(paths)
	seen := map[string]bool{}
	for p := range paths {
		if seen[p] {
			t.Fatal("duplicate reservation")
		}
		seen[p] = true
	}
	if len(seen) != 12 {
		t.Fatal(len(seen))
	}
}

func TestReadRejectsTraversalAndWriteFailures(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "note.json")
	if err := os.WriteFile(p, []byte(`{"save":"../outside.save"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(p); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("traversal: %v", err)
	}
	if _, err := Write(p, &Memo{}, nil, nil); err == nil {
		t.Fatal("write into a file succeeded")
	}
	if _, err := Next(root, &Memo{Time: "invalid"}); err == nil {
		t.Fatal("accepted invalid timestamp")
	}
}
