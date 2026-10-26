package play

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
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

// kv is a browser's key/value storage (localStorage), its values strings.
type kv interface {
	get(key string) (string, bool, error)
	set(key, value string) error
	remove(key string) error
	keys() ([]string, error)
}

// kvStore keeps saves in a kv: a save and the time it was written in one value under
// prefix+name, so that a write is one set that either replaces both or fails and keeps
// the old save. An older version kept the time under prefix+name+"#time"; such saves
// still read.
type kvStore struct {
	kv     kv
	prefix string
	now    func() time.Time
}

// kvTime begins a value that carries its time: the mark, the time, a newline, the save.
const kvTime = "gozen-time "

const oldTimeKey = "#time"

// unpack splits a value into its save and its time (zero for an older value).
func unpack(v string) ([]byte, time.Time) {
	head, body, ok := strings.Cut(v, "\n")
	if rest, marked := strings.CutPrefix(head, kvTime); ok && marked {
		if t, err := time.Parse(time.RFC3339, rest); err == nil {
			return []byte(body), t
		}
	}
	return []byte(v), time.Time{}
}

func (s kvStore) Read(name string) ([]byte, error) {
	v, ok, err := s.kv.get(s.prefix + name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	b, _ := unpack(v)
	return b, nil
}

func (s kvStore) Write(name string, b []byte) error {
	// An older version's time key may stay; the value's own time is read first, and Remove
	// takes both.
	return s.kv.set(s.prefix+name, kvTime+s.now().UTC().Format(time.RFC3339)+"\n"+string(b))
}

func (s kvStore) List() ([]string, error) {
	keys, err := s.kv.keys()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range keys {
		if name, ok := strings.CutPrefix(k, s.prefix); ok && !strings.HasSuffix(name, oldTimeKey) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s kvStore) Remove(name string) error {
	if err := s.kv.remove(s.prefix + name); err != nil {
		return err
	}
	return s.kv.remove(s.prefix + name + oldTimeKey)
}

// ModTime is the time the save was written.
func (s kvStore) ModTime(name string) (time.Time, error) {
	v, ok, err := s.kv.get(s.prefix + name)
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, fs.ErrNotExist
	}
	if _, t := unpack(v); !t.IsZero() {
		return t, nil
	}
	old, ok, err := s.kv.get(s.prefix + name + oldTimeKey)
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, fs.ErrNotExist
	}
	return time.Parse(time.RFC3339, old)
}
