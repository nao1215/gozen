package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
)

const catalogYAML = `
version: 1
dialect: sdxl
emphasis: [soft, normal, strong]
default_emphasis: normal
priorities: {required: 0, subject: 1}
negative_limit: 4
layers:
  - id: place
    defaults: [field]
    items:
      field: {positive: [{word: green field, priority: subject}], negative: [{word: fog, priority: subject}]}
      hill: {positive: [{word: tall hill, priority: subject}]}
  - id: hero
    person: true
    max: 1
    items:
      knight: {positive: [{word: knight, priority: subject}]}
scenes:
  open: {select: {place: [field]}}
  rider: {select: {place: [hill], hero: [knight]}}
`

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func exec(args ...string) (code int, out, diag string) {
	var o, d bytes.Buffer
	code = runExit(args, &o, &d)
	return code, o.String(), d.String()
}

func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)

	// Success and help are 0.
	if code, out, _ := exec("--help"); code != 0 || !strings.Contains(out, "gozen prompt") {
		t.Fatalf("help: %d", code)
	}
	if code, out, _ := exec("-c", cat, "--scene", "rider"); code != 0 || !strings.Contains(out, "positive: tall hill, knight, adult") {
		t.Fatalf("scene: %d %q", code, out)
	}
	// Bad arguments are 2 and point at --help.
	for _, args := range [][]string{
		{},
		{"--scene", "open"},
		{"-c"},

		{"-c", cat, "--format", "yaml"},
		{"-c", cat, "stray"},
		{"-c", cat, "--scene", "open", "--plan", "p.yaml"},
		{"-c", cat, "--nosuchflag"},
		{"-config", cat},
	} {
		code, _, diag := exec(args...)
		if code != 2 || !strings.Contains(diag, "gozen prompt --help") {
			t.Errorf("%v: code %d diag %q", args, code, diag)
		}
	}
	// File and processing failures exit 1.
	for _, args := range [][]string{
		{"-c", filepath.Join(dir, "none.yaml")},

		{"-c", cat, "--plan", filepath.Join(dir, "none-plan.yaml")},
	} {
		code, _, diag := exec(args...)
		if code != 1 || diag == "" {
			t.Errorf("%v: code %d diag %q", args, code, diag)
		}
	}
	// A diagnostics writer that fails still yields exit 1.
	if code := runExit([]string{"-c", filepath.Join(dir, "none.yaml")}, &bytes.Buffer{}, failWriter{}); code != 1 {
		t.Fatalf("failing diagnostics gave %d", code)
	}
	if code := runExit([]string{}, &bytes.Buffer{}, failWriter{}); code != 1 {
		t.Fatalf("failing diagnostics on a usage error gave %d", code)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFixedOptionsAreParsedBeforeCatalogIO(t *testing.T) {
	for _, args := range [][]string{{"--config", "missing.yaml", "--select", "bad"}, {"--config", "missing.yaml", "--scene", "open", "--plan", "bad.yaml"}, {"--config", "missing.yaml", "--scenes", "-", "--select", "place=field"}, {"--config", "missing.yaml", "--format", "bad"}, {"--config", "missing.yaml", "--bogus"}} {
		code, out, d := exec(args...)
		if code != 2 || out != "" || strings.Contains(d, "no such file") {
			t.Fatalf("%v: %d %q %q", args, code, out, d)
		}
	}
}

func TestLayerNamedLikeAFixedOption(t *testing.T) {
	dir := t.TempDir()
	// Fixed selectors let catalog layers reuse option names safely.
	y := strings.Replace(catalogYAML, "id: place", "id: scenes", 1)
	y = strings.ReplaceAll(y, "place:", "scenes:")
	cat := write(t, dir, "c.yaml", y)
	code, _, diag := exec("-c", cat, "--select", "scenes=hill")
	if code != 0 || diag != "" {
		t.Fatalf("a layer shadowing --scenes: %d %q", code, diag)
	}
}

func TestRunPlanFileAndOverrides(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)
	plan := write(t, dir, "plan.yaml", "select:\n  place: [hill]\n  hero: [knight]\n")
	code, out, _ := exec("-c", cat, "--plan", plan, "--format", "json")
	if code != 0 {
		t.Fatalf("plan: %d", code)
	}
	var r struct{ Positive, Negative string }
	if err := decodeData([]byte(out), &r); err != nil || r.Positive != "tall hill, knight, adult" {
		t.Fatalf("plan result %q %v", out, err)
	}
	// An explicit selector overrides the plan's choice for that layer.
	_, out, _ = exec("-c", cat, "--plan", plan, "--select", "place=field", "--format", "json")
	if err := decodeData([]byte(out), &r); err != nil || !strings.HasPrefix(r.Positive, "green field") {
		t.Fatalf("override %q", out)
	}
	// An empty plan file selects the defaults.
	empty := write(t, dir, "empty.yaml", "select: {}\n")
	if code, out, _ := exec("-c", cat, "--plan", empty); code != 0 || !strings.Contains(out, "green field") {
		t.Fatalf("empty plan: %d %q", code, out)
	}
	nullSel := write(t, dir, "null.yaml", "select:\n")
	if code, out, _ := exec("-c", cat, "--plan", nullSel); code != 0 || !strings.Contains(out, "green field") {
		t.Fatalf("null select: %d %q", code, out)
	}
	bad := write(t, dir, "bad.yaml", "select: {}\nfree: text\n")
	if code, _, _ := exec("-c", cat, "--plan", bad); code != 1 {
		t.Fatalf("a plan with free text gave %d", code)
	}
	// Repeating a selector selects several; the maximum is enforced.
	if code, _, _ := exec("-c", cat, "--select", "hero=knight", "--select", "hero=knight"); code != 1 {
		t.Fatalf("duplicate selection gave %d", code)
	}
}

func TestRunValidationFailureOutput(t *testing.T) {
	dir := t.TempDir()
	rules := write(t, dir, "rules.yaml", "rules:\n- {id: no_fog, kind: forbidden, severity: error, terms: [green field]}\n")
	_ = rules
	y := strings.Replace(catalogYAML, "negative_limit: 4", "negative_limit: 4\nvalidation: rules.yaml", 1)
	cat := write(t, dir, "catalog.yaml", y)
	// In text mode the diagnostics go to the diagnostics writer and nothing to stdout.
	code, out, diag := exec("-c", cat, "--scene", "open")
	if code != 1 || out != "" || !strings.Contains(diag, "no_fog") {
		t.Fatalf("text: %d %q %q", code, out, diag)
	}
	// In JSON mode the failed result, with its diagnostics, is the output.
	code, out, _ = exec("-c", cat, "--scene", "open", "--format", "json")
	var r struct {
		Positive    string
		Diagnostics []struct{ Rule, Severity string }
	}
	if code != 1 || decodeData([]byte(out), &r) != nil || r.Positive != "" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Rule != "no_fog" {
		t.Fatalf("json: %d %q", code, out)
	}
	// A failing stdout is reported over the validation error.
	var diagBuf bytes.Buffer
	if got := runExit([]string{"-c", cat, "--scene", "open", "--format", "json"}, failWriter{}, &diagBuf); got != 1 {
		t.Fatalf("lost stdout failure: %d", got)
	}
	if got := runExit([]string{"-c", cat, "--scene", "open"}, &bytes.Buffer{}, failWriter{}); got != 1 {
		t.Fatalf("lost diagnostics failure: %d", got)
	}
}

func TestRunLint(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)
	code, out, _ := exec("lint", "-c", cat)
	if code != 0 || out != "" {
		t.Fatalf("clean lint: %d %q", code, out)
	}
	code, out, _ = exec("lint", "-c", cat, "--format", "json")
	if code != 0 || !strings.Contains(out, `"data":[]`) {
		t.Fatalf("clean lint json: %d %q", code, out)
	}

	rules := write(t, dir, "rules.yaml", "rules:\n- {id: no_knight, kind: forbidden, severity: error, terms: [knight]}\n- {id: soft_hill, kind: forbidden, severity: warning, terms: [tall hill]}\n")
	_ = rules
	failing := write(t, dir, "failing.yaml", strings.Replace(catalogYAML, "negative_limit: 4", "negative_limit: 4\nvalidation: rules.yaml", 1))
	code, out, diag := exec("lint", "-c", failing)
	if code != 1 || !strings.Contains(out, "item/hero/knight error no_knight") || !strings.Contains(out, "scene/rider") || diag != "" {
		t.Fatalf("failing lint: %d\n%s\n%s", code, out, diag)
	}
	code, out, _ = exec("lint", "-c", failing, "--format", "json")
	var ds []struct{ Rule, Severity, Context string }
	if code != 1 || decodeData([]byte(out), &ds) != nil || len(ds) == 0 {
		t.Fatalf("failing lint json: %d %q", code, out)
	}

	// Warnings alone succeed.
	warnOnly := write(t, dir, "warn.yaml", strings.Replace(catalogYAML, "negative_limit: 4", "negative_limit: 4\nvalidation: warn-rules.yaml", 1))
	write(t, dir, "warn-rules.yaml", "rules:\n- {id: soft_hill, kind: forbidden, severity: warning, terms: [tall hill]}\n")
	code, out, _ = exec("lint", "-c", warnOnly)
	if code != 0 || !strings.Contains(out, "warning soft_hill") {
		t.Fatalf("warnings must not fail lint: %d %q", code, out)
	}

	// Lint takes no selection.
	for _, extra := range [][]string{{"--scene", "open"}, {"--plan", "p.yaml"}, {"--select", "place=field"}} {
		code, _, _ := exec(append([]string{"lint", "-c", cat}, extra...)...)
		if code != 2 {
			t.Errorf("lint %v gave %d, want 2", extra, code)
		}
	}
	// Output failures are not swallowed.
	if got := runExit([]string{"lint", "-c", cat, "--format", "json"}, failWriter{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("json output failure: %d", got)
	}
	if got := runExit([]string{"lint", "-c", warnOnly}, failWriter{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("text output failure: %d", got)
	}
}

func TestBatchFileProblems(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)
	if code, _, _ := exec("-c", cat, "--scenes", filepath.Join(dir, "none.txt"), "--format", "jsonl"); code != 1 {
		t.Fatalf("missing scene list gave %d", code)
	}
	ids := write(t, dir, "ids.txt", "  open  \r\n\n   \nrider\n")
	code, out, _ := exec("-c", cat, "--scenes", ids, "--format", "jsonl")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 3 {
		t.Fatalf("batch: %d %q", code, out)
	}
	// A line ending of Windows leaves the ID intact; the failing one carries the error text.
	bad := write(t, dir, "bad.txt", "open\r\nnowhere\r\n")
	code, out, diag := exec("-c", cat, "--scenes", bad, "--format", "jsonl")
	if code != 1 || !strings.Contains(out, `"error"`) || diag != "" {
		t.Fatalf("failed scene: %d %q %q", code, out, diag)
	}
	if !strings.Contains(strings.Split(out, "\n")[0], `"id":"open"`) {
		t.Fatalf("first line %q", out)
	}
}

// run exercises registration and parsing without introducing a production dispatcher.
func run(args []string, out, diagnostics io.Writer) error {
	lint := len(args) > 0 && args[0] == "lint"
	if lint {
		args = args[1:]
	}
	report := cliio.Report{}
	cfg := Config{Input: strings.NewReader(""), Output: &cliio.Output{Writer: out}, Diagnostics: &cliio.Output{Writer: diagnostics}, Report: &report, Lint: lint}
	op := New(cfg)
	report.Format = clihelp.OutputFormat(op.Flags, args)
	help, err := clihelp.Parse(op.Flags, args)
	if err != nil {
		return cliio.Parse(err)
	}
	if help {
		_, err = io.WriteString(out, op.Page.String(op.Flags))
		return err
	}
	return op.Run(context.Background())
}

func runExit(args []string, out, diagnostics io.Writer) int {
	// Use the common finalizer so failed writes have the same exit precedence.
	o, d := &cliio.Output{Writer: out}, &cliio.Output{Writer: diagnostics}
	report := cliio.Report{Command: "prompt", Format: "text"}
	f := clihelp.NewFlagSet("prompt")
	f.String("format", "text", "encoding")
	report.Format = clihelp.OutputFormat(f, args)
	return report.Finish(o, d, run(args, o, d))
}

func decodeData(b []byte, v any) error {
	var envelope struct{ Data json.RawMessage }
	if err := json.Unmarshal(b, &envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, v)
}

func TestRunContracts(t *testing.T) {
	catalog := filepath.Join("..", "..", "..", "docs", "examples", "production", "catalog.yaml")
	for _, tc := range []struct {
		args     []string
		fail     bool
		contains string
	}{
		{[]string{"--help"}, false, "Requirements"},
		{[]string{"-h"}, false, "[Examples]"},
		{[]string{"lint", "--config", "missing.yaml", "--help"}, false, "Requirements"},
		{[]string{"--config", catalog, "--scene", "landscape", "--format", "json"}, false, "quiet garden"},
		{[]string{"lint", "--config", catalog, "--format", "json"}, false, "[]"},
		{[]string{"--config", catalog, "--format", "bad"}, true, ""},
		{[]string{"--config", catalog, "--plan", "missing.yaml", "--scene", "landscape"}, true, ""},
		{[]string{"--config", "missing.yaml"}, true, ""},
		{[]string{"--config", catalog, "--select", "subject=unknown"}, true, ""},
		{[]string{"--config", catalog, "free text"}, true, ""},
		{[]string{"-c", catalog, "--scene=landscape"}, false, "quiet garden"},
		{[]string{"--config=" + catalog, "--select", "setting=studio"}, false, "plain backdrop"},
		{[]string{"-config", catalog}, true, ""},
		{[]string{"--config", catalog, "-scene", "landscape"}, true, ""},
	} {
		var out, diag bytes.Buffer
		err := run(tc.args, &out, &diag)
		if (err != nil) != tc.fail {
			t.Errorf("%v: %v %s", tc.args, err, &diag)
		}
		if !strings.Contains(out.String(), tc.contains) {
			t.Errorf("%v: output %q", tc.args, &out)
		}
	}
	if err := run([]string{"--config", catalog}, badWriter{}, io.Discard); err == nil {
		t.Fatal("lost output failure")
	}
}

type badWriter struct{}

func (badWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestBatchFormatsPreserveResultsAndStdin(t *testing.T) {
	catalog := write(t, t.TempDir(), "catalog.yaml", catalogYAML)
	var aggregate []sceneResult
	for _, format := range []string{"json", "jsonl", "text"} {
		var out, diagnostics bytes.Buffer
		report := cliio.Report{}
		cfg := Config{Input: strings.NewReader(" open\r\nmissing\n\nrider\nopen\n"), Output: &cliio.Output{Writer: &out}, Diagnostics: &cliio.Output{Writer: &diagnostics}, Report: &report}
		op := New(cfg)
		if _, err := clihelp.Parse(op.Flags, []string{"--config", catalog, "--scenes", "-", "--format", format}); err != nil {
			t.Fatal(err)
		}
		err := op.Run(context.Background())
		if report.Finish(cfg.Output, cfg.Diagnostics, err) != 1 || diagnostics.Len() != 0 {
			t.Fatalf("%s: %v %s", format, err, diagnostics.String())
		}
		switch format {
		case "json":
			if err := decodeData(out.Bytes(), &aggregate); err != nil {
				t.Fatal(err)
			}
			if len(aggregate) != 4 || aggregate[0].ID != "open" || aggregate[1].Error == "" || aggregate[2].ID != "rider" || aggregate[3].ID != "open" {
				t.Fatalf("%+v", aggregate)
			}
		case "jsonl":
			dec := json.NewDecoder(&out)
			for i := range aggregate {
				var v struct {
					Type, ID, Status string
					Data             sceneResult
				}
				if err := dec.Decode(&v); err != nil {
					t.Fatal(err)
				}
				if v.Type != "item" || v.ID != aggregate[i].ID || !reflect.DeepEqual(v.Data, aggregate[i]) {
					t.Fatalf("item %d: %+v", i, v)
				}
			}
			var summary struct {
				Type, Status string
				Data         struct{ Succeeded, Failed int }
			}
			if err := dec.Decode(&summary); err != nil {
				t.Fatal(err)
			}
			if summary.Type != "summary" || summary.Status != "failed" || summary.Data.Succeeded != 3 || summary.Data.Failed != 1 {
				t.Fatalf("%+v", summary)
			}
			if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Fatalf("extra record: %v", err)
			}
		case "text":
			if !strings.Contains(out.String(), "missing:") || strings.Count(out.String(), "open:") != 2 || !strings.Contains(out.String(), "tall hill") {
				t.Fatal(out.String())
			}
		}
	}
}

func TestPromptUnknownExplicitIDsAndCancelledBatch(t *testing.T) {
	catalog := write(t, t.TempDir(), "catalog.yaml", catalogYAML)
	for _, extra := range [][]string{{"--scene", "missing"}, {"--select", "place=missing"}, {"--select", "missing=field"}} {
		code, out, d := exec(append([]string{"--config", catalog, "--format", "json"}, extra...)...)
		var diagnostic struct{ Code string }
		if code != 2 || out != "" || json.Unmarshal([]byte(d), &diagnostic) != nil || diagnostic.Code != "GZN2402" {
			t.Fatalf("%v: %d %q %q", extra, code, out, d)
		}
	}
	report := cliio.Report{}
	var out bytes.Buffer
	op := New(Config{Input: strings.NewReader("open\n"), Output: &cliio.Output{Writer: &out}, Diagnostics: &cliio.Output{Writer: io.Discard}, Report: &report})
	if _, err := clihelp.Parse(op.Flags, []string{"--config", catalog, "--scenes", "-"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := op.Run(ctx); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatalf("%v: %s", err, out.String())
	}
}
