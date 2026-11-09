package gamedata_test

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestExtremeInputs(t *testing.T) {
	for _, n := range []int{32, 33} {
		members := strings.TrimSuffix(strings.Repeat("mote,", n), ",")
		_, err := testgame.TryLoad(map[string]string{"data/extreme.yaml": "troops: {crowd: {members: [" + members + "]}}"})
		if (err == nil) != (n == 32) {
			t.Fatalf("count %d: %v", n, err)
		}
		if n == 33 && !strings.Contains(err.Error(), "at most 32") {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8193, 1))); err != nil {
		t.Fatal(err)
	}
	_, err := testgame.TryLoad(map[string]string{"assets/x.png": b.String()})
	if err == nil || !strings.Contains(err.Error(), "picture dimensions") {
		t.Fatalf("oversize: %v", err)
	}
}
