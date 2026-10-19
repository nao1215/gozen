// Command puppet shows a moving portrait (the puppet package) on its own: a
// streamer who blinks, talks, breathes, sways, reacts, and changes her face.
// Keys try each motion; --tour plays them all in order.
package main

import (
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/puppet"
)

// The streamer's pictures and puppet.yaml (CC BY 4.0, see assets/LICENSE).
//
//go:embed assets
var assets embed.FS

const screenW, screenH = 960, 540

const help = `puppet - a moving portrait made with the Gozen puppet package (example)

[Usage]
  puppet [OPTIONS]

  Shows a streamer who blinks, breathes, sways her head and hair, and tilts her
  head now and then. Keys: Space starts and stops talking; H hop, S shrink,
  K shake, N nod, T tilt play a reaction; F changes the face; Esc quits.

[Examples]
  - try the keys
    puppet
  - play every motion once and quit
    puppet --tour
  - write the tour as PNG frames (30 per second) for a video
    puppet --tour --frames frames

[Options]
  Window:
        --window SCALE   the window size as a SCALE of 960x540 (default 1)
        --seed SEED      fix the random numbers of the motion with this SEED

  Tour:
        --tour           play every motion once, then quit
        --frames DIR     with --tour, write every other frame to this new DIR

  General:
    -h, --help           print this help and exit

Requirements: a display and the Ebitengine graphics libraries (on Linux, see
the README's Requirements; Xvfb works for --tour --frames).
See https://github.com/nao1215/gozen#gozen

Documentation:   https://nao1215.github.io/gozen/
Report an issue: https://github.com/nao1215/gozen/issues
GitHub Sponsors: https://github.com/sponsors/nao1215
`

type options struct {
	window float64
	seed   uint64
	tour   bool
	frames string
	help   bool
}

// parse reads GNU-style options: --name value, --name=value, -h.
func parse(args []string) (options, error) {
	o := options{window: 1, seed: 1}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(a, "=")
		next := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case a == "-h" || a == "--help":
			o.help = true
		case name == "--tour" && !hasValue:
			o.tour = true
		case name == "--window":
			v, err := next()
			if err != nil {
				return o, err
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || !(f > 0) || f > 8 {
				return o, fmt.Errorf("--window %q: want a scale above 0, up to 8", v)
			}
			o.window = f
		case name == "--seed":
			v, err := next()
			if err != nil {
				return o, err
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return o, fmt.Errorf("--seed %q: want a whole number", v)
			}
			o.seed = n
		case name == "--frames":
			v, err := next()
			if err != nil {
				return o, err
			}
			o.frames = v
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2:
			return o, fmt.Errorf("unknown option %s; long options take two dashes (-%s)", a, a)
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown option %s", name)
		default:
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if o.frames != "" && !o.tour {
		return o, errors.New("--frames needs --tour")
	}
	return o, nil
}

// cue is one step of the tour.
type cue struct {
	at    float64 // seconds
	label string
	do    func(p *puppet.Puppet) error
}

func tour() []cue {
	none := func(*puppet.Puppet) error { return nil }
	react := func(name string) func(*puppet.Puppet) error {
		return func(p *puppet.Puppet) error { return p.React(name) }
	}
	return []cue{
		{0, "idle: blink, breath, head and hair sway", none},
		{1.0, "Speak()", func(p *puppet.Puppet) error { p.Speak(); return nil }},
		{2.2, "Stop()", func(p *puppet.Puppet) error { p.Stop(); return nil }},
		{2.4, `React("hop")`, react("hop")},
		{3.0, `React("nod")`, react("nod")},
		{3.5, `React("shake")`, react("shake")},
		{4.1, `React("tilt")`, react("tilt")},
		{5.3, `SetFace("surprised")`, func(p *puppet.Puppet) error {
			if err := p.SetFace("surprised"); err != nil {
				return err
			}
			return p.React("shrink")
		}},
		{6.3, `SetFace("smile") + Speak()`, func(p *puppet.Puppet) error { p.Speak(); return p.SetFace("smile") }},
		{7.0, "Stop()", func(p *puppet.Puppet) error { p.Stop(); return nil }},
	}
}

type stage struct {
	p        *puppet.Puppet
	w, h     int
	bg       *ebiten.Image
	tick     int
	talking  bool
	surprise bool
	label    string
	tour     []cue
	frames   string
	shot     int
	err      error
}

func (s *stage) Update() error {
	if s.err != nil {
		return s.err
	}
	if s.tour != nil {
		for len(s.tour) > 0 && float64(s.tick)/60 >= s.tour[0].at {
			s.label = s.tour[0].label
			if err := s.tour[0].do(s.p); err != nil {
				return err
			}
			s.tour = s.tour[1:]
		}
		if len(s.tour) == 0 && float64(s.tick)/60 >= 7.2 {
			return ebiten.Termination
		}
	} else if err := s.keys(); err != nil {
		return err
	}
	s.p.Update()
	s.tick++
	return nil
}

func (s *stage) keys() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		s.talking = !s.talking
		if s.talking {
			s.p.Speak()
		} else {
			s.p.Stop()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		s.surprise = !s.surprise
		face := "smile"
		if s.surprise {
			face = "surprised"
		}
		if err := s.p.SetFace(face); err != nil {
			return err
		}
	}
	for key, name := range map[ebiten.Key]string{ebiten.KeyH: "hop", ebiten.KeyS: "shrink", ebiten.KeyK: "shake", ebiten.KeyN: "nod", ebiten.KeyT: "tilt"} {
		if inpututil.IsKeyJustPressed(key) {
			if err := s.p.React(name); err != nil {
				return err
			}
		}
	}
	s.label = "Space: talk  H/S/K/N/T: hop shrink shake nod tilt  F: face  Esc: quit"
	return nil
}

func (s *stage) Draw(screen *ebiten.Image) {
	screen.DrawImage(s.bg, nil)
	// The streamer stands on the bottom edge, between the background and the UI.
	op := &ebiten.DrawImageOptions{}
	k := 560 / float64(s.h)
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(screenW/2-float64(s.w)*k/2, screenH-float64(s.h)*k+40)
	s.p.Draw(screen, op)
	x := 16
	if s.frames != "" {
		x = 200 // inside the part kept by the documentation's cropped GIF
	}
	ebitenutil.DebugPrintAt(screen, s.label, x, 12)
	if s.frames != "" && s.tick%2 == 0 && s.err == nil {
		s.err = s.save(screen)
	}
}

func (s *stage) save(screen *ebiten.Image) (err error) {
	img := image.NewRGBA(screen.Bounds())
	screen.ReadPixels(img.Pix)
	f, err := os.Create(filepath.Join(s.frames, fmt.Sprintf("f%05d.png", s.shot)))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	s.shot++
	return png.Encode(f, img)
}

func (s *stage) Layout(int, int) (int, int) { return screenW, screenH }

func background() *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, screenW, screenH))
	for y := range screenH {
		t := float64(y) / screenH
		c := color.RGBA{uint8(222 - 30*t), uint8(232 - 24*t), uint8(244 - 10*t), 255}
		for x := range screenW {
			img.SetRGBA(x, y, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}

func run(args []string, stdout io.Writer) error {
	o, err := parse(args)
	if err != nil {
		return usageError{err}
	}
	if o.help {
		_, err := io.WriteString(stdout, help)
		return err
	}
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return err
	}
	rig, err := puppet.Load(sub, "streamer/puppet.yaml")
	if err != nil {
		return err
	}
	p, err := puppet.New(rig, puppet.Config{Rand: rand.New(rand.NewPCG(o.seed, 7))}) //nolint:gosec // Animation timing, not security-sensitive data.
	if err != nil {
		return err
	}
	defer p.Close()
	w, h := rig.Size()
	s := &stage{p: p, w: w, h: h, bg: background(), frames: o.frames}
	if o.tour {
		s.tour = tour()
	}
	if o.frames != "" {
		if err := os.Mkdir(o.frames, 0o750); err != nil {
			return err
		}
		ebiten.SetVsyncEnabled(false)
		ebiten.SetTPS(ebiten.SyncWithFPS) // one Update per frame, so every frame is a tick
	}
	ebiten.SetWindowTitle("Gozen puppet")
	ebiten.SetWindowSize(int(screenW*o.window), int(screenH*o.window))
	return ebiten.RunGame(s)
}

type usageError struct{ error }

func main() {
	err := run(os.Args[1:], os.Stdout)
	if err == nil {
		return
	}
	log.SetFlags(0)
	log.Print(err)
	var u usageError
	if errors.As(err, &u) {
		log.Print(`Run "puppet --help" for usage.`)
		os.Exit(2)
	}
	os.Exit(1)
}
