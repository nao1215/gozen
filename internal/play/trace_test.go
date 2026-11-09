package play

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/traceformat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
	"github.com/nao1215/gozen/internal/savefile"
	"github.com/nao1215/gozen/internal/testgame"
)

// More of what a trace records: the inputs with the mouse, the kinds of requests, the scene
// stack, and what a replay cannot take (a game started with options it cannot reproduce).

// wldAnySlice reads a JSON array.
func wldAnySlice(t *testing.T, v any) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%v is not a list", v)
	}
	return a
}

func wldTracedGame(t *testing.T, script string) (*Game, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts := Options{Location: "camp", Seed: 5, Trace: &buf, Script: script, FS: wldWorldFiles(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData})}
	return wldOpenWith(t, opts), &buf
}

func TestTraceButtonsRoundTrip(t *testing.T) {
	seen := make(map[string]button)
	for b := range numButtons {
		name := buttonNames[b]
		if name == "" {
			t.Fatalf("button %d has no trace name", b)
		}
		if previous, ok := seen[name]; ok {
			t.Fatalf("buttons %d and %d share trace name %q", previous, b, name)
		}
		seen[name] = b
		for _, field := range []string{"press", "held"} {
			t.Run(name+"/"+field, func(t *testing.T) {
				var buf bytes.Buffer
				g := &Game{tr: newTracer(&buf), tick: 1}
				var want input
				if field == "press" {
					want.pressed[b] = true
				} else {
					want.held[b] = true
				}
				g.traceInput(&want)
				path := filepath.Join(t.TempDir(), "trace.jsonl")
				data := append([]byte(`{"t":"start","tick":0,"version":1,"exact_version":3}`+"\n"), buf.Bytes()...)
				data = append(data, []byte(`{"t":"end","tick":2}`+"\n")...)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				r, err := loadReplay(path, &Options{Until: 2})
				if err != nil {
					t.Fatal(err)
				}
				var got input
				r.feed(&got, 1)
				if got.pressed != want.pressed || got.held != want.held {
					t.Fatalf("button %d round trip: pressed %v, held %v", b, got.pressed, got.held)
				}
				r.feed(&got, 2)
				if got.pressed != [numButtons]bool{} || got.held != [numButtons]bool{} {
					t.Fatal("button remained active on an empty tick")
				}
			})
		}
	}
}

func TestReplayRejectsEmptyButtonNames(t *testing.T) {
	for _, field := range []string{"press", "held"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.jsonl")
			data := fmt.Sprintf("{\"t\":\"start\",\"tick\":0,\"version\":1,\"exact_version\":3}\n{\"t\":\"input\",\"tick\":1,%q:[\"\"]}\n{\"t\":\"end\",\"tick\":2}\n", field)
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadReplay(path, &Options{Until: 2})
			if err == nil || !strings.Contains(err.Error(), "empty button name") || !strings.Contains(err.Error(), "record a new trace") {
				t.Fatalf("old trace diagnostic: %v", err)
			}
		})
	}
}

func TestTraceMapZoomReproduction(t *testing.T) {
	fsys := atlasCameraFiles(t)
	var buf bytes.Buffer
	g := wldOpenWith(t, Options{FS: fsys, Location: "large", Trace: &buf, Script: "wait 5\npress map_zoom_in\nwait 10\n"})
	want, ok := g.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("recorded scene %T, want map", g.stack[0])
	}
	initial := want.cameraZoom
	for range 15 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if want.cameraZoom <= initial {
		t.Fatal("recording did not zoom the map")
	}
	g.traceEnd()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	r := wldOpenWith(t, Options{FS: fsys, Replay: path, Until: g.tick})
	for range 15 {
		if err := r.Update(); errors.Is(err, errQuit) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	got, ok := r.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("replayed scene %T, want map", r.stack[0])
	}
	wk, wx, wy, _ := want.fit(g)
	gk, gx, gy, _ := got.fit(r)
	if got.cameraZoom != want.cameraZoom || gk != wk || gx != wx || gy != wy || len(r.stack) != 1 {
		t.Fatalf("map view was not reproduced: zoom %g, want %g", got.cameraZoom, want.cameraZoom)
	}
}

func TestWldTraceInputLinesCarryMouseDigitsAndFast(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 3\nclick 120 340\nwait 10\npress 3\nwait 5\nskip 4\nwait 5\nquit\n")
	// Inject raw values to test serialization, including digits unused by the field.
	// Receiver acknowledgement is covered by the script integration tests.
	g.in.script.acknowledge = false
	screen := wldScreen(g)
	for i := 0; i < 500 && wldStep(t, g, screen); i++ {
	}
	var click, digit, fast traceLine
	for _, l := range traceLines(t, buf.Bytes(), "input") {
		switch {
		case l["click"] == true:
			click = l
		case l["digit"] != nil:
			digit = l
		case l["fast"] == true:
			fast = l
		}
	}
	if m, ok := click["mouse"].([]any); !ok || len(m) != 2 || m[0] != 120.0 || m[1] != 340.0 {
		t.Fatalf("click line %v", click)
	}
	if digit["digit"] != 3.0 {
		t.Fatalf("digit line %v", digit)
	}
	if fast == nil {
		t.Fatal("no line for the held fast-forward")
	}
}

func TestWldTraceRequestsHaveTheirKinds(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 1000000\n")
	wldFrames(t, g, 3)
	g.traceRequest(event.Say{Name: "Ann", Face: "pics/tall1.png", Text: "Hi."})
	g.traceRequest(event.Say{Text: "Narration."})
	g.traceRequest(event.Choice{Prompt: "Which?", Options: []string{"A", "B"}})
	g.traceRequest(event.Notice{Text: "Careful."})
	g.traceRequest(event.Battle{Troop: "one"})
	g.traceRequest(event.Shop{ID: "store"})
	g.traceRequest(event.Save{})
	g.traceRequest(event.Open{Screen: "skills"})
	g.traceRequest(event.Clear{})
	g.traceRequest(event.GameOver{})
	say := traceLines(t, buf.Bytes(), "say")
	if len(say) < 3 {
		t.Fatalf("say lines %v", say)
	}
	last := say[len(say)-3:]
	if last[0]["who"] != "Ann" || last[0]["face"] != "pics/tall1.png" || last[0]["text"] != "Hi." || last[0]["read"] != false {
		t.Fatalf("say %v", last[0])
	}
	if _, has := last[1]["who"]; has || last[1]["text"] != "Narration." {
		t.Fatalf("narration %v", last[1])
	}
	if c, ok := last[2]["choices"].([]any); !ok || len(c) != 2 || last[2]["text"] != "Which?" {
		t.Fatalf("choice %v", last[2])
	}
	if n := traceLines(t, buf.Bytes(), "notice"); len(n) != 1 || n[0]["text"] != "Careful." {
		t.Fatalf("notice %v", n)
	}
	var whats []string
	for _, l := range traceLines(t, buf.Bytes(), "request") {
		w := fmt.Sprint(l["what"])
		switch w {
		case "battle":
			w += ":" + fmt.Sprint(l["troop"])
		case "shop":
			w += ":" + fmt.Sprint(l["id"])
		case "open":
			w += ":" + fmt.Sprint(l["screen"])
		}
		whats = append(whats, w)
	}
	if got := strings.Join(whats, ","); got != "battle:one,shop:store,save,open:skills,clear,game_over" {
		t.Fatalf("requests %s", got)
	}
}

func TestWldTraceSceneLineNamesTheStack(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 20\npress menu\nwait 20\npress cancel\nwait 20\nquit\n")
	screen := wldScreen(g)
	for i := 0; i < 500 && wldStep(t, g, screen); i++ {
	}
	var stacks []string
	for _, l := range traceLines(t, buf.Bytes(), "scene") {
		var names []string
		for _, n := range wldAnySlice(t, l["stack"]) {
			names = append(names, fmt.Sprint(n))
		}
		stacks = append(stacks, strings.Join(names, ">"))
	}
	if len(stacks) < 3 || stacks[0] != "field" || stacks[1] != "field>menu" || stacks[2] != "field" {
		t.Fatalf("scene stacks %v", stacks)
	}
}

func TestWldTraceViewLinesNameThePicturesDrawn(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 1000000\n")
	wldStartEvent(t, g, "ev_auto")
	for range 40 { // every frame drawn: the view line is written by a draw
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		g.Draw(wldScreen(g))
	}
	views := traceLines(t, buf.Bytes(), "view")
	if len(views) == 0 {
		t.Fatal("no view line after the picture stood still")
	}
	var found bool
	for _, v := range views {
		for _, p := range wldAnySlice(t, v["pictures"]) {
			if p == "pics/tall1.png" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("views %v", views)
	}
}

func TestWldReplayRefusesReviewScreen(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{
		Location: "camp", Seed: 5, Trace: &buf, Review: true, ReviewLog: t.TempDir() + "/review.jsonl",
		Script: "wait 5\nquit\n", FS: wldWorldFiles(t, "", nil),
	}
	g := wldOpenWith(t, opts)
	screen := wldScreen(g)
	for i := 0; i < 100 && wldStep(t, g, screen); i++ {
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := wldReplayGame(t, buf.Bytes(), func(o *Options) { o.FS = wldWorldFiles(t, "", nil); o.Until = 1 })
	if err == nil || !strings.Contains(err.Error(), "cg_review at tick 0") {
		t.Fatalf("error %v", err)
	}
}

func TestWldTraceStartHashesTheSavesItStartedWith(t *testing.T) {
	store := savefile.MemStore()
	if err := store.Write("save1.json", []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	opts := Options{Location: "camp", Seed: 5, Trace: &buf, Store: store, Script: "wait 5\nquit\n", FS: wldWorldFiles(t, "", nil)}
	g := wldOpenWith(t, opts)
	screen := wldScreen(g)
	for i := 0; i < 100 && wldStep(t, g, screen); i++ {
	}
	start := traceLines(t, buf.Bytes(), "start")[0]
	setup, ok := start["setup"].(map[string]any)
	if !ok {
		t.Fatalf("setup %v", start["setup"])
	}
	saves, ok := setup["saves"].(map[string]any)
	if !ok {
		t.Fatalf("saves %v", setup["saves"])
	}
	if len(saves) != 1 {
		t.Fatalf("saves in the setup %v", saves)
	}
	for name, sum := range saves {
		if want := fmt.Sprintf("%x", sha256.Sum256([]byte(`{"x":1}`))); name != "save1.json" || sum != want {
			t.Fatalf("save %q hashed as %v", name, sum)
		}
	}
}

// wldBadWriter fails (or writes short) from the write after the first left ones.
type wldBadWriter struct {
	left  int
	short bool
}

func (w *wldBadWriter) Write(p []byte) (int, error) {
	if w.left > 0 {
		w.left--
		return len(p), nil
	}
	if w.short {
		return len(p) / 2, nil
	}
	return 0, errWldDiskFull
}

var errWldDiskFull = errors.New("disk full")

// A trace that cannot be written stops the game with the writer's error: a record with a hole
// in it would replay wrongly, so it is not kept going.
func TestWldTraceWriteFailureStopsTheGame(t *testing.T) {
	for _, c := range []struct {
		name string
		w    *wldBadWriter
		want error
	}{
		{"a full disk", &wldBadWriter{left: 3}, errWldDiskFull},
		{"a short write", &wldBadWriter{left: 3, short: true}, io.ErrShortWrite},
		{"a full disk at once", &wldBadWriter{}, errWldDiskFull},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := Options{Location: "camp", Seed: 5, Trace: c.w, Script: "wait 1000\n", FS: wldWorldFiles(t, "", nil)}
			opts.NoSound, opts.SaveDir, opts.ShotDir = true, t.TempDir(), t.TempDir()
			g, err := New(opts)
			if err != nil {
				if !errors.Is(err, c.want) {
					t.Fatalf("New failed with %v, want %v", err, c.want)
				}
				return
			}
			defer func() {
				if err := g.Close(); err != nil && !errors.Is(err, c.want) {
					t.Error(err)
				}
			}()
			screen := wldScreen(g)
			for range 3000 {
				if err := g.Update(); err != nil {
					if !errors.Is(err, c.want) {
						t.Fatalf("Update failed with %v, want %v", err, c.want)
					}
					return
				}
				g.Draw(screen)
			}
			t.Fatal("the game went on though its trace could not be written")
		})
	}
}

// The stand lines say what hides the portraits: a picture over the stage, and whether that picture
// is the background of a place.
func TestWldTraceStandLinesNameThePictureOverThePortraits(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 1000000\n")
	wldStartEvent(t, g, "ev_cgstand")
	wldFramesDrawn(t, g, 60)
	wldPlay(t, g, "press ok\nwait 90\n", 200)
	wldFramesDrawn(t, g, 60)
	var over traceLine
	for _, l := range traceLines(t, buf.Bytes(), "stand") {
		if l["cg"] == "pics/bg.png" {
			over = l
		}
	}
	if over == nil {
		t.Fatalf("no stand line with the picture over the stage: %v", traceLines(t, buf.Bytes(), "stand"))
	}
	if over["hides"] != true || over["cg_bg"] != true || over["cg_page"] == nil {
		t.Fatalf("stand line %v: the picture hides the portraits and is a place's background", over)
	}
	wldPlay(t, g, "press ok\nwait 90\n", 200)
	wldFramesDrawn(t, g, 60)
	last := traceLines(t, buf.Bytes(), "stand")
	if l := last[len(last)-1]; l["cg"] != nil || l["hides"] != false {
		t.Fatalf("after the picture went: %v", l)
	}
}

func TestTraceReproRestoresSavesAndMemo(t *testing.T) {
	g := wldOpenWith(t, Options{Location: "camp", Seed: 7, FS: wldWorldFiles(t, "", nil)})
	g.st.Gold = 123
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "initial.save")
	if err := os.WriteFile(path, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &replayStore{Store: savefile.MemStore(), times: map[string]time.Time{"save1.json": time.Unix(123, 0)}}
	if err := store.Store.Write("save1.json", saved); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	g = wldOpenWith(t, Options{Seed: 7, FS: wldWorldFiles(t, "", nil), FromMemo: path, Store: store, Trace: &buf, Script: "wait 5\nquit\n"})
	for range 30 {
		if !wldStep(t, g, wldScreen(g)) {
			break
		}
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "run.jsonl")
	if err := os.WriteFile(trace, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	g = wldOpenWith(t, Options{FS: wldWorldFiles(t, "", nil), Replay: trace, Store: savefile.MemStore()})
	if g.st.Gold != 123 {
		t.Fatalf("restored gold %d", g.st.Gold)
	}
	got, err := g.storeRead("save1.json")
	if err != nil || !bytes.Equal(got, saved) {
		t.Fatalf("save: %s, %v", got, err)
	}
	if got, ok := g.storeTime("save1.json"); !ok || !got.Equal(time.Unix(123, 0)) {
		t.Fatalf("slot time: %v %v", got, ok)
	}
	if report := wldReplay(t, g); strings.Contains(report, "diverged") {
		t.Fatal(report)
	}
}

func TestTraceReproPreservesTargetState(t *testing.T) {
	trace := wldRecord(t, wldRecordScript)
	normal, err := wldReplayGame(t, trace, func(o *Options) { o.Until = 100 })
	if err != nil {
		t.Fatal(err)
	}
	for normal.tick < 99 {
		if err := normal.Update(); err != nil {
			t.Fatal(err)
		}
	}
	want, err := json.Marshal(normal.traceSummary())
	if err != nil {
		t.Fatal(err)
	}
	if err := normal.Close(); err != nil {
		t.Fatal(err)
	}
	fast, err := wldReplayGame(t, trace, func(o *Options) { o.Until = 100 })
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for !fast.untilShot {
		if err := fast.Update(); err != nil {
			t.Fatal(err)
		}
		calls++
	}
	got, err := json.Marshal(fast.traceSummary())
	if err != nil {
		t.Fatal(err)
	}
	if fast.tick != 99 || !bytes.Equal(got, want) {
		t.Fatalf("seek at %d: %s; want %s", fast.tick, got, want)
	}
	if calls != 100 {
		t.Fatalf("seek must advance one tick per update: %d calls", calls)
	}
	fast.autoShot = "" // A successful target Draw releases the stop handshake.
	for range 3 {
		if err := fast.Update(); errors.Is(err, errQuit) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(ReplayReport, "diverged") {
		t.Fatal(ReplayReport)
	}
}

func TestTraceReproScriptDirectActions(t *testing.T) {
	trace := wldRecord(t, "wait 5\ndo set: {gold: 123}\nwait 5\ndo call: ev_auto\nwait 40\nquit\n")
	for _, kind := range []string{"dev_set", "dev_do", "jump"} {
		if len(traceLines(t, trace, kind)) != 0 {
			t.Fatalf("redundant mutation record %s", kind)
		}
	}
	var exact traceformat.ExactReplay
	for line := range bytes.SplitSeq(bytes.TrimSpace(trace), []byte("\n")) {
		var header struct {
			Kind string `json:"t"`
			Tick int    `json:"tick"`
		}
		if err := json.Unmarshal(line, &header); err != nil {
			t.Fatal(err)
		}
		if err := exact.Check(header.Kind, header.Tick, line); err != nil {
			t.Fatal(err)
		}
	}
	acts := traceLines(t, trace, "act")
	if len(acts) < 2 {
		t.Fatalf("direct script actions missing: %s", trace)
	}
	g, err := wldReplayGame(t, trace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report := wldReplay(t, g); strings.Contains(report, "diverged") {
		t.Fatal(report)
	}
	if g.st.Gold != 123 {
		t.Fatalf("script state: %d", g.st.Gold)
	}
}

func TestTraceReproActionsDuringChangedSaveNotice(t *testing.T) {
	fsys := gameFiles(t)
	store := savefile.MemStore()
	initial := wldOpenWith(t, Options{FS: fsys, Store: store, Location: "camp"})
	initial.st.Gold = 99
	if err := initial.save(0); err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	fsys["game.yaml"].Data = append(fsys["game.yaml"].Data, []byte("data_version: 2\n")...)
	var trace bytes.Buffer
	g := wldOpenWith(t, Options{
		FS: fsys, Store: store, Trace: &trace,
		Script: "wait 2\npress z\nwait 10\npress z\nwait 40\ndo set: {gold: 123}\nwait 5\ndo set: {gold: 456}\nwait 100\n",
	})
	type frame struct {
		Gold   int
		Notice bool
		Frames int64
	}
	state := func(g *Game) frame {
		f := frame{Notice: g.loadNotice != nil}
		if g.st != nil {
			f.Gold, f.Frames = g.st.Gold, g.st.Frames
		}
		return f
	}
	var frames []frame
	for range 100 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, state(g))
	}
	if g.loadNotice == nil || g.loadNotice.list == nil || g.st.Gold != 456 {
		t.Fatalf("script did not mutate with the notice open: %+v", state(g))
	}
	acts := traceLines(t, trace.Bytes(), "act")
	if len(acts) != 2 {
		t.Fatalf("recorded actions: %v", acts)
	}
	for _, act := range acts {
		if f := frames[int(traceformat.Number(act["tick"]))-1]; !f.Notice || (f.Gold != 123 && f.Gold != 456) {
			t.Fatalf("mutation did not run with the notice open: %+v", f)
		}
	}
	g.traceEnd()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "notice.jsonl")
	if err := os.WriteFile(path, trace.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 100}); err != nil {
		t.Fatalf("exact reproduction refused: %v", err)
	}
	r := wldOpenWith(t, Options{FS: fsys, Store: savefile.MemStore(), Replay: path})
	for tick, want := range frames {
		if err := r.Update(); err != nil {
			t.Fatalf("tick %d: %v", tick+1, err)
		}
		if got := state(r); got != want {
			t.Fatalf("tick %d: replay %+v, recorded %+v", tick+1, got, want)
		}
	}
	r.traceEnd()
	if r.rep.diverge != "" || !r.rep.finished {
		t.Fatalf("replay: %s", r.rep.report())
	}
}

func TestTraceRejectsScriptMutations(t *testing.T) {
	for _, script := range []string{"recruit mote 1", "guarantee-recruit mote", "battle-hp enemy 0 1", "battle-log off"} {
		t.Run(script, func(t *testing.T) {
			files := gameFiles(t)
			for name, body := range testgame.MonsterFiles() {
				files[name] = &fstest.MapFile{Data: []byte(body)}
			}
			files["game.yaml"].Data = append(files["game.yaml"].Data, []byte("\nfonts: {regular: test.ttf}\n")...)
			var buf bytes.Buffer
			g := wldOpenWith(t, Options{FS: files, Location: "field", Battle: "one", Development: true, Trace: &buf, Script: "wait 40\n" + script + "\nwait 2\nquit\n"})
			for range 100 {
				if err := g.Update(); errors.Is(err, errQuit) {
					break
				} else if err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, until := range []int{0, 44} {
				_, err := loadReplay(path, &Options{Until: until})
				if err == nil || !strings.Contains(err.Error(), "cannot reproduce exactly:") {
					t.Fatalf("until %d: %v\n%s", until, err, &buf)
				}
			}
		})
	}
}

func TestTraceRecordsViewportChanges(t *testing.T) {
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append(fsys["game.yaml"].Data, []byte("schema_version: 11\nscreen: {width: 640, height: 360, native: true}\n")...)
	var buf bytes.Buffer
	g, err := New(Options{FS: fsys, Trace: &buf, Seed: 7, Window: 2, Fullscreen: true, NoSound: true, headless: true, Store: savefile.MemStore()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	g.in.script = nil
	g.layout(1280, 720)
	g.layout(1280, 720) // unchanged layouts must not add records
	g.tick = 3
	g.layout(900, 600)
	g.traceEnd()
	if got := len(traceLines(t, buf.Bytes(), "viewport")); got != 2 {
		t.Fatalf("viewport records: %d", got)
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Development: true}
	rep, err := loadReplay(path, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Window != 2 || !opts.Fullscreen || !rep.setup.Config.Fullscreen || opts.Development {
		t.Fatalf("lost start window/fullscreen: %+v", opts)
	}
}

func TestReplayStartupViewport(t *testing.T) {
	for _, tick := range []int{0, 1} {
		t.Run(fmt.Sprintf("synthetic settled tick=%d", tick), func(t *testing.T) {
			trace := fmt.Sprintf(`{"t":"start","tick":0,"version":2,"exact_version":3,"setup":{}}
{"t":"viewport","tick":0,"width":640,"height":360,"scale":1}
{"t":"viewport","tick":0,"width":1280,"height":720,"scale":1}
{"t":"viewport","tick":%d,"width":1024,"height":576,"scale":1}
{"t":"viewport","tick":2,"width":960,"height":540,"scale":1}
`, tick)
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, until := range []int{1, 2} {
				rep, err := loadReplay(path, &Options{Until: until})
				if err != nil {
					t.Fatal(err)
				}
				want := traceformat.Viewport{Width: 1024, Height: 576, Scale: 1}
				if tick >= until {
					want.Width, want.Height = 1280, 720
				}
				if rep.startupViewport == nil || *rep.startupViewport != want {
					t.Fatalf("until=%d: startup viewport = %+v, want %+v", until, rep.startupViewport, want)
				}
			}
		})
	}
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append(fsys["game.yaml"].Data, []byte("schema_version: 11\nscreen: {width: 640, height: 360, native: true}\n")...)
	for _, until := range []int{1, 2} {
		t.Run(fmt.Sprintf("until=%d", until), func(t *testing.T) {
			for _, fullscreen := range []bool{false, true} {
				t.Run(fmt.Sprintf("fullscreen=%t", fullscreen), func(t *testing.T) {
					var buf bytes.Buffer
					g, err := New(Options{FS: fsys, Trace: &buf, Seed: 7, Fullscreen: fullscreen, NoSound: true, Store: savefile.MemStore()})
					if err != nil {
						t.Fatal(err)
					}
					g.in.script = nil
					g.layout(640, 360)
					g.layout(1280, 720)
					g.tick = until - 1
					g.layout(1024, 576) // The window manager settles by the target tick.
					g.traceEnd()
					if err := g.Close(); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(t.TempDir(), "trace.jsonl")
					if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
						t.Fatal(err)
					}
					for _, size := range [][2]int{{1024, 576}, {1280, 720}, {960, 540}} {
						t.Run(fmt.Sprint(size), func(t *testing.T) {
							r, err := New(Options{FS: fsys, Replay: path, Until: until, NoSound: true, headless: true, Store: savefile.MemStore()})
							if err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() {
								if closeErr := r.Close(); closeErr != nil && !errors.Is(closeErr, r.failure) {
									t.Error(closeErr)
								}
							})
							r.in.script = nil
							if until > 1 {
								r.layout(1280, 720)
								if err := r.Update(); err != nil || r.tick != 1 {
									t.Fatalf("unsettled startup: tick=%d, err=%v", r.tick, err)
								}
							}
							r.layout(float64(size[0]), float64(size[1]))
							err = r.Update()
							if size == [2]int{1024, 576} {
								if err != nil || !r.untilShot || r.tick != until-1 {
									t.Fatalf("matching viewport: capture=%t, err=%v", r.untilShot, err)
								}
							} else {
								want := fmt.Sprintf("cannot reproduce exactly: replay startup viewport %dx%d differs from recorded startup viewport 1024x576", size[0], size[1])
								if err == nil || err.Error() != want {
									t.Fatalf("mismatch error: %v, want %s", err, want)
								}
								if r.tick != until-1 || r.untilShot || r.autoShot != "" {
									t.Fatalf("mismatch advanced or scheduled capture: tick=%d, untilShot=%t, autoShot=%q", r.tick, r.untilShot, r.autoShot)
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestReplayStartOptionPresence(t *testing.T) {
	caller := Options{Development: true, Window: 2, Fullscreen: true, Locales: []string{"en-US"}, BookAsPlayed: true}
	for _, tt := range []struct {
		name   string
		fields string
		want   Options
	}{
		{name: "older trace", want: caller},
		{name: "development only", fields: `,"development":false`, want: Options{Window: 2, Fullscreen: true, Locales: []string{"en-US"}, BookAsPlayed: true}},
		{name: "window only", fields: `,"window":0`, want: Options{Development: true, Fullscreen: true, Locales: []string{"en-US"}, BookAsPlayed: true}},
		{name: "fullscreen only", fields: `,"fullscreen":false`, want: Options{Development: true, Window: 2, Locales: []string{"en-US"}, BookAsPlayed: true}},
		{name: "locales only", fields: `,"locales":["fr-FR"]`, want: Options{Development: true, Window: 2, Fullscreen: true, Locales: []string{"fr-FR"}, BookAsPlayed: true}},
		{name: "empty locales", fields: `,"locales":[]`, want: Options{Development: true, Window: 2, Fullscreen: true, Locales: []string{}, BookAsPlayed: true}},
		{name: "null locales", fields: `,"locales":null`, want: Options{Development: true, Window: 2, Fullscreen: true, BookAsPlayed: true}},
		{name: "book only", fields: `,"book_as_played":false`, want: Options{Development: true, Window: 2, Fullscreen: true, Locales: []string{"en-US"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Version 2 predates these optional start fields and exact replay metadata.
			trace := `{"t":"start","tick":0,"version":2,"setup":{}` + tt.fields + "}\n" +
				`{"t":"end","tick":1,"lines":2,"state":{}}` + "\n"
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
				t.Fatal(err)
			}
			opts := caller
			if _, err := loadReplay(path, &opts); err != nil {
				t.Fatal(err)
			}
			got := Options{Development: opts.Development, Window: opts.Window, Fullscreen: opts.Fullscreen, Locales: opts.Locales, BookAsPlayed: opts.BookAsPlayed}
			if got.Development != tt.want.Development || got.Window != tt.want.Window || got.Fullscreen != tt.want.Fullscreen ||
				got.BookAsPlayed != tt.want.BookAsPlayed || !reflect.DeepEqual(got.Locales, tt.want.Locales) {
				t.Fatalf("replayed options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTraceRejectsInjectedState(t *testing.T) {
	g, _ := wldTracedGame(t, "wait 5\n")
	g.opts.State = g.st
	var buf bytes.Buffer
	g.tr = newTracer(&buf)
	if err := g.traceStart(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"unsupported":["injected state"]`) {
		t.Fatal(buf.String())
	}
}

func TestTraceRestoresPresentationSettings(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(strconv.FormatBool(saved), func(t *testing.T) {
			fsys := wldWorldFiles(t, "written_in: en\nlanguages: [en, ja]\nui: {accent: '#123456'}\n", map[string]string{"lang/ja.yaml": twoLanguageWords})
			store := savefile.MemStore()
			if saved {
				if err := store.Write("config.json", []byte(`{"language":"en","window":3,"text_speed":4}`)); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			g := wldOpenWith(t, Options{FS: fsys, Trace: &buf, Store: store, Locales: []string{"ja-JP"}, Location: "camp", Flags: []string{"ready"}, BookAsPlayed: true})
			wantLanguage, wantTheme, wantConfig := g.language(), g.th, g.cfg
			g.traceEnd()
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			replayed := wldOpenWith(t, Options{FS: fsys, Replay: path, Store: savefile.MemStore(), Locales: []string{"en-US"}})
			if replayed.language() != wantLanguage || replayed.th != wantTheme || !reflect.DeepEqual(replayed.cfg, wantConfig) || !replayed.opts.BookAsPlayed || !reflect.DeepEqual(replayed.opts.Flags, []string{"ready"}) {
				t.Fatal("replay lost language, theme, settings, book display or flags")
			}
		})
	}
}

func TestReplayRejectsWindowChangeThroughTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	trace := `{"t":"start","tick":0,"exact_version":3,"version":1,"seed":7}
{"t":"act","tick":3,"name":"window","args":{"width":960,"height":540}}
{"t":"end","tick":5}
`
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 4}); err == nil || !strings.Contains(err.Error(), "the recording changed the window size") {
		t.Fatalf("resize: %v", err)
	}
}

// traceClassifications reads the protocol table for the source audit and loader
// rejection cases without adding a runtime API used only by tests.
func traceClassifications(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../layer1/traceformat/trace.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.ValueSpec)
		if !ok || len(decl.Names) != 1 || decl.Names[0].Name != "kinds" || len(decl.Values) != 1 {
			return true
		}
		literal, ok := decl.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("trace kinds must remain an explicit map literal")
		}
		for _, entry := range literal.Elts {
			pair, ok := entry.(*ast.KeyValueExpr)
			if !ok {
				t.Fatal("trace kind must have an explicit classification")
			}
			key, keyOK := pair.Key.(*ast.BasicLit)
			value, valueOK := pair.Value.(*ast.Ident)
			if !keyOK || key.Kind != token.STRING || !valueOK || (value.Name != "true" && value.Name != "false") {
				t.Fatal("trace classifications must be literal strings and booleans")
			}
			name, err := strconv.Unquote(key.Value)
			if err != nil {
				t.Fatal(err)
			}
			result[name] = value.Name == "true"
		}
		return false
	})
	if len(result) == 0 {
		t.Fatal("missing trace classifications")
	}
	return result
}

func TestTraceRecordKindsHaveExactReplayClassification(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{"repro": true} // The trace CLI adds this seek boundary.
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "emit" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("%s: trace constructor must use a literal kind so the replay audit can enumerate it", name)
				return true
			}
			kind, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			seen[kind] = true
			return true
		})
	}
	classifications := traceClassifications(t)
	for kind := range seen {
		if _, known := classifications[kind]; !known {
			t.Errorf("record %q must be in exactly one replay classification", kind)
		}
	}
	for kind := range classifications {
		if !seen[kind] {
			t.Errorf("stale classification %q", kind)
		}
	}
}

func TestReplayUntilRejectsUnlistedRecordsAtTarget(t *testing.T) {
	refused := []string{"future_kind"}
	for kind, accepted := range traceClassifications(t) {
		if !accepted {
			refused = append(refused, kind)
		}
	}
	for _, kind := range refused {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			trace := fmt.Sprintf("{\"t\":\"start\",\"tick\":0,\"exact_version\":3,\"version\":1}\n{\"t\":%q,\"tick\":2}\n", kind)
			if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadReplay(path, &Options{Until: 2}); err != nil {
				t.Fatal(err)
			}
			_, err := loadReplay(path, &Options{Until: 3})
			want := fmt.Sprintf("cannot reproduce exactly: %s at tick 2", kind)
			if err == nil || err.Error() != want {
				t.Fatalf("got %v; want %s", err, want)
			}
		})
	}
}

func TestReplayRejectsPanelThroughTarget(t *testing.T) {
	for _, tab := range []panelTab{tabCommand, tabFrames} {
		t.Run(map[panelTab]string{tabCommand: "unsubmitted command", tabFrames: "wall-clock frames"}[tab], func(t *testing.T) {
			g := storyGame(t, Options{Location: "camp", Event: "first", Development: true}, "schema_version: 4\n")
			var buf bytes.Buffer
			g.tr = newTracer(&buf)
			if err := g.traceStart(); err != nil {
				t.Fatal(err)
			}
			g.tick = 2
			g.togglePanel()
			for g.panel.tab != tab {
				g.panelTab(true)
			}
			if tab == tabCommand {
				g.typeMemo("set: {flag.typed: true}")
				if g.panel.text == "" || g.st.Flags["typed"] {
					t.Fatal("expected an unsubmitted command")
				}
			} else {
				g.meter.since = time.Now().Add(-2 * time.Second)
				g.meter.updated(time.Now().Add(-time.Millisecond))
				if !g.meter.on || g.meter.text == "" {
					t.Fatal("expected displayed wall-clock measurements")
				}
			}
			g.tick = 4
			g.closePanel()
			g.traceEnd()
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, until := range []int{0, 2, 3, 5} {
				_, err := loadReplay(path, &Options{Until: until})
				if until <= 2 {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || err.Error() != "cannot reproduce exactly: panel at tick 2" {
					t.Fatalf("until %d: %v", until, err)
				}
			}
		})
	}
}

// assertSourceRefused checks the actual runtime record, not only the kind registry.
func assertSourceRefused(t *testing.T, buf *bytes.Buffer, kind string) {
	t.Helper()
	lines := traceLines(t, buf.Bytes(), kind)
	if len(lines) == 0 {
		t.Fatalf("source did not record %s: %s", kind, buf)
	}
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		var exact traceformat.ExactReplay
		if err := exact.Check(kind, int(traceformat.Number(line["tick"])), raw); err == nil {
			t.Fatalf("accepted unrecorded source %s", kind)
		}
	}
}

func TestTraceSourceBoundaries(t *testing.T) {
	for _, source := range []string{"IME", "memo", "graphs", "focus", "carry", "settings", "frame meter", "voice", "reload", "scene review", "picture review", "save error", "unlock error", "audio timing"} {
		t.Run(source, func(t *testing.T) {
			g := storyGame(t, Options{Location: "camp", Event: "first", Development: true}, "schema_version: 4\n")
			var buf bytes.Buffer
			g.tr = newTracer(&buf)
			g.tick = 3
			switch source {
			case "IME":
				f := &textField{}
				f.start(g, func() (float64, float64) { return 0, 0 })
				// Even a composition that is never committed is outside recorded input.
				f.comp = "unfinished"
				assertSourceRefused(t, &buf, "text_input")
			case "memo":
				g.openMemo()
				g.memo.typed = true
				g.typeMemo("unfinished")
				assertSourceRefused(t, &buf, "memo_open")
				g.closeMemo()
			case "graphs":
				g.in.pressed[btnGraphs] = true
				g.updateGraphs()
				assertSourceRefused(t, &buf, "graphs")
			case "focus":
				g.rendering, g.pauseUnfocused = true, true
				if !g.pauseForFocus(false) {
					t.Fatal("expected focus pause")
				}
				assertSourceRefused(t, &buf, "focus_pause")
			case "carry":
				g.db.Game.Carry = []string{"flag.*"}
				g.carry = nil
				g.loadCarry()
				assertSourceRefused(t, &buf, "carry_read")
			case "settings":
				if err := g.saveConfig(); err != nil {
					t.Fatal(err)
				}
				assertSourceRefused(t, &buf, "config_write")
			case "frame meter":
				g.meter.toggle()
				if err := g.traceStart(); err != nil {
					t.Fatal(err)
				}
				assertSourceRefused(t, &buf, "start")
			case "audio timing":
				g.holdWinSong("ending")
				assertSourceRefused(t, &buf, "audio_timing")
			case "reload":
				g.askReload()
				assertSourceRefused(t, &buf, "live_reload")
			case "scene review":
				g.opts.ReviewLog = filepath.Join(t.TempDir(), "review.jsonl")
				(&sceneCheck{}).load(g)
				assertSourceRefused(t, &buf, "scene_review")
			case "picture review":
				g.opts.ReviewLog = filepath.Join(t.TempDir(), "review.jsonl")
				newCGReview(g)
				assertSourceRefused(t, &buf, "cg_review")
			case "save error", "unlock error":
				g.store = &retryStore{Store: savefile.MemStore(), writes: map[string]int{}, fail: map[string]error{g.slotPath(0): errors.New("full"), g.galleryPath(): errors.New("full")}}
				if source == "save error" {
					if err := g.save(0); err == nil {
						t.Fatal("expected failed save")
					}
				} else {
					g.unlockDirty = true
					g.saveUnlocks()
				}
				assertSourceRefused(t, &buf, "persistence_error")
			case "voice":
				g.playVoice(&g.talk, event.Say{Voice: "missing.ogg"})
				assertSourceRefused(t, &buf, "voice")
			}
		})
	}
}

func TestTraceStreamTextRefusedBeforeSubmission(t *testing.T) {
	for _, source := range []string{"comment", "tip", "name"} {
		t.Run(source, func(t *testing.T) {
			g, w, _ := voicedShow(t)
			var buf bytes.Buffer
			g.tr = newTracer(&buf)
			if source == "name" {
				g.st.Streams.Viewer = ""
				g.stack = []scene{newStreamLobby(g, nil)}
			} else {
				w.typing, w.tips = source == "comment", source == "tip"
			}
			g.typeMemo("For seeds")
			assertSourceRefused(t, &buf, "text_input")
			if source != "name" {
				if w.field.text != "For seeds" {
					t.Fatal("normal typing changed")
				}
				g.in.pressed[btnOK] = true
				w.input(g)
				kind := "stream_comment"
				if source == "tip" {
					kind = "stream_tip"
				}
				assertSourceRefused(t, &buf, kind)
			}
		})
	}
}

func TestTraceStreamClockRefused(t *testing.T) {
	var buf bytes.Buffer
	g := wldOpenWith(t, Options{FS: streamsFS(t, "0"), Trace: &buf, Location: "booth", Now: func() time.Time { return noonish }})
	if g.desk == nil {
		t.Fatal("stream clock did not start")
	}
	assertSourceRefused(t, &buf, "now")
}

func TestTraceSaveWriteRestoresDisplayedTime(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(strconv.FormatBool(known), func(t *testing.T) {
			clock := time.Date(2024, 2, 3, 4, 5, 0, 0, time.UTC)
			var store Store = kvStore{kv: &memKV{m: map[string]string{}}, now: func() time.Time { return clock }}
			if !known {
				store = struct{ Store }{store}
			} // Deliberately no ModTime capability.
			var buf bytes.Buffer
			g := storyGame(t, Options{Location: "camp", Trace: &buf, Store: store, Seed: 7}, "schema_version: 4\n")
			g.tick = 2
			if err := g.save(0); err != nil {
				t.Fatal(err)
			}
			want, ok := g.slotLabel(0)
			if !ok {
				t.Fatal("recording cannot read its save")
			}
			g.tick = 3
			g.traceEnd()
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			clock = clock.Add(48 * time.Hour)
			g = storyGame(t, Options{Replay: path, Until: 4, Store: store}, "schema_version: 4\n")
			g.tick = 2
			if err := g.save(0); err != nil {
				t.Fatal(err)
			}
			got, ok := g.slotLabel(0)
			if !ok || got != want {
				t.Fatalf("save screen changed: %q, want %q", got, want)
			}
			if _, gotKnown := g.storeTime(g.slotPath(0)); gotKnown != known {
				t.Fatal("timestamp availability changed")
			}
		})
	}
}

func TestTraceExternalSaveChangesRefused(t *testing.T) {
	for _, source := range []string{"bytes", "timestamp", "read error"} {
		t.Run(source, func(t *testing.T) {
			var buf bytes.Buffer
			dir := t.TempDir()
			g := storyGame(t, Options{Location: "camp", Store: savefile.DirStore(dir)}, "schema_version: 4\n")
			if err := g.save(0); err != nil {
				t.Fatal(err)
			}
			underlying := g.store
			g.opts.Trace, g.tr = &buf, newTracer(&buf)
			if err := g.traceStart(); err != nil {
				t.Fatal(err)
			}
			g.tick = 2
			switch source {
			case "bytes":
				if err := underlying.Write(g.slotPath(0), []byte("changed")); err != nil {
					t.Fatal(err)
				}
				if _, err := g.load(0); err == nil {
					t.Fatal("loading changed save bytes succeeded")
				}
			case "timestamp":
				path := filepath.Join(dir, g.slotPath(0))
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				stamp := info.ModTime().Add(-24 * time.Hour)
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				g.storeTime(g.slotPath(0))
			case "read error":
				store, ok := g.store.(*traceStore)
				if !ok {
					t.Fatal("tracing did not wrap the save store")
				}
				readErr := errors.New("unavailable")
				store.Store = sysFailingStore{Store: savefile.MemStore(), readErr: readErr}
				if _, err := g.load(0); !errors.Is(err, readErr) {
					t.Fatalf("load error = %v, want %v", err, readErr)
				}
			}
			assertSourceRefused(t, &buf, "external_save")
		})
	}
}

func TestTraceResourcesRejectChangedInputs(t *testing.T) {
	for _, file := range []string{"assets/regular.ttf", "assets/image.png", "assets/voice.ogg", "assets/faces.json", "lang/en.yaml"} {
		t.Run(file, func(t *testing.T) {
			base := gameFiles(t)
			base[file] = &fstest.MapFile{Data: []byte("original")}
			want, err := traceResources(base)
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := traceFingerprint(base)
			if err != nil {
				t.Fatal(err)
			}
			r := &replay{version: traceVersion, fingerprint: fingerprint, setup: traceSetup{Resources: want}}
			base[file].Data = []byte("edited")
			g := &Game{opts: Options{FS: base, Seed: 7}}
			if err := r.prepare(g); err == nil || !strings.Contains(err.Error(), "assets or translations changed") {
				t.Fatalf("changed %s: %v", file, err)
			}
		})
	}
}

func TestTraceProfileDeviceInputRefused(t *testing.T) {
	g := bookGame(t, Options{Location: "camp"}, "")
	var buf bytes.Buffer
	g.tr = newTracer(&buf)
	newProfiles(g, nil)
	assertSourceRefused(t, &buf, "profiles")
}

func TestTraceAsyncPictureRefused(t *testing.T) {
	g := storyGame(t, Options{Location: "camp"}, "schema_version: 4\n")
	var buf bytes.Buffer
	g.tr = newTracer(&buf)
	g.dollSheet(newDollSpec("x.png", nil, 30))
	assertSourceRefused(t, &buf, "async_picture")
}

func TestTraceMusicRoomSeedRestored(t *testing.T) {
	var buf bytes.Buffer
	g := storyGame(t, Options{Trace: &buf}, "schema_version: 4\n")
	if g.opts.Seed == 0 {
		t.Fatal("recording retained an unseeded RNG")
	}
	room := newMusicRoom(g)
	pool := []string{"first", "second", "third"}
	var want []string
	for range 20 {
		want = append(want, pickBackground(pool, "", room.rnd))
	}
	g.traceEnd()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	g = storyGame(t, Options{Replay: path, Until: 1}, "schema_version: 4\n")
	room = newMusicRoom(g)
	for _, expected := range want {
		if got := pickBackground(pool, "", room.rnd); got != expected {
			t.Fatalf("shuffle: %s, want %s", got, expected)
		}
	}
}

func TestReplayIgnoresLiveFullscreenHotkey(t *testing.T) {
	g := &Game{rep: &replay{partial: true}}
	g.toggleFullscreen()
	if g.cfg.Fullscreen || g.failure != nil {
		t.Fatal("live fullscreen hotkey changed replay")
	}
}

func TestReplayRefusesUnrecordedSaveClock(t *testing.T) {
	g := &Game{rep: &replay{partial: true}, tick: 4}
	s := &replayStore{Store: savefile.MemStore(), game: g, times: map[string]time.Time{}}
	if err := s.Write(g.slotPath(0), []byte("save")); err == nil || g.failure == nil {
		t.Fatal("an unrecorded save received a fabricated timestamp")
	}
	if _, err := s.Read(g.slotPath(0)); err == nil {
		t.Fatal("refused save reached storage")
	}
}

func TestReplayPreservesMultipleSaveClocksWithinTick(t *testing.T) {
	first := time.Date(2025, 1, 2, 3, 4, 0, 0, time.FixedZone("recorded", 9*60*60))
	second := first.Add(time.Minute)
	g := &Game{rep: &replay{partial: true}, tick: 2}
	name := g.slotPath(0)
	s := &replayStore{Store: savefile.MemStore(), game: g, times: map[string]time.Time{}, writes: map[int]map[string][]*time.Time{2: {name: {&first, &second}}}}
	for _, want := range []time.Time{first, second} {
		if err := s.Write(name, []byte("save")); err != nil {
			t.Fatal(err)
		}
		got, err := s.ModTime(name)
		if err != nil || got.Format(time.RFC3339) != want.Format(time.RFC3339) {
			t.Fatalf("got %v (%v), want %v", got, err, want)
		}
	}
	if err := s.Write(name, []byte("extra")); err == nil {
		t.Fatal("accepted extra write")
	}
}

// Audio uses the real PCM mixer without opening an audio device. Its trace
// must reproduce at every tick with the muted startup used by printed commands.
func TestTraceUnmutedSFXWaitMutedReproduction(t *testing.T) {
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append([]byte("schema_version: 4\n"), fsys["game.yaml"].Data...)
	fsys["data/sound.yaml"] = &fstest.MapFile{Data: []byte(`events:
  sound_wait:
    do:
      - sfx: {name: decide, wait: true}
      - sfx: {name: custom, wait: true}
      - say: "Finished."
`)}
	fsys["assets/sfx/custom.wav"] = &fstest.MapFile{Data: testgame.Wav(0.75)}
	var trace, audio bytes.Buffer
	g, err := New(Options{FS: fsys, Location: "camp", Event: "sound_wait", Seed: 7, Audio: &audio, Trace: &trace, Store: savefile.MemStore(), headless: true, Script: "wait 1000\n"})
	if err != nil {
		t.Fatal(err)
	}
	type frame struct {
		Wait int
		Line string
	}
	var frames []frame
	for range 100 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame{g.waitT, at(g)})
	}
	g.traceEnd()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if audio.Len() == 0 || frames[len(frames)-1].Line != "Finished." {
		t.Fatal("recording did not play through sound waits")
	}
	path := filepath.Join(t.TempDir(), "sound.jsonl")
	if err := os.WriteFile(path, trace.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 100}); err != nil {
		t.Fatalf("exact reproduction refused: %v", err)
	}
	g, err = New(Options{FS: fsys, Replay: path, NoSound: true, Store: savefile.MemStore()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	}()
	for tick, want := range frames {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d, tick %d, limit %d: %v", tick, g.tick, g.rep.limit, err)
		}
		if got := (frame{g.waitT, at(g)}); got != want {
			t.Fatalf("tick %d: muted %+v, recorded %+v", tick, got, want)
		}
	}
	g.traceEnd()
	if g.rep.diverge != "" || !g.rep.finished {
		t.Fatalf("replay: %s", g.rep.report())
	}
}

func TestTraceStartupFocusPause(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(strconv.FormatBool(resume), func(t *testing.T) {
			g := storyGame(t, Options{Location: "camp"}, "schema_version: 4\n")
			var buf bytes.Buffer
			g.tr = newTracer(&buf)
			g.rendering, g.pauseUnfocused = true, true
			for range 2 {
				if !g.pauseForFocus(false) || g.tick != 0 {
					t.Fatal("startup focus pause advanced simulation")
				}
			}
			lines := traceLines(t, buf.Bytes(), "focus_pause")
			if len(lines) != 1 {
				t.Fatalf("startup markers = %d", len(lines))
			}
			raw, err := json.Marshal(lines[0])
			if err != nil {
				t.Fatal(err)
			}
			var exact traceformat.ExactReplay
			if err := exact.Check("focus_pause", 0, raw); err != nil {
				t.Fatal(err)
			}
			if resume && g.pauseForFocus(true) {
				t.Fatal("focused window paused")
			}
			// A later loss, or updates while unfocused, must not be hidden by
			// the accepted startup marker: movement may now be reset.
			buf.Reset()
			g.tick = 3
			g.pauseForFocus(false)
			assertSourceRefused(t, &buf, "focus_pause")
		})
	}
}

// assertTraceTicks checks every written record, including observations and end.
func assertTraceTicks(t *testing.T, b []byte) {
	t.Helper()
	previous := 0
	for line := range bytes.Lines(b) {
		var record traceLine
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		tick := int(traceformat.Number(record["tick"]))
		if tick < previous {
			t.Fatalf("decreasing tick %d after %d: %s", tick, previous, line)
		}
		previous = tick
	}
}

func TestTraceMonotonicTicks(t *testing.T) {
	var buf bytes.Buffer
	g := &Game{tr: newTracer(&buf), tick: 7}
	g.emit("snapshot", nil)
	g.emit("memo_open", traceLine{"tick": 6})
	g.tick = 6
	g.emit("memo", nil)
	g.tick = 8
	g.emit("snapshot", nil)
	assertTraceTicks(t, buf.Bytes())
	for i, record := range traceLines(t, buf.Bytes(), "snapshot", "memo_open", "memo") {
		if want := []int{7, 7, 7, 8}[i]; int(traceformat.Number(record["tick"])) != want {
			t.Fatalf("record %d: %v, want tick %d", i, record, want)
		}
	}
}

func TestTraceScriptBeforeMemo(t *testing.T) {
	for _, command := range []string{"guides off", "snapshot"} {
		t.Run(command, func(t *testing.T) {
			var buf bytes.Buffer
			fsys := gameFiles(t)
			g := wldOpenWith(t, Options{
				FS: fsys, Location: "camp", Seed: 7,
				Trace: &buf, MemoDir: t.TempDir(), NoSound: true, Store: savefile.MemStore(),
				Script: "wait 5\n" + command + "\npress memo\nwait 10\n",
			})
			for range 4 {
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
			}
			want, err := json.Marshal(g.traceSummary())
			if err != nil {
				t.Fatal(err)
			}
			for range 10 {
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
			}
			if g.memo == nil {
				t.Fatal("memo did not open")
			}
			g.traceEnd()
			assertTraceTicks(t, buf.Bytes())
			markers := traceLines(t, buf.Bytes(), "memo_open")
			if len(markers) != 1 {
				t.Fatalf("memo boundaries: %v", markers)
			}
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadReplay(path, &Options{Until: 5}); err != nil {
				t.Fatal(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			r := wldOpenWith(t, Options{FS: fsys, Replay: path, Until: 5, NoSound: true, Store: savefile.MemStore()})
			for range 4 {
				if err := r.Update(); err != nil {
					t.Fatal(err)
				}
			}
			r.traceEnd()
			got, err := json.Marshal(r.traceSummary())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) || r.rep.diverge != "" || !r.rep.finished {
				t.Fatalf("earlier target: %s; got %s, want %s", r.rep.report(), got, want)
			}
		})
	}
}

// Every Update path shares the trace stream contract, including early returns
// and overlays that consume input before the scene sees it.
func TestTraceUpdateInputProperties(t *testing.T) {
	for _, mode := range []string{"normal", "autopilot", "memo", "banners", "phone"} {
		for seed := uint64(1); seed <= 3; seed++ {
			t.Run(fmt.Sprintf("%s/%d", mode, seed), func(t *testing.T) {
				fsys := gameFiles(t)
				fsys["game.yaml"].Data = append([]byte("schema_version: 11\n"), fsys["game.yaml"].Data...)
				fsys["data/trace.yaml"] = &fstest.MapFile{Data: []byte(`events:
  banners:
    - say: Ready.
    - phone: {app: banner, notices: [{title: Notice, event: opened}]}
  phone:
    - say: Ready.
    - phone: {app: banner, var: var.result, notices: [{title: Dismiss me}]}
  opened:
    - set: {flag.opened: true}
  autoplay:
    - say: Line 0.
    - say: Line 1.
    - say: Line 2.
    - say: Line 3.
    - say: Line 4.
    - say: Line 5.
    - say: Line 6.
    - say: Line 7.
    - say: Line 8.
    - say: Line 9.
    - say: Line 10.
    - say: Line 11.
`)}
				var buf bytes.Buffer
				opts := Options{FS: fsys, Location: "camp", Seed: seed, Trace: &buf, NoSound: true, Store: savefile.MemStore()}
				switch mode {
				case "autopilot":
					opts.Autopilot = &Autopilot{Seconds: 10}
					opts.Event = "autoplay"
				case "memo":
					opts.MemoDir = t.TempDir()
				case "banners", "phone":
					opts.Event = mode
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
				physical := &replay{inputs: map[int]traceLine{}}
				for tick := 1; tick <= 140; tick++ {
					physical.inputs[tick] = traceLine{"mouse": []any{float64(100 + uint64(tick)*seed), float64(300)}, "held": []any{"left"}}
				}
				physical.inputs[20]["press"] = []any{"ok"}
				if mode == "banners" {
					sw, _ := g.screen()
					physical.inputs[60] = traceLine{"click": true, "pointer_up": true, "mouse": []any{sw / 2, float64(40)}}
				}
				if mode == "phone" {
					physical.inputs[100]["press"] = []any{"cancel", "menu"}
					physical.inputs[100]["esc"] = true
				}
				if mode == "memo" {
					physical.inputs[6]["press"] = []any{"memo"}
				}
				g.in.replay = physical
				for frame := range 140 {
					beforeTick := g.tick
					beforeInputs := len(traceLines(t, buf.Bytes(), "input"))
					if err := g.Update(); err != nil {
						t.Fatal(err)
					}
					if g.tick == beforeTick && len(traceLines(t, buf.Bytes(), "input")) != beforeInputs {
						t.Fatal("editor input leaked into simulation records")
					}
					if mode == "memo" && frame == 10 {
						if g.memo == nil || g.tick != 5 {
							t.Fatal("F9 did not freeze the simulation")
						}
						physical.inputs[6]["press"] = []any{"cancel"}
					}
				}
				if mode == "banners" && !g.st.Flags["opened"] {
					t.Fatal("banner input was not consumed")
				}
				if mode == "phone" && g.st.Vars["result"] != "dismiss" {
					t.Fatal("phone input was not consumed")
				}
				g.traceEnd()
				assertTraceTicks(t, buf.Bytes())
				inputs := map[int]bool{}
				for line := range bytes.Lines(buf.Bytes()) {
					var record traceLine
					if err := json.Unmarshal(line, &record); err != nil {
						t.Fatal(err)
					}
					tick := int(traceformat.Number(record["tick"]))
					if record["t"] == "input" {
						if inputs[tick] {
							t.Fatalf("duplicate input at tick %d", tick)
						}
						inputs[tick] = true
					}
				}
				if mode == "autopilot" {
					combined := false
					for _, record := range traceLines(t, buf.Bytes(), "input") {
						if traceformat.Number(record["tick"]) != 20 && len(traceformat.Strings(record["press"])) > 0 && len(traceformat.Strings(record["held"])) > 0 && record["mouse"] != nil {
							combined = true
						}
					}
					if !combined {
						t.Fatal("no combined physical and autopilot input")
					}
				}
				if mode == "memo" && len(traceLines(t, buf.Bytes(), "memo_open")) != 1 {
					t.Fatal("expected one memo boundary")
				}
				path := filepath.Join(t.TempDir(), "trace.jsonl")
				if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
				replayOpts := Options{FS: fsys, Replay: path, NoSound: true, Store: savefile.MemStore()}
				frames := 140
				if mode == "memo" {
					// Reproduce tick 4, before the memo boundary at tick 5.
					replayOpts.Until, frames = 5, 4
					if _, err := loadReplay(path, &Options{Until: 6}); err == nil || !strings.Contains(err.Error(), "memo_open") {
						t.Fatalf("missing memo boundary rejection: %v", err)
					}
				}
				if _, err := loadReplay(path, &replayOpts); err != nil {
					t.Fatal(err)
				}
				if err := g.Close(); err != nil {
					t.Fatal(err)
				}
				r, err := New(replayOpts)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := r.Close(); err != nil {
						t.Error(err)
					}
				}()
				for range frames {
					if err := r.Update(); err != nil {
						t.Fatal(err)
					}
				}
				r.traceEnd()
				if r.rep.diverge != "" || !r.rep.finished {
					t.Fatalf("replay: %s", r.rep.report())
				}
			})
		}
	}
}

const reproPickScreens = `
screens:
  routes:
    title: Routes
    entries:
      - {label: Default, do: [{set: {gold: 1}}]}
      - {label: Selected, do: [{set: {gold: 73}}, {open: result}]}
  result:
    title: Selected route
    entries:
      - {label: Finished, text: Selected route reached.}
`

const reproPickEvent = `events:
  choose_route:
    do:
      - choice:
          - {label: Default choice, do: [{set: {gold: 2}}]}
          - {label: Selected choice, do: [{set: {gold: 37}}, {open: routes}]}
`

const reproPickScript = "wait 40\ndo call: choose_route\npick Selected choice\nwait 30\npick Selected\nwait 40\n"

func TestTraceReproScriptPick(t *testing.T) {
	fsys := wldWorldFiles(t, reproPickScreens, map[string]string{"data/pick.yaml": reproPickEvent})
	var recorded bytes.Buffer
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Development: true, Trace: &recorded, Script: reproPickScript + "quit\n"})
	for range 500 {
		if err := g.Update(); errors.Is(err, errQuit) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if g.st.Gold != 73 {
		t.Fatalf("recorded gold = %d", g.st.Gold)
	}
	picks := traceLines(t, recorded.Bytes(), "pick")
	if len(picks) != 2 || picks[0]["index"] != float64(1) || picks[1]["index"] != float64(1) {
		t.Fatalf("picks: %v", picks)
	}
	path := filepath.Join(t.TempDir(), "pick.jsonl")
	if err := os.WriteFile(path, recorded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	for _, until := range []int{0, g.tick} {
		t.Run(strconv.Itoa(until), func(t *testing.T) {
			replayed, err := New(Options{FS: fsys, Replay: path, Until: until, NoSound: true, ShotDir: t.TempDir(), SaveDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := replayed.Close(); err != nil {
					t.Error(err)
				}
			}()
			for range 500 {
				if until > 0 && replayed.tick >= until-1 {
					break
				}
				if err := replayed.Update(); errors.Is(err, errQuit) {
					break
				} else if err != nil {
					t.Fatal(err)
				}
			}
			if replayed.failure != nil || replayed.rep.diverge != "" {
				t.Fatalf("replay: %v %s", replayed.failure, replayed.rep.diverge)
			}
			if replayed.st.Gold != 73 {
				t.Fatalf("replayed gold = %d", replayed.st.Gold)
			}
			kind, items := replayed.scriptPickables()
			if kind != "screen:result" || len(items) != 1 || items[0].label != "Finished" {
				t.Fatalf("screen: %s %v", kind, items)
			}
		})
	}
}

// The dispatch switch is the engine's command table. Audit its cases and the
// parser's aliases, so adding a command cannot silently bypass this inventory.
func TestScriptCommandsHaveReproductionClassification(t *testing.T) {
	classes := map[string]string{
		"press": "input", "hold": "input", "click": "input", "drag": "input",
		"read": "input", "until": "input", "skip": "input", "lines": "input",
		"walk": "input/act", "face": "input/act", "play": "input/act",
		"pick": "pick", "do": "act", "jump": "act", "order": "act", "end_turn": "act",
		"guides": "act/guide", "window": "refused", "type": "refused", "memo": "refused",
		"battle-hp": "refused", "battle-log": "refused", "recruit": "refused", "guarantee-recruit": "refused",
		"wait-graphs": "refused", "wait": "timing", "gap": "timing", "shot": "observation",
		"mark": "observation", "snapshot": "observation", "look": "observation", "quit": "termination",
	}
	file, err := parser.ParseFile(token.NewFileSet(), "input.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	verb := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "verb" {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == "c"
	}
	add := func(e ast.Expr) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			t.Fatal("dynamic script command requires an explicit audit")
			return
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatal(err)
		}
		seen[name] = true
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SwitchStmt:
			if verb(n.Tag) {
				for _, stmt := range n.Body.List {
					clause, ok := stmt.(*ast.CaseClause)
					if !ok {
						t.Fatal("unexpected non-case statement in script command switch")
						return false
					}
					for _, e := range clause.List {
						add(e)
					}
				}
			}
		case *ast.BinaryExpr:
			if n.Op == token.EQL && verb(n.X) {
				add(n.Y)
			}
		}
		return true
	})
	doc, err := os.ReadFile("../../docs/development/graphics.md")
	if err != nil {
		t.Fatal(err)
	}
	for name := range seen {
		if classes[name] == "" {
			t.Errorf("script command %q needs a reproduction classification", name)
		}
		if !strings.Contains(string(doc), "| `"+name+"` |") {
			t.Errorf("script command %q needs a reproduction table row", name)
		}
	}
	for name := range classes {
		if !seen[name] {
			t.Errorf("stale script classification: %s", name)
		}
	}
}

func TestTraceScriptMemoRefusedBeforeDraw(t *testing.T) {
	var trace bytes.Buffer
	g := wldOpenWith(t, Options{FS: gameFiles(t), Location: "camp", MemoDir: t.TempDir(), Trace: &trace, Script: "memo bug Neutral note\nwait 10\n"})
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	markers := traceLines(t, trace.Bytes(), "unsupported_script")
	if len(markers) != 1 || markers[0]["operation"] != "memo" {
		t.Fatalf("markers: %v", markers)
	}
	var exact traceformat.ExactReplay
	raw, err := json.Marshal(markers[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := exact.Check("unsupported_script", 1, raw); err == nil {
		t.Fatal("queued memo accepted")
	}
}

func TestReplayPickWaitsForReceiver(t *testing.T) {
	menu := &list{items: []listItem{{label: "Same"}, {label: "Same"}}}
	g := &Game{tick: 4, stack: []scene{&sysMenu{l: menu}}}
	g.rep = &replay{picks: map[int]traceLine{4: {"on": "menu", "index": float64(1), "label": "Same"}}}
	g.replayPick(&list{})
	if menu.sel != 0 || g.in.pressed[btnOK] || g.rep.pickTick != 0 {
		t.Fatal("covered receiver accepted pick")
	}
	g.replayPick(menu)
	if menu.sel != 1 || !g.in.pressed[btnOK] || g.rep.pickTick != 4 {
		t.Fatal("pick did not select indexed duplicate label at receiver")
	}
	menu.sel = 0
	g.in.pressed[btnOK] = false
	g.replayPick(menu)
	if menu.sel != 0 || g.in.pressed[btnOK] {
		t.Fatal("pick applied twice")
	}
	g.rep.pickTick = 0
	menu.items[1].disabled = true
	g.replayPick(menu)
	if g.failure == nil || menu.sel != 0 || g.in.pressed[btnOK] {
		t.Fatal("disabled replay target accepted")
	}
}

// Exercise the JSON boundary, including an empty following tick.
func TestTracePointerRoundTrip(t *testing.T) {
	for _, want := range []input{
		{mouse: image.Pt(23, 45), moved: true},
		// Returning from touch to a parked mouse changes coordinates without movement.
		{mouse: image.Pt(23, 45)},
		{mouse: image.Pt(23, 45), wheel: -0.125},
		{mouse: image.Pt(23, 45), wheel: 2.75},
		{mouse: image.Pt(23, 45), click: true, pdown: true},
		{mouse: image.Pt(23, 45), rclick: true},
		{mouse: image.Pt(23, 45), pdown: true, pheld: true},
		{mouse: image.Pt(23, 45), pheld: true},
		{mouse: image.Pt(23, 45), pup: true},
		{mouse: image.Pt(23, 45), click: true, moved: true, pup: true},
		{mouse: image.Pt(23, 45), longPress: true},
		{fast: true, esc: true, shift: true, digit: 10},
	} {
		var buf bytes.Buffer
		g := &Game{tr: newTracer(&buf), tick: 1}
		g.traceInput(&want)
		var line traceLine
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		r := &replay{inputs: map[int]traceLine{1: line}}
		var got input
		r.feed(&got, 1)
		if got.mouse != want.mouse || got.moved != want.moved || got.click != want.click || got.rclick != want.rclick || got.pdown != want.pdown || got.pheld != want.pheld || got.pup != want.pup || got.longPress != want.longPress || got.wheel != want.wheel || got.fast != want.fast || got.esc != want.esc || got.shift != want.shift || got.digit != want.digit {
			t.Fatalf("round trip: got %+v, want %+v", got, want)
		}
		if want.longPress && !got.openList() {
			t.Fatal("touch long press did not open the list")
		}
		r.feed(&got, 2)
		// Coordinates persist, but all event flags must clear.
		got.mouse, got.longUsed = image.Point{}, false
		if got.moved || got.click || got.rclick || got.pdown || got.pheld || got.pup || got.longPress || got.wheel != 0 || got.fast || got.esc || got.shift || got.digit != 0 {
			t.Fatalf("stale input: %+v", got)
		}
	}
}

func TestTraceParkedPointerConfirmation(t *testing.T) {
	for _, edge := range []string{"wheel", "held", "release"} {
		t.Run(edge, func(t *testing.T) {
			var buf bytes.Buffer
			original := storyGame(t, Options{}, "schema_version: 4\n")
			original.tr = newTracer(&buf)
			result := ""
			c := newConfirm("Continue?", func() { result = "yes" })
			c.onNo = func() { result = "no" }
			c.age = 6
			original.push(c)
			_, _, _, _, yes, _ := c.layout(original)
			at := yes.Min.Add(image.Pt(1, 1))
			inputs := []input{{mouse: at, moved: true}, {mouse: at}, {mouse: at}, {mouse: at}}
			inputs[1].pressed[btnRight] = true
			switch edge {
			case "wheel":
				inputs[2].wheel = -0.5
			case "held":
				inputs[2].pheld = true
			case "release":
				inputs[2].pup = true
			}
			inputs[3].pressed[btnOK] = true
			for i, in := range inputs {
				original.tick = i + 1
				original.traceInput(&in)
				original.in = in
				c.update(original)
			}
			if result != "no" {
				t.Fatalf("original chose %q", result)
			}
			r := &replay{inputs: make(map[int]traceLine)}
			dec := json.NewDecoder(&buf)
			for dec.More() {
				var line traceLine
				if err := dec.Decode(&line); err != nil {
					t.Fatal(err)
				}
				if line["t"] == "input" {
					r.inputs[int(traceformat.Number(line["tick"]))] = line
				}
			}
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			restored := storyGame(t, Options{}, "schema_version: 4\n")
			result = ""
			c = newConfirm("Continue?", func() { result = "yes" })
			c.onNo = func() { result = "no" }
			c.age = 6
			restored.push(c)
			for tick := 1; tick <= len(inputs); tick++ {
				r.feed(&restored.in, tick)
				c.update(restored)
			}
			if result != "no" {
				t.Fatalf("replay chose %q", result)
			}
		})
	}
}

func TestReplayLegacyPointerRequiresNewRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	data := fmt.Sprintf(`{"t":"start","tick":0,"version":%d,"exact_version":1,"setup":{}}
{"t":"input","tick":1,"mouse":[10,20],"wheel":-0.5}
{"t":"end","tick":2,"lines":3,"state":{}}
`, traceVersion)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 2}); err == nil || !strings.Contains(err.Error(), "record again") {
		t.Fatalf("ambiguous legacy input accepted for exact reproduction: %v", err)
	}
	r, err := loadReplay(path, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	var in input
	r.feed(&in, 1)
	if !in.moved || in.mouse != image.Pt(10, 20) || in.wheel != -0.5 {
		t.Fatalf("legacy ordinary replay changed: %+v", in)
	}
}

func TestTraceAutopilotTitleSaveRefused(t *testing.T) {
	fsys := wldWorldFiles(t, "", nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	store := savefile.MemStore()
	if err := store.Write("save1.json", saved); err != nil {
		t.Fatal(err)
	}
	var trace bytes.Buffer
	g = wldOpenWith(t, Options{FS: fsys, Store: store, Trace: &trace, Autopilot: &Autopilot{}})
	title, ok := g.stack[len(g.stack)-1].(*title)
	if !ok || title.menu.items[title.menu.sel].id != "continue" {
		t.Fatal("existing save did not select Continue")
	}
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	start := traceLines(t, trace.Bytes(), "start")[0]
	raw, err := json.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	var exact traceformat.ExactReplay
	if err := exact.Check("start", 0, raw); err == nil || !strings.Contains(err.Error(), "autopilot") {
		t.Fatalf("exact: %v", err)
	}
	path := filepath.Join(t.TempDir(), "auto.jsonl")
	if err := os.WriteFile(path, trace.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 1}); err == nil || !strings.Contains(err.Error(), "autopilot") {
		t.Fatalf("loader: %v", err)
	}
}

func TestTraceRecordingReplayRefused(t *testing.T) {
	g, trace := wldTracedGame(t, "wait 10\n")
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "original.jsonl")
	if err := os.WriteFile(path, trace.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	var recorded bytes.Buffer
	replayed := wldOpenWith(t, Options{FS: g.opts.FS, Replay: path, Until: 1, Trace: &recorded})
	if !replayed.rep.scripted {
		t.Fatal("source script provenance missing")
	}
	path = filepath.Join(t.TempDir(), "recorded.jsonl")
	if err := os.WriteFile(path, recorded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: 1}); err == nil || !strings.Contains(err.Error(), "recording a replay") {
		t.Fatalf("loader: %v", err)
	}
}

func TestTraceWriterRejectsOversizedRecord(t *testing.T) {
	var buf bytes.Buffer
	writer := lineWriter{w: &buf}
	err := writer.Encode(traceLine{"t": "say", "tick": 1, "text": strings.Repeat("x", traceformat.LineLimit)})
	if err == nil || !strings.Contains(err.Error(), "line limit") || buf.Len() != 0 {
		t.Fatalf("error=%v written=%d", err, buf.Len())
	}
}

func TestTraceAuthorToolConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*Options)
	}{
		{"review", func(o *Options) { o.Review = true; o.ReviewLog = "unused.jsonl" }},
		{"review-log", func(o *Options) { o.ReviewLog = "unused.jsonl" }},
		{"review-sources", func(o *Options) { o.ReviewSources = "unused.json" }},
		{"memos", func(o *Options) { o.MemoDir = "unused" }},
		{"external-input", func(o *Options) { o.ExternalInput = true }},
		{"build-info", func(o *Options) { o.CollectBuildInfo = true }},
		{"live reload", func(o *Options) { o.LiveDir = t.TempDir(); o.Script = "" }},
		{"F5 script", func(o *Options) { o.LiveDir = t.TempDir(); o.Script = "press reload\n" }},
		{"disabled watcher", func(o *Options) { o.LiveDir = t.TempDir() }},
		{"gallery-all", func(o *Options) { o.GalleryAll = true }},
		{"book-as-played", func(o *Options) { o.BookAsPlayed = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			o := Options{FS: wldWorldFiles(t, "", nil), Location: "camp", Seed: 5, Development: true, Trace: &buf, Script: "wait 5\nquit\n"}
			tc.configure(&o)
			o.NoSound, o.Store, o.ShotDir = true, savefile.MemStore(), t.TempDir()
			g, err := New(o)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := g.Close(); err != nil {
					t.Error(err)
				}
			})
			start := traceLines(t, buf.Bytes(), "start")[0]
			b, err := json.Marshal(start)
			if err != nil {
				t.Fatal(err)
			}
			var exact traceformat.ExactReplay
			err = exact.Check("start", 0, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			if o.Review {
				assertSourceRefused(t, &buf, "cg_review")
				return
			}
			replayed, err := wldReplayGame(t, buf.Bytes(), func(o *Options) { o.FS = g.opts.FS })
			if err != nil {
				t.Fatal(err)
			}
			if replayed.opts.GalleryAll != o.GalleryAll || replayed.opts.BookAsPlayed != o.BookAsPlayed || !replayed.opts.Development {
				t.Fatalf("author display options not restored: %+v", replayed.opts)
			}
		})
	}
}

func TestTraceInPlayTagRefusesAtInteraction(t *testing.T) {
	for _, picture := range []string{"", "pics/tall1.png"} {
		t.Run(picture, func(t *testing.T) {
			var buf bytes.Buffer
			logPath := filepath.Join(t.TempDir(), "review.jsonl")
			g := wldOpenWith(t, Options{FS: wldWorldFiles(t, mnuReviewYAML, nil), Location: "camp", Seed: 5, Trace: &buf, ReviewLog: logPath})
			start := traceLines(t, buf.Bytes(), "start")[0]
			check := func() {
				t.Helper()
				b, err := json.Marshal(start)
				if err != nil {
					t.Fatal(err)
				}
				var exact traceformat.ExactReplay
				if err := exact.Check("start", 0, b); err != nil {
					t.Fatalf("start refusal: %v", err)
				}
			}
			check() // Configuration alone does not affect the screen.
			g.tick = 3
			if picture != "" {
				g.Image(data.CG{File: picture})
			}
			g.in.pressed[btnTag] = true
			opened := g.updateTag()
			if picture == "" {
				if opened || g.tag == nil || g.tag.toast == "" || g.tag.toastT != 90 {
					t.Fatalf("no-CG toast: %+v", g.tag)
				}
			} else if !opened || g.tag == nil || g.tag.box == nil {
				t.Fatalf("tag editor: %+v", g.tag)
			}
			assertSourceRefused(t, &buf, "cg_review")
			assertReplaySourceBoundary(t, buf.Bytes(), "cg_review", 3)
			if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("tag submitted without input: %v", err)
			}
		})
	}
}

func TestTraceOptionClassification(t *testing.T) {
	b, err := os.ReadFile("../../docs/development/graphics.md")
	if err != nil {
		t.Fatal(err)
	}
	_, table, ok := strings.Cut(string(b), "### Author options and exact reproduction")
	if !ok {
		t.Fatal("missing option classification")
	}
	table, _, _ = strings.Cut(table, "### Reproduce a trace screen")
	classified := map[string]bool{}
	for line := range strings.SplitSeq(table, "\n") {
		if !strings.HasPrefix(line, "| `--") {
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) != 4 || strings.TrimSpace(columns[2]) == "" {
			t.Fatalf("missing policy: %s", line)
		}
		for option := range strings.SplitSeq(columns[1], ",") {
			name := strings.Trim(strings.TrimSpace(option), "`")
			name = strings.TrimPrefix(name, "--")
			if classified[name] {
				t.Errorf("duplicate classification: %s", name)
			}
			classified[name] = true
		}
	}
	Entry(nil, ".", EntryConfig{}).Flags.VisitAll(func(f *pflag.Flag) {
		if !classified[f.Name] {
			t.Errorf("option --%s needs a reproduction classification", f.Name)
		}
		delete(classified, f.Name)
	})
	for name := range classified {
		t.Errorf("classification has no registered option: --%s", name)
	}
}

// The target is Until-1: a source only invalidates targets including its tick.
func assertReplaySourceBoundary(t *testing.T, trace []byte, kind string, tick int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "boundary.jsonl")
	if err := os.WriteFile(path, trace, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path, &Options{Until: tick}); err != nil {
		t.Fatalf("before %s: %v", kind, err)
	}
	for _, until := range []int{tick + 1, tick + 2} {
		if _, err := loadReplay(path, &Options{Until: until}); err == nil || !strings.Contains(err.Error(), kind+" at tick") {
			t.Fatalf("including %s: %v", kind, err)
		}
	}
}

func TestTraceReloadBoundary(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(strconv.FormatBool(automatic), func(t *testing.T) {
			var buf bytes.Buffer
			dir := t.TempDir()
			g := wldOpenWith(t, Options{FS: wldWorldFiles(t, "", nil), Location: "camp", Seed: 5, Development: true, Trace: &buf})
			g.opts.LiveDir = dir
			g.live.setBase(liveFingerprint(dir))
			g.tick = 3
			if automatic {
				if err := os.Mkdir(filepath.Join(dir, "assets"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "assets", "picture.png"), []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
				g.live.changed.Store(true)
				g.updateLive()
			} else {
				g.in.pressed[btnReload] = true
				g.updateLive()
			}
			assertSourceRefused(t, &buf, "live_reload")
			assertReplaySourceBoundary(t, buf.Bytes(), "live_reload", 3)
		})
	}
}

func TestTraceReviewGalleryBoundary(t *testing.T) {
	var buf bytes.Buffer
	g := wldOpenWith(t, Options{FS: wldWorldFiles(t, mnuReviewYAML, nil), Location: "camp", Seed: 5, Trace: &buf, ReviewLog: filepath.Join(t.TempDir(), "review.jsonl"), GalleryAll: true})
	g.tick = 3
	newGallery(g)
	assertSourceRefused(t, &buf, "cg_review")
	assertReplaySourceBoundary(t, buf.Bytes(), "cg_review", 3)
}

// partyHitFiles makes a real enemy attack cross the actor's HP-face threshold.
func partyHitFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := gameFiles(t)
	files["game.yaml"].Data = append(files["game.yaml"].Data, []byte("schema_version: 7\n")...)
	body := string(files["data/main.yaml"].Data)
	body = strings.Replace(body, "scout: {name: Scout,", "scout: {faces: party, face: normal, images: [{below: 0.9, face: hurt}], name: Scout,", 1)
	body = strings.Replace(body, "mhp: 30, mmp: 0", "mhp: 500, mmp: 0", 1)
	body = strings.Replace(body, `damage: "1", variance: 0`, `damage: "25", variance: 0`, 1)
	files["data/main.yaml"].Data = []byte(body)
	files["assets/party/normal.png"] = &fstest.MapFile{Data: wldPNG(t, 64, 64, color.NRGBA{R: 40, G: 180, B: 80, A: 255})}
	files["assets/party/hurt.png"] = &fstest.MapFile{Data: wldPNG(t, 64, 64, color.NRGBA{R: 220, G: 80, B: 40, A: 255})}
	return files
}

const partyHitScript = "wait 90\npress ok\npress ok\nwait 107\nshot frame\nwait 30\nshot settled\nquit\n"

func TestTracePartyHitFixture(t *testing.T) {
	g := wldOpenWith(t, Options{FS: partyHitFiles(t), Location: "field", Battle: "one", Party: []string{"scout"}, Seed: 7, Script: partyHitScript})

	for range 300 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		shot := g.in.script.shot
		g.in.script.shot = "" // Pixel capture is exercised by TestGraphicsScreensRepro.
		g.Draw(btlImage())
		if shot != "frame" {
			continue
		}
		s := g.currentBattle()
		u := s.b.Party[0]
		if s.shownHP[u] != 25 || s.faceOf(u) != "hurt" || s.foeFade[u].ticks <= 0 || s.foeFade[u].ticks >= 18 {
			t.Fatalf("expected a real hit and an in-progress HP portrait fade at tick %d: HP %d face %s fade %+v", g.tick, s.shownHP[u], s.faceOf(u), s.foeFade[u])
		}
		t.Logf("party hit screenshot at tick %d, fade %d", g.tick, s.foeFade[u].ticks)
		return
	}
	t.Fatal("fixture did not capture the hit")
}

func TestTraceDrawCadencePartyHit(t *testing.T) {
	var buf bytes.Buffer
	g := wldOpenWith(t, Options{FS: partyHitFiles(t), Location: "field", Battle: "one", Party: []string{"scout"}, Seed: 7, Script: partyHitScript, Trace: &buf})
	screen := btlImage()
	skipped, gapEnd := 0, 0
	var want pictureFade
	var target int
	for range 300 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		s := g.currentBattle()
		u := s.b.Party[0]
		if gapEnd == 0 && s.faceOf(u) == "hurt" {
			gapEnd = g.tick + 3
		}
		if gapEnd > g.tick {
			skipped++
			continue
		}
		shot := g.in.script.shot
		g.in.script.shot = ""
		g.Draw(screen)
		if shot == "frame" {
			want, target = s.foeFade[u], g.tick
			break
		}
	}
	if skipped != 3 || target == 0 || want.ticks <= 0 || want.ticks >= 18 {
		t.Fatalf("skipped=%d target=%d fade=%+v", skipped, target, want)
	}
	g.traceEnd()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cadence.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	fast := wldOpenWith(t, Options{FS: partyHitFiles(t), Replay: path, Until: target + 1, Store: savefile.MemStore()})
	if got := fast.rep.drawSkips; !reflect.DeepEqual(got, [][2]int{{gapEnd - 3, gapEnd - 1}}) {
		t.Fatalf("cadence = %v", got)
	}
	for fast.tick < target {
		if err := fast.Update(); err != nil {
			t.Fatal(err)
		}
		fast.Draw(screen)
	}
	s := fast.currentBattle()
	if got := s.foeFade[s.b.Party[0]]; got != want {
		t.Fatalf("replayed fade %+v, want %+v", got, want)
	}
}

func TestTraceDrawCadence(t *testing.T) {
	var buf bytes.Buffer
	g := &Game{tr: newTracer(&buf)}
	if allocs := testing.AllocsPerRun(100, func() {
		g.tick++
		g.traceDrawGap(g.tick - 1)
	}); allocs != 0 || buf.Len() != 0 {
		t.Fatalf("ordinary cadence: %g allocations, %s", allocs, &buf)
	}
	g.tick += 4
	g.traceDrawGap(g.tick - 1)
	if lines := strings.Count(buf.String(), "\n"); lines != 1 {
		t.Fatalf("one gap must produce one record: %s", &buf)
	}
	for _, tc := range []struct {
		name  string
		spans [][2]int
	}{
		{"legacy", nil},
		{"gaps", [][2]int{{2, 4}, {7, 7}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &replay{drawSkips: tc.spans}
			for tick := range 10 {
				want := tc.name == "gaps" && (tick >= 2 && tick <= 4 || tick == 7)
				if r.skipDraw(tick) != want {
					t.Fatalf("tick %d: skip=%v", tick, !want)
				}
			}
		})
	}
}

func TestTraceDrawCadenceValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records string
		valid   bool
	}{
		{"legacy", "", true},
		{"tail", `{"t":"draw_skip","tick":5,"first":1,"last":5}`, true},
		{"zero", `{"t":"draw_skip","tick":5,"first":0,"last":4}`, false},
		{"reversed", `{"t":"draw_skip","tick":5,"first":4,"last":3}`, false},
		{"future", `{"t":"draw_skip","tick":5,"first":1,"last":6}`, false},
		{"fractional", `{"t":"draw_skip","tick":5,"first":1.5,"last":4}`, false},
		{"overlap", "{\"t\":\"draw_skip\",\"tick\":3,\"first\":1,\"last\":2}\n{\"t\":\"draw_skip\",\"tick\":5,\"first\":2,\"last\":4}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			body := "{\"t\":\"start\",\"tick\":0,\"exact_version\":3,\"version\":1,\"seed\":7}\n" + tc.records + "\n{\"t\":\"end\",\"tick\":5}\n"
			body = strings.ReplaceAll(body, "\n\n", "\n")
			err := os.WriteFile(path, []byte(body), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = loadReplay(path, &Options{Until: 5})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

// Trace and replay (ADR 0025, the site's "Trace and replay"): --trace records JSON Lines, and
// --replay plays the same inputs again and reports the first line that differs.

// wldRecordScript is a short play of the novel fixture: four lines of an event and a few presses.
const wldRecordScript = "wait 30\npress ok\nwait 40\npress ok\nwait 40\npress ok\nwait 40\nquit\n"

func wldTraceFS(t *testing.T) *Options {
	t.Helper()
	return &Options{FS: wldWorldFiles(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData})}
}

// wldRecord plays the novel fixture with a trace and returns the trace.
func wldRecord(t *testing.T, script string) []byte {
	t.Helper()
	var buf bytes.Buffer
	opts := wldTraceFS(t)
	opts.Location, opts.Event, opts.Seed, opts.Trace, opts.Script = "camp", "ev_auto", 7, &buf, script
	g := wldOpenWith(t, *opts)
	screen := wldScreen(g)
	for i := 0; i < 5000 && wldStep(t, g, screen); i++ {
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// wldReplayGame opens a replay of the trace bytes on the novel fixture.
func wldReplayGame(t *testing.T, trace []byte, mod func(*Options)) (*Game, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.jsonl")
	if err := os.WriteFile(path, trace, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := wldTraceFS(t)
	opts.Replay, opts.NoSound, opts.SaveDir = path, true, t.TempDir()
	if mod != nil {
		mod(opts)
	}
	g, err := New(*opts)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil && !errors.Is(err, ebiten.Termination) {
			t.Error(err)
		}
	})
	return g, nil
}

// wldReplay plays a replay out and returns the report.
func wldReplay(t *testing.T, g *Game) string {
	t.Helper()
	screen := wldScreen(g)
	for range 20000 {
		if !wldStep(t, g, screen) {
			return ReplayReport
		}
	}
	t.Fatal("the replay did not end")
	return ""
}

func TestWldTraceRecordsTheStructureOfAPlay(t *testing.T) {
	tr := wldRecord(t, wldRecordScript)
	lines := bytes.Split(bytes.TrimSpace(tr), []byte("\n"))
	if len(lines) < 10 {
		t.Fatalf("a trace of %d lines", len(lines))
	}
	start := traceLines(t, tr, "start")
	if len(start) != 1 || start[0]["version"] != 2.0 || start[0]["seed"] != 7.0 || start[0]["fingerprint"] == "" || start[0]["event"] != "ev_auto" {
		t.Fatalf("start line %v", start)
	}
	if !bytes.HasPrefix(lines[0], []byte(`{"t":"start","tick":0`)) {
		t.Fatalf("the first line is not the start at tick 0: %s", lines[0])
	}
	say := traceLines(t, tr, "say")
	if len(say) < 3 || say[0]["text"] != "One." || say[0]["who"] != "Courier" || say[1]["text"] != "Two." {
		t.Fatalf("say lines %v", say)
	}
	if in := traceLines(t, tr, "input"); len(in) < 3 {
		t.Fatalf("input lines %v", in)
	} else if p, ok := in[0]["press"].([]any); !ok || len(p) != 1 || p[0] != "ok" {
		t.Fatalf("the first input %v", in[0])
	}
	if sc := traceLines(t, tr, "scene"); len(sc) == 0 {
		t.Fatal("no scene line")
	}
	if st := traceLines(t, tr, "state"); len(st) < 2 || st[len(st)-1]["terminal"] != true {
		t.Fatalf("state lines %v", st)
	}
	end := traceLines(t, tr, "end")
	if len(end) != 1 || end[0]["lines"] != float64(len(lines)) {
		t.Fatalf("end line %v for %d lines", end, len(lines))
	}
	if !bytes.HasSuffix(bytes.TrimSpace(lines[len(lines)-1]), []byte("}")) || !strings.HasPrefix(string(lines[len(lines)-1]), `{"t":"end"`) {
		t.Fatalf("the last line is not the end: %s", lines[len(lines)-1])
	}
}

func TestWldTraceSameSeedSameTrace(t *testing.T) {
	covSlow(t)
	// Wall time and process-wide memory samples are observational. Keep every
	// other byte, including section boundaries, frame counts and tick counts.
	stable := func(src []byte) []byte {
		var out bytes.Buffer
		for line := range bytes.SplitSeq(bytes.TrimSpace(src), []byte("\n")) {
			var record traceLine
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatal(err)
			}
			if record["t"] == "section" {
				for _, field := range []string{"elapsed_seconds", "fps", "tps", "heap_inuse_bytes", "allocations", "slow_frame_share", "allocations_per_frame"} {
					delete(record, field)
				}
				var err error
				line, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
			}
			out.Write(line)
			out.WriteByte('\n')
		}
		return out.Bytes()
	}
	a := stable(wldRecord(t, wldRecordScript))
	b := stable(wldRecord(t, wldRecordScript))
	if !bytes.Equal(a, b) {
		t.Fatal("two plays with the same seed and inputs wrote different traces")
	}
}

func TestWldReplayMatchesItsRecording(t *testing.T) {
	covSlow(t)
	tr := wldRecord(t, wldRecordScript)
	g, err := wldReplayGame(t, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	report := wldReplay(t, g)
	if !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatalf("report %q", report)
	}
	if g.rep.diverge != "" || g.rep.matched == 0 {
		t.Fatalf("diverge %q matched %d", g.rep.diverge, g.rep.matched)
	}
}

func TestWldReplayReportsTheFirstDifference(t *testing.T) {
	covSlow(t)
	tr := wldRecord(t, wldRecordScript)
	changed := bytes.Replace(tr, []byte(`"text":"Two."`), []byte(`"text":"Zwei."`), 1)
	if bytes.Equal(changed, tr) {
		t.Fatal("the trace has no line to change")
	}
	g, err := wldReplayGame(t, changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	report := wldReplay(t, g)
	if !strings.HasPrefix(report, "replay diverged: tick ") || !strings.Contains(report, "the say line") || !strings.Contains(report, "Zwei.") || !strings.Contains(report, "Two.") {
		t.Fatalf("report %q", report)
	}
}

func TestWldReplayRangeStopsAtUntil(t *testing.T) {
	covSlow(t)
	tr := wldRecord(t, wldRecordScript)
	g, err := wldReplayGame(t, tr, func(o *Options) { o.Until = 60 })
	if err != nil {
		t.Fatal(err)
	}
	report := wldReplay(t, g)
	if !strings.HasPrefix(report, "replay range matched:") || !strings.Contains(report, "up to tick 59") {
		t.Fatalf("report %q", report)
	}
}

func TestWldReplayRefusesABrokenOrChangedRecord(t *testing.T) {
	covSlow(t)
	tr := string(wldRecord(t, wldRecordScript))
	lines := strings.Split(strings.TrimSpace(tr), "\n")
	join := func(ls ...string) []byte { return []byte(strings.Join(ls, "\n") + "\n") }
	firstInput := -1
	for i, l := range lines {
		if strings.HasPrefix(l, `{"t":"input"`) {
			firstInput = i
			break
		}
	}
	if firstInput < 0 {
		t.Fatal("no input line to build on")
	}
	var inputFrame struct {
		Tick int `json:"tick"`
	}
	if err := json.Unmarshal([]byte(lines[firstInput]), &inputFrame); err != nil {
		t.Fatal(err)
	}
	atInput := func(record string) string {
		return strings.Replace(record, `"tick":2`, `"tick":`+strconv.Itoa(inputFrame.Tick), 1)
	}
	cases := []struct {
		name  string
		trace []byte
		mod   func(*Options)
		want  string
	}{
		{"empty", nil, nil, "has no start line"},
		{"no start first", join(lines[1:]...), nil, "does not begin with a start line"},
		{"second start", join(append([]string{lines[0], lines[0]}, lines[1:]...)...), nil, "has more than one start line"},
		{"ticks go back", join(append(append([]string{}, lines[:firstInput]...), `{"t":"input","tick":5,"press":["ok"]}`, `{"t":"input","tick":3,"press":["ok"]}`)...), nil, "invalid order, end or tick"},
		{"lines after the end", join(append(append([]string{}, lines...), `{"t":"input","tick":99999,"press":["ok"]}`)...), nil, "invalid order, end or tick"},
		{"input at tick 0", join(append([]string{lines[0], `{"t":"input","tick":0,"press":["ok"]}`}, lines[1:]...)...), nil, "input must be at tick 1 or later"},
		{"unknown button", join(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"input","tick":2,"press":["jump"]}`))...), nil, `unknown button "jump"`},
		{"button not a word", join(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"input","tick":2,"held":[false]}`))...), nil, "button names in held must be strings"},
		{"repeated input tick", join(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"input","tick":2,"press":["ok"]}`), atInput(`{"t":"input","tick":2,"press":["ok"]}`))...), nil, "repeats input tick " + strconv.Itoa(inputFrame.Tick)},
		{"unknown action", join(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"act","tick":2,"name":"fly"}`))...), nil, `unknown direct action "fly"`},
		{"action at tick 0", join(append([]string{lines[0], `{"t":"act","tick":0,"name":"run"}`}, lines[1:]...)...), nil, "direct actions must be at tick 1 or later"},
		{"cut off", join(lines[:len(lines)-1]...), nil, "no end (the record was cut off)"},
		{"line count", join(append(append([]string{}, lines[:len(lines)-1]...), strings.Replace(lines[len(lines)-1], `"lines":`, `"lines":1`, 1))...), nil, "line count does not match its end"},
		{"unsupported version", []byte(strings.Replace(tr, `"version":2`, `"version":9`, 1)), nil, "record version 9 is not supported"},
		{"old version without until", []byte(strings.Replace(tr, `"version":2`, `"version":1`, 1)), nil, "old record cannot verify its preconditions"},
		{"until past the end", []byte(tr), func(o *Options) { o.Until = 1 << 30 }, "--until is past the end of the record"},
		{"negative until", []byte(tr), func(o *Options) { o.Until = -1 }, "--until needs a positive tick"},
		{"not JSON", join(append([]string{lines[0], "garbage"}, lines[1:]...)...), nil, "run.jsonl"},
		{"no tick", join(append([]string{lines[0], `{"t":"input","press":["ok"]}`}, lines[1:]...)...), nil, "invalid order, end or tick"},
		{"no kind", join(append([]string{lines[0], `{"tick":1}`}, lines[1:]...)...), nil, "invalid order, end or tick"},
		{"no start settings", []byte(strings.Replace(tr, `"setup":`, `"xsetup":`, 1)), nil, "lacks its start settings"},
		{"location not a word", []byte(strings.Replace(tr, `"location":"camp"`, `"location":5`, 1)), nil, "invalid location"},
		{"calm not a flag", []byte(strings.Replace(tr, `"calm":false`, `"calm":"yes"`, 1)), nil, "invalid calm"},
		{"battle not a word", []byte(strings.Replace(tr, `"battle":""`, `"battle":5`, 1)), nil, "invalid battle"},
		{"action without a name", join(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"act","tick":2,"name":5}`))...), nil, "action name must be a string"},
		{"reload partway", wldJoinTrace(append(append(append([]string{}, lines[:firstInput]...), atInput(`{"t":"live_reload","tick":2,"ok":true}`)), lines[firstInput:]...)), nil, "asset or data reload partway"},
		{"end without state", join(append(append([]string{}, lines[:len(lines)-1]...), `{"t":"end","tick":186,"lines":`+strconv.Itoa(len(lines))+`}`)...), nil, "lacks its end state"},
		{"seed zero", []byte(strings.Replace(tr, `"seed":7`, `"seed":0`, 1)), nil, "seed"},
		{"other saves", []byte(strings.Replace(tr, `"saves":{}`, `"saves":{"a.json":"00"}`, 1)), nil, "save slots the replay needs do not match"},
		{"data changed", []byte(tr), func(o *Options) {
			o.FS = wldWorldFiles(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData + "  ev_extra:\n    do:\n      - say: \"More.\"\n"})
		}, "data fingerprint does not match"},
		{"missing file", []byte(tr), func(o *Options) { o.Replay += ".none" }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := wldReplayGame(t, c.trace, c.mod)
			if err == nil {
				t.Fatalf("accepted the record; report %q", func() string {
					if g == nil {
						return ""
					}
					return wldReplay(t, g)
				}())
			}
			if c.name == "missing file" {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("error %q, want a missing file", err)
				}
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q, want it to say %q", err, c.want)
			}
		})
	}
}

// The outcome is emitted before finishOffer discards the receiver result.
func TestTraceInputOutcome(t *testing.T) {
	var b bytes.Buffer
	g := &Game{tr: newTracer(&b), evName: "gate"}
	g.in.offer = &scriptOffer{offered: true, command: scriptCmd{verb: "press"}}
	g.traceInputOutcome(true)
	g.tick = 20
	g.in.offer.accepted = true
	g.traceInputOutcome(true)
	lines := traceLines(t, b.Bytes(), "input_result")
	if len(lines) != 2 || lines[0]["accepted"] != false || lines[1]["accepted"] != true ||
		lines[1]["phone"] != true || lines[1]["event"] != "gate" {
		t.Fatalf("outcomes: %v", lines)
	}
	b.Reset()
	g.in.offer = nil
	g.auto = &autopilot{}
	g.talk.active = true
	g.talk.choice = &list{}
	g.traceInputOutcome(false)
	g.traceInputOutcome(false)
	g.talk.active = false
	g.traceInputOutcome(false)
	lines = traceLines(t, b.Bytes(), "autoplay_wait")
	if len(lines) != 2 || lines[0]["waiting"] != "choice" || lines[1]["waiting"] != "" {
		t.Fatalf("wait transitions: %v", lines)
	}
}

func TestPhoneGameReplayClockAndIdentity(t *testing.T) {
	var recorded bytes.Buffer
	clock := time.Date(2030, 1, 2, 3, 4, 5, 123456789, time.FixedZone("fixture", 9*3600))
	inst := &gamepackage.Installed{Release: gamepackage.Release{ID: "test/game", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}}
	g := &Game{tick: 17, tr: newTracer(&recorded), opts: Options{Now: func() time.Time { return clock }}}
	first, err := g.phoneGameClock(inst)
	if err != nil {
		t.Fatal(err)
	}
	var launch phoneLaunchClock
	if err = json.Unmarshal(bytes.TrimSpace(recorded.Bytes()), &launch); err != nil {
		t.Fatal(err)
	}
	if first.UnixMilli() != clock.UnixMilli() || launch.Offset != 9*3600 {
		t.Fatal("launch clock/offset changed")
	}
	replayed := &Game{tick: 17, rep: &replay{phoneLaunches: map[int]phoneLaunchClock{17: launch}}}
	second, err := replayed.phoneGameClock(inst)
	if err != nil || !first.Equal(second) {
		t.Fatalf("replay clock: %v %v", second, err)
	}
	inst.Release.SHA256 = strings.Repeat("b", 64)
	if _, err = replayed.phoneGameClock(inst); err == nil {
		t.Fatal("changed package accepted")
	}
	replayed.tick = 18
	if _, err = replayed.phoneGameClock(inst); err == nil {
		t.Fatal("unrecorded launch accepted")
	}
}
