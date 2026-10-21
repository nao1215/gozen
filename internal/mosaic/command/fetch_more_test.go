package command

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchRequiredFileMissingOnTheHub(t *testing.T) {
	cache, _ := fakeHub(t, false)
	models[0].Files = append(models[0].Files, fileSpec{Name: "extra.bin", SHA256: strings.Repeat("0", 64), Size: 1})
	code, _, diag := invoke(t, "fetch")
	if code != 1 || !strings.Contains(diag, "extra.bin") || !strings.Contains(diag, "download failed") {
		t.Fatalf("code %d: %s", code, diag)
	}
	if entries := readDir(t, cache); len(entries) != 0 {
		t.Fatalf("a failed download left %v", entries)
	}
}

func TestFetchServerErrorAndUnreachableHub(t *testing.T) {
	cache, _ := fakeHub(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	hubURL = srv.URL
	code, _, diag := invoke(t, "fetch", "censor")
	if code != 1 || !strings.Contains(diag, "500") {
		t.Fatalf("code %d: %s", code, diag)
	}
	srv.Close()
	code, _, diag = invoke(t, "fetch", "censor")
	if code != 1 || !strings.Contains(diag, "censor/") {
		t.Fatalf("unreachable hub: code %d: %s", code, diag)
	}
	if entries := readDir(t, cache); len(entries) != 0 {
		t.Fatalf("left %v", entries)
	}
}

func TestFetchSizeMismatch(t *testing.T) {
	fakeHub(t, false)
	models[0].Files[0].Size++
	code, _, diag := invoke(t, "fetch")
	if code != 1 || !strings.Contains(diag, "SHA-256 mismatch") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestFetchWithoutACacheDirectory(t *testing.T) {
	fakeHub(t, false)
	cacheRoot = func() (string, error) { return "", errors.New("no home") }
	code, _, diag := invoke(t, "fetch")
	if code != 1 || !strings.Contains(diag, "no cache directory") {
		t.Fatalf("code %d: %s", code, diag)
	}
	if _, ok := models[0].cachedModel(); ok {
		t.Fatal("a model is cached without a cache directory")
	}
	_, out, _ := invoke(t, "fetch", "--list")
	if !strings.Contains(out, "not fetched") {
		t.Fatalf("list %q", out)
	}
}

func TestCachedModelNeedsTheRightFiles(t *testing.T) {
	cache, _ := fakeHub(t, false)
	if code, _, diag := invoke(t, "fetch"); code != 0 {
		t.Fatal(diag)
	}
	m := models[0]
	if _, ok := m.cachedModel(); !ok {
		t.Fatal("a fetched model is not cached")
	}
	// A truncated file is not the model.
	p := filepath.Join(cache, "censor", "model.onnx")
	if err := os.WriteFile(p, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.cachedModel(); ok {
		t.Fatal("a file of the wrong size was accepted")
	}
	// A directory is not a file.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.cachedModel(); ok {
		t.Fatal("a directory was accepted")
	}
}

func TestDefaultModelWithoutAnAnswer(t *testing.T) {
	_, hits := fakeHub(t, false)
	interactive = func() bool { return true }
	c := &cli{diag: &output{Writer: &strings.Builder{}}}
	promptIn = strings.NewReader("")
	if _, err := c.defaultModel(false, true); err == nil || !strings.Contains(err.Error(), "no answer") || hits.Load() != 0 {
		t.Fatalf("end of input is not consent: %v (%d requests)", err, hits.Load())
	}
	promptIn = strings.NewReader("YES")
	if p, err := c.defaultModel(false, true); err != nil || p == "" {
		t.Fatalf("an answer without a newline: %q %v", p, err)
	}
	// Without asking, an absent model is simply none.
	if p, err := (&cli{}).defaultModel(false, false); err != nil || p == "" {
		t.Fatalf("cached model: %q %v", p, err)
	}
	cacheRoot = func() (string, error) { return t.TempDir(), nil }
	if p, err := (&cli{}).defaultModel(false, false); err != nil || p != "" {
		t.Fatalf("not cached, not asked: %q %v", p, err)
	}
}

func TestFetchHelpListsTheModels(t *testing.T) {
	code, out, _ := invoke(t, "fetch", "--help")
	if code != 0 || !strings.Contains(out, "gozen art fetch") || !strings.Contains(out, "censor") {
		t.Fatalf("code %d: %q", code, out)
	}
	code, _, diag := invoke(t, "fetch", "--nosuch")
	if code != 2 || !strings.Contains(diag, "gozen art fetch --help") {
		t.Fatalf("code %d: %s", code, diag)
	}
}
