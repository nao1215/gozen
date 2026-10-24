package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nao1215/gozen/internal/cliio"
)

// fakeHub serves a tiny pinned model, counts requests, and points the package at it.
// Nothing here uses the network.
func fakeHub(t *testing.T, corrupt bool) (cache string, hits *atomic.Int32) {
	t.Helper()
	files := map[string]string{"model.onnx": "model-bytes", "labels.json": `["face"]`}
	var spec []fileSpec
	for n, c := range files {
		sum := sha256.Sum256([]byte(c))
		spec = append(spec, fileSpec{Name: n, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(c))})
	}
	spec = append(spec, fileSpec{Name: "threshold.json", SHA256: strings.Repeat("0", 64), Size: 1, Optional: true})
	hits = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		name := filepath.Base(r.URL.Path)
		c, ok := files[name]
		if !ok || r.URL.Path != "/o/r/resolve/rev1/dir/"+name {
			http.NotFound(w, r)
			return
		}
		if corrupt && name == "model.onnx" {
			c = "tampered!!!"
		}
		if _, err := w.Write([]byte(c)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	cache = t.TempDir()
	oldModels, oldHub, oldRoot, oldInter, oldIn := models, hubURL, cacheRoot, interactive, promptIn
	t.Cleanup(func() { models, hubURL, cacheRoot, interactive, promptIn = oldModels, oldHub, oldRoot, oldInter, oldIn })
	models = []modelSpec{{Name: "censor", Repo: "o/r", Rev: "rev1", Dir: "dir", License: "MIT", Files: spec}}
	hubURL = srv.URL
	cacheRoot = func() (string, error) { return cache, nil }
	interactive = func() bool { return false }
	return cache, hits
}

func TestFetchAndList(t *testing.T) {
	cache, _ := fakeHub(t, false)
	_, out, _ := invoke(t, "models", "list")
	if !strings.Contains(out, "not fetched") {
		t.Fatalf("list: %q", out)
	}
	if code, _, diag := invoke(t, "models", "fetch", "censor"); code != 0 {
		t.Fatalf("fetch: %s", diag)
	}
	if b, err := os.ReadFile(filepath.Join(cache, "censor", "model.onnx")); /* #nosec G304 */ err != nil || string(b) != "model-bytes" {
		t.Fatalf("model: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(cache, "censor", "threshold.json")); err == nil {
		t.Fatal("an absent optional file must be skipped")
	}
	_, out, _ = invoke(t, "models", "list")
	if !strings.Contains(out, "fetched "+filepath.Join(cache, "censor")) {
		t.Fatalf("list after fetch: %q", out)
	}
}

func TestFetchHashMismatchDeletes(t *testing.T) {
	cache, _ := fakeHub(t, true)
	code, _, diag := invoke(t, "models", "fetch")
	if code != 1 || !strings.Contains(diag, "SHA-256 mismatch") {
		t.Fatalf("code %d: %s", code, diag)
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 0 {
		t.Fatalf("a bad download must leave nothing: %v", entries)
	}
}

func TestFetchUnknownModel(t *testing.T) {
	fakeHub(t, false)
	if code, _, _ := invoke(t, "models", "fetch", "nope"); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestDefaultModelNeverDownloadsWithoutConsent(t *testing.T) {
	_, hits := fakeHub(t, false)
	c := &cli{diag: &cliio.Output{Writer: &strings.Builder{}}}
	_, err := c.defaultModel(t.Context(), false, true)
	if err == nil || !strings.Contains(err.Error(), "gozen models fetch censor") {
		t.Fatalf("a non-terminal must fail and point at fetch: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("no request without consent")
	}
}

func TestDefaultModelPrompt(t *testing.T) {
	cache, hits := fakeHub(t, false)
	interactive = func() bool { return true }
	c := &cli{diag: &cliio.Output{Writer: &strings.Builder{}}}
	promptIn = strings.NewReader("n\n")
	if _, err := c.defaultModel(t.Context(), false, true); err == nil || hits.Load() != 0 {
		t.Fatalf("n must not download: %v %d", err, hits.Load())
	}
	promptIn = strings.NewReader("y\n")
	p, err := c.defaultModel(t.Context(), false, true)
	if err != nil || p != filepath.Join(cache, "censor", "model.onnx") {
		t.Fatalf("y must download: %q %v", p, err)
	}
	before := hits.Load()
	if p2, err := c.defaultModel(t.Context(), false, true); err != nil || p2 != p || hits.Load() != before {
		t.Fatal("a cached model is used without asking")
	}
}

func TestDefaultModelYes(t *testing.T) {
	_, hits := fakeHub(t, false)
	c := &cli{diag: &cliio.Output{Writer: &strings.Builder{}}}
	p, err := c.defaultModel(t.Context(), true, true)
	if err != nil || p == "" || hits.Load() == 0 {
		t.Fatalf("--yes downloads without asking: %q %v", p, err)
	}
}

func TestOmittedModelUsesCache(t *testing.T) {
	cache, _ := fakeHub(t, false)
	if code, _, diag := invoke(t, "models", "fetch"); code != 0 {
		t.Fatal(diag)
	}
	game, out := fixture(t)
	fs := newFlagSet("t")
	o := (&cli{}).commonFlags(fs)
	if err := fs.Parse([]string{"-o", out}); err != nil {
		t.Fatal(err)
	}
	c, err := o.setup(t.Context(), game)
	if err != nil || c.opts.model != filepath.Join(cache, "censor", "model.onnx") || c.classSource != "labels.json" {
		t.Fatalf("model %q source %q: %v", c.opts.model, c.classSource, err)
	}
}

func TestFetchRequiredFileMissingOnTheHub(t *testing.T) {
	cache, _ := fakeHub(t, false)
	models[0].Files = append(models[0].Files, fileSpec{Name: "extra.bin", SHA256: strings.Repeat("0", 64), Size: 1})
	code, _, diag := invoke(t, "models", "fetch")
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
	code, _, diag := invoke(t, "models", "fetch", "censor")
	if code != 1 || !strings.Contains(diag, "500") {
		t.Fatalf("code %d: %s", code, diag)
	}
	srv.Close()
	code, _, diag = invoke(t, "models", "fetch", "censor")
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
	code, _, diag := invoke(t, "models", "fetch")
	if code != 1 || !strings.Contains(diag, "SHA-256 mismatch") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestFetchWithoutACacheDirectory(t *testing.T) {
	fakeHub(t, false)
	cacheRoot = func() (string, error) { return "", errors.New("no home") }
	code, _, diag := invoke(t, "models", "fetch")
	if code != 1 || !strings.Contains(diag, "no cache directory") {
		t.Fatalf("code %d: %s", code, diag)
	}
	if _, ok := models[0].cachedModel(); ok {
		t.Fatal("a model is cached without a cache directory")
	}
	_, out, _ := invoke(t, "models", "list")
	if !strings.Contains(out, "not fetched") {
		t.Fatalf("list %q", out)
	}
}

func TestCachedModelNeedsTheRightFiles(t *testing.T) {
	cache, _ := fakeHub(t, false)
	if code, _, diag := invoke(t, "models", "fetch"); code != 0 {
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
	c := &cli{diag: &cliio.Output{Writer: &strings.Builder{}}}
	promptIn = strings.NewReader("")
	if _, err := c.defaultModel(t.Context(), false, true); err == nil || !strings.Contains(err.Error(), "no answer") || hits.Load() != 0 {
		t.Fatalf("end of input is not consent: %v (%d requests)", err, hits.Load())
	}
	promptIn = strings.NewReader("YES")
	if p, err := c.defaultModel(t.Context(), false, true); err != nil || p == "" {
		t.Fatalf("an answer without a newline: %q %v", p, err)
	}
	// Without asking, an absent model is simply none.
	if p, err := (&cli{}).defaultModel(t.Context(), false, false); err != nil || p == "" {
		t.Fatalf("cached model: %q %v", p, err)
	}
	cacheRoot = func() (string, error) { return t.TempDir(), nil }
	if p, err := (&cli{}).defaultModel(t.Context(), false, false); err != nil || p != "" {
		t.Fatalf("not cached, not asked: %q %v", p, err)
	}
}

func TestFetchHelpListsTheModels(t *testing.T) {
	code, out, _ := invoke(t, "models", "fetch", "--help")
	if code != 0 || !strings.Contains(out, "gozen models fetch") || !strings.Contains(out, "censor") {
		t.Fatalf("code %d: %q", code, out)
	}
	code, _, diag := invoke(t, "models", "fetch", "--nosuch")
	if code != 2 || !strings.Contains(diag, "gozen models fetch --help") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestCancelledDownloadPreservesCache(t *testing.T) {
	cache, hits := fakeHub(t, false)
	dir := filepath.Join(cache, "censor")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(old, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := models[0].download(ctx, &cliio.Output{Writer: &strings.Builder{}})
	if !errors.Is(err, context.Canceled) || hits.Load() != 0 {
		t.Fatalf("download: %v, requests: %d", err, hits.Load())
	}
	b, err := os.ReadFile(old) //nolint:gosec // Test-owned path beneath t.TempDir.
	if err != nil || string(b) != "previous" {
		t.Fatalf("old cache: %q, %v", b, err)
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial download remains: %v, %v", entries, err)
	}
}
