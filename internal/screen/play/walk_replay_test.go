package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReplayOfAScriptedWalk: a script's walk on a map or in a painted scene moves the party
// without pressing a key, so it is written to the trace as an act, and the replay walks the
// same way and ends at the same place.
func TestReplayOfAScriptedWalk(t *testing.T) {
	covSlow(t)
	for _, tc := range []struct{ at, script, end string }{
		{"town", "wait 5\nwalk square\nwait 30\nquit", "plaza"},
		{"plaza", "wait 5\nwalk bench\nlines done\nwait 20\nquit", "plaza"},
	} {
		path := filepath.Join(t.TempDir(), "run.jsonl")
		f, err := os.Create(path) //nolint:gosec // a file in the test directory
		if err != nil {
			t.Fatal(err)
		}
		g := sysScenesGame(t, Options{Seed: 1, Location: tc.at, Script: tc.script, Trace: f})
		if !sysFrames(t, g, 6000) {
			t.Fatalf("%s: the script did not end", tc.at)
		}
		sysClose(t, g)
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path) //nolint:gosec // reads a file this test wrote
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"name":"walk"`) {
			t.Fatalf("%s: the walk is not in the trace", tc.at)
		}
		r := sysScenesGame(t, Options{Replay: path})
		if !sysFrames(t, r, 20000) {
			t.Fatalf("%s: the replay did not end", tc.at)
		}
		if r.rep == nil || r.rep.diverge != "" || !r.rep.finished {
			t.Fatalf("%s: replay diverged: %q", tc.at, r.rep.diverge)
		}
		if r.st.Location != tc.end || r.st.Location != g.st.Location || r.st.Spot != g.st.Spot {
			t.Fatalf("%s: replay ended at %s/%s, the run at %s/%s", tc.at, r.st.Location, r.st.Spot, g.st.Location, g.st.Spot)
		}
		sysClose(t, r)
	}
}
