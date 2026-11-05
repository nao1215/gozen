package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// profileGame10 is the test expedition at schema version 10.
var profileGame10 = "schema_version: 10\nid: p\n" + testgame.Game

// TestProfileFaceMaps: a face may be a map with the label of its button and a when of
// its own; a face without a when is always offered, and names and maps mix.
func TestProfileFaceMaps(t *testing.T) {
	files := profileFiles(`profiles:
  scout:
    faces: [normal, {face: smile, label: The smile, when: "flag.smiled"}]
`)
	files["game.yaml"] = profileGame10
	db := testgame.Load(t, files)
	if f := db.ProfileFaces("scout"); len(f) != 2 || f[0] != "normal" || f[1] != "smile" {
		t.Fatalf("faces %v", f)
	}
	if db.ProfileFaceLabel("scout", 0) != "normal" || db.ProfileFaceLabel("scout", 1) != "The smile" || db.ProfileFaceLabel("scout", 2) != "" {
		t.Errorf("labels %q %q", db.ProfileFaceLabel("scout", 0), db.ProfileFaceLabel("scout", 1))
	}
	if db.ProfileFaceLocked("scout", 0) || !db.ProfileFaceLocked("scout", 1) || db.ProfileFaceLocked("nobody", 0) {
		t.Error("only the face with a when is locked")
	}
	st := testgame.State(db, 1)
	if !db.ProfileFaceHolds("scout", 0, st.Env()) || db.ProfileFaceHolds("scout", 1, st.Env()) {
		t.Error("the smile is open before flag.smiled")
	}
	st.Flags["smiled"] = true
	if !db.ProfileFaceHolds("scout", 1, st.Env()) {
		t.Error("the smile stays closed after flag.smiled")
	}
	if db.ProfileIcon("scout") != "chars/scout/normal.png" {
		t.Errorf("icon %q", db.ProfileIcon("scout"))
	}
}

// TestProfileFaceChecks: check reports a face map's problems and its schema version.
func TestProfileFaceChecks(t *testing.T) {
	tests := []struct{ name, game, profiles, want string }{
		{"needs version 10", profileGame, `profiles: {scout: {faces: [{face: smile, label: S}]}}`, `profiles.scout.faces[0]: a face with label, when or seen needs schema_version: 10`},
		{"no face", profileGame10, `profiles: {scout: {faces: [{label: S}]}}`, `profiles.scout.faces[0]: face is missing`},
		{"bad when", profileGame10, `profiles: {scout: {faces: [{face: smile, when: "flg.x"}]}}`, `profiles.scout.faces[0].when: "flg.x" uses the unknown name "flg"`},
		{"missing file", profileGame10, `profiles: {scout: {faces: [{face: angry, label: A}]}}`, `profiles.scout.faces[0]: no file assets/chars/scout/angry.png`},
		{"bad layout", strings.Replace(profileGame10, "id: p\n", "id: p\nui: {profile_book: {layout: cards}}\n", 1), `profiles: {scout: {}}`, `ui.profile_book.layout: want list or grid, got "cards"`},
		{"book needs version 10", strings.Replace(profileGame, "id: p\n", "id: p\nui: {profile_book: {title: Book}}\n", 1), `profiles: {scout: {}}`, `ui.profile_book needs schema_version: 10`},
		{"faces of an actor without pictures", profileGame10, `profiles: {ranger: {actor: scout2, faces: [x]}}`, `actor scout2 has no faces nor puppet to show`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := profileFiles(tt.profiles)
			files["game.yaml"] = tt.game
			files["data/more.yaml"] = "actors: {scout2: {name: Scout two, class: ranger, equip: {weapon: stick}}}\n"
			_, err := testgame.TryLoad(files)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
}

// TestProfileWithoutPicture: an actor without faces nor puppet (a voice in the lines)
// still has a page, without a picture or an icon, and the book's shape loads.
func TestProfileWithoutPicture(t *testing.T) {
	files := profileFiles(`profiles: {voice: {actor: scout2, title: Heard not seen}}`)
	files["game.yaml"] = strings.Replace(profileGame10, "id: p\n", "id: p\nui: {profiles: true, profile_book: {title: The book, layout: grid}}\n", 1)
	files["data/more.yaml"] = "actors: {scout2: {name: Scout two, class: ranger, equip: {weapon: stick}}}\n"
	db := testgame.Load(t, files)
	if db.ProfileIcon("voice") != "" || db.ProfileName("voice") != "Scout two" {
		t.Errorf("icon %q name %q", db.ProfileIcon("voice"), db.ProfileName("voice"))
	}
	if b := db.Game.UI.ProfileBook; b == nil || b.Title != "The book" || b.Layout != "grid" {
		t.Errorf("book %+v", b)
	}
}

// TestProfileMetAndSeen: a profile with met opens by meeting its actor, not by a
// condition, and a face with seen opens when a line shows the actor with it.
func TestProfileMetAndSeen(t *testing.T) {
	files := profileFiles(`profiles:
  scout: {met: true, faces: [normal, {face: smile, label: Smile, seen: true}]}
  ranger: {picture: x.png}
`)
	files["game.yaml"] = profileGame10
	db := testgame.Load(t, files)
	st := testgame.State(db, 1)
	if db.ProfileHolds("scout", st.Env()) || !db.ProfileHolds("ranger", st.Env()) {
		t.Error("a met profile opened by a condition")
	}
	if !db.ProfileFaceLocked("scout", 1) || db.ProfileFaceHolds("scout", 1, st.Env()) {
		t.Error("a seen face opened before it was seen")
	}
	met := db.ProfilesMet("scout", "chars/scout/smile.png")
	if f, ok := met["scout"]; !ok || len(f) != 1 || f[0] != "smile" {
		t.Fatalf("met %v", met)
	}
	if f := db.ProfilesMet("scout", "chars/scout/normal.png")["scout"]; len(f) != 0 {
		t.Errorf("the normal face opened the smile: %v", f)
	}
	if db.ProfilesMet("", "") != nil || db.ProfilesMet("ranger", "") != nil {
		t.Error("met without an actor")
	}
	for _, tt := range []struct{ profiles, want string }{
		{`profiles: {ranger: {picture: x.png, met: true}}`, `profiles.ranger.met: met needs an actor`},
		{`profiles: {scout: {faces: [{face: smile, seen: true}]}}`, `needs schema_version: 10`},
	} {
		files := profileFiles(tt.profiles)
		if strings.Contains(tt.profiles, "met") {
			files["game.yaml"] = profileGame10
		}
		if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("want %q, got %v", tt.want, err)
		}
	}
}
