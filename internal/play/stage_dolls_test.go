package play

import (
	"image"
	"testing"
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
