package distribute

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepteams/webp"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
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
	if err := Web(src, out, bundle, "devel"); err != nil {
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
	if err := Web(src, filepath.Join(parent, "site"), bundle, "devel"); err == nil || !strings.Contains(err.Error(), "checksum") {
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
	if err := Web(src, out, webBundle(t), "devel"); err != nil {
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
