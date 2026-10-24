package savefile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/savefile"
)

type modTimer interface {
	ModTime(name string) (time.Time, error)
}

// TestStoresRefuseUnsafeNames: a save name is one plain file name; nothing else touches the disk or the memory.
func TestStoresRefuseUnsafeNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "saves")
	for kind, st := range map[string]savefile.Store{"dir": savefile.DirStore(dir), "mem": savefile.MemStore()} {
		// A name that starts with a dot is a hidden file, which DirStore.List leaves out (and the
		// temporary files of a write are such files): neither store takes one.
		for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/abs", "a//b", "セーブ/1", ".hidden", ".save-1.tmp"} {
			if _, err := st.Read(bad); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("%s Read(%q): %v", kind, bad, err)
			}
			if err := st.Write(bad, []byte("x")); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("%s Write(%q): %v", kind, bad, err)
			}
			if err := st.Remove(bad); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("%s Remove(%q): %v", kind, bad, err)
			}
			if mt, ok := st.(modTimer); ok {
				if _, err := mt.ModTime(bad); !errors.Is(err, fs.ErrInvalid) {
					t.Errorf("%s ModTime(%q): %v", kind, bad, err)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Dir(dir)); err != nil || len(entries) != 0 {
		t.Errorf("a refused name created %v", entries)
	}
}

func TestStoreNamesWithUnicodeAreFine(t *testing.T) {
	for kind, st := range map[string]savefile.Store{"dir": savefile.DirStore(t.TempDir()), "mem": savefile.MemStore()} {
		for _, name := range []string{"セーブ1", "スロット 2", "é", "😀", "a.b", "a b"} {
			if err := st.Write(name, []byte(name)); err != nil {
				t.Errorf("%s Write(%q): %v", kind, name, err)
				continue
			}
			if b, err := st.Read(name); err != nil || string(b) != name {
				t.Errorf("%s Read(%q) = %q %v", kind, name, b, err)
			}
		}
		names, err := st.List()
		if err != nil || len(names) != 6 || !slices.IsSorted(names) {
			t.Errorf("%s List = %v %v", kind, names, err)
		}
	}
}

func TestStoreModTime(t *testing.T) {
	for kind, st := range map[string]savefile.Store{"dir": savefile.DirStore(t.TempDir()), "mem": savefile.MemStore()} {
		mt, ok := st.(modTimer)
		if !ok {
			t.Fatalf("%s has no ModTime", kind)
		}
		if _, err := mt.ModTime("one"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: a missing save has no time: %v", kind, err)
		}
		before := time.Now().Add(-2 * time.Second)
		if err := st.Write("one", []byte("a")); err != nil {
			t.Fatal(err)
		}
		got, err := mt.ModTime("one")
		if err != nil || got.Before(before) || got.After(time.Now().Add(2*time.Second)) {
			t.Errorf("%s: ModTime %v %v", kind, got, err)
		}
		if err := st.Remove("one"); err != nil {
			t.Fatal(err)
		}
		if _, err := mt.ModTime("one"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: a removed save has no time: %v", kind, err)
		}
	}
}

func TestDirStoreListSkipsTemporaryAndDirectories(t *testing.T) {
	dir := t.TempDir()
	st := savefile.DirStore(dir)
	if err := st.Write("one", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".save-123.tmp"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := st.List()
	if err != nil || !slices.Equal(names, []string{"one"}) {
		t.Errorf("%v %v", names, err)
	}
	// a directory that is a file cannot be listed
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Windows reports a file used as a directory as a missing path, so it lists as empty there.
	if _, err := savefile.DirStore(file).List(); err == nil && runtime.GOOS != "windows" {
		t.Error("listing a file")
	}
	if err := savefile.DirStore(file).Write("x", []byte("a")); err == nil {
		t.Error("writing below a file")
	}
}

func TestMemStoreCopiesWhatItKeeps(t *testing.T) {
	st := savefile.MemStore()
	in := []byte("abc")
	if err := st.Write("s", in); err != nil {
		t.Fatal(err)
	}
	in[0] = 'X'
	got, err := st.Read("s")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Errorf("the store shares the caller's bytes: %q", got)
	}
	got[1] = 'Y'
	again, err := st.Read("s")
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != "abc" {
		t.Errorf("the store shares the returned bytes: %q", again)
	}
}

func TestStores(t *testing.T) {
	for name, st := range map[string]savefile.Store{"dir": savefile.DirStore(filepath.Join(t.TempDir(), "saves")), "mem": savefile.MemStore()} {
		t.Run(name, func(t *testing.T) {
			if _, err := st.Read("one"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			if names, err := st.List(); err != nil || len(names) != 0 {
				t.Fatal(names, err)
			}
			if err := st.Write("one", []byte("a")); err != nil {
				t.Fatal(err)
			}
			if err := st.Write("one", []byte("b")); err != nil {
				t.Fatal(err)
			}
			if b, err := st.Read("one"); err != nil || string(b) != "b" {
				t.Fatal(string(b), err)
			}
			if names, err := st.List(); err != nil || strings.Join(names, ",") != "one" {
				t.Fatal(names, err)
			}
			if err := st.Remove("one"); err != nil || st.Remove("one") != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{"", "../x", "a/b", "."} {
				if err := st.Write(bad, nil); !errors.Is(err, fs.ErrInvalid) {
					t.Errorf("%q: %v", bad, err)
				}
			}
		})
	}
}
