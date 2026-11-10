package play

import (
	"image"
	"testing"
	"testing/fstest"
	"time"
)

func TestDollCaptureWaitsForRequestedPicture(t *testing.T) {
	for _, mode := range []string{"until", "trace-shots", "script", "shot", "normal"} {
		t.Run(mode, func(t *testing.T) {
			g := &Game{tick: 18}
			switch mode {
			case "until":
				g.opts.Until = 19
			case "trace-shots":
				g.opts.TraceShots = true
			case "script":
				g.opts.Script = "input.txt"
			case "shot":
				g.autoShot = "frame"
			}
			c := g.scenes()
			defer g.closeScenes()
			sp := newDollSpec("face.png", nil, 30)
			loaded := make(chan struct{})
			c.pending[sp.k] = loaded
			// An unrelated background load must not hold up this screen.
			c.pending["unrelated"] = make(chan struct{})
			if mode == "normal" {
				if g.dollFace(sp) != nil {
					t.Fatal("unfinished face appeared")
				}
				return
			}
			go func() {
				time.Sleep(10 * time.Millisecond)
				c.mu.Lock()
				c.baked[sp.k] = baked{
					sheet: image.NewRGBA(image.Rect(0, 0, 4, 4)),
					face:  image.NewRGBA(image.Rect(0, 0, 2, 2)),
				}
				c.mu.Unlock()
				close(loaded)
			}()
			if g.dollFace(sp) == nil || g.failure != nil {
				t.Fatalf("capture did not load the face: %v", g.failure)
			}
			if g.tick != 18 {
				t.Fatalf("waiting advanced tick to %d", g.tick)
			}
		})
	}
}

func TestDollCaptureTimeoutDoesNotSavePlaceholder(t *testing.T) {
	g := &Game{tick: 18, autoShot: "frame", pictureDeadline: time.Now().Add(-time.Second)}
	sp := newDollSpec("face.png", nil, 30)
	g.scenes().pending[sp.k] = make(chan struct{})
	defer g.closeScenes()
	if g.dollFace(sp) != nil || g.failure == nil {
		t.Fatal("unfinished picture did not fail at the deadline")
	}
	if g.tick != 18 {
		t.Fatal("timeout advanced simulation")
	}
	// No image is needed: the failure must reject the shot before reading pixels.
	g.saveShotQuiet(nil, "frame")
}

func TestDollCaptureSettlesFailedPicture(t *testing.T) {
	for _, path := range []string{"invalid.png", "missing.png"} {
		t.Run(path, func(t *testing.T) {
			g := &Game{tick: 18, autoShot: "frame", pictureDeadline: time.Now().Add(time.Second)}
			g.pics = newPictures(fstest.MapFS{"assets/invalid.png": {Data: []byte("not an image")}})
			defer g.closeScenes()
			sp := newDollSpec(path, nil, 30)
			if g.dollSheet(sp) != nil || g.failure != nil {
				t.Fatalf("failed picture did not settle as a placeholder: %v", g.failure)
			}
			select {
			case <-g.scenes().pending[sp.k]:
			default:
				t.Fatal("failed load is still pending")
			}
			// A settled failure remains settled, even after the draw deadline.
			g.pictureDeadline = time.Now().Add(-time.Second)
			for range 10 {
				if g.dollFace(sp) != nil || g.failure != nil {
					t.Fatalf("failed picture was retried: %v", g.failure)
				}
			}
			if g.tick != 18 {
				t.Fatal("failed load advanced simulation")
			}
		})
	}
}

func TestDollCaptureCompletedLoadWinsOverDeadline(t *testing.T) {
	g := &Game{pictureDeadline: time.Now().Add(-time.Second)}
	done := make(chan struct{})
	close(done)
	for range 100 {
		if !g.waitDoll(done) || g.failure != nil {
			t.Fatalf("completed load timed out: %v", g.failure)
		}
	}
}

// TestDollBakeKeepsOffTheGame: a doll baking in the background must not read or make the
// game's scene cache (g.scenes()) from its goroutine: a reload drops and remakes the cache
// on the game's thread meanwhile. Run with -race.
func TestDollBakeKeepsOffTheGame(t *testing.T) {
	files := gameFiles(t)
	files["assets/bake.png"] = &fstest.MapFile{Data: btlPNG(64, 128)}
	g, err := New(Options{FS: files, Location: "camp", NoSound: true, SaveDir: t.TempDir(), Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := range 4 {
		sp := newDollSpec("bake.png", nil, 30+float64(i))
		g.dollSheet(sp) // starts the bake
		c := g.sceneCache
		c.mu.Lock()
		done := c.pending[sp.k]
		c.mu.Unlock()
		g.closeScenes() // F5 while it bakes
		g.scenes()
		<-done
	}
}
