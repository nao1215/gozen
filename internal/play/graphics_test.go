//go:build graphics

package play

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// hostBin is testdata/host, built once by TestMain for every scenario.
var hostBin string

func TestGraphicsScreensDungeonSnap(t *testing.T) {
	root := t.TempDir()
	game, saves := filepath.Join(root, "game"), filepath.Join(root, "saves")
	if err := os.CopyFS(game, wldWorldFiles(t, "", nil)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(saves, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saves, "config.json"), []byte(`{"dungeon_snap":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var script strings.Builder
	script.WriteString("gap 1\nwait 40\nshot initial\n")
	for _, key := range []string{"up", "left", "right", "down"} {
		fmt.Fprintf(&script, "press %s\nshot %s-early\nwait 3\nshot %s-middle\nwait 20\nshot %s-settled\n", key, key, key, key)
	}
	script.WriteString("press reload\nwait 40\nshot reloaded\nquit\n")
	path := filepath.Join(root, "script.txt")
	if err := os.WriteFile(path, []byte(script.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--start", "fx_maze", "--calm", "--mute", "--script", path, "--shots", root, "--save-dir", saves) //nolint:gosec // Fixed host and private fixture paths.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	// Compare the rendered view, excluding UI overlays at the screen edges.
	// Re-uploading textures may change a channel by one byte through GPU rounding.
	equalView := func(a, b image.Image) bool {
		bounds := a.Bounds()
		for y := bounds.Dy() / 3; y < bounds.Dy()*2/3; y++ {
			for x := bounds.Dx() / 3; x < bounds.Dx()*2/3; x++ {
				ar, ag, ab, aa := a.At(x, y).RGBA()
				br, bg, bb, ba := b.At(x, y).RGBA()
				if abs(int(ar)-int(br)) > 257 || abs(int(ag)-int(bg)) > 257 || abs(int(ab)-int(bb)) > 257 || aa != ba {
					return false
				}
			}
		}
		return true
	}
	for _, key := range []string{"up", "left", "right", "down"} {
		settled := shotImage(t, root, key+"-settled")
		for _, phase := range []string{"early", "middle"} {
			if !equalView(shotImage(t, root, key+"-"+phase), settled) {
				t.Errorf("%s %s contains a transition instead of the current view", key, phase)
			}
		}
	}
	if equalView(shotImage(t, root, "initial"), shotImage(t, root, "up-settled")) {
		t.Fatal("forward movement did not change the rendered scene")
	}
	if !equalView(shotImage(t, root, "down-settled"), shotImage(t, root, "reloaded")) {
		t.Fatal("live reload changed the settled view")
	}
}

// TestMain builds testdata/host once: every scenario runs it as a process of its own,
// so the scenarios run side by side (t.Parallel) instead of each waiting for go run.
func TestMain(m *testing.M) {
	code, err := buildHostAndRun(m)
	if err != nil {
		if _, werr := fmt.Fprintln(os.Stderr, err); werr != nil {
			os.Exit(2)
		}
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func buildHostAndRun(m *testing.M) (code int, err error) {
	dir, err := os.MkdirTemp("", "gozen-graphics-")
	if err != nil {
		return 1, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	hostBin = filepath.Join(dir, "host")
	if runtime.GOOS == "windows" {
		hostBin += ".exe"
	}
	build := exec.CommandContext(context.Background(), "go", "build", "-o", hostBin, "./testdata/host") //nolint:gosec // Fixed Go executable building the test host.
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return 1, fmt.Errorf("build testdata/host: %w", err)
	}
	return m.Run(), nil
}

// Each scenario gets a fresh process because Ebitengine supports one loop per
// process. PNGs are produced by ReadPixels inside Draw, never outside the loop.
func TestGraphicsScreens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		args   []string
		script string
	}{
		{name: "title", args: nil},
		{name: "field", args: []string{"--start", "inn"}},
		{name: "dialogue", args: []string{"--start", "plaza", "--event", "kes_roof"}},
		{name: "battle", args: []string{"--start", "b1", "--party", "maren,ox,tilly", "--battle", "drips"}},
		{name: "dungeon", args: []string{"--start", "b1", "--party", "maren,ox,tilly"}},
		{name: "recruit", args: []string{"--start", "b1", "--party", "maren,ox,tilly,kes", "--level", "6"}, script: "guarantee-recruit drip_slime\ndo battle: drip_and_rat\nplay join 5\nwait 30\nshot frame\nquit\n"},
		// F7 balance display. Wait until measuring finishes, and check that the win-rate bar color appears in the upper-right panel.
		{name: "balance", args: []string{"--start", "b2", "--party", "maren,ox,tilly", "--level", "1"}, script: "wait 20\npress graphs\nwait-graphs\nshot frame\nquit\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			script := filepath.Join(root, "script.txt")
			body := tc.script
			if body == "" {
				body = "wait 80\nshot frame\nquit\n"
			}
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			// the Go Zen sample (1920x1080) at half size in the window
			args := []string{"--dir", "../../examples/go-zen/game", "--dev", "--mute", "--seed", "7", "--window", "0.5", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")}
			args = append(args, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			b, err := os.ReadFile(filepath.Join(root, "frame.png")) //nolint:gosec // Fixed basename inside this test's private temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 1920 || img.Bounds().Dy() != 1080 { // the screen of Go Zen, whatever the window
				t.Fatalf("frame bounds %v", img.Bounds())
			}
			colors := map[[4]uint32]bool{}
			for y := 0; y < 1080; y += 6 {
				for x := 0; x < 1920; x += 6 {
					r, g, b, a := img.At(x, y).RGBA()
					colors[[4]uint32{r, g, b, a}] = true
				}
			}
			if len(colors) < 8 {
				t.Fatalf("screen lacks rendered content: %d colors", len(colors))
			}
			if tc.name == "balance" && !hasBar(img) {
				t.Fatal("the balance panel shows no win-rate bar")
			}
			if tc.name == "recruit" && !hasRecruitMonster(img) {
				t.Fatal("recruit question has no large blue monster in the center")
			}
		})
	}
}

// hasBar checks whether the upper-right balance display has the first troop's bar color (around #6cc4f0).
func hasBar(img image.Image) bool {
	for y := 24; y < 560; y++ {
		for x := 1000; x < 1900; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if nearColor(r>>8, 0x6c) && nearColor(g>>8, 0xc4) && nearColor(b>>8, 0xf0) {
				return true
			}
		}
	}
	return false
}

func nearColor(v, want uint32) bool { return v+8 >= want && v <= want+8 }

func hasRecruitMonster(img image.Image) bool {
	blue := 0
	for y := 280; y < 680; y += 4 {
		for x := 700; x < 1220; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			if b > r*3/2 && g > r*5/4 && b > 0x5000 {
				blue++
			}
		}
	}
	return blue > 250
}

// logGraphicsTiming separates fixed-script elapsed time from sampled rendering speed.
func logGraphicsTiming(t *testing.T, started time.Time, diagnostic string) {
	t.Helper()
	sample := ""
	for line := range strings.SplitSeq(diagnostic, "\n") {
		if strings.Contains(line, " fps ") && strings.Contains(line, " tps ") {
			sample = line
		}
	}
	if sample == "" {
		t.Fatal("graphics run did not report FPS/TPS")
	}
	t.Logf("fixed-script elapsed %s; last frame sample: %s", time.Since(started).Round(time.Millisecond), sample)
}

func TestGraphicsScreensSaveRandomContinuation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game, saves := filepath.Join(root, "game"), filepath.Join(root, "saves")
	if err := os.CopyFS(game, btlFiles(t, "", "")); err != nil {
		t.Fatal(err)
	}
	run := func(name, script string, args ...string) []byte {
		t.Helper()
		path, trace := filepath.Join(root, name+".txt"), filepath.Join(root, name+".jsonl")
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		argv := []string{"--dir", game, "--dev", "--mute", "--script", path, "--trace", trace, "--shots", root, "--save-dir", saves}
		argv = append(argv, args...)
		cmd := exec.CommandContext(ctx, hostBin, argv...) //nolint:gosec // Fixed test host and isolated fixture paths.
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\nstdout: %s\nstderr: %s", name, err, &stdout, &stderr)
		}
		b, err := os.ReadFile(trace) //nolint:gosec // Generated trace under the isolated test directory.
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	direct := run("direct", "wait 20\ndo open: save\nwait 10\npress ok\nwait 100\nwait 20\ndo battle: two\npress ok 160\nquit\n", "--start", "camp", "--party", "scout,guide", "--seed", "31")
	saved, err := os.ReadFile(filepath.Join(saves, "save1.json")) //nolint:gosec // Generated save under the isolated test directory.
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Rand   []byte
		Cursor json.RawMessage
	}
	if err := json.Unmarshal(saved, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Rand) == 0 || len(envelope.Cursor) > 0 {
		t.Fatal("fixture did not save between events with random state")
	}
	// Continue selects the newest save, which is the post-battle autosave.
	// Move back to the manual slot containing the pre-battle state.
	loaded := run("loaded", "wait 20\npress ok\nwait 20\npress up 3\npress ok\nwait 40\ndo battle: two\npress ok 160\nquit\n", "--seed", "999")
	for i, trace := range [][]byte{direct, loaded} {
		captures := traceLines(t, trace, "battle_capture")
		if len(captures) != 1 {
			t.Fatalf("run %d capture count %d; requests: %v", i, len(captures), traceLines(t, trace, "request", "act"))
		}
		raw, err := json.Marshal(captures[0]["capture"])
		if err != nil {
			t.Fatal(err)
		}
		var capture struct{ Rand []byte }
		if err := json.Unmarshal(raw, &capture); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(capture.Rand, envelope.Rand) {
			t.Fatalf("run %d: battle random state %x, saved %x", i, capture.Rand, envelope.Rand)
		}
	}
	a, b := traceLines(t, direct, "ai_decision", "battle_step", "battle_result"), traceLines(t, loaded, "ai_decision", "battle_step", "battle_result")
	for _, lines := range [][]traceLine{a, b} {
		for _, line := range lines {
			delete(line, "tick")
		}
	}
	if len(traceLines(t, direct, "battle_result")) != 1 || !reflect.DeepEqual(a, b) {
		t.Fatal("native load menu changed deterministic battle continuation")
	}
}

func TestGraphicsScreensCasualtyPolicy(t *testing.T) {
	t.Parallel()
	for _, recoverHP := range []bool{false, true} {
		t.Run(map[bool]string{true: "recover", false: "preserve"}[recoverHP], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			game, saves := filepath.Join(root, "game"), filepath.Join(root, "saves")
			content := fmt.Sprintf(`events:
  expedition:
    do:
      - battle: doom
        fighters: [guide]
        retry: false
        recover: %v
        on_lose:
          - set: {var.losses: "var.losses + 1", var.final_hp: "last_battle.hp.guide"}
      - say: "The expedition ended."
      - save: true
`, recoverHP)
			if err := os.CopyFS(game, btlFiles(t, "", content)); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "script.txt")
			script := "wait 20\ndo call: expedition\npress ok 180\nwait 100\nquit\n"
			if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--start", "camp", "--party", "scout,guide", "--seed", "31", "--script", path, "--shots", root, "--save-dir", saves) //nolint:gosec // Fixed test host and isolated fixture paths.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("host: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			raw, err := os.ReadFile(filepath.Join(saves, "save1.json")) //nolint:gosec // Generated save in the isolated fixture.
			if err != nil {
				t.Fatal(err)
			}
			var save struct {
				State struct {
					Actors     map[string]struct{ HP int }
					Vars       map[string]any
					LastBattle struct {
						Result string
						HP     map[string]int
					} `json:"last_battle"`
				}
			}
			if err := json.Unmarshal(raw, &save); err != nil {
				t.Fatal(err)
			}
			want := 0
			if recoverHP {
				want = 1
			}
			st := save.State
			if st.Actors["guide"].HP != want || st.Actors["scout"].HP <= 1 || st.Vars["losses"] != float64(1) || st.LastBattle.Result != "lose" || len(st.LastBattle.HP) != 1 || st.LastBattle.HP["guide"] != 0 {
				t.Fatalf("saved outcome: %+v", st)
			}
		})
	}
}
