// Dango is a maintainer-only test reporter. It is never a gozen command.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"slices"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/repro"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/dev/hygiene"
	"github.com/nao1215/gozen/internal/diag"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, execute)) }

type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}
type (
	testRunner     func([]string) ([]testEvent, error)
	decisionResult struct {
		hygiene.Decision
		Status  string   `json:"status"`
		Details []string `json:"details"`
	}
)

type lintResult struct {
	Decisions []decisionResult `json:"decisions"`
}

func run(args []string, stdout, stderr io.Writer, runner testRunner) int {
	if len(args) > 0 && args[0] == "repro" {
		return runRepro(args[1:], stdout, stderr)
	}
	out, errs := &cliio.Output{Writer: stdout}, &cliio.Output{Writer: stderr}
	report := cliio.Report{Command: "dango lint", Format: "text"}
	flags := clihelp.NewFlagSet("dango lint")
	list := flags.Bool("list", false, "list decisions and enforcing tests")
	format := flags.String("format", "text", "result representation: text or json")
	report.Format = clihelp.OutputFormat(flags, args)
	if len(args) > 0 && args[0] == "lint" {
		args = args[1:]
	} else if len(args) != 1 || (args[0] != "--help" && args[0] != "-h") {
		return finish(report, out, errs, cliio.Invalid(diag.BadValue, "usage: go tool dango lint|repro --help"))
	}
	report.Format = clihelp.OutputFormat(flags, args)
	help, err := clihelp.Parse(flags, args)
	if err != nil {
		return finish(report, out, errs, cliio.Parse(err))
	}
	report.Format = *format
	if *format != "text" && *format != "json" {
		return finish(report, out, errs, cliio.Invalid(diag.NotAChoice, "--format must be text or json"))
	}
	if flags.NArg() != 0 {
		return finish(report, out, errs, cliio.Invalid(diag.BadValue, "lint takes no operands"))
	}
	if help {
		page := clihelp.Page{
			Name: "dango lint", Summary: "report maintainer decision tests",
			Usage:    []string{"go tool dango lint [--list] [--format text|json]"},
			About:    "Maintainer-only repository checks; not shipped to game authors. Use go tool dango repro --help for reproduction bundles.",
			Examples: []clihelp.Example{{"Run decision tests", "go tool dango lint"}, {"List decisions", "go tool dango lint --list --format json"}},
			Groups:   []clihelp.Group{{Title: "Checks", Flags: []string{"list", "format", "help"}}},
			Notes:    []string{"Requirements: Go and the repository working directory."},
		}
		out.Print(page.String(flags))
		return finish(report, out, errs, nil)
	}
	decisions := hygiene.Decisions()
	result := lintResult{Decisions: make([]decisionResult, 0, len(decisions))}
	for _, d := range decisions {
		result.Decisions = append(result.Decisions, decisionResult{d, "not_run", []string{}})
	}
	failed := false
	if !*list {
		events, runErr := runner(testArgs(decisions))
		if runErr != nil {
			return finish(report, out, errs, runErr)
		}
		failed, err = collect(&result, events)
		if err != nil {
			return finish(report, out, errs, err)
		}
	}
	status := "ok"
	if failed {
		status = "failed"
	}
	if report.Machine() {
		err = report.Result(out, status, result)
	} else {
		printResult(out, result, *list)
	}
	if err == nil && failed {
		err = cliio.ErrReported
	}
	return finish(report, out, errs, err)
}

// The shared reporter supplies classification and diagnostics. Only the text
// usage hint differs for this maintainer entry point.
func finish(report cliio.Report, out, errs *cliio.Output, err error) int {
	if _, ok := errors.AsType[*cliio.UsageError](err); ok && !report.Machine() {
		errs.Println(err)
		errs.Println(`Run "go tool dango --help" for usage.`)
		if code := report.Finish(out, errs, nil); code != 0 {
			return code
		}
		return 2
	}
	return report.Finish(out, errs, err)
}

func testArgs(ds []hygiene.Decision) []string {
	var packages, names []string
	for _, d := range ds {
		for _, t := range d.Tests {
			packages = append(packages, "./"+t.Package)
			names = append(names, regexp.QuoteMeta(t.Name))
		}
	}
	slices.Sort(packages)
	packages = slices.Compact(packages)
	slices.Sort(names)
	names = slices.Compact(names)
	args := []string{"test", "-json", "-count=1", "-run", "^(" + strings.Join(names, "|") + ")$"}
	return append(args, packages...)
}

// execute captures Go's test protocol, never interpreting source or deciding policy.
func execute(args []string) ([]testEvent, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Arguments come only from the checked-in decision table.
	// Lint compares generated schema; an inherited authoring setting must not rewrite it.
	cmd.Env = append(cmd.Environ(), "GOZEN_UPDATE_SCHEMA=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return nil, fmt.Errorf("start go test: %w", err)
	}
	events := []testEvent{}
	dec := json.NewDecoder(&stdout)
	for {
		var event testEvent
		if e := dec.Decode(&event); e != nil {
			if errors.Is(e, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go test output: %w", e)
		}
		events = append(events, event)
	}
	// Build failures have no failed test and cannot produce a decision result.
	testFailed := slices.ContainsFunc(events, func(e testEvent) bool { return e.Action == "fail" && e.Test != "" })
	if err != nil && !testFailed {
		return nil, fmt.Errorf("go test failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return events, nil
}

func collect(result *lintResult, events []testEvent) (bool, error) {
	states := map[string]string{}
	logs := map[string][]string{}
	packages := map[string]string{}
	for _, e := range events {
		pkg := strings.TrimPrefix(e.Package, "github.com/nao1215/gozen/")
		name, _, _ := strings.Cut(e.Test, "/")
		key := pkg + "/" + name
		if e.Test == "" {
			if e.Action == "pass" || e.Action == "fail" {
				packages[pkg] = e.Action
			}
			continue
		}
		if e.Test == name && (e.Action == "pass" || e.Action == "fail" || e.Action == "skip") {
			states[key] = e.Action
		}
		if e.Action == "output" && !strings.HasPrefix(e.Output, "===") && !strings.HasPrefix(strings.TrimSpace(e.Output), "---") {
			logs[key] = append(logs[key], strings.Join(strings.Fields(e.Output), " "))
		}
	}
	failed := false
	for i := range result.Decisions {
		d := &result.Decisions[i]
		d.Status = "pass"
		for _, test := range d.Tests {
			key := test.Package + "/" + test.Name
			state := states[key]
			if state != "pass" && state != "fail" {
				return false, fmt.Errorf("enforcing test did not complete: %s (%s)", key, state)
			}
			if packages[test.Package] != "pass" && packages[test.Package] != "fail" {
				return false, fmt.Errorf("test package did not complete: %s", test.Package)
			}
			if state == "fail" {
				failed = true
				d.Status = "fail"
				d.Details = append(d.Details, key+": "+strings.Join(logs[key], " "))
			}
		}
	}
	if !failed {
		for pkg, status := range packages {
			if status == "fail" {
				return false, fmt.Errorf("test package failed outside a registered decision: %s", pkg)
			}
		}
	}
	return failed, nil
}

func printResult(out *cliio.Output, result lintResult, list bool) {
	for _, d := range result.Decisions {
		if list {
			var tests []string
			for _, t := range d.Tests {
				tests = append(tests, t.Package+"."+t.Name)
			}
			out.Printf("%s %s: %s Tests: %s\n", d.ID, d.Name, d.Reason, strings.Join(tests, ", "))
		} else if d.Status == "fail" {
			out.Printf("%s %s: %s Fix: %s %s\n", d.ID, d.Name, d.Reason, d.Fix, strings.Join(d.Details, " "))
		}
	}
	if !list && !slices.ContainsFunc(result.Decisions, func(d decisionResult) bool { return d.Status == "fail" }) {
		out.Println("All decisions pass.")
	}
}

func runRepro(args []string, stdout, stderr io.Writer) int {
	out, errs := &cliio.Output{Writer: stdout}, &cliio.Output{Writer: stderr}
	flags := clihelp.NewFlagSet("dango repro")
	target := flags.String("repro", "", "reproduction target: TICK, TIME (12.5s), or PNG")
	dir := flags.StringP("out", "o", "", "write artifacts to a new DIR")
	format := flags.String("format", "text", "result representation: text or json")
	report := cliio.Report{Command: "dango repro", Format: clihelp.OutputFormat(flags, args)}
	help, err := clihelp.Parse(flags, args)
	if err != nil {
		return finish(report, out, errs, cliio.Parse(err))
	}
	report.Format = *format
	if *format != "text" && *format != "json" {
		return finish(report, out, errs, cliio.Invalid(diag.NotAChoice, "--format must be text or json"))
	}
	if help {
		page := clihelp.Page{
			Name: "dango repro", Summary: "write trace reproduction artifacts",
			Usage:    []string{"go tool dango repro FILE --repro TICK|TIME|PNG [--out DIR] [--format text|json]"},
			Examples: []clihelp.Example{{"Bundle a trace", "go tool dango repro run.jsonl --repro 60 --out repro"}},
			Groups:   []clihelp.Group{{Title: "Reproduction", Flags: []string{"repro", "out", "format", "help"}}},
			Notes:    []string{"Requirements: a recorded trace file. Generated commands are never executed."},
		}
		out.Print(page.String(flags))
		return finish(report, out, errs, nil)
	}
	if flags.NArg() != 1 || flags.Arg(0) == "-" || *target == "" {
		return finish(report, out, errs, cliio.Invalid(diag.BadValue, "repro needs a trace file and --repro TICK|TIME|PNG"))
	}
	bundle, err := repro.Generate(repro.Config{Path: flags.Arg(0), Target: *target, Out: *dir})
	if err == nil {
		if report.Machine() {
			err = report.Result(out, "ok", bundle)
		} else {
			out.Println(bundle.Command)
		}
	}
	return finish(report, out, errs, err)
}
