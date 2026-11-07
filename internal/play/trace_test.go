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
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/event"
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
				data := append([]byte(`{"t":"start","tick":0,"version":1,"exact_version":1}`+"\n"), buf.Bytes()...)
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
			data := fmt.Sprintf("{\"t\":\"start\",\"tick\":0,\"version\":1,\"exact_version\":1}\n{\"t\":\"input\",\"tick\":1,%q:[\"\"]}\n{\"t\":\"end\",\"tick\":2}\n", field)
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

func TestWldReplayRefusesOptionsItCannotReproduce(t *testing.T) {
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
	_, err := wldReplayGame(t, buf.Bytes(), func(o *Options) { o.FS = wldWorldFiles(t, "", nil) })
	if err == nil || !strings.Contains(err.Error(), "cannot reproduce the start conditions [review]") {
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

func TestTraceReproBatchesPreserveTargetState(t *testing.T) {
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
	loop := &seekLoop{Game: fast, drew: true}
	calls := 0
	for !fast.untilShot {
		if err := loop.Update(); err != nil {
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
	// This event-driven replay now draws each tick; idle batching is checked separately.
	if calls > 100 {
		t.Fatalf("seek did not reach its target promptly: %d calls", calls)
	}
	fast.autoShot = "" // A successful target Draw releases the stop handshake.
	for range 3 {
		if err := loop.Update(); errors.Is(err, errQuit) {
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
	var exact ExactReplay
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

func TestTraceReproBatchesIdleTicks(t *testing.T) {
	g := wldOpenWith(t, Options{FS: gameFiles(t), Location: "camp", Until: 100})
	g.rep = &replay{partial: true}
	loop := &seekLoop{Game: g, drew: true}
	if err := loop.Update(); err != nil {
		t.Fatal(err)
	}
	if g.tick <= 1 {
		t.Fatalf("idle seek did not batch: tick %d", g.tick)
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
	trace := `{"t":"start","tick":0,"exact_version":1,"version":1,"seed":7}
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
	for kind := range seen {
		if exactReplayKinds[kind] == inexactReplayKinds[kind] {
			t.Errorf("record %q must be in exactly one replay classification", kind)
		}
	}
	for _, group := range []map[string]bool{exactReplayKinds, inexactReplayKinds} {
		for kind := range group {
			if !seen[kind] {
				t.Errorf("stale classification %q", kind)
			}
		}
	}
}

func TestReplayUntilRejectsUnlistedRecordsAtTarget(t *testing.T) {
	for _, kind := range append(sortedKeys(inexactReplayKinds), "future_kind") {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			trace := fmt.Sprintf("{\"t\":\"start\",\"tick\":0,\"exact_version\":1,\"version\":1}\n{\"t\":%q,\"tick\":2}\n", kind)
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
		var exact ExactReplay
		if err := exact.Check(kind, int(num(line["tick"])), raw); err == nil {
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

func TestExactReplayRequiresSourceAuditRevision(t *testing.T) {
	for _, revision := range []int{0, 2} {
		var exact ExactReplay
		err := exact.Check("start", 0, fmt.Appendf(nil, `{"exact_version":%d}`, revision))
		if err == nil || !strings.Contains(err.Error(), "record again") {
			t.Fatalf("revision %d: %v", revision, err)
		}
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
			var exact ExactReplay
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
				previous := 0
				inputs := map[int]bool{}
				for line := range bytes.Lines(buf.Bytes()) {
					var record traceLine
					if err := json.Unmarshal(line, &record); err != nil {
						t.Fatal(err)
					}
					tick := int(num(record["tick"]))
					if tick < previous {
						t.Fatalf("decreasing tick %d after %d: %s", tick, previous, line)
					}
					previous = tick
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
						if num(record["tick"]) != 20 && len(strs(record["press"])) > 0 && len(strs(record["held"])) > 0 && record["mouse"] != nil {
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
		"snapshot": "observation", "look": "observation", "quit": "termination",
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
	var exact ExactReplay
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
