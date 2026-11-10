package docs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"go/scanner"
	"go/token"
	"hash"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// inputsPath records, for every picture of the manifest, a hash of everything that
// makes it: the engine and sample source the run's binary is built from, the sample's
// game files, the run's script and arguments, the picture's recipe and the encoder
// (this package). make docs-images writes it after remaking the pictures; when the
// hashes still match, the committed pictures are current without playing anything
// (TestDocImageInputs, make docs-images-fresh). The full remake (make
// docs-images-check) also checks it, so the two cannot disagree for long.
const inputsPath = "docs/images/inputs.sum"

// inputsVersion changes when the hashing below changes what it reads.
const inputsVersion = "gozen docs images inputs v1"

// notEncoder are the files of this package that do not change a picture: the other
// documentation checks and this file. Every other file of the package is part of the
// encoder, so a new file counts until it is listed here.
var notEncoder = []string{
	"adr_links_test.go", "blocks_test.go", "doc.go", "image_inputs_test.go", "language_test.go",
	"mdlinks_test.go", "schema_test.go", "site_test.go", "steps_test.go",
}

// imageInputs are the hashes of the named inputs ("package ./cmd/gozen", "game
// examples/office/game", "run workboard", "encoder", "go") and of every picture, a hash
// of its recipe and of the inputs it takes.
type imageInputs struct {
	inputs map[string]string
	images map[string]string
	uses   map[string][]string // picture -> the inputs it takes
}

// computeImageInputs hashes the inputs of every picture of m, reading the repository at dir.
func computeImageInputs(m manifest, dir string) (imageInputs, error) {
	h := newInputHasher(dir)
	in := imageInputs{inputs: map[string]string{}, images: map[string]string{}, uses: map[string][]string{}}
	var err error
	if in.inputs["go"], err = h.goModule(); err != nil {
		return in, err
	}
	if in.inputs["encoder"], err = h.encoder(); err != nil {
		return in, err
	}
	for name, s := range m.Samples {
		if in.inputs["package "+s.Package], err = h.mainPackage(s.Package); err != nil {
			return in, fmt.Errorf("sample %s: %w", name, err)
		}
		if s.Game != "" {
			if in.inputs["game "+s.Game], err = h.tree(s.Game); err != nil {
				return in, fmt.Errorf("sample %s: %w", name, err)
			}
		}
	}
	for name, r := range m.Runs {
		if in.inputs["run "+name], err = h.run(r); err != nil {
			return in, fmt.Errorf("run %s: %w", name, err)
		}
	}
	for name, im := range m.Images {
		uses := []string{"go", "encoder"}
		for _, rn := range m.imageRuns([]string{name}) {
			s := m.Samples[m.Runs[rn].Sample]
			uses = append(uses, "run "+rn, "package "+s.Package)
			if s.Game != "" {
				uses = append(uses, "game "+s.Game)
			}
		}
		slices.Sort(uses)
		uses = slices.Compact(uses)
		recipe, err := json.Marshal(im)
		if err != nil {
			return in, err
		}
		d := sha256.New()
		writeLine(d, "%s\nimage %s\n%s\n", inputsVersion, name, recipe)
		for _, u := range uses {
			writeLine(d, "%s %s\n", u, in.inputs[u])
		}
		in.images[name] = hex.EncodeToString(d.Sum(nil))
		in.uses[name] = uses
	}
	return in, nil
}

// inputHasher hashes files of the repository, each once.
type inputHasher struct {
	root  string
	files map[string]string // slash path -> hash
	pkgs  map[string]string // import path -> hash of the package's own files
	// deps are the packages of this module each package imports, kept with its
	// hash so that a package hashed for one sample still leads to its imports for
	// the next: the samples come in map order.
	deps map[string][]string
}

func newInputHasher(dir string) *inputHasher {
	return &inputHasher{root: dir, files: map[string]string{}, pkgs: map[string]string{}, deps: map[string][]string{}}
}

func (h *inputHasher) abs(p string) string {
	return filepath.Join(h.root, filepath.FromSlash(path.Clean(strings.TrimPrefix(p, "./"))))
}

// file hashes one file as it is.
func (h *inputHasher) file(p string) (string, error) {
	if s, ok := h.files[p]; ok {
		return s, nil
	}
	b, err := os.ReadFile(h.abs(p))
	if err != nil {
		return "", err
	}
	s := sum(b)
	h.files[p] = s
	return s, nil
}

// goSource hashes a Go file without its comments, keeping the //go: directives, so a
// change to a comment does not make the pictures stale.
func (h *inputHasher) goSource(p string) (string, error) {
	key := "go:" + p
	if s, ok := h.files[key]; ok {
		return s, nil
	}
	b, err := os.ReadFile(h.abs(p))
	if err != nil {
		return "", err
	}
	// The tokens without the comments, and the //go: directives (embed, build, linkname).
	fset := token.NewFileSet()
	var sc scanner.Scanner
	var bad error
	sc.Init(fset.AddFile(p, -1, len(b)), b, func(pos token.Position, msg string) { bad = fmt.Errorf("%s: %s", pos, msg) }, scanner.ScanComments)
	var buf bytes.Buffer
	for {
		_, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			if strings.HasPrefix(lit, "//go:") {
				buf.WriteString(lit + "\n")
			}
			continue
		}
		fmt.Fprintf(&buf, "%d %s\n", tok, lit)
	}
	if bad != nil {
		return "", bad
	}
	s := sum(buf.Bytes())
	h.files[key] = s
	return s, nil
}

// tree hashes every file under a directory (or one file), with their paths. Names
// that start with a dot are left out, as they are not part of a checkout.
func (h *inputHasher) tree(p string) (string, error) {
	p = path.Clean(strings.TrimPrefix(p, "./"))
	d := sha256.New()
	err := filepath.WalkDir(h.abs(p), func(file string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(e.Name(), ".") && file != h.abs(p) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(h.root, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		s, err := h.file(rel)
		if err != nil {
			return err
		}
		writeLine(d, "%s %s\n", rel, s)
		return nil
	})
	return hex.EncodeToString(d.Sum(nil)), err
}

// goModule hashes go.mod: the Go version and the versions of every dependency
// outside this module.
func (h *inputHasher) goModule() (string, error) {
	b, err := os.ReadFile(h.abs("go.mod"))
	if err != nil {
		return "", err
	}
	// This local maintainer entry has no effect on runtime dependencies. Keep
	// hashing all dependency directives, including any future external tools.
	b = bytes.ReplaceAll(b, []byte("\n\ntool github.com/nao1215/gozen/cmd/internal/dango\n"), []byte("\n"))
	return sum(b), nil
}

// encoder hashes the files of this package that make, encode and compare the pictures.
func (h *inputHasher) encoder() (string, error) {
	entries, err := os.ReadDir(h.abs("internal/dev/docs"))
	if err != nil {
		return "", err
	}
	d := sha256.New()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || slices.Contains(notEncoder, e.Name()) {
			continue
		}
		s, err := h.goSource("internal/dev/docs/" + e.Name())
		if err != nil {
			return "", err
		}
		writeLine(d, "%s %s\n", e.Name(), s)
	}
	return hex.EncodeToString(d.Sum(nil)), nil
}

// modulePath is the path of this repository's module.
func (h *inputHasher) modulePath() (string, error) {
	b, err := os.ReadFile(h.abs("go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

// mainPackage hashes what the binary of a sample is built from: the files of every
// package of this module it imports, as Linux/amd64 with cgo builds them (where the
// pictures are made), and every other file in the sample's own directory (which it
// may read at run time). Packages of other modules are pinned by go.mod.
func (h *inputHasher) mainPackage(pkg string) (string, error) {
	mod, err := h.modulePath()
	if err != nil {
		return "", err
	}
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "linux", "amd64", true
	ctx.BuildTags = nil
	start := path.Clean(strings.TrimPrefix(pkg, "./"))
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if _, ok := h.pkgs[dir]; ok {
			queue = append(queue, h.deps[dir]...)
			continue
		}
		p, err := ctx.ImportDir(h.abs(dir), 0)
		if err != nil {
			return "", fmt.Errorf("package %s: %w", dir, err)
		}
		d := sha256.New()
		for _, f := range append(slices.Clone(p.GoFiles), p.CgoFiles...) {
			s, err := h.goSource(dir + "/" + f)
			if slices.Contains(p.CgoFiles, f) {
				s, err = h.file(dir + "/" + f) // the cgo preamble is a comment
			}
			if err != nil {
				return "", err
			}
			writeLine(d, "%s %s\n", f, s)
		}
		other := slices.Concat(p.CFiles, p.CXXFiles, p.HFiles, p.SFiles, p.SysoFiles, p.MFiles, p.FFiles)
		slices.Sort(other)
		for _, f := range other {
			s, err := h.file(dir + "/" + f)
			if err != nil {
				return "", err
			}
			writeLine(d, "%s %s\n", f, s)
		}
		for _, pattern := range p.EmbedPatterns {
			matches, err := filepath.Glob(filepath.Join(h.abs(dir), filepath.FromSlash(strings.TrimPrefix(pattern, "all:"))))
			if err != nil || len(matches) == 0 {
				return "", fmt.Errorf("package %s: embed %s matches nothing", dir, pattern)
			}
			for _, match := range matches {
				rel, err := filepath.Rel(h.root, match)
				if err != nil {
					return "", err
				}
				s, err := h.tree(filepath.ToSlash(rel))
				if err != nil {
					return "", err
				}
				writeLine(d, "embed %s %s\n", filepath.ToSlash(rel), s)
			}
		}
		h.pkgs[dir] = hex.EncodeToString(d.Sum(nil))
		for _, imp := range p.Imports {
			if rel, ok := strings.CutPrefix(imp, mod+"/"); ok {
				h.deps[dir] = append(h.deps[dir], rel)
			}
		}
		queue = append(queue, h.deps[dir]...)
	}
	d := sha256.New()
	for _, dir := range sortedKeys(seen) {
		writeLine(d, "package %s %s\n", dir, h.pkgs[dir])
	}
	// What the sample itself may read from its directory, beside its Go files.
	entries, err := os.ReadDir(h.abs(start))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		s, err := h.tree(start + "/" + e.Name())
		if err != nil {
			return "", err
		}
		writeLine(d, "file %s/%s %s\n", start, e.Name(), s)
	}
	return hex.EncodeToString(d.Sum(nil)), nil
}

// run hashes a run's recipe, its script, the repository files its arguments and check
// name (such as a check script), and the game directory its edit copies.
func (h *inputHasher) run(r run) (string, error) {
	recipe, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	d := sha256.New()
	writeLine(d, "%s\n", recipe)
	named := slices.Concat([]string{r.Script}, r.Args, r.Check)
	if r.Edit != nil {
		named = append(named, r.Edit.Copy)
	}
	for _, p := range named {
		if p == "" || strings.Contains(p, "{") || filepath.IsAbs(p) || strings.HasPrefix(p, "-") {
			continue
		}
		if _, err := os.Stat(h.abs(p)); err != nil {
			continue // a plain argument, not a file
		}
		s, err := h.tree(p)
		if err != nil {
			return "", err
		}
		writeLine(d, "file %s %s\n", path.Clean(p), s)
	}
	return hex.EncodeToString(d.Sum(nil)), nil
}

// writeLine adds a formatted line to a hash, which never fails to write.
func writeLine(d hash.Hash, format string, args ...any) {
	d.Write(fmt.Appendf(nil, format, args...))
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// readImageInputs reads inputsPath; a missing file reads as empty.
func readImageInputs(file string) (imageInputs, error) {
	in := imageInputs{inputs: map[string]string{}, images: map[string]string{}}
	b, err := os.ReadFile(file) //nolint:gosec // The committed hash file.
	if errors.Is(err, fs.ErrNotExist) {
		return in, nil
	}
	if err != nil {
		return in, err
	}
	for i, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hash, rest, ok := strings.Cut(line, " ")
		kind, name, ok2 := strings.Cut(rest, " ")
		if !ok || !ok2 || len(hash) != 64 {
			return in, fmt.Errorf("%s:%d: want HASH image NAME or HASH input NAME", inputsPath, i+1)
		}
		switch kind {
		case "image":
			in.images[name] = hash
		case "input":
			in.inputs[name] = hash
		default:
			return in, fmt.Errorf("%s:%d: unknown kind %q", inputsPath, i+1, kind)
		}
	}
	return in, nil
}

// format writes the hash file: the inputs, then the pictures, sorted.
func (in imageInputs) format() []byte {
	var b bytes.Buffer
	b.WriteString("# The inputs of every picture of docs/images/manifest.yaml (docs/development/docs.md).\n" +
		"# Written by make docs-images; checked by go test ./internal/dev/docs (make docs-images-fresh).\n" +
		"# Do not edit: a picture whose hash differs is remade by make docs-images.\n")
	for _, k := range sortedKeys(in.inputs) {
		fmt.Fprintf(&b, "%s input %s\n", in.inputs[k], k)
	}
	for _, k := range sortedKeys(in.images) {
		fmt.Fprintf(&b, "%s image %s\n", in.images[k], k)
	}
	return b.Bytes()
}

// staleImages compares the recorded hashes with the current ones for the given
// pictures and explains each difference.
func staleImages(recorded, current imageInputs, names []string) []string {
	var bad []string
	for _, name := range names {
		was, ok := recorded.images[name]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("docs/images/%s: not recorded", name))
		case was != current.images[name]:
			var changed []string
			for _, u := range current.uses[name] {
				if recorded.inputs[u] != current.inputs[u] {
					changed = append(changed, u)
				}
			}
			why := "its recipe changed"
			if len(changed) > 0 {
				why = "changed: " + strings.Join(changed, ", ")
			}
			bad = append(bad, fmt.Sprintf("docs/images/%s: %s", name, why))
		}
	}
	return bad
}

// recordImageInputs writes the current hashes of the given pictures (the ones just
// remade) into inputsPath, keeping the other recorded pictures and dropping pictures
// that left the manifest.
func recordImageInputs(file string, m manifest, current imageInputs, names []string) error {
	recorded, err := readImageInputs(file)
	if err != nil {
		return err
	}
	for name := range recorded.images {
		if _, ok := m.Images[name]; !ok {
			delete(recorded.images, name)
		}
	}
	for _, name := range names {
		recorded.images[name] = current.images[name]
		for _, u := range current.uses[name] {
			recorded.inputs[u] = current.inputs[u]
		}
	}
	for u := range recorded.inputs {
		if _, ok := current.inputs[u]; !ok {
			delete(recorded.inputs, u)
		}
	}
	return os.WriteFile(file, recorded.format(), 0o644) //nolint:gosec // A committed file, readable like the others.
}

// staleMessage is the failure of the fast check.
func staleMessage(stale []string) string {
	return fmt.Sprintf("%d documentation pictures may be stale: what makes them changed since %s was written.\n"+
		"Remake them with `make docs-images ONLY=%s` (or all with `make docs-images`), look at the changed pictures, and commit them with %s:\n  %s",
		len(stale), inputsPath, strings.Join(staleNames(stale), ","), inputsPath, strings.Join(stale, "\n  "))
}

// TestDocImageInputs is the fast staleness check: without a display it hashes what
// makes every picture and compares it with inputsPath. A picture whose hash matches
// was made from the same inputs, so it is current; the Docs workflow remakes the
// pictures whenever inputsPath changes, and nightly, to prove it.
//
// With GOZEN_DOCS_INPUTS=record (make docs-images-record) it writes the current
// hashes of every picture instead, without remaking any: for a change that
// cannot alter how a picture looks. The Docs workflow then remakes the pictures
// because inputsPath changed, and fails if one of them does differ.
func TestDocImageInputs(t *testing.T) {
	t.Parallel()
	m := loadManifest(t)
	current, err := computeImageInputs(m, root)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOZEN_DOCS_INPUTS") == "record" {
		if err := recordImageInputs(filepath.Join(root, inputsPath), m, current, sortedKeys(m.Images)); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded the inputs of %d pictures in %s without remaking them", len(m.Images), inputsPath)
		return
	}
	recorded, err := readImageInputs(filepath.Join(root, inputsPath))
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for name := range recorded.images {
		if _, ok := m.Images[name]; !ok {
			bad = append(bad, fmt.Sprintf("docs/images/%s: recorded but not in the manifest", name))
		}
	}
	bad = append(bad, staleImages(recorded, current, sortedKeys(m.Images))...)
	if len(bad) > 0 {
		slices.Sort(bad)
		t.Fatal(staleMessage(bad))
	}
}

// The hash follows each kind of input, and only the pictures that take it.
func TestImageInputsFollowChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		f := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.26\n")
	write("internal/dev/docs/images_test.go", "package docs\n")
	write("internal/dev/docs/steps_test.go", "package docs\n")
	write("cmd/a/main.go", "package main\n\nimport _ \"example.com/m/engine\"\n\nfunc main() {}\n")
	write("cmd/b/main.go", "package main\n\nfunc main() {}\n")
	write("cmd/b/assets/x.txt", "x")
	write("engine/e.go", "package engine\n\n// Draw draws.\nfunc Draw() int { return 1 }\n")
	write("engine/e_test.go", "package engine\n")
	write("game/game.yaml", "title: a\n")
	write("s.txt", "shot one\n")
	write("check.py", "pass\n")
	m := manifest{
		Samples: map[string]sample{"a": {Package: "./cmd/a", Game: "game"}, "b": {Package: "./cmd/b"}},
		Runs: map[string]run{
			"ra": {Sample: "a", Script: "s.txt", Args: []string{"--shots", "{out}"}, Check: []string{"python3", "check.py"}},
			"rb": {Sample: "b", Script: "s.txt"},
		},
		Images: map[string]image{"a.jpg": {Run: "ra", Shot: "one"}, "b.jpg": {Run: "rb", Shot: "one"}},
	}
	base, err := computeImageInputs(m, dir)
	if err != nil {
		t.Fatal(err)
	}
	stale := func(names ...string) {
		t.Helper()
		now, err := computeImageInputs(m, dir)
		if err != nil {
			t.Fatal(err)
		}
		got := staleImages(base, now, sortedKeys(m.Images))
		if len(got) != len(names) {
			t.Fatalf("stale %v, want %v", got, names)
		}
		for i, n := range names {
			if !strings.HasPrefix(got[i], "docs/images/"+n+":") {
				t.Fatalf("stale %v, want %v", got, names)
			}
		}
		base = now
	}
	write("engine/e.go", "package engine\n\n// Draw draws the screen.\nfunc Draw() int { return 1 }\n")
	write("engine/e_test.go", "package engine // tests\n")
	write("internal/dev/docs/steps_test.go", "package docs // steps\n")
	stale() // comments, tests and the other docs checks change no picture
	write("go.mod", "module example.com/m\n\ngo 1.26\n\ntool github.com/nao1215/gozen/cmd/internal/dango\n")
	stale() // the local maintainer command cannot change runtime pictures
	write("go.mod", "module example.com/m\n\ngo 1.26\n\ntool github.com/nao1215/gozen/cmd/internal/dango\n\nrequire example.com/library v1.0.0\n")
	stale("a.jpg", "b.jpg") // dependency changes remain rendering inputs
	write("go.mod", "module example.com/m\n\ngo 1.26\n")
	stale("a.jpg", "b.jpg")
	write("engine/e.go", "package engine\n\nfunc Draw() int { return 2 }\n")
	stale("a.jpg")
	write("game/game.yaml", "title: b\n")
	stale("a.jpg")
	write("check.py", "assert True\n")
	stale("a.jpg")
	write("cmd/b/assets/x.txt", "y")
	stale("b.jpg")
	write("s.txt", "wait 1\nshot one\n")
	stale("a.jpg", "b.jpg")
	write("internal/dev/docs/images_test.go", "package docs\n\nconst q = 1\n")
	stale("a.jpg", "b.jpg")
	m.Images["b.jpg"] = image{Run: "rb", Shot: "one", Tolerance: 0.1}
	stale("b.jpg")

	file := filepath.Join(dir, "inputs.sum")
	if err := recordImageInputs(file, m, base, []string{"a.jpg"}); err != nil {
		t.Fatal(err)
	}
	got, err := readImageInputs(file)
	if err != nil {
		t.Fatal(err)
	}
	if bad := staleImages(got, base, []string{"a.jpg", "b.jpg"}); len(bad) != 1 || !strings.Contains(bad[0], "b.jpg: not recorded") {
		t.Fatalf("after recording a.jpg: %v", bad)
	}
}

// A sample's package hash does not depend on the samples hashed before it with
// the same hasher: the samples come in map order, and a package already hashed
// for one must still lead to the packages it imports.
func TestImageInputsIndependentOfSampleOrder(t *testing.T) {
	t.Parallel()
	m := loadManifest(t)
	var pkgs []string
	for _, s := range m.Samples {
		if !slices.Contains(pkgs, s.Package) {
			pkgs = append(pkgs, s.Package)
		}
	}
	slices.Sort(pkgs)
	alone := map[string]string{}
	for _, pkg := range pkgs {
		s, err := newInputHasher(root).mainPackage(pkg)
		if err != nil {
			t.Fatal(err)
		}
		alone[pkg] = s
	}
	reversed := slices.Clone(pkgs)
	slices.Reverse(reversed)
	for _, order := range [][]string{pkgs, reversed} {
		h := newInputHasher(root)
		for _, pkg := range order {
			s, err := h.mainPackage(pkg)
			if err != nil {
				t.Fatal(err)
			}
			if s != alone[pkg] {
				t.Errorf("%s: hashed after %v differs from hashed alone", pkg, order)
			}
		}
	}
}
