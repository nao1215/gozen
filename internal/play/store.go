package play

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/nao1215/gozen/internal/savefile"

	"github.com/nao1215/gozen/internal/rpg"
)

// Store is where saves are kept (same shape as savefile.Store): an OS directory, memory, or the browser's localStorage.
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, b []byte) error
	List() ([]string, error)
	Remove(name string) error
}

// openStore decides the location from Options. Set given to true when it is explicit (Store, SaveDir) or a trial play such as a script
// (memory that keeps no save).
func openStore(opts Options, id string, trial bool) (st Store, given bool, err error) {
	switch {
	case opts.Store != nil:
		return opts.Store, true, nil
	case opts.SaveDir != "":
		return savefile.DirStore(opts.SaveDir), true, nil
	case trial:
		return savefile.MemStore(), true, nil
	case opts.StoreFor != nil:
		st, err = opts.StoreFor(id)
		return st, false, err
	}
	st, err = defaultStore(id)
	return st, false, err
}

func (g *Game) storeRead(name string) ([]byte, error) { return g.store.Read(name) }

func (g *Game) storeHas(name string) bool {
	_, err := g.store.Read(name)
	return err == nil
}

// storeTime is the time the save was written (if the location knows it).
func (g *Game) storeTime(name string) (time.Time, bool) {
	if m, ok := g.store.(interface {
		ModTime(name string) (time.Time, error)
	}); ok {
		if t, err := m.ModTime(name); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// writePreserving keeps the current contents as a fingerprinted backup (<name>.before-<sha256>.bak) before writing.
func (g *Game) writePreserving(name string, b []byte) (string, error) {
	old, err := g.store.Read(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", g.store.Write(name, b)
	}
	if err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.before-%x.bak", name, sha256.Sum256(old))
	if saved, err := g.store.Read(backup); err == nil {
		if !bytes.Equal(saved, old) {
			return "", fmt.Errorf("backup content mismatch: %s", backup)
		}
	} else if err := g.store.Write(backup, old); err != nil {
		return "", err
	}
	return backup, g.store.Write(name, b)
}

// saveState writes the state. If the current contents are unreadable or in an old format needing correction, it keeps a backup.
func (g *Game) saveState(name string) (string, error) {
	b, err := g.saveBytes()
	if err != nil {
		return "", err
	}
	old, err := g.store.Read(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err == nil {
		previous, loadErr := rpg.ParseWithConfig(g.db, old, nil, rpg.Config{AllowLegacy: true})
		if loadErr != nil || previous.LoadReport.Changed() {
			return g.writePreserving(name, b)
		}
	}
	return "", g.store.Write(name, b)
}
