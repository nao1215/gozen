package mosaic

import (
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

// guardYAML runs a YAML decode and turns a panic of the decoder into an error. Fix files and
// the YAML files that name pictures are untrusted input (SECURITY.md), and the decoder panics
// on some shapes, such as a list of five numbers where a rectangle of four is wanted. This
// is the guard of internal/layer1/decodeguard; the mosaic component may not import layer 1.
func guardYAML(decode func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yaml: the decoder rejected the input: %v", r)
		}
	}()
	return decode()
}

type Fix struct {
	Add   [][4]int `yaml:"add,omitempty" json:"add,omitempty"`
	Poly  [][]int  `yaml:"poly,omitempty" json:"poly,omitempty"`
	Drop  [][4]int `yaml:"drop,omitempty" json:"drop,omitempty"`
	Skip  bool     `yaml:"skip,omitempty" json:"skip,omitempty"`
	Block int      `yaml:"block,omitempty" json:"block,omitempty"`

	Margin float64 `yaml:"margin,omitempty" json:"margin,omitempty"`

	Grow [][8]int `yaml:"grow,omitempty" json:"grow,omitempty"`

	Checked bool   `yaml:"checked,omitempty" json:"checked,omitempty"`
	Note    string `yaml:"note,omitempty" json:"note,omitempty"`
}

func (f Fix) Polygons() []Polygon {
	var out []Polygon
	for _, p := range f.Poly {
		var pg Polygon
		for i := 0; i+1 < len(p); i += 2 {
			pg = append(pg, image.Pt(p[i], p[i+1]))
		}
		out = append(out, pg)
	}
	return out
}

func (f Fix) Empty() bool {
	return len(f.Add) == 0 && len(f.Poly) == 0 && len(f.Drop) == 0 && len(f.Grow) == 0 && !f.Skip && f.Block == 0 && f.Margin == 0 && !f.Checked && f.Note == ""
}

type Fixes map[string]Fix

func LoadFixes(path string) (Fixes, error) {
	fx := Fixes{}
	if path == "" {
		return fx, nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // The caller explicitly selects this input file.
	if os.IsNotExist(err) {
		return fx, nil
	}
	if err != nil {
		return nil, err
	}
	if err := guardYAML(func() error {
		file, err := parser.ParseBytes(b, 0)
		if err != nil {
			return err
		}
		if len(file.Docs) != 1 || file.Docs[0].Body == nil {
			return errors.New("fix YAML requires one nonempty document")
		}
		if err := yaml.NodeToValue(file.Docs[0].Body, &fx, yaml.Strict()); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if fx == nil {
		fx = Fixes{}
	}
	if err := ValidateFixes(fx); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return fx, nil
}

func (fx Fixes) Save(path, header string) error {
	if err := ValidateFixes(fx); err != nil {
		return err
	}
	keys := make([]string, 0, len(fx))
	for k, f := range fx {
		if !f.Empty() {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	if header != "" {
		for line := range strings.SplitSeq(strings.TrimRight(header, "\n"), "\n") {
			sb.WriteString("# " + line + "\n")
		}
	}
	if len(keys) == 0 {
		sb.WriteString("{}\n")
	}
	for _, k := range keys {
		kb, err := yaml.Marshal(k)
		if err != nil {
			return err
		}
		vb, err := yaml.MarshalWithOptions(fx[k], yaml.Flow(true))
		if err != nil {
			return err
		}
		sb.WriteString(strings.TrimSpace(string(kb)) + ": " + strings.TrimSpace(string(vb)) + "\n")
	}
	return WriteFile(path, func(w io.Writer) error { _, err := io.WriteString(w, sb.String()); return err })
}

func Combine(f, o Fix) Fix {
	f.Add = append(append([][4]int{}, f.Add...), o.Add...)
	f.Poly = append(append([][]int{}, f.Poly...), o.Poly...)
	f.Drop = append(append([][4]int{}, f.Drop...), o.Drop...)
	f.Grow = append(append([][8]int{}, f.Grow...), o.Grow...)
	if o.Margin > 0 {
		f.Margin = o.Margin
	}
	f.Skip = f.Skip || o.Skip
	f.Checked = f.Checked || o.Checked
	if o.Block > 0 {
		f.Block = o.Block
	}
	if o.Note != "" {
		f.Note = strings.TrimSpace(f.Note + " " + o.Note)
	}
	return f
}

func Rect(r [4]int) image.Rectangle { return image.Rect(r[0], r[1], r[0]+r[2], r[1]+r[3]) }

func ValidateFixes(fx Fixes) error {
	for k, f := range fx {
		if !fs.ValidPath(k) || k == "." || strings.ContainsAny(k, "\\:") {
			return fmt.Errorf("invalid image path %q", k)
		}
		if f.Block < 0 || f.Block > 256 {
			return errors.New("block must be 0..256")
		}
		for _, g := range f.Grow {
			for _, v := range g {
				if v < -1_000_000 || v > 1_000_000 {
					return errors.New("grow coordinate exceeds limit")
				}
			}
			if g[2] <= 0 || g[3] <= 0 {
				return fmt.Errorf("%s: the grow box %v needs a width and height above 0", k, g)
			}
		}
		if f.Margin < 0 || f.Margin > 1 || math.IsNaN(f.Margin) || math.IsInf(f.Margin, 0) {
			return fmt.Errorf("%s: margin %v is below 0", k, f.Margin)
		}
		for _, r := range append(append([][4]int{}, f.Add...), f.Drop...) {
			for _, v := range r {
				if v < -1_000_000 || v > 1_000_000 {
					return errors.New("rectangle coordinate exceeds limit")
				}
			}
			if r[2] <= 0 || r[3] <= 0 {
				return fmt.Errorf("%s: the box %v needs a width and height above 0", k, r)
			}
		}
		for _, p := range f.Poly {
			for _, v := range p {
				if v < -1_000_000 || v > 1_000_000 {
					return errors.New("polygon coordinate exceeds limit")
				}
			}
			if len(p) < 6 || len(p)%2 != 0 || len(p) > 8192 {
				return fmt.Errorf("%s: the polygon %v needs 3 or more points as x, y pairs", k, p)
			}
			var twiceArea int64
			for i := 0; i < len(p); i += 2 {
				j := (i + 2) % len(p)
				twiceArea += int64(p[i])*int64(p[j+1]) - int64(p[j])*int64(p[i+1])
			}
			if twiceArea == 0 {
				return errors.New("polygon area must be nonzero")
			}

		}
	}
	return nil
}
