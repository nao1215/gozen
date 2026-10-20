package gfx

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Font is a caller-supplied font for custom screens and overlays. It has no global
// font or scale state. Use it only from the Ebitengine game thread.
type Font struct{ source *text.GoTextFaceSource }

// Load parses a TrueType or OpenType font from path in fsys. It retains decoded
// data, borrows the FS only during this call, and returns nil on failure.
func Load(fsys fs.FS, path string) (*Font, error) {
	if fsys == nil || !fs.ValidPath(path) {
		return nil, &fs.PathError{Op: "font", Path: path, Err: fs.ErrInvalid}
	}
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	src, err := text.NewGoTextFaceSource(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", path, err)
	}
	return &Font{source: src}, nil
}

// Measure returns pixel dimensions with line spacing 1.4 times size. Nonpositive
// or nonfinite sizes produce zero dimensions.
func (f *Font) Measure(s string, size float64) (float64, float64) {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return 0, 0
	}
	return text.Measure(s, &text.GoTextFace{Source: f.source, Size: size}, size*1.4)
}

// Draw draws text using the host's transform, alignment, and color. options is
// copied; nil selects the Ebitengine defaults. Invalid sizes draw nothing.
func (f *Font) Draw(dst *ebiten.Image, s string, size float64, options *text.DrawOptions) {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return
	}
	var op text.DrawOptions
	if options != nil {
		op = *options
	}
	if op.LineSpacing == 0 {
		op.LineSpacing = size * 1.4
	}
	text.Draw(dst, s, &text.GoTextFace{Source: f.source, Size: size}, &op)
}
