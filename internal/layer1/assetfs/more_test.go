package assetfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

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
			info, _ := e.Info()
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
		if b, _ := o.ReadFile("a.png"); string(b) != "new" {
			t.Errorf("%s: an unreadable manifest means everything is overlaid, got %q", name, b)
		}
		if s, err := o.Stale(); s != nil || err != nil {
			t.Errorf("%s: %v %v", name, s, err)
		}
	}
	// a manifest entry without a sum never matches
	top := fstest.MapFS{"a.png": {Data: []byte("new")}, "report.json": {Data: []byte(`[{"file":"a.png"}]`)}}
	o, _ := New(base, top, Config{Manifest: defaultManifest})
	if b, _ := o.ReadFile("a.png"); string(b) != "orig" {
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
	o, _ := New(base, top, Config{Prefix: "assets", Manifest: defaultManifest})
	stale, err := o.Stale()
	if err != nil || !slices.Equal(stale, []string{"a.png"}) {
		t.Errorf("stale %v %v", stale, err)
	}
	// a path of top that is a directory in the original
	top2 := fstest.MapFS{"dir": {Data: []byte("a file where the original has a directory")}, "report.json": {Data: []byte(`[]`)}}
	o2, _ := New(base, top2, Config{Prefix: "assets", Manifest: defaultManifest})
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
	if field(map[string]any{"a": map[string]any{"b": "c"}}, "a.b") != "c" || field(map[string]any{"a": 1}, "a.b") != nil || field(nil, "") != nil {
		t.Error("field follows dotted keys only through objects")
	}
}
