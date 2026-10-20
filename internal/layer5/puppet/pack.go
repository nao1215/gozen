package puppet

import (
	"bytes"
	"fmt"
	"image"
	"path"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// Pack rewrites a loaded rig so that every overlay is a small picture with at.
// It returns the new spec and the pictures to write next to it, by file name.
// The base keeps its name. Loading the packed files gives the same patches.
func Pack(r *Rig) (*puppetdef.Spec, map[string]*image.NRGBA) {
	spec := *r.Spec
	files := map[string]*image.NRGBA{path.Base(spec.Base): r.Base}
	spec.Base = path.Base(spec.Base)
	names := map[*image.NRGBA]string{r.Base: spec.Base}
	name := func(stem string, img *image.NRGBA) string {
		if n, ok := names[img]; ok {
			return n
		}
		n := stem + ".png"
		for i := 2; files[n] != nil; i++ {
			n = fmt.Sprintf("%s-%d.png", stem, i)
		}
		files[n] = img
		names[img] = n
		return n
	}
	pic := func(stem string, p *Patch) puppetdef.Pic {
		return puppetdef.Pic{Image: name(stem, p.Img), At: []float64{float64(p.At.X), float64(p.At.Y)}}
	}
	eyes := func(f Face) *puppetdef.Eyes {
		if f.Eyes == nil {
			return &puppetdef.Eyes{Off: true}
		}
		d := f.Eyes.Double
		return &puppetdef.Eyes{Closed: pic("eyes-"+f.Name, &f.Eyes.Closed), Every: f.Eyes.Every[:], Double: &d}
	}
	mouth := func(f Face) *puppetdef.Mouth {
		if f.Mouth == nil {
			return &puppetdef.Mouth{Off: true}
		}
		m := &puppetdef.Mouth{Speed: f.Mouth.Speed}
		for i, fr := range f.Mouth.Frames {
			if fr == nil {
				m.Frames = append(m.Frames, puppetdef.Pic{Image: puppetdef.FaceKeyword})
				continue
			}
			m.Frames = append(m.Frames, pic(fmt.Sprintf("mouth-%s-%d", f.Name, i+1), fr))
		}
		return m
	}
	spec.Eyes, spec.Mouth = nil, nil
	spec.Faces = map[string]puppetdef.Face{}
	sameEyes, sameMouth := true, true
	for _, f := range r.Faces[1:] {
		sameEyes = sameEyes && sameOverlay(eyesPatches(f), eyesPatches(r.Faces[0]))
		sameMouth = sameMouth && sameOverlay(mouthPatches(f), mouthPatches(r.Faces[0])) && sameSpeed(f, r.Faces[0])
	}
	if sameEyes {
		spec.Eyes = eyes(r.Faces[0])
		if spec.Eyes.Off {
			spec.Eyes = nil
		}
	}
	if sameMouth {
		spec.Mouth = mouth(r.Faces[0])
		if spec.Mouth.Off {
			spec.Mouth = nil
		}
	}
	for _, f := range r.Faces {
		var out puppetdef.Face
		if f.Patch != nil {
			p := pic("face-"+f.Name, f.Patch)
			out.Image, out.At = p.Image, p.At
		}
		if !sameEyes {
			out.Eyes = eyes(f)
		}
		if !sameMouth {
			out.Mouth = mouth(f)
		}
		spec.Faces[f.Name] = out
	}
	if len(r.Faces) == 1 && r.Faces[0].Name == puppetdef.DefaultFace && r.Faces[0].Patch == nil {
		spec.Faces = nil
	}
	return &spec, files
}

func eyesPatches(f Face) []*Patch {
	if f.Eyes == nil {
		return nil
	}
	return []*Patch{&f.Eyes.Closed}
}

func mouthPatches(f Face) []*Patch {
	if f.Mouth == nil {
		return nil
	}
	return f.Mouth.Frames
}

func sameSpeed(a, b Face) bool {
	if a.Mouth == nil || b.Mouth == nil {
		return a.Mouth == b.Mouth
	}
	return a.Mouth.Speed == b.Mouth.Speed
}

func sameOverlay(a, b []*Patch) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) {
			return false
		}
		if a[i] != nil && (a[i].At != b[i].At || a[i].Img.Rect != b[i].Img.Rect || !bytes.Equal(a[i].Img.Pix, b[i].Img.Pix)) {
			return false
		}
	}
	return true
}

// Describe lists where each overlay lies, one line each, for authors checking positions.
func Describe(r *Rig) string {
	var b []byte
	w, h := r.Size()
	b = fmt.Appendf(b, "base %s %dx%d\n", r.Spec.Base, w, h)
	rect := func(p *Patch) string {
		rc := p.Rect()
		return fmt.Sprintf("at [%d, %d] size %dx%d", rc.Min.X, rc.Min.Y, rc.Dx(), rc.Dy())
	}
	for _, f := range r.Faces {
		if f.Patch != nil {
			b = fmt.Appendf(b, "face %s: %s\n", f.Name, rect(f.Patch))
		} else {
			b = fmt.Appendf(b, "face %s: the base\n", f.Name)
		}
		if f.Eyes != nil {
			b = fmt.Appendf(b, "  eyes closed: %s\n", rect(&f.Eyes.Closed))
		}
		if f.Mouth != nil {
			for i, fr := range f.Mouth.Frames {
				if fr == nil {
					b = fmt.Appendf(b, "  mouth %d: the face's own\n", i+1)
				} else {
					b = fmt.Appendf(b, "  mouth %d: %s\n", i+1, rect(fr))
				}
			}
		}
	}
	return string(b)
}
