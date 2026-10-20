package trial

import (
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestTrialKeepsFaceBoxes: assets/faces.json is named by no YAML, yet the painted scenes
// read it; the trial tree keeps it.
func TestTrialKeepsFaceBoxes(t *testing.T) {
	cfg, err := Parse([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Build(testgame.ScenesFS(nil), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Assets, "faces.json") {
		t.Fatalf("the trial's assets lack faces.json: %v", result.Assets)
	}
}
