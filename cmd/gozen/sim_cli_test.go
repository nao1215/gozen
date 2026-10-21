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
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	return r
}

func TestSimDiveReportsEachFloorByTheOptionsGiven(t *testing.T) {
	dir := designGame(t)
	r := diveJSON(t, dir, "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden")
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
	again := diveJSON(t, dir, "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden")
	if jsonString(t, r) != jsonString(t, again) {
		t.Error("the same seed gave a different dive")
	}
	other := diveJSON(t, dir, "cave", "--floors", "4", "--battles", "3", "--tries", "5", "--boss", "3:warden", "--seed", "99")
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
	out, _ := invoke(t, 0, "sim", "dive", dir, "cave", "--floors", "3", "--battles", "2", "--tries", "4", "--boss", "3:warden", "--grind", "0.5",
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
	r := diveJSON(t, dir, "cave", "--floors", "3", "--battles", "2", "--tries", "4", "--guards", "1:lesson", "--guards-only")
	if r.Floors[0].Guard != "lesson" {
		t.Errorf("--guards names the troop at the end of the floor: %+v", r.Floors[0])
	}
	if r.Floors[0].Battles != 2 {
		t.Errorf("guards-only measures a guard without fighting it: %+v", r.Floors[0])
	}
	r = diveJSON(t, dir, "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--no-town")
	for _, f := range r.Floors {
		if f.Spent != 0 || f.WentBack {
			t.Errorf("no town means no spending and no trip back: %+v", f)
		}
	}
	// The equipment of a shop is bought when it is better and affordable: the gold left is lower than without the shop.
	plain := diveJSON(t, dir, "cave", "--floors", "2", "--battles", "1", "--tries", "3")
	shopped := diveJSON(t, dir, "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--shops", "smith")
	if shopped.Floors[0].Gold >= plain.Floors[0].Gold {
		t.Errorf("the party kept its gold in front of the shop: %d vs %d", shopped.Floors[0].Gold, plain.Floors[0].Gold)
	}
	shops := diveJSON(t, dir, "cave", "--floors", "2", "--battles", "1", "--tries", "3", "--shops", "smith", "--no-town")
	if shops.Floors[0].Gold != plain.Floors[0].Gold {
		t.Errorf("without the town nothing is bought: %d vs %d", shops.Floors[0].Gold, plain.Floors[0].Gold)
	}
}

func TestSimDiveRejectsInvalidOptions(t *testing.T) {
	dir := designGame(t)
	bad := filepath.Join(t.TempDir(), "taken")
	invoke(t, 0, "sim", "dive", dir, "cave", "--floors", "1", "--battles", "0", "--tries", "1", "--arrival-saves", bad)
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{nil, 2, "expected <game-dir> <location>"},
		{[]string{dir}, 2, "expected <game-dir> <location>"},
		{[]string{dir, "cave", "--floors", "0"}, 2, "invalid floor, battle, try, turn, or rest count"},
		{[]string{dir, "cave", "--battles", "-1"}, 2, "invalid floor"},
		{[]string{dir, "cave", "--tries", "0"}, 2, "invalid floor"},
		{[]string{dir, "cave", "--turns", "0"}, 2, "invalid floor"},
		{[]string{dir, "cave", "--rest", "-1"}, 2, "invalid floor"},
		{[]string{dir, "cave", "--grind", "1.5"}, 2, "rates must be finite values in [0,1]"},
		{[]string{dir, "cave", "--worn-hp", "NaN"}, 2, "rates must be"},
		{[]string{dir, "cave", "--worn-mp", "-0.1"}, 2, "rates must be"},
		{[]string{dir, "nowhere"}, 2, `unknown location "nowhere"`},
		{[]string{dir, "cave", "--boss", "x"}, 2, "--boss: expected unique positive floor:troop pairs"},
		{[]string{dir, "cave", "--boss", "0:warden"}, 2, "--boss"},
		{[]string{dir, "cave", "--boss", "1:warden,1:lesson"}, 2, "--boss"},
		{[]string{dir, "cave", "--boss", "1:"}, 2, "--boss"},
		{[]string{dir, "cave", "--boss", "1:ghost"}, 2, `unknown troop "ghost"`},
		{[]string{dir, "cave", "--supplies", "tonic"}, 1, "want item:count"},
		{[]string{dir, "cave", "--supplies", "ghost:1"}, 1, `unknown item "ghost"`},
		{[]string{dir, "cave", "--supplies", "tonic:0"}, 1, "positive integer"},
		{[]string{dir, "cave", "--supplies", "tonic:x"}, 1, "positive integer"},
		{[]string{dir, "cave", "--rest-event", "ghost"}, 1, "--rest-event: unknown event"},
		{[]string{dir, "cave", "--scene-event", "ghost", "--scene-when", "true"}, 1, "--scene-event: unknown event"},
		{[]string{dir, "cave", "--scene-event", "gift"}, 1, "--scene-event and --scene-when go together"},
		{[]string{dir, "cave", "--scene-event", "gift", "--scene-when", "("}, 1, "--scene-when"},
		{[]string{dir, "cave", "--back-item", "ghost"}, 1, "--back-item: unknown item"},
		{[]string{dir, "cave", "--guards", "1:ghost"}, 2, "--guards"},
		{[]string{dir, "cave", "--guards", "1:lesson,1:pair"}, 2, "--guards"},
		{[]string{dir, "cave", "--guards", "x:lesson"}, 2, "--guards"},
		{[]string{dir, "cave", "--floor-event", "ghost"}, 1, "--floor-event: unknown event"},
		{[]string{dir, "cave", "--format", "xml"}, 2, "text or json"},
		{[]string{dir, "cave", "--arrival-saves", bad}, 1, ""},
		{[]string{dir, "cave", "--shops", "ghost"}, 2, `unknown shop "ghost"`},
		{[]string{dir, "cave", "--rest-cost", "("}, 2, "--rest-cost"},
		{[]string{filepath.Join(t.TempDir(), "absent"), "cave"}, 1, diag.GameUnreadable.String()},
	} {
		out, stderr := invoke(t, tc.want, append([]string{"sim", "dive"}, tc.args...)...)
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: stdout %q stderr %q (want %q)", tc.args, out, stderr, tc.msg)
		}
	}
}

func TestSimFloorsShowsMadeSeededAndDrawnFloors(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--to", "4")
	for _, want := range []string{"B1F (seed 7)  11x11 open ", "B2F (seed 7)", "B3F (drawn)  9x3 open 7 deadends 4  down -1", "B4F (seed 7)", "#########\n#S.G.B.T#\n#########"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	noMap, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--from", "2", "--to", "3", "--no-map")
	if strings.Contains(noMap, "###") || strings.Count(noMap, "\nB")+1 != 2 || strings.Contains(noMap, "B1F") {
		t.Errorf("--no-map prints the numbers of floors 2 and 3 only:\n%s", noMap)
	}
	one, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--floor", "3", "--no-map")
	if strings.Count(one, "\n") != 1 || !strings.HasPrefix(one, "B3F (drawn)") {
		t.Errorf("--floor 3: %q", one)
	}
	// Comparing seeds leaves the game's own floor untouched.
	try, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--floor", "2", "--try", "3", "--no-map")
	lines := strings.Split(strings.TrimSpace(try), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "seed   0  11x11 open ") || !strings.HasPrefix(lines[2], "seed   2  ") {
		t.Errorf("--try 3 gives one line for each seed:\n%s", try)
	}
	if again, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--to", "4"); again != out {
		t.Error("looking at seeds changed the floors")
	}
	// A note on a floor is printed after its numbers.
	world := filepath.Join(dir, "data", "world.yaml")
	write(t, world, strings.Replace(string(read(t, world)), "          3:\n            map: |", "          3:\n            note: the guardian's room\n            map: |", 1))
	if noted, _ := invoke(t, 0, "sim", "floors", dir, "cave", "--floor", "3", "--no-map"); !strings.Contains(noted, "\n  the guardian's room\n") {
		t.Errorf("note: %q", noted)
	}
}

func TestSimFloorsRejectsWhatItCannotShow(t *testing.T) {
	dir := designGame(t)
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{dir}, "expected <game-dir> <location>"},
		{[]string{dir, "cave", "--from", "0"}, "invalid floor range"},
		{[]string{dir, "cave", "--from", "5", "--to", "2"}, "invalid floor range"},
		{[]string{dir, "cave", "--floor", "-1"}, "invalid floor range"},
		{[]string{dir, "cave", "--try", "2"}, "--try requires --floor"},
		{[]string{dir, "camp"}, "is not a generated dungeon"},
		{[]string{dir, "nowhere"}, "is not a generated dungeon"},
	} {
		out, stderr := invoke(t, 2, append([]string{"sim", "floors"}, tc.args...)...)
		if out != "" || !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: %q %q", tc.args, out, stderr)
		}
	}
}

func TestSimBattleOptionsAndJSON(t *testing.T) {
	dir := designGame(t)
	out, _ := invoke(t, 0, "sim", "battle", dir, "pair", "-n", "20", "--format", "json", "--hp", "--streak", "5", "--level", "6", "--gear", "blade")
	var rs []simResult
	if err := json.Unmarshal([]byte(out), &rs); err != nil || len(rs) != 1 {
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
	text, _ := invoke(t, 0, "sim", "battle", dir, "pair", "-n", "20", "--hp", "--streak", "5", "--level", "6", "--gear", "blade", "--party", "scout", "--no-tree", "--turns", "30")
	for _, want := range []string{"pair: 20 battles  win ", "hp left after a win: p10 ", "wins in a row without recovery (up to 5): p10 "} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q: %s", want, text)
		}
	}
	verbose, _ := invoke(t, 0, "sim", "battle", dir, "lesson", "-n", "1", "-v")
	if !strings.Contains(verbose, "  ") || !strings.Contains(verbose, "lesson: 1 battles") {
		t.Errorf("verbose prints the messages of the first battle: %q", verbose)
	}
	if quiet, _ := invoke(t, 0, "sim", "battle", dir, "lesson", "-n", "1", "-v", "--format", "json"); strings.Contains(quiet, "-- turn") {
		t.Error("json output has no battle log")
	}
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{dir}, 2, "expected <game-dir> <troop>"},
		{[]string{dir, "ghost"}, 2, `unknown troop "ghost"`},
		{[]string{dir, "lesson", "--format", "xml"}, 2, "text or json"},
		{[]string{dir, "lesson", "--level", "-1"}, 2, "--level must be nonnegative"},
		{[]string{dir, "lesson", "--party", "ghost"}, 1, ""},
		{[]string{dir, "lesson", "--gear", "ghost"}, 1, ""},
		{[]string{dir, "lesson", "--state", "a.save", "--state", "b.save", "-v"}, 2, "--verbose goes with one start"},
	} {
		_, stderr := invoke(t, tc.want, append([]string{"sim", "battle"}, tc.args...)...)
		if !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: %q", tc.args, stderr)
		}
	}
}
