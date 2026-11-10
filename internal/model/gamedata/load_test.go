package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
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

func TestCompactEventAndItems(t *testing.T) {
	for _, body := range []string{
		"  opening:\n    - give: token\n    - take: token\n    - give: {item: token, count: 0}\n    - wait: {seconds: 0}\n",
		"  opening: [{give: token}, {take: token}, {give: {item: token, count: 0}}, {wait: {seconds: 0}}]\n",
	} {
		fsys := fstest.MapFS{
			"game.yaml":       {Data: []byte("schema_version: 11\nid: story\nstart: {location: station, event: opening}\n")},
			"data/story.yaml": {Data: []byte("locations: {station: {name: Station}}\nitems: {token: {name: Token}}\nevents:\n" + body)},
		}
		db, err := Load(fsys)
		if err != nil {
			t.Fatal(err)
		}
		commands := db.Events["opening"].Do
		if len(commands) != 4 || commands[0].Give.N() != 1 || commands[1].Take.N() != 1 || commands[2].Give.N() != 0 {
			t.Fatalf("item count changed: %+v", commands)
		}
		fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "11", "10", 1))
		if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "schema_version: 11") {
			t.Fatalf("old schema: %v", err)
		}
	}
}

func TestCompactEventDiagnosticPosition(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 11\nid: story\nstart: {location: station}\n")},
		"data/story.yaml": {Data: []byte("locations: {station: {name: Station}}\nevents:\n  broken:\n    - give: missing\n")},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "data/story.yaml:4:7:") || !strings.Contains(err.Error(), `unknown item "missing"`) {
		t.Fatalf("diagnostic: %v", err)
	}
}

func TestTacticYAMLDefaults(t *testing.T) {
	for _, tc := range []struct {
		body    string
		enabled bool
	}{
		{"{condition: any, action: attack}", false},
		{"{enabled: true, condition: any, action: attack}", true},
		{"{enabled: false, condition: any, action: attack}", false},
	} {
		for _, version := range []int{10, 11} {
			var row TacticRow
			if err := yaml.Unmarshal([]byte(tc.body), &row); err != nil {
				t.Fatal(err)
			}
			// Exercise the version-aware loader walk with the authored raw presence.
			var raw yaml.MapSlice
			if err := yaml.UnmarshalWithOptions([]byte(tc.body), &raw, yaml.UseOrderedMap()); err != nil {
				t.Fatal(err)
			}
			db := &DB{Game: Game{SchemaVersion: version}}
			if err := db.resolveNumbers(raw, &row, "row"); err != nil {
				t.Fatal(err)
			}
			want := tc.enabled || version == 11 && !strings.Contains(tc.body, "enabled")
			if row.Enabled != want {
				t.Fatalf("version %d %s: got %v", version, tc.body, row.Enabled)
			}
		}
	}
}

// TestAssetNamesStayUnderAssets: a name that climbs out of assets/ names no asset,
// even when the file it reaches exists.
func TestAssetNamesStayUnderAssets(t *testing.T) {
	db := &DB{FS: fstest.MapFS{"game.yaml": {Data: []byte("title: T")}, "assets/x.png": {Data: []byte("x")}}}
	c := &checker{db: db}
	c.file("game.yaml fonts.regular", "../game.yaml")
	if len(c.errs) == 0 {
		t.Error(`fonts.regular: ../game.yaml passes as an asset (assets/../game.yaml)`)
	}
}
