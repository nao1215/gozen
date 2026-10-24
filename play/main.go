package play

import (
	"context"
	"io/fs"

	runtime "github.com/nao1215/gozen/internal/play"
)

// Main runs the game in dir of fsys in a window until the player closes it: the
// standard options in args (GNU style; nil is none, os.Args is never read), --help on
// cfg.Output, and the author tools with --dev. Call it on the main thread; Ebitengine
// allows one game loop a process. Closing the window returns nil, a cancelled ctx its
// error. The library never exits the process: main decides the exit status.
func Main(ctx context.Context, fsys fs.FS, dir string, args []string, cfg Config) error {
	c := runtime.EntryConfig{Mute: cfg.Mute, Output: cfg.Output}
	if cfg.Store != nil {
		c.Store = func(id string) (runtime.Store, error) { return cfg.Store(id) }
	}
	return runtime.Main(ctx, fsys, dir, args, c)
}
