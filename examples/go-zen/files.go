package main

import (
	"embed"
	"io/fs"
)

// The game's YAML (game.yaml, data/) and its pictures, music and font (assets/) as they
// are laid out in a game directory.
//
//go:embed game
var files embed.FS

func gameFS() (fs.FS, error) { return fs.Sub(files, "game") }
