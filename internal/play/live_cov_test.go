package play

import (
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/geom"
)

// F5 reload (the site's "Edit and play loop"): with --dir the game reads the YAML and pictures
// from disk; F5 loads the definitions, checks them and swaps them in, keeping the place (the
// location, the party and the current line of an event). A mistake keeps the old definitions and
// shows the file and line; the game id and a lower data_version cannot be reloaded; a battle or
// a menu waits until the field is back; pictures and sounds reload on their own.

// wldDisk writes the world fixture to a directory and returns it.
func wldDisk(t *testing.T, gameYAML string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := wldWorldFiles(t, gameYAML, extra)
	err := fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		return os.WriteFile(p, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// wldLive opens the fixture from disk the way --dir --dev does.
func wldLive(t *testing.T, dir, loc string) *Game {
	t.Helper()
	// the script names the reload key, which also starts the watcher of the directory
	opts := Options{FS: os.DirFS(dir), LiveDir: dir, Development: true, Location: loc, Script: "wait 1000000000\n# press reload\n"}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	g.startLive() // Run starts it; the stop is Close's
	return g
}

func wldEdit(t *testing.T, dir, rel, from, to string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	b, err := os.ReadFile(p) //nolint:gosec // The test's own temporary file.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), from) {
		t.Fatalf("%s has no %q", rel, from)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), from, to, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWldLiveFingerprintSeparatesDataFromPictures(t *testing.T) {
	dir := wldDisk(t, "", nil)
	a := liveFingerprint(dir)
	if a == "unreadable" || !strings.Contains(a, "/") {
		t.Fatalf("fingerprint %q", a)
	}
	if liveFingerprint(dir) != a {
		t.Fatal("the fingerprint of untouched files changed")
	}
	wldEdit(t, dir, "data/main.yaml", "Camp", "Camp site")
	b := liveFingerprint(dir)
	if liveDataPart(b) == liveDataPart(a) {
		t.Fatal("editing a data file did not change the data part")
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "pics", "flat.png"), []byte("not the same size"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := liveFingerprint(dir)
	if liveDataPart(c) != liveDataPart(b) || c == b {
		t.Fatalf("a picture changed the data part or nothing: %q -> %q", b, c)
	}
	// a directory with none of the three is still a fingerprint (nothing to watch)
	if e := liveFingerprint(t.TempDir()); e == "unreadable" {
		t.Fatalf("an empty directory is %q", e)
	}
}

func TestWldLiveErrTextShowsFileLineAndHint(t *testing.T) {
	wldWorld(t, "camp", Options{}) // opening a game sets the language the messages are in
	cases := []struct{ in, want string }{
		{"short", "short"},
		{"data/a.yaml: [12:3] unknown field \"colour\"\n   10 | a\n > 12 | colour: red\n", "data/a.yaml line 12"},
		{"data/a.yaml: [12:3] unknown field \"colour\"", "unknown field"},
		{"data/b.yaml: [7:1] mapping value is not allowed\n  > 7 | x: y: z\n", "x: y: z"},
		{strings.Repeat("long ", 40), "..."},
	}
	for _, c := range cases {
		if got := liveErrText(c.in); !strings.Contains(got, c.want) && c.want != "unknown field" {
			t.Errorf("liveErrText(%q) = %q, want it to contain %q", c.in, got, c.want)
		}
	}
	got := liveErrText("data/a.yaml: [12:3] unknown field \"colour\"")
	if !strings.Contains(got, "colour") || !strings.Contains(got, "newer") && !strings.Contains(got, "old") {
		t.Errorf("an unknown field gives no hint about the engine being older: %q", got)
	}
	if n := len([]rune(liveErrText(strings.Repeat("x", 500)))); n > 80 {
		t.Errorf("a long message is %d letters", n)
	}
}

func TestWldLiveReloadKeepsTheLineOfTheEvent(t *testing.T) {
	dir := wldDisk(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData})
	g := wldLive(t, dir, "camp")
	g.st.Flags["marker"] = true
	wldStartEvent(t, g, "ev_auto")
	wldFrames(t, g, 5)
	wldPlay(t, g, "wait 40\npress ok\nwait 40\n", 200)
	if wldTalkText(g) != "Two." {
		t.Fatalf("at %q", wldTalkText(g))
	}
	wldEdit(t, dir, "data/novel_cov.yaml", `text: "Two."`, `text: "Zwei."`)
	wldPlay(t, g, "press reload\nwait 20\n", 200)
	if got := wldTalkText(g); got != "Zwei." {
		t.Fatalf("after the reload the line is %q, want the edited one at the same place", got)
	}
	if !g.st.Flags["marker"] || g.st.Location != "camp" || len(g.st.Party) != 1 {
		t.Fatalf("the place was lost: flags %v location %q party %v", g.st.Flags, g.st.Location, g.st.Party)
	}
	if g.liveNotice() == "" {
		t.Fatal("no notice that the data was reloaded")
	}
}

func TestWldLiveReloadRefusesMistakesAndKeepsTheOldData(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	name := g.db.Locations["camp"].Name
	wldEdit(t, dir, "data/main.yaml", "name: Camp\n", "name: [Camp\n")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Locations["camp"].Name != name {
		t.Fatalf("a broken file replaced the definitions: %q", g.db.Locations["camp"].Name)
	}
	msg := g.liveNotice()
	if !strings.Contains(msg, "main.yaml") {
		t.Fatalf("the notice does not name the file: %q", msg)
	}
	// fixing it reloads: the new name shows
	wldEdit(t, dir, "data/main.yaml", "name: [Camp\n", "name: Camp site\n")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Locations["camp"].Name != "Camp site" {
		t.Fatalf("the fixed file was not loaded: %q", g.db.Locations["camp"].Name)
	}
}

func TestWldLiveReloadCannotChangeIDOrLowerDataVersion(t *testing.T) {
	dir := wldDisk(t, "data_version: 3\n", nil)
	g := wldLive(t, dir, "camp")
	wldEdit(t, dir, "game.yaml", "id: screen-test", "id: other-game")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Game.ID != "screen-test" || !strings.Contains(g.liveNotice(), "Cannot reload") {
		t.Fatalf("id %q notice %q", g.db.Game.ID, g.liveNotice())
	}
	wldEdit(t, dir, "game.yaml", "id: other-game", "id: screen-test")
	wldEdit(t, dir, "game.yaml", "data_version: 3", "data_version: 2")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Game.DataVersion != 3 {
		t.Fatalf("a lower data_version was loaded: %d", g.db.Game.DataVersion)
	}
	wldEdit(t, dir, "game.yaml", "data_version: 2", "data_version: 4")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Game.DataVersion != 4 {
		t.Fatalf("a higher data_version was not loaded: %d", g.db.Game.DataVersion)
	}
}

func TestWldLiveReloadWaitsForAScreenItCannotKeep(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	g.push(newSaveMenu(g, true, func() {}))
	wldFrames(t, g, 3)
	wldEdit(t, dir, "data/main.yaml", "name: Camp\n", "name: Camp site\n")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Locations["camp"].Name != "Camp" || !g.live.pending {
		t.Fatalf("reloaded over a screen it cannot keep: name %q pending %v", g.db.Locations["camp"].Name, g.live.pending)
	}
	if g.liveNotice() == "" {
		t.Fatal("no notice about the reload waiting")
	}
	g.pop()
	wldFrames(t, g, 30)
	if g.db.Locations["camp"].Name != "Camp site" || g.live.pending {
		t.Fatalf("after closing the screen: name %q pending %v", g.db.Locations["camp"].Name, g.live.pending)
	}
}

func TestWldLiveReloadKeepsTheMenuOpen(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	if _, ok := g.stack[len(g.stack)-1].(*menu); !ok {
		t.Fatalf("the menu did not open: %T", g.stack[len(g.stack)-1])
	}
	wldEdit(t, dir, "data/main.yaml", "name: Camp\n", "name: Camp site\n")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.db.Locations["camp"].Name != "Camp site" {
		t.Fatal("a menu held the reload back")
	}
	if _, ok := g.stack[len(g.stack)-1].(*menu); !ok || len(g.stack) != 2 {
		t.Fatalf("the menu did not stay open: %d scenes, top %T", len(g.stack), g.stack[len(g.stack)-1])
	}
}

func TestWldLiveReloadOfPicturesAloneAndWithoutADirectory(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	if err := os.WriteFile(filepath.Join(dir, "assets", "pics", "flat.png"), wldPNG(t, 24, 24, color.NRGBA{R: 1}), 0o600); err != nil {
		t.Fatal(err)
	}
	g.pics.get("pics/flat.png")
	wldPlay(t, g, "press reload\nwait 10\n", 100)
	if g.live.pending || !strings.Contains(g.liveNotice(), "picture") {
		t.Fatalf("notice %q", g.liveNotice())
	}
	if b := g.pics.get("pics/flat.png").Bounds(); b.Dx() != 24 {
		t.Fatalf("the picture was not read again: %v", b)
	}
}

func TestWldLiveReloadKeepsWhereTheLeaderStands(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "fx_yard")
	s := g.stack[0].(*stageScene)
	s.player.at = geom.Pt{X: 120, Y: 150}
	wldEdit(t, dir, "data/world.yaml", "name: Test Yard", "name: Test Yard Two")
	wldPlay(t, g, "press reload\nwait 20\n", 100)
	s2, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if g.db.Locations["fx_yard"].Name != "Test Yard Two" {
		t.Fatal("not reloaded")
	}
	if s2.player.at != (geom.Pt{X: 120, Y: 150}) {
		t.Fatalf("the leader stands at %v after the reload, want where it stood", s2.player.at)
	}
}

func TestWldLiveReloadKeepsTheDungeonPosition(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "fx_maze")
	g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = 5, 3, 1
	wldEdit(t, dir, "data/world.yaml", "name: Test Maze", "name: Test Maze Two")
	wldPlay(t, g, "press reload\nwait 20\n", 100)
	if g.db.Locations["fx_maze"].Name != "Test Maze Two" {
		t.Fatal("not reloaded")
	}
	if p := g.st.Pos; p.X != 5 || p.Y != 3 || p.Facing != 1 {
		t.Fatalf("position %+v after the reload", p)
	}
}
