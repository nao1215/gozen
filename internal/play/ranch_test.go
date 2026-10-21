package play

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

func ranchGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	for path, body := range testgame.MonsterFiles() {
		fsys[path] = &fstest.MapFile{Data: []byte(body)}
	}
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte(testgame.MonsterFiles()["game.yaml"] + "\nfonts: {regular: test.ttf}\n")}
	g, err := New(Options{FS: fsys, NoSound: true, Location: "camp", SaveDir: t.TempDir(), ShotDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

func TestRanchPreviewAndFusionCommand(t *testing.T) {
	g := ranchGame(t)
	a, _ := g.st.Recruit("mote", 2, 0, nil)
	b, _ := g.st.Recruit("mote", 2, 0, nil)
	r := newRanch(g, nil)
	g.push(r)
	r.show(g, "breed_first")
	if len(r.options) != 2 {
		t.Fatalf("parents = %v", r.options)
	}
	r.list.sel = 0
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if r.first != a || r.step != "breed_second" {
		t.Fatalf("first %s step %s", r.first, r.step)
	}
	r.list.sel = 0
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if r.second != b || r.step != "skills" {
		t.Fatalf("second %s step %s", r.second, r.step)
	}
	if !strings.Contains(strings.Join(r.options, ","), "fire") {
		t.Fatalf("inheritable skills = %v", r.options)
	}
	for i, id := range r.options {
		if id == "fire" {
			r.list.sel = i
			break
		}
	}
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if len(r.skills) != 1 || r.skills[0] != "fire" {
		t.Fatalf("chosen skills = %v", r.skills)
	}
	r.list.sel = len(r.options) - 1
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	c, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok || !strings.Contains(c.msg, "?") {
		t.Fatalf("hidden preview = %T %+v", g.stack[len(g.stack)-1], c)
	}
	g.pop()
	c.yes()
	for i := 0; g.events.Running() && i < 20; i++ {
		g.events.Next()
	}
	if g.st.Actors[a] != nil || g.st.Actors[b] != nil || g.st.Vars["bestiary.boss.caught"] != float64(1) {
		t.Fatalf("fusion state = %+v", g.st.Actors)
	}
}
