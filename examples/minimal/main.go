// Command minimal loads a tiny embedded game without a graphics display.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/rpg"
)

//go:embed game.yaml data/*.yaml
var files embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err) //nolint:errcheck // The process already reports failure even if stderr cannot be written.
		os.Exit(1)
	}
}

func run() error {
	db, err := data.Load(files, ".")
	if err != nil {
		return err
	}
	state := rpg.New(db, nil)
	if err := headless.New(db, state).Start(); err != nil {
		return err
	}
	_, err = fmt.Printf("location=%s gold=%d\n", state.Location, state.Gold)
	return err
}
