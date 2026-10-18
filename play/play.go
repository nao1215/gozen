package play

import (
	"context"
	"io"
	"io/fs"

	"github.com/hajimehoshi/ebiten/v2"

	runtime "github.com/nao1215/gozen/internal/play"
)

// Config selects a game's startup behavior. Zero values use the game's window
// size, audio, title screen, and UserConfigDir/gozen/<game-id> for saves.
type Config struct {
	// Seed fixes gameplay randomness; zero chooses a new seed.
	Seed uint64
	// SaveDir overrides the complete save directory, without appending the game ID.
	SaveDir string
	// Mute disables the audio device.
	Mute bool
	// WindowScale scales the window only; zero fits the monitor, otherwise (0,4].
	WindowScale float64
	// Fullscreen overrides the saved window setting for this run.
	Fullscreen bool
	// Development enables explicit author tools. Nil disables them.
	Development *Development
}

// Development configures author-driven runs. Scripts and Start use a temporary
// save directory unless Config.SaveDir is explicit; normal player saves are isolated.
// All writers remain owned by the caller and are never closed by the game.
type Development struct {
	// Start skips the title and starts at this location ID.
	Start string
	// Script drives input; see docs/development/graphics.md for its grammar.
	Script string
	// Shots is the output directory required for scripted screenshots.
	Shots string
	// Trace receives JSON lines; nil disables tracing.
	Trace io.Writer
	// Notes is an explicit directory for in-game notes; empty disables the note key.
	Notes string
	// CollectBuildInfo opts into embedded revision metadata in notes.
	CollectBuildInfo bool
	// ExternalInput permits Linux zenity; unavailable hosts use in-window input.
	ExternalInput bool
}

func options(fsys fs.FS, cfg Config) runtime.Options {
	o := runtime.Options{FS: fsys, Seed: cfg.Seed, SaveDir: cfg.SaveDir, NoSound: cfg.Mute, Window: cfg.WindowScale, Fullscreen: cfg.Fullscreen}
	if d := cfg.Development; d != nil {
		o.Development = true
		o.Location, o.Script, o.ShotDir, o.Trace, o.MemoDir = d.Start, d.Script, d.Shots, d.Trace, d.Notes
		o.CollectBuildInfo, o.ExternalInput = d.CollectBuildInfo, d.ExternalInput
	}
	return o
}

// Run loads game.yaml and data/ from fsys and runs the screen on the main thread.
// Cancellation returns ctx.Err; a player closing the window is success. The FS
// is borrowed, never closed or modified. Ebitengine allows one Run per process.
func Run(ctx context.Context, fsys fs.FS, cfg Config) error {
	return runtime.Run(ctx, options(fsys, cfg))
}

// Game lets a host compose Gozen's screen into its own ebiten.Game. It is confined
// to the game thread. Only one Game may be open at a time; always call Close.
type Game struct{ game *runtime.Game }

// New loads a game without starting the loop. No window loop is started, but the
// process must have a graphics environment. The caller retains ownership of fsys.
func New(fsys fs.FS, cfg Config) (*Game, error) {
	g, err := runtime.New(options(fsys, cfg))
	if err != nil {
		return nil, err
	}
	return &Game{game: g}, nil
}

// Update advances input and gameplay. Normal script completion returns
// ebiten.Termination; I/O failures return their underlying error.
func (g *Game) Update() error { return g.game.Update() }

// Draw renders the current screen. Call only from the host's Draw method.
func (g *Game) Draw(dst *ebiten.Image) { g.game.Draw(dst) }

// Layout returns the game's configured logical framebuffer size.
func (g *Game) Layout(w, h int) (int, int) { return g.game.Layout(w, h) }

// Close releases resources and flushes read progress, reporting write failures.
// It is idempotent and must be called after the loop stops, on the game thread.
func (g *Game) Close() error { return g.game.Close() }
