package puppet_test

import (
	"math/rand/v2"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/puppet"
)

// streamScreen draws a background, the streamer, and the UI on top.
type streamScreen struct {
	streamer *puppet.Puppet
	w, h     int
	talking  bool
}

func (s *streamScreen) Update() error {
	if s.talking {
		s.streamer.Speak()
	} else {
		s.streamer.Stop()
	}
	s.streamer.Update()
	return nil
}

func (s *streamScreen) Draw(screen *ebiten.Image) {
	// ... draw the room behind the streamer here ...
	op := &ebiten.DrawImageOptions{}
	scale := 560 / float64(s.h)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(480-float64(s.w)*scale/2, 540-float64(s.h)*scale+40)
	s.streamer.Draw(screen, op)
	// ... draw the chat and the buttons over it here ...
}

func (s *streamScreen) Layout(int, int) (int, int) { return 960, 540 }

// A streaming game puts the streamer between the room and the UI, moves the
// mouth while a line is shown, and plays a reaction when a gift arrives.
func Example() {
	rig, err := puppet.Load(os.DirFS("assets"), "streamer/puppet.yaml")
	if err != nil {
		return
	}
	p, err := puppet.New(rig, puppet.Config{Rand: rand.New(rand.NewPCG(1, 2))}) //nolint:gosec // Animation timing, not security-sensitive data.
	if err != nil {
		return
	}
	defer p.Close()
	w, h := rig.Size()
	screen := &streamScreen{streamer: p, w: w, h: h}

	// When a gift arrives:
	_ = p.React("hop")         //nolint:errcheck // hop is a built-in reaction.
	_ = p.SetFace("surprised") //nolint:errcheck // An unknown face keeps the current one.
	_ = ebiten.RunGame(screen) //nolint:errcheck // The example ignores the loop's end.
}
