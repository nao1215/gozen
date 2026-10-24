package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
)

// root is the repository root, from this package's directory.
const root = "../../.."

const manifestPath = "docs/images/manifest.yaml"

// manifest is docs/images/manifest.yaml: where every documentation picture comes from.
type manifest struct {
	Samples map[string]sample      `yaml:"samples"`
	Runs    map[string]run         `yaml:"runs"`
	Images  map[string]image       `yaml:"images"`
	Static  map[string]staticImage `yaml:"static"`
}

// staticImage is a picture that is not a capture of a sample, such as the
// logo: it is drawn once from vector sources, and the ledger records where it
// comes from instead of how to remake it.
type staticImage struct {
	Source  string `yaml:"source"`  // what it was made from and with what
	License string `yaml:"license"` // the license of the picture and of what it uses (fonts)
}

// listed reports whether name is in the ledger, made or static.
func (m manifest) listed(name string) bool {
	_, made := m.Images[name]
	_, static := m.Static[name]
	return made || static
}

type sample struct {
	Package string `yaml:"package"`
	Game    string `yaml:"game"`
}

type run struct {
	Sample string   `yaml:"sample"`
	Script string   `yaml:"script"`
	Args   []string `yaml:"args"`
	Check  []string `yaml:"check"`
	Edit   *edit    `yaml:"edit"`
}

// edit changes a copy of a game's files while a run plays, so a picture can show the hot
// reload (F5): the directory Copy is copied to {game} before the run starts, and once the
// run has written the shot After, Old in File (under the copy) becomes New.
type edit struct {
	Copy  string `yaml:"copy"`
	After string `yaml:"after"`
	File  string `yaml:"file"`
	Old   string `yaml:"old"`
	New   string `yaml:"new"`
}

// pick chooses frames of a run by their shot names and the trace.
type pick struct {
	Run    string `yaml:"run"` // in frames, take them from this run instead of the image's
	Shots  string `yaml:"shots"`
	Every  int    `yaml:"every"`
	Near   string `yaml:"near"`
	From   *int   `yaml:"from"`
	To     *int   `yaml:"to"`
	At     *int   `yaml:"at"`
	Before string `yaml:"before"`
	Middle bool   `yaml:"middle"`
}

type jpegEncoding struct {
	Width   int `yaml:"width"`
	Height  int `yaml:"height"`
	Quality int `yaml:"quality"`
}

type gifEncoding struct {
	FPS     int    `yaml:"fps"`
	Width   int    `yaml:"width"`
	Height  int    `yaml:"height"`
	Colors  int    `yaml:"colors"`
	SplitAt int    `yaml:"split_at"` // keep two visually distinct phases in order
	Crop    string `yaml:"crop"`
}

type image struct {
	Run       string        `yaml:"run"`
	Shot      string        `yaml:"shot"`
	Pick      *pick         `yaml:"pick"`
	Frames    []pick        `yaml:"frames"`
	JPEG      *jpegEncoding `yaml:"jpeg"`
	GIF       *gifEncoding  `yaml:"gif"`
	Tolerance float64       `yaml:"tolerance"`
}

// tolerance is the share of pixels that may differ before an image is stale.
func (im image) tolerance() float64 {
	if im.Tolerance > 0 {
		return im.Tolerance
	}
	return 0.02
}

func loadManifest(t testing.TB) manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := yaml.UnmarshalWithOptions(b, &m, yaml.Strict()); err != nil {
		t.Fatalf("%s: %v", manifestPath, err)
	}
	return m
}

// validate reports what is wrong in the manifest itself.
func (m manifest) validate(readFile func(string) ([]byte, error)) []string {
	var bad []string
	for name, s := range m.Samples {
		if s.Package == "" {
			bad = append(bad, fmt.Sprintf("sample %s: no package", name))
		} else if _, err := readFile(path.Join(s.Package, "main.go")); err != nil {
			bad = append(bad, fmt.Sprintf("sample %s: %s has no main.go", name, s.Package))
		}
		if s.Game != "" {
			if _, err := readFile(path.Join(s.Game, "game.yaml")); err != nil {
				bad = append(bad, fmt.Sprintf("sample %s: %s has no game.yaml", name, s.Game))
			}
		}
	}
	scripts := map[string]string{}
	for name, r := range m.Runs {
		if _, ok := m.Samples[r.Sample]; !ok {
			bad = append(bad, fmt.Sprintf("run %s: unknown sample %q", name, r.Sample))
		}
		if r.Script != "" {
			b, err := readFile(r.Script)
			if err != nil {
				bad = append(bad, fmt.Sprintf("run %s: script %s: %v", name, r.Script, err))
			}
			scripts[name] = string(b)
		}
		if !slices.ContainsFunc(r.Args, func(a string) bool { return strings.Contains(a, "{out}") }) {
			bad = append(bad, fmt.Sprintf("run %s: no argument writes to {out}", name))
		}
		if e := r.Edit; e != nil {
			args := r.Args
			if m.Samples[r.Sample].Game != "" {
				args = append(slices.Clone(args), "{game}")
			}
			bad = append(bad, e.validate(name, scripts[name], args, readFile)...)
		}
	}
	for file, im := range m.Images {
		where := "image " + file
		if file != path.Base(file) || strings.HasPrefix(file, ".") {
			bad = append(bad, where+": name a file directly in docs/images")
		}
		r, ok := m.Runs[im.Run]
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: unknown run %q", where, im.Run))
		}
		sources := 0
		for _, set := range []bool{im.Shot != "", im.Pick != nil, len(im.Frames) > 0} {
			if set {
				sources++
			}
		}
		if sources != 1 {
			bad = append(bad, where+": give exactly one of shot, pick and frames")
		}
		switch ext := path.Ext(file); {
		case ext == ".jpg" && im.JPEG != nil && im.GIF == nil && len(im.Frames) == 0:
		case ext == ".gif" && im.GIF != nil && im.JPEG == nil && len(im.Frames) > 0:
		default:
			bad = append(bad, where+": a .jpg is one frame with jpeg, a .gif is frames with gif")
		}
		if im.Shot != "" && ok && r.Script != "" && !regexp.MustCompile(`(?m)^shot `+regexp.QuoteMeta(im.Shot)+`\s*$`).MatchString(scripts[im.Run]) {
			bad = append(bad, fmt.Sprintf("%s: %s has no `shot %s`", where, r.Script, im.Shot))
		}
		for _, p := range append(slices.Clone(im.Frames), derefPick(im.Pick)...) {
			if _, err := path.Match(p.Shots, "x"); err != nil || p.Shots == "" {
				bad = append(bad, fmt.Sprintf("%s: bad shots pattern %q", where, p.Shots))
			}
			if _, ok := m.Runs[p.Run]; p.Run != "" && !ok {
				bad = append(bad, fmt.Sprintf("%s: frames from an unknown run %q", where, p.Run))
			}
		}
	}
	for file, s := range m.Static {
		where := "static " + file
		if file != path.Base(file) || strings.HasPrefix(file, ".") {
			bad = append(bad, where+": name a file directly in docs/images")
		}
		if _, ok := m.Images[file]; ok {
			bad = append(bad, where+": also listed under images")
		}
		if s.Source == "" || s.License == "" {
			bad = append(bad, where+": give its source and license")
		}
	}
	sort.Strings(bad)
	return bad
}

// validate reports what is wrong in a run's edit: the copy, the shot it waits for and the
// text it changes must exist, and the run must read the copy.
func (e edit) validate(run, script string, args []string, readFile func(string) ([]byte, error)) []string {
	where := "run " + run + ": edit"
	var bad []string
	if _, err := readFile(path.Join(e.Copy, "game.yaml")); e.Copy == "" || err != nil {
		bad = append(bad, fmt.Sprintf("%s: copy %q is no game directory", where, e.Copy))
	}
	if !slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "{game}") }) {
		bad = append(bad, where+": no argument reads the copy ({game})")
	}
	if e.After == "" || !regexp.MustCompile(`(?m)^shot `+regexp.QuoteMeta(e.After)+`\s*$`).MatchString(script) {
		bad = append(bad, fmt.Sprintf("%s: the script has no `shot %s`", where, e.After))
	}
	b, err := readFile(path.Join(e.Copy, e.File))
	switch {
	case err != nil:
		bad = append(bad, fmt.Sprintf("%s: file %s: %v", where, e.File, err))
	case e.Old == "" || strings.Count(string(b), e.Old) != 1:
		bad = append(bad, fmt.Sprintf("%s: %s must hold old exactly once", where, e.File))
	case e.New == e.Old:
		bad = append(bad, where+": new is old")
	}
	return bad
}

func derefPick(p *pick) []pick {
	if p == nil {
		return nil
	}
	return []pick{*p}
}

// imageExt are the picture and video files the documentation may show.
var imageExt = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|svg|avif|mp4|webm)$`)

// reference is one picture a document shows.
type reference struct {
	doc    string // repository-relative document
	target string // as written
	file   string // repository-relative file it resolves to ("" if external)
}

var (
	mdImage     = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)`)
	htmlImage   = regexp.MustCompile(`(?i)<(?:img|source|video)\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	layoutImage = regexp.MustCompile(`(?:"|/|imgsrc/)(?:images/)?([A-Za-z0-9_.-]+\.(?:png|jpe?g|gif|webp|svg|mp4))`)
)

// siteDocument reports whether doc is published: README.md or a website source.
func siteDocument(doc string) bool {
	return doc == "README.md" || strings.HasPrefix(doc, "website/")
}

// collectReferences reads every Markdown document and website template of the
// repository and returns the local pictures they show.
func collectReferences(fsys fs.FS) ([]reference, error) {
	var refs []reference
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch p {
			case ".git", "build", "bin", "dist", "website/public", "website/resources", ".cache":
				return fs.SkipDir
			}
			if d.Name() == "testdata" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		isLayout := strings.HasPrefix(p, "website/layouts/") || strings.HasPrefix(p, "website/content/") && strings.HasSuffix(p, ".gotmpl")
		if !strings.HasSuffix(p, ".md") && !isLayout {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var targets []string
		if isLayout {
			for _, m := range layoutImage.FindAllSubmatch(b, -1) {
				targets = append(targets, "/images/"+string(m[1]))
			}
		} else {
			for _, m := range mdImage.FindAllSubmatch(b, -1) {
				targets = append(targets, string(m[1]))
			}
		}
		for _, m := range htmlImage.FindAllSubmatch(b, -1) {
			targets = append(targets, string(m[1]))
		}
		for _, target := range targets {
			refs = append(refs, reference{doc: p, target: target, file: resolve(p, target)})
		}
		return nil
	})
	return refs, err
}

// resolve turns a picture reference into a repository path, or "" for an external one.
func resolve(doc, target string) string {
	if i := strings.IndexAny(target, "?#"); i >= 0 {
		target = target[:i]
	}
	switch {
	case strings.Contains(target, "://") || strings.HasPrefix(target, "data:") || strings.HasPrefix(target, "//") || strings.Contains(target, "{{"):
		return ""
	case strings.HasPrefix(doc, "website/") && strings.HasPrefix(target, "/images/"):
		// The site mounts docs/images at /images (website/hugo.toml).
		return path.Join("docs/images", strings.TrimPrefix(target, "/images/"))
	case strings.HasPrefix(target, "/"):
		return path.Clean(strings.TrimPrefix(target, "/"))
	default:
		return path.Join(path.Dir(doc), target)
	}
}

// checkReferences compares the manifest with the files in docs/images and the references.
func checkReferences(fsys fs.FS, m manifest, refs []reference) []string {
	var bad []string
	referenced := map[string]bool{}
	for _, r := range refs {
		if r.file == "" {
			continue
		}
		inImages := path.Dir(r.file) == "docs/images"
		if siteDocument(r.doc) && !inImages {
			bad = append(bad, fmt.Sprintf("%s: %s is not in docs/images; README.md and the website show only the pictures of %s", r.doc, r.target, manifestPath))
			continue
		}
		if !inImages {
			continue
		}
		name := path.Base(r.file)
		referenced[name] = true
		if !m.listed(name) {
			bad = append(bad, fmt.Sprintf("%s: %s is not in %s; add how it is made and run make docs-images", r.doc, r.target, manifestPath))
		}
		if _, err := fs.Stat(fsys, r.file); err != nil {
			bad = append(bad, fmt.Sprintf("%s: %s does not exist", r.doc, r.target))
		}
	}
	var names []string
	for name := range m.Images {
		names = append(names, name)
	}
	for name := range m.Static {
		names = append(names, name)
	}
	for _, name := range names {
		if !referenced[name] {
			bad = append(bad, fmt.Sprintf("%s: %s is listed but no document shows it; remove it from the manifest and docs/images", manifestPath, name))
		}
		if _, err := fs.Stat(fsys, "docs/images/"+name); err != nil {
			bad = append(bad, fmt.Sprintf("%s: docs/images/%s is missing; run make docs-images", manifestPath, name))
		}
	}
	entries, err := fs.ReadDir(fsys, "docs/images")
	if err != nil {
		bad = append(bad, err.Error())
	}
	for _, e := range entries {
		if e.Name() == path.Base(manifestPath) {
			continue
		}
		if !m.listed(e.Name()) {
			bad = append(bad, fmt.Sprintf("docs/images/%s is not in %s: a picture made by hand cannot be committed here", e.Name(), manifestPath))
		}
	}
	// The website takes its pictures from docs/images only.
	err = fs.WalkDir(fsys, "website", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (p == "website/public" || p == "website/resources") {
			return fs.SkipDir
		}
		if !d.IsDir() && imageExt.MatchString(p) {
			bad = append(bad, fmt.Sprintf("%s: put the website's pictures in docs/images and list them in %s", p, manifestPath))
		}
		return nil
	})
	if err != nil {
		bad = append(bad, err.Error())
	}
	sort.Strings(bad)
	return slices.Compact(bad)
}

// TestImageManifest checks, without a display, that the manifest is complete:
// every picture of README.md, the website and docs/ is listed, every listed
// picture is shown and exists, and no other picture is in docs/images.
func TestImageManifest(t *testing.T) {
	m := loadManifest(t)
	fsys := os.DirFS(root)
	read := func(p string) ([]byte, error) { return fs.ReadFile(fsys, path.Clean(strings.TrimPrefix(p, "./"))) }
	bad := m.validate(read)
	refs, err := collectReferences(fsys)
	if err != nil {
		t.Fatal(err)
	}
	bad = append(bad, checkReferences(fsys, m, refs)...)
	if len(bad) > 0 {
		t.Fatalf("the documentation pictures do not match %s:\n%s", manifestPath, strings.Join(bad, "\n"))
	}
}

// The reference check fails for each way the documentation and the manifest can drift.
func TestCheckReferencesFindsDrift(t *testing.T) {
	m := manifest{Images: map[string]image{"a.jpg": {}, "listed-only.jpg": {}}}
	fsys := mapFS{
		"README.md":                         "![a](docs/images/a.jpg) ![b](docs/images/hand.jpg) ![c](e2e/x.png) ![d](https://example.com/x.png)",
		"website/content/p.md":              `<img src="/images/gone.jpg">`,
		"docs/images/a.jpg":                 "x",
		"docs/images/hand.jpg":              "x",
		"docs/images/manifest.yaml":         "",
		"website/assets/logo.png":           "x",
		"website/public/images/a.jpg":       "x",
		"docs/architecture/x.md":            "![a](../images/a.jpg)",
		"website/layouts/baseof.html":       `{{ absURL "images/a.jpg" }}`,
		"internal/x/testdata/notes/note.md": "![a](missing.png)",
	}.fs()
	refs, err := collectReferences(fsys)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(checkReferences(fsys, m, refs), "\n")
	for _, want := range []string{
		"README.md: docs/images/hand.jpg is not in docs/images/manifest.yaml",
		"README.md: e2e/x.png is not in docs/images",
		"website/content/p.md: /images/gone.jpg does not exist",
		"listed-only.jpg is listed but no document shows it",
		"docs/images/listed-only.jpg is missing",
		"docs/images/hand.jpg is not in docs/images/manifest.yaml: a picture made by hand",
		"website/assets/logo.png: put the website's pictures in docs/images",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "example.com") || strings.Contains(got, "website/public") || strings.Contains(got, "testdata") {
		t.Errorf("external, built or test files were checked:\n%s", got)
	}
}

func TestManifestValidate(t *testing.T) {
	files := map[string]string{"examples/s/main.go": "package main", "s.txt": "wait 1\nshot one\n"}
	read := func(p string) ([]byte, error) {
		if b, ok := files[p]; ok {
			return []byte(b), nil
		}
		return nil, errors.New("no file")
	}
	m := manifest{
		Samples: map[string]sample{"s": {Package: "./examples/s"}, "gone": {Package: "./examples/gone", Game: "examples/missing/game"}},
		Runs:    map[string]run{"r": {Sample: "s", Script: "s.txt", Args: []string{"--shots", "{out}"}}, "q": {Sample: "nope"}},
		Images: map[string]image{
			"one.jpg":   {Run: "r", Shot: "one", JPEG: &jpegEncoding{}},
			"two.jpg":   {Run: "r", Shot: "two", JPEG: &jpegEncoding{}},
			"three.gif": {Run: "r", Shot: "one", JPEG: &jpegEncoding{}},
			"four.gif":  {Run: "x", Frames: []pick{{Shots: "["}}, GIF: &gifEncoding{}},
			"five.gif":  {Run: "r", Frames: []pick{{Shots: "a*"}, {Run: "y", Shots: "b*"}}, GIF: &gifEncoding{}},
		},
	}
	got := strings.Join(m.validate(read), "\n")
	for _, want := range []string{
		"sample gone: ./examples/gone has no main.go",
		"sample gone: examples/missing/game has no game.yaml",
		"run q: unknown sample",
		"run q: no argument writes to {out}",
		"image two.jpg: s.txt has no `shot two`",
		"image three.gif: a .jpg is one frame",
		"image four.gif: unknown run",
		`image four.gif: bad shots pattern "["`,
		`image five.gif: frames from an unknown run "y"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "one.jpg") {
		t.Errorf("a good image was reported:\n%s", got)
	}
}

// mapFS builds a file system from file contents.
type mapFS map[string]string

func (m mapFS) fs() fs.FS {
	out := fstest.MapFS{}
	for name, body := range m {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}
