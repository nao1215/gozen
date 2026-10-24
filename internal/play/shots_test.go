package play

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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

func TestCloseFinishesScreenshots(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "write failure"}[blocked], func(t *testing.T) {
			g, err := New(Options{FS: gameFiles(t), NoSound: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := g.Close(); err != nil {
					t.Error(err)
				}
			})
			path := filepath.Join(t.TempDir(), "capture.png")
			if blocked {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			g.shots.write(path, sysSolid(8, 6, color.NRGBA{R: 19, A: 255}))
			err = g.Close()
			if (err != nil) != blocked {
				t.Fatalf("Close() = %v, blocked = %v", err, blocked)
			}
			if !blocked {
				f, err := os.Open(path) //nolint:gosec // Reads the screenshot path created by this test.
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := f.Close(); err != nil {
						t.Error(err)
					}
				}()
				img, err := png.Decode(f)
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds() != image.Rect(0, 0, 8, 6) {
					t.Fatalf("incomplete screenshot: %v", img.Bounds())
				}
			}
		})
	}
}
