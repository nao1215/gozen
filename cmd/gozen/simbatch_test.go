package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSimBatchPlaysEachCaseAsSimScript: every case of gozen sim batch writes the trace,
// prints the output and exits as gozen sim script with the same arguments, whatever ran
// before it in the same process: the random numbers, the saves and the state of one case
// do not reach the next.
func TestSimBatchPlaysEachCaseAsSimScript(t *testing.T) {
	dir := fixture(t)
	work := t.TempDir()
	fight := filepath.Join(work, "fight.txt")
	write(t, fight, "wait 10\ndo say: \"Night falls.\"\nuntil idle\ndo battle: lesson\nuntil idle\nuntil idle\nsnapshot\nquit\n")
	saving := filepath.Join(work, "save.txt")
	write(t, saving, "wait 10\ndo save: 1\nuntil idle\nsnapshot\nquit\n")
	broken := filepath.Join(work, "broken.txt")
	write(t, broken, "wait ten\n")
	cases := [][]string{
		{dir, "--start", "camp", "--seed", "5", "--script", fight, "--trace", filepath.Join(work, "a.jsonl")},
		{dir, "--start", "camp", "--seed", "6", "--script", fight, "--trace", filepath.Join(work, "b.jsonl")},
		{dir, "--start", "camp", "--seed", "5", "--script", saving, "--trace", filepath.Join(work, "c.jsonl"), "--save-dir", filepath.Join(work, "saves")},
		{dir, "--start", "camp", "--seed", "5", "--script", fight, "--trace", filepath.Join(work, "a.jsonl")},
		{dir, "--start", "camp", "--script", broken},
		{dir, "--start", "nowhere", "--script", fight},
		{filepath.Join(work, "missing"), "--script", fight},
	}
	// What gozen sim script gives for each case on its own.
	type outcome struct {
		exit           int
		stdout, stderr string
		trace          string
	}
	alone := make([]outcome, len(cases))
	for i, args := range cases {
		var o outcome
		o.exit, o.stdout, o.stderr = runArgs(append([]string{"sim", "script"}, args...))
		if p := traceOf(args); p != "" && o.exit == 0 {
			o.trace = string(read(t, p))
		}
		alone[i] = o
	}
	if err := os.RemoveAll(filepath.Join(work, "saves")); err != nil {
		t.Fatal(err)
	}
	var in strings.Builder
	for i, args := range cases {
		b, err := json.Marshal(batchCase{ID: string(rune('a' + i)), Args: args})
		if err != nil {
			t.Fatal(err)
		}
		in.Write(b)
		in.WriteString("\n\n")
	}
	manifest := filepath.Join(work, "cases.jsonl")
	write(t, manifest, in.String()+"{\"args\": 1}\n")
	stdout, _ := invoke(t, 1, "sim", "batch", "--cases", manifest)
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != len(cases)+2 {
		t.Fatalf("%d results for %d cases: %s", len(lines), len(cases)+1, stdout)
	}
	traces := map[string]string{}
	for i, l := range lines[:len(cases)] {
		var envelope struct {
			Data batchResult `json:"data"`
		}
		if err := json.Unmarshal([]byte(l), &envelope); err != nil {
			t.Fatal(err)
		}
		r := envelope.Data
		want := alone[i]
		if r.ID != string(rune('a'+i)) || r.Exit != want.exit || r.Stdout != want.stdout || r.Stderr != want.stderr {
			t.Errorf("case %d: got %+v, want exit %d stdout %q stderr %q", i, r, want.exit, want.stdout, want.stderr)
		}
		if p := traceOf(cases[i]); p != "" && want.exit == 0 {
			got := string(read(t, p))
			if simTraceWithoutClocks(t, got) != simTraceWithoutClocks(t, want.trace) {
				t.Errorf("case %d: the trace differs from gozen sim script's", i)
			}
			traces[p] = got
		}
	}
	if alone[0].exit != 0 || alone[2].exit != 0 || alone[4].exit == 0 || alone[5].exit == 0 || alone[6].exit == 0 {
		t.Fatalf("the fixture cases do not cover success and failure: %+v", alone)
	}
	if alone[0].trace == alone[1].trace {
		t.Error("two seeds gave the same trace; the cases do not show the random numbers")
	}
	var bad struct {
		Data batchResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(lines[len(cases)]), &bad); err != nil || bad.Data.Exit != 2 || bad.Data.Stderr == "" {
		t.Errorf("a case that does not parse: %s", lines[len(cases)])
	}
}

// TestSimBatchArguments: sim batch takes no operands, and a case reads neither its
// script nor its trace from the standard streams the cases use.
func TestSimBatchArguments(t *testing.T) {
	dir := fixture(t)
	invoke(t, 2, "sim", "batch", dir)
	invoke(t, 2, "sim", "batch")
	invoke(t, 2, "sim", "batch", "--games", "0")
	manifest := filepath.Join(t.TempDir(), "cases.jsonl")
	write(t, manifest, `{"id":"x","args":["`+filepath.ToSlash(dir)+`","--script","-"]}`+"\n")
	stdout, _ := invoke(t, 1, "sim", "batch", "--cases", manifest)
	var r struct {
		Data batchResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.Split(stdout, "\n")[0]), &r); err != nil || r.Data.ID != "x" || r.Data.Exit != 2 {
		t.Errorf("--script - in a case: %s", stdout)
	}
}

// TestGameCacheReadsAChangedGameAgain: the definitions of a directory are read once,
// again when one of its files changes, and the oldest are dropped past the limit.
func TestGameCacheReadsAChangedGameAgain(t *testing.T) {
	dir, other := fixture(t), fixture(t)
	g := &gameCache{keep: 1}
	first := g.definitions(dir, os.DirFS(dir))
	if first == nil || g.definitions(dir, os.DirFS(dir)) != first {
		t.Fatal("the same game was read twice")
	}
	f, err := os.OpenFile(filepath.Join(dir, "game.yaml"), os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# changed\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	changed := g.definitions(dir, os.DirFS(dir))
	if changed == nil || changed == first {
		t.Error("a changed game was not read again")
	}
	if g.definitions(other, os.DirFS(other)) == nil || len(g.entries) != 1 {
		t.Errorf("kept %d games, want 1", len(g.entries))
	}
	broken := t.TempDir()
	write(t, filepath.Join(broken, "game.yaml"), "title: [\n")
	if g.definitions(broken, os.DirFS(broken)) != nil {
		t.Error("a game that does not load gave definitions")
	}
	var none *gameCache
	if none.definitions(dir, os.DirFS(dir)) != nil {
		t.Error("no cache gave definitions")
	}
}

func runArgs(args []string) (int, string, string) {
	var out, stderr strings.Builder
	exit := run(args, &out, &stderr)
	return exit, out.String(), stderr.String()
}

func traceOf(args []string) string {
	for i, a := range args {
		if a == "--trace" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestSimBatchKeepsWhatTheGameLogsInTheCase: the game logs runtime errors of its events
// to the standard error; in a batch they are part of that case's stderr, and the log
// goes back where it was after the case.
func TestSimBatchKeepsWhatTheGameLogsInTheCase(t *testing.T) {
	dir := fixture(t)
	script := filepath.Join(t.TempDir(), "log.txt")
	write(t, script, "wait 10\ndo gold: \"'word'\"\nwait 10\nquit\n")
	record, err := json.Marshal(batchCase{ID: "log", Args: []string{dir, "--script", script, "--start", "camp"}})
	if err != nil {
		t.Fatal(err)
	}
	before := log.Writer()
	got := runCase(context.Background(), record, nil)
	if !strings.Contains(got.Stderr, "number") {
		t.Fatal(got)
	}
	if log.Writer() != before {
		t.Fatal("log writer was not restored")
	}
}

func TestRunCaseRejectsTrailingJSON(t *testing.T) {
	for _, suffix := range []string{`{}`, `null`, `garbage`, `42`} {
		got := runCase(context.Background(), []byte(`{"id":"bad","args":["--help"]} `+suffix), nil)
		if got.Exit != 2 || got.Stdout != "" || got.ID != "bad" {
			t.Fatal(got)
		}
	}
	if got := runCase(context.Background(), []byte("{\"id\":\"ok\",\"args\":[\"--help\"]} \t\r"), nil); got.Exit != 0 {
		t.Fatal(got)
	}
}

func BenchmarkBatchDefinitions(b *testing.B) {
	dir := "../../e2e/atago/fixtures/game"
	fsys := os.DirFS(dir)
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached=%v", cached), func(b *testing.B) {
			games := &gameCache{keep: 4}
			b.ReportAllocs()
			for b.Loop() {
				if !cached {
					games.entries = nil
				}
				if games.definitions(dir, fsys) == nil {
					b.Fatal("fixture did not load")
				}
			}
		})
	}
}

func BenchmarkCheckGame(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		exit, _, stderr := runArgs([]string{"check", "../../e2e/atago/fixtures/game"})
		if exit != 0 {
			b.Fatal(stderr)
		}
	}
}

func TestRunCaseRejectsInlineStandardStreams(t *testing.T) {
	for _, arg := range []string{"--script=-", "--trace=-"} {
		record, err := json.Marshal(batchCase{ID: "inline", Args: []string{"game", arg}})
		if err != nil {
			t.Fatal(err)
		}
		result := runCase(context.Background(), record, nil)
		if result.Exit != 2 || result.Stdout != "" {
			t.Fatal(result)
		}
	}
}

func TestSimBatchCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, cases             string
		exit, succeeded, failed int
	}{
		{"empty", "", 0, 0, 0},
		{"success", "{\"id\":\"help\",\"args\":[\"--help\"]}\n", 0, 1, 0},
		{"failure", "{\"id\":\"bad\",\"args\":1}\n", 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := filepath.Join(t.TempDir(), "cases.jsonl")
			write(t, manifest, tc.cases)
			stdout, stderr := invoke(t, tc.exit, "sim", "batch", "--cases", manifest)
			if stderr != "" {
				t.Fatalf("duplicate error: %s", stderr)
			}
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != tc.succeeded+tc.failed+1 {
				t.Fatal(stdout)
			}
			var summary struct {
				Version               int
				Command, Type, Status string
				Data                  struct{ Succeeded, Failed int }
			}
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
				t.Fatal(err)
			}
			status := "ok"
			if tc.failed > 0 {
				status = "failed"
			}
			if summary.Version != 1 || summary.Command != "sim batch" || summary.Type != "summary" || summary.Status != status || summary.Data.Succeeded != tc.succeeded || summary.Data.Failed != tc.failed {
				t.Fatalf("summary: %+v", summary)
			}
			if got := run([]string{"sim", "batch", "--cases", manifest}, failingWriter{}, io.Discard); got != 1 {
				t.Fatalf("failed write exit: %d", got)
			}
		})
	}
}

func TestRunCaseDoesNotDispatchCommands(t *testing.T) {
	saved := tree["sim script"]
	delete(tree, "sim script")
	t.Cleanup(func() { tree["sim script"] = saved })
	got := runCase(context.Background(), []byte(`{"id":"help","args":["--help"]}`), nil)
	if got.Exit != 0 || !strings.Contains(got.Stdout, "[Usage]") {
		t.Fatalf("case depends on command dispatch: %+v", got)
	}
}
