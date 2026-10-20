// Package assetfs reads a game directory with its images overlaid by those from another directory,
// and builds a distributable directory from the overlaid images.
//
// The overlay is a "layered read": when opening <prefix>/<relative path>, the top directory's file at
// <relative path> is returned if it exists, otherwise the original file. The original directory is only read, never changed.
// If the top directory has a manifest (Config.Manifest), only files built from the current original image are overlaid.
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

	man Manifest // how to read the overlay's manifest (not checked when Name is "")

	mu     sync.Mutex
	loaded bool              // whether the manifest has been loaded
	sums   map[string]string // manifest file -> sha256 of the original image (nil if there is no manifest)
	hashes map[string]string // original image (base path) -> content sha256 (memo)
}

// Manifest describes how to read the per-image records (a JSON array) kept in the overlay directory.
// The File field of an element is the path relative to prefix; Sum is the sha256 of the original image it was built from.
// If the overlay directory has a file called Name, only images that appear in the records and whose Sum matches
// the current original are overlaid (so a stale replacement is not served when the original image changes
// under the same name). Without it, everything is overlaid.
type Manifest struct {
	Name string // name of the file inside the overlay directory
	File string // name of the element field holding the relative path
	Sum  string // name of the element field holding the original image's sha256
}

// Forget drops the manifest and the memoized original-image sha256s (re-read on next need, e.g. after the overlay is rebuilt).
func (o *OverlayFS) Forget() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.loaded, o.sums, o.hashes = false, nil, nil
}

// load reads the manifest (call with o.mu held). If unreadable or malformed it is treated as absent.
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
		} // if absent, "" (unused)
		o.sums[path.Clean(file)] = strings.ToLower(sum)
	}
}

// fresh reports whether top's r (the replacement for base's name) may be used. Without a manifest it always is.
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

// Coverage describes the state of the replacement for base's name (a path under prefix).
//   - Has: the overlay directory has a replacement image.
//   - Used: that image is overlaid now (with a manifest, only if built from the current original).
//   - Checked: it appears in the manifest and its sum equals the current original (the tool has seen the current image; if there is no replacement, it is known to be unneeded).
//     false without a manifest.
//
// Use it to check before shipping where an original that needs replacement must not go out as is, such as sales images.
type Coverage struct{ Has, Used, Checked bool }

// Coverage returns the state of the replacement for name.
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

// Stale returns the sorted paths (relative to prefix) of images that exist in the overlay but are not overlaid because they no longer match the original.
// It returns nil without a manifest. Images missing from the original (fs.ErrNotExist) are not counted; if the original cannot be read (permissions, I/O)
// that error is returned.
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
			return nil // absent from the original (records, working diagrams, etc.), so not overlaid and not counted
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

// rel returns the path of name relative to prefix if name is under prefix.
func (o *OverlayFS) rel(name string) (string, bool) {
	if o.prefix == "" || o.prefix == "." {
		return name, name != "."
	}
	r, ok := strings.CutPrefix(name, o.prefix+"/")
	return r, ok && r != ""
}

// topFile returns top's path if name has a replacement (only files that also exist in base are replaced).
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

// dir is a base directory (the listed replaced files get top's Info).
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

// ReadDir returns base's listing (replaced files carry top's Info).
func (o *OverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	es, err := fs.ReadDir(o.base, name)
	return o.swap(name, es), err
}

// swap gives the replaced files in the listing of directory name top's Info.
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

// Tree is how to build the distributable directory.
type Tree struct {
	Src    string   // the original game directory (read only)
	Top    string   // directory of replacement images (laid out relative to Src/Prefix)
	Prefix string   // directory of the images to replace (relative to Src, usually assets)
	Dst    string   // output destination (recreated; must not be inside Src or Top)
	Skip   []string // things not to copy (relative to Src; if Top is inside Src it is excluded automatically)
	// Outputs lists the images in Top accepted as current output (relative to Prefix). nil accepts all.
	// An image in Top that is not accepted (left over from an earlier run) is an error rather than being used.
	Outputs []string
}

// Result is the outcome of Build.
type Result struct {
	Files    int      // number of files copied
	Replaced []string // replaced images (relative to Prefix)
}

// Build copies Src to Dst, replacing files under Prefix that have the same relative path in Top with Top's version.
// Files in Top that are not in Src (survey lists, working diagrams, etc.) are not copied. Src is not changed.
// If ins is given, the original hashes, approvals and required replacements are checked before distribution.
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
	// If approvals are given, check all records against the existing replacements before copying.
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
	// Re-verify that the distributed tree really matches the approved content, so changes made during copying are not missed.
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

// within reports whether p is dir itself or inside it.
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
