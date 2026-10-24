package command

import (
	"context"
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
)

// publishManual stages derived outputs and the fix document before replacing
// either. A model or rendering failure leaves the previous files untouched.
func publishManual(ctx context.Context, c config, pic, orig, sub string, size, step int) (err error) {
	parent := filepath.Dir(c.out)
	if err = os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".mosaic-stage-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	if st, statErr := os.Stat(c.out); statErr == nil {
		if !st.IsDir() {
			return errors.New("output is not a directory")
		}
		if err = fs.WalkDir(os.DirFS(c.out), ".", func(_ string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("output tree must not contain symlinks")
			}
			return nil
		}); err != nil {
			return err
		}
		if err = os.CopyFS(stage, os.DirFS(c.out)); err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	originalOut := c.out
	c.displayOut = originalOut
	c.out = stage
	if _, err = run(ctx, c, []string{pic}); err != nil {
		return err
	}
	if sub != "check" {
		if _, err = manualView(c, pic, orig, "masked", image.Rectangle{}, size, step, ""); err != nil {
			return err
		}
	}
	if c.outw.Err != nil || c.diag.Err != nil {
		return errors.Join(c.outw.Err, c.diag.Err)
	}
	temp, err := os.CreateTemp(filepath.Dir(c.opts.manualFile), ".mosaic-fix-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	if err = temp.Close(); err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(tempName); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	if err = c.manual.Save(tempName, "gozen mosaic: asset-relative paths, original image pixel coordinates."); err != nil {
		return err
	}
	backup := stage + "-previous"
	hadOld := false
	if _, e := os.Stat(originalOut); e == nil {
		if err = os.Rename(originalOut, backup); err != nil {
			return err
		}
		hadOld = true
	}
	if err = os.Rename(stage, originalOut); err != nil {
		if hadOld {
			err = errors.Join(err, os.Rename(backup, originalOut))
		}
		return err
	}
	if err = os.Rename(tempName, c.opts.manualFile); err != nil {
		err = errors.Join(err, os.RemoveAll(originalOut))
		if hadOld {
			err = errors.Join(err, os.Rename(backup, originalOut))
		}
		return err
	}
	if hadOld {
		err = os.RemoveAll(backup)
	}
	return err
}
