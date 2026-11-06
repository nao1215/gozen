package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestCheckBaselineBudgets(t *testing.T) {
	old := warningCounts("structure", gamedata.Diagnostics{{Code: "GZN0112", File: "data/a.yaml", Severity: "warning"}, {Code: "GZN0112", File: "data/a.yaml", Severity: "warning"}})
	for _, tc := range []struct {
		name, code, file string
		count            int
		grow             bool
	}{
		{"same", "GZN0112", "data/a.yaml", 2, false},
		{"reduced", "GZN0112", "data/a.yaml", 1, false},
		{"removed", "GZN0112", "data/a.yaml", 0, false},
		{"increased", "GZN0112", "data/a.yaml", 3, true},
		{"new file", "GZN0112", "data/b.yaml", 1, true},
		{"new code", "GZN0113", "data/a.yaml", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ds gamedata.Diagnostics
			for range tc.count {
				ds = append(ds, gamedata.Diagnostic{Code: tc.code, File: tc.file, Severity: "warning"})
			}
			if got := warningCounts("structure", ds).growsFrom(old); got != tc.grow {
				t.Fatalf("growth = %v", got)
			}
		})
	}
}

func TestCheckStrictAndBaseline(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			dir := fixture(t)
			args := []string{"check", dir, "--only", "structure", "--format", format}
			invoke(t, 0, append(args, "--strict")...)
			addWorld(t, dir, "unused.yaml", "events:\n  unused:\n    trigger: auto\n    when: 'false'\n    do: [{set: {flag.forgotten: true}}]\n")
			invoke(t, 0, args...)
			invoke(t, 1, append(args, "--strict")...)
			baseline := filepath.Join(t.TempDir(), "baseline.json")
			invoke(t, 0, append(args, "--write-baseline", baseline)...)
			invoke(t, 0, append(args, "--baseline", baseline)...)
			invoke(t, 0, append(args, "--strict", "--baseline", baseline)...)
			invoke(t, 1, append(args, "--write-baseline", baseline)...)
			addWorld(t, dir, "more.yaml", "events:\n  more:\n    trigger: auto\n    when: 'false'\n    do: [{set: {flag.other: true}}]\n")
			invoke(t, 1, append(args, "--baseline", baseline)...)
			// Validation errors cannot be hidden by a warning budget or saved as one.
			addWorld(t, dir, "broken.yaml", "events: {broken: {do: [{call: absent}]}}\n")
			fresh := filepath.Join(t.TempDir(), "failed.json")
			invoke(t, 1, append(args, "--baseline", baseline, "--write-baseline", fresh)...)
			if _, err := os.Stat(fresh); !os.IsNotExist(err) {
				t.Fatalf("failed check wrote baseline: %v", err)
			}
		})
	}
}

func TestReadCheckBaselineRejectsMalformedInput(t *testing.T) {
	for _, body := range []string{
		`{`, `{}`, `{"version":2,"only":"structure","counts":{}}`,
		`{"version":1,"only":"layout","counts":{}}`,
		`{"version":1,"only":"structure","counts":{},"unknown":1}`,
		`{"version":1,"only":"structure","counts":{}} {}`,
		`{"version":1,"only":"structure","counts":{"GZN1101":{"data/a.yaml":1}}}`,
		`{"version":1,"only":"structure","counts":{"GZN0112":{"data/a.yaml":-1}}}`,
		`{"version":1,"only":"structure","counts":{"GZN0112":{"../a.yaml":1}}}`,
	} {
		name := filepath.Join(t.TempDir(), "baseline.json")
		write(t, name, body)
		if _, err := readCheckBaseline(name, "structure"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	name := filepath.Join(t.TempDir(), "baseline.json")
	b := warningCounts("structure", gamedata.Diagnostics{{Severity: "warning", Code: "GZN0112", File: "data/a.yaml"}})
	if err := writeCheckBaseline(name, b); err != nil {
		t.Fatal(err)
	}
	got, err := readCheckBaseline(name, "structure")
	if err != nil || got.Counts["GZN0112"]["data/a.yaml"] != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if err := writeCheckBaseline(name, b); err == nil || !strings.Contains(err.Error(), "GZN1501") {
		t.Fatal(err)
	}
}
