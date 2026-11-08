package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/cmd/internal/repro"
	"github.com/nao1215/gozen/internal/layer1/traceformat"
	"github.com/nao1215/gozen/internal/play"
	"github.com/nao1215/gozen/internal/savefile"
	"github.com/nao1215/gozen/internal/testgame"
)

func reproFixture(t *testing.T) string {
	t.Helper()
	return traceFile(t,
		`{"t":"start","tick":0,"exact_version":3,"version":2,"game_dir":"fixture game","seed":18446744073709551615,"event":"intro","flags":["ready"],"setup":{"saves":{},"save_data":{}}}`,
		`{"t":"input","tick":1,"press":["ok"]}`,
		`{"t":"shot","tick":60,"file":"shots/first.png"}`,
		`{"t":"view","tick":90,"shot":"shots/view.png"}`,
		`{"t":"input","tick":91,"press":["cancel"]}`,
		`{"t":"end","tick":120,"lines":6,"state":null}`)
}

func TestReproCommandAndBundle(t *testing.T) {
	path := reproFixture(t)
	dir := filepath.Join(t.TempDir(), "bundle space ' quoted")
	out, _ := invoke(t, 0, "trace", path, "--repro", "60", "--out", dir, "--format", "json")
	var b repro.Bundle
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
	var disk repro.Bundle
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

	invoke(t, 1, "trace", path, "--repro", "60", "--out", dir)
	invoke(t, 2, "trace", path, "--out", dir)
	invoke(t, 2, "trace", path, "--repro", "60", "--check")
}

func TestReproRejectsGuides(t *testing.T) {
	start := `{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":7,"scripted":false}`
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
	if _, err := repro.Generate(repro.Config{Path: path, Target: "first-visit.png"}); err == nil {
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
			`{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":7,"window":2}`,
			`{"t":"viewport","tick":0,"width":1280,"height":720,"scale":1}`,
			`{"t":"viewport","tick":1,"width":1280,"height":720,"scale":1}`,
			change)
		if _, err := repro.Generate(repro.Config{Path: path, Target: "1"}); err != nil {
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
			tick := -1
			commandRecorded := false
			for line := range bytes.SplitSeq(bytes.TrimSpace(trace.Bytes()), []byte{'\n'}) {
				var record struct {
					Kind string `json:"t"`
					Tick int    `json:"tick"`
				}
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatal(err)
				}
				if record.Kind == "panel" && tick < 0 {
					tick = record.Tick
				}
				commandRecorded = commandRecorded || record.Kind == command.kind
			}
			if tick < 0 || !commandRecorded {
				t.Fatalf("typed command not recorded: %s", &trace)
			}
			path := traceFile(t, strings.TrimSpace(trace.String()))
			want := fmt.Sprintf("cannot reproduce exactly: panel at tick %d", tick)
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
			if _, err := repro.Generate(repro.Config{Path: path, Target: strconv.Itoa(tick - 1)}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReproRejectsUnknownFutureRecord(t *testing.T) {
	path := traceFile(t, `{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":7}`, `{"t":"future_kind","tick":2}`)
	out, stderr := invoke(t, 1, "trace", path, "--repro", "2")
	if out != "" || !strings.Contains(stderr, "cannot reproduce exactly: future_kind at tick 2") {
		t.Fatalf("stdout=%s stderr=%s", out, stderr)
	}
	if _, err := repro.Generate(repro.Config{Path: path, Target: "1"}); err != nil {
		t.Fatal(err)
	}
}

func TestReproUnknownGameDirectory(t *testing.T) {
	for _, field := range []string{"", `,"game_dir":""`} {
		path := traceFile(t, `{"t":"start","tick":0,"exact_version":3,"seed":7`+field+`}`)
		out, stderr := invoke(t, 1, "trace", path, "--repro", "0")
		if out != "" || !strings.Contains(stderr, "game directory is unknown") {
			t.Fatalf("stdout %q, stderr %q", out, stderr)
		}
	}
}

func TestReproLargeInitialSaves(t *testing.T) {
	files := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 10\nid: large-save-repro\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	files["assets/x.png"] = &fstest.MapFile{Data: picture.Bytes()}
	for _, size := range []int{8 << 20, (8 << 20) + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			store := savefile.MemStore()
			data := bytes.Repeat([]byte(" "), size)
			for _, name := range []string{"save1.json", "save2.json"} {
				if err := store.Write(name, data); err != nil {
					t.Fatal(err)
				}
			}
			var trace bytes.Buffer
			game, err := play.New(play.Options{FS: files, GameDir: "fixture", Seed: 7, Location: "camp", NoSound: true, Store: store, Trace: &trace})
			if err != nil {
				t.Fatal(err)
			}
			if err := game.Close(); err != nil {
				t.Fatal(err)
			}
			path := traceFile(t, strings.TrimSpace(trace.String()))
			if size > 8<<20 {
				_, stderr := invoke(t, 1, "trace", path, "--repro", "0")
				if !strings.Contains(stderr, "exceeds the 8 MiB limit") {
					t.Fatal(stderr)
				}
				return
			}
			for line := range bytes.SplitSeq(trace.Bytes(), []byte("\n")) {
				if len(line) >= traceformat.LineLimit {
					t.Fatal("unreadable trace record")
				}
			}
			bundle := filepath.Join(t.TempDir(), "bundle")
			invoke(t, 0, "trace", path, "--repro", "0", "--out", bundle)
			invoke(t, 0, "trace", path)
			for _, name := range []string{"save1.json", "save2.json"} {
				got, err := os.ReadFile(filepath.Join(bundle, "saves", name)) //nolint:gosec // Reads this test's generated bundle.
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("bundled save: %v", err)
				}
			}
			replayed, err := play.New(play.Options{FS: files, NoSound: true, Store: savefile.MemStore(), Replay: filepath.Join(bundle, "trace.jsonl"), Until: 1})
			if err != nil {
				t.Fatal(err)
			}
			if err := replayed.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReproRefusesUnrecordedSources(t *testing.T) {
	for _, tt := range []struct{ fields, want string }{
		{`"autopilot":true`, "autopilot changes selections"},
		{`"setup":{"unsupported":["recording a replay"]}`, "recording a replay"},
	} {
		path := traceFile(t, `{"t":"start","tick":0,"exact_version":3,"game_dir":"fixture","seed":7,`+tt.fields+`}`)
		_, stderr := invoke(t, 1, "trace", path, "--repro", "0")
		if !strings.Contains(stderr, tt.want) {
			t.Fatal(stderr)
		}
	}
}

func TestReproRefusesMissingInitialSaveRecord(t *testing.T) {
	path := traceFile(t, `{"t":"start","tick":0,"exact_version":3,"game_dir":"fixture","seed":7,"setup":{"saves":{"save1.json":"digest"},"save_data":{}}}`)
	_, stderr := invoke(t, 1, "trace", path, "--repro", "0")
	if !strings.Contains(stderr, "missing or does not match its digest") {
		t.Fatal(stderr)
	}
}

func TestReproConfiguredAuthorTools(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"review", "review-log", "review-sources", "memos", "external-input", "build-info", "live reload"} {
		t.Run(mode, func(t *testing.T) {
			var trace bytes.Buffer
			o := play.Options{
				FS: fstest.MapFS{
					"game.yaml":      {Data: []byte("schema_version: 10\nid: tools-repro\n" + testgame.Game)},
					"data/main.yaml": {Data: []byte(testgame.Data)},
					"assets/x.png":   {Data: picture.Bytes()},
				},
				GameDir: "fixture", Seed: 7, NoSound: true, Store: savefile.MemStore(), Trace: &trace,
			}
			switch mode {
			case "review":
				o.Review = true
			case "review-log":
				o.ReviewLog = filepath.Join(t.TempDir(), "review.jsonl")
			case "review-sources":
				o.ReviewSources = filepath.Join(t.TempDir(), "sources.json")
			case "memos":
				o.MemoDir = t.TempDir()
			case "external-input":
				o.ExternalInput = true
			case "build-info":
				o.CollectBuildInfo = true
			case "live reload":
				o.LiveDir = t.TempDir()
			}
			g, err := play.New(o)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			path := traceFile(t, strings.TrimSpace(trace.String()))
			for _, bundle := range []bool{false, true} {
				args := []string{"trace", path, "--repro", "0", "--format", "json"}
				if bundle {
					args = append(args, "--out", filepath.Join(t.TempDir(), "bundle"))
				}
				code := 0
				if mode == "review" {
					code = 1
				}
				out, stderr := invoke(t, code, args...)
				if (mode == "review" && !strings.Contains(stderr, "cg_review")) || (mode != "review" && (stderr != "" || out == "")) {
					t.Fatalf("stdout=%s stderr=%s", out, stderr)
				}
			}
		})
	}
}
