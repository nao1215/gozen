package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimRoutesExploresNovelChoices(t *testing.T) {
	out, _ := invoke(t, 0, "sim", "routes", "../../examples/novel/game", "--format", "json")
	var report routesReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
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
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	definition := strings.Replace(string(b), "events:\n", "events:\n  unreachable-ending: {do: [{clear: true}]}\n  unreachable-choice: {do: [{choice: [{label: Never offered, do: [{clear: true}]}]}]}\n  unused-flag: {do: [{set: {flag.never_read: true}}]}\n", 1)
	if err := os.WriteFile(path, []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := invoke(t, 0, "sim", "routes", dir, "--format", "json")
	var report routesReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ending := range report.UnreachableEndings {
		found = found || ending == "clear"
	}
	if !found {
		t.Fatalf("the unreachable clear ending is not reported: %+v", report)
	}
	if len(report.UnofferedOptions) == 0 || report.UnofferedOptions[0].Option != "Never offered" {
		t.Fatalf("the unreachable choice option is not reported: %+v", report.UnofferedOptions)
	}
	if len(report.FlagsSetNeverRead) != 1 || report.FlagsSetNeverRead[0] != "never_read" {
		t.Fatalf("the GZN0112 flag is not reported: %+v", report.FlagsSetNeverRead)
	}
	invoke(t, 1, "sim", "routes", dir, "--fail-unreachable")
}
