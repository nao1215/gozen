package gamedata

import "testing"

func TestFacePictureWithoutPortrait(t *testing.T) {
	db := &DB{}
	for _, actor := range []*Actor{nil, {Name: "Rowan"}} {
		for _, outfit := range []string{"", "travel"} {
			if got := db.FacePicture(actor, "normal", outfit); got != "" {
				t.Fatalf("portrait without a directory: %q", got)
			}
		}
	}
}
