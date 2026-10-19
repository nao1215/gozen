package assetfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// TestOverlay: 上にある絵は上から、無い絵・assets の外は元から読む（重ねる読み方）。
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
	// fs.Sub を通しても効く（エンジンは --dir でも埋め込みでも fs.Sub の後に包む）
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

// TestBuildLeavesNoRawPicture: 配る版の木では、モザイクの絵がある物は全部モザイクの絵になり、
// 元の無修正の絵は残らない。元のディレクトリは変わらず、モザイクの作業の物（調べ・図）は入らない。
func TestBuildLeavesNoRawPicture(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "game")
	top := filepath.Join(src, "assets_mosaic") // 元のディレクトリの中にある（外して写す）
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
	if got := read(t, filepath.Join(out, "assets/cg/x.jpg")); got != "mosaic x" {
		t.Errorf("x = %q", got)
	}
	if got := read(t, filepath.Join(out, "assets/cg/y.jpg")); got != "raw y" {
		t.Errorf("y = %q", got)
	}
	if read(t, filepath.Join(src, "assets/cg/x.jpg")) != "raw x" {
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
	// 元の絵のどれとも同じ中身の絵が残っていない（モザイクが要る絵について）
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && read(t, p) == "raw x" {
			t.Errorf("the raw picture is left at %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// モザイクが要るのに出力が無い絵は、元のまま残るので誤りにする
	ins = append(ins, Approval{File: "bg/z.png", Required: true, Approved: true, Source: sum("raw z")})
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out}).Build(ins); err == nil || !strings.Contains(err.Error(), "bg/z.png") {
		t.Errorf("a raw picture that needs a mosaic passed: %v", err)
	}
	// 今の出力に無い絵（前の回の残り）がモザイクのディレクトリにあれば誤りにする
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out, Outputs: []string{}}).Build(ins[:3]); err == nil || !strings.Contains(err.Error(), "cg/x.jpg") {
		t.Errorf("a stale output passed: %v", err)
	}
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: out, Outputs: []string{"cg/x.jpg"}}).Build(ins[:3]); err != nil {
		t.Error(err)
	}
	// 書き出す先が元の中なら作らない
	if _, err := (Tree{Src: src, Top: top, Prefix: "assets", Dst: filepath.Join(src, "out")}).Build(nil); err == nil {
		t.Error("wrote into the source")
	}
}

func TestBlocking(t *testing.T) {
	ins := []Approval{{File: "a", Approved: true}, {File: "b", Status: "todo"}, {File: "c", Approved: true}, {File: "d", Status: "review"}}
	var got []string
	for _, in := range Blocking(ins) {
		got = append(got, in.File)
	}
	if strings.Join(got, ",") != "b,d" {
		t.Errorf("Blocking = %v", got)
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TestOverlayManifest: 置き場に manifest があれば、元の絵の sha256 が記録と同じ物だけ重ねる。
// 合わない物（元の絵を同じ名前のまま取り込み直した）・sha256 の無い物・記録に無い物は元の絵を出す。
// manifest が無ければ今までどおり全部重ねる。
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

	// 元の絵を掛けた時の物に戻すと（覚えを捨てた後は）また重ねる
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

	// manifest が無い置き場: 全部重ねる
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
