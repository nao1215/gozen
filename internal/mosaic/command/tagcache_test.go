package command

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func taggerDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "model.onnx"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTagCacheTagsOnceAndSaves(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	m.tagger = &fakeTagger{tags: map[string]float32{"general": 0.9}}
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	args := []string{"auto", "--model", model, "--tagger", taggerDir(t), "--tags", tags, "-o", out, game, "pattern.png"}
	if code, _, diag := invoke(t, args...); code != 0 {
		t.Fatal(diag)
	}
	if m.tagger.calls != 1 || m.tagger.closed != 1 {
		t.Fatalf("tagger calls %d closed %d", m.tagger.calls, m.tagger.closed)
	}
	b := readFile(t, tags)
	var cache map[string]tagEntry
	if err := json.Unmarshal(b, &cache); err != nil || cache["pattern.png"].Tags["general"] != 0.9 || cache["pattern.png"].Stamp == "" {
		t.Fatalf("cache %s %v", b, err)
	}
	if !readEntries(t, out)[0].Tagged {
		t.Fatal("the entry does not say it was tagged")
	}
	// Run again: the cache answers, the model is not used.
	if code, _, diag := invoke(t, args...); code != 0 {
		t.Fatal(diag)
	}
	if m.tagger.calls != 1 {
		t.Fatalf("a cached picture was tagged again (%d calls)", m.tagger.calls)
	}
	// The picture changes: its stamp no longer matches, so it is tagged again.
	p := filepath.Join(game, "assets", "pattern.png")
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, time.Now(), st.ModTime().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if code, _, diag := invoke(t, args...); code != 0 {
		t.Fatal(diag)
	}
	if m.tagger.calls != 2 {
		t.Fatalf("a changed picture kept its old tags (%d calls)", m.tagger.calls)
	}
}

func TestTagCacheReadOnlyCommandsDoNotTagOrWrite(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	m.tagger = &fakeTagger{tags: map[string]float32{"general": 0.9}}
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	dir := taggerDir(t)
	for _, args := range [][]string{
		{"auto", "-n", "--model", model, "--tagger", dir, "--tags", tags, "-o", out, game, "pattern.png"},
		{"inspect", "--model", model, "--tagger", dir, "--tags", tags, "-o", out, game, "pattern.png"},
	} {
		if code, _, diag := invoke(t, args...); code != 0 {
			t.Fatalf("%v: %s", args, diag)
		}
		if m.tagger.calls != 0 || slices.Contains(m.opened, dir) {
			t.Fatalf("%v: a read-only command used the tagger (%d calls, opened %v)", args, m.tagger.calls, m.opened)
		}
		if _, err := os.Stat(tags); !os.IsNotExist(err) {
			t.Fatalf("%v: wrote the tag cache", args)
		}
	}
}

func TestTagCacheWithoutATaggerModel(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	// --tags without --tagger and no model.onnx in the working directory: warn, go on without tags.
	code, _, diag := invoke(t, "auto", "--model", model, "--tags", tags, "-o", out, game, "pattern.png")
	if code != 0 || !strings.Contains(diag, "タガーのモデル") {
		t.Fatalf("code %d: %q", code, diag)
	}
	e := readEntries(t, out)[0]
	if e.Tagged || len(e.Review) != 1 || !strings.Contains(e.Review[0], "タグが無い") {
		t.Fatalf("entry %+v", e)
	}
	// A checked picture does not need tags.
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := os.WriteFile(fixes, []byte("pattern.png: {checked: true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join(filepath.Dir(out), "out2")
	if code, _, diag := invoke(t, "auto", "--model", model, "--tags", tags, "--fix", fixes, "-o", out2, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	if e := readEntries(t, out2)[0]; len(e.Review) != 0 {
		t.Fatalf("review of a checked picture: %v", e.Review)
	}
}

func TestTagCacheErrors(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	// A tagger directory without model.onnx is an error, not a warning.
	code, _, diag := invoke(t, "auto", "--model", model, "--tagger", t.TempDir(), "--tags", tags, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "tagger model") {
		t.Fatalf("code %d: %s", code, diag)
	}
	// A tagger that cannot be opened.
	code, _, diag = invoke(t, "auto", "--model", model, "--tagger", taggerDir(t), "--tags", tags, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "fake: no tagger") {
		t.Fatalf("code %d: %s", code, diag)
	}
	// A tagger that fails on a picture.
	m.tagger = &fakeTagger{err: errors.New("the tagger broke")}
	code, _, diag = invoke(t, "auto", "--model", model, "--tagger", taggerDir(t), "--tags", tags, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "the tagger broke") {
		t.Fatalf("code %d: %s", code, diag)
	}
	// A cache file that is not JSON.
	if err := os.WriteFile(tags, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, diag = invoke(t, "auto", "--model", model, "--tags", tags, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "tags.json") {
		t.Fatalf("code %d: %s", code, diag)
	}
	// A cache path that is a directory cannot be read.
	if code, _, _ := invoke(t, "auto", "--model", model, "--tags", t.TempDir(), "-o", out, game, "pattern.png"); code != 1 {
		t.Fatalf("a directory as the tag cache gave %d", code)
	}
}

func TestTagEvidenceAloneAsksForReview(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx") // finds nothing
	m.tagger = &fakeTagger{tags: map[string]float32{"penis": 0.9, "explicit": 0.8}}
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	code, _, diag := invoke(t, "auto", "--model", model, "--tagger", taggerDir(t), "--tags", tags, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e := readEntries(t, out)[0]
	if e.Output != "" || len(e.Review) != 1 || !strings.Contains(e.Review[0], "tag evidence requires manual review") {
		t.Fatalf("tags saw a penis the detector did not: %+v", e)
	}
}

func TestTagCacheUnits(t *testing.T) {
	var nilCache *tagCache
	if tags, err := nilCache.get("a.png", "a.png"); tags != nil || err != nil {
		t.Fatal("a nil cache has no tags")
	}
	if err := nilCache.save(); err != nil || nilCache.close() != nil {
		t.Fatal("a nil cache saves and closes quietly")
	}
	tc, err := loadTagCache("", "", "", 1)
	if err != nil || tc.path != "" {
		t.Fatal(err)
	}
	if tags, err := tc.get("a.png", "a.png"); tags != nil || err != nil {
		t.Fatal("no cache path means no tags")
	}
	if _, err := stamp(filepath.Join(t.TempDir(), "none")); err == nil {
		t.Fatal("stamp of a missing file")
	}
	// A file outside the assets is keyed by its absolute path.
	dir := t.TempDir()
	pic := filepath.Join(dir, "x.png")
	if err := os.WriteFile(pic, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "tags.json")
	tc, err = loadTagCache(file, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	st, err := stamp(pic)
	if err != nil {
		t.Fatal(err)
	}
	tc.entries[pic] = tagEntry{Stamp: st, Tags: map[string]float32{"k": 0.5}}
	got, err := tc.get("", pic)
	if err != nil || got["k"] != 0.5 {
		t.Fatalf("keyed by absolute path: %v %v", got, err)
	}
	if _, err := tc.get("x.png", filepath.Join(dir, "gone.png")); err == nil {
		t.Fatal("a missing picture has no stamp")
	}
	// A cache with nothing new is not rewritten.
	if err := tc.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("an unchanged cache was written")
	}
}
