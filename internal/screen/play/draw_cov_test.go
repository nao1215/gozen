package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// covSlow skips a test that drives many frames (more than about 50 ms) under -short, so that
// go test -short stays quick; the full run and the coverage run keep it.
func covSlow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("drives many frames; runs without -short")
	}
}

// Tests draw without a game loop. Ebitengine then cannot run the draw commands: it keeps
// every one of them (with copies of their vertices) until a frame begins, which never
// happens in a test process. So each Draw a test makes stays in memory until the process
// ends. Drawing every frame of every test kept gigabytes, so the frame loops of the tests
// draw only some frames (covDraws), on one shared screen.

// covDrawEvery is how many frames the test loops run per draw in the first covDenseFrames
// frames of a game; after that they draw one frame in covSparseEvery (long runs, such as a
// whole battle or a walk with random keys, repeat the same screens).
const (
	covDrawEvery   = 6
	covDenseFrames = 180
	covSparseEvery = 240
)

// covDraws reports whether a test loop draws the frame of the given count.
func covDraws(frame int) bool {
	if frame < covDenseFrames {
		return frame%covDrawEvery == 0
	}
	return frame%covSparseEvery == 0
}

// covTarget is the one screen the tests draw to.
var covTarget *ebiten.Image

// covScreen is the shared screen the tests draw to.
func covScreen() *ebiten.Image {
	if covTarget == nil {
		covTarget = ebiten.NewImage(1280, 720)
	}
	return covTarget
}
