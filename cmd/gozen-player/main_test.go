//go:build !js

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserPlayerBuildAndStartup(t *testing.T) {
	b, err := os.ReadFile("main_js.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	start, run := strings.Index(source, "g.StartRendering()"), strings.Index(source, "ebiten.RunGame(")
	if start < 0 || run < start || !strings.Contains(source, "g.Close()") {
		t.Fatal("browser must initialize rendering before the loop and close afterward")
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", filepath.Join(t.TempDir(), "player.wasm"), ".") //nolint:gosec // Fixed build command and test-owned output path.
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
