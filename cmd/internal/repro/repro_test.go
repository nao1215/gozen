package repro

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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

func TestReproMissingPrerequisites(t *testing.T) {
	for _, start := range []string{
		`{"t":"start","tick":0,"exact_version":3,"seed":1}`,
		`{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":0}`,
		`{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":1,"setup":{"saves":{"save1.json":"abc"}}}`,
		`{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":1,"setup":{"unsupported":["review"]}}`,
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
		path := traceFile(t, `{"t":"start","tick":0,"exact_version":3,"game_dir":"game","seed":7}`, record)
		if _, _, err := readRepro(path, "2"); err == nil || !strings.Contains(err.Error(), "cannot reproduce exactly:") {
			t.Fatalf("record %s: %v", record, err)
		}
		if _, _, err := readRepro(path, "1"); err != nil {
			t.Fatalf("target before mutation: %v", err)
		}
	}
}

func TestReproCutDrawCadence(t *testing.T) {
	path := traceFile(t,
		`{"t":"start","tick":0,"exact_version":3,"version":1,"game_dir":"fixture","seed":7}`,
		`{"t":"draw_skip","tick":5,"first":2,"last":4}`,
		`{"t":"draw_skip","tick":10,"first":8,"last":9}`,
		`{"t":"end","tick":12}`)
	for _, tick := range []int{1, 3, 5, 8, 10} {
		_, cut, err := readRepro(path, strconv.Itoa(tick))
		if err != nil {
			t.Fatal(err)
		}
		last := 0
		var spans [][2]int
		for line := range bytes.SplitSeq(bytes.TrimSpace(cut), []byte{'\n'}) {
			var record struct {
				Kind  string `json:"t"`
				Tick  int    `json:"tick"`
				First int    `json:"first"`
				Last  int    `json:"last"`
			}
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatal(err)
			}
			if record.Tick < last || record.Tick > tick {
				t.Fatalf("unordered cut: %s", cut)
			}
			last = record.Tick
			if record.Kind == "draw_skip" {
				spans = append(spans, [2]int{record.First, record.Last})
			}
		}
		for n := 1; n <= tick; n++ {
			skipped := false
			for _, span := range spans {
				skipped = skipped || n >= span[0] && n <= span[1]
			}
			if want := n >= 2 && n <= 4 || n >= 8 && n <= 9; skipped != want {
				t.Fatalf("target %d, tick %d: skipped=%v, cut=%s", tick, n, skipped, cut)
			}
		}
	}
}

func TestReproCommandAndBundle(t *testing.T) {
	path := reproFixture(t)
	dir := filepath.Join(t.TempDir(), "bundle space ' quoted")
	b, err := Generate(Config{Path: path, Target: "60", Out: dir})
	if err != nil {
		t.Fatal(err)
	}
	if b.Tick != 60 || b.Argv[7] != "--until" || b.Argv[8] != "61" || !strings.HasSuffix(b.Screenshot, "until_000061.png") {
		t.Fatalf("%+v", b)
	}
	data, err := os.ReadFile(filepath.Join(dir, "repro.json")) //nolint:gosec // Reads this test's generated bundle.
	if err != nil {
		t.Fatal(err)
	}
	var disk Bundle
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
}

func traceFile(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
