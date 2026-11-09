package assetfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// TestOverlay: pictures above are read from above; missing pictures and anything outside assets are read from the original (layered reading).
func TestOverlay(t *testing.T) {
	base := fstest.MapFS{
		"game.yaml":        {Data: []byte("base yaml")},
		"assets/cg/a.jpg":  {Data: []byte("raw a")},
		"assets/cg/b.jpg":  {Data: []byte("raw b")},
		"assets/bg/c.png":  {Data: []byte("raw c")},
		"assets/report.js": {Data: []byte("raw r")},
	}
	top := fstest.MapFS{
		"cg/a.jpg":    {Data: []byte("mosaic a")},
		"game.yaml":   {Data: []byte("must not be used")},
		"report.json": {Data: []byte("{}")},
		"bg/c.png/x":  {Data: []byte("a dir named like the picture")},
	}
	o := overlay(t, base, "assets", top)
	for name, want := range map[string]string{
		"assets/cg/a.jpg": "mosaic a",
		"assets/cg/b.jpg": "raw b",
		"assets/bg/c.png": "raw c",
		"game.yaml":       "base yaml",
	} {
		b, err := fs.ReadFile(o, name)
		if err != nil || string(b) != want {
			t.Errorf("ReadFile(%s) = %q, %v; want %q", name, b, err, want)
		}
		f, err := o.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if st.Size() != int64(len(want)) {
			t.Errorf("Open(%s).Stat().Size() = %d, want %d", name, st.Size(), len(want))
		}
	}
	if _, err := fs.ReadFile(o, "assets/report.json"); err == nil {
		t.Error("a file only in the top is read")
	}
	// It works through fs.Sub too (the engine wraps after fs.Sub for both --dir and embedded).
	sub, err := fs.Sub(fstest.MapFS{"g/assets/cg/a.jpg": {Data: []byte("raw a")}}, "g")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(overlay(t, sub, "assets", top), "assets/cg/a.jpg"); err != nil || string(b) != "mosaic a" {
		t.Errorf("through fs.Sub: %q", b)
	}
	if err := fstest.TestFS(o, "game.yaml", "assets/cg/a.jpg", "assets/cg/b.jpg"); err != nil {
		t.Error(err)
	}
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, s := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestBuildLeavesNoRawPicture: in the distributed tree, every picture that has a masked version becomes the masked picture,
// and the original unmodified picture is not left. The source directory is unchanged, and mosaic working files (inspection, diagrams) are not included.
func TestBuildLeavesNoRawPicture(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "game")
	top := filepath.Join(src, "assets_mosaic") // inside the source directory (excluded when copying)
	write(t, src, map[string]string{
		"game.yaml":        "yaml",
		"assets/cg/x.jpg":  "raw x",
		"assets/cg/y.jpg":  "raw y",
		"assets/bg/z.png":  "raw z",
		"assets/cg/in.jpg": "drawn with a mosaic",
	})
	write(t, top, map[string]string{
		"cg/x.jpg":            "mosaic x",
		"report.json":         "{}",
		"review/sheet_01.jpg": "sheet",
		"manual/cg/x.jpg.png": "fig",
	})
	out := filepath.Join(dir, "release", "game")
	write(t, out, map[string]string{"stale.txt": "old"})
	ins := []Approval{
		{File: "cg/x.jpg", Required: true, Approved: true},
		{File: "cg/y.jpg", Approved: true},
		{File: "cg/in.jpg", Required: true, Approved: true},
	}
	ins[2].Required = false
	for i := range ins {
		ins[i].Source = sum(read(t, filepath.Join(src, "assets", ins[i].File)))
	}
	res, err := Tree{Src: src, Top: top, Prefix: "assets", Dst: out}.Build(ins)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(out, "assets", "cg", "x.jpg")); got != "mosaic x" {
		t.Errorf("x = %q", got)
	}
	if got := read(t, filepath.Join(out, "assets", "cg", "y.jpg")); got != "raw y" {
		t.Errorf("y = %q", got)
	}
	if read(t, filepath.Join(src, "assets", "cg", "x.jpg")) != "raw x" {
		t.Error("the original was changed")
	}
	if strings.Join(res.Replaced, ",") != "cg/x.jpg" || res.Files != 5 {
		t.Errorf("result = %+v", res)
	}
	for _, p := range []string{"assets_mosaic", "assets/report.json", "assets/review", "stale.txt"} {
		if _, err := os.Stat(filepath.Join(out, p)); err == nil {
			t.Errorf("%s is in the tree", p)
		}
	}
	// no picture with the same content as any original is left (for pictures that need masking)
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && read(t, p) == "raw x" {
			t.Errorf("the raw picture is left at %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// a picture that needs masking but has no output stays as the original, so it is an error
	ins = append(ins, Approval{File: "bg/z.png", Required: true, Approved: true, Source: sum("raw z")})
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out}).Build(ins); err == nil || !strings.Contains(err.Error(), "bg/z.png") {
		t.Errorf("a raw picture that needs a mosaic passed: %v", err)
	}
	// if the mosaic directory has a picture not in the current output (left over from a previous run), it is an error
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out, Outputs: []string{}}).Build(ins[:3]); err == nil || !strings.Contains(err.Error(), "cg/x.jpg") {
		t.Errorf("a stale output passed: %v", err)
	}
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out, Outputs: []string{"cg/x.jpg"}}).Build(ins[:3]); err != nil {
		t.Error(err)
	}
	// do not create it if the destination is inside the source
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(src, "out")}).Build(nil); err == nil {
		t.Error("wrote into the source")
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TestOverlayManifest: if the store has a manifest, layer only those whose original picture's sha256 matches the record.
// Mismatches (the original re-imported under the same name), entries without a sha256 and entries not in the record show the original picture.
// Without a manifest, everything is layered as before.
func TestOverlayManifest(t *testing.T) {
	base := fstest.MapFS{
		"assets/cg/same.jpg":   {Data: []byte("raw same")},
		"assets/cg/new.jpg":    {Data: []byte("raw new picture")},
		"assets/cg/nosum.jpg":  {Data: []byte("raw nosum")},
		"assets/cg/absent.jpg": {Data: []byte("raw absent")},
	}
	report := `[
  {"file": "cg/same.jpg", "size": [1, 1], "source": "` + sum("raw same") + `"},
  {"file": "cg/new.jpg", "source": "` + sum("raw old picture") + `"},
  {"file": "cg/nosum.jpg"}
]`
	top := fstest.MapFS{
		"cg/same.jpg":   {Data: []byte("mosaic same")},
		"cg/new.jpg":    {Data: []byte("mosaic old picture")},
		"cg/nosum.jpg":  {Data: []byte("mosaic nosum")},
		"cg/absent.jpg": {Data: []byte("mosaic absent")},
		"report.json":   {Data: []byte(report)},
	}
	want := map[string]string{
		"assets/cg/same.jpg":   "mosaic same",
		"assets/cg/new.jpg":    "raw new picture",
		"assets/cg/nosum.jpg":  "raw nosum",
		"assets/cg/absent.jpg": "raw absent",
	}
	o := overlay(t, base, "assets", top).useManifest(defaultManifest)
	for name, w := range want {
		b, err := fs.ReadFile(o, name)
		if err != nil || string(b) != w {
			t.Errorf("ReadFile(%s) = %q, %v; want %q", name, b, err, w)
		}
		f, err := o.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if st.Size() != int64(len(w)) {
			t.Errorf("Open(%s).Stat size %d; want %d", name, st.Size(), len(w))
		}
	}
	stale, err := o.Stale()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stale, ","); got != "cg/absent.jpg,cg/new.jpg,cg/nosum.jpg" {
		t.Errorf("Stale = %s", got)
	}

	// if it goes back to the one from when the original picture was applied (after the memo is discarded), it is layered again
	base["assets/cg/new.jpg"] = &fstest.MapFile{Data: []byte("raw old picture")}
	o.Forget()
	if b, err := fs.ReadFile(o, "assets/cg/new.jpg"); err != nil || string(b) != "mosaic old picture" {
		t.Errorf("after the original came back: %q", b)
	}
	stale, err = o.Stale()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(stale); n != 2 {
		t.Errorf("更新後の古い差し替え = %d、期待値 2", n)
	}

	// a store without a manifest: layer everything
	delete(top, "report.json")
	p := overlay(t, base, "assets", top).useManifest(defaultManifest)
	for _, name := range []string{"assets/cg/nosum.jpg", "assets/cg/absent.jpg"} {
		if b, err := fs.ReadFile(p, name); err != nil || !strings.HasPrefix(string(b), "mosaic") {
			t.Errorf("without a manifest, ReadFile(%s) = %q", name, b)
		}
	}
	if s, err := p.Stale(); err != nil || s != nil {
		t.Errorf("without a manifest, Stale = %v", s)
	}
}

// deniedFS fails to open one name with a permission error.
type deniedFS struct {
	fs.FS
	name string
}

func (d deniedFS) Open(name string) (fs.File, error) {
	if name == d.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return d.FS.Open(name)
}

// Stale reports an unreadable original instead of treating it as absent.
func TestStaleReportsUnreadableOriginals(t *testing.T) {
	base := fstest.MapFS{"assets/cg/a.jpg": {Data: []byte("raw a")}}
	top := fstest.MapFS{
		"cg/a.jpg":    {Data: []byte("mosaic a")},
		"report.json": {Data: []byte(`[{"file": "cg/a.jpg", "source": "` + sum("raw old") + `"}]`)},
	}
	o := overlay(t, deniedFS{FS: base, name: "assets/cg/a.jpg"}, "assets", top).useManifest(defaultManifest)
	if s, err := o.Stale(); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Stale = %v, %v; want a permission error", s, err)
	}
	// A missing original is still not counted.
	o = overlay(t, fstest.MapFS{}, "assets", top).useManifest(defaultManifest)
	if s, err := o.Stale(); err != nil || len(s) != 0 {
		t.Fatalf("Stale without the original = %v, %v", s, err)
	}
}

func TestNewValidatesItsArguments(t *testing.T) {
	ok := fstest.MapFS{}
	cases := []struct {
		name      string
		base, top fs.FS
		cfg       Config
	}{
		{"nil base", nil, ok, Config{}},
		{"nil top", ok, nil, Config{}},
		{"prefix escapes", ok, ok, Config{Prefix: "../x"}},
		{"absolute prefix", ok, ok, Config{Prefix: "/x"}},
		{"manifest path escapes", ok, ok, Config{Manifest: Manifest{Name: "../m.json", File: "f", Sum: "s"}}},
		{"manifest without file field", ok, ok, Config{Manifest: Manifest{Name: "m.json", Sum: "s"}}},
		{"manifest without sum field", ok, ok, Config{Manifest: Manifest{Name: "m.json", File: "f"}}},
	}
	for _, c := range cases {
		if _, err := New(c.base, c.top, c.cfg); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	o, err := New(ok, ok, Config{})
	if err != nil || o.prefix != "." {
		t.Errorf("an empty prefix is the root: %v %v", o, err)
	}
}

// TestRootOverlayAndCoverage: with the root as the prefix, a file replaces the original of the same path;
// Coverage reports Has/Used/Checked per file, and says nothing for paths outside the overlay.
func TestRootOverlayAndCoverage(t *testing.T) {
	base := fstest.MapFS{"a.png": {Data: []byte("orig-a")}, "sub/b.png": {Data: []byte("orig-b")}, "c.png": {Data: []byte("orig-c")}}
	top := fstest.MapFS{
		"a.png":       {Data: []byte("new-a")},
		"sub/b.png":   {Data: []byte("new-b")},
		"d.png":       {Data: []byte("only-top")},
		"report.json": {Data: []byte(`[{"file":"a.png","source":"` + sum("orig-a") + `"},{"file":"sub/b.png","source":"` + sum("changed") + `"},{"file":"c.png","source":"` + sum("orig-c") + `"},{"file":"","source":"x"},{"file":"n.png"}]`)},
	}
	o, err := New(base, top, Config{Manifest: defaultManifest})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		want Coverage
	}{
		{"a.png", Coverage{Has: true, Used: true, Checked: true}},
		{"sub/b.png", Coverage{Has: true}},
		{"c.png", Coverage{Checked: true}},
		{"d.png", Coverage{}},
		{"missing.png", Coverage{}},
		{".", Coverage{}},
	}
	for _, c := range cases {
		if got := o.Coverage(c.name); got != c.want {
			t.Errorf("Coverage(%q) = %+v, want %+v", c.name, got, c.want)
		}
	}
	if b, err := o.ReadFile("a.png"); err != nil || string(b) != "new-a" {
		t.Errorf("a fresh replacement is served: %q %v", b, err)
	}
	if b, err := o.ReadFile("sub/b.png"); err != nil || string(b) != "orig-b" {
		t.Errorf("a replacement made from another original is not served: %q %v", b, err)
	}
	if _, err := o.ReadFile("d.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a file only in top is never added: %v", err)
	}
	stale, err := o.Stale()
	if err != nil || !slices.Equal(stale, []string{"sub/b.png"}) {
		t.Errorf("stale %v %v", stale, err)
	}
	// Forget re-reads the manifest after the overlay is rebuilt
	top["report.json"] = &fstest.MapFile{Data: []byte(`[{"file":"sub/b.png","source":"` + sum("orig-b") + `"}]`)}
	if got := o.Coverage("sub/b.png"); got.Used {
		t.Error("the manifest is memoized until Forget")
	}
	o.Forget()
	if got := o.Coverage("sub/b.png"); !got.Used || !got.Checked {
		t.Errorf("after Forget the new manifest holds: %+v", got)
	}
	if got := o.Coverage("a.png"); got.Checked || got.Used {
		t.Errorf("a.png is no longer in the manifest: %+v", got)
	}
}

func TestOverlayRejectsBadNamesAndListsReplacements(t *testing.T) {
	base := fstest.MapFS{"assets/a.png": {Data: []byte("orig")}, "assets/dir/b.png": {Data: []byte("orig-b")}}
	top := fstest.MapFS{"a.png": {Data: []byte("replaced, longer")}}
	o := overlay(t, base, "assets", top)
	for _, name := range []string{"", "/assets/a.png", "../assets/a.png", "assets//a.png"} {
		if _, err := o.Open(name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Open(%q): %v", name, err)
		}
		if _, err := o.ReadFile(name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("ReadFile(%q): %v", name, err)
		}
	}
	es, err := fs.ReadDir(o, "assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Name() == "a.png" {
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != int64(len("replaced, longer")) {
				t.Errorf("a listed replacement carries the replacement's size, got %d", info.Size())
			}
		}
	}
	f, err := o.Open("assets")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // A test file.
	d, ok := f.(fs.ReadDirFile)
	if !ok {
		t.Fatal("a directory opened through the overlay lists itself")
	}
	if list, err := d.ReadDir(-1); err != nil || len(list) != 2 {
		t.Errorf("%v %v", list, err)
	}
	if info, err := o.Stat("assets/a.png"); err != nil || info.Size() != int64(len("replaced, longer")) {
		t.Errorf("Stat follows the replacement: %v %v", info, err)
	}
	if _, err := o.Stat("assets/none.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat of nothing: %v", err)
	}
	if _, err := o.Open("assets/none.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open of nothing: %v", err)
	}
	// a directory in top does not replace a file of the original
	topDir := fstest.MapFS{"a.png/inner": {Data: []byte("x")}}
	if b, err := overlay(t, base, "assets", topDir).ReadFile("assets/a.png"); err != nil || string(b) != "orig" {
		t.Errorf("a directory in top must not replace a file: %q %v", b, err)
	}
	// a file of top for a directory of the original is not served
	topFile := fstest.MapFS{"dir": {Data: []byte("x")}}
	if _, err := overlay(t, base, "assets", topFile).ReadFile("assets/dir/b.png"); err != nil {
		t.Errorf("reading below a directory: %v", err)
	}
}

func TestManifestWithBrokenContentCountsAsAbsent(t *testing.T) {
	base := fstest.MapFS{"a.png": {Data: []byte("orig")}}
	for name, manifest := range map[string]string{"not json": "{", "wrong shape": `{"file":"a.png"}`} {
		top := fstest.MapFS{"a.png": {Data: []byte("new")}, "report.json": {Data: []byte(manifest)}}
		o, err := New(base, top, Config{Manifest: defaultManifest})
		if err != nil {
			t.Fatal(err)
		}
		if b, err := o.ReadFile("a.png"); err != nil || string(b) != "new" {
			t.Errorf("%s: an unreadable manifest means everything is overlaid, got %q", name, b)
		}
		if s, err := o.Stale(); s != nil || err != nil {
			t.Errorf("%s: %v %v", name, s, err)
		}
	}
	// a manifest entry without a sum never matches
	top := fstest.MapFS{"a.png": {Data: []byte("new")}, "report.json": {Data: []byte(`[{"file":"a.png"}]`)}}
	o, err := New(base, top, Config{Manifest: defaultManifest})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := o.ReadFile("a.png"); err != nil || string(b) != "orig" {
		t.Errorf("an entry without a sum does not vouch for the replacement, got %q", b)
	}
}

func TestStaleSkipsDirectoriesAndFilesNotInTheOriginal(t *testing.T) {
	base := fstest.MapFS{"assets/a.png": {Data: []byte("orig")}, "assets/dir/x": {Data: []byte("x")}}
	top := fstest.MapFS{
		"a.png":       {Data: []byte("new")},
		"dir":         {Mode: fs.ModeDir},
		"extra.png":   {Data: []byte("not in the original")},
		"report.json": {Data: []byte(`[{"file":"a.png","source":"wrong"}]`)},
	}
	o, err := New(base, top, Config{Prefix: "assets", Manifest: defaultManifest})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := o.Stale()
	if err != nil || !slices.Equal(stale, []string{"a.png"}) {
		t.Errorf("stale %v %v", stale, err)
	}
	// a path of top that is a directory in the original
	top2 := fstest.MapFS{"dir": {Data: []byte("a file where the original has a directory")}, "report.json": {Data: []byte(`[]`)}}
	o2, err := New(base, top2, Config{Prefix: "assets", Manifest: defaultManifest})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := o2.Stale(); err != nil || len(s) != 0 {
		t.Errorf("a file over a directory is not an image: %v %v", s, err)
	}
}

func TestGateChecks(t *testing.T) {
	src, top := t.TempDir(), t.TempDir()
	write(t, src, map[string]string{"assets/a.png": "orig", "assets/b.png": "orig-b"})
	g := Gate{Base: os.DirFS(src), Top: os.DirFS(top), Prefix: "assets", Records: []Approval{
		{File: "a.png", Source: sum("orig"), Approved: true},
		{File: "b.png", Source: sum("orig-b"), Approved: false, Status: "pending"},
	}}
	if err := g.Check("a.png"); err != nil {
		t.Errorf("an approved original: %v", err)
	}
	if err := g.Check("assets/a.png"); err != nil {
		t.Errorf("the prefix is accepted: %v", err)
	}
	if err := g.Check("b.png"); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Errorf("an unapproved record says its status: %v", err)
	}
	if err := g.Check("../a.png"); err == nil {
		t.Error("a path that leaves the tree")
	}
	if err := g.Check("c.png"); err != nil {
		t.Errorf("an unknown file with nothing to replace it is not watched: %v", err)
	}
	g.Watch = []string{"c"}
	if err := g.Check("c.png"); err == nil || !strings.Contains(err.Error(), "no approval") {
		t.Errorf("a watched file with no record: %v", err)
	}
	write(t, top, map[string]string{"c.png": "replacement"})
	g.Watch = nil
	if err := g.Check("c.png"); err == nil {
		t.Error("a replacement with no record is not accepted")
	}
	// the source file of a record is gone
	g.Records = append(g.Records, Approval{File: "gone.png", Source: "x", Approved: true})
	if err := g.Check("gone.png"); err == nil {
		t.Error("a record whose source is missing")
	}
	// the replacement cannot be read for a reason other than being absent
	g2 := Gate{Base: os.DirFS(src), Top: deniedFS{FS: fstest.MapFS{}, name: "a.png"}, Prefix: "assets", Records: g.Records}
	if err := g2.Check("a.png"); err == nil {
		t.Error("an unreadable replacement")
	}
}

func TestBuildRefusesOverlappingAndStaleOutputs(t *testing.T) {
	src, top := t.TempDir(), t.TempDir()
	write(t, src, map[string]string{"assets/a.png": "orig", "assets/b.png": "orig-b", "game.yaml": "g", "skip/me.txt": "x"})
	write(t, top, map[string]string{"a.png": "new-a", "b.png": "left over"})
	dst := filepath.Join(t.TempDir(), "out")

	for name, tree := range map[string]Tree{
		"dst inside src": {Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(src, "out")},
		"dst is src":     {Src: src, Top: top, Prefix: "assets", Dst: src},
		"dst inside top": {Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(top, "out")},
		"src inside dst": {Src: filepath.Join(dst, "src"), Top: top, Prefix: "assets", Dst: dst},
	} {
		if _, err := tree.Build(nil); err == nil || !strings.Contains(err.Error(), "must be apart") {
			t.Errorf("%s: %v", name, err)
		}
	}
	res, err := Tree{Src: src, Top: top, Prefix: "assets", Dst: dst, Outputs: []string{"a.png"}, Skip: []string{"skip"}}.Build(nil)
	if err == nil || !strings.Contains(err.Error(), "b.png") || !strings.Contains(err.Error(), "older run") {
		t.Fatalf("a leftover picture must be reported: %v", err)
	}
	if !slices.Equal(res.Replaced, []string{"a.png"}) {
		t.Errorf("replaced %v", res.Replaced)
	}
	if _, err := os.Stat(filepath.Join(dst, "skip", "me.txt")); !os.IsNotExist(err) {
		t.Error("a skipped folder was copied")
	}
	// without an Outputs list every picture of top is used
	res, err = Tree{Src: src, Top: top, Prefix: "assets", Dst: dst, Skip: []string{"skip"}}.Build(nil)
	if err != nil || !slices.Equal(res.Replaced, []string{"a.png", "b.png"}) {
		t.Fatalf("%v %v", res.Replaced, err)
	}
	if read(t, filepath.Join(dst, "assets", "b.png")) != "left over" {
		t.Error("the replacement was not copied")
	}
	if res.Files != 3 {
		t.Errorf("copied %d files, want 3 (two pictures and game.yaml)", res.Files)
	}
	// a top that lies inside src is not copied into the output
	inner := filepath.Join(src, "work")
	write(t, src, map[string]string{"work/a.png": "work file"})
	res, err = Tree{Src: src, Top: inner, Prefix: "assets", Dst: dst, Skip: []string{"skip"}}.Build(nil)
	if err != nil || !slices.Equal(res.Replaced, []string{"a.png"}) {
		t.Fatalf("%v %v", res.Replaced, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "work")); !os.IsNotExist(err) {
		t.Error("the replacement folder inside the source was copied")
	}
	// a record that fails the gate stops the build before anything is copied
	_, err = Tree{Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(t.TempDir(), "out2"), Outputs: []string{"a.png", "b.png"}}.Build([]Approval{{File: "a.png", Source: sum("orig"), Approved: true, Required: true}, {File: "b.png", Source: "wrong", Approved: true}})
	if err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Errorf("a changed source stops the build: %v", err)
	}
	// a file of top that is in src but has no record
	_, err = Tree{Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(t.TempDir(), "out3")}.Build([]Approval{{File: "a.png", Source: sum("orig"), Approved: true, Required: true}})
	if err == nil || !strings.Contains(err.Error(), "no approval for b.png") {
		t.Errorf("a replacement with no record stops the build: %v", err)
	}
}

func TestBuildWithoutATopCopiesTheSource(t *testing.T) {
	src := t.TempDir()
	write(t, src, map[string]string{"assets/a.png": "orig", "game.yaml": "g"})
	dst := filepath.Join(t.TempDir(), "out")
	res, err := Tree{Src: src, Prefix: "assets", Dst: dst}.Build([]Approval{{File: "a.png", Source: sum("orig"), Approved: true}})
	if err != nil || res.Files != 2 || len(res.Replaced) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if read(t, filepath.Join(dst, "assets", "a.png")) != "orig" {
		t.Error("the original was not copied")
	}
	// a required replacement with no top can never be met
	_, err = Tree{Src: src, Prefix: "assets", Dst: dst}.Build([]Approval{{File: "a.png", Source: sum("orig"), Approved: true, Required: true}})
	if err == nil || !strings.Contains(err.Error(), "required replacement") {
		t.Errorf("%v", err)
	}
	// the old destination is recreated, so a stale file in it disappears
	write(t, filepath.Dir(dst), map[string]string{"out/old.txt": "stale"})
	if _, err := (Tree{Src: src, Prefix: "assets", Dst: dst}).Build(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "old.txt")); !os.IsNotExist(err) {
		t.Error("the destination was not recreated")
	}
}

func TestWithinAndHelpers(t *testing.T) {
	sep := string(filepath.Separator)
	for _, c := range []struct {
		p, dir string
		want   bool
	}{
		{sep + "a" + sep + "b", sep + "a", true},
		{sep + "a", sep + "a", true},
		{sep + "ab", sep + "a", false},
		{sep + "a", sep + "a" + sep + "b", false},
		{sep + "x", sep + "a", false},
	} {
		if got := within(c.p, c.dir); got != c.want {
			t.Errorf("within(%q, %q) = %v", c.p, c.dir, got)
		}
	}
	if Field(map[string]any{"a": map[string]any{"b": "c"}}, "a.b") != "c" || Field(map[string]any{"a": 1}, "a.b") != nil || Field(nil, "") != nil {
		t.Error("field follows dotted keys only through objects")
	}
}
