package play

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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
	a := wldRecord(t, wldRecordScript)
	b := wldRecord(t, wldRecordScript)
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
		{"unknown button", join(append(append([]string{}, lines[:firstInput]...), `{"t":"input","tick":2,"press":["jump"]}`)...), nil, `unknown button "jump"`},
		{"repeated input tick", join(append(append([]string{}, lines[:firstInput]...), `{"t":"input","tick":2,"press":["ok"]}`, `{"t":"input","tick":2,"press":["ok"]}`)...), nil, "repeats input tick 2"},
		{"unknown action", join(append(append([]string{}, lines[:firstInput]...), `{"t":"act","tick":2,"name":"fly"}`)...), nil, `unknown direct action "fly"`},
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
		{"action without a name", join(append(append([]string{}, lines[:firstInput]...), `{"t":"act","tick":2,"name":5}`)...), nil, "action name must be a string"},
		{"reload partway", wldJoinTrace(append(append(append([]string{}, lines[:firstInput]...), `{"t":"live_reload","tick":2,"ok":true}`), lines[firstInput:]...)), nil, "asset or data reload partway"},
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
