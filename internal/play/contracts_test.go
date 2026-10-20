package play

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

func gameFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.SetNRGBA(x, y, color.NRGBA{R: 80, G: 120, B: 170, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"game.yaml":       {Data: []byte(testgame.Game + "\nid: screen-test\nfonts: {regular: test.ttf}\n")},
		"data/main.yaml":  {Data: []byte(testgame.Data)},
		"assets/test.ttf": {Data: goregular.TTF}, "assets/x.png": {Data: b.Bytes()},
		"data/room.yaml": {Data: []byte(`locations:
  room:
    name: Test room
    dungeon:
      map: |
        #####
        #S..#
        #####
      tiles: {S: {start: true, facing: east}}
`)},
	}
}
func openGame(t *testing.T, opts Options) *Game {
	t.Helper()
	opts.FS = gameFiles(t)
	opts.NoSound = true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	g, err := New(opts)
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
func TestSceneTransitionAndSaveRoundTrip(t *testing.T) {
	g := openGame(t, Options{Seed: 12, Location: "camp"})
	g.Handle(event.MoveEffect{Location: "room"})
	if _, ok := g.stack[0].(*dungeonScene); !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	dungeon.Ensure(g.db, g.st)
	if moved, _ := dungeon.Forward(g.db, g.st); !moved {
		t.Fatal("did not move")
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	want := g.st.Pos
	g.Handle(event.MoveEffect{Location: "camp"})
	st, err := g.load(0)
	if err != nil {
		t.Fatal(err)
	}
	g.loadGame(st)
	if st.Pos != want || st.Location != "room" {
		t.Fatalf("restored %+v at %s", st.Pos, st.Location)
	}
	if _, ok := g.stack[0].(*dungeonScene); !ok {
		t.Fatal("load did not restore dungeon screen")
	}
}
func TestCreationFailureReleasesResources(t *testing.T) {
	fsys := gameFiles(t)
	fsys["assets/test.ttf"].Data = []byte("not a font")
	if _, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir()}); err == nil {
		t.Fatal("accepted malformed font")
	}
	g := openGame(t, Options{})
	if _, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: t.TempDir()}); err == nil {
		t.Fatal("accepted simultaneous game")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Update(); err == nil {
		t.Fatal("updated closed game")
	}
}
func TestScriptSaveIsolationAndCancellation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	g, err := New(Options{FS: gameFiles(t), NoSound: true, Script: "wait 1\nquit", ShotDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.store.Write("probe.json", []byte("{}")); err != nil || !g.storeGiven {
		t.Fatalf("script saves are not isolated in memory: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g.context = ctx
	if !errors.Is(g.Update(), context.Canceled) {
		t.Fatal("cancellation not returned")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("player directory changed: %v %v", entries, err)
	}
}
func TestNotesOptInAndBuildInfoOptOut(t *testing.T) {
	g := openGame(t, Options{Location: "camp"})
	g.in.pressed[btnMemo] = true
	m := g.memoInfo()
	g.memoResume(&m)
	if m.Version != "" || len(m.Retake) != 0 || m.Trace != "" {
		t.Fatalf("unexpected development metadata: %+v", m)
	}
	g.quickMemo("note", "A neutral test note", "human")
	if len(g.quick) != 0 {
		t.Fatal("recorded disabled notes")
	}
}
func TestSaveAndReadProgressWriteFailure(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	g := openGame(t, Options{Location: "camp"})
	g.store, g.storeGiven = gozen.DirStore(blocked), true
	if err := g.save(0); err == nil {
		t.Fatal("save failure hidden")
	}
	g.markRead(event.Say{Text: "Read progress"})
	g.flushRead()
	if g.failure == nil {
		t.Fatal("progress write failure hidden")
	}
	g.failure = nil
	g.readDirty = false
}
func TestScriptValidation(t *testing.T) {
	for _, s := range []string{"unknown", "wait -1", "press unknown", "shot ../escape", strings.Repeat("x", 70000)} {
		if _, err := parseScript(s); err == nil {
			t.Fatalf("accepted %q", s[:min(len(s), 30)])
		}
	}
	g := openGame(t, Options{Script: "quit", ShotDir: t.TempDir()})
	for range 3 {
		if err := g.Update(); errors.Is(err, ebiten.Termination) {
			return
		} else if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("script did not terminate")
}

func TestBattleCaptureWithoutBuildMetadata(t *testing.T) {
	g := openGame(t, Options{Seed: 23, Location: "camp"})
	var output bytes.Buffer
	g.tr = newTracer(&output)
	var troop string
	for id := range g.db.Troops {
		troop = id
		break
	}
	g.traceBattleCapture(troop)
	var line struct {
		Capture battleCapture `json:"capture"`
	}
	if err := json.Unmarshal(output.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if err := line.Capture.validate(g); err != nil {
		t.Fatal(err)
	}
	line.Capture.Engine = "incompatible"
	if err := line.Capture.validate(g); err == nil {
		t.Fatal("accepted incompatible playback version")
	}
}

func TestSettingsReadFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: dir}); err == nil {
		t.Fatal("settings I/O failure hidden")
	}
}

type failingTrace struct{ err error }

func (w failingTrace) Write([]byte) (int, error) { return 0, w.err }

func TestTraceWriterFailureIsReturned(t *testing.T) {
	want := errors.New("trace storage unavailable")
	g, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: t.TempDir(), Trace: failingTrace{want}})
	if err != nil {
		if !errors.Is(err, want) {
			t.Fatal(err)
		}
		return
	}
	if err := g.Update(); !errors.Is(err, want) {
		t.Fatalf("update: %v", err)
	}
	if err := g.Close(); !errors.Is(err, want) {
		t.Fatalf("close: %v", err)
	}
}
