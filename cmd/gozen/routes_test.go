package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/cliio"
)

func TestSimRoutesExploresNovelChoices(t *testing.T) {
	out, _ := invoke(t, 0, "sim", "routes", "../../examples/novel/game", "--format", "json")
	var report routesReport
	if err := json.Unmarshal(resultData(t, out, "sim routes"), &report); err != nil {
		t.Fatal(err)
	}
	if report.Paths < 2 || len(report.Endings) == 0 || report.Endings[0].Paths == 0 {
		t.Fatalf("novel routes were not explored: %+v", report)
	}
	if report.Truncated {
		t.Fatal("the novel fits within the default path cap")
	}
}

func TestSimRoutesReportsUnreachableClear(t *testing.T) {
	dir := designGame(t)
	path := filepath.Join(dir, "data", "world.yaml")
	b, err := os.ReadFile(path) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	definition := strings.Replace(string(b), "events:\n", "events:\n  unreachable-ending: {do: [{clear: true}]}\n  unreachable-choice: {do: [{choice: [{label: Never offered, do: [{clear: true}]}]}]}\n  unused-flag: {do: [{set: {flag.never_read: true}}]}\n", 1)
	if err := os.WriteFile(path, []byte(definition), 0o600); err != nil { //nolint:gosec // Test-owned path under t.TempDir.
		t.Fatal(err)
	}
	out, _ := invoke(t, 0, "sim", "routes", dir, "--format", "json")
	var report routesReport
	if err := json.Unmarshal(resultData(t, out, "sim routes"), &report); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ending := range report.UnreachableEndings {
		found = found || ending == "clear"
	}
	if !found {
		t.Fatalf("the unreachable clear ending is not reported: %+v", report)
	}
	if !slices.ContainsFunc(report.UnofferedOptions, func(option routeOption) bool { return option.Option == "Never offered" }) {
		t.Fatalf("the unreachable choice option is not reported: %+v", report.UnofferedOptions)
	}
	if len(report.FlagsSetNeverRead) != 1 || report.FlagsSetNeverRead[0] != "never_read" {
		t.Fatalf("the GZN0112 flag is not reported: %+v", report.FlagsSetNeverRead)
	}
	invoke(t, 1, "sim", "routes", dir, "--fail-unreachable")
	for _, format := range []string{"json", "jsonl"} {
		output, stderr := invoke(t, 1, "sim", "routes", dir, "--fail-unreachable", "--format", format)
		if stderr != "" {
			t.Fatalf("%s duplicated the reported failure: %s", format, stderr)
		}
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var summary struct {
			Status string       `json:"status"`
			Data   routesReport `json:"data"`
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Status != "failed" || !slices.Equal(summary.Data.UnreachableEndings, report.UnreachableEndings) {
			t.Fatalf("%s lost the failed result: %s", format, output)
		}
	}
}

func TestSimRoutesExpiredDeadlineIsFailure(t *testing.T) {
	var stdout, diagnostics bytes.Buffer
	c := &command{path: "sim routes", out: &cliio.Output{Writer: &stdout}, stderr: &cliio.Output{Writer: &diagnostics}}
	op := c.operation(tree[c.path])
	if err := c.parse(op.Flags, []string{designGame(t), "--format", "json"}, op.Page); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if exit := c.finish(op.Run(ctx)); exit != 1 {
		t.Fatalf("expired deadline: exit %d; %s; %s", exit, stdout.String(), diagnostics.String())
	}
	out, stderr := stdout.String(), diagnostics.String()
	var report routesReport
	if err := json.Unmarshal(resultData(t, out, "sim routes"), &report); err != nil {
		t.Fatal(err)
	}
	if stderr != "" || !report.TimedOut || report.Truncated || !slices.Contains(report.Errors, "exploration timed out") {
		t.Fatalf("timeout was not reported as a failure: %s; %s", out, stderr)
	}
}

func TestSimRoutesCannotPickHiddenMapPoint(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "game")
			commands := "[{pick: Workshop}]"
			if open {
				commands = "[{set: {flag.built: true}}, {pick: Workshop}]"
			}
			files := fstest.MapFS{
				"game.yaml":      {Data: []byte("schema_version: 8\nid: hidden-point\nstart: {location: board, party: [], event: enter}\n")},
				"assets/map.png": {Data: []byte("synthetic image sentinel")},
				"data/world.yaml": {Data: []byte(`locations:
  board:
    map:
      picture: map.png
      activation: instant
      points:
        well: {at: [0, 0], start: true}
        workshop: {at: [10, 0], label: Workshop, hide: closed, when: flag.built, locked: Build first., do: [{clear: true}]}
events:
  enter:
    do: ` + commands)},
			}
			if err := os.CopyFS(dir, files); err != nil {
				t.Fatal(err)
			}
			code := 1
			if open {
				code = 0
			}
			out, _ := invoke(t, code, "sim", "routes", dir, "--format", "json")
			var report routesReport
			if err := json.Unmarshal(resultData(t, out, "sim routes"), &report); err != nil {
				t.Fatal(err)
			}
			if open {
				if len(report.Errors) != 0 || len(report.Endings) != 1 || report.Endings[0].Kind != "clear" {
					t.Fatalf("open point: %+v", report)
				}
			} else if len(report.Errors) != 1 || !strings.Contains(report.Errors[0], `there is no "Workshop"`) {
				t.Fatalf("hidden point was offered to pick: %+v", report)
			}
		})
	}
}
