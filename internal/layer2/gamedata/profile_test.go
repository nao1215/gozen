package gamedata_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// profileGame is the test expedition at schema version 4, its scout with faces.
var profileGame = "schema_version: 4\nid: p\n" + testgame.Game

const profileFaces = "actors: {scout: {name: Scout, class: ranger, faces: chars/scout, equip: {weapon: stick}}}\n"

func profileFiles(profiles string) map[string]string {
	return map[string]string{
		"game.yaml":                     profileGame,
		"data/main.yaml":                strings.Replace(testgame.Data, "  scout: {name: Scout, class: ranger, equip: {weapon: stick}}\n", "", 1),
		"data/faces.yaml":               profileFaces,
		"data/profiles.yaml":            profiles,
		"assets/chars/scout/normal.png": "x",
		"assets/chars/scout/smile.png":  "x",
	}
}

// TestProfilesLoad: a profile whose id is an actor's belongs to it, takes its name and
// face, and keeps the order written.
func TestProfilesLoad(t *testing.T) {
	db := testgame.Load(t, profileFiles(`profiles:
  scout:
    title: The first to walk
    faces: [normal, smile]
    when: "flag.met"
    fields: [{label: Role, value: Lead}]
    text: "Carries {gold} gold."
    relations: [{who: you, calls: Boss, text: Trusts you.}]
  ranger:
    name: The ranger
    picture: x.png
`))
	if got := db.Order["profiles"]; len(got) != 2 || got[0] != "scout" || got[1] != "ranger" {
		t.Fatalf("order %v", got)
	}
	if db.ProfileActor("scout") != "scout" || db.ProfileName("scout") != "Scout" {
		t.Errorf("scout: actor %q name %q", db.ProfileActor("scout"), db.ProfileName("scout"))
	}
	if db.ProfileActor("ranger") != "" || db.ProfileName("ranger") != "The ranger" {
		t.Errorf("ranger: actor %q name %q", db.ProfileActor("ranger"), db.ProfileName("ranger"))
	}
	if f := db.ProfileFaces("scout"); len(f) != 2 || f[1] != "smile" {
		t.Errorf("faces %v", f)
	}
	st := testgame.State(db, 1)
	if db.ProfileHolds("scout", st.Env()) {
		t.Error("scout is open before flag.met")
	}
	st.Flags["met"] = true
	if !db.ProfileHolds("scout", st.Env()) || !db.ProfileHolds("ranger", st.Env()) || db.ProfileHolds("nobody", st.Env()) {
		t.Error("the when of the profiles does not hold as written")
	}
}

// TestProfilesCheck: check reports what a profile points at and does not find.
func TestProfilesCheck(t *testing.T) {
	tests := []struct{ name, profiles, want string }{
		{"no actor of the id", `profiles: {nobody: {title: X}}`, `profiles.nobody: no actor "nobody" of the same id`},
		{"unknown actor", `profiles: {x: {actor: nobody}}`, `profiles.x.actor: unknown actor "nobody"`},
		{"unknown enemy", `profiles: {x: {enemy: nobody}}`, `profiles.x.enemy: unknown enemy "nobody"`},
		{"two pictures", `profiles: {x: {actor: scout, picture: x.png}}`, `write one of actor, enemy and picture`},
		{"missing picture", `profiles: {x: {picture: none.png}}`, `profiles.x.picture: no file assets/none.png`},
		{"missing face", `profiles: {scout: {faces: [normal, angry]}}`, `profiles.scout.faces[1]: no file assets/chars/scout/angry.png`},
		{"faces without an actor", `profiles: {x: {picture: x.png, faces: [a]}}`, `faces need an actor`},
		{"bad when", `profiles: {scout: {when: "flg.x"}}`, `profiles.scout.when: "flg.x" uses the unknown name "flg"`},
		{"bad text", `profiles: {scout: {text: "{gld} gold"}}`, `profiles.scout.text: "gld" uses the unknown name`},
		{"bad value", `profiles: {scout: {fields: [{label: A, value: "{gld}"}]}}`, `profiles.scout.fields[0].value`},
		{"no label", `profiles: {scout: {fields: [{value: B}]}}`, `profiles.scout.fields[0]: label is missing`},
		{"unknown who", `profiles: {scout: {relations: [{who: stranger, calls: S}]}}`, `profiles.scout.relations[0].who: unknown character "stranger"`},
		{"unknown kind", `profiles: {scout: {relations: [{who: you, kind: devotion}]}}`, `profiles.scout.relations[0].kind: unknown relationship kind "devotion"`},
		{"unknown field", `profiles: {scout: {age: 3}}`, `age`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testgame.TryLoad(profileFiles(tt.profiles))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
	t.Run("needs version 4", func(t *testing.T) {
		files := profileFiles(`profiles: {scout: {}}`)
		files["game.yaml"] = testgame.Game
		_, err := testgame.TryLoad(files)
		if err == nil || !strings.Contains(err.Error(), "profiles needs schema_version: 4") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("open profiles without profiles", func(t *testing.T) {
		files := profileFiles(`events: {e: {do: [{open: profiles}]}}`)
		_, err := testgame.TryLoad(files)
		if err == nil || !strings.Contains(err.Error(), "open: profiles needs profiles") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("ran and open profiles are accepted", func(t *testing.T) {
		testgame.Load(t, profileFiles(`profiles: {scout: {when: "ran.sub.count > 0", relations: [{who: scout, calls: Me}]}}
events: {e: {do: [{open: profiles}]}}`))
	})
}

// TestProfilesTranslate: the texts of a profile are translated like the names, and the
// ones without a translation are warned of.
func TestProfilesTranslate(t *testing.T) {
	files := profileFiles(`profiles:
  scout:
    title: The first to walk
    fields: [{label: Role, value: Lead}]
    text: Walks ahead.
    relations: [{who: you, calls: Boss, text: Trusts you.}]
`)
	files["game.yaml"] = twoLanguages + "language: ja\n" + testgame.Game
	files["lang/ja.yaml"] = "text:\n  \"The first to walk\": \"先を歩く人\"\n  \"Role\": \"役目\"\n"
	db := testgame.Load(t, files)
	p := db.Profiles["scout"]
	if p.Title != "先を歩く人" || p.Fields[0].Label != "役目" || p.Text != "Walks ahead." {
		t.Fatalf("translated: %q %q %q", p.Title, p.Fields[0].Label, p.Text)
	}
	db.Show("en")
	if p.Title != "The first to walk" {
		t.Fatalf("back to English: %q", p.Title)
	}
	w := errors.Join(db.SceneWarnings()...).Error()
	for _, want := range []string{
		`profiles.scout.text: not translated to ja: "Walks ahead."`,
		`profiles.scout.fields[0].value: not translated to ja: "Lead"`,
		`profiles.scout.relations[0].calls: not translated to ja: "Boss"`,
	} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning %q in\n%s", want, w)
		}
	}
}

// TestProfilesFaceBoxes: the pictures of a book are face pictures for gozen art faces,
// with the actors' faces; check warns of one without a box in assets/faces.json.
func TestProfilesFaceBoxes(t *testing.T) {
	files := profileFiles(`profiles:
  scout: {faces: [smile, normal]}
  keeper: {picture: x.png}
`)
	db := testgame.Load(t, files)
	pics := db.ScenePictures()
	for _, want := range []string{"chars/scout/normal.png", "chars/scout/smile.png", "x.png"} {
		if !slices.Contains(pics, want) {
			t.Errorf("no %s in %v", want, pics)
		}
	}
	if db.ProfileIcon("scout") != "chars/scout/smile.png" || db.ProfileIcon("keeper") != "x.png" {
		t.Errorf("icons %q %q", db.ProfileIcon("scout"), db.ProfileIcon("keeper"))
	}
	w := errors.Join(db.SceneWarnings()...).Error()
	if !strings.Contains(w, "assets/faces.json: no such file") {
		t.Errorf("no warning of the missing faces file in\n%s", w)
	}
	files["assets/faces.json"] = `{"version": 1, "pictures": {}}`
	db = testgame.Load(t, files)
	w = errors.Join(db.SceneWarnings()...).Error()
	if !strings.Contains(w, "assets/x.png: no face box in assets/faces.json") {
		t.Errorf("no warning of the picture without a box in\n%s", w)
	}
}
