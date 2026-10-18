package detect

import (
	"image"
	"testing"
)

func TestPassesTilesAndClustering(t *testing.T) {
	if _, err := PassesByName("unknown"); err == nil {
		t.Fatal("unknown pass accepted")
	}
	passes, err := PassesByName("full640,tile2")
	if err != nil || len(passes) != 2 {
		t.Fatalf("passes: %v %v", passes, err)
	}
	b := image.Rect(7, 9, 47, 49)
	rs := tiles(b, 2, 1.0/3)
	if len(rs) != 4 {
		t.Fatal("tile count")
	}
	for _, r := range rs {
		if r.Empty() || !r.In(b) {
			t.Fatalf("tile outside input: %v", r)
		}
	}
	boxes := []Box{{Rect: image.Rect(1, 1, 5, 5), Label: "region", Score: 0.7, Pass: "a"}, {Rect: image.Rect(2, 2, 5, 5), Label: "region", Score: 0.8, Pass: "b"}}
	gs := Cluster(boxes, 0.5, 1)
	if len(gs) != 1 || gs[0].Hits != 2 || gs[0].Passes != 2 || gs[0].Score != 0.8 {
		t.Fatalf("cluster metadata: %v", gs)
	}
	if boxes[0].Score != 0.7 {
		t.Fatal("cluster mutated input")
	}
}
