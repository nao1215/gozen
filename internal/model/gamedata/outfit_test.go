package gamedata

import (
	"testing"
	"testing/fstest"
)

// Place outfit change: the outfit's image if present, otherwise the usual image.
func TestFacePictureInAnOutfit(t *testing.T) {
	db := &DB{FS: fstest.MapFS{"assets/chars/a/winter/normal.png": {Data: []byte("x")}}}
	a := &Actor{Faces: "chars/a"}
	if got := db.FacePicture(a, "normal", "winter"); got != "chars/a/winter/normal.png" {
		t.Errorf("in the outfit: %s", got)
	}
	if got := db.FacePicture(a, "happy", "winter"); got != "chars/a/happy.png" {
		t.Errorf("a face without its outfit picture: %s", got)
	}
	if got := db.FacePicture(a, "normal", ""); got != "chars/a/normal.png" {
		t.Errorf("no outfit: %s", got)
	}
}
