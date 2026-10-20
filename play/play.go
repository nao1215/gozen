package play

import (
	"errors"
	"io"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/layer4/sessionlink"
	runtime "github.com/nao1215/gozen/internal/play"
)

// Config is what the program decides about the screen. The zero value works.
type Config struct {
	// Store opens the saves of a game by its id. Nil keeps them in the user's config
	// directory (gozen/<id>), and in a browser in the page's localStorage.
	Store func(id string) (gozen.Store, error)
	// Mute plays no sound.
	Mute bool
	// Output receives Main's --help and reports. Nil discards them.
	Output io.Writer
}

// Screen is Gozen's screen as an ebiten.Game, for a program that runs its own loop.
// Use it from the game thread only; one Screen may be open at a time. Close it.
type Screen struct{ game *runtime.Game }

// New shows the session's game without the title screen: a session just made by
// gozen.New starts with the start event, one that has run (Begin, Do, Restore) goes on
// from where it is. The screen plays on the session's state, which the session reads
// with Query afterwards; the screen's own events are not the session's. The session's
// Config.Dev decides whether set may change stats.
func New(s *gozen.Session, cfg Config) (*Screen, error) {
	if s == nil {
		return nil, errors.New("play: nil session")
	}
	parts, ok := sessionlink.Of(s)
	if !ok {
		return nil, errors.New("play: a session that did not come from gozen.New")
	}
	opts := runtime.Options{FS: parts.FS, DB: parts.DB, State: parts.State, Fresh: parts.Fresh, SetStats: parts.Dev, NoSound: cfg.Mute, Seed: parts.Seed, Report: cfg.Output}
	if cfg.Store != nil {
		opts.StoreFor = func(id string) (runtime.Store, error) { return cfg.Store(id) }
	}
	g, err := runtime.New(opts)
	if err != nil {
		return nil, err
	}
	return &Screen{game: g}, nil
}

// Update advances the screen one tick (ebiten.Game).
func (sc *Screen) Update() error { return sc.game.Update() }

// Draw draws the screen on dst (ebiten.Game).
func (sc *Screen) Draw(dst *ebiten.Image) { sc.game.Draw(dst) }

// Layout is the game's screen size (ebiten.Game).
func (sc *Screen) Layout(w, h int) (int, int) { return sc.game.Layout(w, h) }

// Close releases the screen's resources: call it after the loop stops.
func (sc *Screen) Close() error { return sc.game.Close() }
