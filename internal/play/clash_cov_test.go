package play

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"
)

// clashFiles is the campaign of internal/play/testdata/campaign (a march with fight: clash_view) with
// the Go font and the YAML of the clash replaced by repl (old -> new).
func clashFiles(t *testing.T, repl map[string]string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	root := os.DirFS("testdata/campaign")
	for _, name := range []string{"game.yaml", "data/main.yaml", "assets/guard.png", "assets/hero/normal.png", "assets/keep.png", "assets/land.png"} {
		b, err := fs.ReadFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for old, nw := range repl {
			s = strings.ReplaceAll(s, old, nw)
		}
		fsys[name] = &fstest.MapFile{Data: []byte(s)}
	}
	fsys["assets/regular.ttf"] = &fstest.MapFile{Data: goregular.TTF}
	return fsys
}

// clashPlay marches the hero on the keep by the clash and plays the clash out, drawing some
// of its frames, and reports the scene it ended in.
func clashPlay(t *testing.T, repl map[string]string) *Game {
	t.Helper()
	g := wldOpenWith(t, Options{FS: clashFiles(t, repl), Location: "war"})
	wldFrames(t, g, 40)
	wldPlay(t, g, "order storm - keep\nwait 30\npress cancel\nwait 20\norder storm hero keep\nwait 60\n", 400)
	screen := wldScreen(g)
	for i := range 80 {
		wldPlay(t, g, "wait 20\n", 100)
		g.Draw(screen)
		if i == 3 || i >= 20 { // OK plays faster, and ends the table
			wldPlay(t, g, "press ok\nwait 3\n", 100)
		}
	}
	return g
}

func TestClashRanksPlaysOut(t *testing.T) {
	covSlow(t)
	g := clashPlay(t, nil)
	if _, ok := g.stack[len(g.stack)-1].(*clashScene); ok {
		t.Fatal("the clash did not end")
	}
}

func clashArmiesPlay(t *testing.T, layout string) {
	g := clashPlay(t, map[string]string{
		"clash: {power: \"a.troops\"}":              "clash: {power: \"a.troops\", layout: " + layout + ", max_soldiers: 6, armies: {men: [guard.png, keep.png], monsters: [guard.png]}, rope: true, skills: {volley: {label: Volley, charge: 1, all: true, power: \"a.troops * 0.1\", cutin: true}, rally: {label: Rally, charge: 1, heal: \"a.max * 0.2\"}}}",
		"generals: {hero: {troops: 900, might: 1}}": "generals: {hero: {troops: 900, might: 1, army: men, skills: [volley, rally]}}",
		"guard: {name: Guard, image: guard.png,":    "guard: {name: Guard, army: monsters, image: guard.png,",
	})
	if _, ok := g.stack[len(g.stack)-1].(*clashScene); ok {
		t.Fatalf("%s: the clash did not end", layout)
	}
}

func TestClashArmiesBelowPlaysOut(t *testing.T) {
	covSlow(t)
	clashArmiesPlay(t, "below")
}

func TestClashArmiesFrontPlaysOut(t *testing.T) {
	covSlow(t)
	clashArmiesPlay(t, "front")
}
