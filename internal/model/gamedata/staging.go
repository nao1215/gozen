package gamedata

import (
	"fmt"
	"io/fs"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/base/safeint"
	"github.com/nao1215/gozen/internal/model/puppetdef"
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
	c.say(w, &Say{Who: IDRef{ID: s.Who}, Face: s.Face}, nil)
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
	if !finiteNumber(e.Seconds) || !finiteNumber(e.Hold) || e.Seconds < 0 || e.Hold < 0 {
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
	if !finiteNumber(m.Fade) || m.Fade < 0 {
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

// Weather kinds fall over the scene (ADR 0066): WeatherNone clears it.
const (
	WeatherNone   = "none"
	WeatherRain   = "rain"
	WeatherSnow   = "snow"
	WeatherPetals = "petals"
	WeatherEmbers = "embers"
)

// WeatherKinds are the words weather takes, in the order the documentation lists them.
func WeatherKinds() []string {
	return []string{WeatherRain, WeatherSnow, WeatherPetals, WeatherEmbers, WeatherNone}
}

// WeatherCue is the weather command: what falls over the scene from now on, until another
// weather command changes it, kept in saves. A word is the kind at its default power.
type WeatherCue struct {
	// Kind is rain, snow, petals, embers or none.
	Kind string `yaml:"kind"`
	// Power is how much falls, from 0 (nothing) to 1 (a storm); 0.5 when left out.
	Power float64 `yaml:"power"`
	// Wind leans the fall, from -1 (to the left) to 1 (to the right).
	Wind float64 `yaml:"wind"`
	// Color is the color of the drops, flakes, petals or sparks (each kind's own when left out).
	Color Color `yaml:"color"`
	// Seconds is how long the change takes (1 when left out; 0 with seconds: 0 is at once).
	Seconds *float64 `yaml:"seconds"`
}

// UnmarshalYAML reads a word (the kind) or a map.
func (w *WeatherCue) UnmarshalYAML(unmarshal func(any) error) error {
	var t string
	if err := unmarshal(&t); err == nil {
		*w = WeatherCue{Kind: t}
		return nil
	}
	type plain WeatherCue
	return unmarshal((*plain)(w))
}

// Weather is what the state keeps of a weather command: the kind and how it falls, without
// the time the change took.
type Weather struct {
	Kind  string  `json:"kind"`
	Power float64 `json:"power"`
	Wind  float64 `json:"wind,omitempty"`
	// Color is #rrggbb, empty for the kind's own.
	Color string `json:"color,omitempty"`
}

// Weather is what the cue leaves falling; nil for none.
func (w WeatherCue) Weather() *Weather {
	if w.Kind == WeatherNone || w.Kind == "" {
		return nil
	}
	out := &Weather{Kind: w.Kind, Power: w.Power, Wind: w.Wind}
	if out.Power == 0 {
		out.Power = 0.5
	}
	if w.Color.Valid {
		out.Color = fmt.Sprintf("#%02x%02x%02x", w.Color.R, w.Color.G, w.Color.B)
	}
	return out
}

// Ticks is how long the change takes at 60 ticks a second.
func (w WeatherCue) Ticks() int { return cueTicks(w.Seconds) }

// cueTicks is seconds at 60 ticks a second, a second when left out.
func cueTicks(seconds *float64) int {
	if seconds == nil {
		return 60
	}
	return safeint.From(math.Round(*seconds * 60))
}

// Tone presets are the words tone takes (ADR 0066): ToneNone restores the colors.
const (
	ToneNone  = "none"
	ToneGray  = "gray"
	ToneSepia = "sepia"
	ToneNight = "night"
	ToneDusk  = "dusk"
)

// TonePreset is the tone a word names (false for none or a word that is not a preset).
func TonePreset(word string) (Tone, bool) {
	switch word {
	case ToneGray:
		return Tone{Gray: 1}, true
	case ToneSepia:
		return Tone{Gray: 1, Color: "#ffdcaa", Vignette: 0.3}, true
	case ToneNight:
		return Tone{Color: "#7080c0", Bright: -0.25, Vignette: 0.45}, true
	case ToneDusk:
		return Tone{Color: "#ffb88a", Vignette: 0.2}, true
	}
	return Tone{}, false
}

// ToneCue is the tone command: how the scene is colored from now on, until another tone
// command changes it, kept in saves. A word is a preset (gray, sepia, night, dusk, none);
// a map sets the parts itself, starting from the scene as drawn.
type ToneCue struct {
	// Preset is the word of the word form.
	Preset string `yaml:"-"`
	// Color multiplies the scene (white leaves it as drawn).
	Color Color `yaml:"color"`
	// Gray takes the color away, from 0 (none) to 1 (gray).
	Gray float64 `yaml:"gray"`
	// Bright darkens (down to -1, black) or lightens (up to 1, white) the scene.
	Bright float64 `yaml:"bright"`
	// Vignette darkens the edges of the screen, from 0 to 1.
	Vignette float64 `yaml:"vignette"`
	// Seconds is how long the change takes (1 when left out; seconds: 0 is at once).
	Seconds *float64 `yaml:"seconds"`
}

// UnmarshalYAML reads a word (a preset) or a map.
func (t *ToneCue) UnmarshalYAML(unmarshal func(any) error) error {
	var word string
	if err := unmarshal(&word); err == nil {
		*t = ToneCue{Preset: word}
		return nil
	}
	type plain ToneCue
	return unmarshal((*plain)(t))
}

// Tone is what the state keeps of a tone command: the parts, without the time the change took.
type Tone struct {
	// Color is #rrggbb, empty for white.
	Color    string  `json:"color,omitempty"`
	Gray     float64 `json:"gray,omitempty"`
	Bright   float64 `json:"bright,omitempty"`
	Vignette float64 `json:"vignette,omitempty"`
}

// Tone is how the cue leaves the scene colored; nil for the scene as drawn.
func (t ToneCue) Tone() *Tone {
	if t.Preset != "" {
		p, ok := TonePreset(t.Preset)
		if !ok {
			return nil
		}
		return &p
	}
	out := Tone{Gray: t.Gray, Bright: t.Bright, Vignette: t.Vignette}
	if t.Color.Valid {
		out.Color = fmt.Sprintf("#%02x%02x%02x", t.Color.R, t.Color.G, t.Color.B)
	}
	if out == (Tone{}) || out == (Tone{Color: "#ffffff"}) {
		return nil
	}
	return &out
}

// Ticks is how long the change takes at 60 ticks a second.
func (t ToneCue) Ticks() int { return cueTicks(t.Seconds) }

// weatherCue checks a weather command: its kind, and its numbers in their ranges.
func (c *checker) weatherCue(w string, cue WeatherCue) {
	if !c.needsVersion(w, "weather", SchemaVersion12) {
		return
	}
	if kinds := WeatherKinds(); !slices.Contains(kinds, cue.Kind) {
		c.errorf(diag.NotAWord, w, "want %s, not %q", strings.Join(kinds, ", "), cue.Kind)
	}
	if !finiteNumber(cue.Power) || cue.Power < 0 || cue.Power > 1 {
		c.errorf(diag.OutOfRange, w+".power", "power is from 0 to 1")
	}
	if !finiteNumber(cue.Wind) || cue.Wind < -1 || cue.Wind > 1 {
		c.errorf(diag.OutOfRange, w+".wind", "wind is from -1 to 1")
	}
	c.cueSeconds(w, cue.Seconds)
	if cue.Kind == WeatherNone && (cue.Power != 0 || cue.Wind != 0 || cue.Color.Valid) {
		c.errorf(diag.Shape, w, "weather none takes only seconds")
	}
}

// toneCue checks a tone command: a preset word, or its parts in their ranges.
func (c *checker) toneCue(w string, cue ToneCue) {
	if !c.needsVersion(w, "tone", SchemaVersion12) {
		return
	}
	if cue.Preset != "" {
		if _, ok := TonePreset(cue.Preset); !ok && cue.Preset != ToneNone {
			c.errorf(diag.NotAWord, w, "want %s, %s, %s, %s, %s or a map, not %q", ToneGray, ToneSepia, ToneNight, ToneDusk, ToneNone, cue.Preset)
		}
		return
	}
	for _, part := range []struct {
		name      string
		v, lo, hi float64
	}{{"gray", cue.Gray, 0, 1}, {"bright", cue.Bright, -1, 1}, {"vignette", cue.Vignette, 0, 1}} {
		if !finiteNumber(part.v) || part.v < part.lo || part.v > part.hi {
			c.errorf(diag.OutOfRange, w+"."+part.name, "%s is from %g to %g", part.name, part.lo, part.hi)
		}
	}
	c.cueSeconds(w, cue.Seconds)
}

// cueSeconds checks the seconds a weather or tone change takes.
func (c *checker) cueSeconds(w string, seconds *float64) {
	if seconds != nil && (!finiteNumber(*seconds) || *seconds < 0 || *seconds > 60) {
		c.errorf(diag.OutOfRange, w+".seconds", "seconds is from 0 to 60")
	}
}
