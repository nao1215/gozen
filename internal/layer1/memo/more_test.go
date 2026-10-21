package memo

import (
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextAndWriteAtRejectBadInput(t *testing.T) {
	if _, err := Next("", &Memo{}); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("an empty root: %v", err)
	}
	if _, err := Next(t.TempDir(), nil); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("a nil memo: %v", err)
	}
	if _, err := Write(t.TempDir(), nil, nil, nil); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("a nil memo is invalid: %v", err)
	}
	if _, err := WriteAt(filepath.Join(t.TempDir(), "001"), nil, nil, nil); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("WriteAt with a nil memo: %v", err)
	}
	m := &Memo{Time: "yesterday"}
	if _, err := Next(t.TempDir(), m); err == nil {
		t.Error("a time that is not RFC 3339 was accepted")
	}
}

// TestNextNumbersWithinTheDay: reservations are numbered 001, 002 ... in a folder of the note's day (taken from its Time).
func TestNextNumbersWithinTheDay(t *testing.T) {
	root := t.TempDir()
	m := &Memo{Time: "2026-03-04T10:00:00+09:00"}
	a, err := Next(root, m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Next(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(a) != "001" || filepath.Base(b) != "002" || filepath.Base(filepath.Dir(a)) != "2026-03-04" {
		t.Errorf("%s %s", a, b)
	}
	m2 := &Memo{}
	c, err := Next(root, m2)
	if err != nil || m2.Time == "" || c == "" {
		t.Errorf("an empty time is filled in: %q %v", m2.Time, err)
	}
}

func TestNextFailsWhereTheRootIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Next(file, &Memo{}); err == nil {
		t.Error("a file as the notes folder was accepted")
	}
}

// TestWriteAtCleansUpWhenAnArtifactCannotBeCreated: a shot whose file already exists fails, and the reservation is removed.
func TestWriteAtCleansUpWhenAnArtifactCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	base, err := Next(root, &Memo{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".png", []byte("taken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAt(base, &Memo{}, image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil); err == nil {
		t.Fatal("overwrote an existing picture")
	}
	if _, err := os.Stat(base + ".json"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the failed reservation was left behind")
	}
	if b, _ := os.ReadFile(base + ".png"); string(b) != "taken" { //nolint:gosec // Path is isolated in t.TempDir.
		t.Error("a file that was not ours was changed")
	}
}

func TestWriteAtCleansUpWhenTheSaveCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	base, err := Next(root, &Memo{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(base+".save", 0o700); err != nil { // a directory where the save goes
		t.Fatal(err)
	}
	if _, err := WriteAt(base, &Memo{}, image.NewNRGBA(image.Rect(0, 0, 1, 1)), []byte("s")); err == nil {
		t.Fatal("wrote a save over a directory")
	}
	if _, err := os.Stat(base + ".png"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the picture made before the failure was not removed")
	}
	if _, err := os.Stat(base + ".json"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the reservation was not removed")
	}
}

func TestReadRejectsBadNotes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, _, err := Read(write("a.json", "{not json")); err == nil || !strings.Contains(err.Error(), "a.json") {
		t.Errorf("broken JSON names the file: %v", err)
	}
	if _, _, err := Read(write("b.json", `{"text":"no save"}`)); err == nil || !strings.Contains(err.Error(), "no save") {
		t.Errorf("a note without a save: %v", err)
	}
	for _, name := range []string{"../escape.save", "sub/dir.save", `a\b.save`, "/abs.save"} {
		p := write("c.json", `{"save":`+quote(name)+`}`)
		if _, _, err := Read(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("a save name %q that leaves the folder: %v", name, err)
		}
	}
	if _, _, err := Read(filepath.Join(dir, "missing.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing note: %v", err)
	}
	// a .save path with no .json beside it reads the save alone
	save := write("d.save", "bytes")
	m, b, err := Read(save)
	if err != nil || string(b) != "bytes" || m.Text != "" {
		t.Errorf("a save alone: %+v %q %v", m, b, err)
	}
	if _, _, err := Read(filepath.Join(dir, "gone.save")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing save alone: %v", err)
	}
	// a .save path with a broken .json beside it
	write("e.json", "[")
	write("e.save", "x")
	if _, _, err := Read(filepath.Join(dir, "e.save")); err == nil {
		t.Error("a broken json beside a save was ignored")
	}
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return `"` + s + `"`
}

func TestAllListsNotesInOrderAndFillsTheSource(t *testing.T) {
	root := t.TempDir()
	for _, m := range []*Memo{
		{Time: "2026-01-02T00:00:00Z", Text: "second day"},
		{Time: "2026-01-01T00:00:00Z", Text: "first day", Source: LLM},
		{Time: "2026-01-01T00:00:00Z", Text: "first day again"},
	} {
		if _, err := Write(root, m, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	// an empty file and a non-note file are skipped
	if err := os.WriteFile(filepath.Join(root, "2026-01-01", "009.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	var ids, texts []string
	for _, e := range all {
		ids = append(ids, e.ID)
		texts = append(texts, e.Text)
		if e.Source == "" {
			t.Errorf("%s has no source", e.ID)
		}
	}
	if strings.Join(ids, ",") != "2026-01-01/001,2026-01-01/002,2026-01-02/001" || texts[0] != "first day" {
		t.Errorf("%v %v", ids, texts)
	}
	if all[0].Source != LLM || all[1].Source != Human {
		t.Errorf("a note without a source is the human's: %q %q", all[0].Source, all[1].Source)
	}
	if err := os.WriteFile(filepath.Join(root, "2026-01-01", "010.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := All(root); err == nil {
		t.Error("a broken note was skipped silently")
	}
	if got, err := All(filepath.Join(root, "nothing")); err != nil || len(got) != 0 {
		t.Errorf("no folder means no notes: %v %v", got, err)
	}
}

func TestVersionIsOffUnlessAsked(t *testing.T) {
	if Version(false) != "" {
		t.Error("version must be off by default")
	}
	v := Version(true)
	if strings.ContainsAny(v, " \n") {
		t.Errorf("version %q should be a bare revision", v)
	}
}
