package play

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
	"github.com/nao1215/gozen/internal/savefile"
)

func TestPhoneArchiveCachePreservesBytesAndCancellation(t *testing.T) {
	st := savefile.MemStore()
	cache := phoneArchiveStore{st}
	original := []byte{0, 255, 128, 13, 10, 0}
	sum := sha256.Sum256(original)
	digest := hex.EncodeToString(sum[:])
	if err := cache.Save(t.Context(), digest, original); err != nil {
		t.Fatal(err)
	}
	got, err := cache.Load(t.Context(), digest)
	if err != nil || string(got) != string(original) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = cache.Save(ctx, digest, original); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if err = cache.Save(t.Context(), digest, []byte("changed")); err == nil {
		t.Fatal("changed bytes accepted")
	}
	got, err = cache.Load(t.Context(), digest)
	if err != nil || string(got) != string(original) {
		t.Fatalf("old archive lost: %v", err)
	}
	if _, err = cache.Load(t.Context(), "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}

type phoneFailStore struct{ Store }

func (phoneFailStore) Write(string, []byte) error { return errors.New("storage full") }

func TestPhonePackageSelectionCommitsAfterStorage(t *testing.T) {
	old := gamepackage.Release{ID: "test/game", Version: "1.0.0"}
	next := &gamepackage.Installed{Release: gamepackage.Release{ID: "test/game", Version: "1.1.0"}}
	st := savefile.MemStore()
	g := &Game{store: phoneFailStore{st}, packages: &phonePackages{selected: map[string]gamepackage.Release{"game": old}, opened: map[string]*gamepackage.Installed{}}}
	if err := g.selectPhonePackage("game", next); err == nil {
		t.Fatal("write failure ignored")
	}
	if g.packages.selected["game"].Version != "1.0.0" || g.packages.opened["game"] != nil {
		t.Fatal("failed update changed selection")
	}
	g.store = st
	if err := g.selectPhonePackage("game", next); err != nil {
		t.Fatal(err)
	}
	b, err := st.Read("phone-apps.json")
	if err != nil {
		t.Fatal(err)
	}
	var selected map[string]gamepackage.Release
	if err = json.Unmarshal(b, &selected); err != nil || selected["game"].Version != "1.1.0" {
		t.Fatalf("selection not persisted: %v", err)
	}
}

type phoneTestFetcher map[string][]byte

func (f phoneTestFetcher) Fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, ok := f[url]
	if !ok || int64(len(b)) > limit {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), b...), nil
}

func TestPhoneSignedCatalogRestoresOfflineAndRejectsTampering(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	catalogBytes, err := os.ReadFile("../../examples/office/game/apps/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, catalogBytes))
	for _, name := range []string{"counter", "trail"} {
		t.Run(name, func(t *testing.T) {
			f := gameFiles(t)
			config, err := os.ReadFile("testdata/phone-" + name + "/game.yaml") //nolint:gosec // Fixed fixture names from the test table.
			if err != nil {
				t.Fatal(err)
			}
			f["game.yaml"] = &fstest.MapFile{Data: config}
			f["catalog.yaml"] = &fstest.MapFile{Data: []byte("schema: 1\nreleases: []\n")}
			db, err := data.Load(f)
			if err != nil {
				t.Fatal(err)
			}
			st := savefile.MemStore()
			makeGame := func() *Game { return &Game{db: db, opts: Options{FS: f, Store: st}, store: st, storeGiven: true} }
			g := makeGame()
			if err = g.preparePhonePackages(); err != nil {
				t.Fatal(err)
			}
			cfg := db.Game.Phone
			g.packages.installer.Fetcher = phoneTestFetcher{cfg.CatalogURL: catalogBytes, cfg.CatalogSignatureURL: []byte(signature)}
			snapshot, err := g.packages.installer.RefreshSignedCatalog(t.Context(), cfg.CatalogURL, cfg.CatalogSignatureURL, cfg.CatalogPublicKey)
			if err != nil {
				t.Fatal(err)
			}
			if err = g.acceptPhoneCatalog(snapshot); err != nil {
				t.Fatal(err)
			}
			restored := makeGame()
			if err = restored.preparePhonePackages(); err != nil {
				t.Fatal(err)
			}
			app := cfg.Apps[0]
			if _, err = restored.packages.catalog.Resolve(app.Package, app.Version, phoneEngineVersion()); err != nil {
				t.Fatal(err)
			}
			if err = g.acceptPhoneCatalog(gamepackage.SignedCatalog{Catalog: gamepackage.Catalog{Schema: 1}}); err == nil {
				t.Fatal("catalog rollback accepted")
			}
			encoded, err := st.Read("phone-catalog.json")
			if err != nil {
				t.Fatal(err)
			}
			var saved phoneCatalogSnapshot
			if err = json.Unmarshal(encoded, &saved); err != nil {
				t.Fatal(err)
			}
			saved.Data = []byte("schema: 2\nreleases: []\n")
			encoded, err = json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			if err = st.Write("phone-catalog.json", encoded); err != nil {
				t.Fatal(err)
			}
			if err = makeGame().preparePhonePackages(); err == nil {
				t.Fatal("tampered cached catalog accepted")
			}
		})
	}
}

func TestPhoneOptionalArchiveTraceRestoresWithoutNativeCache(t *testing.T) {
	catalogBytes, e := os.ReadFile("../../examples/office/game/apps/catalog.yaml")
	if e != nil {
		t.Fatal(e)
	}
	catalog, e := gamepackage.Parse(catalogBytes)
	if e != nil {
		t.Fatal(e)
	}
	release, e := catalog.Resolve("fixtures/counter", "1.0.0", phoneEngineVersion())
	if e != nil {
		t.Fatal(e)
	}
	archive, e := os.ReadFile("../../examples/office/game/apps/counter.zip")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(catalogBytes)
	for _, mode := range []string{"memory", "native", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			native := mode != "memory"
			files := gameFiles(t)
			files["catalog.yaml"] = &fstest.MapFile{Data: catalogBytes}
			files["game.yaml"].Data = append(files["game.yaml"].Data, []byte(fmt.Sprintf("\nphone:\n  catalog: catalog.yaml\n  catalog_sha256: %x\n  apps:\n    - {id: counter, package: fixtures/counter, version: 1.0.0, install: optional}\n", sum))...)
			st := savefile.MemStore()
			saveDir := ""
			var cache gamepackage.Store = phoneArchiveStore{st}
			if native {
				saveDir = t.TempDir()
				cache = gamepackage.FileStore{Root: filepath.Join(saveDir, "phone-packages")}
			}
			if e := cache.Save(t.Context(), release.SHA256, archive); e != nil {
				t.Fatal(e)
			}
			if mode == "oversized" {
				// Sparse bytes must be rejected by actual stat size, not stale lock metadata.
				f, e := os.OpenFile(filepath.Join(saveDir, "phone-packages", release.SHA256+".zip"), os.O_WRONLY, 0) //nolint:gosec // Verified fixture digest within the test temporary cache.
				if e != nil {
					t.Fatal(e)
				}
				truncateErr := f.Truncate(gamepackage.MaxStoredArchiveBytes)
				if e = errors.Join(truncateErr, f.Close()); e != nil {
					t.Fatal(e)
				}
			}

			selected, e := json.Marshal(map[string]gamepackage.Release{"counter": release})
			if e != nil {
				t.Fatal(e)
			}
			if e = st.Write("phone-apps.json", selected); e != nil {
				t.Fatal(e)
			}
			var recording bytes.Buffer
			g, e := New(Options{FS: files, Store: st, SaveDir: saveDir, NoSound: true, headless: true, Trace: &recording, Seed: 7})
			if e != nil {
				t.Fatal(e)
			}
			if e = g.Update(); e != nil {
				t.Fatal(e)
			}
			if e = g.Close(); e != nil {
				t.Fatal(e)
			}
			if native {
				if _, e = st.Read("phone-package-" + release.SHA256 + ".b64"); !errors.Is(e, fs.ErrNotExist) {
					t.Fatal("trace persisted virtual archive in player saves")
				}
				if e = os.RemoveAll(saveDir); e != nil {
					t.Fatal(e)
				}
			}
			name := filepath.Join(t.TempDir(), "phone.jsonl")
			if e = os.WriteFile(name, recording.Bytes(), 0o600); e != nil {
				t.Fatal(e)
			}
			replay, e := New(Options{FS: files, Store: savefile.MemStore(), NoSound: true, headless: true, Replay: name})
			if mode == "oversized" {
				if e == nil {
					t.Fatal(errors.Join(errors.New("oversized package trace claimed exact replay"), replay.Close()))
				}
				if !strings.Contains(e.Error(), "8 MiB") || recording.Len() > 1<<20 {
					t.Fatalf("unsupported snapshot: trace=%d bytes, error=%v", recording.Len(), e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			installed, e := replay.phoneInstalled("counter")
			if e != nil {
				t.Fatal(errors.Join(e, replay.Close()))
			}
			if _, e = fs.ReadFile(installed.FS, "game.yaml"); e != nil {
				t.Fatal(e)
			}
			if e = replay.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestPhoneIconMetadataRefreshAndIdleCost(t *testing.T) {
	catalogBytes, e := os.ReadFile("../../examples/office/game/apps/catalog.yaml")
	if e != nil {
		t.Fatal(e)
	}
	catalog, e := gamepackage.Parse(catalogBytes)
	if e != nil {
		t.Fatal(e)
	}
	old, e := catalog.Resolve("fixtures/counter", "1.0.0", phoneEngineVersion())
	if e != nil {
		t.Fatal(e)
	}
	g := &Game{store: savefile.MemStore(), db: &data.DB{Game: data.Game{Phone: &data.PhoneApps{Apps: []data.PhoneApp{{ID: "counter", Package: old.ID, Version: ">=1.0.0 <2.0.0", Install: "optional"}}}}}, packages: &phonePackages{catalog: catalog, selected: map[string]gamepackage.Release{"counter": old}}}
	for _, key := range []string{"package:counter", "installed:counter"} {
		if r, e := g.phoneIconRelease(key); e != nil || r.SHA256 != old.SHA256 {
			t.Fatalf("initial icon: %+v %v", r, e)
		}
		if n := testing.AllocsPerRun(100, func() {
			if _, e := g.phoneIconRelease(key); e != nil {
				t.Fatal(e)
			}
		}); n != 0 {
			t.Fatalf("cached icon metadata allocates %.0f objects per draw (PNG decoding must stay outside idle draws)", n)
		}
	}
	next := old
	next.Version = "1.1.0"
	next.SHA256 = strings.Repeat("a", 64)
	updated := catalog
	updated.Releases = append(append([]gamepackage.Release(nil), catalog.Releases...), next)
	if e = g.acceptPhoneCatalog(gamepackage.SignedCatalog{Catalog: updated}); e != nil {
		t.Fatal(e)
	}
	if r, e := g.phoneIconRelease("package:counter"); e != nil || r.SHA256 != next.SHA256 {
		t.Fatalf("store icon did not refresh: %+v %v", r, e)
	}
	if r, e := g.phoneIconRelease("installed:counter"); e != nil || r.SHA256 != old.SHA256 {
		t.Fatalf("catalog refresh silently changed installed icon: %+v %v", r, e)
	}
	if e = g.selectPhonePackage("counter", &gamepackage.Installed{Release: next}); e != nil {
		t.Fatal(e)
	}
	if r, e := g.phoneIconRelease("installed:counter"); e != nil || r.SHA256 != next.SHA256 {
		t.Fatalf("installed icon did not refresh after selection: %+v %v", r, e)
	}
}

func TestPhoneEngineDevelopmentCompatibility(t *testing.T) {
	for _, version := range []string{"devel", "(devel)", "v0.22.0+dirty", "v0.22.1-0.20261108010203-012345abcdef", "v0.0.0-20261108010203-012345abcdef+dirty"} {
		if got := phoneEngineTarget(version); got != phoneDevelopmentVersion {
			t.Errorf("development %s: got %s", version, got)
		}
	}
	for _, version := range []string{"0.22.0", "v0.23.0", "invalid+dirty", "v0.23.0-rc1", "v0.22.1-0.20269908010203-012345abcdef", "v01.2.3+dirty"} {
		if got := phoneEngineTarget(version); got != strings.TrimPrefix(version, "v") {
			t.Errorf("release or invalid %s changed to %s", version, got)
		}
	}
}
