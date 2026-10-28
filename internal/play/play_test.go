package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestStartRenderingLifecycle(t *testing.T) {
	for _, scripted := range []bool{false, true} {
		g := sysInn(t, Options{LiveDir: t.TempDir(), Development: true})
		if scripted {
			g.opts.Script = "press reload"
		}
		g.rendering = true // An offscreen Draw must not suppress loop initialization.
		g.StartRendering()
		if !g.rendering || g.pauseUnfocused == scripted || g.live.stop == nil || !ebiten.IsRunnableOnUnfocused() {
			t.Fatal("render loop initialization missing")
		}
		stop := g.live.stop
		g.StartRendering()
		if g.live.stop != stop {
			t.Fatal("duplicate live watcher")
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stop:
		default:
			t.Fatal("Close left live watcher running")
		}
	}
}
