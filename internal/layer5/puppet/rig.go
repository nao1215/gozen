// Package puppet is the shared implementation of moving portraits: pictures
// resolved from the YAML, the motion state, the deformed mesh, and drawing.
// It belongs to presentation layer 5; the public puppet package wraps it.
package puppet

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/png" // the pictures of a puppet are PNG,
	"io/fs"
	"path"

	_ "golang.org/x/image/webp" // or WebP in a browser build (smaller to download)

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// MaxSide is the largest picture side accepted, to bound memory and textures.
const MaxSide = 8192

// Rig is a loaded puppet: the spec and its pictures decoded on the CPU.
type Rig struct {
	Spec  *puppetdef.Spec
	Base  *image.NRGBA
	Faces []Face // Faces[0] is the first face.
}

// Face is one expression with the overlays that fit it.
type Face struct {
	Name  string
	Patch *Patch // nil is the base itself.
	Eyes  *Eyes  // nil means no blinking.
	Mouth *Mouth // nil means no talking.
}

// Eyes is the closed-eyes overlay and its timing.
type Eyes struct {
	Closed Patch
	Every  [2]float64
	Double float64
}

// Mouth is the talking overlays; a nil frame is the face's own mouth.
type Mouth struct {
	Frames []*Patch
	Speed  float64
}

// Size is the base size in pixels.
func (r *Rig) Size() (int, int) { return r.Base.Bounds().Dx(), r.Base.Bounds().Dy() }

// Face returns the index of the named face.
func (r *Rig) Face(name string) (int, bool) {
	for i, f := range r.Faces {
		if f.Name == name {
			return i, true
		}
	}
	return 0, false
}

// Load reads the YAML at file and every picture it names, relative to its directory.
// It borrows fsys only during the call.
func Load(fsys fs.FS, file string) (*Rig, error) {
	if fsys == nil || !fs.ValidPath(file) {
		return nil, &fs.PathError{Op: "puppet", Path: file, Err: fs.ErrInvalid}
	}
	b, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	spec, err := puppetdef.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	l := loader{fsys: fsys, dir: path.Dir(file), cache: map[string]*image.NRGBA{}}
	rig, err := l.resolve(spec)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return rig, nil
}

type loader struct {
	fsys  fs.FS
	dir   string
	cache map[string]*image.NRGBA
	base  *image.NRGBA
}

func (l *loader) resolve(s *puppetdef.Spec) (*Rig, error) {
	base, err := l.read(s.Base)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	l.base = base
	rig := &Rig{Spec: s, Base: base}
	for _, name := range s.FaceNames() {
		f := s.Faces[name]
		face := Face{Name: name}
		full := base // the whole face picture, for cutting its overlays
		if f.Image != "" {
			p, whole, err := l.pic(puppetdef.Pic{Image: f.Image, At: f.At, Area: f.Area}, base)
			if err != nil {
				return nil, fmt.Errorf("faces.%s: %w", name, err)
			}
			face.Patch = p
			full = whole
		}
		eyes := s.Eyes
		if f.Eyes != nil {
			eyes = f.Eyes
		}
		if eyes != nil && !eyes.Off {
			p, _, err := l.pic(eyes.Closed, full)
			if err != nil {
				return nil, fmt.Errorf("faces.%s: eyes.closed: %w", name, err)
			}
			e := &Eyes{Closed: *p, Every: [2]float64{2, 6}, Double: 0.2}
			if eyes.Every != nil {
				e.Every = [2]float64{eyes.Every[0], eyes.Every[1]}
			}
			if eyes.Double != nil {
				e.Double = *eyes.Double
			}
			face.Eyes = e
		}
		mouth := s.Mouth
		if f.Mouth != nil {
			mouth = f.Mouth
		}
		if mouth != nil && !mouth.Off {
			m := &Mouth{Speed: mouth.Speed}
			if m.Speed == 0 {
				m.Speed = 10
			}
			for i, fr := range mouth.Frames {
				if fr.Image == puppetdef.FaceKeyword && fr.At == nil && fr.Area == nil {
					m.Frames = append(m.Frames, nil)
					continue
				}
				p, _, err := l.pic(fr, full)
				if err != nil {
					return nil, fmt.Errorf("faces.%s: mouth.frames[%d]: %w", name, i, err)
				}
				m.Frames = append(m.Frames, p)
			}
			face.Mouth = m
		}
		rig.Faces = append(rig.Faces, face)
	}
	return rig, nil
}

// pic resolves a picture against ref (the picture it is drawn over). It also
// returns the whole picture with the patch applied.
func (l *loader) pic(p puppetdef.Pic, ref *image.NRGBA) (*Patch, *image.NRGBA, error) {
	img, err := l.read(p.Image)
	if err != nil {
		return nil, nil, err
	}
	bounds := l.base.Bounds()
	if p.At != nil {
		at := image.Pt(int(p.At[0]+0.5), int(p.At[1]+0.5))
		patch := &Patch{Img: img, At: at}
		if !patch.Rect().In(bounds) {
			return nil, nil, fmt.Errorf("%s at %v (%dx%d) lies outside the base %dx%d", p.Image, at, img.Bounds().Dx(), img.Bounds().Dy(), bounds.Dx(), bounds.Dy())
		}
		return patch, over(ref, patch), nil
	}
	if img.Bounds().Size() != bounds.Size() {
		return nil, nil, fmt.Errorf("%s is %dx%d: give at, or make it as large as the base (%dx%d)", p.Image, img.Bounds().Dx(), img.Bounds().Dy(), bounds.Dx(), bounds.Dy())
	}
	var area image.Rectangle
	if p.Area != nil {
		area = image.Rect(int(p.Area[0]), int(p.Area[1]), int(p.Area[0]+p.Area[2]), int(p.Area[1]+p.Area[3]))
		if !area.In(bounds) {
			return nil, nil, fmt.Errorf("%s: area %v lies outside the base", p.Image, area)
		}
	}
	if opaque(img) {
		img = withAlpha(img, ref)
	}
	patch, ok := Cut(ref, img, area)
	if !ok {
		return nil, nil, fmt.Errorf("%s does not differ from the picture under it", p.Image)
	}
	return &patch, img, nil
}

// opaque reports whether a picture has no transparency at all, as image
// generators often write it. Such a picture takes the alpha of the one under it.
func opaque(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}

func withAlpha(img, ref *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = ref.Pix[i]
	}
	return out
}

func over(ref *image.NRGBA, p *Patch) *image.NRGBA {
	out := image.NewNRGBA(ref.Bounds())
	copy(out.Pix, ref.Pix)
	draw.Draw(out, p.Rect(), p.Img, image.Point{}, draw.Over)
	return out
}

func (l *loader) read(name string) (*image.NRGBA, error) {
	p := path.Join(l.dir, name)
	if img, ok := l.cache[p]; ok {
		return img, nil
	}
	if !fs.ValidPath(p) {
		return nil, fmt.Errorf("picture %q leaves the puppet's file system", name)
	}
	b, err := fs.ReadFile(l.fsys, p)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxSide || cfg.Height > MaxSide {
		return nil, fmt.Errorf("%s: %dx%d is not within 1 to %d pixels", name, cfg.Width, cfg.Height, MaxSide)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	n := toNRGBA(img)
	l.cache[p] = n
	return n, nil
}
