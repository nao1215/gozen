package play

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"
)

// memKV is a browser's key/value storage in memory; full refuses each set after the first
// n (a quota running out).
type memKV struct {
	m    map[string]string
	sets int
	full int
}

func (k *memKV) get(key string) (string, bool, error) { v, ok := k.m[key]; return v, ok, nil }
func (k *memKV) remove(key string) error              { delete(k.m, key); return nil }
func (k *memKV) keys() ([]string, error)              { return slices.Sorted(mapsKeys(k.m)), nil }
func (k *memKV) set(key, value string) error {
	k.sets++
	if k.full > 0 && k.sets > k.full {
		return errors.New("quota exceeded")
	}
	k.m[key] = value
	return nil
}

func mapsKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// The browser's store keeps a save and its time in one value: a write either replaces
// both or fails leaving the old save; saves written by an older version, with the time
// under a key of its own, still read.
func TestKVStoreWritesOnce(t *testing.T) {
	kv := &memKV{m: map[string]string{}}
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := kvStore{kv: kv, prefix: "gozen/x/", now: func() time.Time { return clock }}
	if err := s.Write("slot1", []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	kv.full = kv.sets + 1 // one more set goes in, then the storage is full
	clock = clock.Add(time.Hour)
	werr := s.Write("slot1", []byte(`{"a":2}`))
	want, wantTime := `{"a":2}`, clock
	if werr != nil { // a failed write keeps the old save, its time too
		want, wantTime = `{"a":1}`, clock.Add(-time.Hour)
	}
	if b, err := s.Read("slot1"); err != nil || string(b) != want {
		t.Fatalf("after the write (%v): %q %v, want %q", werr, b, err, want)
	}
	if mt, err := s.ModTime("slot1"); err != nil || !mt.Equal(wantTime) {
		t.Fatalf("time %v %v, want %v", mt, err, wantTime)
	}
	// an older version's save: the body as it is, the time under name#time
	kv.full = 0
	kv.m["gozen/x/slot2"], kv.m["gozen/x/slot2#time"] = `{"b":1}`, "2025-05-06T07:08:09Z"
	if b, err := s.Read("slot2"); err != nil || string(b) != `{"b":1}` {
		t.Fatalf("old save: %q %v", b, err)
	}
	if mt, err := s.ModTime("slot2"); err != nil || mt.Year() != 2025 {
		t.Fatalf("old save's time: %v %v", mt, err)
	}
	if names, err := s.List(); err != nil || strings.Join(names, ",") != "slot1,slot2" {
		t.Fatalf("list %v %v", names, err)
	}
	if err := s.Remove("slot2"); err != nil || len(kv.m) != 1 {
		t.Fatalf("remove left %v (%v)", kv.m, err)
	}
	if _, err := s.Read("slot2"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a removed save reads %v", err)
	}
}
