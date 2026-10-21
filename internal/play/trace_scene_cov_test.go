package play

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A play recorded in a place and played again goes the same way (ADR 0025): the property holds
// for each way of showing a place, with the random encounters of a dungeon.

func wldRoundTrip(t *testing.T, loc string, script string, extra map[string]string) (trace []byte, report string) {
	t.Helper()
	mk := func() Options {
		return Options{FS: wldWorldFiles(t, wldMeters, extra), Location: loc, Seed: 11}
	}
	var buf bytes.Buffer
	o := mk()
	o.Trace, o.Script = &buf, script
	g := wldOpenWith(t, o)
	screen := wldScreen(g)
	for i := 0; i < 20000 && wldStep(t, g, screen); i++ {
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "run.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	r := mk()
	r.Replay, r.NoSound, r.SaveDir = path, true, t.TempDir()
	rg, err := New(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rg.Close(); err != nil {
			t.Error(err)
		}
	}()
	return buf.Bytes(), wldReplay(t, rg)
}

func TestWldReplayDungeonWithEncounters(t *testing.T) {
	covSlow(t)
	script := "wait 60\n"
	for range 2 {
		script += "press up\nwait 14\npress right\nwait 14\npress left\nwait 14\n"
	}
	script += "until\nquit\n"
	tr, report := wldRoundTrip(t, "fx_maze", script, nil)
	if !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatalf("report %q", report)
	}
	if len(traceLines(t, tr, "input")) < 5 {
		t.Fatal("the recording has few inputs")
	}
}

func TestWldReplayMapTravel(t *testing.T) {
	covSlow(t)
	tr, report := wldRoundTrip(t, "fx_named", "wait 60\nclick 240 280\nwait 100\nquit\n", nil)
	if tl := traceLines(t, tr, "travel"); len(tl) != 1 || tl[0]["to"] != "shrine" {
		t.Fatalf("travel lines %v", tl)
	}
	if !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatalf("report %q", report)
	}
}

func TestWldReplayPaintedScene(t *testing.T) {
	covSlow(t)
	tr, report := wldRoundTrip(t, "fx_yard2", "wait 60\nclick 900 560\nwait 60\nclick 300 600\nwait 60\nquit\n", nil)
	if !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatalf("report %q", report)
	}
	if sp := traceLines(t, tr, "spot"); len(sp) == 0 {
		t.Fatal("the trace never says where the party stands")
	}
}
