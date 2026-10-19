// Package puppetdef decodes and checks the YAML of a moving portrait (puppet).
// It belongs to model layer 2: it reads no images and knows nothing of drawing.
// The presentation layer resolves the pictures and checks their sizes.
package puppetdef

import (
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// FaceKeyword in mouth frames means "the face picture's own mouth" (no overlay).
const FaceKeyword = "face"

// DefaultFace names the only face when faces is omitted.
const DefaultFace = "default"

// Reaction kinds understood by the presentation layer.
var Kinds = []string{"hop", "shrink", "shake", "nod", "tilt", "freeze"}

// Spec is one puppet file. Every coordinate is a pixel of the base picture with
// the origin at its top left. Picture paths are relative to the YAML file.
type Spec struct {
	Base  string          `yaml:"base,omitempty"`
	Faces map[string]Face `yaml:"faces,omitempty"`
	// Face is the first face (default: "default" when faces is omitted, else required
	// unless faces has one entry).
	Face string `yaml:"face,omitempty"`
	// Fade is the face change in seconds (0 means 0.25).
	Fade      float64             `yaml:"fade,omitempty"`
	Eyes      *Eyes               `yaml:"eyes,omitempty"`
	Mouth     *Mouth              `yaml:"mouth,omitempty"`
	Head      *Head               `yaml:"head,omitempty"`
	Hair      []Shape             `yaml:"hair,omitempty"`
	Motion    Motion              `yaml:"motion,omitempty"`
	Reactions map[string]Reaction `yaml:"reactions,omitempty"`
}

// Pic is a picture placed on the base. A picture as large as the base needs no
// position: the differing part is cut out when loading. A smaller picture needs at.
// Area limits the search for the differing part of a full picture.
type Pic struct {
	Image string    `yaml:"image,omitempty"`
	At    []float64 `yaml:"at,omitempty"`
	Area  []float64 `yaml:"area,omitempty"`
}

// UnmarshalYAML accepts a bare path as well as the mapping.
func (p *Pic) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err == nil {
		*p = Pic{Image: s}
		return nil
	}
	type plain Pic
	var v plain
	if err := yaml.UnmarshalWithOptions(b, &v, yaml.Strict()); err != nil {
		return err
	}
	*p = Pic(v)
	return nil
}

// Face is one expression. An empty face is the base picture itself. Eyes and
// Mouth replace the top-level ones for this face.
type Face struct {
	Image string    `yaml:"image,omitempty"`
	At    []float64 `yaml:"at,omitempty"`
	Area  []float64 `yaml:"area,omitempty"`
	Eyes  *Eyes     `yaml:"eyes,omitempty"`
	Mouth *Mouth    `yaml:"mouth,omitempty"`
}

// Eyes is the blink: the closed eyes drawn over the open ones.
type Eyes struct {
	Closed Pic `yaml:"closed,omitempty"`
	// Every is the shortest and longest gap between blinks in seconds (default 2 to 6).
	Every []float64 `yaml:"every,omitempty"`
	// Double is the chance of a second blink right after (default 0.2).
	Double *float64 `yaml:"double,omitempty"`
	// Off turns blinking off for a face whose eyes do not fit the closed picture.
	Off bool `yaml:"off,omitempty"`
}

// Mouth is the talk: while speaking, the frames take turns.
type Mouth struct {
	Frames []Pic `yaml:"frames,omitempty"`
	// Speed is changes per second (0 means 10).
	Speed float64 `yaml:"speed,omitempty"`
	Off   bool    `yaml:"off,omitempty"`
}

// Head is the part of the picture that turns and shifts. Center and Radius make a
// circle that covers the head with its hair; Neck is the pivot at the neck root.
// The weight falls off outside the circle and above the neck, so the joint bends smoothly.
type Head struct {
	Center []float64 `yaml:"center,omitempty"`
	Radius float64   `yaml:"radius,omitempty"`
	Neck   []float64 `yaml:"neck,omitempty"`
}

// Shape is a rectangle [x, y, w, h] or a polygon [[x, y], ...]. Hair shapes sway
// behind the head; the bottom of a shape moves the most.
type Shape struct {
	Rect []float64   `yaml:"rect,omitempty"`
	Poly [][]float64 `yaml:"poly,omitempty"`
}

// Motion holds the strengths. An omitted block uses the default. In a block, a
// zero strength turns that motion off and a zero period uses the default.
type Motion struct {
	Breath *Breath `yaml:"breath,omitempty"`
	Sway   *Sway   `yaml:"sway,omitempty"`
	Tilt   *Tilt   `yaml:"tilt,omitempty"`
	Hair   *Spring `yaml:"hair,omitempty"`
	Talk   *Talk   `yaml:"talk,omitempty"`
}

// Breath stretches the body up and down by Depth (a fraction of the height).
type Breath struct {
	Depth  float64 `yaml:"depth,omitempty"`
	Period float64 `yaml:"period,omitempty"`
}

// Sway turns the head by up to Angle degrees and shifts it by up to Shift pixels.
type Sway struct {
	Angle  float64 `yaml:"angle,omitempty"`
	Shift  float64 `yaml:"shift,omitempty"`
	Period float64 `yaml:"period,omitempty"`
}

// Tilt is the occasional head tilt: Angle degrees, once in Every seconds.
type Tilt struct {
	Angle float64   `yaml:"angle,omitempty"`
	Every []float64 `yaml:"every,omitempty"`
}

// Spring makes the hair follow the head late. Gain 0 turns it off.
type Spring struct {
	Gain      float64 `yaml:"gain,omitempty"`
	Stiffness float64 `yaml:"stiffness,omitempty"`
	Damping   float64 `yaml:"damping,omitempty"`
}

// Talk moves the head while speaking (Nod in pixels).
type Talk struct {
	Nod float64 `yaml:"nod,omitempty"`
}

// Reaction is a named one-shot motion. Kind defaults to the name for the
// built-in names. Amount scales the default strength (0 means 1); Time is seconds.
type Reaction struct {
	Kind   string  `yaml:"kind,omitempty"`
	Amount float64 `yaml:"amount,omitempty"`
	Time   float64 `yaml:"time,omitempty"`
}

// Parse decodes and validates a puppet file.
func Parse(b []byte) (*Spec, error) {
	var s Spec
	if err := yaml.UnmarshalWithOptions(b, &s, yaml.Strict()); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// FaceNames returns the face names in a stable order (the first face first).
func (s *Spec) FaceNames() []string {
	if len(s.Faces) == 0 {
		return []string{DefaultFace}
	}
	names := make([]string, 0, len(s.Faces))
	for n := range s.Faces {
		names = append(names, n)
	}
	slices.Sort(names)
	first := s.FirstFace()
	i := slices.Index(names, first)
	return append([]string{first}, slices.Delete(names, i, i+1)...)
}

// FirstFace is the face shown first.
func (s *Spec) FirstFace() string {
	if s.Face != "" {
		return s.Face
	}
	if len(s.Faces) == 0 {
		return DefaultFace
	}
	for n := range s.Faces {
		return n // Validate ensures there is exactly one.
	}
	return ""
}

// Pictures lists every picture path the spec names (relative to its file), once each.
func (s *Spec) Pictures() []string {
	var out []string
	add := func(p string) {
		if p != "" && p != FaceKeyword && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	overlays := func(e *Eyes, m *Mouth) {
		if e != nil {
			add(e.Closed.Image)
		}
		if m != nil {
			for _, f := range m.Frames {
				add(f.Image)
			}
		}
	}
	add(s.Base)
	overlays(s.Eyes, s.Mouth)
	for _, n := range s.FaceNames() {
		f := s.Faces[n]
		add(f.Image)
		overlays(f.Eyes, f.Mouth)
	}
	return out
}

// ReactionOf returns the reaction a name plays: a defined one or a built-in kind.
func (s *Spec) ReactionOf(name string) (Reaction, bool) {
	if r, ok := s.Reactions[name]; ok {
		if r.Kind == "" {
			r.Kind = name
		}
		return r, true
	}
	if slices.Contains(Kinds, name) {
		return Reaction{Kind: name}, true
	}
	return Reaction{}, false
}

// Validate checks the structure. It does not read pictures or modify the spec.
func (s *Spec) Validate() error {
	if err := checkPath(s.Base); err != nil {
		return fmt.Errorf("base: %w", err)
	}
	if s.Face != "" && len(s.Faces) > 0 {
		if _, ok := s.Faces[s.Face]; !ok {
			return fmt.Errorf("face: %q is not in faces", s.Face)
		}
	}
	if s.Face != "" && len(s.Faces) == 0 && s.Face != DefaultFace {
		return fmt.Errorf("face: %q is not in faces", s.Face)
	}
	if s.Face == "" && len(s.Faces) > 1 {
		return errors.New("face: name the first face when there are several faces")
	}
	if err := nonNeg("fade", s.Fade); err != nil {
		return err
	}
	for name, f := range s.Faces {
		if name == "" {
			return errors.New("faces: empty name")
		}
		if f.Image != "" {
			if err := checkPic(Pic{Image: f.Image, At: f.At, Area: f.Area}); err != nil {
				return fmt.Errorf("faces.%s: %w", name, err)
			}
		} else if f.At != nil || f.Area != nil {
			return fmt.Errorf("faces.%s: at and area need an image", name)
		}
		if err := checkEyes(f.Eyes); err != nil {
			return fmt.Errorf("faces.%s.eyes: %w", name, err)
		}
		if err := checkMouth(f.Mouth); err != nil {
			return fmt.Errorf("faces.%s.mouth: %w", name, err)
		}
	}
	if err := checkEyes(s.Eyes); err != nil {
		return fmt.Errorf("eyes: %w", err)
	}
	if err := checkMouth(s.Mouth); err != nil {
		return fmt.Errorf("mouth: %w", err)
	}
	if s.Head != nil {
		h := s.Head
		if len(h.Center) != 2 || len(h.Neck) != 2 || !finite(h.Center...) || !finite(h.Neck...) {
			return errors.New("head: center and neck need [x, y]")
		}
		if !(h.Radius > 0) || math.IsInf(h.Radius, 0) {
			return errors.New("head: radius must be positive")
		}
		if h.Neck[1] <= h.Center[1] {
			return errors.New("head: neck must be below the center")
		}
	}
	if len(s.Hair) > 0 && s.Head == nil {
		return errors.New("hair: needs head (the hair follows the head)")
	}
	for i, sh := range s.Hair {
		if err := checkShape(sh); err != nil {
			return fmt.Errorf("hair[%d]: %w", i, err)
		}
	}
	if err := s.Motion.check(); err != nil {
		return fmt.Errorf("motion.%w", err)
	}
	for name, r := range s.Reactions {
		kind := r.Kind
		if kind == "" {
			kind = name
		}
		if name == "" || !slices.Contains(Kinds, kind) {
			return fmt.Errorf("reactions.%s: kind %q is not one of %s", name, kind, strings.Join(Kinds, ", "))
		}
		if err := nonNeg("reactions."+name+".amount", r.Amount); err != nil {
			return err
		}
		if err := nonNeg("reactions."+name+".time", r.Time); err != nil {
			return err
		}
	}
	return nil
}

func (m Motion) check() error {
	if b := m.Breath; b != nil {
		if err := nonNeg("breath", b.Depth, b.Period); err != nil {
			return err
		}
		if b.Depth > 0.2 {
			return errors.New("breath: depth is a fraction of the height (at most 0.2)")
		}
	}
	if w := m.Sway; w != nil {
		if err := nonNeg("sway", w.Angle, w.Shift, w.Period); err != nil {
			return err
		}
		if w.Angle > 45 {
			return errors.New("sway: angle is in degrees (at most 45)")
		}
	}
	if t := m.Tilt; t != nil {
		if err := nonNeg("tilt", t.Angle); err != nil {
			return err
		}
		if t.Angle > 45 {
			return errors.New("tilt: angle is in degrees (at most 45)")
		}
		if err := checkRange("tilt.every", t.Every); err != nil {
			return err
		}
	}
	if h := m.Hair; h != nil {
		if err := nonNeg("hair", h.Gain, h.Stiffness, h.Damping); err != nil {
			return err
		}
	}
	if t := m.Talk; t != nil {
		if err := nonNeg("talk", t.Nod); err != nil {
			return err
		}
	}
	return nil
}

func checkEyes(e *Eyes) error {
	if e == nil || e.Off {
		if e != nil && e.Closed.Image != "" {
			return errors.New("off eyes take no closed picture")
		}
		return nil
	}
	if err := checkPic(e.Closed); err != nil {
		return fmt.Errorf("closed: %w", err)
	}
	if err := checkRange("every", e.Every); err != nil {
		return err
	}
	if d := e.Double; d != nil && (!(*d >= 0) || *d > 1) {
		return errors.New("double: a chance in [0, 1]")
	}
	return nil
}

func checkMouth(m *Mouth) error {
	if m == nil || m.Off {
		if m != nil && len(m.Frames) > 0 {
			return errors.New("off mouth takes no frames")
		}
		return nil
	}
	if len(m.Frames) < 2 {
		return errors.New("frames: at least two shapes (use face for the face's own mouth)")
	}
	for i, f := range m.Frames {
		if f.Image == FaceKeyword && f.At == nil && f.Area == nil {
			continue
		}
		if err := checkPic(f); err != nil {
			return fmt.Errorf("frames[%d]: %w", i, err)
		}
	}
	return nonNeg("speed", m.Speed)
}

func checkPic(p Pic) error {
	if err := checkPath(p.Image); err != nil {
		return err
	}
	if p.At != nil && (len(p.At) != 2 || !finite(p.At...)) {
		return errors.New("at needs [x, y]")
	}
	if p.Area != nil {
		if p.At != nil {
			return errors.New("area is for full pictures; a picture with at needs none")
		}
		if len(p.Area) != 4 || !finite(p.Area...) || p.Area[2] <= 0 || p.Area[3] <= 0 {
			return errors.New("area needs [x, y, w, h] with a positive size")
		}
	}
	return nil
}

func checkShape(s Shape) error {
	switch {
	case s.Rect != nil && s.Poly != nil:
		return errors.New("give rect or poly, not both")
	case s.Rect != nil:
		if len(s.Rect) != 4 || !finite(s.Rect...) || s.Rect[2] <= 0 || s.Rect[3] <= 0 {
			return errors.New("rect needs [x, y, w, h] with a positive size")
		}
	case s.Poly != nil:
		if len(s.Poly) < 3 {
			return errors.New("poly needs at least three points")
		}
		for _, p := range s.Poly {
			if len(p) != 2 || !finite(p...) {
				return errors.New("poly points need [x, y]")
			}
		}
	default:
		return errors.New("give rect or poly")
	}
	return nil
}

func checkPath(p string) error {
	if p == "" {
		return errors.New("needs a picture path")
	}
	if path.IsAbs(p) || strings.Contains(p, `\`) || !strings.HasSuffix(strings.ToLower(p), ".png") {
		return fmt.Errorf("picture %q must be a relative PNG path with slashes", p)
	}
	return nil
}

func checkRange(name string, r []float64) error {
	if r == nil {
		return nil
	}
	if len(r) != 2 || !finite(r...) || r[0] <= 0 || r[1] < r[0] {
		return fmt.Errorf("%s needs [shortest, longest] seconds, positive and in order", name)
	}
	return nil
}

func nonNeg(name string, vs ...float64) error {
	for _, v := range vs {
		if !(v >= 0) || math.IsInf(v, 0) {
			return fmt.Errorf("%s: values must be finite and not negative", name)
		}
	}
	return nil
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
