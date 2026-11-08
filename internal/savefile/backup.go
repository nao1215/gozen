package savefile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WritePreserving keeps the original file as a backup named with a content fingerprint, then calls Write.
// If the backup already exists, its contents are verified. If backup fails, the original file is not replaced.
func WritePreserving(path string, b []byte) (string, error) {
	return writePreserving(path, b, os.ReadFile, Write, createBackup)
}

// WriteStorePreserving is the player's shared backup policy for directory,
// memory and browser saves. Only a missing backup may be created; unreadable
// or mismatched backups leave both files intact. Directory creation is exclusive.
// Like Store, callers serialize writes to a save.
func WriteStorePreserving(st Store, name string, b []byte) (string, error) {
	var create func(string, []byte) error
	if d, ok := st.(dirStore); ok {
		create = func(name string, b []byte) error { return createBackup(filepath.Join(string(d), name), b) }
	} else {
		create = st.Write
	}
	return writePreserving(name, b, st.Read, st.Write, create)
}

func writePreserving(name string, b []byte, read func(string) ([]byte, error), write, create func(string, []byte) error) (string, error) {
	old, err := read(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", write(name, b)
	}
	if err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.before-%x.bak", name, sha256.Sum256(old))
	saved, err := read(backup)
	if errors.Is(err, fs.ErrNotExist) {
		err = create(backup, old)
		if errors.Is(err, fs.ErrExist) {
			saved, err = read(backup)
		} else if err == nil {
			saved = old
		}
	}
	if err != nil {
		return "", err
	}
	if !bytes.Equal(saved, old) {
		return "", fmt.Errorf("backup content mismatch: %s", backup)
	}
	return backup, write(name, b)
}

func createBackup(path string, b []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // Caller-owned save path; exclusive creation preserves existing backups.
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			err = errors.Join(err, os.Remove(path))
		}
	}()
	if _, err = f.Write(b); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
