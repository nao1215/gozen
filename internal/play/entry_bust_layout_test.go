package play

import (
	"image"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// A bust with a height goes across the top of the entry, the text under it;
// a bare full-width bust is the whole entry.
func TestBustLayoutHeightAndBare(t *testing.T) {
	r := image.Rect(100, 200, 240, 450)
	left, _, _ := bustLayout(r, &data.EntryBust{})
	if left.Min.X != 106 || left.Max.Y != 444 || left.Dx() >= r.Dx()/2 {
		t.Errorf("a plain bust is not at the left: %v", left)
	}
	top, _, _ := bustLayout(r, &data.EntryBust{Height: .6})
	if top.Min.X != 106 || top.Max.X != 234 || top.Max.Y != 350 {
		t.Errorf("a bust with a height is not across the top: %v", top)
	}
	whole, _, _ := bustLayout(r, &data.EntryBust{Width: 1, Bare: true})
	if whole != r {
		t.Errorf("a bare full-width bust is not the whole entry: %v", whole)
	}
}
