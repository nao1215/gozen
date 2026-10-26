package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestSimScriptWritesTheTrace: gozen sim script plays a run --script script without a
// window, writes the trace to a file and says how far it went; with --trace - the trace is
// the standard output. The same seed gives the same trace.
func TestSimScriptWritesTheTrace(t *testing.T) {
	dir := fixture(t)
	script := filepath.Join(t.TempDir(), "script.txt")
	write(t, script, "wait 10\ndo say: \"Night falls.\"\nuntil idle\ndo battle: lesson\nuntil idle\nuntil idle\nshot camp\nsnapshot\nquit\n")
	trace := filepath.Join(t.TempDir(), "deep", "run.jsonl")
	stdout, _ := invoke(t, 0, "sim", "script", dir, "--start", "camp", "--seed", "5", "--script", script, "--trace", trace)
	if !strings.Contains(stdout, " ticks, ") || !strings.Contains(stdout, trace) {
		t.Errorf("stdout: %q", stdout)
	}
	file := string(read(t, trace))
	piped, _ := invoke(t, 0, "sim", "script", dir, "--start", "camp", "--seed", "5", "--script", script, "--trace", "-")
	if piped != file {
		t.Error("the trace on the standard output differs from the file of the same run")
	}
	kinds := map[string]bool{}
	var end map[string]any
	for l := range strings.Lines(file) {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		kind, ok := m["t"].(string)
		if !ok {
			t.Fatalf("no kind: %q", l)
		}
		kinds[kind] = true
		end = m
	}
	for _, k := range []string{"start", "say", "dev_do", "battle", "snapshot", "end"} {
		if !kinds[k] {
			t.Errorf("no %s line", k)
		}
	}
	if kinds["shot"] || end["t"] != "end" {
		t.Errorf("shot line or no end: %v %v", kinds, end["t"])
	}
	stdout, _ = invoke(t, 0, "sim", "script", dir, "--script", script)
	if !strings.HasSuffix(stdout, " ticks\n") {
		t.Errorf("without a trace: %q", stdout)
	}
}

// TestSimScriptArguments: the game directory and --script are required, a script that does
// not parse and an unknown place fail before playing.
func TestSimScriptArguments(t *testing.T) {
	dir := fixture(t)
	script := filepath.Join(t.TempDir(), "script.txt")
	write(t, script, "quit\n")
	_, stderr := invoke(t, 2, "sim", "script", dir)
	if !strings.Contains(stderr, "--script FILE is required") {
		t.Errorf("stderr: %q", stderr)
	}
	invoke(t, 2, "sim", "script", "--script", script)
	invoke(t, 2, "sim", "script", dir, "--script", script, "--level", "-1")
	invoke(t, 1, "sim", "script", filepath.Join(t.TempDir(), "none"), "--script", script)
	invoke(t, 1, "sim", "script", dir, "--script", filepath.Join(t.TempDir(), "missing.txt"))
	bad := filepath.Join(t.TempDir(), "bad.txt")
	write(t, bad, "press nowhere\n")
	_, stderr = invoke(t, 1, "sim", "script", dir, "--script", bad)
	if !strings.Contains(stderr, "unknown button") {
		t.Errorf("stderr: %q", stderr)
	}
	_, stderr = invoke(t, 2, "sim", "script", dir, "--script", script, "--start", "nowhere")
	if !strings.Contains(stderr, "unknown location") {
		t.Errorf("stderr: %q", stderr)
	}
}
