package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	slashpath "path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/mosaic/detect"
)

// parseClasses reads --classes. Empty means the default class order.
func parseClasses(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return slices.Clone(detect.Labels), nil
	}
	out := strings.Split(v, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	if err := detect.ValidateClasses(out); err != nil {
		return nil, fmt.Errorf("--classes: %w", err)
	}
	return out, nil
}

func (o *options) validate() error {
	if o.block < 0 || o.block > 256 || o.padBlocks < 0 || o.padBlocks > 256 || o.minBlocks < 0 || o.minBlocks > 256 || o.dilate < -1 || o.dilate > 256 || o.threads < 1 || o.threads > 1024 || o.quality < 1 || o.quality > 100 {
		return errors.New("invalid numeric options: block/pad/min 0..256, dilate -1..256, threads 1..1024, quality 1..100")
	}
	// A fixed order, so that several invalid options always name the same one.
	for _, v := range []struct {
		name  string
		value float64
	}{{"threshold", o.minScore}, {"sure", o.sure}, {"margin", o.margin}} {
		if math.IsNaN(v.value) || math.IsInf(v.value, 0) || v.value < 0 || v.value > 1 {
			return fmt.Errorf("--%s must be finite and in [0,1]", v.name)
		}
	}
	return nil
}

// canonical resolves existing ancestors too, so an absent output below a link
// cannot bypass source containment checks.
func canonical(name string) (string, error) {
	a, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		r, err := filepath.EvalSymlinks(a)
		if err == nil {
			for _, t := range slices.Backward(tail) {
				r = filepath.Join(r, t)
			}
			return r, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(a)
		if parent == a {
			return "", err
		}
		tail = append(tail, filepath.Base(a))
		a = parent
	}
}

func within(p, root string) bool {
	r, err := filepath.Rel(root, p)
	return err == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r))
}
func overlap(a, b string) bool { return within(a, b) || within(b, a) }
func protectPaths(game, out string, files ...string) error {
	source, err := canonical(filepath.Join(game, "assets"))
	if err != nil {
		return err
	}
	dst, err := canonical(out)
	if err != nil {
		return err
	}
	if overlap(dst, source) {
		return errors.New("output must be separate from source assets")
	}
	root, err := canonical(game)
	if err != nil {
		return err
	}
	if within(root, dst) {
		return errors.New("output cannot contain the input game")
	}
	var selected []string
	for _, name := range files {
		if name == "" {
			continue
		}
		p, err := canonical(name)
		if err != nil {
			return err
		}
		if within(p, source) || overlap(p, dst) {
			return errors.New("fix/manual/tags files must be outside assets and output")
		}
		for _, other := range selected {
			if p == other {
				return errors.New("fix/manual/tags must use separate files")
			}
			a, ea := os.Stat(p)
			b, eb := os.Stat(other)
			if ea == nil && eb == nil && os.SameFile(a, b) {
				return errors.New("fix/manual/tags files alias each other")
			}
		}
		selected = append(selected, p)
	}
	return nil
}

func relativePicture(rel string) bool {
	return fs.ValidPath(rel) && rel != "." && !strings.ContainsAny(rel, "\\:")
}

// outputRel is where auto writes the pixelated picture of an asset, below --out: the path
// of the asset, and <name>.png for a WebP one, which is written as PNG. Every command
// that looks for an output goes through it, so that they agree on the name.
func outputRel(rel string) string {
	if strings.HasSuffix(strings.ToLower(rel), ".webp") {
		return rel + ".png"
	}
	return rel
}

// sourceRel is the reverse of outputRel: the asset that a file below --out was made from.
// <name>.webp.png stands for <name>.webp unless the assets hold a picture of that very name.
func sourceRel(assets, rel string) string {
	if trimmed, ok := strings.CutSuffix(rel, ".png"); ok && outputRel(trimmed) == rel && !isFile(filepath.Join(assets, filepath.FromSlash(rel))) {
		return trimmed
	}
	return rel
}

func isFile(name string) bool {
	st, err := os.Stat(name)
	return err == nil && !st.IsDir()
}

// below gives the slash-separated path of a file under a directory.
func below(dir, file string) (string, bool) {
	root, err := filepath.Abs(dir)
	if err != nil || !within(file, root) {
		return "", false
	}
	rel, err := filepath.Rel(root, file)
	return filepath.ToSlash(rel), err == nil
}

// resolve reads an IMAGE operand, the same way for auto, inspect and the manual commands.
// rel is the slash-separated path below assets/, path the file that was named, and orig
// the original picture in the assets. The readings, in order:
//
//   - a path below <game-dir>/assets/, with or without a leading assets/, when that
//     picture exists. It comes first, so that a file of the same name in the working
//     directory does not hide the asset;
//   - a path to an existing file. One inside the assets is that asset, one inside --out
//     stands for the asset it was made from, and any other has no asset (rel is empty);
//   - otherwise the asset of that name, which the caller then reports as missing.
//
// A backslash separates directories on every system: it is not allowed in an asset name
// (relativePicture), and a Windows-style operand such as assets\a.png names the same
// picture as assets/a.png.
func resolve(c config, arg string) (rel, path, orig string) {
	assets := filepath.Join(c.gameDir, "assets")
	asset := func(rel string) string { return filepath.Join(assets, filepath.FromSlash(rel)) }
	name := slashpath.Clean(strings.ReplaceAll(arg, "\\", "/"))
	stripped := strings.TrimPrefix(name, "assets/")
	for _, rel := range []string{name, stripped} {
		if relativePicture(rel) && isFile(asset(rel)) {
			return rel, asset(rel), asset(rel)
		}
	}
	if isFile(arg) {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", arg, arg
		}
		if rel, ok := below(assets, abs); ok {
			return rel, arg, asset(rel)
		}
		if rel, ok := below(c.out, abs); ok {
			rel = sourceRel(assets, rel)
			return rel, arg, asset(rel)
		}
		return "", arg, arg
	}
	return stripped, asset(stripped), asset(stripped)
}

func validPicture(c config, rel string) error {
	if !relativePicture(rel) {
		return fmt.Errorf("invalid asset-relative image %q", rel)
	}
	root, err := canonical(filepath.Join(c.gameDir, "assets"))
	if err != nil {
		return err
	}
	source := filepath.Join(c.gameDir, "assets", filepath.FromSlash(rel))
	p, err := canonical(source)
	if err != nil {
		return err
	}
	if !within(p, root) {
		return errors.New("image escapes source assets")
	}
	dst := filepath.Join(c.out, filepath.FromSlash(outputRel(rel)))
	resolved, err := canonical(dst)
	if err != nil {
		return err
	}
	out, err := canonical(c.out)
	if err != nil {
		return err
	}
	if !within(resolved, out) {
		return errors.New("image output escapes output directory")
	}
	a, ea := os.Stat(source)
	b, eb := os.Stat(dst)
	if ea == nil && eb == nil && os.SameFile(a, b) {
		return errors.New("output aliases original image")
	}
	return nil
}

func protectView(c config, orig, dst string) error {
	source, err := canonical(filepath.Join(c.gameDir, "assets"))
	if err != nil {
		return err
	}
	target, err := canonical(dst)
	if err != nil {
		return err
	}
	input, err := canonical(orig)
	if err != nil {
		return err
	}
	if within(target, source) || target == input {
		return errors.New("view output cannot replace source images")
	}
	for _, name := range []string{c.opts.manualFile, c.opts.fixFile, c.opts.tagsFile} {
		if name != "" {
			p, err := canonical(name)
			if err != nil {
				return err
			}
			if p == target {
				return errors.New("view output cannot replace configuration")
			}
		}
	}
	return nil
}

// noSymlinks refuses an output tree that holds a symbolic link, the directory itself
// included: a link could lead a write or a removal out of the tree. A directory that does
// not exist yet has none.
func noSymlinks(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == dir {
				return nil
			}
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("output tree must not contain symlinks")
		}
		return nil
	})
}

func validateReport(c config) error {
	if err := noSymlinks(c.out); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(c.out, "report.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var entries []entry
	if err = json.Unmarshal(b, &entries); err != nil {
		return fmt.Errorf("invalid report: %w", err)
	}
	for _, e := range entries {
		if !relativePicture(e.File) || (e.Output != "" && !relativePicture(e.Output)) {
			return errors.New("invalid paths in report")
		}
		if err = validPicture(c, e.File); err != nil {
			return err
		}
	}
	return nil
}
