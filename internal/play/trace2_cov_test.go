package play

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/event"
)

// More of what a trace records: the inputs with the mouse, the kinds of requests, the scene
// stack, and what a replay cannot take (a game started with options it cannot reproduce).

func wldTracedGame(t *testing.T, script string) (*Game, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts := Options{Location: "camp", Seed: 5, Trace: &buf, Script: script, FS: wldWorldFiles(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData})}
	return wldOpenWith(t, opts), &buf
}

func TestWldTraceInputLinesCarryMouseDigitsAndFast(t *testing.T) {
	g, buf := wldTracedGame(t, "wait 3\nclick 120 340\nwait 10\npress 3\nwait 5\nskip 4\nwait 5\nquit\n")
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
		w, _ := l["what"].(string)
		switch w {
		case "battle":
			w += ":" + l["troop"].(string)
		case "shop":
			w += ":" + l["id"].(string)
		case "open":
			w += ":" + l["screen"].(string)
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
		for _, n := range l["stack"].([]any) {
			names = append(names, n.(string))
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
	wldFrames(t, g, 40)
	views := traceLines(t, buf.Bytes(), "view")
	if len(views) == 0 {
		t.Fatal("no view line after the picture stood still")
	}
	var found bool
	for _, v := range views {
		for _, p := range v["pictures"].([]any) {
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
	store := gozen.MemStore()
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
	saves, _ := start["setup"].(map[string]any)["saves"].(map[string]any)
	if len(saves) != 1 {
		t.Fatalf("saves in the setup %v", saves)
	}
	for name, sum := range saves {
		if want := fmt.Sprintf("%x", sha256.Sum256([]byte(`{"x":1}`))); name != "save1.json" || sum != want {
			t.Fatalf("save %q hashed as %v", name, sum)
		}
	}
}
