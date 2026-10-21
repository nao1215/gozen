package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

// designGame copies the design sample (a camp, an inn and a three-floor cave with a
// guardian) and the stock assets into a fresh directory.
func designGame(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "design game")
	if err := os.CopyFS(dir, os.DirFS("testdata/design")); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, "assets"), os.DirFS("../../e2e/atago/fixtures/game/assets")); err != nil {
		t.Fatal(err)
	}
	return dir
}

type designPersona struct {
	ID       string  `json:"id"`
	Cleared  bool    `json:"cleared"`
	Days     int     `json:"days"`
	Deepest  int     `json:"deepest"`
	Level    int     `json:"level"`
	Battles  int     `json:"battles"`
	Lost     int     `json:"lost"`
	Wipes    int     `json:"wipes"`
	Retries  int     `json:"retries"`
	Seed     uint64  `json:"seed"`
	EndMin   float64 `json:"end_min"`
	ClearT   float64 `json:"clear_t"`
	Problems []string `json:"problem_list"`
	Rewards  map[string]int `json:"rewards"`
	Events   []struct {
		Kind   string  `json:"kind"`
		T      float64 `json:"t"`
		Reward bool    `json:"reward"`
		Expect bool    `json:"expect"`
		Troop  string  `json:"troop"`
	} `json:"events"`
	Bosses []struct {
		Floor    int
		Measured bool
		Tries    int
		Wins     int
	} `json:"bosses"`
	Duels []struct {
		Opened bool
		Grown  bool
		Tries  int
	} `json:"duels"`
}

func designJSON(t *testing.T, args ...string) []designPersona {
	t.Helper()
	out, _ := invoke(t, 0, append([]string{"sim", "design"}, append(args, "--format", "json")...)...)
	var got struct {
		FormatVersion int             `json:"format_version"`
		Personas      []designPersona `json:"personas"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if got.FormatVersion != 1 {
		t.Fatalf("format_version %d", got.FormatVersion)
	}
	return got.Personas
}

// The same seed gives the same play, and the play obeys the game's own rules: a loss
// is followed by a retry or a wipe, and the guardian's tries are the battles lost and won.
func TestDesignPlaysThePersonasDeterministically(t *testing.T) {
	dir := designGame(t)
	first := designJSON(t, dir)
	if len(first) != 3 || first[0].ID != "steady" || first[1].ID != "eager" || first[2].ID != "wanderer" {
		t.Fatalf("personas: %+v", first)
	}
	again := designJSON(t, dir)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatal("the same configuration played differently twice")
	}
	steady, eager := first[0], first[1]
	if !steady.Cleared || steady.Deepest != 3 || steady.Seed != 3 {
		t.Fatalf("steady: %+v", steady)
	}
	if steady.Wipes != 0 || steady.Lost != 0 {
		t.Fatalf("a persona that grinds to the measured win rate lost: %+v", steady)
	}
	if eager.Wipes < 1 || eager.Lost < eager.Wipes {
		t.Fatalf("eager walks into the guardian unprepared and must fall: %+v", eager)
	}
	if eager.Retries < 1 {
		t.Fatalf("battle retry was not used on a loss: %+v", eager)
	}
	for _, p := range first {
		if len(p.Bosses) != 1 || p.Bosses[0].Floor != 3 || p.Bosses[0].Tries < 1 {
			t.Fatalf("%s: the guardian of floor 3 was not met: %+v", p.ID, p.Bosses)
		}
		clear := 0
		for _, e := range p.Events {
			if e.Kind == "clear" {
				clear++
			}
		}
		if clear != 1 {
			t.Errorf("%s: %d clear events", p.ID, clear)
		}
	}
	if !first[0].Bosses[0].Measured {
		t.Error("the guardian's win rate was not measured on arrival")
	}
	// A seed given on the command line replaces every persona's own.
	for _, p := range designJSON(t, dir, "--seed", "11") {
		if p.Seed != 11 {
			t.Errorf("%s: seed %d", p.ID, p.Seed)
		}
	}
	only := designJSON(t, dir, "--only", "eager,steady")
	if len(only) != 2 || only[0].ID != "steady" {
		t.Fatalf("--only: %+v", only)
	}
}

func TestDesignPersonaPoliciesAndDuels(t *testing.T) {
	dir := designGame(t)
	steady := designJSON(t, dir, "--only", "steady")[0]
	if steady.Rewards["picture"] != 1 {
		t.Errorf("the first picture is a reward once: %v", steady.Rewards)
	}
	if steady.Rewards["gift"] < 1 || steady.Rewards["skill"] != 2 || steady.Rewards["chest"] < 1 {
		t.Errorf("rewards: %v", steady.Rewards)
	}
	if len(steady.Duels) != 1 || !steady.Duels[0].Opened {
		t.Errorf("the duel opened by a true condition was not measured: %+v", steady.Duels)
	}
	gifts := 0
	for _, e := range steady.Events {
		if e.Kind == "gift" && e.Reward {
			gifts++
		}
		if e.Kind == "quiet" {
			t.Error("a rule that yields to a gift fired beside it")
		}
	}
	if gifts == 0 {
		t.Error("the town policy never took the gift")
	}
}

func TestDesignReportsOnDisk(t *testing.T) {
	dir := designGame(t)
	root := filepath.Join(t.TempDir(), "runs")
	first := filepath.Join(root, "20200101-000000")
	notes := filepath.Join(t.TempDir(), "notes")
	out, _ := invoke(t, 0, "sim", "design", dir, "-o", first, "--memos", notes)
	if strings.TrimSpace(out) != filepath.Join(first, "report.md") {
		t.Fatalf("stdout: %q", out)
	}
	report := string(read(t, filepath.Join(first, "report.md")))
	for _, want := range []string{"Steady (steady)", "Eager (eager)", "| B3F |", "No previous run", "(a guide; marks only, nothing fails)", "## Time bands"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	for _, p := range []string{"band.png", "timeline/steady.jsonl", "timeline/eager.jsonl", "summary.json"} {
		if info, err := os.Stat(filepath.Join(first, p)); err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", p, err)
		}
	}
	line := strings.SplitN(string(read(t, filepath.Join(first, "timeline", "eager.jsonl"))), "\n", 2)[0]
	if !strings.Contains(line, `"kind":"persona"`) || !strings.Contains(line, `"id":"eager"`) {
		t.Errorf("timeline header: %s", line)
	}
	days, err := os.ReadDir(notes)
	if err != nil || len(days) == 0 {
		t.Fatalf("no notes were written: %v", err)
	}
	if !strings.Contains(report, "## Notes") && !strings.Contains(report, "notes") {
		t.Errorf("the report does not say the notes were written: %s", report)
	}

	// A later run in the same root compares itself with the earlier one.
	second, _ := invoke(t, 0, "sim", "design", dir, "--root", root, "-o", filepath.Join(root, "20200102-000000"))
	cmp := string(read(t, strings.TrimSpace(second)))
	if !strings.Contains(cmp, "Previous run: 20200101-000000") || strings.Contains(cmp, "No previous run") {
		t.Errorf("no comparison with the previous run:\n%s", cmp[strings.Index(cmp, "## Change"):])
	}
}

func TestDesignListAndInvalidInput(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "design", dir, "--list")
	var cfg struct {
		Personas []struct{ ID string }
		Release  string
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil || len(cfg.Personas) != 3 || cfg.Release != "gift" {
		t.Fatalf("--list: %v %q", err, out)
	}
	list := filepath.Join(t.TempDir(), "list")
	invoke(t, 0, "sim", "design", dir, "--list", "-o", list)
	if _, err := os.Lstat(list); err == nil {
		t.Error("--list wrote an output directory")
	}
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{dir, "--only", "nobody"}, 2, "unknown persona"},
		{[]string{dir, "--only", "nobody", "--format", "json"}, 2, "matched no personas"},
		{[]string{dir, "--format", "xml"}, 2, "text or json"},
		{[]string{}, 2, "expected <game-dir>"},
		{[]string{dir, dir}, 2, "expected <game-dir>"},
		{[]string{filepath.Join(t.TempDir(), "absent")}, 1, ""},
		{[]string{dir, "--config", filepath.Join(t.TempDir(), "absent.yaml")}, 1, ""},
	} {
		out, stderr := invoke(t, tc.want, append([]string{"sim", "design"}, tc.args...)...)
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: stdout %q stderr %q", tc.args, out, stderr)
		}
	}
	for _, tc := range []struct{ name, config, msg string }{
		{"no personas", "personas: []\n", "no personas"},
		{"unknown key", "personas: [{id: a}]\nsurprise: 1\n", "surprise"},
		{"negative days", "game: {days: -1}\npersonas: [{id: a}]\n", "nonnegative"},
		{"same id", "personas: [{id: a}, {id: a}]\n", "same id twice"},
		{"bad id", "personas: [{id: ../a}]\n", "no id or the same id"},
		{"bad explore", "personas: [{id: a, explore: everywhere}]\n", "explore"},
		{"bad grind", "personas: [{id: a, grind: 2}]\n", "invalid days, floors or grind"},
		{"time", "time: {line: -1}\npersonas: [{id: a}]\n", "finite and nonnegative"},
		{"image regexp", "time: {images: [{match: '('}]}\npersonas: [{id: a}]\n", "time.images[0]"},
		{"rule without source", "rewards: [{kind: k}]\npersonas: [{id: a}]\n", "write one of"},
		{"rule with two sources", "rewards: [{kind: k, images: a, events: b}]\npersonas: [{id: a}]\n", "write one of"},
		{"rule without kind", "rewards: [{images: a}]\npersonas: [{id: a}]\n", "no kind"},
		{"rule regexp", "rewards: [{kind: k, events: '('}]\npersonas: [{id: a}]\n", "rule"},
		{"target", "targets: {persona: z}\npersonas: [{id: a}]\n", "unknown persona"},
		{"memo kind", "memos: {dry: absent}\npersonas: [{id: a}]\n", "memo.kinds"},
		{"location", "game: {town: [nowhere]}\npersonas: [{id: a}]\n", "unknown location"},
		{"not a dungeon", "game: {dungeon: camp}\npersonas: [{id: a}]\n", "is not a dungeon"},
		{"item", "game: {back_item: ghost}\npersonas: [{id: a}]\n", "unknown item"},
		{"shop", "game: {shops: [ghost]}\npersonas: [{id: a}]\n", "unknown shop"},
		{"boss", "game: {bosses: {2: ghost}}\npersonas: [{id: a}]\n", "unknown troop"},
		{"duel numbers", "duels: {first: [0.1]}\npersonas: [{id: a}]\n", "low and the high"},
		{"duel persona", "duels: {personas: [z]}\npersonas: [{id: a}]\n", "unknown persona"},
		{"duel troop", "duels: {list: [{troop: ghost}]}\npersonas: [{id: a}]\n", "unknown troop"},
		{"duel fighter", "duels: {fighters: [ghost]}\npersonas: [{id: a}]\n", "unknown actor"},
		{"back event", "game: {back_event: ghost}\npersonas: [{id: a}]\n", "unknown event"},
		{"choice id", "game: {prefer: ['@ghost']}\npersonas: [{id: a}]\n", "unknown choice id"},
		{"visit entry", "personas: [{id: a, town: [{at: camp, do: '@ghost'}]}]\n", "must resolve once"},
		{"policy id", "personas: [{id: a, policies: [{phase: step, try: [{tile: T}]}]}]\n", "missing or duplicate id"},
		{"policy phase", "personas: [{id: a, policies: [{id: p, phase: noon, try: [{tile: T}]}]}]\n", "unknown phase"},
		{"policy reward", "personas: [{id: a, policies: [{id: p, phase: step, reward: ghost, try: [{tile: T}]}]}]\n", "unknown reward"},
		{"policy empty", "personas: [{id: a, policies: [{id: p, phase: step}]}]\n", "no attempts"},
		{"policy two", "personas: [{id: a, policies: [{id: p, phase: step, try: [{tile: T, item: tonic}]}]}]\n", "write one of"},
		{"policy tile", "personas: [{id: a, policies: [{id: p, phase: step, try: [{tile: TT}]}]}]\n", "one character"},
		{"policy explore", "personas: [{id: a, policies: [{id: p, phase: step, try: [{explore: nothing}]}]}]\n", "unknown exploration"},
		{"policy at", "personas: [{id: a, policies: [{id: p, phase: town, try: [{entry: gift}]}]}]\n", "needs at"},
		{"policy item", "personas: [{id: a, policies: [{id: p, phase: step, try: [{item: shard}]}]}]\n", "needs an event"},
		{"policy entry", "personas: [{id: a, policies: [{id: p, phase: town, try: [{entry: ghost, at: camp}]}]}]\n", "must resolve once"},
	} {
		write(t, filepath.Join(dir, "bad.yaml"), tc.config)
		out, stderr := invoke(t, 1, "sim", "design", dir, "--config", filepath.Join(dir, "bad.yaml"), "-o", filepath.Join(t.TempDir(), "never"))
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%s: stdout %q stderr %q (want %q)", tc.name, out, stderr, tc.msg)
		}
	}
}
