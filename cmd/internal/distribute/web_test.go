package distribute

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/deepteams/webp"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
	"github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/gamepackage"
	"github.com/nao1215/gozen/internal/testgame"
)

// webWASM and webJS are the player and the wasm_exec.js of webBundle: a minimal WASM header
// tests packaging only; the browser E2E runs the real player.
var (
	webWASM = []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	webJS   = []byte("// neutral runtime fixture\n")
)

// webBundle is a web runtime bundle of webWASM and webJS.
func webBundle(t *testing.T) string {
	t.Helper()
	bundle := t.TempDir()
	wasm, js := webWASM, webJS
	for name, b := range map[string][]byte{"gozen-player.wasm": wasm, "wasm_exec.js": js, "LICENSE": []byte("MIT"), "GO-LICENSE": []byte("BSD fixture"), "THIRD_PARTY_LICENSES/notice.txt": []byte("fixture notice")} {
		if err := cli.WriteNew(filepath.Join(bundle, filepath.FromSlash(name)), b); err != nil {
			t.Fatal(err)
		}
	}
	hash := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
	r := Runtime{Version: "devel", Target: "js-wasm", Player: "gozen-player.wasm", SHA256: hash(wasm), WASMExecSHA256: hash(js)}
	metadata, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.WriteNew(filepath.Join(bundle, "runtime.json"), metadata); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestWebManifestAndRuntimeValidation(t *testing.T) {
	src, bundle := t.TempDir(), webBundle(t)
	wasm := webWASM
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "web-test", Title: "Web test"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.WriteNew(filepath.Join(src, "assets", "space #%.txt"), []byte("kept")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, bundle, "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "game", "files.json")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	var entries []webFile
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	found, pictures, zipped := false, 0, 0
	for _, entry := range entries {
		served, err := os.ReadFile(filepath.Join(out, "game", filepath.FromSlash(entry.File))) //nolint:gosec // Test-owned exported file.
		if err != nil || int64(len(served)) != entry.Size {
			t.Fatalf("manifest entry %+v: %v", entry, err)
		}
		source, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(entry.Name))) //nolint:gosec // Test-owned game file.
		if err != nil {
			t.Fatal(err)
		}
		// a file is served as it is, as a WebP of its pixels, or gzipped
		switch entry.File {
		case entry.Name:
			if entry.Gzip || !bytes.Equal(served, source) {
				t.Errorf("%s: served changed", entry.Name)
			}
		case entry.Name + ".webp":
			pictures++
			want, _, err := image.DecodeConfig(bytes.NewReader(source))
			if err != nil {
				t.Fatal(err)
			}
			got, err := webp.DecodeConfig(bytes.NewReader(served))
			if err != nil || got.Width != want.Width || got.Height != want.Height || len(served) >= len(source) {
				t.Errorf("%s: WebP %+v, %v, %d bytes from %d", entry.Name, got, err, len(served), len(source))
			}
		case entry.Name + ".gz":
			zipped++
			z, err := gzip.NewReader(bytes.NewReader(served))
			if err != nil {
				t.Fatal(err)
			}
			back, err := io.ReadAll(z)
			if err != nil || !entry.Gzip || !bytes.Equal(back, source) {
				t.Errorf("%s: gzip does not give the file back: %v", entry.Name, err)
			}
		default:
			t.Errorf("manifest entry %+v", entry)
		}
		if entry.Name == "assets/space #%.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("special-character asset missing from manifest")
	}
	if pictures == 0 || zipped == 0 {
		t.Errorf("%d pictures became WebP and %d files were gzipped", pictures, zipped)
	}
	// the player is served gzipped
	player, err := os.ReadFile(filepath.Join(out, "gozen-player.wasm.gz")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	if z, err := gzip.NewReader(bytes.NewReader(player)); err != nil {
		t.Fatal(err)
	} else if back, err := io.ReadAll(z); err != nil || !bytes.Equal(back, wasm) {
		t.Errorf("gozen-player.wasm.gz: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "gozen-player.wasm")); err == nil {
		t.Error("the uncompressed player was left in the export")
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	// the page shows how much of the player and of the game's files has come
	for _, want := range []string{`id="fill"`, `id="amount"`, "window.goZenProgress", "response.body.getReader()", `goZenProgress("player"`, "DecompressionStream", "unlockSpeech"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html has no %s", want)
		}
	}
	if err := os.WriteFile(filepath.Join(bundle, "wasm_exec.js"), []byte("wrong runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	if err := Web(t.Context(), src, filepath.Join(parent, "site"), bundle, "devel", ""); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("mismatched JavaScript: %v", err)
	}
	files, err := os.ReadDir(parent)
	if err != nil || len(files) != 0 {
		t.Fatalf("partial output: %v, %v", files, err)
	}
}

// TestWebFetchesVoicesLater: the recorded voices of a stream game (the streams file's
// voices folder) are marked to be fetched only when the game opens them; its other files,
// sounds too, are not.
func TestWebFetchesVoicesLater(t *testing.T) {
	src := t.TempDir()
	files := testgame.StreamFiles()
	files["game.yaml"] = testgame.Game + "id: voices-later\n" + testgame.StreamsGame
	files["data/streams.yaml"] = "voices: voice\n" + files["data/streams.yaml"]
	files["assets/voice/ann/6f2529aa52538b73.ogg"] = "OggS voice" // Hello, everyone.
	files["assets/bell.ogg"] = "OggS bell"
	files["data/main.yaml"], files["assets/x.png"] = testgame.Data, "x"
	for name, text := range files {
		if err := cli.WriteNew(filepath.Join(src, filepath.FromSlash(name)), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, webBundle(t), "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "game", "files.json")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	var entries []webFile
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	later := 0
	for _, e := range entries {
		if want := strings.HasPrefix(e.Name, "assets/voice/"); e.Later != want {
			t.Errorf("%s: fetched later %v, want %v", e.Name, e.Later, want)
		}
		if e.Later {
			later++
		}
	}
	if later != 1 {
		t.Errorf("%d files fetched later, not the one voice", later)
	}
}

// TestWebPKeepsThinLines: a painted puppet's lines (its eyes, lips, strands of hair: the
// pixels that differ much from the one beside them) stay sharp in its WebP, which the
// default 4:2:0 color of WebP at quality 85 smears.
func TestWebPKeepsThinLines(t *testing.T) {
	src, err := os.ReadFile("../../../examples/puppet/assets/streamer/streamer.png")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := image.NewNRGBA(img.Bounds())
	draw.Draw(want, want.Bounds(), img, img.Bounds().Min, draw.Src)
	out := toWebP(src)
	if out == nil {
		t.Fatal("not made a WebP")
	}
	d, err := webp.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	got := image.NewNRGBA(d.Bounds())
	draw.Draw(got, got.Bounds(), d, d.Bounds().Min, draw.Src)
	var se, n float64
	for i := 4; i < len(want.Pix); i += 4 {
		edge := 0
		for k := range 3 {
			edge += max(int(want.Pix[i+k])-int(want.Pix[i-4+k]), int(want.Pix[i-4+k])-int(want.Pix[i+k]))
		}
		if want.Pix[i+3] < 255 || edge < 90 {
			continue
		}
		for k := range 3 {
			e := float64(want.Pix[i+k]) - float64(got.Pix[i+k])
			se += e * e
			n++
		}
	}
	psnr := 10 * math.Log10(255*255/(se/n))
	t.Logf("the lines keep a PSNR of %.1f dB at %d bytes (PNG %d)", psnr, len(out), len(src))
	if psnr < 32 {
		t.Fatalf("the lines keep a PSNR of %.1f dB, not 32", psnr)
	}
}

func TestWebConversionPreservesCollidingDirectories(t *testing.T) {
	src, bundle := t.TempDir(), webBundle(t)
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "collision-test", Title: "Collision test"}); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"assets/note.txt":               []byte(strings.Repeat("A neutral note.\n", 100)),
		"assets/note.txt.gz/notice.txt": []byte("directory content"),
	} {
		if err := cli.WriteNew(filepath.Join(src, filepath.FromSlash(name)), b); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, bundle, "devel", ""); err != nil {
		t.Fatal(err)
	}
	files := manifest(t, out)
	if f := files["assets/note.txt"]; f.File != f.Name || f.Gzip {
		t.Fatalf("colliding conversion: %+v", f)
	}
	b, err := os.ReadFile(filepath.Join(out, "game", "assets", "note.txt.gz", "notice.txt")) //nolint:gosec // Test-owned exported file.
	if err != nil || string(b) != "directory content" {
		t.Fatalf("directory content lost: %q, %v", b, err)
	}
}

func TestWebRejectsOversizedPhoneBundles(t *testing.T) {
	b, err := os.ReadFile("../../../examples/office/game/apps/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var catalog gamepackage.Catalog
	if err := json.Unmarshal(b, &catalog); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile("../../../examples/office/game/apps/counter.zip")
	if err != nil {
		t.Fatal(err)
	}
	original := catalog.Releases[0]
	for _, tc := range []struct {
		name           string
		size, unpacked int64
		optional       bool
		wantError      bool
	}{
		{"valid bundle", original.Size, original.UnpackedSize, false, false},
		{"archive", gamepackage.MaxArchiveBytes + 1, gamepackage.MaxUnpackedBytes, false, true},
		{"expanded", gamepackage.MaxArchiveBytes, gamepackage.MaxUnpackedBytes + 1, false, true},
		{"optional native release", gamepackage.MaxArchiveBytes + 1, gamepackage.MaxUnpackedBytes, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &catalog.Releases[0]
			r.Size, r.UnpackedSize = tc.size, tc.unpacked
			encoded, err := json.Marshal(catalog)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(encoded)
			app := gamedata.PhoneApp{ID: "counter", Package: r.ID, Version: ">=1.0.0 <2.0.0", BundledVersion: r.Version, Install: "bundled", Path: "counter.zip"}
			if tc.optional {
				app.Install = "optional"
			}
			cfg := &gamedata.PhoneApps{Catalog: "catalog.json", CatalogSHA256: hex.EncodeToString(digest[:]), Apps: []gamedata.PhoneApp{app}}
			err = checkWebPhonePackages(t.Context(), fstest.MapFS{"catalog.json": {Data: encoded}, "counter.zip": {Data: archive}}, cfg)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestWebRejectsPhoneBundleEntryOverflow(t *testing.T) {
	b, err := os.ReadFile("../../../examples/office/game/apps/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := gamepackage.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	original, err := zip.OpenReader("../../../examples/office/game/apps/counter.zip")
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, file := range original.File {
		if err := writer.Copy(file); err != nil {
			t.Fatal(err)
		}
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	for i := range 4097 {
		if _, err := writer.Create(fmt.Sprintf("assets/empty-%d.txt", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	release := &catalog.Releases[0]
	release.Size = int64(archive.Len())
	sum := sha256.Sum256(archive.Bytes())
	release.SHA256 = hex.EncodeToString(sum[:])
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	cfg := &gamedata.PhoneApps{Catalog: "catalog.json", CatalogSHA256: hex.EncodeToString(digest[:]), Apps: []gamedata.PhoneApp{{ID: "counter", Package: release.ID, Version: release.Version, Install: "bundled", Path: "counter.zip"}}}
	fsys := fstest.MapFS{"catalog.json": {Data: encoded}, "counter.zip": {Data: archive.Bytes()}}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "counter.zip"), archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := gamepackage.OpenArchive(t.Context(), os.DirFS(dir), "counter.zip", *release)
	if err != nil {
		t.Fatalf("native archive unexpectedly rejected: %v", err)
	}
	if err := installed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := checkWebPhonePackages(t.Context(), fsys, cfg); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("browser entry overflow: %v", err)
	}
}
