package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "camp sample 日本語")
	if err := os.CopyFS(dir, os.DirFS("../../e2e/atago/fixtures/game")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func invoke(t *testing.T, want int, args ...string) (string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	got := run(args, &out, &stderr)
	if got != want {
		t.Fatalf("%v: exit %d want %d\nstdout: %s\nstderr: %s", args, got, want, out.String(), stderr.String())
	}
	return out.String(), stderr.String()
}

func resultData(t *testing.T, output, command string) json.RawMessage {
	t.Helper()
	var result struct {
		Version int             `json:"version"`
		Command string          `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Version != 1 || result.Command != command || len(result.Data) == 0 {
		t.Fatalf("invalid %s result: %s", command, output)
	}
	return result.Data
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// helpPaths are every page: the top, the groups, the leaves, and the leaves of
// the production tools.
func helpPaths() []string {
	paths := []string{"", "prompt", "prompt lint", "mosaic", "models fetch", "run"}
	paths = append(paths, slices.Sorted(maps.Keys(groups))...)
	return append(paths, slices.Sorted(maps.Keys(pages))...)
}

func TestHelpAndArgumentContract(t *testing.T) {
	for _, sub := range helpPaths() {
		args := append(strings.Fields(sub), "--help")
		out, stderr := invoke(t, 0, args...)
		if !strings.Contains(out, "https://github.com/nao1215/gozen") || stderr != "" {
			t.Fatalf("%s: %q %q", sub, out, stderr)
		}
		// Requirements are on the page or on its group's page (ADR 0024).
		if !strings.Contains(out, "Requirements:") {
			parent := strings.Fields(sub)
			if len(parent) > 1 {
				parent = parent[:1]
			} else {
				parent = nil
			}
			up, _ := invoke(t, 0, append(parent, "--help")...)
			if !strings.Contains(up, "Requirements:") {
				t.Errorf("%s: no Requirements: on the page or its group", sub)
			}
		}
		if sub != "" {
			invoke(t, 2, append(strings.Fields(sub), "--unknown")...)
			if sub != "version" && sub != "models fetch" && sub != "theme list" {
				invoke(t, 2, strings.Fields(sub)...)
			}
		}
	}
	invoke(t, 2)
	invoke(t, 2, "absent")
	invoke(t, 2, "check", "one", "two")
	invoke(t, 2, "check", "--only", "fonts", "unused")
	for _, args := range [][]string{
		{"sim", "battle", "unused", "--troop", "lesson", "-n", "0"},
		{"sim", "battle", "unused", "--troop", "lesson", "--level", "-1"},
		{"sim", "dive", "unused", "--location", "grotto", "--grind", "NaN"},
		{"sim", "dive", "unused", "--location", "grotto", "--tries", "0"},
		{"sim", "floors", "unused", "--location", "grotto", "--try", "1"},
		{"sim", "floors", "unused", "--location", "grotto", "--from", "0"},
		{"trace", "unused", "--to", "-1"},
		{"trace", "unused", "--music-late", "Inf"},
		{"trial", "unused", "--config", "../private.yaml"},
		{"trial", "unused", "out", "extra", "-n"},
		{"trial", "unused", "--out", "out", "--overlay", "overlay"},
		{"play", "shots", "unused", "list", "--raw"},
	} {
		out, _ := invoke(t, 2, args...)
		if out != "" {
			t.Fatalf("invalid input wrote stdout: %q", out)
		}
	}
}

func TestGroupsAndUnknownCommands(t *testing.T) {
	for name, n := range tree {
		if n.subs == nil || n.build != nil || name == "" {
			continue
		}
		out, stderr := invoke(t, 2, name)
		if out != "" || !strings.Contains(stderr, "needs a command: "+strings.Join(n.subs, ", ")) || !strings.Contains(stderr, `Run "gozen `+name+` --help"`) {
			t.Fatalf("%s: %q %q", name, out, stderr)
		}
		invoke(t, 2, name, "absent")
	}
	for _, args := range [][]string{{"layout"}, {"curve"}, {"sim", "game", "lesson"}, {"mosaic", "fetch"}} {
		_, stderr := invoke(t, 2, args...)
		if !strings.Contains(stderr, "unknown command") || strings.Contains(stderr, "is now") {
			t.Fatalf("%v: %q", args, stderr)
		}
	}
}

func TestHelpPagesHaveTheSameShape(t *testing.T) {
	for _, sub := range helpPaths() {
		out, _ := invoke(t, 0, append(strings.Fields(sub), "--help")...)
		for _, want := range []string{
			"[Usage]\n", "[Examples]\n", "[Options]\n", "  General:\n", "-h, --help",
			"Documentation:   https://nao1215.github.io/gozen/\n", "Report an issue: https://github.com/nao1215/gozen/issues\n",
			"GitHub Sponsors: https://github.com/sponsors/nao1215\n",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q", sub, want)
			}
		}
		if !strings.HasPrefix(out, strings.TrimSpace("gozen "+sub)+" - ") {
			t.Errorf("%s: first line %q", sub, strings.SplitN(out, "\n", 2)[0])
		}
		if lines := strings.SplitN(out, "\n", 3); len(lines) < 3 || lines[1] != "" {
			t.Errorf("%s: the title line wraps", sub)
		}
		if strings.Contains(out, "  Other:\n") {
			t.Errorf("%s: an option is missing from the groups", sub)
		}
		if _, ok := groups[sub]; (ok || sub == "" || sub == "mosaic" || sub == "prompt") != strings.Contains(out, "[Commands]\n") {
			t.Errorf("%s: [Commands] belongs to the top and the groups only", sub)
		}
		for line := range strings.SplitSeq(out, "\n") {
			// Example command lines may run longer, as in the other tools' help.
			if len([]rune(line)) > 80 && !strings.HasPrefix(line, "    gozen ") && !strings.HasPrefix(line, "  gozen ") && !strings.HasPrefix(line, "    xvfb-run ") {
				t.Errorf("%s: line over 80 columns: %q", sub, line)
			}
		}
	}
	top, _ := invoke(t, 0, "--help")
	if !strings.Contains(top, "[Commands]\n") || !strings.Contains(top, "--version") || !strings.Contains(top, `Run "gozen <command> --help"`) {
		t.Fatal(top)
	}
}

func TestGNUOptionForms(t *testing.T) {
	dir := fixture(t)
	want, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "lesson", "--battles", "3", "--seed", "9")
	for _, args := range [][]string{
		{"sim", "battle", dir, "--troop", "lesson", "--battles=3", "--seed=9"},
		{"sim", "battle", "-n", "3", dir, "--seed", "9", "--troop", "lesson"},
		{"sim", "battle", "-n3", dir, "--troop=lesson", "--seed=9"},
	} {
		if got, _ := invoke(t, 0, args...); got != want {
			t.Fatalf("%v: %q != %q", args, got, want)
		}
	}
	for _, args := range [][]string{
		{"sim", "battle", dir, "--troop", "lesson", "-seed", "9"}, {"check", "-only", "structure", dir}, {"notes", dir, "-out", "x"}, {"-version"},
	} {
		out, stderr := invoke(t, 2, args...)
		if out != "" || !strings.Contains(stderr, "single-dash long option") {
			t.Fatalf("%v: %q %q", args, out, stderr)
		}
	}
	invoke(t, 2, "-v")
	if v, _ := invoke(t, 0, "--version"); !strings.HasPrefix(v, "gozen ") {
		t.Fatal(v)
	}
}

func TestDispatchHelpVersionAndDelegates(t *testing.T) {
	top, _ := invoke(t, 0, "--help")
	for _, sub := range []string{"check", "sim", "capture", "book", "export", "release", "trial", "prompt", "mosaic", "models", "version", "help"} {
		if !strings.Contains(top, "  "+sub+" ") {
			t.Fatalf("top help lacks %s: %s", sub, top)
		}
	}
	if out, _ := invoke(t, 0, "help"); out != top {
		t.Fatal("gozen help differs from gozen --help")
	}
	if out, _ := invoke(t, 0, "help", "help"); out != top {
		t.Fatal("gozen help help differs from gozen --help")
	}
	for _, path := range [][]string{{"sim"}, {"sim", "battle"}, {"mosaic", "add"}, {"models", "fetch"}} {
		viaHelp, _ := invoke(t, 0, append([]string{"help"}, path...)...)
		if direct, _ := invoke(t, 0, append(path, "--help")...); viaHelp != direct {
			t.Fatalf("help %v differs from %v --help", path, path)
		}
	}
	for _, path := range [][]string{{"mosaic"}, {"models", "fetch"}} {
		out, stderr := invoke(t, 0, append([]string{"help"}, path...)...)
		if !strings.HasPrefix(out, "gozen "+strings.Join(path, " ")+" - ") || stderr != "" {
			t.Fatalf("%v: %s %s", path, out, stderr)
		}
	}

	invoke(t, 2, "help", "absent")
	invoke(t, 2, "help", "sim", "extra")
	invoke(t, 2, "version", "extra")
	if out, _ := invoke(t, 0, "version"); !strings.HasPrefix(out, "gozen ") {
		t.Fatal(out)
	}
	if out, _ := invoke(t, 0, "prompt", "--config", "../../docs/examples/production/catalog.yaml", "--scene", "landscape"); !strings.Contains(out, "quiet garden") {
		t.Fatal(out)
	}
	invoke(t, 1, "prompt", "--config", "missing.yaml")
	invoke(t, 2, "mosaic")
	invoke(t, 2, "mosaic", "absent")
}

func TestHelpExplainsCodes(t *testing.T) {
	for _, e := range diag.All() {
		out, stderr := invoke(t, 0, "help", e.Code.String())
		if !strings.HasPrefix(out, e.Code.String()+" - "+e.Summary) || stderr != "" {
			t.Fatalf("%s: %q %q", e.Code, out, stderr)
		}
		if lower, _ := invoke(t, 0, "help", strings.ToLower(e.Code.String())); lower != out {
			t.Fatalf("%s: case matters", e.Code)
		}
	}
	out, stderr := invoke(t, 2, "help", "GZN9999")
	if out != "" || !strings.Contains(stderr, "unknown diagnostic code GZN9999") || !strings.Contains(stderr, diag.URL) {
		t.Fatalf("%q %q", out, stderr)
	}
}

func TestCheckWithoutLayout(t *testing.T) {
	dir := fixture(t)
	out, stderr := invoke(t, 0, "check", "--only", "structure", dir)
	if !strings.Contains(out, "ok: 2 locations") || !strings.Contains(out, "Structure checked") || stderr != "" {
		t.Fatalf("%q %q", out, stderr)
	}
	// Structure-only checks do not parse fonts; full checks do.
	write(t, filepath.Join(dir, "assets", "regular.ttf"), "not a font")
	invoke(t, 0, "check", dir, "--only", "structure")
	invoke(t, 1, "check", dir)
	if got := run([]string{"check", "--only", "structure", dir}, failWriter{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("lost output failure: %d", got)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("failed output") }

func TestReadCommandsAndDeterminism(t *testing.T) {
	dir := fixture(t)
	for _, args := range [][]string{{"check", dir}, {"check", "--only", "layout", dir}, {"book", dir}, {"sim", "battle", dir, "--troop", "lesson", "-n", "3"}, {"sim", "dive", dir, "--location", "grotto", "--floors", "1", "--battles", "1", "--tries", "1"}, {"sim", "floors", dir, "--location", "grotto", "--floor", "1", "--try", "2"}} {
		out, stderr := invoke(t, 0, args...)
		if out == "" || stderr != "" {
			t.Fatalf("%v: %q %q", args, out, stderr)
		}
		out2, _ := invoke(t, 0, args...)
		if out2 != out {
			t.Fatalf("nondeterministic %v", args)
		}
	}
	before, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "lesson", "-n", "3", "--seed", "9")
	after, _ := invoke(t, 0, "sim", "battle", "--seed", "9", dir, "-n", "3", "--troop", "lesson")
	if before != after {
		t.Fatal("flag placement changes result")
	}
	out, _ := invoke(t, 0, "sim", "dive", dir, "--location", "grotto", "--floors", "1", "--battles", "0", "--tries", "1")
	if !strings.Contains(out, "B1") || !strings.Contains(out, "0") {
		t.Fatal(out)
	}
	invoke(t, 2, "sim", "battle", dir, "--troop", "absent", "-n", "1")
	invoke(t, 2, "sim", "dive", dir, "--location", "grotto", "--boss", "x:lesson")
}

func TestTrialRoundTripNoOverwriteAndDryRun(t *testing.T) {
	dir := fixture(t)
	root := t.TempDir()
	out := filepath.Join(root, "trial")
	list := filepath.Join(root, "list.txt")
	invoke(t, 0, "trial", dir, "--out", out, "--list", list, "-n")
	for _, p := range []string{out, list} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry run wrote %s", p)
		}
	}
	invoke(t, 0, "trial", dir, "--out", out, "--list", list)
	if _, err := data.LoadDir(os.DirFS(out), "."); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(t, filepath.Join(dir, "assets", "mote.png")), read(t, filepath.Join(out, "assets", "mote.png"))) {
		t.Fatal("asset changed")
	}
	invoke(t, 1, "trial", dir, "--out", out)
	write(t, filepath.Join(out, "sentinel"), "keep")
	invoke(t, 1, "trial", dir, "--out", out)
	if string(read(t, filepath.Join(out, "sentinel"))) != "keep" {
		t.Fatal("existing output changed")
	}
	invoke(t, 1, "trial", dir, "--out", filepath.Join(dir, "output"))
}

func approvals(t *testing.T, dir string, approved bool) (string, string) {
	t.Helper()
	top := t.TempDir()
	sum := sha256.Sum256(read(t, filepath.Join(dir, "assets", "mote.png")))
	record := map[string]any{"file": "mote.png", "source": hex.EncodeToString(sum[:]), "required": false, "approved": approved}
	b, err := json.Marshal([]any{record})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(top, "approvals.json"), string(b))
	b, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	inspect := filepath.Join(t.TempDir(), "inspections.jsonl")
	write(t, inspect, string(b)+"\n")
	return top, inspect
}

func TestReleaseRoundTripAndApprovalFailure(t *testing.T) {
	dir := fixture(t)
	top, inspect := approvals(t, dir, true)
	out := filepath.Join(t.TempDir(), "release")
	invoke(t, 0, "release", dir, "--overlay", top, "--out", out, "--inspect", inspect)
	if _, err := data.LoadDir(os.DirFS(out), "."); err != nil {
		t.Fatal(err)
	}
	invoke(t, 1, "release", "--inspect", inspect, "--overlay", top, dir, "--out", out)
	top, inspect = approvals(t, dir, false)
	bad := filepath.Join(t.TempDir(), "rejected")
	invoke(t, 1, "release", dir, "--out", bad, "--inspect", inspect, "--overlay", top)
	if _, err := os.Lstat(bad); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected release produced output")
	}
}

func TestDesignAndMemosArtifacts(t *testing.T) {
	dir := fixture(t)
	out := filepath.Join(t.TempDir(), "report")
	invoke(t, 0, "sim", "design", dir, "--out", out, "-v")
	for _, p := range []string{"summary.json", "report.md", "band.png", "timeline/walker.jsonl", "debug.log"} {
		read(t, filepath.Join(out, p))
	}
	var summary map[string]any
	if err := json.Unmarshal(read(t, filepath.Join(out, "summary.json")), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["hash"] != "" {
		t.Fatal("unexpected version metadata")
	}
	invoke(t, 1, "sim", "design", dir, "--out", out)
	config := filepath.Join(dir, "designeval.yaml")
	write(t, config, "personas: [{id: '../escape'}]\n")
	invoke(t, 1, "sim", "design", dir, "--out", filepath.Join(t.TempDir(), "bad"))
	memos := t.TempDir()
	write(t, filepath.Join(memos, "2026-01-01", "001.json"), `{"kind":"note","label":"Note","text":"<marker>","image":"001.png"}`)
	if err := os.WriteFile(filepath.Join(memos, "2026-01-01", "001.png"), read(t, filepath.Join(dir, "assets", "mote.png")), 0o600); err != nil {
		t.Fatal(err)
	}
	html := filepath.Join(t.TempDir(), "html")
	invoke(t, 0, "notes", memos, "--out", html)
	if !strings.Contains(string(read(t, filepath.Join(html, "index.html"))), "&lt;marker&gt;") {
		t.Fatal("HTML was not escaped")
	}
	read(t, filepath.Join(html, "thumbs", "2026-01-01_001.jpg"))
	invoke(t, 1, "notes", memos, "--out", html)
	write(t, filepath.Join(memos, "2026-01-01", "001.json"), `{"image":"../outside.png"}`)
	invoke(t, 1, "notes", memos, "--out", filepath.Join(t.TempDir(), "bad"))
}

func TestTraceValidationAndFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "trace.jsonl")
	write(t, file, "{\"t\":\"say\",\"tick\":60,\"text\":\"A clear path.\"}\n")
	out, _ := invoke(t, 0, "trace", file)
	if !strings.Contains(out, "00:01.00") || !strings.Contains(out, "A clear path.") {
		t.Fatal(out)
	}
	for _, bad := range []string{`null`, `{`, `{"t":"state","party":[3]}`, `{"t":"state","party":null}`, `{"t":"say","tick":-1}`, `{"tick":0}`} {
		write(t, file, bad)
		invoke(t, 1, "trace", file)
	}
	write(t, file, "{\"t\":\"music\",\"tick\":0,\"wanted\":\"bell\",\"song\":\"\"}\n{\"t\":\"end\",\"tick\":120}\n")
	out, _ = invoke(t, 1, "trace", file, "--check")
	if !strings.Contains(out, "music-late") {
		t.Fatal(out)
	}
}

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }
func TestFailedOutputIsNonzero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"sim", "battle", "--help"}, {"book", fixture(t)}, {"sim", "battle", fixture(t), "--troop", "lesson", "-n", "1"}} {
		var stderr bytes.Buffer
		if got := run(args, failingWriter{}, &stderr); got != 1 {
			t.Fatalf("%v: %d", args, got)
		}
	}
}

func TestTrialPrunesUnreachableContent(t *testing.T) {
	dir := fixture(t)
	p := filepath.Join(dir, "data", "world.yaml")
	world := string(read(t, p))
	world = strings.Replace(world, "actions: [{label: Finish, do: [{call: finish}]}]", "actions: [{label: Finish, do: [{call: finish}]}, {label: Locked, when: flag.locked, do: [{call: locked}]}]", 1)
	world += "\n  locked: {do: [{say: A closed gate.}, {image: hidden.png}]}\n"
	write(t, p, world)
	write(t, filepath.Join(dir, "trial.yaml"), "assume: {flag.locked: false}\n")
	if err := os.WriteFile(filepath.Join(dir, "assets", "hidden.png"), read(t, filepath.Join(dir, "assets", "mote.png")), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "trial")
	invoke(t, 0, "trial", dir, "--out", output)
	db, err := data.LoadDir(os.DirFS(output), ".")
	if err != nil {
		t.Fatal(err)
	}
	if db.Events["locked"] != nil {
		t.Fatal("unreachable event retained")
	}
	if _, err := os.Stat(filepath.Join(output, "assets", "hidden.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unreachable asset retained")
	}
}

func TestOperationRegistrationIsInertAndEnvironmentIndependent(t *testing.T) {
	t.Chdir(t.TempDir())
	for path, node := range tree {
		if node.build == nil {
			continue
		}
		t.Run(path, func(t *testing.T) {
			var previous string
			for _, value := range []string{"first-unavailable-library", "second-unavailable-library"} {
				t.Setenv("ONNXRUNTIME_LIB", value)
				var out, stderr bytes.Buffer
				c := &command{path: path, out: &cliio.Output{Writer: &out}, stderr: &cliio.Output{Writer: &stderr}}
				op := c.operation(node)
				if op.Flags == nil || op.Run == nil {
					t.Fatal("incomplete operation")
				}
				page := op.Page.String(op.Flags)
				if out.Len() != 0 || stderr.Len() != 0 || strings.Contains(page, value) {
					t.Fatal("registering help ran a command or exposed environment defaults")
				}
				if previous != "" && page != previous {
					t.Fatal("help changed with the environment")
				}
				previous = page
			}
		})
	}
}

func TestMachineHelpUsesTheRegisteredFlagsWithoutInputs(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ONNXRUNTIME_LIB", "must-not-be-opened")
	out, _ := invoke(t, 0, "help", "--format", "json")
	var catalog struct{ Commands []helpCommand }
	if err := json.Unmarshal(resultData(t, out, "help"), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Commands) != len(tree) {
		t.Fatalf("%d commands, want %d", len(catalog.Commands), len(tree))
	}
	seen := map[string]bool{}
	for _, description := range catalog.Commands {
		if seen[description.Command] {
			t.Fatalf("duplicate %s", description.Command)
		}
		seen[description.Command] = true
		_, _, flags := describeCommand(description.Command, tree[description.Command], tree)
		for _, option := range description.Options {
			flag := flags.Lookup(option.Name)
			if flag == nil || flag.DefValue != option.Default || flag.Value.Type() != option.Type {
				t.Fatalf("%s: %+v", description.Command, option)
			}
		}
	}
	one, _ := invoke(t, 0, "help", "capture", "--format", "json")
	var selected struct{ Commands []helpCommand }
	if err := json.Unmarshal(resultData(t, one, "help"), &selected); err != nil || len(selected.Commands) != 1 || selected.Commands[0].Command != "capture" {
		t.Fatalf("%s: %v", one, err)
	}
	lines, _ := invoke(t, 0, "help", "capture", "--format", "jsonl")
	if strings.Count(strings.TrimSpace(lines), "\n") != 1 {
		t.Fatalf("%s", lines)
	}
	files, err := os.ReadDir(".")
	if err != nil || len(files) != 0 {
		t.Fatalf("help changed directory: %v %v", files, err)
	}
	invoke(t, 2, "help", "missing", "--format", "json")
}
