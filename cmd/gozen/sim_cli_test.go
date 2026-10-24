package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

type diveFloor struct {
	Floor     int      `json:"floor"`
	Battles   int      `json:"battles"`
	Lost      int      `json:"lost"`
	Rested    bool     `json:"rested"`
	Boss      string   `json:"boss"`
	BossRate  float64  `json:"boss_rate"`
	Grinding  int      `json:"grinding"`
	Troops    []string `json:"troops"`
	Guard     string   `json:"guard"`
	Spent     int      `json:"spent"`
	Stranded  int      `json:"stranded"`
	Gold      int      `json:"gold"`
	HPLeft    float64  `json:"hp_left"`
	WentBack  bool     `json:"went_back"`
	GuardBack bool     `json:"guard_back"`
}

type diveReport struct {
	Floors []diveFloor `json:"floors"`
	Order  []string    `json:"order"`
}

func diveJSON(t *testing.T, args ...string) diveReport {
	t.Helper()
	out, _ := invoke(t, 0, append([]string{"sim", "dive", "--format", "json"}, args...)...)
	var r diveReport
	if err := json.Unmarshal(resultData(t, out, "sim dive"), &r); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	return r
}

func TestSimDiveReportsEachFloorByTheOptionsGiven(t *testing.T) {
	dir := designGame(t)
	r := diveJSON(t, dir, "--location", "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden")
	if len(r.Floors) != 4 {
		t.Fatalf("one entry for each floor asked: %d", len(r.Floors))
	}
	for i, f := range r.Floors {
		if f.Floor != i+1 {
			t.Errorf("floor %d is numbered %d", i+1, f.Floor)
		}
		if f.Boss == "" && f.Battles != 3 {
			t.Errorf("B%d: %d battles, want the 3 asked for", f.Floor, f.Battles)
		}
		if f.Boss != "" && (f.Floor != 3 || f.Boss != "warden") {
			t.Errorf("only floor 3 has the boss: %+v", f)
		}
	}
	if r.Floors[2].Boss != "warden" || r.Floors[2].BossRate < 0 || r.Floors[2].BossRate > 1 {
		t.Errorf("the boss of floor 3 has a win rate: %+v", r.Floors[2])
	}
	again := diveJSON(t, dir, "--location", "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden")
	if jsonString(t, r) != jsonString(t, again) {
		t.Error("the same seed gave a different dive")
	}
	other := diveJSON(t, dir, "--location", "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden", "--seed", "99")
	if jsonString(t, other) == jsonString(t, r) {
		t.Error("another seed gave the same dive")
	}
	if len(r.Order) == 0 {
		t.Error("the party is listed")
	}
}

func TestSimDiveEveryOptionRuns(t *testing.T) {
	dir := designGame(t)
	saves := filepath.Join(t.TempDir(), "arrivals")
	out, _ := invoke(t, 0, "sim", "dive", dir, "--location", "cave", "--floors", "3", "--battles", "2", "--tries", "4", "--boss", "3:warden", "--grind", "0.5",
		"--rest", "2", "--supplies", "tonic:2,tonic:1", "--shops", "smith", "--back-item", "shard", "--rest-cost", "members * 5", "--worn-hp", "0.5", "--worn-mp", "0.2",
		"--events", "--guards", "1:lesson,2:pair", "--retreat", "--floor-event", "home", "--rest-event", "drink", "--scene-event", "gift", "--scene-when", "gold > 0",
		"--arrival-saves", saves, "--turns", "20")
	for _, want := range []string{"floor Lv", "B1", "B2", "B3", "skills used (the run's own battles and the boss tries):", "Scout Lv", "Aide Lv"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	// An arrival save is written for every boss floor, and nowhere else.
	read(t, filepath.Join(saves, "floor-3.json"))
	r := diveJSON(t, dir, "--location", "cave", "--floors", "3", "--battles", "2", "--tries", "4", "--guards", "1:lesson", "--guards-only")
	if r.Floors[0].Guard != "lesson" {
		t.Errorf("--guards names the troop at the end of the floor: %+v", r.Floors[0])
	}
	if r.Floors[0].Battles != 2 {
		t.Errorf("guards-only measures a guard without fighting it: %+v", r.Floors[0])
	}
	r = diveJSON(t, dir, "--location", "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--no-town")
	for _, f := range r.Floors {
		if f.Spent != 0 || f.WentBack {
			t.Errorf("no town means no spending and no trip back: %+v", f)
		}
	}
	// The equipment of a shop is bought when it is better and affordable: the gold left is lower than without the shop.
	plain := diveJSON(t, dir, "--location", "cave", "--floors", "2", "--battles", "1", "--tries", "3")
	shopped := diveJSON(t, dir, "--location", "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--shops", "smith")
	if shopped.Floors[0].Gold >= plain.Floors[0].Gold {
		t.Errorf("the party kept its gold in front of the shop: %d vs %d", shopped.Floors[0].Gold, plain.Floors[0].Gold)
	}
	shops := diveJSON(t, dir, "--location", "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--shops", "smith", "--no-town")
	if shops.Floors[0].Gold != plain.Floors[0].Gold {
		t.Errorf("without the town nothing is bought: %d vs %d", shops.Floors[0].Gold, plain.Floors[0].Gold)
	}
}

func TestSimDiveRejectsInvalidOptions(t *testing.T) {
	dir := designGame(t)
	bad := filepath.Join(t.TempDir(), "taken")
	invoke(t, 0, "sim", "dive", dir, "--location", "cave", "--floors", "1", "--battles", "0", "--tries", "1", "--arrival-saves", bad)
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{nil, 2, "expected one game directory"},
		{[]string{dir}, 2, "--location ID is required"},
		{[]string{dir, "--location", "cave", "--floors", "0"}, 2, "invalid floor, battle, try, turn, or rest count"},
		{[]string{dir, "--location", "cave", "--battles", "-1"}, 2, "invalid floor"},
		{[]string{dir, "--location", "cave", "--tries", "0"}, 2, "invalid floor"},
		{[]string{dir, "--location", "cave", "--turns", "0"}, 2, "invalid floor"},
		{[]string{dir, "--location", "cave", "--rest", "-1"}, 2, "invalid floor"},
		{[]string{dir, "--location", "cave", "--grind", "1.5"}, 2, "rates must be finite values in [0,1]"},
		{[]string{dir, "--location", "cave", "--worn-hp", "NaN"}, 2, "rates must be"},
		{[]string{dir, "--location", "cave", "--worn-mp", "-0.1"}, 2, "rates must be"},
		{[]string{dir, "--location", "nowhere"}, 2, `unknown location "nowhere"`},
		{[]string{dir, "--location", "cave", "--boss", "x"}, 2, "--boss: expected unique positive floor:troop pairs"},
		{[]string{dir, "--location", "cave", "--boss", "0:warden"}, 2, "--boss"},
		{[]string{dir, "--location", "cave", "--boss", "1:warden,1:lesson"}, 2, "--boss"},
		{[]string{dir, "--location", "cave", "--boss", "1:"}, 2, "--boss"},
		{[]string{dir, "--location", "cave", "--boss", "1:ghost"}, 2, `unknown troop "ghost"`},
		{[]string{dir, "--location", "cave", "--supplies", "tonic"}, 1, "want item:count"},
		{[]string{dir, "--location", "cave", "--supplies", "ghost:1"}, 1, `unknown item "ghost"`},
		{[]string{dir, "--location", "cave", "--supplies", "tonic:0"}, 1, "positive integer"},
		{[]string{dir, "--location", "cave", "--supplies", "tonic:x"}, 1, "positive integer"},
		{[]string{dir, "--location", "cave", "--rest-event", "ghost"}, 1, "--rest-event: unknown event"},
		{[]string{dir, "--location", "cave", "--scene-event", "ghost", "--scene-when", "true"}, 1, "--scene-event: unknown event"},
		{[]string{dir, "--location", "cave", "--scene-event", "gift"}, 1, "--scene-event and --scene-when go together"},
		{[]string{dir, "--location", "cave", "--scene-event", "gift", "--scene-when", "("}, 1, "--scene-when"},
		{[]string{dir, "--location", "cave", "--back-item", "ghost"}, 1, "--back-item: unknown item"},
		{[]string{dir, "--location", "cave", "--guards", "1:ghost"}, 2, "--guards"},
		{[]string{dir, "--location", "cave", "--guards", "1:lesson,1:pair"}, 2, "--guards"},
		{[]string{dir, "--location", "cave", "--guards", "x:lesson"}, 2, "--guards"},
		{[]string{dir, "--location", "cave", "--floor-event", "ghost"}, 1, "--floor-event: unknown event"},
		{[]string{dir, "--location", "cave", "--format", "xml"}, 2, "text, json, or jsonl"},
		{[]string{dir, "--location", "cave", "--arrival-saves", bad}, 1, ""},
		{[]string{dir, "--location", "cave", "--shops", "ghost"}, 2, `unknown shop "ghost"`},
		{[]string{dir, "--location", "cave", "--rest-cost", "("}, 2, "--rest-cost"},
		{[]string{filepath.Join(t.TempDir(), "absent"), "--location", "cave"}, 1, diag.GameUnreadable.String()},
	} {
		out, stderr := invoke(t, tc.want, append([]string{"sim", "dive"}, tc.args...)...)
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: stdout %q stderr %q (want %q)", tc.args, out, stderr, tc.msg)
		}
	}
}

func TestSimFloorsShowsMadeSeededAndDrawnFloors(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--to", "4")
	for _, want := range []string{"B1F (seed 7)  11x11 open ", "B2F (seed 7)", "B3F (drawn)  9x3 open 7 deadends 4  down -1", "B4F (seed 7)", "#########\n#S.G.B.T#\n#########"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	noMap, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--from", "2", "--to", "3", "--no-map")
	if strings.Contains(noMap, "###") || strings.Count(noMap, "\nB")+1 != 2 || strings.Contains(noMap, "B1F") {
		t.Errorf("--no-map prints the numbers of floors 2 and 3 only:\n%s", noMap)
	}
	one, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--floor", "3", "--no-map")
	if strings.Count(one, "\n") != 1 || !strings.HasPrefix(one, "B3F (drawn)") {
		t.Errorf("--floor 3: %q", one)
	}
	// Comparing seeds leaves the game's own floor untouched.
	try, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--floor", "2", "--try", "3", "--no-map")
	lines := strings.Split(strings.TrimSpace(try), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "seed   0  11x11 open ") || !strings.HasPrefix(lines[2], "seed   2  ") {
		t.Errorf("--try 3 gives one line for each seed:\n%s", try)
	}
	if again, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--to", "4"); again != out {
		t.Error("looking at seeds changed the floors")
	}
	// A note on a floor is printed after its numbers.
	world := filepath.Join(dir, "data", "world.yaml")
	write(t, world, strings.Replace(string(read(t, world)), "          3:\n            map: |", "          3:\n            note: the guardian's room\n            map: |", 1))
	if noted, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--floor", "3", "--no-map"); !strings.Contains(noted, "\n  the guardian's room\n") {
		t.Errorf("note: %q", noted)
	}
}

func TestSimFloorsRejectsWhatItCannotShow(t *testing.T) {
	dir := designGame(t)
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{dir}, "--location ID is required"},
		{[]string{dir, "--location", "cave", "--from", "0"}, "invalid floor range"},
		{[]string{dir, "--location", "cave", "--from", "5", "--to", "2"}, "invalid floor range"},
		{[]string{dir, "--location", "cave", "--floor", "-1"}, "invalid floor range"},
		{[]string{dir, "--location", "cave", "--try", "2"}, "--try requires --floor"},
		{[]string{dir, "--location", "camp"}, "is not a generated dungeon"},
		{[]string{dir, "--location", "nowhere"}, "is not a generated dungeon"},
	} {
		out, stderr := invoke(t, 2, append([]string{"sim", "floors"}, tc.args...)...)
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: %q %q", tc.args, out, stderr)
		}
	}
}

func TestSimBattleOptionsAndJSON(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "pair", "-n", "20", "--format", "json", "--hp", "--streak", "5", "--level", "6", "--gear", "blade")
	var rs []simResult
	if err := json.Unmarshal(resultData(t, out, "sim battle"), &rs); err != nil || len(rs) != 1 {
		t.Fatalf("%v %q", err, out)
	}
	r := rs[0]
	if r.Troop != "pair" || r.Battles != 20 || r.Won+r.Lost+r.Fled+r.Draw != 20 {
		t.Fatalf("every battle has one result: %+v", r)
	}
	if want := float64(r.Won) / 20; r.WinRate != want {
		t.Errorf("win rate %v, want %v", r.WinRate, want)
	}
	if r.Won > 0 && r.HP == nil {
		t.Error("--hp adds the HP left after a win")
	}
	if r.Streak == nil || r.Streak.P10 > r.Streak.P50 || r.Streak.P50 > r.Streak.P90 || r.Streak.P90 > 5 {
		t.Errorf("streak percentiles are ordered and capped by --streak: %+v", r.Streak)
	}
	text, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "pair", "-n", "20", "--hp", "--streak", "5", "--level", "6", "--gear", "blade", "--party", "scout", "--no-tree", "--turns", "30")
	for _, want := range []string{"pair: 20 battles  win ", "hp left after a win: p10 ", "wins in a row without recovery (up to 5): p10 "} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q: %s", want, text)
		}
	}
	verbose, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "lesson", "-n", "1", "-v")
	if !strings.Contains(verbose, "  ") || !strings.Contains(verbose, "lesson: 1 battles") {
		t.Errorf("verbose prints the messages of the first battle: %q", verbose)
	}
	if quiet, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "lesson", "-n", "1", "-v", "--format", "json"); strings.Contains(quiet, "-- turn") {
		t.Error("json output has no battle log")
	}
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{dir}, 2, "--troop ID is required"},
		{[]string{dir, "--troop", "ghost"}, 2, `unknown troop "ghost"`},
		{[]string{dir, "--troop", "lesson", "--format", "xml"}, 2, "text, json, or jsonl"},
		{[]string{dir, "--troop", "lesson", "--level", "-1"}, 2, "--level must be nonnegative"},
		{[]string{dir, "--troop", "lesson", "--party", "ghost"}, 1, ""},
		{[]string{dir, "--troop", "lesson", "--gear", "ghost"}, 1, ""},
		{[]string{dir, "--troop", "lesson", "--state", "a.save", "--state", "b.save", "-v"}, 2, "--verbose goes with one start"},
	} {
		_, stderr := invoke(t, tc.want, append([]string{"sim", "battle"}, tc.args...)...)
		if !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: %q", tc.args, stderr)
		}
	}
}

func TestSimFloorsCountsWalksThroughWarpsAndEncounterGauges(t *testing.T) {
	dir := designGame(t)
	world := filepath.Join(dir, "data", "world.yaml")
	// A warp tile on the way: the walk to the stairs is shorter through it, and the encounter gauge changes the expected fights.
	edit(t, world, "          - {char: T, count: 1}", "          - {char: T, count: 1}\n          - {char: W, count: 2}")
	edit(t, world, "        T: {name: Chest,", "        W: {name: Warp, do: [{move: cave, at: S}]}\n        T: {name: Chest,")
	edit(t, world, "encounters: {rate: 0.2,", "encounters: {rate: 0.2, gauge: 0.5,")
	out, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--floor", "1", "--no-map")
	if !strings.HasPrefix(out, "B1F (seed 7)") || !strings.Contains(out, " W:") {
		t.Fatalf("the warp tiles are listed with their distance: %q", out)
	}
	plain := designGame(t)
	base, _ := invoke(t, 0, "sim", "floors", plain, "--location", "cave", "--floor", "1", "--no-map")
	if out == base {
		t.Error("a gauge and warps change the numbers")
	}
	try, _ := invoke(t, 0, "sim", "floors", dir, "--location", "cave", "--floor", "1", "--try", "2", "--no-map")
	if strings.Count(try, "seed ") != 2 {
		t.Errorf("--try: %q", try)
	}
}

func TestSimBattleVerboseShowsTheTurnsAndSeveralStarts(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "warden", "-n", "1", "-v", "--level", "6")
	if !strings.Contains(out, "-- turn 2") || !strings.Contains(out, "warden: 1 battles") {
		t.Errorf("a battle of several turns is printed turn by turn:\n%s", out)
	}
	saves := filepath.Join(t.TempDir(), "saves")
	invoke(t, 0, "sim", "dive", dir, "--location", "cave", "--floors", "3", "--battles", "0", "--tries", "2", "--boss", "3:warden", "--arrival-saves", saves)
	state := filepath.Join(saves, "floor-3.json")
	both, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "warden", "-n", "4", "--state", state, "--state", state, "--as-is")
	lines := strings.Split(strings.TrimSpace(both), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], state+" warden: 4 battles") {
		t.Errorf("each start is named when there are several:\n%s", both)
	}
	js, _ := invoke(t, 0, "sim", "battle", dir, "--troop", "warden", "-n", "4", "--state", state, "--state", state, "--format", "json")
	var rs []simResult
	if err := json.Unmarshal(resultData(t, js, "sim battle"), &rs); err != nil || len(rs) != 2 || rs[0].State != state || rs[0].WinRate != rs[1].WinRate {
		t.Errorf("%v %+v", err, rs)
	}
}

func TestSimDiveListsEverySkillAndWhatWasNeverUsed(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "dive", dir, "--location", "cave", "--floors", "3", "--battles", "4", "--tries", "3", "--boss", "3:warden", "--grind", "0.5")
	// Both members of the party are listed with the skills they used, most used first.
	for _, want := range []string{"  Scout Lv", "  Aide Lv", ": Tap "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "never used: Spark") {
		t.Errorf("a skill nobody used is named:\n%s", out)
	}
	if !strings.Contains(out, "100%") {
		t.Errorf("the boss row shows the win rate at arrival:\n%s", out)
	}
}

func TestSimBattleFormsAreDeterministic(t *testing.T) {
	for _, tc := range []struct {
		troop string
		args  []string
		wins  int
	}{
		{"drips", []string{"--level", "5"}, 28},
		{"tollkeeper", []string{"--level", "9", "--party", "maren,ox,tilly,kes"}, 30},
	} {
		t.Run(tc.troop, func(t *testing.T) {
			args := append([]string{"sim", "battle", "--battles", "30", "--seed", "42", "--format", "json"}, tc.args...)
			args = append(args, "../../examples/go-zen/game", "--troop", tc.troop)
			out, _ := invoke(t, 0, args...)
			var rows []struct{ Won, Lost, Draw int }
			if err := json.Unmarshal(resultData(t, out, "sim battle"), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Won != tc.wins || rows[0].Won+rows[0].Lost+rows[0].Draw != 30 {
				t.Fatalf("simulation: %+v", rows)
			}
		})
	}
}

func TestSimFloorsStructuredOutputPreservesMapsAndSeeds(t *testing.T) {
	dir := designGame(t)
	args := []string{"sim", "floors", dir, "--location", "cave", "--floor", "1"}
	text, _ := invoke(t, 0, args...)
	output, _ := invoke(t, 0, append(args, "--format", "json")...)
	var rows []floorResult
	if err := json.Unmarshal(resultData(t, output, "sim floors"), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Seed == nil || *rows[0].Seed != 7 || rows[0].Map == "" || !strings.Contains(text, rows[0].Map) || !strings.Contains(text, rows[0].Numbers.line()) {
		t.Fatalf("JSON changed the generated floor: %s", output)
	}
	output, _ = invoke(t, 0, append(args, "--try", "2", "--format", "jsonl")...)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("seed records: %s", output)
	}
	for i, line := range lines[:2] {
		var row floorResult
		if err := json.Unmarshal(resultData(t, line, "sim floors"), &row); err != nil {
			t.Fatal(err)
		}
		if row.Seed == nil || *row.Seed != uint64(i) || row.Floor != 1 || row.Map != "" || row.Numbers.Width <= 0 {
			t.Fatalf("seed comparison changed or omitted a value: %s", line)
		}
	}
}

func TestSimulationMachineFormatsAgree(t *testing.T) {
	dir := fixture(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"battle", []string{dir, "--troop", "lesson", "--battles", "2", "--seed", "7"}},
		{"dive", []string{dir, "--location", "grotto", "--floors", "2", "--battles", "1", "--tries", "1", "--seed", "7"}},
		{"floors", []string{dir, "--location", "grotto", "--from", "1", "--to", "2"}},
		{"campaign", []string{"../../examples/campaign/game", "--campaigns", "1", "--compare", "strategies", "--seed", "7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"sim", tc.name}, tc.args...)
			document, stderr := invoke(t, 0, append(args, "--format", "json")...)
			if stderr != "" {
				t.Fatal(stderr)
			}
			stream, stderr := invoke(t, 0, append(args, "--format", "jsonl")...)
			if stderr != "" {
				t.Fatal(stderr)
			}
			lines := strings.Split(strings.TrimSpace(stream), "\n")
			if len(lines) < 2 {
				t.Fatalf("missing items: %s", stream)
			}
			for _, line := range lines[:len(lines)-1] {
				var item struct {
					Type string          `json:"type"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal([]byte(line), &item); err != nil {
					t.Fatal(err)
				}
				if item.Type != "item" || len(item.Data) == 0 {
					t.Fatalf("invalid item: %s", line)
				}
			}
			if string(resultData(t, document, "sim "+tc.name)) != string(resultData(t, lines[len(lines)-1], "sim "+tc.name)) {
				t.Fatalf("JSON and JSONL disagree: %s\n%s", document, stream)
			}
		})
	}
}
