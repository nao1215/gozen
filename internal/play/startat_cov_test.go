package play

import (
	"strings"
	"testing"
	"testing/fstest"
)

const sysGallery = "gallery: {scenes: [{title: Intro, event: story, flag: seen_story}]}\n"

func sysRunFast(t *testing.T, g *Game, limit int) {
	t.Helper()
	for range limit {
		if g.fast == nil {
			return
		}
		sysFrames(t, g, 1)
	}
	t.Fatal("reading on never stopped")
}

// TestStartAtEventReadsOnToTheLine: --event with --line reads the event on and stops at that line.
func TestStartAtEventReadsOnToTheLine(t *testing.T) {
	for _, c := range []struct {
		line int
		want string
	}{{1, "The lamps are lit."}, {2, "Gold 100."}, {3, "A bell rings far off."}} {
		g := sysNovel(t, Options{Seed: 1, Location: "camp", Event: "story", Line: c.line})
		sysRunFast(t, g, 3000)
		if !g.talk.active || g.talk.say.Text != c.want || g.evLines != c.line {
			t.Fatalf("line %d: stopped at %q (line %d)", c.line, g.talk.say.Text, g.evLines)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStartAtEventAnswersTheChoices: --answers picks the options on the way.
func TestStartAtEventAnswersTheChoices(t *testing.T) {
	g := sysNovel(t, Options{Seed: 1, Location: "camp", Event: "story", Line: 5, Answers: []int{1}})
	sysRunFast(t, g, 5000)
	if len(g.evAnswers) != 1 || g.evAnswers[0] != 1 {
		t.Fatalf("answers %v", g.evAnswers)
	}
	if !g.talk.active || g.evLines != 5 {
		t.Fatalf("stopped at line %d: %q", g.evLines, g.talk.say.Text)
	}
}

// TestStartAtEventWithNothingToRead: an event with no lines stops reading on at once.
func TestStartAtEventWithNothingToRead(t *testing.T) {
	g := sysOpen(t, Options{Seed: 1, Location: "camp", Event: "sub", Line: 4}, nil)
	sysRunFast(t, g, 500)
	if g.st.Gold != 105 {
		t.Fatalf("gold %d", g.st.Gold)
	}
}

// TestStartAtUnknownThings: an unknown event or scene is an error that names it.
func TestStartAtUnknownThings(t *testing.T) {
	for _, c := range []struct {
		o    Options
		want string
	}{
		{Options{Event: "nope"}, `unknown event "nope"`},
		{Options{Scene: "nope"}, `unknown gallery scene "nope"`},
		{Options{Scene: "9"}, `unknown gallery scene "9"`},
	} {
		fsys := gameFiles(t)
		fsys["game.yaml"].Data = append(fsys["game.yaml"].Data, sysGallery...)
		fsys["data/novel.yaml"] = &fstest.MapFile{Data: []byte(novelData)}
		c.o.FS, c.o.NoSound, c.o.SaveDir = fsys, true, t.TempDir()
		g, err := New(c.o)
		if err == nil {
			sysClose(t, g)
			t.Errorf("%+v accepted", c.o)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("error %q does not mention %q", err, c.want)
		}
	}
}

// TestStartAtGalleryScene: --scene replays a recollection by title or by number.
func TestStartAtGalleryScene(t *testing.T) {
	for _, scene := range []string{"Intro", "1"} {
		g := sysOpen4(t, Options{Seed: 1, Scene: scene, Line: 2}, map[string]string{"data/novel.yaml": novelData, "game.yaml+": sysGallery})
		if _, ok := g.stack[len(g.stack)-1].(*sceneReplay); !ok {
			t.Fatalf("scene %q: top is %T", scene, g.stack[len(g.stack)-1])
		}
		if g.fast == nil {
			t.Fatalf("scene %q: not reading on", scene)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
