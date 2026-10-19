package main

import (
	"io"
	"os"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
)

func (c *command) check(dir string) error {
	return c.checkTo(dir, c.out)
}

// checkStructure checks without fonts or text layout, so games that have no
// fonts yet can still validate definitions, references, expressions, and assets.
func (c *command) checkStructure(dir string) error {
	if _, err := checkdata.Load(os.DirFS(dir), c.out, c.stderr); err != nil {
		return err
	}
	c.out.Println("Structure checked; layout was not checked. Use gozen layout <game-dir>.")
	return nil
}

func (c *command) checkTo(dir string, out io.Writer) error {
	fsys := os.DirFS(dir)
	db, err := checkdata.Load(fsys, out, c.stderr)
	if err != nil {
		return err
	}
	return layoutMessages(db, fsys, c.stderr)
}
