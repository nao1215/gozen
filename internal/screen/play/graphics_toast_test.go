//go:build graphics

package play

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestGraphicsToastsStayReadableOnGlass shows a guide's line and then an inbox
// notification on a dark screen under a nearly clear glass (ui.material opacity 0.1):
// their cards stay nearly opaque, so the trace's layout lines measure their names and
// lines at a WCAG contrast of 4.5:1 at least on what is behind them.
func TestGraphicsToastsStayReadableOnGlass(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, gameFiles(t)); err != nil {
		t.Fatal(err)
	}
	gameRoot, err := os.OpenRoot(game)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := gameRoot.Close(); err != nil {
			t.Error(err)
		}
	})
	write := func(path string, b []byte) {
		t.Helper()
		if err := gameRoot.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dark := image.NewNRGBA(image.Rect(0, 0, 64, 36))
	draw.Draw(dark, dark.Bounds(), &image.Uniform{color.NRGBA{R: 24, G: 28, B: 36, A: 255}}, image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := png.Encode(&b, dark); err != nil {
		t.Fatal(err)
	}
	write("assets/dark.png", b.Bytes())
	write("game.yaml", fmt.Appendf(nil, `schema_version: 11
id: toasts
%s
fonts: {regular: test.ttf}
screen: {width: 1280, height: 720}
ui:
  material: {kind: glass, opacity: 0.1, blur: 4}
planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
screens:
  board:
    title: Board
    kind: workspace
    view: {fullscreen: true, background: dark.png}
    entries:
    - {label: Hours, style: plate, bounds: [40, 150, 200, 100]}
    guide:
      steps:
      - {who: guide, text: 'Read the board before you act.'}
  desk:
    title: Desk
    kind: workspace
    view: {fullscreen: true, background: dark.png}
    entries:
    - {label: Mail, style: plate, bounds: [40, 150, 200, 100]}
`, testgame.Game))
	write("data/toasts.yaml", []byte(`events:
  ping:
    do:
      - message: {who: scout, text: 'The kiln is late again.', event: brief}
      - open: desk
  brief:
    do:
      - say: {who: scout, text: Noted.}
`))
	script := filepath.Join(root, "toasts.txt")
	if err := os.WriteFile(script, []byte("guides on\ndo open: board\nwait 60\npress ok\nwait 30\npress x\nwait 30\ndo call: ping\nwait 60\nquit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(root, "toasts.jsonl")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "camp", //nolint:gosec // Fixed test host and isolated fixture paths.
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	raw, err := os.ReadFile(trace) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	type text struct {
		Text  string  `json:"text"`
		Back  string  `json:"back"`
		Ratio float64 `json:"ratio"`
	}
	watched := map[string]bool{"Read the board before you act.": true, "Guide": true, "The kiln is late again.": true, "Scout": true}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if !strings.Contains(line, `"t":"layout"`) {
			continue
		}
		var l struct {
			Texts []text `json:"texts"`
		}
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatal(err)
		}
		for _, x := range l.Texts {
			if !watched[x.Text] || x.Back == "" {
				continue
			}
			seen[x.Text] = true
			if x.Ratio < 4.5 {
				t.Errorf("%q on %s: contrast %.2f, want 4.5 at least", x.Text, x.Back, x.Ratio)
			}
		}
	}
	for _, want := range []string{"Read the board before you act.", "The kiln is late again."} {
		if !seen[want] {
			t.Errorf("no layout text %q in the trace:\n%s", want, raw)
		}
	}
}
