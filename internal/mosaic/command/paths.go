package command

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/mosaic/detect"
)

func knownLabel(l string) bool { return slices.Contains(detect.Labels, l) }
func (o *options) validate() error {
	if o.block < 0 || o.block > 256 || o.padBlocks < 0 || o.padBlocks > 256 || o.minBlocks < 0 || o.minBlocks > 256 || o.dilate < -1 || o.dilate > 256 || o.threads < 1 || o.threads > 1024 || o.quality < 1 || o.quality > 100 {
		return fmt.Errorf("invalid numeric options: block/pad/min 0..256, dilate -1..256, threads 1..1024, quality 1..100")
	}
	for name, v := range map[string]float64{"threshold": o.minScore, "sure": o.sure, "margin": o.margin} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("-%s must be finite and in [0,1]", name)
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
			for i := len(tail) - 1; i >= 0; i-- {
				r = filepath.Join(r, tail[i])
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
		return fmt.Errorf("output must be separate from source assets")
	}
	root, err := canonical(game)
	if err != nil {
		return err
	}
	if within(root, dst) {
		return fmt.Errorf("output cannot contain the input game")
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
			return fmt.Errorf("fix/manual/tags files must be outside assets and output")
		}
		for _, other := range selected {
			if p == other {
				return fmt.Errorf("fix/manual/tags must use separate files")
			}
			a, ea := os.Stat(p)
			b, eb := os.Stat(other)
			if ea == nil && eb == nil && os.SameFile(a, b) {
				return fmt.Errorf("fix/manual/tags files alias each other")
			}
		}
		selected = append(selected, p)
	}
	return nil
}
func relativePicture(rel string) bool {
	return fs.ValidPath(rel) && rel != "." && !strings.ContainsAny(rel, "\\:")
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
		return fmt.Errorf("image escapes source assets")
	}
	dst := filepath.Join(c.out, filepath.FromSlash(rel))
	resolved, err := canonical(dst)
	if err != nil {
		return err
	}
	out, err := canonical(c.out)
	if err != nil {
		return err
	}
	if !within(resolved, out) {
		return fmt.Errorf("image output escapes output directory")
	}
	a, ea := os.Stat(source)
	b, eb := os.Stat(dst)
	if ea == nil && eb == nil && os.SameFile(a, b) {
		return fmt.Errorf("output aliases original image")
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
		return fmt.Errorf("view output cannot replace source images")
	}
	for _, name := range []string{c.opts.manualFile, c.opts.fixFile, c.opts.tagsFile} {
		if name != "" {
			p, err := canonical(name)
			if err != nil {
				return err
			}
			if p == target {
				return fmt.Errorf("view output cannot replace configuration")
			}
		}
	}
	return nil
}

func validateReport(c config) error {
	if err := filepath.WalkDir(c.out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == c.out {
				return nil
			}
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("output tree must not contain symlinks")
		}
		return nil
	}); err != nil {
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
			return fmt.Errorf("invalid paths in report")
		}
		if err = validPicture(c, e.File); err != nil {
			return err
		}
	}
	return nil
}
