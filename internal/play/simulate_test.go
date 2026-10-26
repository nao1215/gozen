package play

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"
)

// simulateScript is a script over the camp of gameFiles: a line said, a battle fought on
// AUTO, money given, a picture asked for, and the state written out.
const simulateScript = "gap 4\nwait 10\ndo say: \"The fire is lit.\"\nuntil idle\ndo battle: one\nuntil idle\nuntil idle\ndo gold: 5\nshot fire\nsnapshot\nwait 3\nquit\n"

// simTrace runs Simulate over gameFiles and returns its trace lines.
func simTrace(t *testing.T, script string, lines string) ([]traceLine, SimulateResult) {
	t.Helper()
	var out bytes.Buffer
	cfg := SimulateConfig{}
	if lines != "" {
		cfg.Lines = strings.NewReader(lines)
	}
	res, err := Simulate(context.Background(), Options{FS: gameFiles(t), Location: "camp", Seed: 3, Script: script, Trace: &out}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got []traceLine
	for l := range strings.Lines(out.String()) {
		var m traceLine
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%v: %q", err, l)
		}
		got = append(got, m)
	}
	return got, res
}

// simKinds is the lines of these kinds with their ticks, as text to compare; the count
// of lines in end is left out (a line-by-line run has its ready lines too).
func simKinds(lines []traceLine, kinds ...string) []string {
	var out []string
	for _, l := range lines {
		for _, k := range kinds {
			if l["t"] == k {
				l = maps.Clone(l)
				if k == "end" {
					delete(l, "lines")
				}
				out = append(out, comparableLine(l)+"@"+jsonOf(l["tick"]))
			}
		}
	}
	return out
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// TestSimulateWritesTheTraceOfAScript: a script runs without a window or a screenshot
// directory and the trace has its lines, the snapshot, and the end with the state; shot
// takes no picture and writes no shot line.
func TestSimulateWritesTheTraceOfAScript(t *testing.T) {
	lines, res := simTrace(t, simulateScript, "")
	if res.Ticks == 0 || res.Lines != len(lines) {
		t.Fatalf("result %+v for %d lines", res, len(lines))
	}
	count := map[string]int{}
	for _, l := range lines {
		kind, ok := l["t"].(string)
		if !ok {
			t.Fatalf("no kind: %v", l)
		}
		count[kind]++
	}
	for _, k := range []string{"start", "say", "dev_do", "battle", "snapshot", "end"} {
		if count[k] == 0 {
			t.Errorf("no %s line: %v", k, count)
		}
	}
	if count["shot"] != 0 || count["ready"] != 0 {
		t.Errorf("a shot or ready line in a file run: %v", count)
	}
	end := lines[len(lines)-1]
	state, ok := end["state"].(map[string]any)
	if end["t"] != "end" || !ok || state["location"] != "camp" {
		t.Fatalf("end line: %v", end)
	}
	snap := simKinds(lines, "snapshot")
	if len(snap) != 1 || !strings.Contains(snap[0], `"gold":`) {
		t.Errorf("snapshot: %v", snap)
	}
}

// TestSimulateIsDeterministicAndInteractive: the same script given at once or line by line
// goes the same way, tick for tick, with the same seed; line by line, a ready line comes
// before each read and a line that does not parse is reported and skipped.
func TestSimulateIsDeterministicAndInteractive(t *testing.T) {
	file, _ := simTrace(t, simulateScript, "")
	again, _ := simTrace(t, simulateScript, "")
	piped, _ := simTrace(t, "", strings.Replace(simulateScript, "shot fire\n", "shot fire\npress nowhere\n", 1))
	kinds := []string{"say", "dev_do", "battle", "input", "scene", "state", "snapshot", "end"}
	want := simKinds(file, kinds...)
	if got := simKinds(again, kinds...); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatal("the same script and seed went another way")
	}
	if got := simKinds(piped, kinds...); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("line by line went another way:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	ready, rejected := 0, 0
	for _, l := range piped {
		switch l["t"] {
		case "ready":
			ready++
			if _, ok := l["waiting"]; !ok {
				t.Errorf("ready without waiting: %v", l)
			}
		case "script_error":
			rejected++
			msg, ok := l["error"].(string)
			if !ok || l["line"] != "press nowhere" || !strings.Contains(msg, "unknown button") {
				t.Errorf("script_error: %v", l)
			}
		}
	}
	if ready < strings.Count(simulateScript, "\n")-1 || rejected != 1 {
		t.Errorf("%d ready lines and %d rejected", ready, rejected)
	}
}

// TestSimulateEndsWithItsInput: line by line, the end of the input ends the run with an
// end line, and a cancelled context stops it.
func TestSimulateEndsWithItsInput(t *testing.T) {
	lines, _ := simTrace(t, "", "wait 5\n")
	if last := lines[len(lines)-1]; last["t"] != "end" {
		t.Fatalf("last line %v", last)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Simulate(ctx, Options{FS: gameFiles(t), Script: "quit"}, SimulateConfig{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

// TestSimulateRejectsWhatItCannotDo: no script, and options that need a window or replace
// the script, are argument errors.
func TestSimulateRejectsWhatItCannotDo(t *testing.T) {
	for name, opts := range map[string]Options{
		"no script": {FS: gameFiles(t)},
		"autoplay":  {FS: gameFiles(t), Script: "quit", Autopilot: &Autopilot{Seconds: 1}},
		"replay":    {FS: gameFiles(t), Script: "quit", Replay: "run.jsonl"},
	} {
		if _, err := Simulate(context.Background(), opts, SimulateConfig{}); !errors.As(err, new(*ArgumentError)) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Simulate(context.Background(), Options{FS: gameFiles(t), Script: "snapshot now"}, SimulateConfig{}); err == nil || !strings.Contains(err.Error(), "snapshot takes no arguments") {
		t.Errorf("bad script: %v", err)
	}
}

// TestSimulatePicturesHaveTheirSize: a run that never draws decodes no pixels, but a
// picture still has its size, which the rules can read (a tall portrait stands on the stage).
func TestSimulatePicturesHaveTheirSize(t *testing.T) {
	g, err := New(Options{FS: gameFiles(t), NoSound: true, headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	}()
	if b := g.pics.get("x.png").Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Errorf("x.png is %v", b)
	}
	if g.pics.get("none.png") != missing {
		t.Error("a missing picture is not the missing image")
	}
}
