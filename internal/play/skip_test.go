package play

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
)

// novelData is a short story: two lines, a third, a choice and a line after it, and an event
// it calls.
const novelData = `events:
  story:
    do:
      - say: "The lamps are lit."
      - say: {who: Courier, text: "Gold {gold}."}
      - say: "A bell rings far off."
      - choice:
          - {label: Wait, do: [{say: "You wait."}]}
          - {label: Go, do: [{call: aside}]}
      - say: "The night goes on."
  aside:
    do:
      - if: "flag.ghost"
        then: [{say: "A light in the distance."}]
`

func novelGame(t *testing.T, opts Options) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/novel.yaml"] = &fstest.MapFile{Data: []byte(novelData)}
	opts.FS = fsys
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

// runToEnd updates the game until its script quits.
func runToEnd(t *testing.T, g *Game) {
	t.Helper()
	for range 2000 {
		if err := g.Update(); errors.Is(err, ebiten.Termination) {
			return
		} else if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("the script did not quit")
}

// traceLines are the lines of a trace of the given kinds.
func traceLines(t *testing.T, b []byte, kinds ...string) []traceLine {
	t.Helper()
	var out []traceLine
	for ln := range bytes.Lines(b) {
		var l traceLine
		if err := json.Unmarshal(ln, &l); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(kinds, fmt.Sprint(l["t"])) {
			out = append(out, l)
		}
	}
	return out
}

func source(text string) uint64 { return event.SourceKey("", text) }

// TestReadRecord: read.json is an envelope with the game's id; a bare list of keys is read
// and kept as a backup on the next write; another game's record or a newer one is not
// overwritten (ADR 0026).
func TestReadRecord(t *testing.T) {
	g := openGame(t, Options{})
	legacy := []byte("[1, 2]")
	if err := g.store.Write("read.json", legacy); err != nil {
		t.Fatal(err)
	}
	g.loadRead()
	if !g.read[1] || !g.read[2] || !g.readNeedsBackup || g.readErr != nil {
		t.Fatalf("legacy record: %v %v %v", g.read, g.readNeedsBackup, g.readErr)
	}
	g.markKey(3)
	g.flushRead()
	b, err := g.store.Read("read.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"format_version":1,"game_id":"screen-test","keys":[1,2,3]}`; string(b) != want {
		t.Fatalf("written %s, want %s", b, want)
	}
	if old, err := g.store.Read(fmt.Sprintf("read.json.before-%x.bak", sha256.Sum256(legacy))); err != nil || !bytes.Equal(old, legacy) {
		t.Fatalf("no backup of the legacy record: %q %v", old, err)
	}
	for _, c := range []struct{ record, want string }{
		{`{"format_version":1,"game_id":"another","keys":[1]}`, "another game"},
		{`{"format_version":2,"game_id":"screen-test","keys":[1]}`, "format 2"},
		{`{"format_version":1,"game_id":"screen-test","keys":[1],"extra":true}`, "unknown field"},
		{`{"format_version":1,"game_id":"screen-test"}`, "no record"},
	} {
		if err := g.store.Write("read.json", []byte(c.record)); err != nil {
			t.Fatal(err)
		}
		g.loadRead()
		if g.readErr == nil || !strings.Contains(g.readErr.Error(), c.want) {
			t.Errorf("%s: %v", c.record, g.readErr)
		}
		g.markKey(9)
		g.flushRead()
		if b, err := g.store.Read("read.json"); err != nil || string(b) != c.record {
			t.Errorf("%s was overwritten with %s", c.record, b)
		}
	}
}

// TestReadByLegacyKey: a line read before the source keys still counts as read.
func TestReadByLegacyKey(t *testing.T) {
	g := openGame(t, Options{})
	s := event.Say{Name: "Courier", Text: "Gold 5.", Key: event.SourceKey("Courier", "Gold {gold}.")}
	g.read = map[uint64]bool{legacyKey(s): true}
	if !g.isRead(s) {
		t.Fatal("the legacy key is not read")
	}
	g.markRead(s)
	if !g.read[s.Key] {
		t.Fatal("the source key was not written")
	}
}

// TestEventLinesAndReadRate: an event's lines are collected into choice branches, if
// branches and called events, whatever the conditions.
func TestEventLinesAndReadRate(t *testing.T) {
	g := novelGame(t, Options{})
	got := g.eventLines("story")
	want := []uint64{
		source("The lamps are lit."), event.SourceKey("Courier", "Gold {gold}."), source("A bell rings far off."),
		source("You wait."), source("A light in the distance."), source("The night goes on."),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lines %x, want %x", got, want)
	}
	g.read = map[uint64]bool{want[0]: true, want[4]: true}
	if read, all := g.readRate("story"); read != 2 || all != 6 {
		t.Fatalf("read rate %d/%d, want 2/6", read, all)
	}
}

// TestSkipModeStopsAtUnread: skip mode passes the lines read and stops before the first
// line that was not, so the trace has no unread line while skipping.
func TestSkipModeStopsAtUnread(t *testing.T) {
	g := novelGame(t, Options{Location: "camp", Event: "story", Script: "wait 10\npress skip\nwait 120\nquit\n", ShotDir: t.TempDir()})
	g.read = map[uint64]bool{source("The lamps are lit."): true, event.SourceKey("Courier", "Gold {gold}."): true}
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	lines := traceLines(t, out.Bytes(), "say", "skip")
	var got []string
	for _, l := range lines {
		got = append(got, fmt.Sprintf("%v %v %v %v", l["t"], l["text"], l["read"], l["why"]))
	}
	want := []string{
		"say The lamps are lit. true <nil>",
		"skip <nil> <nil> <nil>",
		"say Gold 100. true <nil>",
		"skip <nil> <nil> unread",
		"say A bell rings far off. false <nil>",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("trace:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if g.skipping || g.evLines != 3 {
		t.Fatalf("skipping %v at line %d", g.skipping, g.evLines)
	}
}

// TestSceneSkipStopsAtChoice: when every line of the event was read, scene skip replays at
// once to the next choice.
func TestSceneSkipStopsAtChoice(t *testing.T) {
	g := novelGame(t, Options{Location: "camp", Event: "story", Script: "wait 10\npress scene_skip\nwait 10\nquit\n", ShotDir: t.TempDir()})
	g.read = map[uint64]bool{}
	for _, k := range g.eventLines("story") {
		g.read[k] = true
	}
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	skips := traceLines(t, out.Bytes(), "skip")
	if len(skips) != 2 || skips[0]["on"] != true || skips[0]["scene"] != true || skips[1]["why"] != "choice" {
		t.Fatalf("skip lines %v", skips)
	}
	if g.evLines != 4 || g.talk.choice == nil {
		t.Fatalf("at line %d, choice %v", g.evLines, g.talk.choice != nil)
	}
}

// TestSceneSkipNeedsEveryLineRead: a line not read anywhere in the event (even in a branch
// not taken) keeps scene skip off.
func TestSceneSkipNeedsEveryLineRead(t *testing.T) {
	g := novelGame(t, Options{Location: "camp", Event: "story", Script: "wait 10\npress scene_skip\nwait 10\nquit\n", ShotDir: t.TempDir()})
	g.read = map[uint64]bool{}
	for _, k := range g.eventLines("story") {
		g.read[k] = true
	}
	delete(g.read, source("A light in the distance."))
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	if skips := traceLines(t, out.Bytes(), "skip"); len(skips) != 0 || g.evLines != 1 {
		t.Fatalf("skipped %v to line %d", skips, g.evLines)
	}
}
