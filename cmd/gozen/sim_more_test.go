package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

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
	if err := json.Unmarshal([]byte(js), &rs); err != nil || len(rs) != 2 || rs[0].State != state || rs[0].WinRate != rs[1].WinRate {
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

func TestPackTrialListsTheRangesOfVariables(t *testing.T) {
	dir := designGame(t)
	write(t, filepath.Join(dir, "trial.yaml"), "{}\n")
	list := filepath.Join(t.TempDir(), "list.txt")
	invoke(t, 0, "trial", dir, "--out", filepath.Join(t.TempDir(), "trial"), "--list", list)
	text := string(read(t, list))
	if !strings.Contains(text, "# ranges of variables and flags\n") || !strings.Contains(text, "var.offer = ") || !strings.Contains(text, "<- ") {
		t.Errorf("the ranges of the variables are listed with what sets them:\n%s", text)
	}
}
