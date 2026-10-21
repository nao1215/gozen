package gozen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nao1215/gozen/internal/savefile"
)

// Store keeps saves by name. A session's saves are written one at a time; a Store does
// not guard one place shared by several processes.
type Store interface {
	// A save's name is one plain file name that does not start with a dot; another is an
	// error that is ErrInvalid.
	//
	// Read returns the save; a missing one is an error that is fs.ErrNotExist.
	Read(name string) ([]byte, error)
	// Write replaces the save whole: a failed write keeps the old one.
	Write(name string, b []byte) error
	// List is the names of the saves, sorted.
	List() ([]string, error)
	// Remove deletes the save; a missing one is not an error.
	Remove(name string) error
}

// validName accepts one plain file name. A name that starts with a dot is refused: it would
// be a hidden file, which DirStore.List leaves out, as it does the temporary files of a write.
func validName(name string) error {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) || !fs.ValidPath(name) {
		return fmt.Errorf("%w: save name %q", ErrInvalid, name)
	}
	return nil
}

// DirStore keeps saves as files in a directory of the OS, made when first written.
func DirStore(dir string) Store { return dirStore(dir) }

type dirStore string

func (d dirStore) Read(name string) ([]byte, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(string(d), name)) //nolint:gosec // The caller owns the directory; name is a plain file name (validName).
}

func (d dirStore) Write(name string, b []byte) error {
	if err := validName(name); err != nil {
		return err
	}
	return savefile.Write(filepath.Join(string(d), name), b)
}

func (d dirStore) List() ([]string, error) {
	entries, err := os.ReadDir(string(d))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// ModTime is when the save was last written (for a screen that shows it).
func (d dirStore) ModTime(name string) (time.Time, error) {
	if err := validName(name); err != nil {
		return time.Time{}, err
	}
	fi, err := os.Stat(filepath.Join(string(d), name))
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}

func (d dirStore) Remove(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(string(d), name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// MemStore keeps saves in memory: for tests and scripted plays.
func MemStore() Store { return &memStore{saves: map[string][]byte{}, times: map[string]time.Time{}} }

type memStore struct {
	mu    sync.Mutex
	saves map[string][]byte
	times map[string]time.Time
}

// ModTime is when the save was last written.
func (m *memStore) ModTime(name string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.times[name]
	if !ok {
		return time.Time{}, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return t, nil
}

func (m *memStore) Read(name string) ([]byte, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.saves[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), b...), nil
}

func (m *memStore) Write(name string, b []byte) error {
	if err := validName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves[name] = append([]byte(nil), b...)
	m.times[name] = time.Now()
	return nil
}

func (m *memStore) List() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.saves))
	for k := range m.saves {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func (m *memStore) Remove(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.saves, name)
	delete(m.times, name)
	return nil
}
