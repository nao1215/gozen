package gamepackage

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"
)

type fetchFunc func(context.Context, string, int64) ([]byte, error)

func (f fetchFunc) Fetch(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	return f(ctx, url, maxBytes)
}
func validateFixture(fsys fs.FS) error { _, e := fs.ReadFile(fsys, "game.yaml"); return e }

func TestInstallUpdateOfflineAndCancellation(t *testing.T) {
	r, b := archiveFixture(t, nil)
	store := FileStore{Root: t.TempDir()}
	calls := 0
	i := Installer{Engine: "0.23.0", Store: store, Validate: validateFixture, Fetcher: fetchFunc(func(_ context.Context, url string, limit int64) ([]byte, error) {
		calls++
		if url != r.URL || limit != r.Size {
			t.Fatal("unbounded fetch")
		}
		return b, nil
	})}
	old, e := i.Install(t.Context(), r)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if err := old.Close(); err != nil {
			t.Error(err)
		}
	})
	retry, retryErr := i.Install(t.Context(), r)
	if retryErr == nil {
		if closeErr := retry.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if e = retryErr; e != nil || calls != 1 {
		t.Fatalf("retry fetched again: %d %v", calls, e)
	}
	newer, updated := archiveFixture(t, []zipEntry{{"data/new.yaml", []byte("events: {}\n"), 0o600}})
	newer.Version = "1.1.0"
	i.Fetcher = fetchFunc(func(context.Context, string, int64) ([]byte, error) { return updated, nil })
	current, e := i.Install(t.Context(), newer)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if err := current.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, e = fs.ReadFile(current.FS, "data/new.yaml"); e != nil {
		t.Fatal(e)
	}
	if _, e = fs.ReadFile(old.FS, "data/new.yaml"); !errors.Is(e, fs.ErrNotExist) {
		t.Fatalf("old handle changed: %v", e)
	}
	i.Fetcher = nil
	reopened, openErr := i.Open(t.Context(), r)
	if openErr == nil {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if e = openErr; e != nil {
		t.Fatalf("old offline: %v", e)
	}
	reopened, openErr = i.Open(t.Context(), newer)
	if openErr == nil {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if e = openErr; e != nil {
		t.Fatalf("new offline: %v", e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = i.Open(ctx, r); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancelled open: %v", e)
	}
	entries, e := os.ReadDir(store.Root)
	if e != nil || len(entries) != 2 {
		t.Fatalf("cache entries: %v %v", entries, e)
	}
}

func TestInstallFailureNeverPublishes(t *testing.T) {
	r, b := archiveFixture(t, nil)
	for _, tc := range []string{"download", "digest", "validation", "cancel", "incompatible"} {
		t.Run(tc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			store := FileStore{Root: t.TempDir()}
			i := Installer{Engine: "0.23.0", Store: store, Validate: validateFixture, Fetcher: fetchFunc(func(context.Context, string, int64) ([]byte, error) {
				if tc == "download" {
					return nil, errors.New("interrupted")
				}
				if tc == "cancel" {
					cancel()
				}
				if tc == "digest" {
					return []byte("truncated"), nil
				}
				return b, nil
			})}
			if tc == "validation" {
				i.Validate = func(fs.FS) error { return errors.New("bad definitions") }
			}
			if tc == "incompatible" {
				i.Engine = "0.22.0"
			}
			if _, e := i.Install(ctx, r); e == nil {
				t.Fatal("unexpected install success")
			}
			entries, e := os.ReadDir(store.Root)
			if e != nil || len(entries) != 0 {
				t.Fatalf("published failed install: %v %v", entries, e)
			}
		})
	}
}

func TestFileStoreAtomicConcurrentPublish(t *testing.T) {
	r, b := archiveFixture(t, nil)
	s := FileStore{Root: t.TempDir()}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if e := s.Save(t.Context(), r.SHA256, b); e != nil {
				t.Errorf("save: %v", e)
			}
		})
	}
	wg.Wait()
	got, e := s.Load(t.Context(), r.SHA256)
	if e != nil || !bytes.Equal(got, b) {
		t.Fatalf("published bytes: %v", e)
	}
	entries, e := os.ReadDir(s.Root)
	if e != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v %v", entries, e)
	}
	if e = s.Save(t.Context(), r.SHA256, []byte("changed")); e == nil {
		t.Fatal("replaced verified version")
	}
	got, e = s.Load(t.Context(), r.SHA256)
	if e != nil || !bytes.Equal(got, b) {
		t.Fatal("failed save corrupted installed bytes")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if e = s.Save(ctx, r.SHA256, b); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancelled save: %v", e)
	}
	if _, e = s.Load(t.Context(), "../escape"); e == nil {
		t.Fatal("unsafe cache key")
	}
	if e = os.WriteFile(filepath.Join(s.Root, r.SHA256+".zip"), []byte("corrupt"), 0o600); e != nil {
		t.Fatal(e)
	}
	if e = s.Save(t.Context(), r.SHA256, b); e == nil {
		t.Fatal("silently overwrote corrupt cache")
	}
}

func TestFileStoreConcurrentStreamingRetainsOpenReaders(t *testing.T) {
	r, b := archiveFixture(t, nil)
	store := FileStore{Root: t.TempDir()}
	const workers = 8
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	installer := Installer{Engine: "0.23.0", Store: store, Validate: validateFixture, Fetcher: gatedStreamFetch{b, ready, start}}
	type result struct {
		game *Installed
		err  error
	}
	results := make(chan result, workers)
	for range workers {
		go func() { game, err := installer.Install(t.Context(), r); results <- result{game, err} }()
	}
	for range workers {
		<-ready
	}
	close(start)
	games := make([]*Installed, 0, workers)
	for range workers {
		got := <-results
		if got.err != nil {
			t.Error(got.err)
			continue
		}
		games = append(games, got.game)
		t.Cleanup(func() {
			if err := got.game.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if len(games) != workers {
		t.FailNow()
	}
	// Hold a reader from each successful installation while publishing again.
	for _, game := range games {
		reader, err := game.FS.Open("game.yaml")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := reader.Close(); err != nil {
				t.Error(err)
			}
		})
		if err := store.Save(t.Context(), r.SHA256, b); err != nil {
			t.Fatal(err)
		}
		if contents, err := io.ReadAll(reader); err != nil || len(contents) == 0 {
			t.Fatalf("open reader changed: %q %v", contents, err)
		}
	}
	entries, err := os.ReadDir(store.Root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("published cache: %v %v", entries, err)
	}
}

type gatedStreamFetch struct {
	data  []byte
	ready chan<- struct{}
	start <-chan struct{}
}

func (f gatedStreamFetch) Fetch(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("unexpected memory download")
}

func (f gatedStreamFetch) FetchTo(ctx context.Context, _ string, _ int64, dst io.Writer) error {
	f.ready <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.start:
	}
	_, err := dst.Write(f.data)
	return err
}

func TestCatalogRefreshRequiresIndependentTrust(t *testing.T) {
	r, _ := archiveFixture(t, nil)
	b, e := yaml.Marshal(Catalog{Schema: 1, Releases: []Release{r}})
	if e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	public := base64.StdEncoding.EncodeToString(key[ed25519.SeedSize:])
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, b)))
	calls := 0
	i := Installer{Fetcher: fetchFunc(func(_ context.Context, url string, n int64) ([]byte, error) {
		calls++
		if strings.HasSuffix(url, ".sig") {
			if n != 256 {
				t.Fatal("unbounded signature")
			}
			return signature, nil
		}
		if n != maxCatalogBytes {
			t.Fatal("unbounded catalog")
		}
		return b, nil
	})}
	const catalogURL = "https://example.com/catalog.yaml"
	const signatureURL = "https://example.com/catalog.sig"
	if _, e = i.RefreshSignedCatalog(t.Context(), catalogURL, signatureURL, public); e != nil {
		t.Fatal(e)
	}
	if _, e = i.RefreshSignedCatalog(t.Context(), catalogURL, signatureURL, ""); e == nil || calls != 2 {
		t.Fatal("fetched without anchor")
	}
	if _, e = i.RefreshSignedCatalog(t.Context(), "http://example.com/catalog.yaml", signatureURL, public); e == nil || calls != 2 {
		t.Fatal("fetched insecure catalog")
	}
	b = append(b, ' ')
	if _, e = i.RefreshSignedCatalog(t.Context(), catalogURL, signatureURL, public); e == nil {
		t.Fatal("accepted unauthenticated catalog")
	}
}

type streamFetch struct {
	data []byte
	mode string
}

func (s streamFetch) Fetch(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("memory fetch must not be used")
}

func (s streamFetch) FetchTo(ctx context.Context, _ string, _ int64, w io.Writer) error {
	switch s.mode {
	case "interrupted":
		if _, e := w.Write(s.data[:len(s.data)/2]); e != nil {
			return e
		}
		return errors.New("interrupted transfer")
	case "oversized":
		_, e := w.Write(append(bytes.Clone(s.data), 0))
		return e
	case "truncated":
		_, e := w.Write(s.data[:len(s.data)/2])
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	_, e := w.Write(s.data)
	return e
}

func TestStreamingInstallAndFailedPromotion(t *testing.T) {
	r, b := archiveFixture(t, nil)
	for _, mode := range []string{"success", "interrupted", "oversized", "truncated", "invalid definitions"} {
		t.Run(mode, func(t *testing.T) {
			store := FileStore{Root: t.TempDir()}
			i := Installer{Engine: "0.23.0", Store: store, Fetcher: streamFetch{b, mode}, Validate: validateFixture}
			if mode == "invalid definitions" {
				i.Validate = func(fs.FS) error { return errors.New("bad game") }
			}
			game, e := i.Install(t.Context(), r)
			entries, readErr := os.ReadDir(store.Root)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if mode != "success" {
				if e == nil || len(entries) != 0 {
					t.Fatalf("failed transfer published: %v %v", e, entries)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if len(entries) != 1 {
				t.Fatalf("unexpected cache: %v", entries)
			}
			if _, e = fs.ReadFile(game.FS, "game.yaml"); e != nil {
				t.Fatal(e)
			}
			if e = game.Close(); e != nil {
				t.Fatal(e)
			}
			i.Fetcher = nil
			game, e = i.Open(t.Context(), r)
			if e != nil {
				t.Fatal(e)
			}
			if e = game.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestBrowserInstallUsesReviewedMirrorWithoutChangingIdentity(t *testing.T) {
	r, b := archiveFixture(t, nil)
	r.BrowserURL = "https://raw.githubusercontent.com/sample/game/0000000000000000000000000000000000000000/releases/game.zip"
	calls := 0
	i := Installer{Browser: true, Engine: "0.23.0", Store: FileStore{Root: t.TempDir()}, Validate: validateFixture, Fetcher: fetchFunc(func(_ context.Context, address string, _ int64) ([]byte, error) {
		calls++
		if address != r.BrowserURL {
			t.Fatal("browser used redirecting native URL")
		}
		return b, nil
	})}
	game, e := i.Install(t.Context(), r)
	if e != nil {
		t.Fatal(e)
	}
	if game.Release != r {
		t.Fatal("mirror changed installed release identity")
	}
	if e = game.Close(); e != nil {
		t.Fatal(e)
	}
	game, e = i.Open(t.Context(), r)
	if e != nil {
		t.Fatal(e)
	}
	if e = game.Close(); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("offline opening fetched archive")
	}
	missing := r
	missing.BrowserURL = ""
	i.Store = FileStore{Root: t.TempDir()}
	if _, e = i.Install(t.Context(), missing); e == nil || calls != 1 {
		t.Fatal("browser downloaded without approved mirror")
	}
}

func TestStreamingCleanupFailureClosesInstalledHandle(t *testing.T) {
	r, b := archiveFixture(t, nil)
	store := FileStore{Root: t.TempDir()}
	calls := 0
	var retained fs.FS
	installer := Installer{Engine: "0.23.0", Store: store, Fetcher: streamFetch{b, "success"}, Validate: func(fsys fs.FS) error {
		calls++
		if calls != 2 {
			return validateFixture(fsys)
		}
		retained = fsys
		entries, err := os.ReadDir(store.Root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".install-") {
				continue
			}
			name := filepath.Join(store.Root, entry.Name())
			if err := os.Remove(name); err != nil {
				return err
			}
			if err := os.Mkdir(name, 0o700); err != nil {
				return err
			}
			// A nonempty replacement forces deferred staging cleanup to fail.
			return os.WriteFile(filepath.Join(name, "occupied"), []byte("fixture"), 0o600)
		}
		return errors.New("staging fixture missing")
	}}
	game, err := installer.Install(t.Context(), r)
	if err == nil || game != nil || retained == nil {
		t.Fatalf("cleanup result: game=%v err=%v retained=%v", game, err, retained != nil)
	}
	if _, err := fs.ReadFile(retained, "game.yaml"); err == nil {
		t.Fatal("failed installation leaked its archive handle")
	}
}

func TestFileStoreDeepCacheRoot(t *testing.T) {
	root := t.TempDir()
	for len(root) < 300 {
		root = filepath.Join(root, "deep-cache-directory")
	}
	r, b := archiveFixture(t, nil)
	store := FileStore{Root: root}
	installer := Installer{Engine: "0.23.0", Store: store, Validate: validateFixture, Fetcher: streamFetch{b, "success"}}
	game, err := installer.Install(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := game.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Snapshot(t.Context(), r.SHA256, int64(len(b))); err != nil || !bytes.Equal(got, b) {
		t.Fatalf("deep snapshot: %v", err)
	}
}
