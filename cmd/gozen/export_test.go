package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/distribute"
	"github.com/nao1215/gozen/internal/clihelp"
)

func TestExportWebReportsPublishedDirectory(t *testing.T) {
	game, bundle := fixture(t), t.TempDir()
	// A minimal WASM header exercises packaging only, not player execution.
	wasm, js := "\x00asm\x01\x00\x00\x00", "// neutral runtime fixture\n"
	hash := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	}
	metadata, err := json.Marshal(distribute.Runtime{
		Version: clihelp.Version(), Target: "js-wasm", Player: "gozen-player.wasm",
		SHA256: hash(wasm), WASMExecSHA256: hash(js),
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"runtime.json": string(metadata), "gozen-player.wasm": wasm, "wasm_exec.js": js,
		"LICENSE": "fixture license", "GO-LICENSE": "fixture license", "THIRD_PARTY_LICENSES/notice.txt": "fixture notice",
	} {
		write(t, filepath.Join(bundle, name), content)
	}
	reuse := filepath.Join(t.TempDir(), "reuse-export")
	var earlier string
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "published")
			args := []string{"export", game, "--runtime", bundle, "--target", "web", "--out", destination, "--format", format}
			if earlier != "" {
				args = append(args, "--reuse", earlier)
			}
			out, stderr := invoke(t, 0, args...)
			if stderr != "" {
				t.Fatalf("successful export wrote stderr: %s", stderr)
			}
			if format == "text" {
				if out != "created "+destination+" for web\n" {
					t.Fatalf("text report: %q", out)
				}
			} else {
				var result struct {
					Directory string `json:"directory"`
					Target    string `json:"target"`
				}
				if err := json.Unmarshal(resultData(t, out, "export"), &result); err != nil {
					t.Fatal(err)
				}
				if result.Directory != destination || result.Target != "web" {
					t.Fatalf("machine report: %+v", result)
				}
			}
			for _, name := range []string{"index.html", "runtime.json", "gozen-player.wasm.gz", "game/files.json"} {
				if len(read(t, filepath.Join(destination, name))) == 0 {
					t.Errorf("empty published file %s", name)
				}
			}
			// The first export belongs to the parent test so reuse can outlive a subtest.
			if earlier == "" {
				earlier = reuse
				if err := os.CopyFS(earlier, os.DirFS(destination)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	write(t, filepath.Join(bundle, "wasm_exec.js"), "changed runtime")
	parent := t.TempDir()
	out, stderr := invoke(t, 1, "export", game, "--runtime", bundle, "--target", "web", "--out", filepath.Join(parent, "failed"))
	if out != "" || !strings.Contains(stderr, "checksum mismatch") {
		t.Fatalf("failed export report: %q, %q", out, stderr)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed export left files: %v, %v", entries, err)
	}
}

func TestExportRejectsInvalidOptionsBeforePublishing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		code    int
		message string
	}{
		{"missing-runtime", nil, 2, "requires --runtime"},
		{"unknown-target", []string{"--runtime", "missing", "--target", "unknown"}, 2, "unsupported export target"},
		{"desktop-reuse", []string{"--runtime", "missing", "--target", "linux-amd64", "--reuse", "missing"}, 2, "--reuse is for --target web"},
		{"missing-bundle", []string{"--runtime", "missing", "--target", "linux-amd64"}, 1, "runtime.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			args := append([]string{"export", "missing-game", "--out", filepath.Join(parent, "output")}, tc.args...)
			out, stderr := invoke(t, tc.code, args...)
			if out != "" || !strings.Contains(stderr, tc.message) {
				t.Fatalf("invalid export report: %q, %q", out, stderr)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid export left files: %v, %v", entries, err)
			}
		})
	}
}
