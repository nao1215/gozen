package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadNarrativeWithoutCombat(t *testing.T) {
	for _, tc := range []struct{ party, command, want string }{
		{"", "say: {who: guide, text: Welcome}", ""},
		{"party: [guide],", "say: Welcome", "needs a class"},
		{"", "join: guide", "needs a class"},
	} {
		fsys := fstest.MapFS{
			"game.yaml":       {Data: []byte("schema_version: 8\nid: story\nstart: {" + tc.party + "location: station, event: opening}\n")},
			"data/story.yaml": {Data: []byte("actors:\n  guide: {name: Rowan}\nlocations:\n  station: {name: Station}\nevents:\n  opening:\n    do:\n      - " + tc.command + "\n")},
		}
		db, err := Load(fsys)
		if tc.want != "" {
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(db.Classes) != 0 || len(db.Skills) != 0 || len(db.Game.Start.Party) != 0 {
			t.Fatal("narrative acquired combat definitions")
		}
	}
}

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
