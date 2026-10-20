package docs

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

// docsVersion is the version the local module proxy serves this checkout as, so
// `go install github.com/nao1215/gozen/...@latest` in a step builds this checkout.
const docsVersion = "v0.0.0-docs"

const modulePath = "github.com/nao1215/gozen"

// TestDocSteps runs the shell steps of README.md and the website as a reader
// would, in temporary directories, against this checkout. It runs only when
// GOZEN_DOCS_STEPS=1 (make docs-steps): it builds Go programs and runs one game
// under xvfb-run. GOZEN_DOCS_ONLY=FILE limits it to one document.
//
// A local module proxy serves this checkout as the latest version of the
// module, so `go install`, `go run ...@latest` and `go get` are run as written
// and fetch nothing of Gozen from the network. The dependencies come from the
// module cache, then from GOPROXY.
func TestDocSteps(t *testing.T) {
	if os.Getenv("GOZEN_DOCS_STEPS") != "1" {
		t.Skip("set GOZEN_DOCS_STEPS=1 to run the documentation's steps (make docs-steps)")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	env := stepEnv(t, rootAbs, tmp)
	only := os.Getenv("GOZEN_DOCS_ONLY")
	for _, doc := range stepDocuments(t) {
		if only != "" && only != doc {
			continue
		}
		t.Run(doc, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, doc)) //nolint:gosec // A document of this repository.
			if err != nil {
				t.Fatal(err)
			}
			blocks, bad := parseBlocks(doc, src)
			if len(bad) > 0 {
				t.Fatal(strings.Join(bad, "\n"))
			}
			page := filepath.Join(tmp, "pages", strings.NewReplacer("/", "_", ".", "_").Replace(doc))
			runPage(t, rootAbs, page, env, blocks)
		})
	}
}

// stepEnv builds the gozen command and the module proxy and returns the
// environment of the steps: a fresh GOPATH and HOME, the build cache of the caller.
func stepEnv(t *testing.T, rootAbs, tmp string) []string {
	t.Helper()
	goEnv := func(key string) string {
		out, err := exec.Command("go", "env", key).Output() //nolint:gosec // go env of a fixed key.
		if err != nil {
			t.Fatalf("go env %s: %v", key, err)
		}
		return strings.TrimSpace(string(out))
	}
	gocache, modcache, upstream := goEnv("GOCACHE"), goEnv("GOMODCACHE"), goEnv("GOPROXY")
	tools := filepath.Join(tmp, "tools")
	command(t, rootAbs, 5*time.Minute, "go", "build", "-trimpath", "-buildvcs=false", "-o", filepath.Join(tools, exe("gozen")), "./cmd/gozen")
	proxy := filepath.Join(tmp, "proxy")
	if err := buildProxy(rootAbs, proxy); err != nil {
		t.Fatal(err)
	}
	gopath := filepath.Join(tmp, "gopath")
	home := filepath.Join(tmp, "home")
	for _, d := range []string{gopath, home} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	proxies := []string{fileURL(proxy), fileURL(filepath.Join(modcache, "cache", "download"))}
	if upstream != "" && upstream != "off" {
		proxies = append(proxies, upstream)
	}
	keep := []string{"PATH", "DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "TMPDIR", "SystemRoot", "LANG", "CC", "CGO_ENABLED"}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if slices.Contains(keep, k) && k != "PATH" {
			env = append(env, kv)
		}
	}
	return append(env,
		"PATH="+strings.Join([]string{filepath.Join(gopath, "bin"), tools, os.Getenv("PATH")}, string(os.PathListSeparator)),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GOPATH="+gopath,
		"GOCACHE="+gocache,
		"GOPROXY="+strings.Join(proxies, ","),
		"GONOSUMDB="+modulePath,
		"GOPRIVATE=",
		"GOFLAGS=-modcacherw",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// buildProxy writes a GOPROXY directory that serves the checkout as docsVersion:
// the tracked and untracked (not ignored) files, as `go mod download` would see a release.
func buildProxy(rootAbs, dir string) error {
	out, err := exec.Command("git", "-C", rootAbs, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output() //nolint:gosec // git on this checkout.
	if err != nil {
		return fmt.Errorf("listing the module's files: %w", err)
	}
	at := filepath.Join(dir, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(at, 0o750); err != nil {
		return err
	}
	names := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	// A directory with its own go.mod (examples/go-zen) is another module and is
	// not part of this module's zip, as in a release.
	var nested []string
	for _, name := range names {
		if path.Base(name) == "go.mod" && name != "go.mod" {
			nested = append(nested, path.Dir(name)+"/")
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		if slices.ContainsFunc(nested, func(dir string) bool { return strings.HasPrefix(name, dir) }) {
			continue
		}
		info, err := os.Lstat(filepath.Join(rootAbs, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			continue // deleted in the working tree, or a link
		}
		b, err := os.ReadFile(filepath.Join(rootAbs, filepath.FromSlash(name))) //nolint:gosec // A file git lists in this checkout.
		if err != nil {
			return err
		}
		w, err := zw.Create(modulePath + "@" + docsVersion + "/" + name)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(rootAbs, "go.mod")) //nolint:gosec // This module's go.mod.
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"list":                []byte(docsVersion + "\n"),
		docsVersion + ".info": []byte(`{"Version":"` + docsVersion + `","Time":"2026-01-01T00:00:00Z"}`),
		docsVersion + ".mod":  mod,
		docsVersion + ".zip":  buf.Bytes(),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(at, name), b, 0o600); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, filepath.FromSlash(modulePath), "@latest"), files[docsVersion+".info"], 0o600)
}

// runPage runs the blocks of one document in order. Steps without `each` share
// one directory and its current directory, as a reader in one terminal.
func runPage(t *testing.T, rootAbs, page string, env []string, blocks []block) {
	t.Helper()
	cwd := filepath.Join(page, "work")
	if err := os.MkdirAll(cwd, 0o750); err != nil {
		t.Fatal(err)
	}
	var stdout string
	n := 0
	for _, b := range blocks {
		kind := ""
		if b.mark != nil {
			kind = b.mark.kind
		}
		switch {
		case kind == "skip":
			t.Logf("%s: skipped: %s", b.where, b.mark.reason)
		case kind == "file":
			p := filepath.Join(cwd, filepath.FromSlash(b.mark.opts["path"]))
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(b.body), 0o600); err != nil {
				t.Fatal(err)
			}
		case kind == "output":
			if missing := missingLines(b.body, stdout); missing != "" {
				t.Errorf("%s: the output above it does not print %q; it printed:\n%s", b.where, missing, stdout)
			}
		case shellLangs[b.lang]:
			n++
			var err error
			stdout, cwd, err = runShell(t, rootAbs, filepath.Join(page, fmt.Sprintf("step-%02d", n)), cwd, env, b)
			if err != nil {
				t.Errorf("%s: %v", b.where, err)
			}
		}
	}
}

// commands splits a shell block into its commands: lines joined at a trailing
// backslash, without blank and comment lines.
func commands(body string) []string {
	var cmds []string
	var cur strings.Builder
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if cur.Len() == 0 && (trimmed == "" || strings.HasPrefix(trimmed, "#")) {
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			cur.WriteString(strings.TrimSpace(strings.TrimSuffix(trimmed, "\\")) + " ")
			continue
		}
		cur.WriteString(trimmed)
		cmds = append(cmds, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		cmds = append(cmds, cur.String())
	}
	return cmds
}

var goRun = regexp.MustCompile(`\bgo run\s+(\S+).*$`)

// compileOnly turns `go run TARGET ARGS` into a build of TARGET, for steps that
// would open a window and wait for a player.
func compileOnly(cmd string) (string, bool) {
	m := goRun.FindStringSubmatchIndex(cmd)
	if m == nil {
		return cmd, false
	}
	target := cmd[m[2]:m[3]]
	build := `go build -o "$GOZEN_DOCS_TMP/a.out" ` + target
	if strings.Contains(target, "@") {
		build = "go install " + target
	}
	return cmd[:m[0]] + build, true
}

func runShell(t *testing.T, rootAbs, dir, cwd string, env []string, b block) (stdout, newCWD string, err error) {
	t.Helper()
	mk := b.mark
	if mk == nil {
		mk = &mark{kind: "run", opts: map[string]string{}}
	}
	type numbered struct {
		n   int // the command's number in the block, 1-based
		cmd string
	}
	skip := map[int]bool{}
	if e := mk.opts["except"]; e != "" {
		for n := range strings.SplitSeq(e, ",") {
			v, err := strconv.Atoi(n)
			if err != nil {
				return "", cwd, fmt.Errorf("verify: except=%s: %w", e, err)
			}
			skip[v] = true
		}
	}
	all := commands(b.body)
	for n := range skip {
		if n > len(all) {
			return "", cwd, fmt.Errorf("verify: except=%d, but the block has %d commands", n, len(all))
		}
	}
	var cmds []numbered
	for i, c := range all {
		if skip[i+1] {
			t.Logf("%s: command %d skipped: %s", b.where, i+1, mk.reason)
			continue
		}
		cmds = append(cmds, numbered{i + 1, c})
	}
	if mk.kind == "build" {
		built := 0
		for i, c := range cmds {
			var ok bool
			if cmds[i].cmd, ok = compileOnly(c.cmd); ok {
				built++
			}
		}
		if built == 0 {
			return "", cwd, fmt.Errorf("verify: build, but no `go run` to build")
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", cwd, err
	}
	if mk.kind != "each" {
		if err := copyFixture(rootAbs, mk.opts["fixture"], cwd); err != nil {
			return "", cwd, err
		}
		state := filepath.Join(dir, "cwd")
		var script strings.Builder
		script.WriteString("set -eo pipefail\n")
		for _, c := range cmds {
			script.WriteString(c.cmd + "\n")
		}
		script.WriteString("pwd > " + strconv.Quote(state) + "\n")
		out, err := bash(t, dir, cwd, env, script.String())
		if err != nil {
			return out, cwd, err
		}
		next, err := os.ReadFile(state) //nolint:gosec // A file the step's script wrote.
		if err != nil {
			return out, cwd, err
		}
		return out, strings.TrimSpace(string(next)), nil
	}
	var printed strings.Builder
	var errs []string
	for _, c := range cmds {
		each := filepath.Join(dir, fmt.Sprintf("each-%02d", c.n))
		if err := os.MkdirAll(each, 0o750); err != nil {
			return "", cwd, err
		}
		if err := copyFixture(rootAbs, mk.opts["fixture"], each); err != nil {
			return "", cwd, err
		}
		out, err := bash(t, dir, each, env, "set -eo pipefail\n"+c.cmd+"\n")
		printed.WriteString(out)
		if err != nil {
			errs = append(errs, fmt.Sprintf("command %d `%s`: %v", c.n, c.cmd, err))
		}
	}
	if len(errs) > 0 {
		return printed.String(), cwd, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return printed.String(), cwd, nil
}

func bash(t *testing.T, dir, cwd string, env []string, script string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", script) //nolint:gosec // The steps of this repository's documents, in a temporary directory.
	cmd.Dir = cwd
	cmd.Env = append(slices.Clone(env), "GOZEN_DOCS_TMP="+dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%w\n%s\nstdout:\n%s\nstderr:\n%s", err, script, tail(stdout.String()), tail(stderr.String()))
	}
	return stdout.String(), nil
}

// copyFixture copies testdata/fixtures/NAME into dir, and the repository files
// its fixture.yaml names.
func copyFixture(rootAbs, name, dir string) error {
	if name == "" {
		return nil
	}
	src := filepath.Join("testdata", "fixtures", name)
	if err := copyTree(src, dir, func(rel string) bool { return rel != "fixture.yaml" }); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(src, "fixture.yaml")) //nolint:gosec // A fixture of this package.
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	var spec struct {
		Copy yaml.MapSlice `yaml:"copy"`
	}
	if err := yaml.UnmarshalWithOptions(b, &spec, yaml.Strict()); err != nil {
		return fmt.Errorf("%s: %w", filepath.Join(src, "fixture.yaml"), err)
	}
	for _, item := range spec.Copy {
		dst, from := fmt.Sprint(item.Key), fmt.Sprint(item.Value)
		if err := copyTree(filepath.Join(rootAbs, filepath.FromSlash(from)), filepath.Join(dir, filepath.FromSlash(dst)), nil); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies a file or a directory.
func copyTree(src, dst string, keep func(rel string) bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if keep != nil && rel != "." && !keep(filepath.ToSlash(rel)) {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // A fixture or a file of this repository.
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // The step's own directory.
	})
}

// missingLines returns the first line of want that the output does not print,
// in order; runs of spaces count as one.
func missingLines(want, got string) string {
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	var lines []string
	for l := range strings.SplitSeq(got, "\n") {
		lines = append(lines, norm(l))
	}
	at := 0
	for w := range strings.SplitSeq(want, "\n") {
		w = norm(w)
		if w == "" {
			continue
		}
		i := slices.Index(lines[at:], w)
		if i < 0 {
			return w
		}
		at += i + 1
	}
	return ""
}

func TestCommandsAndCompileOnly(t *testing.T) {
	got := commands("# a comment\n\ngo run . --dev \\\n  --mute   # one\ngozen check game\n")
	if len(got) != 2 || got[0] != "go run . --dev --mute   # one" || got[1] != "gozen check game" {
		t.Fatalf("commands: %q", got)
	}
	for in, want := range map[string]string{
		"go run github.com/nao1215/gozen/examples/minimal@latest --mute": "go install github.com/nao1215/gozen/examples/minimal@latest",
		"xvfb-run -a go run . --dev --script s.txt":                      `xvfb-run -a go build -o "$GOZEN_DOCS_TMP/a.out" .`,
	} {
		if got, ok := compileOnly(in); !ok || got != want {
			t.Errorf("compileOnly(%q) = %q", in, got)
		}
	}
	if _, ok := compileOnly("gozen check game"); ok {
		t.Error("a command without go run was built")
	}
	if m := missingLines("a  b\nc", "x\na b\ny\nc\n"); m != "" {
		t.Errorf("missingLines: %q", m)
	}
	if m := missingLines("c\na b", "a b\nc\n"); m != "a b" {
		t.Errorf("out of order: %q", m)
	}
}
