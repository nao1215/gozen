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
// decoded in the background at the speed of the machine, are left out. Save-write
// records are compared without their wall-clock time.
func TestGraphicsSimulateMatchesRun(t *testing.T) {
	for _, c := range []struct {
		name, fixture string
		font          bool
		args          []string
		opts          Options
		script        string
		state         string
		outcome       string
		result        string
	}{
		{
			name: "date card retains pick", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo open: board\nwait 10\npick Next phase\npress ok\npick Count\nwait 10\nsnapshot\nshot counted\nquit\n",
			state:  "counted",
		},
		{
			name: "phone lock timeout", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo call: phone_timeout\nwait 35\nsnapshot\nshot timeout\nwait 100\nsnapshot\nquit\n", state: "finished",
			outcome: "timeout", result: "retry",
		},
		{
			name: "waiting banner timeout", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo call: phone_waiting_banner\nwait 35\nsnapshot\nshot banner\nwait 100\nsnapshot\nquit\n", state: "finished",
			outcome: "notice", result: "expired",
		},
		{
			name: "phone page timeout", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo call: phone_page\nwait 35\nsnapshot\nshot page\nwait 100\nsnapshot\nquit\n", state: "finished",
			outcome: "page", result: "unread",
		},
		{
			name: "banner keyboard open", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo call: phone_waiting_banner\nwait 25\nsnapshot\nshot banner\npress ok\nwait 30\nsnapshot\nquit\n", state: "finished",
			outcome: "notice", result: "open",
		},
		{
			name: "banner keyboard dismiss", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo call: phone_waiting_banner\nwait 25\nsnapshot\nshot banner\npress cancel\nwait 30\nsnapshot\nquit\n", state: "finished",
			outcome: "notice", result: "dismiss",
		},
		{
			name: "phone lines finish chat", fixture: "../../e2e/atago/fixtures/input-readiness",
			args: []string{"--start", "studio"}, opts: Options{Location: "studio"},
			script: "wait 20\ndo open: board\nwait 10\npick Chat\nlines chat\nsnapshot\nshot chatted\nquit\n",
			state:  "chatted",
		},
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
			if c.state != "" {
				for name, trace := range map[string][]byte{"run": ran, "sim": sim.Bytes()} {
					lines := bytes.Split(bytes.TrimSpace(trace), []byte("\n"))
					var end struct {
						Type  string `json:"t"`
						State struct {
							Vars map[string]any `json:"vars"`
						} `json:"state"`
					}
					if err := json.Unmarshal(lines[len(lines)-1], &end); err != nil {
						t.Fatal(err)
					}
					if c.outcome != "" {
						if end.State.Vars[c.outcome] != c.result {
							t.Fatalf("%s outcome: want %s=%s, got %v", name, c.outcome, c.result, end.State.Vars)
						}
						snapshots := traceLines(t, trace, "snapshot")
						if len(snapshots) != 2 {
							t.Fatalf("%s: want snapshots before and after the outcome, got %d", name, len(snapshots))
						}
						state, ok := snapshots[0]["state"].(map[string]any)
						if !ok {
							t.Fatalf("%s snapshot lacks state: %v", name, snapshots[0])
						}
						before, ok := state["vars"].(map[string]any)
						if !ok {
							t.Fatalf("%s snapshot lacks vars: %v", name, state)
						}
						if before[c.state] != nil || before[c.outcome] != nil {
							t.Fatalf("%s continued before expiry: %v", name, before)
						}
					}
					if end.Type != "end" || end.State.Vars[c.state] != float64(1) {
						t.Fatalf("%s final state: want %s=1: %s", name, c.state, lines[len(lines)-1])
					}
				}
			}
			want, got := simulateComparable(t, ran, c.state == ""), simulateComparable(t, sim.Bytes(), c.state == "")
			minimum := 10
			if c.outcome != "" {
				// Timeout scripts have no input and emit nine comparable records
				// without the redundant dev_do. Their before/after state is checked above.
				minimum = 9
			}
			if len(want) < minimum {
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

// simulateComparable is the trace's lines but those of presentation and music, each with its
// tick when requested; the end line without its count of lines and save_write
// without its wall-clock time. Presentation
// readiness can add render-wait ticks without changing the event sequence.
func simulateComparable(t *testing.T, b []byte, ticks bool) []string {
	t.Helper()
	var out []string
	for l := range strings.Lines(string(b)) {
		var m traceLine
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%v: %q", err, l)
		}
		switch m["t"] {
		case "shot", "view", "viewport", "draw_skip", "stand", "layout", "menu", "music", "start":
			continue
		case "end":
			m = maps.Clone(m)
			delete(m, "lines")
		case "save_write":
			delete(m, "time")
		}
		line := comparableLine(m)
		if ticks {
			line += "@" + jsonOf(m["tick"])
		}
		out = append(out, line)
	}
	return out
}
