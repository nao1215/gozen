package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/play"
	"github.com/nao1215/gozen/internal/savefile"
	"github.com/nao1215/gozen/internal/testgame"
)

func reproFixture(t *testing.T) string {
	t.Helper()
	return traceFile(t,
		`{"t":"start","tick":0,"version":2,"game_dir":"fixture game","seed":18446744073709551615,"event":"intro","flags":["ready"],"setup":{"saves":{},"save_data":{}}}`,
		`{"t":"input","tick":1,"press":["ok"]}`,
		`{"t":"shot","tick":60,"file":"shots/first.png"}`,
		`{"t":"view","tick":90,"shot":"shots/view.png"}`,
		`{"t":"input","tick":91,"press":["cancel"]}`,
		`{"t":"end","tick":120,"lines":6,"state":null}`)
}

func TestReproMappingAndCut(t *testing.T) {
	path := reproFixture(t)
	for _, tt := range []struct {
		target string
		tick   int
	}{{"0", 0}, {"60", 60}, {"1.5s", 90}, {"first.png", 60}, {"shots/view.png", 90}} {
		t.Run(tt.target, func(t *testing.T) {
			b, cut, err := readRepro(path, tt.target)
			if err != nil {
				t.Fatal(err)
			}
			if b.Tick != tt.tick || b.Until != tt.tick+1 || b.Seed != ^uint64(0) {
				t.Fatalf("bundle %+v", b)
			}
			if strings.Contains(string(cut), `"tick":91`) || strings.Contains(string(cut), `"t":"end"`) {
				t.Fatalf("cut: %s", cut)
			}
			if !strings.Contains(string(cut), `"t":"repro"`) {
				t.Fatal("missing boundary")
			}
			if string(b.StartOptions["event"]) != `"intro"` {
				t.Fatal(b.StartOptions)
			}
		})
	}
	for _, target := range []string{"-1", "121", "-1s", "3s", "absent.png", "1.5"} {
		if _, _, err := readRepro(path, target); err == nil {
			t.Fatalf("accepted %s", target)
		}
	}
	records := []reproRecord{{tick: 1, shot: "a/same.png"}, {tick: 2, shot: "b/same.png"}}
	if _, err := reproTick("same.png", records, 2); err == nil {
		t.Fatal("ambiguous basename accepted")
	}
}

func TestReproCommandAndBundle(t *testing.T) {
	path := reproFixture(t)
	dir := filepath.Join(t.TempDir(), "bundle space ' quoted")
	out, _ := invoke(t, 0, "trace", path, "--repro", "60", "--out", dir, "--format", "json")
	var b reproBundle
	if err := json.Unmarshal(resultData(t, out, "trace"), &b); err != nil {
		t.Fatal(err)
	}
	if b.Tick != 60 || b.Argv[7] != "--until" || b.Argv[8] != "61" || !strings.HasSuffix(b.Screenshot, "until_000061.png") {
		t.Fatalf("%+v", b)
	}
	data, err := os.ReadFile(filepath.Join(dir, "repro.json")) //nolint:gosec // Reads this test's generated bundle.
	if err != nil {
		t.Fatal(err)
	}
	var disk reproBundle
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	left, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("disk bundle differs")
	}
	if _, err := exec.LookPath("sh"); err == nil {
		// Check exact shell argument round trips, including metacharacters.
		args := []string{"a b", "a'b", "$(false)", "line\nbreak"}
		cmd := exec.CommandContext(t.Context(), "sh", "-c", "printf '%s\\0' "+strings.TrimPrefix(reproCommand(append([]string{"unused"}, args...)), "'unused' ")) //nolint:gosec // Fixed test arguments exercise shell quoting.
		got, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != strings.Join(args, "\x00")+"\x00" {
			t.Fatalf("quoted args %q", got)
		}
	}
	invoke(t, 1, "trace", path, "--repro", "60", "--out", dir)
	invoke(t, 2, "trace", path, "--out", dir)
	invoke(t, 2, "trace", path, "--repro", "60", "--check")
}

func TestReproMissingPrerequisites(t *testing.T) {
	for _, start := range []string{
		`{"t":"start","tick":0,"seed":1}`,
		`{"t":"start","tick":0,"game_dir":"game","seed":0}`,
		`{"t":"start","tick":0,"game_dir":"game","seed":1,"setup":{"saves":{"save1.json":"abc"}}}`,
		`{"t":"start","tick":0,"game_dir":"game","seed":1,"setup":{"unsupported":["review"]}}`,
	} {
		if _, _, err := readRepro(traceFile(t, start), "0"); err == nil {
			t.Fatalf("accepted %s", start)
		}
	}
}

func TestReproRejectsUnsupportedMutations(t *testing.T) {
	for _, record := range []string{
		`{"t":"unsupported_script","tick":2,"operation":"recruit"}`,
		`{"t":"unsupported_script","tick":2,"operation":"guarantee-recruit"}`,
		`{"t":"battle_setup","tick":2,"hp":1}`,
	} {
		path := traceFile(t, `{"t":"start","tick":0,"game_dir":"game","seed":7}`, record)
		if _, _, err := readRepro(path, "2"); err == nil || !strings.Contains(err.Error(), "cannot reproduce exactly:") {
			t.Fatalf("record %s: %v", record, err)
		}
		if _, _, err := readRepro(path, "1"); err != nil {
			t.Fatalf("target before mutation: %v", err)
		}
	}
}

func TestReproRejectsGuides(t *testing.T) {
	start := `{"t":"start","tick":0,"game_dir":"game","seed":7,"scripted":false}`
	guide := `{"t":"guide","tick":2,"screen":"board","step":1,"text":"The kiln log first."}`
	shot := `{"t":"shot","tick":3,"file":"first-visit.png"}`
	path := traceFile(t, start, guide, shot)
	for _, bundle := range []bool{false, true} {
		args := []string{"trace", path, "--repro", "first-visit.png"}
		if bundle {
			args = append(args, "--out", filepath.Join(t.TempDir(), "bundle"))
		}
		_, stderr := invoke(t, 1, args...)
		if !strings.Contains(stderr, "cannot reproduce exactly: guide at tick") {
			t.Fatal(stderr)
		}
	}
	path = traceFile(t, start, `{"t":"act","tick":1,"name":"guides","args":{"on":true}}`, guide, shot)
	if _, _, err := readRepro(path, "first-visit.png"); err == nil {
		t.Fatal("explicit guides accepted")
	}
}

func TestReproActualGuidesScript(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml": {Data: []byte("schema_version: 10\nid: guide-repro\n" + testgame.Game + `
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
      - {who: guide, text: 'Read the board.'}
`)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	fsys["assets/x.png"] = &fstest.MapFile{Data: picture.Bytes()}
	var trace bytes.Buffer
	result, err := play.Simulate(t.Context(), play.Options{FS: fsys, GameDir: "fixture", Location: "camp", Seed: 7, Store: savefile.MemStore(), Trace: &trace, Script: "guides on\ndo open: board\nwait 10\nquit\n"}, play.SimulateConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trace.String(), `"t":"guide"`) {
		t.Fatalf("no actual guide: %s", &trace)
	}
	path := traceFile(t, strings.TrimSpace(trace.String()))
	for _, bundle := range []bool{false, true} {
		args := []string{"trace", path, "--repro", strconv.Itoa(result.Ticks), "--format", "json"}
		if bundle {
			args = append(args, "--out", filepath.Join(t.TempDir(), "bundle"))
		}
		out, stderr := invoke(t, 1, args...)
		if out != "" || !strings.Contains(stderr, "cannot reproduce exactly: guide at tick") {
			t.Fatalf("stdout=%s stderr=%s", out, stderr)
		}
	}
}

func TestReproRejectsWindowChanges(t *testing.T) {
	for _, change := range []string{
		`{"t":"act","tick":2,"name":"window","args":{"width":960,"height":540}}`,
		`{"t":"viewport","tick":2,"width":960,"height":540,"scale":1}`,
	} {
		path := traceFile(t,
			`{"t":"start","tick":0,"game_dir":"game","seed":7,"window":2}`,
			`{"t":"viewport","tick":0,"width":1280,"height":720,"scale":1}`,
			`{"t":"viewport","tick":1,"width":1280,"height":720,"scale":1}`,
			change)
		if _, _, err := readRepro(path, "1"); err != nil {
			t.Fatal(err)
		}
		for _, bundle := range []bool{false, true} {
			args := []string{"trace", path, "--repro", "2"}
			if bundle {
				args = append(args, "--out", filepath.Join(t.TempDir(), "bundle"))
			}
			out, stderr := invoke(t, 1, args...)
			if out != "" || !strings.Contains(stderr, "cannot reproduce exactly: the recording changed the window size") {
				t.Fatalf("stdout=%s stderr=%s", out, stderr)
			}
		}
	}
}

func TestReproRejectsTypedPanelCommands(t *testing.T) {
	for _, command := range []struct{ yaml, kind string }{
		{"set: {gold: 123}", "dev_set"},
		{"wait: 1", "dev_do"},
	} {
		t.Run(command.kind, func(t *testing.T) {
			files := fstest.MapFS{
				"game.yaml":      {Data: []byte("schema_version: 10\nid: panel-repro\n" + testgame.Game)},
				"data/main.yaml": {Data: []byte(testgame.Data)},
			}
			var picture bytes.Buffer
			if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
				t.Fatal(err)
			}
			files["assets/x.png"] = &fstest.MapFile{Data: picture.Bytes()}
			var trace bytes.Buffer
			result, err := play.Simulate(t.Context(), play.Options{
				FS: files, GameDir: "fixture", Location: "camp", Seed: 7,
				Development: true, Store: savefile.MemStore(), Trace: &trace,
				Script: "wait 10\npress panel\nwait 3\npress right 3\nwait 3\ntype " + command.yaml + "\nwait 3\npress ok\nwait 3\nquit\n",
			}, play.SimulateConfig{})
			if err != nil {
				t.Fatal(err)
			}
			records, err := readReproRecords(trace.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			tick := -1
			for _, record := range records {
				if record.kind == command.kind {
					tick = record.tick
				}
			}
			if tick < 0 {
				t.Fatalf("typed command not recorded: %s", &trace)
			}
			path := traceFile(t, strings.TrimSpace(trace.String()))
			want := fmt.Sprintf("cannot reproduce exactly: %s at tick %d", command.kind, tick)
			out, stderr := invoke(t, 1, "trace", path, "--repro", strconv.Itoa(result.Ticks))
			if out != "" || !strings.Contains(stderr, want) {
				t.Fatalf("stdout=%s stderr=%s", out, stderr)
			}
			// The same trace must be refused by the replay loader before running.
			game, replayErr := play.New(play.Options{FS: files, Store: savefile.MemStore(), Replay: path, Until: tick + 1})
			if game != nil {
				if err := game.Close(); err != nil {
					t.Fatal(err)
				}
			}
			err = replayErr
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("replay: %v", err)
			}
			if _, _, err := readRepro(path, strconv.Itoa(tick-1)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReproRejectsUnknownFutureRecord(t *testing.T) {
	path := traceFile(t, `{"t":"start","tick":0,"game_dir":"game","seed":7}`, `{"t":"future_kind","tick":2}`)
	out, stderr := invoke(t, 1, "trace", path, "--repro", "2")
	if out != "" || !strings.Contains(stderr, "cannot reproduce exactly: future_kind at tick 2") {
		t.Fatalf("stdout=%s stderr=%s", out, stderr)
	}
	if _, _, err := readRepro(path, "1"); err != nil {
		t.Fatal(err)
	}
}
