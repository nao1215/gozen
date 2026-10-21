package play

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const mnuGalleryYAML = `
gallery:
  cgs:
    - {file: cg/a.png, title: Alpha}
    - {files: ["cg/b.*.png"], title: Beta}
    - {file: cg/c.png, title: Gamma, flag: gamma_seen}
  scenes:
    - {id: s1, title: First, event: recall1, flag: met_one}
    - {id: s2, title: Second, event: recall2, seen: cg/a.png, pages: [{title: Middle, line: 2}]}
    - {id: s3, title: Third, event: recall1, when: "gold > 500", legacy_keys: ["scene:old third"]}
  music_pictures: {theme: [cg/a.png, cg/b.001.png]}
`

const mnuGalleryData = `
events:
  recall1:
    do:
      - say: "First line."
      - say: "Second line."
      - set: {flag.met_one: true}
  recall2:
    do:
      - say: "One."
      - say: "Two."
      - say: "Three."
music:
  bars: 2
  beats_per_bar: 4
  one_shot: [battle]
  mood_names: {battle: Fight}
  songs:
    theme: {dir: m, bpm: 120, moods: {neutral: calm, battle: fight}, name: Theme}
    plain: {file: x.png}
`

// mnuGalleryFS is the camp game with a gallery of three pictures, three recollections and two songs.
func mnuGalleryFS(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append([]byte("schema_version: 4\n"), append(fsys["game.yaml"].Data, []byte(mnuGalleryYAML)...)...)
	fsys["data/gallery.yaml"] = &fstest.MapFile{Data: []byte(mnuGalleryData)}
	png := fsys["assets/x.png"].Data
	for _, p := range []string{"cg/a.png", "cg/b.001.png", "cg/b.002.png", "cg/c.png"} {
		fsys["assets/"+p] = &fstest.MapFile{Data: png}
	}
	fsys["assets/m/calm.ogg"] = &fstest.MapFile{Data: []byte("x")}
	fsys["assets/m/fight.ogg"] = &fstest.MapFile{Data: []byte("x")}
	return fsys
}

func mnuGalleryGame(t *testing.T, opts Options) *Game {
	t.Helper()
	opts.FS, opts.NoSound = mnuGalleryFS(t), true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.Location == "" {
		opts.Location = "camp"
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil && !errors.Is(err, fs.ErrClosed) {
			t.Error(err)
		}
	})
	return g
}

// Opened pictures are written to the player's record and are still open in the next run.
func TestGalleryUnlocksPersist(t *testing.T) {
	dir := t.TempDir()
	g := mnuGalleryGame(t, Options{SaveDir: dir, Seed: 1})
	if !g.hasGallery() {
		t.Fatal("a game with a gallery says it has none")
	}
	if len(g.unlocked) != 0 {
		t.Fatalf("fresh game has unlocks: %v", g.unlocked)
	}
	g.seenPicture("cg/b.002.png") // a later page of the picture opens the picture
	if !g.unlocked["cg:cg/b.*.png"] && !g.unlocked["cg:"+g.db.Game.Gallery.CGs[1].Key()] {
		t.Fatalf("showing a page did not open the picture: %v", g.unlocked)
	}
	g.seenPicture("cg/a.png")
	if !g.unlocked["cg:cg/a.png"] {
		t.Fatalf("unlocks %v", g.unlocked)
	}
	// the second recollection opens by having seen cg/a.png
	if !g.unlocked[g.db.Game.Gallery.Scenes[1].Key()] {
		t.Fatalf("scene by seen did not open: %v", g.unlocked)
	}
	g.seenPicture("not/in/gallery.png")
	want := len(g.unlocked)
	b, err := g.storeRead("gallery.json")
	if err != nil {
		t.Fatal(err)
	}
	var rec galleryRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.FormatVersion != galleryFormatVersion || rec.GameID != "screen-test" || len(rec.Unlocks) != want {
		t.Fatalf("record %+v, want %d unlocks", rec, want)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := mnuGalleryGame(t, Options{SaveDir: dir, Seed: 1})
	if len(g2.unlocked) != want || !g2.unlocked["cg:cg/a.png"] {
		t.Fatalf("after a restart: %v", g2.unlocked)
	}
}

func TestReadUnlocksRules(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		legacy bool
		err    string
		n      int
	}{
		{"legacy flat map", `{"cg:a": true, "scene:x": true}`, true, "", 2},
		{"versioned", `{"format_version": 1, "game_id": "g", "unlocks": {"cg:a": true}}`, false, "", 1},
		{"not json", `{`, false, "unexpected", 0},
		{"null", `null`, false, "", 0},
		{"array", `[]`, false, "cannot unmarshal", 0},
		{"unknown field", `{"format_version": 1, "game_id": "g", "unlocks": {}, "extra": 1}`, false, "unknown field", 0},
		{"newer format", `{"format_version": 2, "game_id": "g", "unlocks": {}}`, false, "2", 0},
		{"other game", `{"format_version": 1, "game_id": "h", "unlocks": {}}`, false, "\"h\"", 0},
		{"no game id", `{"format_version": 1, "unlocks": {}}`, false, "\"\"", 0},
		{"null game id", `{"format_version": 1, "game_id": null, "unlocks": {}}`, false, "", 0},
		{"no unlocks", `{"format_version": 1, "game_id": "g"}`, false, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, legacy, err := readUnlocks([]byte(c.in), "g")
			if c.name == "null" || c.name == "null game id" || c.name == "no unlocks" {
				if err == nil {
					t.Fatalf("accepted %s: %v", c.in, m)
				}
				return
			}
			if (err != nil) != (c.err != "") || (err != nil && !strings.Contains(err.Error(), c.err)) {
				t.Fatalf("err %v, want containing %q", err, c.err)
			}
			if err == nil && (legacy != c.legacy || len(m) != c.n) {
				t.Fatalf("legacy %v map %v", legacy, m)
			}
		})
	}
}

// A record that cannot be read is never overwritten (the player's unlocks are not lost), and
// an old flat record is migrated with a backup of the old file next to it.
func TestGalleryUnlockRecordSafety(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gallery.json"), []byte(`{"format_version": 9, "game_id": "screen-test", "unlocks": {"cg:cg/a.png": true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := mnuGalleryGame(t, Options{SaveDir: dir, Seed: 1})
	if g.unlockReadErr == nil {
		t.Fatal("a record of a newer format was accepted")
	}
	before, _ := os.ReadFile(filepath.Join(dir, "gallery.json"))
	g.unlock("cg:cg/c.png")
	after, _ := os.ReadFile(filepath.Join(dir, "gallery.json"))
	if string(before) != string(after) {
		t.Fatalf("the unreadable record was overwritten: %s", after)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	dir = t.TempDir()
	legacy := `{"cg:cg/a.png": true, "scene:old third": true}`
	if err := os.WriteFile(filepath.Join(dir, "gallery.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	g = mnuGalleryGame(t, Options{SaveDir: dir, Seed: 1})
	if !g.unlockNeedsBackup || !g.unlocked["cg:cg/a.png"] {
		t.Fatalf("legacy record: backup %v unlocks %v", g.unlockNeedsBackup, g.unlocked)
	}
	third := g.db.Game.Gallery.Scenes[2].Key()
	if !g.unlocked[third] {
		t.Fatalf("the old key of the third recollection was not carried over: %v", g.unlocked)
	}
	g.unlock("cg:cg/c.png")
	if g.unlockNeedsBackup {
		t.Fatal("still marked for backup after the write")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backup string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "gallery.json.before-") {
			backup = e.Name()
		}
	}
	if backup == "" {
		t.Fatalf("no backup of the old record: %v", entries)
	}
	got, err := os.ReadFile(filepath.Join(dir, backup))
	if err != nil || string(got) != legacy {
		t.Fatalf("backup %q err %v", got, err)
	}
	var rec galleryRecord
	b, _ := os.ReadFile(filepath.Join(dir, "gallery.json"))
	if err := json.Unmarshal(b, &rec); err != nil || rec.FormatVersion != 1 || !rec.Unlocks["cg:cg/c.png"] {
		t.Fatalf("new record %s err %v", b, err)
	}
}

func TestGalleryCheckUnlocks(t *testing.T) {
	g := mnuGalleryGame(t, Options{Seed: 1})
	gal := g.db.Game.Gallery
	g.checkUnlocks()
	if g.unlocked[gal.Scenes[0].Key()] || g.unlocked["cg:cg/c.png"] || g.unlocked[gal.Scenes[2].Key()] {
		t.Fatalf("opened early: %v", g.unlocked)
	}
	g.st.Flags["met_one"] = true
	g.st.Flags["gamma_seen"] = true
	g.st.Gold = 500
	g.checkUnlocks()
	if !g.unlocked[gal.Scenes[0].Key()] || !g.unlocked["cg:cg/c.png"] {
		t.Fatalf("flags did not open: %v", g.unlocked)
	}
	if g.unlocked[gal.Scenes[2].Key()] {
		t.Fatal("gold > 500 opened at exactly 500")
	}
	g.st.Gold = 501
	g.checkUnlocks()
	if !g.unlocked[gal.Scenes[2].Key()] {
		t.Fatalf("a condition that holds did not open: %v", g.unlocked)
	}
	// a replay never opens or records anything
	n := len(g.unlocked)
	g.replaying = true
	g.unlock("cg:never")
	g.checkUnlocks()
	g.replaying = false
	if len(g.unlocked) != n {
		t.Fatal("unlocks recorded during a replay")
	}
	// a game without a state (the title) does nothing
	g.st = nil
	g.checkUnlocks()
}
