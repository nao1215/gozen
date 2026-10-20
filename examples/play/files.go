// Command play runs Wayfinder, an original sample game: play.Main in a window, and
// play.New on a gozen.Session in a browser (GOOS=js).
package main

import (
	"embed"
	"io/fs"
	"testing/fstest"
)

// The game's YAML (game.yaml, data/) and its generated pictures (assets/) are
// embedded as they are laid out in a game directory. The game has no fonts in
// game.yaml, so its text is set in Gozen's built-in Go Regular.
//
//go:embed game
var files embed.FS

func gameFS() (fstest.MapFS, error) {
	sub, err := fs.Sub(files, "game")
	if err != nil {
		return nil, err
	}
	fsys := fstest.MapFS{}
	err = fs.WalkDir(sub, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, name)
		if err != nil {
			return err
		}
		fsys[name] = &fstest.MapFile{Data: b}
		return nil
	})
	return fsys, err
}
