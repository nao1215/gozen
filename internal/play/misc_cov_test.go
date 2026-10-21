package play

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestFaceOfChoosesAnExistingFace: an actor's face file is its faces folder and the face asked
// for, the default face when the asked one has no picture, nothing for actors without faces.
func TestFaceOfChoosesAnExistingFace(t *testing.T) {
	png := string(testgame.Picture(8, 8))
	g := sysOpen(t, Options{}, map[string]string{
		"data/faces.yaml":              "actors:\n  bard: {name: Bard, class: ranger, faces: chars/bard, face: normal}\n",
		"assets/chars/bard/normal.png": png, "assets/chars/bard/smile.png": png,
	})
	for _, c := range []struct{ actor, face, want string }{
		{"bard", "", "chars/bard/normal.png"},
		{"bard", "smile", "chars/bard/smile.png"},
		{"bard", "gone", "chars/bard/normal.png"},
		{"bard", "normal", "chars/bard/normal.png"},
		{"scout", "smile", ""},
		{"nobody", "smile", ""},
	} {
		if got := g.faceOf(c.actor, c.face); got != c.want {
			t.Errorf("faceOf(%s, %q) = %q, want %q", c.actor, c.face, got, c.want)
		}
	}
}

// TestSaveShotRefusesBadNames: a shot name that is not a plain file name fails the run instead
// of writing outside the shot directory.
func TestSaveShotRefusesBadNames(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	for _, name := range []string{"", ".", "..", "../up", "a/b", `a\b`} {
		g.failure = nil
		g.saveShotQuiet(nil, name)
		if g.failure == nil || !strings.Contains(g.failure.Error(), "basename") {
			t.Errorf("%q: failure %v", name, g.failure)
		}
	}
	g.failure = nil
	if g.shotDir() != "." {
		t.Fatalf("shot directory %q", g.shotDir())
	}
	g.opts.ShotDir = "out"
	if g.shotDir() != "out" {
		t.Fatalf("shot directory %q", g.shotDir())
	}
}

// TestRestoreStateRefusesADamagedSnapshot: the retry of a lost battle with a damaged snapshot
// keeps the state as it is.
func TestRestoreStateRefusesADamagedSnapshot(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Gold: 55}, nil)
	if g.restoreState([]byte("junk")) {
		t.Fatal("restored from junk")
	}
	if g.st.Gold != 55 {
		t.Fatalf("gold %d", g.st.Gold)
	}
	snap, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	g.st.Gold = 1
	if !g.restoreState(snap) || g.st.Gold != 55 {
		t.Fatalf("restore: gold %d", g.st.Gold)
	}
}

// TestWaitingSeesAConfirmationAsAChoice: for scripts, a confirmation box is a choice to answer.
func TestWaitingSeesAConfirmationAsAChoice(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	if what, _ := g.waiting(); what != "" {
		t.Fatalf("waiting for %q on a quiet field", what)
	}
	g.push(newConfirm("?", func() {}))
	if what, _ := g.waiting(); what != "choice" {
		t.Fatalf("waiting for %q at a confirmation", what)
	}
}

// TestEffectsSetTheirTimers: shake, flash and fade each start their timer; an unknown effect does
// nothing.
func TestEffectsSetTheirTimers(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	g.Effect("shake")
	g.Effect("flash")
	if g.shake != 24 || g.flash != 20 || g.fadeT != 0 {
		t.Fatalf("shake %d flash %d fade %d", g.shake, g.flash, g.fadeT)
	}
	g.Effect("fade")
	if g.fadeT != 60 {
		t.Fatalf("fade %d", g.fadeT)
	}
	g.Effect("nothing")
	if g.shake != 24 || g.flash != 20 || g.fadeT != 60 {
		t.Fatal("an unknown effect changed something")
	}
	// the timers run down by themselves.
	sysFrames(t, g, 70)
	if g.shake != 0 || g.flash != 0 || g.fadeT != 0 {
		t.Fatalf("timers did not run out: %d %d %d", g.shake, g.flash, g.fadeT)
	}
}

// TestAudioWriterFailureEndsTheRun: audio that cannot be recorded is an error of the next update.
func TestAudioWriterFailureEndsTheRun(t *testing.T) {
	want := errors.New("audio sink gone")
	g := sysOpen(t, Options{Location: "camp", Audio: failingTrace{err: want}}, nil)
	var err error
	for range 3 {
		if err = g.Update(); err != nil {
			break
		}
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	g.failure = nil
}

// TestImageOverIsFittedToTheScreen: a picture laid over a CG is limited to 80% of the screen's
// width, whatever size was asked.
func TestImageOverIsFittedToTheScreen(t *testing.T) {
	events := `events:
  over:
    do:
      - image: {file: x.png, over: {file: x.png}, over_size: 3}
      - say: "big"
      - image: {file: x.png, over: {file: x.png}}
      - say: "default"
`
	g := sysOpen4(t, Options{Location: "camp", Event: "over", Line: 1, Script: "read 3\nread 3\nread 3\nquit"}, map[string]string{"data/over.yaml": events})
	got := sysPlayLines(t, g, 3000)
	if got["big"].cg != "x.png" || got["default"].cg != "x.png" {
		t.Fatalf("pictures %+v", got)
	}
	if g.cg.Over.File != "x.png" {
		t.Fatalf("over %q", g.cg.Over.File)
	}
}

// TestImageStandInKeepsTheFaces: a CG listed as a stand-in keeps the speakers' pictures over
// it, and a picture that fades away over a fade of its own takes that time.
func TestImageStandInKeepsTheFaces(t *testing.T) {
	events := `events:
  stand:
    do:
      - image: {file: x.png}
      - say: "stand in"
      - image: {file: x.png}
      - say: "same"
      - image: {file: "", fade: 2}
      - say: "gone"
`
	g := sysOpen4(t, Options{Location: "camp", Event: "stand", Line: 1, Script: "read 3\nread 3\nread 3\nquit"},
		map[string]string{"data/stand.yaml": events, "game.yaml+": "stand_ins: stand.txt\n", "assets/stand.txt": "x.png\n"})
	got := sysPlayLines(t, g, 3000)
	if len(got) != 3 {
		t.Fatalf("lines %v", got)
	}
	if got["gone"].cg != "" {
		t.Fatalf("picture after fading out: %q", got["gone"].cg)
	}
}

// TestMoveBetweenDungeonsReusesTheScreen: leaving a dungeon for another keeps the dungeon screen.
func TestMoveBetweenDungeonsReusesTheScreen(t *testing.T) {
	g := sysOpen(t, Options{Location: "hall"}, map[string]string{"data/hall.yaml": sysCorridor})
	first := g.stack[0]
	g.Move("room")
	if g.stack[0] != first {
		t.Fatal("a new dungeon screen was made for the next dungeon")
	}
	if g.st.Location != "room" {
		t.Fatalf("at %s", g.st.Location)
	}
	// the screen is the dungeon's, not the field's, after coming back.
	g.Move("camp")
	if _, ok := g.stack[0].(*field); !ok {
		t.Fatalf("camp is on %T", g.stack[0])
	}
}

// TestNewGameRunsTheStartEvent: choosing "new game" runs the event game.yaml names as the start.
func TestNewGameRunsTheStartEvent(t *testing.T) {
	fsys := gameFiles(t)
	d := strings.Replace(string(fsys["game.yaml"].Data), "start: {location: camp, party: [scout], gold: 100, items: {potion: 2}}",
		"start: {location: camp, party: [scout], gold: 100, items: {potion: 2}, event: sub}", 1)
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte(d)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer sysClose(t, g)
	g.newGame()
	if !g.events.Running() {
		t.Fatal("the start event did not start")
	}
	sysFrames(t, g, 10)
	if g.st.Gold != 105 {
		t.Fatalf("gold %d, want 105", g.st.Gold)
	}
}
