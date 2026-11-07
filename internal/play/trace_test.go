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
	trace := `{"t":"start","tick":0,"version":1,"seed":7}
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
			trace := fmt.Sprintf("{\"t\":\"start\",\"tick\":0,\"version\":1}\n{\"t\":%q,\"tick\":2}\n", kind)
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
