package main

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestFacesWithoutModel: a game with no painted scene has nothing to do; one with a stage
// and no model says how to get it, and writes nothing.
func TestFacesWithoutModel(t *testing.T) {
	out, _ := invoke(t, 0, "faces", fixture(t))
	if !strings.Contains(out, "nothing to do") {
		t.Fatalf("a game without a stage: %s", out)
	}
	game := scenesGame(t)
	_, stderr := invoke(t, 1, "faces", game, "--model", t.TempDir()+"/none.onnx")
	if stderr == "" || strings.Contains(stderr, "no file") {
		t.Fatalf("the reason: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(game, "assets", "faces.json")); err == nil {
		t.Fatal("a failed run wrote faces.json")
	}
	invoke(t, 2, "art", "faces")
}

// scenesGame is the scenes test game (a stage with a person's picture) in a directory.
func scenesGame(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "scenes")
	fsys := testgame.ScenesFS(nil)
	delete(fsys, "assets/faces.json")
	if err := os.CopyFS(dir, fsys); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBestFace(t *testing.T) {
	b := image.Rect(0, 0, 100, 300)
	boxes := []detect.Box{
		{Rect: image.Rect(40, 200, 60, 220), Score: 0.9}, // low in the picture: a badge
		{Rect: image.Rect(40, 20, 60, 40), Score: 0.6},
	}
	if got, ok := bestFace(boxes, b); !ok || got.Rect.Min.Y != 20 {
		t.Fatalf("best %v %v", got, ok)
	}
	if _, ok := bestFace(nil, b); ok {
		t.Fatal("a face in nothing")
	}
}
