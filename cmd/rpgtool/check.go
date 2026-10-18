package main

import (
	"io"
	"os"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
)

func (c *command) check(dir string) error {
	return c.checkTo(dir, c.out)
}

func (c *command) checkTo(dir string, out io.Writer) error {
	fsys := os.DirFS(dir)
	db, err := checkdata.Load(fsys, out, c.stderr)
	if err != nil {
		return err
	}
	return layoutMessages(db, fsys, c.stderr)
}
