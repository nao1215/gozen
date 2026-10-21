package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	code = Run(args, &o, &d)
	return code, o.String(), d.String()
}

func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)

	// Success and help are 0.
	if code, out, _ := exec("--help"); code != 0 || !strings.Contains(out, "gozen art prompt") {
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
		{"-c", cat, "-c", cat},
		{"-c", cat, "--format", "yaml"},
		{"-c", cat, "stray"},
		{"-c", cat, "--scene", "open", "--plan", "p.yaml"},
		{"-c", cat, "--nosuchflag"},
		{"-config", cat},
	} {
		code, _, diag := exec(args...)
		if code != 2 || !strings.Contains(diag, "gozen art prompt --help") {
			t.Errorf("%v: code %d diag %q", args, code, diag)
		}
	}
	// Other failures are 1 and print the requirements.
	for _, args := range [][]string{
		{"-c", filepath.Join(dir, "none.yaml")},
		{"-c", cat, "--scene", "nowhere"},
		{"-c", cat, "--hero", "ghost"},
		{"-c", cat, "--plan", filepath.Join(dir, "none-plan.yaml")},
	} {
		code, _, diag := exec(args...)
		if code != 1 || !strings.Contains(diag, "Requirements") {
			t.Errorf("%v: code %d diag %q", args, code, diag)
		}
	}
	// A diagnostics writer that fails still yields exit 1.
	if code := Run([]string{"-c", filepath.Join(dir, "none.yaml")}, &bytes.Buffer{}, failWriter{}); code != 1 {
		t.Fatalf("failing diagnostics gave %d", code)
	}
	if code := Run([]string{}, &bytes.Buffer{}, failWriter{}); code != 1 {
		t.Fatalf("failing diagnostics on a usage error gave %d", code)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestConfigArgForms(t *testing.T) {
	cases := []struct {
		args []string
		want string
		fail bool
	}{
		{[]string{"--config", "a.yaml"}, "a.yaml", false},
		{[]string{"-c", "a.yaml"}, "a.yaml", false},
		{[]string{"--config=a.yaml"}, "a.yaml", false},
		{[]string{"-ca.yaml"}, "a.yaml", false},
		{[]string{"--scene", "x", "-c", "a.yaml"}, "a.yaml", false},
		{[]string{"--", "-c", "a.yaml"}, "", true}, // after -- nothing is an option
		{[]string{"-c"}, "", true},
		{[]string{"--config"}, "", true},
		{[]string{"-c", "a.yaml", "--config=b.yaml"}, "", true},
		{[]string{"-configx", "a"}, "", true},
		{nil, "", true},
	}
	for _, c := range cases {
		got, err := configArg(c.args)
		if (err != nil) != c.fail || got != c.want {
			t.Errorf("%v: got %q, %v", c.args, got, err)
		}
		if c.fail {
			if _, ok := errors.AsType[usageError](err); !ok {
				t.Errorf("%v: not a usage error: %v", c.args, err)
			}
		}
	}
}

func TestLayerNamedLikeAFixedOption(t *testing.T) {
	dir := t.TempDir()
	// The catalog refuses the names of the fixed options as layer IDs, so a layer cannot shadow one.
	y := strings.Replace(catalogYAML, "id: place", "id: scenes", 1)
	y = strings.ReplaceAll(y, "place:", "scenes:")
	cat := write(t, dir, "c.yaml", y)
	code, _, diag := exec("-c", cat)
	if code != 1 || !strings.Contains(diag, "scenes") {
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
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.Positive != "tall hill, knight, adult" {
		t.Fatalf("plan result %q %v", out, err)
	}
	// A layer flag overrides the plan's choice for that layer.
	_, out, _ = exec("-c", cat, "--plan", plan, "--place", "field", "--format", "json")
	if err := json.Unmarshal([]byte(out), &r); err != nil || !strings.HasPrefix(r.Positive, "green field") {
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
	// Repeating a layer flag selects several; the maximum is enforced.
	if code, _, _ := exec("-c", cat, "--hero", "knight", "--hero", "knight"); code != 1 {
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
	if code != 1 || json.Unmarshal([]byte(out), &r) != nil || r.Positive != "" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Rule != "no_fog" {
		t.Fatalf("json: %d %q", code, out)
	}
	// A failing stdout is reported over the validation error.
	var diagBuf bytes.Buffer
	if got := Run([]string{"-c", cat, "--scene", "open", "--format", "json"}, failWriter{}, &diagBuf); got != 1 {
		t.Fatalf("lost stdout failure: %d", got)
	}
	if got := Run([]string{"-c", cat, "--scene", "open"}, &bytes.Buffer{}, failWriter{}); got != 1 {
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
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("clean lint json: %d %q", code, out)
	}

	rules := write(t, dir, "rules.yaml", "rules:\n- {id: no_knight, kind: forbidden, severity: error, terms: [knight]}\n- {id: soft_hill, kind: forbidden, severity: warning, terms: [tall hill]}\n")
	_ = rules
	failing := write(t, dir, "failing.yaml", strings.Replace(catalogYAML, "negative_limit: 4", "negative_limit: 4\nvalidation: rules.yaml", 1))
	code, out, diag := exec("lint", "-c", failing)
	if code != 1 || !strings.Contains(out, "item/hero/knight error no_knight") || !strings.Contains(out, "scene/rider") || !strings.Contains(diag, "lint found errors") {
		t.Fatalf("failing lint: %d\n%s\n%s", code, out, diag)
	}
	code, out, _ = exec("lint", "-c", failing, "--format", "json")
	var ds []struct{ Rule, Severity, Context string }
	if code != 1 || json.Unmarshal([]byte(out), &ds) != nil || len(ds) == 0 {
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
	for _, extra := range [][]string{{"--scene", "open"}, {"--plan", "p.yaml"}, {"--place", "field"}} {
		code, _, _ := exec(append([]string{"lint", "-c", cat}, extra...)...)
		if code != 2 {
			t.Errorf("lint %v gave %d, want 2", extra, code)
		}
	}
	// Output failures are not swallowed.
	if got := Run([]string{"lint", "-c", cat, "--format", "json"}, failWriter{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("json output failure: %d", got)
	}
	if got := Run([]string{"lint", "-c", warnOnly}, failWriter{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("text output failure: %d", got)
	}
}

func TestBatchFileProblems(t *testing.T) {
	dir := t.TempDir()
	cat := write(t, dir, "catalog.yaml", catalogYAML)
	if code, _, _ := exec("-c", cat, "--scenes", filepath.Join(dir, "none.txt"), "--format", "json"); code != 1 {
		t.Fatalf("missing scene list gave %d", code)
	}
	ids := write(t, dir, "ids.txt", "  open  \r\n\n   \nrider\n")
	code, out, _ := exec("-c", cat, "--scenes", ids, "--format", "json")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("batch: %d %q", code, out)
	}
	// A line ending of Windows leaves the ID intact; the failing one carries the error text.
	bad := write(t, dir, "bad.txt", "open\r\nnowhere\r\n")
	code, out, diag := exec("-c", cat, "--scenes", bad, "--format", "json")
	if code != 1 || !strings.Contains(out, `"error"`) || !strings.Contains(diag, "one or more scenes failed") {
		t.Fatalf("failed scene: %d %q %q", code, out, diag)
	}
	if !strings.Contains(strings.Split(out, "\n")[0], `"id":"open"`) {
		t.Fatalf("first line %q", out)
	}
}
