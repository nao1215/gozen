package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// floorOne replaces the made first floor of the design sample with a drawn one.
func floorOne(t *testing.T, dir, floor string) {
	t.Helper()
	var b strings.Builder
	for _, row := range strings.Split(strings.TrimSpace(floor), "\n") {
		b.WriteString("              " + row + "\n")
	}
	edit(t, filepath.Join(dir, "data", "world.yaml"), "        floors:\n          3:", "        floors:\n          1:\n            map: |\n"+b.String()+"          3:")
}

func onlyPersona(t *testing.T, dir, id string) designPersona {
	t.Helper()
	got := designJSON(t, dir, "--only", id)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	return got[0]
}

func TestDesignWalksThroughATrapOnTheOnlyWayDown(t *testing.T) {
	dir := designGame(t)
	// The stairs lie behind a tile that stops the party: the bot goes through it when there is no way round.
	edit(t, filepath.Join(dir, "data", "world.yaml"), "        T: {name: Chest,", "        X: {name: Trap, do: [{say: A trap snaps shut.}]}\n        T: {name: Chest,")
	floorOne(t, dir, "#########\n#S.X.>..#\n#########")
	p := onlyPersona(t, dir, "eager")
	if p.Deepest < 2 || len(p.Problems) != 0 {
		t.Errorf("the bot should pass the trap and go down: deepest %d problems %v", p.Deepest, p.Problems)
	}
}

func TestDesignReportsAFloorItCannotLeave(t *testing.T) {
	dir := designGame(t)
	// The stairs are walled in.
	floorOne(t, dir, "#########\n#S..#>..#\n#########")
	out := filepath.Join(t.TempDir(), "run")
	_, stderr := invoke(t, 1, "sim", "design", dir, "--only", "eager", "-o", out, "-v", "--memos", filepath.Join(t.TempDir(), "memos"))
	if !strings.Contains(stderr, "reported simulation errors") {
		t.Errorf("stderr: %q", stderr)
	}
	report := string(read(t, filepath.Join(out, "report.md")))
	if !strings.Contains(report, "could not go down from floor 1") {
		t.Errorf("the report names the floor:\n%s", report)
	}
	log := string(read(t, filepath.Join(out, "debug.log")))
	if !strings.Contains(log, "could not go down") || !strings.Contains(log, "#S..#*") && !strings.Contains(log, "#*") {
		t.Errorf("the log draws the map with the party:\n%s", log[:min(900, len(log))])
	}
}

func TestDesignKeepsGrindingUntilTheGuardianCanBeBeaten(t *testing.T) {
	dir := designGame(t)
	cfg := filepath.Join(dir, "designeval.yaml")
	edit(t, cfg, "personas:\n", "personas:\n  - {id: careful, name: Careful, days: 8, explore: stairs, floors: 3, grind: 0.9}\n")
	p := onlyPersona(t, dir, "careful")
	if len(p.Bosses) != 1 || !p.Bosses[0].Measured {
		t.Fatalf("%+v", p.Bosses)
	}
	if p.Bosses[0].Tries < 1 {
		t.Errorf("the guardian was met: %+v", p.Bosses)
	}
	if !p.Cleared {
		t.Errorf("a persona that fights until the guardian is beatable clears the game: %+v", p)
	}
	var grind int
	for _, b := range designReportBoss(t, dir, "careful") {
		grind = b
	}
	if grind == 0 {
		t.Error("the report says how many fights were added")
	}
}

func designReportBoss(t *testing.T, dir, id string) []int {
	t.Helper()
	out := filepath.Join(t.TempDir(), "run")
	invoke(t, 0, "sim", "design", dir, "--only", id, "-o", out)
	report := string(read(t, filepath.Join(out, "report.md")))
	var got []int
	for _, l := range strings.Split(report, "\n") {
		if !strings.HasPrefix(l, "| B3F |") {
			continue
		}
		// "100% Lv4, 3 fights 2 wins, +10"
		if i := strings.LastIndex(l, ", +"); i >= 0 {
			n := 0
			for _, c := range l[i+3:] {
				if c < '0' || c > '9' {
					break
				}
				n = n*10 + int(c-'0')
			}
			got = append(got, n)
		}
	}
	return got
}

func TestDesignTownFallsBackToEveryPlaceAndNightTalks(t *testing.T) {
	dir := designGame(t)
	cfg := filepath.Join(dir, "designeval.yaml")
	// No town list: every place that is neither a dungeon, the inn nor an evening place is visited.
	edit(t, cfg, "  town: [camp]\n", "")
	// The night is spent talking to the host, then sleeping.
	edit(t, cfg, "      - {id: drink, phase: night, try: [{item: tonic}]}\n", "")
	p := onlyPersona(t, dir, "steady")
	if !p.Cleared {
		t.Errorf("steady still clears: %+v", p)
	}
	// Talk and sleep are the same entry: talking is the whole night.
	edit(t, cfg, "  talk: Host\n", "  talk: Sleep\n")
	if p := onlyPersona(t, dir, "steady"); !p.Cleared {
		t.Errorf("steady with a night that is one entry: %+v", p)
	}
}

func TestDesignGoesOneFloorDeeperEachDayWhenTheGoalIsOneFloor(t *testing.T) {
	dir := designGame(t)
	cfg := filepath.Join(dir, "designeval.yaml")
	// A persona that wants one new floor a day goes home from there.
	edit(t, cfg, "personas:\n", "personas:\n  - {id: shallow, name: Shallow, days: 2, explore: events, floors: 1}\n")
	edit(t, cfg, "  bosses: {3: warden}\n", "  bosses: {}\n")
	edit(t, cfg, "  guard_tile: B\n", "")
	p := onlyPersona(t, dir, "shallow")
	if p.Cleared || p.Wipes != 0 || len(p.Bosses) != 0 {
		t.Errorf("without a guardian nothing ends the game: %+v", p)
	}
	if p.Days != 2 || p.Deepest != 2 {
		t.Errorf("one more floor on each of the 2 days: days %d deepest %d", p.Days, p.Deepest)
	}
}
