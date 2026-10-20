package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
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
	for name, v := range map[string]float64{"threshold": o.minScore, "sure": o.sure, "margin": o.margin} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("--%s must be finite and in [0,1]", name)
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

func validateReport(c config) error {
	if err := filepath.WalkDir(c.out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == c.out {
				return nil
			}
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("output tree must not contain symlinks")
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
			return errors.New("invalid paths in report")
		}
		if err = validPicture(c, e.File); err != nil {
			return err
		}
	}
	return nil
}
