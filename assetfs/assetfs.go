// Package assetfs は、ゲームのディレクトリの絵を別のディレクトリの絵で差し替えて読む作りと、
// 差し替えた絵で配る版のディレクトリを作る作り。
//
// 差し替えは「重ねる読み方」: <prefix>/<相対パス> を開くとき、上のディレクトリに <相対パス> の
// ファイルがあればそれを、無ければ元のファイルを返す。元のディレクトリは読むだけで変えない。
// 上のディレクトリに manifest（Config.Manifest）があれば、今の元の絵から作った物だけを重ねる。
package assetfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Config selects a subtree and optional source-hash manifest for an overlay.
// The zero value overlays the root without a manifest. Prefix and Manifest.Name
// are slash-separated fs.FS paths; neither may escape its filesystem.
type Config struct {
	Prefix   string
	Manifest Manifest
}

// New overlays matching base files with top files under cfg.Prefix. Directories
// and files absent from base are never added. The caller retains both FS values;
// New does not close them. The resulting FS supports concurrent reads.
func New(base, top fs.FS, cfg Config) (*OverlayFS, error) {
	if base == nil || top == nil {
		return nil, fmt.Errorf("assetfs: nil filesystem: %w", fs.ErrInvalid)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "."
	}
	if !fs.ValidPath(prefix) {
		return nil, fmt.Errorf("assetfs: invalid prefix: %w", fs.ErrInvalid)
	}
	if cfg.Manifest.Name != "" && (!fs.ValidPath(cfg.Manifest.Name) || cfg.Manifest.File == "" || cfg.Manifest.Sum == "") {
		return nil, fmt.Errorf("assetfs: invalid manifest: %w", fs.ErrInvalid)
	}
	return &OverlayFS{base: base, prefix: prefix, top: top, man: cfg.Manifest}, nil
}

// OverlayFS is a read-only filesystem returned by New. Concurrent reads are safe.
type OverlayFS struct {
	base   fs.FS
	prefix string
	top    fs.FS

	man Manifest // 置き場の manifest の読み方（Name が "" なら確かめない）

	mu     sync.Mutex
	loaded bool              // manifest を読んだか
	sums   map[string]string // manifest の file → 元の絵の sha256（manifest が無ければ nil）
	hashes map[string]string // 元の絵（base のパス）→ 中身の sha256（覚え）
}

// Manifest は重ねる側の置き場にある「絵ごとの記録」（JSON の配列）の読み方。
// 要素の File の項目が prefix からの相対パス、Sum の項目がその絵を作った時の元の絵の中身の sha256。
// 置き場に Name のファイルがあれば、記録に載っていて Sum が今の元の絵と同じ物だけを重ねる
// （元の絵が同じ名前のまま別の絵に変わったとき、古い差し替えを出さないため）。無ければ全部重ねる。
type Manifest struct {
	Name string // 置き場の中のファイルの名前
	File string // 要素の相対パスの項目の名前
	Sum  string // 要素の元の絵の sha256 の項目の名前
}

// Forget は manifest と元の絵の sha256 の覚えを捨てる（次に要る時に読み直す。置き場を作り直した後など）。
func (o *OverlayFS) Forget() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.loaded, o.sums, o.hashes = false, nil, nil
}

// load は manifest を読む（o.mu を持って呼ぶ）。読めない・形が違うときは無い物として扱う。
func (o *OverlayFS) load() {
	if o.loaded {
		return
	}
	o.loaded, o.sums, o.hashes = true, nil, map[string]string{}
	if o.man.Name == "" {
		return
	}
	b, err := fs.ReadFile(o.top, o.man.Name)
	if err != nil {
		return
	}
	var es []map[string]any
	if json.Unmarshal(b, &es) != nil {
		return
	}
	o.sums = map[string]string{}
	for _, e := range es {
		file, ok := field(e, o.man.File).(string)
		if !ok || file == "" {
			continue
		}
		sum, ok := field(e, o.man.Sum).(string)
		if !ok {
			sum = ""
		} // 無ければ ""（使わない）
		o.sums[path.Clean(file)] = strings.ToLower(sum)
	}
}

// fresh は top の r（base の name の差し替え）を使ってよいか。manifest が無ければいつも使う。
func (o *OverlayFS) fresh(name, r string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.load()
	if o.sums == nil {
		return true
	}
	want := o.sums[r]
	if want == "" {
		return false
	}
	got, ok := o.hashes[name]
	if !ok {
		b, err := fs.ReadFile(o.base, name)
		if err != nil {
			return false
		}
		h := sha256.Sum256(b)
		got = hex.EncodeToString(h[:])
		o.hashes[name] = got
	}
	return got == want
}

// Coverage は base の name（prefix の下のパス）の差し替えの様子。
//   - Has: 置き場に差し替えの絵がある。
//   - Used: その絵を今重ねる（manifest があれば、今の元の絵から作った物）。
//   - Checked: manifest に載っていて、その sum が今の元の絵と同じ（今の絵を道具が見た。差し替えが無ければ「要らない」と分かる）。
//     manifest が無ければ false。
//
// 販売用の画像のように、差し替えの要る絵を元のまま出してはならない所で、出す前に確かめる。
type Coverage struct{ Has, Used, Checked bool }

// Coverage は name の差し替えの様子を返す。
func (o *OverlayFS) Coverage(name string) Coverage {
	var c Coverage
	r, ok := o.rel(name)
	if !ok {
		return c
	}
	if st, err := fs.Stat(o.top, r); err == nil && !st.IsDir() {
		c.Has = true
	}
	if st, err := fs.Stat(o.base, name); err != nil || st.IsDir() {
		return Coverage{}
	}
	o.mu.Lock()
	o.load()
	listed := o.sums != nil && o.sums[r] != ""
	o.mu.Unlock()
	if listed {
		c.Checked = o.fresh(name, r)
	}
	c.Used = c.Has && o.fresh(name, r)
	return c
}

// Stale は置き場にあるのに元の絵と合わないので重ねない絵（prefix からの相対パス、並べた物）を返す。
// manifest が無ければ nil。元に無い絵（fs.ErrNotExist）は数えず、元を読めない時（権限・I/O）は
// そのエラーを返す。
func (o *OverlayFS) Stale() ([]string, error) {
	o.mu.Lock()
	o.load()
	none := o.sums == nil
	o.mu.Unlock()
	if none {
		return nil, nil
	}
	var out []string
	err := fs.WalkDir(o.top, ".", func(r string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		name := r
		if o.prefix != "" && o.prefix != "." {
			name = o.prefix + "/" + r
		}
		st, err := fs.Stat(o.base, name)
		if errors.Is(err, fs.ErrNotExist) {
			return nil // 元に無い物（記録・作業の図など）は重ねないので数えない
		}
		if err != nil {
			return err
		}
		if st.IsDir() {
			return nil
		}
		if !o.fresh(name, r) {
			out = append(out, r)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// rel は name が prefix の下なら prefix からの相対パスを返す。
func (o *OverlayFS) rel(name string) (string, bool) {
	if o.prefix == "" || o.prefix == "." {
		return name, name != "."
	}
	r, ok := strings.CutPrefix(name, o.prefix+"/")
	return r, ok && r != ""
}

// topFile は name の差し替えがあれば top のパスを返す（base にもあるファイルだけ差し替える）。
func (o *OverlayFS) topFile(name string) (string, bool) {
	r, ok := o.rel(name)
	if !ok {
		return "", false
	}
	st, err := fs.Stat(o.top, r)
	if err != nil || st.IsDir() {
		return "", false
	}
	if st, err := fs.Stat(o.base, name); err != nil || st.IsDir() {
		return "", false
	}
	if !o.fresh(name, r) {
		return "", false
	}
	return r, true
}

// Open implements fs.FS and rejects invalid relative paths.
func (o *OverlayFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if r, ok := o.topFile(name); ok {
		return o.top.Open(r)
	}
	f, err := o.base.Open(name)
	if err != nil {
		return nil, err
	}
	if d, ok := f.(fs.ReadDirFile); ok {
		return dir{ReadDirFile: d, o: o, name: name}, nil
	}
	return f, nil
}

// dir は base のディレクトリ（一覧の差し替えたファイルの Info を top の物にする）。
type dir struct {
	fs.ReadDirFile
	o    *OverlayFS
	name string
}

func (d dir) ReadDir(n int) ([]fs.DirEntry, error) {
	es, err := d.ReadDirFile.ReadDir(n)
	return d.o.swap(d.name, es), err
}

// Stat implements fs.StatFS using the selected file.
func (o *OverlayFS) Stat(name string) (fs.FileInfo, error) {
	if r, ok := o.topFile(name); ok {
		return fs.Stat(o.top, r)
	}
	return fs.Stat(o.base, name)
}

// ReadFile implements fs.ReadFileFS without changing either filesystem.
func (o *OverlayFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	if r, ok := o.topFile(name); ok {
		return fs.ReadFile(o.top, r)
	}
	return fs.ReadFile(o.base, name)
}

// ReadDir は base の一覧を返す（差し替えたファイルの Info は top の物）。
func (o *OverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	es, err := fs.ReadDir(o.base, name)
	return o.swap(name, es), err
}

// swap はディレクトリ name の一覧のうち、差し替えたファイルの Info を top の物にする。
func (o *OverlayFS) swap(name string, es []fs.DirEntry) []fs.DirEntry {
	for i, e := range es {
		if e.IsDir() {
			continue
		}
		if r, ok := o.topFile(path.Join(name, e.Name())); ok {
			if st, err := fs.Stat(o.top, r); err == nil {
				es[i] = fs.FileInfoToDirEntry(st)
			}
		}
	}
	return es
}

// Tree は配る版のディレクトリの作り方。
type Tree struct {
	Src    string   // 元のゲームのディレクトリ（読むだけ）
	Top    string   // 差し替えの絵のディレクトリ（Src/Prefix からの相対パスで置く）
	Prefix string   // 差し替える絵のディレクトリ（Src からの相対パス。ふつう assets）
	Dst    string   // 書き出す先（作り直す。Src・Top の中は不可）
	Skip   []string // 写さない物（Src からの相対パス。Top が Src の中ならそれも自動で外す）
	// Outputs は Top のうち今の出力として認める絵（Prefix からの相対パス）。nil なら全部認める。
	// 認めない絵が Top にあれば（前の回の残り）、使わずに誤りにする。
	Outputs []string
}

// Result は Build の結果。
type Result struct {
	Files    int      // 写したファイルの数
	Replaced []string // 差し替えた絵（Prefix からの相対パス）
}

// Build は Src を Dst に写し、Prefix の下のファイルのうち Top に同じ相対パスの物があるものを Top の物で置き換える。
// Top にあって Src に無いファイル（調べの一覧・作業の図など）は写さない。Src は変えない。
// ins を渡すと元ハッシュと承認、必須差し替えを配布前に照合する。
func (t Tree) Build(ins []Approval) (Result, error) {
	var res Result
	src, err := filepath.Abs(t.Src)
	if err != nil {
		return res, err
	}
	dst, err := filepath.Abs(t.Dst)
	if err != nil {
		return res, err
	}
	top := ""
	if t.Top != "" {
		if top, err = filepath.Abs(t.Top); err != nil {
			return res, err
		}
	}
	if within(dst, src) || (top != "" && within(dst, top)) || within(src, dst) {
		return res, fmt.Errorf("assetfs: the output %s must be apart from %s and %s", t.Dst, t.Src, t.Top)
	}
	// 承認が渡された場合、コピーに先立って全記録と実在する差し替えを照合する。
	if ins != nil {
		gate := Gate{Base: os.DirFS(src), Prefix: t.Prefix, Records: ins, Outputs: t.Outputs}
		if top != "" {
			gate.Top = os.DirFS(top)
		}
		for _, a := range ins {
			if err := gate.Check(a.File); err != nil {
				return res, err
			}
		}
		if top != "" {
			err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.Type().IsRegular() {
					return nil
				}
				r, err := filepath.Rel(top, p)
				if err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(src, t.Prefix, r)); os.IsNotExist(err) {
					return nil
				}
				return gate.Check(filepath.ToSlash(r))
			})
			if err != nil {
				return res, err
			}
		}
	}
	skip := map[string]bool{}
	for _, s := range t.Skip {
		skip[filepath.Clean(filepath.Join(src, s))] = true
	}
	if top != "" && within(top, src) {
		skip[top] = true
	}
	prefix := filepath.Join(src, filepath.FromSlash(t.Prefix))
	var allowed map[string]bool
	if t.Outputs != nil {
		allowed = map[string]bool{}
		for _, o := range t.Outputs {
			allowed[filepath.ToSlash(filepath.Clean(o))] = true
		}
	}
	var stale []string
	if err := os.RemoveAll(dst); err != nil {
		return res, err
	}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip[p] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		r, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, r)
		if d.IsDir() {
			return os.MkdirAll(out, 0o750)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		from := p
		if top != "" && within(p, prefix) {
			a, err := filepath.Rel(prefix, p)
			if err != nil {
				return err
			}
			if st, err := os.Stat(filepath.Join(top, a)); err == nil && st.Mode().IsRegular() {
				if a = filepath.ToSlash(a); allowed != nil && !allowed[a] {
					stale = append(stale, a)
				} else {
					from = filepath.Join(top, filepath.FromSlash(a))
					res.Replaced = append(res.Replaced, a)
				}
			}
		}
		res.Files++
		return copyFile(from, out)
	})
	if err != nil {
		return res, err
	}
	sort.Strings(res.Replaced)
	if len(stale) > 0 {
		sort.Strings(stale)
		return res, fmt.Errorf("assetfs: %d pictures in %s are not the current outputs (left from an older run): %s",
			len(stale), t.Top, strings.Join(stale, ", "))
	}
	// コピー中の変更も見逃さず、実際の配布木が承認された内容か再照合する。
	for _, a := range ins {
		gate := Gate{Base: os.DirFS(src), Prefix: t.Prefix, Records: ins, Outputs: t.Outputs}
		if top != "" {
			gate.Top = os.DirFS(top)
		}
		if err := gate.Check(a.File); err != nil {
			return res, err
		}
		rel := filepath.Join(t.Prefix, filepath.FromSlash(a.File))
		from := filepath.Join(src, rel)
		if slices.Contains(res.Replaced, a.File) {
			from = filepath.Join(top, filepath.FromSlash(a.File))
		}
		want, err := os.ReadFile(from) //nolint:gosec // The caller selects the source and destination; traversal validates their containment.
		if err != nil {
			return res, err
		}
		got, err := os.ReadFile(filepath.Join(dst, rel)) //nolint:gosec // The caller selects the source and destination; traversal validates their containment.
		if err != nil {
			return res, err
		}
		if !bytes.Equal(want, got) || (a.Required && strings.EqualFold(a.Source, fmt.Sprintf("%x", sha256.Sum256(got)))) {
			return res, fmt.Errorf("assetfs: output differs from approved content: %s", a.File)
		}
	}
	return res, nil
}

// within は p が dir そのものかその中か。
func within(p, dir string) bool {
	r, err := filepath.Rel(dir, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

func copyFile(from, to string) (err error) {
	in, err := os.Open(from) //nolint:gosec // The caller selects the source and destination; traversal validates their containment.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return err
	}
	out, err := os.Create(to) //nolint:gosec // The caller selects the source and destination; traversal validates their containment.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
