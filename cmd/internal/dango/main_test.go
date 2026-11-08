package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/dev/hygiene"
)

func passingEvents() []testEvent {
	var events []testEvent
	packages := map[string]bool{}
	for _, d := range hygiene.Decisions() {
		for _, test := range d.Tests {
			pkg := "github.com/nao1215/gozen/" + test.Package
			events = append(events, testEvent{Action: "pass", Package: pkg, Test: test.Name})
			packages[pkg] = true
		}
	}
	for pkg := range packages {
		events = append(events, testEvent{Action: "pass", Package: pkg})
	}
	return events
}

func TestRunContracts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		code        int
		failure     bool
		environment bool
	}{
		{"pass", []string{"lint"}, 0, false, false},
		{"fail", []string{"lint"}, 1, true, false},
		{"json pass", []string{"lint", "--format=json"}, 0, false, false},
		{"json fail", []string{"lint", "--format", "json"}, 1, true, false},
		{"list", []string{"lint", "--list"}, 0, false, false},
		{"list json", []string{"lint", "--list", "--format=json"}, 0, false, false},
		{"environment", []string{"lint", "--format=json"}, 1, false, true},
		{"invalid", []string{"lint", "--unknown", "--format=json"}, 2, false, false},
		{"old flag", []string{"lint", "--json"}, 2, false, false},
		{"format", []string{"lint", "--format=xml"}, 2, false, false},
		{"operand", []string{"lint", "extra"}, 2, false, false},
		{"single dash", []string{"lint", "-list"}, 2, false, false},
		{"help", []string{"--help"}, 0, false, false},
		{"missing", nil, 2, false, false},
		{"unknown machine command", []string{"unknown", "--format=json"}, 2, false, false},
		{"missing reproduction operands", []string{"repro"}, 2, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			called := false
			runner := func(args []string) ([]testEvent, error) {
				called = true
				if !strings.Contains(strings.Join(args, " "), "TestDecisionCoverage") {
					t.Fatal(args)
				}
				if tc.environment {
					return nil, errors.New("go unavailable")
				}
				events := passingEvents()
				if tc.failure {
					events[0].Action = "fail"
					events = append(events, testEvent{Action: "output", Package: events[0].Package, Test: events[0].Test, Output: "remove private location\n"})
				}
				return events, nil
			}
			code := run(tc.args, &out, &errs, runner)
			if code != tc.code {
				t.Fatalf("exit %d want %d: %s / %s", code, tc.code, &out, &errs)
			}
			list := strings.Contains(strings.Join(tc.args, " "), "--list")
			if list && called {
				t.Fatal("listing ran tests")
			}
			machine := strings.Contains(strings.Join(tc.args, " "), "json") && !strings.Contains(strings.Join(tc.args, " "), "--json")
			if machine {
				target := out.String()
				if errs.Len() > 0 {
					target = errs.String()
				}
				var value map[string]any
				dec := json.NewDecoder(strings.NewReader(target))
				if err := dec.Decode(&value); err != nil {
					t.Fatal(err, target)
				}
				if value["version"] != float64(1) || value["command"] != "dango lint" {
					t.Fatal(value)
				}
				if err := dec.Decode(&value); err != io.EOF {
					t.Fatal("not exactly one document", err)
				}
			}
			if tc.failure && !machine && (!strings.Contains(out.String(), "0004") || !strings.Contains(out.String(), "Fix:") || !strings.Contains(out.String(), "remove private location")) {
				t.Fatal(out.String())
			}
			if tc.failure && errs.Len() != 0 {
				t.Fatal("duplicated failure", errs.String())
			}
			if tc.code == 2 || tc.environment {
				if out.Len() != 0 || errs.Len() == 0 {
					t.Fatal("fatal output separation")
				}
			}
		})
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, nil }
func TestFailedOutput(t *testing.T) {
	runner := func([]string) ([]testEvent, error) { return passingEvents(), nil }
	for _, args := range [][]string{{"lint"}, {"lint", "--format=json"}, {"lint", "--list"}, {"--help"}, {"lint", "--unknown"}} {
		if got := run(args, brokenWriter{}, brokenWriter{}, runner); got != 1 {
			t.Errorf("%v: %d", args, got)
		}
	}
}

func TestIncompleteTestsAreProcessingFailures(t *testing.T) {
	for _, action := range []string{"skip", "run", ""} {
		var out, errs bytes.Buffer
		runner := func([]string) ([]testEvent, error) {
			events := passingEvents()
			events[0].Action = action
			return events, nil
		}
		if got := run([]string{"lint", "--format=json"}, &out, &errs, runner); got != 1 || out.Len() != 0 || !strings.Contains(errs.String(), "did not complete") {
			t.Fatalf("%s: %d: %s / %s", action, got, &out, &errs)
		}
	}
}

func TestReproContracts(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(trace, []byte(`{"t":"start","tick":0,"exact_version":3,"game_dir":"missing game","seed":7}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{trace, "--repro", "0", "--format=json"}, 0},
		{[]string{trace, "--repro", "absent.png", "--format=json"}, 1},
		{[]string{trace + "missing", "--repro", "0", "--format=json"}, 1},
		{[]string{trace, "--repro", "0", "--format=jsonl"}, 2},
		{[]string{"-", "--repro", "0", "--format=json"}, 2},
		{[]string{trace, "--repro", "0", "--unknown", "--format=json"}, 2},
		{[]string{"--help"}, 0},
	} {
		var out, errs bytes.Buffer
		runner := func([]string) ([]testEvent, error) { t.Fatal("repro ran tests"); return nil, nil }
		code := run(append([]string{"repro"}, tc.args...), &out, &errs, runner)
		if code != tc.code {
			t.Fatalf("%v: %d: %s %s", tc.args, code, &out, &errs)
		}
		if code != 0 && out.Len() != 0 {
			t.Fatal("failure wrote stdout", out.String())
		}
		if code == 0 && errs.Len() != 0 {
			t.Fatal(errs.String())
		}
	}
	var errs bytes.Buffer
	if code := run([]string{"repro", trace, "--repro", "0"}, brokenWriter{}, &errs, nil); code != 1 {
		t.Fatalf("write failure exit %d", code)
	}
}
