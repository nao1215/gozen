package play

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
)

// btlRecord plays a scripted battle with the script and returns the trace it wrote.
func btlRecord(t *testing.T, troop, script string) []byte {
	t.Helper()
	var out bytes.Buffer
	g, _ := btlOpen(t, Options{Battle: troop, Location: "camp", Script: script, ShotDir: t.TempDir(), Trace: &out, Seed: 11}, "", "")
	btlRunToEnd(t, g, 8000)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if len(traceLines(t, out.Bytes(), "battle_result")) == 0 {
		t.Fatal("the scripted battle did not reach its end")
	}
	return out.Bytes()
}

func btlWriteTrace(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "run.jsonl")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// btlReplay replays battle n of a trace and returns the report and the traced game.
func btlReplay(t *testing.T, path string, n int) string {
	t.Helper()
	g, err := New(Options{FS: btlFiles(t, "", ""), NoSound: true, SaveDir: t.TempDir(), Replay: path, ReplayBattle: n, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	}()
	btlRunToEnd(t, g, 12000)
	return ReplayReport
}

// TestBattleReplayReproducesABattle: a battle recorded with a player's inputs is replayed from
// its capture alone, and the replay matches every step, decision and reward and says so.
func TestBattleReplayReproducesABattle(t *testing.T) {
	covSlow(t)
	trace := btlRecord(t, "two", "press ok 160\nquit\n")
	lines := traceLines(t, trace, "battle_capture", "battle_result")
	if len(lines) != 2 {
		t.Fatalf("capture/result lines %d", len(lines))
	}
	report := btlReplay(t, btlWriteTrace(t, trace), 1)
	if strings.Contains(report, "diverged") || report == "" {
		t.Fatalf("report %q", report)
	}
	if !strings.Contains(report, "戦闘一致") && !strings.Contains(strings.ToLower(report), "battle") {
		t.Fatalf("not a battle replay report: %q", report)
	}
}

// TestBattleReplayDetectsADifference: a recorded step that no longer matches is reported as a
// divergence, not accepted.
func TestBattleReplayDetectsADifference(t *testing.T) {
	covSlow(t)
	trace := btlRecord(t, "two", "press ok 160\nquit\n")
	var edited [][]byte
	changed := false
	for ln := range bytes.SplitSeq(bytes.TrimSpace(trace), []byte("\n")) {
		if !changed && bytes.Contains(ln, []byte(`"t":"battle_step"`)) && bytes.Contains(ln, []byte(`"amount":`)) && !bytes.Contains(ln, []byte(`"amount":0`)) {
			ln = bytes.Replace(ln, []byte(`"amount":`), []byte(`"amount":9`), 1)
			changed = true
		}
		edited = append(edited, ln)
	}
	if !changed {
		t.Fatal("no damage step to change")
	}
	report := btlReplay(t, btlWriteTrace(t, bytes.Join(edited, []byte("\n"))), 1)
	if !strings.Contains(report, "diverged") {
		t.Fatalf("a changed step was accepted: %q", report)
	}
}

// TestBattleReplayRefusals: traces that cannot be replayed as a battle are refused with a
// message at load, before any frame is played.
func TestBattleReplayRefusals(t *testing.T) {
	covSlow(t)
	good := btlRecord(t, "one", "press ok 160\nquit\n")
	goodLines := bytes.Split(bytes.TrimSpace(good), []byte("\n"))
	join := func(ls ...[]byte) []byte { return append(bytes.Join(ls, []byte("\n")), '\n') }
	find := func(kind string) []byte {
		for _, l := range goodLines {
			if bytes.Contains(l, []byte(`"t":"`+kind+`"`)) {
				return l
			}
		}
		t.Fatalf("no %s line", kind)
		return nil
	}
	start, capture := find("start"), find("battle_capture")
	tick := func(n int) string { return `"tick":` + strconv.Itoa(n) }
	for name, tc := range map[string]struct {
		trace []byte
		n     int
	}{
		"not json":           {[]byte("garbage\n"), 1},
		"no tick":            {[]byte(`{"t":"start"}` + "\n"), 1},
		"not a start":        {[]byte(`{"t":"scene",` + tick(0) + `}` + "\n"), 1},
		"no such battle":     {good, 9},
		"ended first":        {join(start, []byte(`{"t":"end",`+tick(0)+`}`)), 1},
		"no result":          {join(start, capture), 1},
		"next before result": {join(start, capture, capture), 1},
		"time goes back":     {join(start, capture, []byte(`{"t":"input","tick":1}`), []byte(`{"t":"input","tick":0}`)), 1},
		"reload":             {join(start, capture, []byte(`{"t":"reload","tick":99999}`)), 1},
		"unknown button":     {join(start, capture, []byte(`{"t":"input","tick":99999,"press":["warp"]}`)), 1},
		"two inputs":         {join(start, capture, []byte(`{"t":"input","tick":99999,"press":["ok"]}`), []byte(`{"t":"input","tick":99999,"press":["ok"]}`)), 1},
		"direct act":         {join(start, capture, []byte(`{"t":"act","tick":99999,"name":"run"}`)), 1},
		"act without name":   {join(start, capture, []byte(`{"t":"act","tick":99999}`)), 1},
	} {
		path := btlWriteTrace(t, tc.trace)
		g, err := New(Options{FS: btlFiles(t, "", ""), NoSound: true, SaveDir: t.TempDir(), Replay: path, ReplayBattle: tc.n, Seed: 1})
		if err == nil {
			t.Errorf("%s: accepted", name)
			btlClose(t, g)
		}
	}
	if g, err := New(Options{FS: btlFiles(t, "", ""), NoSound: true, SaveDir: t.TempDir(), Replay: filepath.Join(t.TempDir(), "missing.jsonl"), ReplayBattle: 1, Seed: 1}); err == nil {
		t.Error("a missing trace file was accepted")
		btlClose(t, g)
	}
	for name, o := range map[string]Options{
		"battle number without replay": {ReplayBattle: 1},
		"negative number":              {ReplayBattle: -1, Replay: "x"},
		"with until":                   {ReplayBattle: 1, Replay: "x", Until: 5},
	} {
		o.FS, o.NoSound, o.SaveDir = btlFiles(t, "", ""), true, t.TempDir()
		if g, err := New(o); err == nil {
			t.Errorf("%s: accepted", name)
			btlClose(t, g)
		}
	}
}

// TestBattleCaptureIsRefusedWhenTheGameChanged: a capture is only played against the same
// data, the same engine version, a sane start and fighters that exist.
func TestBattleCaptureIsRefusedWhenTheGameChanged(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	c := btlLines(t, out, "battle_capture")
	if len(c) != 1 {
		t.Fatalf("%d captures", len(c))
	}
	raw, err := json.Marshal(c[0]["capture"])
	if err != nil {
		t.Fatal(err)
	}
	var base battleCapture
	if err := json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	if err := base.validate(g); err != nil {
		t.Fatalf("a fresh capture is refused: %v", err)
	}
	for name, mutate := range map[string]func(*battleCapture){
		"version":     func(c *battleCapture) { c.Version = 2 },
		"no save":     func(c *battleCapture) { c.Save = nil },
		"no rand":     func(c *battleCapture) { c.Rand = nil },
		"neg tick":    func(c *battleCapture) { c.Tick = -1 },
		"neg fade":    func(c *battleCapture) { c.Fade = -1 },
		"troop":       func(c *battleCapture) { c.Troop = "nobody" },
		"fingerprint": func(c *battleCapture) { c.Fingerprint = "0" },
		"no print":    func(c *battleCapture) { c.Fingerprint = "" },
		"engine":      func(c *battleCapture) { c.Engine = "other" },
		"no engine":   func(c *battleCapture) { c.Engine = "" },
		"fighter":     func(c *battleCapture) { c.Fighters = []string{"ghost"} },
	} {
		c := base
		mutate(&c)
		if err := c.validate(g); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestBattleTraceStepsAndComparable: the step line carries who did what to whom, and documents compare
// by content, not by key order.
func TestBattleTraceStepsAndComparable(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	g.traceBattleStep(battle.Step{Kind: battle.Damage, User: s.b.Party[0], Target: s.b.Foes[0], Amount: 12, Crit: true, Skill: "attack"})
	l := btlLines(t, out, "battle_step")
	last := l[len(l)-1]
	user, target := btlDoc(t, last["user"]), btlDoc(t, last["target"])
	if user["id"] != "scout" || user["enemy"] != false || target["id"] != "mote" || target["enemy"] != true || last["amount"] != 12.0 || last["crit"] != true {
		t.Fatalf("step %v", last)
	}
	if battleComparable(`{"a":1,"b":[1,2.5]}`) != battleComparable(`{"b":[1,2.5],"a":1}`) {
		t.Fatal("documents differing only in key order compare unequal")
	}
	if battleComparable(`{"a":1}`) == battleComparable(`{"a":2}`) {
		t.Fatal("different documents compare equal")
	}
	if battleComparable("not json") != "not json" {
		t.Fatal("a non-document is changed")
	}
}
