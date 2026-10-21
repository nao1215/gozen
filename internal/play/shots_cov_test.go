package play

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sysSolid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// TestShotWriterWritesEveryQueuedShot: all queued shots are on disk, as PNGs with the pixels
// that were queued, once wait returns; the directory is created.
func TestShotWriterWritesEveryQueuedShot(t *testing.T) {
	var w shotWriter
	dir := filepath.Join(t.TempDir(), "deep", "shots")
	for i := range 12 {
		img := w.pixels(image.Rect(0, 0, 8, 6))
		c := color.NRGBA{R: uint8(i * 20), G: 7, B: 9, A: 255}
		copy(img.Pix, sysSolid(8, 6, c).Pix)
		w.write(filepath.Join(dir, "s"+string(rune('a'+i))+".png"), img)
	}
	if err := w.wait(); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		f, err := os.Open(filepath.Join(dir, "s"+string(rune('a'+i))+".png")) //nolint:gosec // reads a file this test wrote
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := img.At(3, 3).RGBA()
		if r>>8 != uint32(i*20) || g>>8 != 7 || b>>8 != 9 {
			t.Fatalf("shot %d has the wrong pixels: %d %d %d", i, r>>8, g>>8, b>>8)
		}
	}
}

// TestShotWriterAfterWaitWritesAtOnce: a shot taken after wait is written by the caller; wait
// twice is harmless; with nothing queued it never starts workers.
func TestShotWriterAfterWaitWritesAtOnce(t *testing.T) {
	var w shotWriter
	if err := w.wait(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "late.png")
	w.write(path, sysSolid(2, 2, color.NRGBA{A: 255}))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a shot after wait was not written at once: %v", err)
	}
	if err := w.wait(); err != nil {
		t.Fatal(err)
	}
	var started shotWriter
	started.write(filepath.Join(t.TempDir(), "x.png"), sysSolid(2, 2, color.NRGBA{A: 255}))
	if err := started.wait(); err != nil {
		t.Fatal(err)
	}
	started.write(filepath.Join(t.TempDir(), "y.png"), sysSolid(2, 2, color.NRGBA{A: 255}))
}

// TestShotWriterReportsFailuresOnce: a shot that cannot be written is reported by failed once,
// and by wait when not asked before.
func TestShotWriterReportsFailuresOnce(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var w shotWriter
	w.write(filepath.Join(blocked, "a.png"), sysSolid(2, 2, color.NRGBA{A: 255}))
	w.write(filepath.Join(blocked, "b.png"), sysSolid(2, 2, color.NRGBA{A: 255}))
	err := w.wait()
	if err == nil {
		t.Fatal("write failure hidden")
	}
	if w.failed() != nil {
		t.Fatal("the failure was reported twice")
	}
	// a path that is a directory cannot be created as a file.
	dirAsFile := t.TempDir()
	if err := writeShot(dirAsFile, sysSolid(1, 1, color.NRGBA{A: 255})); err == nil {
		t.Fatal("wrote over a directory")
	}
	w2 := shotWriter{}
	w2.done(nil)
	if w2.failed() != nil {
		t.Fatal("nil error recorded")
	}
}

// TestShotWriterReusesBuffersOfTheSameSize: pixels hands back a written buffer only for the
// same bounds.
func TestShotWriterReusesBuffersOfTheSameSize(t *testing.T) {
	var w shotWriter
	a := w.pixels(image.Rect(0, 0, 4, 4))
	w.pool.Put(a)
	if b := w.pixels(image.Rect(0, 0, 5, 5)); b == a || b.Rect.Dx() != 5 {
		t.Fatal("a buffer of another size was reused")
	}
}

// TestRecordNeedsSomethingToRecord: without a script, autopilot or replay Record refuses; with
// no ffmpeg on the path it says how to get it.
func TestRecordNeedsSomethingToRecord(t *testing.T) {
	err := Record(context.Background(), Options{FS: gameFiles(t)}, filepath.Join(t.TempDir(), "o.mp4"))
	if err == nil || !strings.Contains(err.Error(), "script") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("PATH", "")
	err = Record(context.Background(), Options{FS: gameFiles(t), Script: "quit"}, filepath.Join(t.TempDir(), "o.mp4"))
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("err = %v", err)
	}
}

// TestRecordRefusesBrokenData: a game that does not load stops Record before anything starts.
func TestRecordRefusesBrokenData(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	fsys := gameFiles(t)
	delete(fsys, "game.yaml")
	err := Record(context.Background(), Options{FS: fsys, Script: "quit"}, filepath.Join(t.TempDir(), "o.mp4"))
	if err == nil {
		t.Fatal("recorded a game without game.yaml")
	}
}

// TestRunRefusesACancelledContext and bad options.
func TestRunRefusesACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, Options{FS: gameFiles(t)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if err := Run(context.Background(), Options{}); err == nil {
		t.Fatal("ran without a filesystem")
	}
}
