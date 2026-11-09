package gamepackage

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Windows checks sharing across all hard links to the same file. Cache readers
// permit deletion so a concurrent publisher can unlink its temporary name while
// installed readers continue using the immutable destination through this handle.
func openCacheFile(name string) (*os.File, error) {
	full, err := filepath.Abs(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	// Extended absolute paths preserve ordinary os.Open's long-path support.
	if !strings.HasPrefix(full, `\\?\`) && !strings.HasPrefix(full, `\\.\`) {
		if strings.HasPrefix(full, `\\`) {
			full = `\\?\UNC\` + strings.TrimPrefix(full, `\\`)
		} else {
			full = `\\?\` + full
		}
	}
	path, err := syscall.UTF16PtrFromString(full)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return os.NewFile(uintptr(handle), name), nil
}
