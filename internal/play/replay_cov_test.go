package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// sysRecord plays a scripted run of the story with a trace written to a file and returns the
// file's path.
func sysRecord(t *testing.T, script string, opts Options) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.jsonl")
	f, err := os.Create(path) //nolint:gosec // a file in the test directory
	if err != nil {
		t.Fatal(err)
	}
	opts.Trace, opts.Script = f, script
	g := sysNovel(t, opts)
	if !sysFrames(t, g, 4000) {
		t.Fatal("the recorded script did not end")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func sysReplay(t *testing.T, path string, opts Options) *Game {
	t.Helper()
	opts.Replay = path
	return sysNovel(t, opts)
}

func sysRunReplay(t *testing.T, g *Game) {
	t.Helper()
	if !sysFrames(t, g, 20000) {
		t.Fatal("the replay did not end")
	}
}

// TestReplayOfARecordedRunMatches: playing the trace of a run again with the same data gives the
// same lines, choices and final state (the replay reports an exact match).
func TestReplayOfARecordedRunMatches(t *testing.T) {
	path := sysRecord(t, "wait 5\nread 3\nread 3\nread 3\nread 3\nread 3\nwait 20\nquit", Options{Seed: 7, Location: "camp", Event: "story"})
	g := sysReplay(t, path, Options{})
	sysRunReplay(t, g)
	if g.rep == nil || g.rep.diverge != "" || !g.rep.finished {
		t.Fatalf("replay diverged: %q finished %v", g.rep.diverge, g.rep.finished)
	}
	if strings.HasPrefix(ReplayReport, "replay diverged") {
		t.Fatalf("report %q", ReplayReport)
	}
	if g.rep.matched == 0 {
		t.Fatal("nothing was checked")
	}
	if g.st.Location != "camp" {
		t.Fatalf("ended at %s", g.st.Location)
	}
}

// TestReplayRefusesDifferentData: a trace replayed against data that says something else is
// refused before it starts, not played into a confusing divergence.
func TestReplayRefusesDifferentData(t *testing.T) {
	path := sysRecord(t, "wait 5\nread 3\nread 3\nread 3\nwait 20\nquit", Options{Seed: 7, Location: "camp", Event: "story"})
	changed := strings.Replace(novelData, "The lamps are lit.", "The lamps are out.", 1)
	fsys := gameFiles(t)
	fsys["data/novel.yaml"] = &fstest.MapFile{Data: []byte(changed)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Replay: path})
	if err == nil {
		sysClose(t, g)
		t.Fatal("replayed a trace against other data")
	}
	if !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("error %q", err)
	}
}

// TestReplayRefusesDamagedTraces: a trace that does not start with the start line, is not
// JSON, runs backwards in ticks or goes on after its end is refused with the line number.
func TestReplayRefusesDamagedTraces(t *testing.T) {
	good := sysRecord(t, "wait 5\nread 3\nquit", Options{Seed: 7, Location: "camp", Event: "story"})
	b, err := os.ReadFile(good) //nolint:gosec // reads a file this test wrote
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 4 {
		t.Fatalf("only %d trace lines", len(lines))
	}
	for name, body := range map[string]string{
		"not json":         "{nope\n",
		"empty":            "",
		"no start":         lines[1] + "\n",
		"after the end":    string(b) + lines[len(lines)-1] + "\n",
		"duplicated start": lines[0] + "\n" + lines[0] + "\n",
		"ticks backwards":  lines[0] + "\n" + `{"t":"input","tick":9}` + "\n" + `{"t":"input","tick":3}` + "\n",
	} {
		p := filepath.Join(t.TempDir(), "bad.jsonl")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		fsys := gameFiles(t)
		g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Replay: p})
		if err == nil {
			sysClose(t, g)
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestReplayUntilChecksOnlyTheStretch: --until replays the first ticks and reports a stretch match.
func TestReplayUntilChecksOnlyTheStretch(t *testing.T) {
	path := sysRecord(t, "wait 5\nread 3\nread 3\nread 3\nwait 20\nquit", Options{Seed: 7, Location: "camp", Event: "story"})
	g := sysReplay(t, path, Options{Until: 40})
	for i := range 400 {
		if err := g.Update(); err != nil {
			if !sysQuit(err) {
				t.Fatal(err)
			}
			break
		}
		if g.autoShot != "" {
			g.autoShot = ""
		}
		if i == 399 {
			t.Fatal("never stopped")
		}
	}
	if g.tick > 40 {
		t.Fatalf("ran to tick %d", g.tick)
	}
	if _, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: t.TempDir(), Replay: path, Until: -1}); err == nil {
		t.Fatal("accepted a negative --until")
	}
}
