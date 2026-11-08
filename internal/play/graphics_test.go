//go:build graphics

package play

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
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

// TestGraphicsScreensRepro runs the exact printed shell command, then compares
// pixels rather than PNG compression. Each CLI process owns one graphics loop.
func TestGraphicsScreensRepro(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("printed POSIX shell command requires sh")
	}
	root := t.TempDir()
	// Opt-in artifacts survive t.TempDir cleanup so a coordinator can inspect
	// intermittent pixel differences after running the real graphics loop.
	if os.Getenv("GOZEN_REPRO_ARTIFACTS") == "1" {
		var err error
		root, err = os.MkdirTemp("", "screens-repro-") //nolint:usetesting // Explicitly retained for inspection after the test exits.
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("reproduction artifacts: %s", root)
	}
	binary := filepath.Join(root, "gozen")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/gozen") //nolint:gosec // Builds the CLI into the test directory.
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // This test owns the CLI binary and its fixture arguments.
		var out, diag bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &diag
		if err := cmd.Run(); err != nil {
			t.Fatalf("gozen %v: %v\n%s\n%s", args, err, &out, &diag)
		}
		return out.String()
	}
	for _, mode := range []string{"novel", "script-pick", "timed-fade", "map-zoom", "campaign", "campaign-warm", "battle-puppet", "party-hit", "seek-1000", "native-window", "native-script", "guides"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(root, mode)
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			game := filepath.Join(dir, "game")
			fixture := mode
			if mode == "campaign-warm" {
				fixture = "campaign"
			}
			if mode == "script-pick" || mode == "timed-fade" || strings.HasPrefix(mode, "native-") || mode == "guides" {
				fixture = "novel"
			}
			source := os.DirFS(filepath.Join("testdata", fixture))
			if mode == "party-hit" {
				source = partyHitFiles(t)
			}
			if mode == "seek-1000" {
				source = gameFiles(t)
			}
			if mode == "map-zoom" {
				source = atlasCameraFiles(t)
			}
			if mode == "battle-puppet" {
				source = enemyPuppetFiles(t)
			}
			if err := os.CopyFS(game, source); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(game, "assets"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
				t.Fatal(err)
			}
			start := []string{"--start", "war"}
			body := "wait 40\npress ok\nwait 30\nshot frame\nquit\n"
			if mode == "map-zoom" {
				start = []string{"--start", "large"}
				body = "wait 20\npress map_zoom_in\nwait 60\nshot frame\nquit\n"
			}
			if mode == "campaign-warm" {
				// Exercise atlas promotion after many intermediate portrait draws;
				// uncapped seeking must retain exact pixel equality.
				body = "wait 180\npress ok\nwait 180\nshot frame\nquit\n"
			}
			if fixture == "novel" {
				writeNovelFigure(t, game)
			}
			if mode == "novel" {
				start = []string{"--start", "room"}
				body = "do call: story\nwait 40\npress ok\nwait 30\nshot frame\nquit\n"
			}
			if mode == "script-pick" {
				gameRoot, err := os.OpenRoot(game)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := gameRoot.Close(); err != nil {
						t.Error(err)
					}
				})
				original, err := gameRoot.ReadFile("game.yaml")
				if err != nil {
					t.Fatal(err)
				}
				if err := gameRoot.WriteFile("game.yaml", append(original, []byte(reproPickScreens)...), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := gameRoot.WriteFile("data/pick.yaml", []byte(reproPickEvent), 0o600); err != nil {
					t.Fatal(err)
				}
				start = []string{"--start", "room"}
				body = reproPickScript + "shot frame\nquit\n"
			}
			if mode == "timed-fade" {
				// Change the screen after seeking has begun, animate it, then trigger a
				// fade without input. Capture while the pre-transition image is frozen.
				event := `events:
  timed_fade:
    do:
      - wait: {seconds: 0.2, skip: false}
      - image: {file: fig.png, fade: 0.2}
      - wait: {seconds: 0.5, skip: false}
      - effect: fade
`
				if err := os.WriteFile(filepath.Join(game, "data", "timed_fade.yaml"), []byte(event), 0o600); err != nil {
					t.Fatal(err)
				}
				start = []string{"--start", "room"}
				body = "wait 40\ndo call: timed_fade\nwait 50\nshot frame\nquit\n"
			}
			if mode == "battle-puppet" {
				start = []string{"--start", "field", "--party", "scout,guide", "--battle", "one"}
				body = "wait 90\nshot frame\nquit\n"
			}
			if mode == "party-hit" {
				start = []string{"--start", "field", "--party", "scout", "--battle", "one"}
				body = partyHitScript
			}
			if mode == "seek-1000" {
				start = []string{"--start", "camp"}
				body = "wait 1010\nshot frame\nquit\n"
			}
			if strings.HasPrefix(mode, "native-") || mode == "guides" {
				gameRoot, err := os.OpenRoot(game)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := gameRoot.Close(); err != nil {
						t.Error(err)
					}
				})
				b, err := gameRoot.ReadFile("game.yaml")
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.Replace(b, []byte("schema_version: 4"), []byte("schema_version: 11"), 1)
				b = bytes.Replace(b, []byte("screen: {"), []byte("screen: {native: true, "), 1)
				if mode == "guides" {
					b = append(b, []byte(`
planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
screens:
  board:
    title: Board
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Hours, style: plate, bounds: [40, 150, 200, 100]}
    guide:
      steps:
      - {who: reader, text: 'Read the board.'}
`)...)
				}
				if err := gameRoot.WriteFile("game.yaml", b, 0o600); err != nil {
					t.Fatal(err)
				}
				start = []string{"--start", "room", "--window", "2"}
				body = "window 960 540\nwait 40\nshot frame\nquit\n"
				if mode == "guides" {
					body = "wait 40\nguides on\ndo open: board\nwait 40\nshot frame\nquit\n"
				}
			}
			script := filepath.Join(dir, "input.txt")
			// Let the opening settle, then capture an ordinary scripted frame.
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(dir, "run.jsonl")
			recordArgs := []string{"run", game, "--dev", "--mute", "--seed", "7", "--shots", dir, "--trace", trace, "--trace-shots", "--save-dir", filepath.Join(dir, "saves")}
			if mode == "native-window" {
				recordArgs = append(recordArgs, "--until", "75")
			} else {
				recordArgs = append(recordArgs, "--script", script)
			}
			run(t, append(recordArgs, start...)...)
			if mode == "battle-puppet" {
				for j, mutation := range []string{"recruit mote 1", "guarantee-recruit mote", "battle-hp enemy 0 1"} {
					input := filepath.Join(dir, fmt.Sprintf("mutation-%d.txt", j))
					log := filepath.Join(dir, fmt.Sprintf("mutation-%d.jsonl", j))
					if err := os.WriteFile(input, []byte("wait 40\n"+mutation+"\nwait 5\nshot mutation\nquit\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					run(t, append([]string{"run", game, "--dev", "--mute", "--seed", "7", "--script", input, "--trace", log, "--shots", dir, "--save-dir", filepath.Join(dir, "mutation-saves")}, start...)...)
					for _, args := range [][]string{
						{"trace", log, "--repro", "mutation.png"},
						{"run", game, "--dev", "--mute", "--replay", log},
					} {
						cmd := exec.CommandContext(t.Context(), binary, args...) //nolint:gosec // Test-owned CLI and trace.
						var stdout, stderr bytes.Buffer
						cmd.Stdout, cmd.Stderr = &stdout, &stderr
						kind := "unsupported_script"
						if strings.HasPrefix(mutation, "battle-hp ") {
							kind = "battle_setup"
						}
						err := cmd.Run()
						var exit *exec.ExitError
						if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "cannot reproduce exactly: "+kind+" at tick ") {
							t.Fatalf("%s: gozen %v: %v\n%s\n%s", mutation, args, err, &stdout, &stderr)
						}
					}
				}
			}
			targets := []string{"frame.png", "60"}
			if mode == "party-hit" {
				targets = []string{"frame.png", "settled.png"}
			}
			if mode == "seek-1000" {
				targets = []string{"1000"}
			}
			if mode == "guides" || mode == "native-script" {
				targets = []string{"frame.png", "frame.png"}
			}
			if mode == "native-window" {
				targets = []string{"until_000075.png", "until_000075.png"}
			}

			data, err := os.ReadFile(trace) //nolint:gosec // Reads this test's own trace.
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range traceLines(t, data, "view") {
				if shot, ok := line["shot"].(string); ok && shot != "" && mode != "seek-1000" {
					targets = append(targets, shot)
					break
				}
			}
			for i, target := range targets {
				args := []string{"trace", trace, "--repro", target, "--format", "json"}
				if mode == "campaign" || (i == 1 && (mode == "guides" || strings.HasPrefix(mode, "native-"))) {
					args = append(args, "--out", filepath.Join(dir, fmt.Sprintf("bundle-%d", i)))
				}
				if mode == "native-script" || mode == "guides" {
					cmd := exec.CommandContext(t.Context(), binary, args...) //nolint:gosec // Test-owned CLI and fixture arguments.
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err := cmd.Run()
					var exit *exec.ExitError
					want := "cannot reproduce exactly: the recording changed the window size [GZN1401]"
					if mode == "guides" {
						guides := traceLines(t, data, "guide")
						if len(guides) == 0 {
							t.Fatal("recording has no guide to refuse")
						}
						want = fmt.Sprintf("cannot reproduce exactly: guide at tick %v [GZN1401]", guides[0]["tick"])
					}
					if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 {
						t.Fatalf("reproduction refusal: %v stdout=%s stderr=%s", err, &stdout, &stderr)
					}
					var diagnostic struct {
						Version  int    `json:"version"`
						Command  string `json:"command"`
						Severity string `json:"severity"`
						Code     string `json:"code"`
						Message  string `json:"message"`
					}
					if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil {
						t.Fatalf("reproduction diagnostic: %v: %s", err, &stderr)
					}
					if diagnostic.Version != 1 || diagnostic.Command != "trace" || diagnostic.Severity != "error" || diagnostic.Code != "GZN1401" || diagnostic.Message != want {
						t.Fatalf("unexpected reproduction diagnostic: %s", &stderr)
					}
					continue
				}
				out := run(t, args...)
				var result struct {
					Data struct {
						Command    string `json:"command"`
						Screenshot string `json:"screenshot"`
					} `json:"data"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.Command == "" {
					t.Fatalf("missing command: %s", out)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
				cmd := exec.CommandContext(ctx, "sh", "-c", result.Data.Command) //nolint:gosec // Executes the command generated from this test's own trace.
				cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				started := time.Now()
				err := cmd.Run()
				elapsed := time.Since(started)
				cancel()
				if err != nil {
					t.Fatalf("reproduce: %v\n%s\n%s", err, &stdout, &stderr)
				}
				if mode == "seek-1000" {
					t.Logf("seek 1000 ticks (including process startup and PNG): %s", elapsed)
					if baseline := os.Getenv("GOZEN_REPRO_BASELINE"); baseline != "" {
						ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
						before := exec.CommandContext(ctx, baseline, "run", game, "--dev", "--mute", "--replay", trace, "--until", "1000", "--shots", filepath.Join(dir, "before"), "--save-dir", filepath.Join(dir, "before-saves")) //nolint:gosec // Explicit test binary supplied by the coordinator.
						before.Stdout, before.Stderr = &stdout, &stderr
						started := time.Now()
						err := before.Run()
						duration := time.Since(started)
						cancel()
						if err != nil {
							t.Fatalf("baseline seek: %v\n%s\n%s", err, &stdout, &stderr)
						}
						t.Logf("baseline seek 1000 ticks (including process startup and PNG): %s", duration)
					}
				}
				if strings.HasSuffix(target, ".png") {
					originalPath := target
					if !filepath.IsAbs(originalPath) {
						originalPath = filepath.Join(dir, target)
					}
					original := shotImage(t, filepath.Dir(originalPath), strings.TrimSuffix(filepath.Base(originalPath), ".png"))
					reproduced := shotImage(t, filepath.Dir(result.Data.Screenshot), strings.TrimSuffix(filepath.Base(result.Data.Screenshot), ".png"))
					if !reflect.DeepEqual(original, reproduced) {
						var changed image.Rectangle
						count, maxDelta := 0, 0
						for y := original.Bounds().Min.Y; y < original.Bounds().Max.Y; y++ {
							for x := original.Bounds().Min.X; x < original.Bounds().Max.X; x++ {
								r, g, b, a := original.At(x, y).RGBA()
								rr, gg, bb, aa := reproduced.At(x, y).RGBA()
								if r == rr && g == gg && b == bb && a == aa {
									continue
								}
								count++
								changed = changed.Union(image.Rect(x, y, x+1, y+1))
								for _, delta := range []int{int(r) - int(rr), int(g) - int(gg), int(b) - int(bb), int(a) - int(aa)} {
									maxDelta = max(maxDelta, delta, -delta)
								}
							}
						}
						// Allow driver rounding by one 8-bit step in at most 0.05% of pixels.
						if original.Bounds() != reproduced.Bounds() || maxDelta > 257 || count > original.Bounds().Dx()*original.Bounds().Dy()/2000 {
							t.Fatalf("%s target %q: %d pixels differ in %v, max channel delta %d/65535; bounds %v/%v; original %s; reproduced %s", mode, target, count, changed, maxDelta, original.Bounds(), reproduced.Bounds(), originalPath, result.Data.Screenshot)
						}
					}
				}
			}
		})
	}
}

// Repeat the comparison spec's complete batch: added is the third job, after
// the dialogue has populated and released different rendering resources.
func TestGraphicsScreensCaptureIdenticalPixels(t *testing.T) {
	root := t.TempDir()
	artifacts := ""
	if os.Getenv("GOZEN_REPRO_ARTIFACTS") == "1" {
		artifacts = t.TempDir()
		// Save in the test directory first; retain a copy before its cleanup so
		// failures remain inspectable after go test exits.
		t.Cleanup(func() {
			retained, err := os.MkdirTemp("", "capture-pixel-diff-") //nolint:usetesting // Explicit opt-in retention after test cleanup.
			if err != nil {
				t.Error(err)
				return
			}
			if err := os.CopyFS(retained, os.DirFS(artifacts)); err != nil {
				t.Error(err)
			}
			t.Logf("capture pixel artifacts: %s", retained)
		})
	}
	binary := filepath.Join(root, "gozen")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/gozen") //nolint:gosec // Builds the CLI into the test directory.
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "captures.yaml")
	const jobs = `shots:
  - {id: same, start: camp, wait: 2}
  - {id: edited, event: finish, line: 1, wait: 2}
  - {id: added, start: camp, wait: 2}
`
	if err := os.WriteFile(config, []byte(jobs), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := map[string]image.Image{}
	for run := range 8 {
		out := filepath.Join(root, fmt.Sprintf("run-%d", run))
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		cmd := exec.CommandContext(ctx, binary, "capture", "../../e2e/atago/fixtures/game", "--config", config, "--out", out) //nolint:gosec // Test-owned binary, config and output; neutral repository fixture.
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			t.Fatalf("capture %d: %v\nstdout: %s\nstderr: %s", run, err, &stdout, &stderr)
		}
		for _, id := range []string{"same", "edited", "added"} {
			got := shotImage(t, out, id)
			if run == 0 {
				baseline[id] = got
				continue
			}
			want := baseline[id]
			if got.Bounds() != want.Bounds() {
				t.Fatalf("run %d job %s: bounds %v, want %v", run, id, got.Bounds(), want.Bounds())
			}
			changed := 0
			var differences strings.Builder
			var region image.Rectangle
			for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
				for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
					r, g, b, a := got.At(x, y).RGBA()
					wr, wg, wb, wa := want.At(x, y).RGBA()
					if r != wr || g != wg || b != wb || a != wa {
						changed++
						if artifacts != "" {
							fmt.Fprintf(&differences, "(%d,%d) baseline RGBA16=(%d,%d,%d,%d) actual RGBA16=(%d,%d,%d,%d)\n", x, y, wr, wg, wb, wa, r, g, b, a)
						}
						region = region.Union(image.Rect(x, y, x+1, y+1))
					}
				}
			}
			if changed != 0 {
				if artifacts != "" {
					prefix := filepath.Join(artifacts, fmt.Sprintf("run-%d-%s", run, id))
					for _, pic := range []struct {
						suffix string
						img    image.Image
					}{{"baseline", want}, {"actual", got}} {
						var buf bytes.Buffer
						if err := png.Encode(&buf, pic.img); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(prefix+"-"+pic.suffix+".png", buf.Bytes(), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.WriteFile(prefix+"-diff.txt", []byte(differences.String()), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				t.Errorf("run %d job %s: %d pixels differ in %v", run, id, changed, region)
			}
		}
	}
}

// TestGraphicsScreensPhoneChrome checks actual PNG pixels after independent child
// title/gameplay draws, so visual inspection cannot mistake dark phone chrome for
// a missing parent layer. The child must change while the host controls stay put.
func TestGraphicsScreensPhoneChrome(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("../../e2e/atago/fixtures/game")); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(game, "apps"), os.DirFS("../../examples/office/game/apps")); err != nil {
		t.Fatal(err)
	}
	catalog, err := fs.ReadFile(os.DirFS(game), "apps/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`schema_version: 11
id: phone-chrome-test
title: Phone chrome test
screen: {width: 960, height: 540}
start: {location: camp, party: [scout], event: phone_probe}
phone:
  catalog: apps/catalog.yaml
  catalog_sha256: %x
  apps:
    - {id: counter, package: fixtures/counter, version: 1.0.0, install: bundled, path: apps/counter.zip}
`, sha256.Sum256(catalog))
	files := map[string]string{
		"game.yaml":       config,
		"data/phone.yaml": "events:\n  phone_probe:\n    do:\n      - say: Open the phone.\n      - phone: {app: game, game: counter, hold: portrait}\n      - say: Returned.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(game, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(root, "script.txt")
	if err := os.WriteFile(script, []byte("gap 12\nwait 90\npress ok\nwait 100\npress ok\nwait 100\nshot child-title\npress ok\nwait 100\nshot child-play\npress ok\nwait 50\npress ok\nwait 30\npress menu\nwait 30\npress ok\nwait 30\nshot child-save-menu\npress ok\nwait 60\nshot child-save-message\nwait 60\npress down\npress ok\nwait 30\nshot child-load-menu\npress ok\nwait 30\nshot child-confirm\npress cancel\nwait 20\npress cancel\nwait 20\npress down\npress ok\nwait 30\nshot child-settings\npress cancel\nwait 20\nhold shift+cancel 1\nwait 40\nshot host-return\nquit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--seed", "7", "--trace", filepath.Join(root, "trace.jsonl")) //nolint:gosec // Fixed host and private fixture paths.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}

	replayDir := filepath.Join(root, "replay")
	replayCmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--replay", filepath.Join(root, "trace.jsonl"), "--trace", filepath.Join(root, "replay.jsonl"), "--save-dir", replayDir) //nolint:gosec // Built test host and isolated fixture paths.
	stdout.Reset()
	stderr.Reset()
	replayCmd.Stdout, replayCmd.Stderr = &stdout, &stderr
	if err := replayCmd.Run(); err != nil {
		for _, name := range []string{"trace.jsonl", "replay.jsonl"} {
			body, readErr := fs.ReadFile(os.DirFS(root), name)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, kind := range []string{"say", "phone_game", "end", "input"} {
				t.Logf("%s %s: %v", name, kind, traceLines(t, body, kind))
			}
		}
		t.Fatalf("replay: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	if !strings.Contains(stdout.String(), "replay fully matched:") || strings.Contains(stdout.String(), "diverged") {
		t.Fatalf("missing exact replay success: %s", &stdout)
	}
	original, err := fs.ReadFile(os.DirFS(root), "trace.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fs.ReadFile(os.DirFS(root), "replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	ends, wantEnds := traceLines(t, replayed, "end"), traceLines(t, original, "end")
	if len(ends) != 1 || len(wantEnds) != 1 || !reflect.DeepEqual(ends[0]["state"], wantEnds[0]["state"]) {
		t.Fatalf("host final state differs: %v / %v", ends, wantEnds)
	}
	saves, err := fs.Glob(os.DirFS(filepath.Join(root, "saves")), "phone-save-*-save1.json")
	if err != nil || len(saves) != 1 {
		t.Fatalf("child save missing: %v %v", saves, err)
	}
	saved, err := fs.ReadFile(os.DirFS(filepath.Join(root, "saves")), saves[0])
	if err != nil {
		t.Fatal(err)
	}
	var savedState struct {
		State struct {
			Gold int `json:"gold"`
		} `json:"state"`
	}
	if err := json.Unmarshal(saved, &savedState); err != nil {
		t.Fatal(err)
	}
	if savedState.State.Gold != 4 {
		t.Fatalf("child gameplay did not save its result: %d", savedState.State.Gold)
	}
	// Full replay intentionally uses an in-memory store. Verify the actual child
	// view through exact partial replay rather than expecting writes on disk.
	target := 0
	for _, shot := range traceLines(t, original, "shot") {
		name, validName := shot["file"].(string)
		tick, validTick := shot["tick"].(float64)
		if !validName || !validTick {
			t.Fatalf("invalid shot record: %v", shot)
		}
		if strings.HasSuffix(name, "child-play.png") {
			target = int(tick) + 1
		}
	}
	if target == 0 {
		t.Fatal("missing child gameplay capture")
	}
	partial := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--replay", filepath.Join(root, "trace.jsonl"), "--until", strconv.Itoa(target), "--shots", replayDir) //nolint:gosec // Built test host and isolated fixture paths.
	stdout.Reset()
	stderr.Reset()
	partial.Stdout, partial.Stderr = &stdout, &stderr
	if err := partial.Run(); err != nil {
		t.Fatalf("partial replay: %v\n%s\n%s", err, &stdout, &stderr)
	}
	reproduced := shotImage(t, replayDir, fmt.Sprintf("until_%06d", target))
	if !reflect.DeepEqual(shotImage(t, root, "child-play"), reproduced) {
		t.Fatal("exact replay changed embedded gameplay pixels")
	}
	title, play := shotImage(t, root, "child-title"), shotImage(t, root, "child-play")
	for name, rect := range map[string]image.Rectangle{
		"back text":  image.Rect(430, 30, 550, 50),
		"back arrow": image.Rect(375, 30, 400, 50),
	} {
		bright := 0
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				r, g, b, a := title.At(x, y).RGBA()
				pr, pg, pb, pa := play.At(x, y).RGBA()
				if r != pr || g != pg || b != pb || a != pa {
					t.Fatalf("%s changed at %d,%d after child draw", name, x, y)
				}
				if r > 100*257 && g > 100*257 && b > 100*257 {
					bright++
				}
			}
		}
		if bright < 20 {
			t.Errorf("%s has only %d bright pixels; host control disappeared", name, bright)
		}
	}
	changed := false
	for y := 100; y < 480; y++ {
		for x := 370; x < 590; x++ {
			if title.At(x, y) != play.At(x, y) {
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("child title and gameplay are identical; transition was not exercised")
	}
}

// Render compact ordinary menus in the same authored dimensions used by phone
// children, and inspect drawn text rather than accepting an image-size smoke.
func TestGraphicsScreensCompactMenus(t *testing.T) {
	t.Parallel()
	for _, screen := range [][2]int{{360, 540}, {640, 360}} {
		t.Run(fmt.Sprintf("%dx%d", screen[0], screen[1]), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			game := filepath.Join(root, "game")
			if err := os.MkdirAll(filepath.Join(game, "data"), 0o700); err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("schema_version: 11\nid: compact-menu-test\nlanguage: en\nscreen: {width: %d, height: %d}\nstart: {location: room}\n", screen[0], screen[1])
			if err := os.WriteFile(filepath.Join(game, "game.yaml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(game, "data", "world.yaml"), []byte("locations:\n  room:\n    name: A long location label that must keep its slot number visible\n    safe: true\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			script := "gap 12\nwait 30\ndo open: save\nwait 30\nshot save-empty\npress ok\nwait 20\nshot save-message\nwait 100\ndo open: load\nwait 30\nshot load\npress ok\nwait 30\nshot confirm\npress cancel\npress cancel\ndo open: config\nwait 30\nshot settings\n"
			for range 12 {
				script += "press down\n"
			}
			script += "wait 30\nshot settings-bottom\nquit\n"
			scriptPath := filepath.Join(root, "script.txt")
			if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--start", "room", "--dev", "--mute", "--script", scriptPath, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", filepath.Join(root, "trace.jsonl")) //nolint:gosec // Built graphics host with private fixtures.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("menus: %v\n%s\n%s", err, &stdout, &stderr)
			}
			body, err := fs.ReadFile(os.DirFS(root), "trace.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			seenSlot, seenSettings := false, false
			logicalWidth := float64(screen[0]) * 720 / float64(screen[1])
			for _, line := range traceLines(t, body, "layout") {
				encoded, err := json.Marshal(line)
				if err != nil {
					t.Fatal(err)
				}
				var layout struct {
					Texts []struct {
						Text string     `json:"text"`
						Rect [4]float64 `json:"rect"`
					} `json:"texts"`
				}
				if err := json.Unmarshal(encoded, &layout); err != nil {
					t.Fatal(err)
				}
				for _, text := range layout.Texts {
					slot := strings.HasPrefix(text.Text, "1:") || strings.HasPrefix(text.Text, "2:") || strings.HasPrefix(text.Text, "3:")
					settings := text.Text == "Settings" || strings.Contains(text.Text, "Text speed")
					if !slot && !settings {
						continue
					}
					seenSlot = seenSlot || slot
					seenSettings = seenSettings || settings
					r := text.Rect
					if r[0] < 0 || r[1] < 0 || r[0]+r[2] > logicalWidth+1 || r[1]+r[3] > 721 {
						t.Errorf("drawn menu text outside %vx720: %q %v", logicalWidth, text.Text, r)
					}
				}
			}
			if !seenSlot || !seenSettings {
				t.Fatalf("missing rendered menu coverage: slots=%v settings=%v", seenSlot, seenSettings)
			}
			// Centered confirmation text is drawn directly by gfx, so the layout
			// trace does not contain it. Check the actual overlay pixels here;
			// TestConfirmFitsPortraitAndLandscape checks its text and button bounds.
			load, confirm := shotImage(t, root, "load"), shotImage(t, root, "confirm")
			changed := 0
			for y := range screen[1] {
				for x := range screen[0] {
					if load.At(x, y) != confirm.At(x, y) {
						changed++
					}
				}
			}
			if changed < screen[0]*screen[1]/10 {
				t.Fatalf("confirmation overlay did not visibly cover load menu: %d changed pixels", changed)
			}
			for _, name := range []string{"save-empty", "save-message", "load", "confirm", "settings", "settings-bottom"} {
				if img := shotImage(t, root, name); img.Bounds().Dx() != screen[0] || img.Bounds().Dy() != screen[1] {
					t.Fatalf("%s dimensions: %v", name, img.Bounds())
				}
			}
		})
	}
}
