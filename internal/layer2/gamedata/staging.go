package gamedata

import (
	"fmt"
	"io/fs"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// The words of staging (schema_version 4, ADR 0028): stand, and the map forms of effect,
// wait and sfx. Each keeps its old string or number form.

// Stand brings a portrait onto the stage of the novel window, or takes it off. Written
// "none" it takes everyone off.
type Stand struct {
	// Who is the actor (or the name of a speaker with a picture in face).
	Who string `yaml:"who"`
	// Face is a face of the actor, or a picture path for anyone else.
	Face string `yaml:"face"`
	// At is where the portrait stands: left, center, right or a fraction of the width.
	At StandAt `yaml:"at"`
	// Enter is how it comes in: fade (the default) or slide.
	Enter string `yaml:"enter"`
	// Leave takes it off: fade or slide.
	Leave string `yaml:"leave"`
	// None takes everyone off (stand: none).
	None bool `yaml:"-"`
}

// UnmarshalYAML reads "none" or a map.
func (s *Stand) UnmarshalYAML(unmarshal func(any) error) error {
	var t string
	if err := unmarshal(&t); err == nil {
		if t != "none" {
			return fmt.Errorf("stand is none or a map with who, not %q", t)
		}
		s.None = true
		return nil
	}
	type plain Stand
	return unmarshal((*plain)(s))
}

// StandAt is where a portrait stands: left, center, right or a fraction of the width.
type StandAt string

// UnmarshalYAML reads a word or a number.
func (a *StandAt) UnmarshalYAML(unmarshal func(any) error) error {
	var v any
	if err := unmarshal(&v); err != nil {
		return err
	}
	*a = StandAt(fmt.Sprint(v))
	return nil
}

// X is the place as a fraction of the screen's width (false when it is not a place).
func (a StandAt) X() (float64, bool) {
	switch a {
	case "left":
		return 0.25, true
	case "center", "":
		return 0.5, true
	case "right":
		return 0.75, true
	}
	x, err := strconv.ParseFloat(string(a), 64)
	return x, err == nil && x >= 0 && x <= 1
}

// EffectCue is a visual effect: shake, flash, fade, or a reaction of a moving portrait
// (hop, shrink, nod, tilt, freeze, or one its file names).
type EffectCue struct {
	Name string `yaml:"name"`
	// Who shakes one portrait, or plays the reaction on that actor (default: the last speaker).
	Who string `yaml:"who"`
	// Seconds is how long it takes (default: the effect's own).
	Seconds float64 `yaml:"seconds"`
	// Color is the color a fade goes through (default black).
	Color Color `yaml:"color"`
	// Hold is how long a fade stays on its color, in seconds.
	Hold float64 `yaml:"hold"`
	// mapped: written as a map (schema_version 4).
	mapped bool
}

// UnmarshalYAML reads a word or a map.
func (e *EffectCue) UnmarshalYAML(unmarshal func(any) error) error {
	var t string
	if err := unmarshal(&t); err == nil {
		*e = EffectCue{Name: t}
		return nil
	}
	type plain EffectCue
	if err := unmarshal((*plain)(e)); err != nil {
		return err
	}
	e.mapped = true
	return nil
}

// WaitCue is a pause: seconds, and whether skipping passes it.
type WaitCue struct {
	Seconds float64 `yaml:"seconds"`
	// Skip false holds even in skip mode (default true).
	Skip *bool `yaml:"skip"`
	// mapped: written as a map (schema_version 4).
	mapped bool
}

// UnmarshalYAML reads a number of seconds or a map.
func (w *WaitCue) UnmarshalYAML(unmarshal func(any) error) error {
	var n float64
	if err := unmarshal(&n); err == nil {
		*w = WaitCue{Seconds: n}
		return nil
	}
	type plain WaitCue
	if err := unmarshal((*plain)(w)); err != nil {
		return err
	}
	w.mapped = true
	return nil
}

// Skippable reports whether skipping passes the wait.
func (w WaitCue) Skippable() bool { return w.Skip == nil || *w.Skip }

// SFXCue is a sound effect and whether the event waits for it to end.
type SFXCue struct {
	Name string `yaml:"name"`
	// Wait holds the event until the sound ends.
	Wait bool `yaml:"wait"`
	// mapped: written as a map (schema_version 4).
	mapped bool
}

// UnmarshalYAML reads a name or a map.
func (s *SFXCue) UnmarshalYAML(unmarshal func(any) error) error {
	var t string
	if err := unmarshal(&t); err == nil {
		*s = SFXCue{Name: t}
		return nil
	}
	type plain SFXCue
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	s.mapped = true
	return nil
}

// ScreenEffects are the effects of the whole screen.
var ScreenEffects = []string{"shake", "flash", "fade"}

// stand checks a stand command.
func (c *checker) stand(w string, s *Stand) {
	if !c.needsVersion(w, "stand", SchemaVersion4) || s.None {
		return
	}
	db := c.db
	if s.Who == "" {
		c.errorf(diag.Shape, w+".who", "stand needs who (or none)")
		return
	}
	c.say(w, &Say{Who: s.Who, Face: s.Face}, nil)
	if a, ok := db.Actors[s.Who]; ok && (a.Faces == "" || a.NoStanding) && a.Puppet == "" {
		c.errorf(diag.UnknownReference, w+".who", "%s has no standing picture (faces)", s.Who)
	}
	if _, ok := s.At.X(); !ok {
		c.errorf(diag.NotAWord, w+".at", "want left, center, right or a fraction from 0 to 1, not %q", s.At)
	}
	if s.Enter != "" && s.Enter != "fade" && s.Enter != "slide" {
		c.errorf(diag.NotAWord, w+".enter", "want fade or slide, not %q", s.Enter)
	}
	if s.Leave != "" && s.Leave != "fade" && s.Leave != "slide" {
		c.errorf(diag.NotAWord, w+".leave", "want fade or slide, not %q", s.Leave)
	}
	if s.Leave != "" && (s.Enter != "" || s.Face != "") {
		c.errorf(diag.Shape, w, "a stand that leaves takes no enter or face")
	}
}

// effectCue checks the word of an effect and its map form.
func (c *checker) effectCue(w string, e EffectCue) {
	reaction := !slices.Contains(ScreenEffects, e.Name)
	if reaction && !c.needsVersion(w, "the reaction "+e.Name, SchemaVersion4) {
		return
	}
	if e.mapped && !c.needsVersion(w, "effect as a map", SchemaVersion4) {
		return
	}
	if reaction && !slices.Contains(c.reactionNames(), e.Name) {
		c.errorf(diag.NotAWord, w, "want %s, or a reaction (%s), not %q", strings.Join(ScreenEffects, ", "), strings.Join(c.reactionNames(), ", "), e.Name)
	}
	if e.Who != "" { // an actor, or the name of a speaker stood with a picture
		if e.Name == "flash" || e.Name == "fade" {
			c.errorf(diag.Shape, w+".who", "%s is of the whole screen; who goes with shake or a reaction", e.Name)
		}
	}
	if e.Seconds < 0 || e.Hold < 0 {
		c.errorf(diag.OutOfRange, w, "seconds and hold are 0 or more")
	}
	if (e.Color.Valid || e.Hold != 0) && e.Name != "fade" {
		c.errorf(diag.Shape, w, "color and hold go with fade")
	}
}

// reactionNames are the reaction words: the kinds of puppetdef and the reactions the
// game's moving portraits name.
func (c *checker) reactionNames() []string {
	if c.reactions != nil {
		return c.reactions
	}
	names := map[string]bool{}
	for _, k := range puppetdef.Kinds {
		names[k] = true
	}
	for _, id := range slices.Sorted(maps.Keys(c.db.Actors)) {
		a := c.db.Actors[id]
		if a == nil || a.Puppet == "" {
			continue
		}
		b, err := fs.ReadFile(c.db.FS, "assets/"+a.Puppet)
		if err != nil {
			continue
		}
		if spec, err := puppetdef.Parse(b); err == nil {
			for r := range spec.Reactions {
				names[r] = true
			}
		}
	}
	delete(names, "shake") // shake is the screen's; a portrait shakes with who
	c.reactions = slices.Sorted(maps.Keys(names))
	return c.reactions
}

// waitCue checks a wait.
func (c *checker) waitCue(w string, x WaitCue) {
	if x.mapped {
		c.needsVersion(w, "wait as a map", SchemaVersion4)
	}
	if x.Seconds < 0 || math.IsNaN(x.Seconds) || math.IsInf(x.Seconds, 0) {
		c.errorf(diag.OutOfRange, w, "seconds are a number, 0 or more")
	}
}

// sfxCue checks a sound effect.
func (c *checker) sfxCue(w string, s SFXCue) {
	if s.mapped {
		c.needsVersion(w, "sfx as a map", SchemaVersion4)
	}
	if s.Name == "" {
		c.errorf(diag.Shape, w, "sfx needs the name of a sound")
	}
}

// musicCommand checks the fade and stop of the music command.
func (c *checker) musicCommand(w string, m MusicCue) {
	if m.Fade != 0 || m.Stop {
		c.needsVersion(w, "music fade and stop", SchemaVersion4)
	}
	if m.Fade < 0 {
		c.errorf(diag.OutOfRange, w+".fade", "fade is seconds, 0 or more")
	}
	if m.Stop && (m.Song != "" || m.Mood != "") {
		c.errorf(diag.Shape, w, "music stop takes no song or mood")
	}
	if !m.Stop {
		m.Fade = 0
		c.music(w, m)
	}
}
