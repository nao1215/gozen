package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/gozen/internal/designeval"
	lang "github.com/nao1215/gozen/internal/text"
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
	ID       string         `json:"id"`
	Cleared  bool           `json:"cleared"`
	Days     int            `json:"days"`
	Deepest  int            `json:"deepest"`
	Level    int            `json:"level"`
	Battles  int            `json:"battles"`
	Lost     int            `json:"lost"`
	Wipes    int            `json:"wipes"`
	Retries  int            `json:"retries"`
	Seed     uint64         `json:"seed"`
	EndMin   float64        `json:"end_min"`
	ClearT   float64        `json:"clear_t"`
	Problems []string       `json:"problem_list"`
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

func jsonString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tail is the text from the first occurrence of marker on (all of it when marker is absent).
func tail(s, marker string) string {
	if i := strings.Index(s, marker); i >= 0 {
		return s[i:]
	}
	return s
}

func designJSON(t *testing.T, args ...string) []designPersona {
	t.Helper()
	out, _ := invoke(t, 0, append([]string{"sim", "design"}, append(args, "--format", "json")...)...)
	var got struct {
		Version int `json:"version"`
		Data    struct {
			Personas []designPersona `json:"personas"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if got.Version != 1 {
		t.Fatalf("format_version %d", got.Version)
	}
	return got.Data.Personas
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
	if jsonString(t, first) != jsonString(t, again) {
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
		clears := 0
		for _, e := range p.Events {
			if e.Kind == "clear" {
				clears++
			}
		}
		if clears != 1 {
			t.Errorf("%s: %d clear events", p.ID, clears)
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
	only := designJSON(t, dir, "--only", "eager", "--only", "steady")
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
	line, _, _ := strings.Cut(string(read(t, filepath.Join(first, "timeline", "eager.jsonl"))), "\n")
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
		t.Errorf("no comparison with the previous run:\n%s", tail(cmp, "## Change"))
	}
}

func TestDesignListAndInvalidInput(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "design", dir, "--list", "--format", "json")
	var cfg struct {
		Data struct {
			Personas []struct{ ID string }
			Release  string
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil || len(cfg.Data.Personas) != 3 || cfg.Data.Release != "gift" {
		t.Fatalf("--list: %v %q", err, out)
	}
	list := filepath.Join(t.TempDir(), "list")
	invoke(t, 2, "sim", "design", dir, "--list", "-o", list)
	if _, err := os.Lstat(list); err == nil {
		t.Error("--list wrote an output directory")
	}
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{dir, "--only", "nobody"}, 2, "unknown persona"},
		{[]string{dir, "--only", "nobody", "--format", "json"}, 2, "unknown persona"},
		{[]string{dir, "--format", "xml"}, 2, "format"},
		{[]string{dir, "--seed", "0"}, 2, "--seed must be positive"},
		{[]string{}, 2, "expected one game directory"},
		{[]string{dir, dir}, 2, "expected one game directory"},
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
		{"duel without a troop", "duels: {list: [{open: 'true'}]}\npersonas: [{id: a}]\n", "needs a troop"},
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

func TestDesignFormatPreservesSimulationAndArtifacts(t *testing.T) {
	dir := designGame(t)
	reference := designJSON(t, dir, "--seed", "43", "--only", "steady")
	for _, format := range []string{"text", "json", "jsonl"} {
		out := filepath.Join(t.TempDir(), "report")
		stdout, stderr := invoke(t, 0, "sim", "design", dir, "--seed", "43", "--only", "steady", "--out", out, "--format", format)
		if stderr != "" {
			t.Fatalf("unexpected progress in %s: %s", format, stderr)
		}
		for _, path := range []string{"summary.json", "report.md", "band.png", "timeline/steady.jsonl"} {
			read(t, filepath.Join(out, path))
		}
		var saved struct {
			Personas []designPersona `json:"personas"`
		}
		if err := json.Unmarshal(read(t, filepath.Join(out, "summary.json")), &saved); err != nil {
			t.Fatal(err)
		}
		if len(saved.Personas) != 1 || saved.Personas[0].Battles != reference[0].Battles || saved.Personas[0].Days != reference[0].Days {
			t.Fatalf("format changed simulation: %s", format)
		}
		if format == "text" {
			continue
		}
		records := strings.Split(strings.TrimSpace(stdout), "\n")
		want := 1
		if format == "jsonl" {
			want = 2
		}
		if len(records) != want {
			t.Fatalf("%s: %d records", format, len(records))
		}
		var result struct {
			Version int `json:"version"`
			Data    struct {
				Personas []designPersona `json:"personas"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(records[len(records)-1]), &result); err != nil {
			t.Fatal(err)
		}
		if result.Version != 1 || jsonString(t, result.Data.Personas) != jsonString(t, reference) {
			t.Fatalf("%s changed results", format)
		}
	}
	root := filepath.Join(t.TempDir(), "unused")
	invoke(t, 0, "sim", "design", dir, "--only", "steady", "--root", root)
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("no --out created files: %v", err)
	}
}

func TestDesignMarkdownEscapesTableHeadersAndCells(t *testing.T) {
	if err := lang.Use(lang.English, nil); err != nil {
		t.Fatal(err)
	}
	cfg := &designeval.Config{Release: "gift", Rewards: []designeval.Rule{{Kind: "gift", Label: "Gift|label\ncontinued"}}}
	sums := []designeval.Summary{{ID: "reader", Name: "Reader|guest\nname"}}
	out := designReport(cfg, []*designeval.Run{{}}, sums, "run", "hash", "", nil)
	for _, want := range []string{`Gift\|label<br>continued`, `Reader\|guest<br>name (reader)`} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks escaped cell %q", want)
		}
	}
}

// edit replaces text in a file of the game once and fails if it is not there.
func edit(t *testing.T, path, old, repl string) {
	t.Helper()
	s := string(read(t, path))
	if !strings.Contains(s, old) {
		t.Fatalf("%s lacks %q", path, old)
	}
	write(t, path, strings.Replace(s, old, repl, 1))
}

func designReportOf(t *testing.T, args ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "run")
	stdout, _ := invoke(t, 0, append([]string{"sim", "design", "-o", out}, args...)...)
	if strings.TrimSpace(stdout) != filepath.Join(out, "report.md") {
		t.Fatalf("stdout %q", stdout)
	}
	return string(read(t, filepath.Join(out, "report.md")))
}

// A guardian nobody can beat: nobody reaches the end, the targets are marked, and the table
// says how the fights went.
func TestDesignReportsPersonasThatDoNotReachTheEnd(t *testing.T) {
	dir := designGame(t)
	edit(t, filepath.Join(dir, "data", "world.yaml"), "stats: {mhp: 400, mmp: 0, atk: 30,", "stats: {mhp: 90000, mmp: 0, atk: 400,")
	report := designReportOf(t, dir)
	for _, want := range []string{
		"did not reach",
		"- Steady: did not reach the end (down to B3F, 8 days) ×",
		"- Eager vs Steady: one of them did not reach the end",
		"| B3F |",
		"Did not reach the end (",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestDesignMarksTargetsThatAreMissed(t *testing.T) {
	dir := designGame(t)
	// A tolerance of a hair: two personas that clear in different times are off target.
	edit(t, filepath.Join(dir, "designeval.yaml"), "within: 0.5", "within: 0.0001")
	report := designReportOf(t, dir)
	if !strings.Contains(report, "Eager vs Steady to the end") || !strings.Contains(report, "×") {
		t.Errorf("report:\n%s", report)
	}
	if !strings.Contains(report, "- Steady: reached the end") {
		t.Errorf("must_clear ○ line missing:\n%s", report)
	}
}

func TestDesignShowsHoursAndWhoGetsTheReward(t *testing.T) {
	dir := designGame(t)
	cfg := filepath.Join(dir, "designeval.yaml")
	edit(t, cfg, "battle: 5", "battle: 500")
	edit(t, cfg, "who: Anyone}", "who: Anyone}\n  - {kind: friend, label: Friend, events: \"^(gift)$\", names: {gift: The giver}, flow: gift}")
	edit(t, cfg, "release: gift", "release: friend")
	report := designReportOf(t, dir, "--only", "steady")
	if !regexp.MustCompile(`\d+h\d\dm`).MatchString(report) {
		t.Errorf("a run of more than an hour is shown in hours:\n%s", report)
	}
	if !strings.Contains(report, "reward targets: The giver ") || !strings.Contains(report, "reward routes: gift ") {
		t.Errorf("who and by which route the release came:\n%s", report)
	}
}

// A command that fails while a persona plays is not a failure of the tool: the report keeps
// the first of them and the exit status says the run found problems.
func TestDesignKeepsTheReportWhenThePlayFindsProblems(t *testing.T) {
	dir := designGame(t)
	edit(t, filepath.Join(dir, "data", "world.yaml"), "      - {id: gift, label: Open gift,", "      - {id: broken, label: Break it, do: [{set: {var.x: \"floor + 1\"}}]}\n      - {id: gift, label: Open gift,")
	edit(t, filepath.Join(dir, "designeval.yaml"), `rotate: ["@yes", "@no"]}`, `rotate: ["@yes", "@no"]}`+"\n      - {at: camp, do: \"@broken\"}")
	out := filepath.Join(t.TempDir(), "run")
	stdout, stderr := invoke(t, 1, "sim", "design", dir, "-o", out)
	if !strings.Contains(stderr, "reported simulation errors; see report.md") {
		t.Errorf("stderr: %q", stderr)
	}
	if strings.TrimSpace(stdout) != filepath.Join(out, "report.md") {
		t.Errorf("the report is still announced: %q", stdout)
	}
	report := string(read(t, filepath.Join(out, "report.md")))
	if !strings.Contains(report, "Could not play (") || !strings.Contains(report, "a function") {
		t.Errorf("report:\n%s", report)
	}
	var j struct {
		Personas []struct {
			Problems int `json:"problems"`
		}
	}
	if err := json.Unmarshal(read(t, filepath.Join(out, "summary.json")), &j); err != nil || j.Personas[0].Problems == 0 {
		t.Errorf("summary: %v %+v", err, j)
	}
}

func TestDesignComparesWithAPreviousRunThatLacksAPersona(t *testing.T) {
	dir := designGame(t)
	root := filepath.Join(t.TempDir(), "runs")
	invoke(t, 0, "sim", "design", dir, "--only", "steady", "-o", filepath.Join(root, "20200101-000000"))
	// A directory without a summary, and one with a summary of another format, are skipped.
	if err := os.MkdirAll(filepath.Join(root, "20200101-000001"), 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "20200101-000002", "summary.json"), `{"format_version": 2, "personas": []}`)
	write(t, filepath.Join(root, "20200101-000003", "summary.json"), `not json`)
	out, _ := invoke(t, 0, "sim", "design", dir, "--root", root, "-o", filepath.Join(root, "20200102-000000"))
	report := string(read(t, strings.TrimSpace(out)))
	if !strings.Contains(report, "Previous run: 20200101-000000") {
		t.Errorf("the newest usable run before this one is the one compared:\n%s", tail(report, "## Change"))
	}
	if !strings.Contains(report, "| Eager | (not in the previous run) |  |  |  |") {
		t.Errorf("a persona the previous run lacks is said so:\n%s", tail(report, "## Change"))
	}
	if !strings.Contains(report, "| Steady | To the end (min) |") {
		t.Errorf("rows of the persona present in both:\n%s", tail(report, "## Change"))
	}
	// A run is only compared with runs made before it.
	later, _ := invoke(t, 0, "sim", "design", dir, "--root", root, "-o", filepath.Join(root, "20190101-000000"))
	if r := string(read(t, strings.TrimSpace(later))); !strings.Contains(r, "No previous run") {
		t.Errorf("an older name has nothing before it:\n%s", tail(r, "## Change"))
	}
}

func TestDesignOutputPlacementAndVerboseLog(t *testing.T) {
	dir := designGame(t)
	// The report may not go inside the game, nor over an existing directory.
	invoke(t, 1, "sim", "design", dir, "-o", filepath.Join(dir, "report"))
	taken := t.TempDir()
	invoke(t, 1, "sim", "design", dir, "-o", taken)
	// A failed run leaves no half-made directory behind.
	bad := filepath.Join(t.TempDir(), "bad")
	cfg := filepath.Join(t.TempDir(), "cfg.yaml")
	write(t, cfg, "personas: [{id: a}]\nmemos: {dry: absent}\n")
	invoke(t, 1, "sim", "design", dir, "--config", cfg, "-o", bad)
	if _, err := os.Lstat(bad); err == nil {
		t.Error("a configuration that does not validate created the output")
	}
	// -v writes the persona's choices to debug.log; without it there is none.
	out := filepath.Join(t.TempDir(), "v")
	invoke(t, 0, "sim", "design", dir, "-v", "--only", "eager", "-o", out)
	log := string(read(t, filepath.Join(out, "debug.log")))
	if !strings.Contains(log, "eager d1 ") || !strings.Contains(log, "dive (deepest 0") {
		t.Errorf("debug.log:\n%s", log[:min(500, len(log))])
	}
	quiet := filepath.Join(t.TempDir(), "q")
	invoke(t, 0, "sim", "design", dir, "--only", "eager", "-o", quiet)
	if _, err := os.Stat(filepath.Join(quiet, "debug.log")); err == nil {
		t.Error("debug.log without -v")
	}
}

// A note variable that is not a plain value (the name of a function, not a number) is left out of
// the notes; it does not stop the play from being written down.
func TestDesignLeavesOutNoteVariablesThatAreNotValues(t *testing.T) {
	dir := designGame(t)
	edit(t, filepath.Join(dir, "game.yaml"), "vars: [{name: depth, value: var.floor}]", "vars: [{name: depth, value: var.floor}, {name: oops, value: floor}]")
	notes := filepath.Join(t.TempDir(), "notes")
	out := filepath.Join(t.TempDir(), "run")
	invoke(t, 0, "sim", "design", dir, "--only", "eager", "-o", out, "--memos", notes)
	found := 0
	if err := filepath.WalkDir(notes, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".json" {
			return err
		}
		b := string(read(t, p))
		if strings.Contains(b, `"oops"`) {
			t.Errorf("%s keeps a variable that is no value", p)
		}
		if strings.Contains(b, `"depth"`) {
			found++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Error("the variable that is a number is still kept")
	}
}
