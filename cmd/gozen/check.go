package main

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
	"github.com/nao1215/gozen/internal/diag"
)

// gameFS opens the game directory dir. When dir has no readable game.yaml it
// fails with GZN1110 and names the file it looked for, so the message says
// which directory was given.
func gameFS(dir string) (fs.FS, error) {
	name := filepath.Join(dir, "game.yaml")
	f, err := os.Open(name) //nolint:gosec // The game directory is the command's operand.
	if err == nil {
		var st fs.FileInfo
		st, err = f.Stat()
		if err == nil && st.IsDir() {
			err = &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		return nil, diag.GameUnreadable.Wrap(err)
	}
	return os.DirFS(dir), nil
}

func (c *command) check(dir string) error {
	return c.checkTo(dir, c.out)
}

// checkStructure checks without fonts or text layout, so games that have no
// fonts yet can still validate definitions, references, expressions, and assets.
func (c *command) checkStructure(dir string) error {
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	if _, err := checkdata.Load(fsys, c.out, c.stderr); err != nil {
		return err
	}
	c.out.Println("Structure checked; layout was not checked. Use gozen check --only layout <game-dir>.")
	return nil
}

// checkTo checks the structure and then the text layout. The summary of the
// structure goes to out only when the whole check passes, so a failing check
// never prints "ok".
func (c *command) checkTo(dir string, out io.Writer) error {
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	var summary bytes.Buffer
	db, err := checkdata.Load(fsys, &summary, c.stderr)
	if err != nil {
		return err
	}
	if err := layoutMessages(db, fsys, c.stderr); err != nil {
		return err
	}
	_, err = out.Write(summary.Bytes())
	return err
}
