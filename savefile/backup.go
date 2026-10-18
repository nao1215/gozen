package savefile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
)

// WritePreserving は元ファイルを内容の指紋付きの控えに残してから Write する。
// 控えが既にあれば中身を照合する。控えの失敗時は元ファイルを置き換えない。
func WritePreserving(path string, b []byte) (savedPath string, err error) {
	old, err := os.ReadFile(path) //nolint:gosec // The caller explicitly owns and selects the save destination.
	if os.IsNotExist(err) {
		return "", Write(path, b)
	}
	if err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.before-%x.bak", path, sha256.Sum256(old))
	f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // The caller explicitly owns and selects the save destination.
	if os.IsExist(err) {
		saved, readErr := os.ReadFile(backup) //nolint:gosec // The caller explicitly owns and selects the save destination.
		if readErr != nil {
			return "", readErr
		}
		if !bytes.Equal(saved, old) {
			return "", fmt.Errorf("backup content mismatch: %s", backup)
		}
	} else if err != nil {
		return "", err
	} else {
		ok := false
		defer func() {
			if !ok {
				err = errors.Join(err, os.Remove(backup)) //nolint:gosec // Removes only the failed backup created for the caller-selected path.
			}
		}()
		if _, err = f.Write(old); err != nil {
			return "", errors.Join(err, f.Close())
		}
		if err = f.Sync(); err != nil {
			return "", errors.Join(err, f.Close())
		}
		if err = f.Close(); err != nil {
			return "", err
		}
		ok = true
	}
	if err := Write(path, b); err != nil {
		return backup, err
	}
	return backup, nil
}
