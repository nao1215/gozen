package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/model/gamedata"
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
		`{"version":1,"only":"structure","counts":{"GZN2201":{"data/a.yaml":1}}}`,
		`{"version":1,"only":"structure","counts":{"gzn0112":{"data/a.yaml":1}}}`,
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

// Keep all checked-in samples clean under the same strict CLI authors use.
func TestCheckSamplesStrict(t *testing.T) {
	count := 0
	for _, root := range []string{"../../examples", "../../docs/examples"} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() != "game.yaml" {
				return nil
			}
			count++
			t.Run(filepath.Dir(path), func(t *testing.T) {
				_, stderr := invoke(t, 0, "check", "--strict", filepath.Dir(path))
				if stderr != "" {
					t.Fatalf("sample warnings: %s", stderr)
				}
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count == 0 {
		t.Fatal("no samples checked")
	}
}

func TestCheckBaselineRetainsProcessingFamilyWarnings(t *testing.T) {
	// Some structural review hints already use GZN11xx with warning severity.
	// Baselines must round-trip every warning emitted by Diagnose, not only 0xxx.
	b := warningCounts("", gamedata.Diagnostics{
		{Severity: "warning", Code: "GZN1104", File: "data/world.yaml"},
		{Severity: "warning", Code: "", File: ""},
		{Severity: "error", Code: "GZN1101", File: "data/world.yaml"},
	})
	name := filepath.Join(t.TempDir(), "baseline.json")
	if err := writeCheckBaseline(name, b); err != nil {
		t.Fatal(err)
	}
	got, err := readCheckBaseline(name, "")
	if err != nil || b.growsFrom(got) || got.Counts["GZN1104"]["data/world.yaml"] != 1 || got.Counts[""][""] != 1 || len(got.Counts) != 2 {
		t.Fatalf("%+v: %v", got, err)
	}
}

func TestCheckEnforcementReportsScopeAndIOFailures(t *testing.T) {
	dir := fixture(t)
	out, _ := invoke(t, 0, "check", dir, "--strict", "--only", "structure")
	if !strings.Contains(out, "layout was not checked") {
		t.Fatal(out)
	}
	invoke(t, 1, "check", dir, "--baseline", filepath.Join(t.TempDir(), "missing.json"))
	invoke(t, 1, "check", dir, "--write-baseline", filepath.Join(t.TempDir(), "absent", "budget.json"))
	// Existing game inputs cannot be overwritten by the explicit output flag.
	original := read(t, filepath.Join(dir, "game.yaml"))
	invoke(t, 1, "check", dir, "--write-baseline", filepath.Join(dir, "game.yaml"))
	if string(read(t, filepath.Join(dir, "game.yaml"))) != string(original) {
		t.Fatal("game.yaml overwritten")
	}
	for _, format := range []string{"text", "json", "jsonl"} {
		if got := run([]string{"check", dir, "--strict", "--format", format}, failWriter{}, &bytes.Buffer{}); got != 1 {
			t.Fatalf("%s stdout failure exit %d", format, got)
		}
	}
	addWorld(t, dir, "unused.yaml", "events: {unused: {do: [{say: Hello}]}}\n")
	if got := run([]string{"check", dir, "--strict"}, &bytes.Buffer{}, failWriter{}); got != 1 {
		t.Fatalf("stderr failure exit %d", got)
	}
}

func TestCheckStrictRespectsSelectedScope(t *testing.T) {
	dir := fixture(t)
	addWorld(t, dir, "unused.yaml", "events: {unused: {do: [{say: Hello}]}}\n")
	for _, format := range []string{"text", "json", "jsonl"} {
		args := []string{"check", dir, "--format", format}
		invoke(t, 0, append(args, "--strict=false")...)
		invoke(t, 1, append(args, "--strict", "--only", "structure")...)
		out, stderr := invoke(t, 0, append(args, "--strict", "--only", "layout")...)
		if strings.Contains(out+stderr, "GZN0124") {
			t.Fatalf("layout leaked structural warnings: %s %s", out, stderr)
		}
	}
	// Layout-only checks must not read the structural complexity budget.
	write(t, filepath.Join(dir, "check-complexity.yaml"), "invalid: [\n")
	for _, format := range []string{"text", "json", "jsonl"} {
		for _, strict := range []string{"--strict", "--strict=false"} {
			invoke(t, 0, "check", dir, "--format", format, strict, "--only", "layout")
		}
		invoke(t, 1, "check", dir, "--format", format, "--strict", "--only", "structure")
	}
	addWorld(t, dir, "unused.yaml", "events: {unused: {do: [{say: '"+longLine(40)+"'}]}}\n")
	out, stderr := invoke(t, 1, "check", dir, "--strict", "--only", "layout")
	if !strings.Contains(stderr, "GZN0101") || strings.Contains(out+stderr, "GZN0124") {
		t.Fatalf("layout result: %s %s", out, stderr)
	}
	for _, format := range []string{"text", "json", "jsonl"} {
		out, stderr := invoke(t, 1, "check", t.TempDir(), "--format", format, "--strict", "--only", "layout")
		if !strings.Contains(out+stderr, "GZN1110") {
			t.Fatalf("missing game was not coded: %s %s", out, stderr)
		}
	}
}
