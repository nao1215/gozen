//go:build graphics

package play

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

// TestGraphicsSimulateMatchesRun plays the same script with the same seed twice: in a
// window through the game's --dev --script (as gozen run does), and with Simulate. The
// lines of what happened (talk, choices, battles, dungeon steps, campaign orders and turns,
// inputs, scenes, saves and states) are the same, tick for tick, and so is the state at the
// end. Only the lines of drawing (shot, view, stand, menu) and the music, whose songs are
// decoded in the background at the speed of the machine, are left out.
func TestGraphicsSimulateMatchesRun(t *testing.T) {
	for _, c := range []struct {
		name, fixture string
		font          bool
		args          []string
		opts          Options
		script        string
	}{
		{
			name: "talk and a choice", fixture: "testdata/novel", font: true,
			args: []string{"--start", "room"}, opts: Options{Location: "room"},
			script: "gap 4\nwait 20\ndo call: story\nread 4\nread 4\nread 4\nread 4\npress down\nwait 6\npress ok\nuntil idle\nshot after\nquit\n",
		},
		{
			name: "a campaign order and the turn", fixture: "testdata/campaign", font: true,
			args: []string{"--start", "war"}, opts: Options{Location: "war"},
			script: "wait 40\norder storm hero keep\nwait 80\nuntil idle\nend_turn\nuntil idle\nwait 120\nshot turn\nquit\n",
		},
		{
			name: "dungeon steps and battles", fixture: "../../e2e/atago/fixtures/game",
			args: []string{"--start", "grotto", "--party", "scout", "--level", "3"}, opts: Options{Location: "grotto", Party: []string{"scout"}, Level: 3},
			script: "gap 4\nwait 30\npress up\nwait 10\npress right\nwait 10\npress up\nwait 10\npress up\nuntil idle\ndo battle: lesson\nuntil idle\nuntil idle\nshot deep\nquit\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "game")
			if err := os.CopyFS(game, os.DirFS(c.fixture)); err != nil {
				t.Fatal(err)
			}
			if c.font {
				if err := os.MkdirAll(filepath.Join(game, "assets"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
					t.Fatal(err)
				}
				var fig bytes.Buffer
				if err := png.Encode(&fig, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(game, "assets", "fig.png"), fig.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			script := filepath.Join(root, "script.txt")
			if err := os.WriteFile(script, []byte(c.script), 0o600); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(root, "run.jsonl")
			args := append([]string{
				"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7",
				"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace,
			}, c.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			ran, err := os.ReadFile(trace) //nolint:gosec // This test's own trace.
			if err != nil {
				t.Fatal(err)
			}
			var sim bytes.Buffer
			opts := c.opts
			opts.FS, opts.Seed, opts.Script, opts.Trace = os.DirFS(game), 7, c.script, &sim
			if _, err := Simulate(context.Background(), opts, SimulateConfig{}); err != nil {
				t.Fatal(err)
			}
			want, got := simulateComparable(t, ran), simulateComparable(t, sim.Bytes())
			if len(want) < 10 {
				t.Fatalf("the run wrote only %d lines to compare", len(want))
			}
			for i := range max(len(want), len(got)) {
				if i >= len(want) || i >= len(got) || want[i] != got[i] {
					w, g := "(none)", "(none)"
					if i < len(want) {
						w = want[i]
					}
					if i < len(got) {
						g = got[i]
					}
					t.Fatalf("line %d of %d/%d differs:\nrun %s\nsim %s", i+1, len(want), len(got), w, g)
				}
			}
			if !strings.Contains(string(ran), `"t":"shot"`) {
				t.Error("the run took no screenshots")
			}
		})
	}
}

// simulateComparable is the trace's lines but those of drawing and music, each with its
// tick; the end line without its count of lines.
func simulateComparable(t *testing.T, b []byte) []string {
	t.Helper()
	var out []string
	for l := range strings.Lines(string(b)) {
		var m traceLine
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%v: %q", err, l)
		}
		switch m["t"] {
		case "shot", "view", "stand", "menu", "music", "start":
			continue
		case "end":
			m = maps.Clone(m)
			delete(m, "lines")
		}
		out = append(out, comparableLine(m)+"@"+jsonOf(m["tick"]))
	}
	return out
}
