package mosaic

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// WriteFile replaces a selected output through a private temporary file. It
// never truncates a symlink or hard link target. The caller validates parents.
func WriteFile(name string, encode func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(name), ".mosaic-write-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if e := os.Remove(tmp); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	err = errors.Join(encode(f), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, name)
}
