package distribute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
)

func TestWebManifestAndRuntimeValidation(t *testing.T) {
	src, bundle := t.TempDir(), t.TempDir()
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "web-test", Title: "Web test"}); err != nil {
		t.Fatal(err)
	}
	// A minimal WASM header tests packaging only. The browser E2E runs the real player.
	wasm := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	js := []byte("// neutral runtime fixture\n")
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
	var entries []struct {
		Name, File string
		Size       int64
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		info, err := os.Stat(filepath.Join(out, "game", filepath.FromSlash(entry.File)))
		if err != nil || entry.Name != entry.File || info.Size() != entry.Size {
			t.Fatalf("manifest entry %+v: %v", entry, err)
		}
		if entry.Name == "assets/space #%.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("special-character asset missing from manifest")
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	// the page shows how much of the player and of the game's files has come
	for _, want := range []string{`id="fill"`, `id="amount"`, "window.goZenProgress", "response.body.getReader()", `goZenProgress("player"`} {
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
