package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestSimScriptWritesTheTrace: gozen sim script plays a run --script script without a
// window, writes the trace to a file and says how far it went; with --trace - the trace is
// the standard output. The same seed gives the same gameplay records, apart from storage clocks.
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
	if simTraceWithoutClocks(t, piped) != simTraceWithoutClocks(t, file) {
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
	for _, k := range []string{"start", "say", "act", "battle", "snapshot", "end"} {
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

// simTraceWithoutClocks compares separate runs without their recorded storage
// timestamps. Every other field, including timestamp availability, must match.
func simTraceWithoutClocks(t *testing.T, trace string) string {
	t.Helper()
	var out strings.Builder
	for line := range strings.Lines(trace) {
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if string(record["t"]) == `"save_write"` {
			delete(record, "time")
		}
		if string(record["t"]) == `"start"` && record["setup"] != nil {
			var setup map[string]json.RawMessage
			if err := json.Unmarshal(record["setup"], &setup); err != nil {
				t.Fatal(err)
			}
			delete(setup, "save_times")
			b, err := json.Marshal(setup)
			if err != nil {
				t.Fatal(err)
			}
			record["setup"] = b
		}
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.String()
}

// The trace replaces its file, so naming the script as the trace is refused
// before the script is lost.
func TestSimScriptRefusesToOverwriteItsScript(t *testing.T) {
	dir := fixture(t)
	script := filepath.Join(t.TempDir(), "script.txt")
	write(t, script, "wait 2\nquit\n")
	_, stderr := invoke(t, 2, "sim", "script", dir, "--start", "camp", "--script", script, "--trace", script)
	if !strings.Contains(stderr, "--trace would overwrite") {
		t.Errorf("stderr: %q", stderr)
	}
	if got := string(read(t, script)); got != "wait 2\nquit\n" {
		t.Errorf("the script changed: %q", got)
	}
}
