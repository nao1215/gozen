package command

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
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
	_, out, _ := invoke(t, "fetch", "--list")
	if !strings.Contains(out, "not fetched") {
		t.Fatalf("list: %q", out)
	}
	if code, _, diag := invoke(t, "fetch", "censor"); code != 0 {
		t.Fatalf("fetch: %s", diag)
	}
	if b, err := os.ReadFile(filepath.Join(cache, "censor", "model.onnx")); /* #nosec G304 */ err != nil || string(b) != "model-bytes" {
		t.Fatalf("model: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(cache, "censor", "threshold.json")); err == nil {
		t.Fatal("an absent optional file must be skipped")
	}
	_, out, _ = invoke(t, "fetch", "--list")
	if !strings.Contains(out, "fetched "+filepath.Join(cache, "censor")) {
		t.Fatalf("list after fetch: %q", out)
	}
}

func TestFetchHashMismatchDeletes(t *testing.T) {
	cache, _ := fakeHub(t, true)
	code, _, diag := invoke(t, "fetch")
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
	if code, _, _ := invoke(t, "fetch", "nope"); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestDefaultModelNeverDownloadsWithoutConsent(t *testing.T) {
	_, hits := fakeHub(t, false)
	c := &cli{diag: &output{Writer: &strings.Builder{}}}
	_, err := c.defaultModel(false, true)
	if err == nil || !strings.Contains(err.Error(), "gozen art fetch censor") {
		t.Fatalf("a non-terminal must fail and point at fetch: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("no request without consent")
	}
}

func TestDefaultModelPrompt(t *testing.T) {
	cache, hits := fakeHub(t, false)
	interactive = func() bool { return true }
	c := &cli{diag: &output{Writer: &strings.Builder{}}}
	promptIn = strings.NewReader("n\n")
	if _, err := c.defaultModel(false, true); err == nil || hits.Load() != 0 {
		t.Fatalf("n must not download: %v %d", err, hits.Load())
	}
	promptIn = strings.NewReader("y\n")
	p, err := c.defaultModel(false, true)
	if err != nil || p != filepath.Join(cache, "censor", "model.onnx") {
		t.Fatalf("y must download: %q %v", p, err)
	}
	before := hits.Load()
	if p2, err := c.defaultModel(false, true); err != nil || p2 != p || hits.Load() != before {
		t.Fatal("a cached model is used without asking")
	}
}

func TestDefaultModelYes(t *testing.T) {
	_, hits := fakeHub(t, false)
	c := &cli{diag: &output{Writer: &strings.Builder{}}}
	p, err := c.defaultModel(true, true)
	if err != nil || p == "" || hits.Load() == 0 {
		t.Fatalf("--yes downloads without asking: %q %v", p, err)
	}
}

func TestOmittedModelUsesCache(t *testing.T) {
	cache, _ := fakeHub(t, false)
	if code, _, diag := invoke(t, "fetch"); code != 0 {
		t.Fatal(diag)
	}
	game, out := fixture(t)
	fs := newFlagSet("t")
	o := (&cli{}).commonFlags(fs)
	if err := fs.Parse([]string{"-o", out}); err != nil {
		t.Fatal(err)
	}
	c, err := o.setup(game)
	if err != nil || c.opts.model != filepath.Join(cache, "censor", "model.onnx") || c.classSource != "labels.json" {
		t.Fatalf("model %q source %q: %v", c.opts.model, c.classSource, err)
	}
}
