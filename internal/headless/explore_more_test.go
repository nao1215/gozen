package headless_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const exploreExtra = `
music:
  songs: {theme: {file: x.png}}
events:
  tour:
    do:
      - say: {who: scout, text: Hi}
      - notice: Heads up
      - music: {song: theme}
      - sfx: ding
      - mood: calm
      - image: {file: x.png}
      - move: field
  end_game:
    do: [{say: bye}, {clear: true}, {say: never}]
  lose_it:
    do:
      - battle: boss
      - say: after
  retried:
    do: [{say: Back on your feet}]
  forever:
    do: [{if: "true", then: [{call: forever}]}]
  scripted:
    do:
      - choice: [{label: A, do: []}, {label: B, do: []}]
      - battle: one
`

// TestExploreRecordsEverythingInOrder: lines, notices, music, sounds, pictures and moves are kept in the order they happened.
func TestExploreRecordsEverythingInOrder(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": exploreExtra})
	st := testgame.State(db, 1)
	paths := headless.Explore(db, st, db.Events["tour"].Do, headless.Config{})
	if len(paths) != 1 {
		t.Fatalf("a script with no choices has one path, got %d", len(paths))
	}
	p := paths[0]
	var kinds []string
	for _, s := range p.Steps {
		kinds = append(kinds, s.Kind)
	}
	if got := strings.Join(kinds, ","); !strings.HasPrefix(got, "say,notice,music,sfx,image,move") {
		t.Errorf("steps %s", got)
	}
	if len(p.Notices) != 1 || p.Notices[0] != "Heads up" || len(p.Moves) != 1 || p.Moves[0] != "field" || len(p.Images) != 1 {
		t.Errorf("notices %v moves %v images %v", p.Notices, p.Moves, p.Images)
	}
	if p.Log() != "" && !strings.Contains(p.Log(), "") {
		t.Error("Log")
	}
	if len(p.Errs) != 0 {
		t.Errorf("errors %v", p.Errs)
	}
	// arriving in the field ran the enter event of the field unless NoArrive
	if !slices.ContainsFunc(p.Lines, func(l string) bool { return strings.Contains(l, "The trail opens.") }) {
		t.Errorf("the field's arrival event was not run: %v", p.Lines)
	}
	q := headless.Explore(db, st, db.Events["tour"].Do, headless.Config{NoArrive: true})[0]
	if slices.ContainsFunc(q.Lines, func(l string) bool { return strings.Contains(l, "The trail opens.") }) {
		t.Errorf("NoArrive still ran the arrival: %v", q.Lines)
	}
}

func TestExploreClearStopsThePath(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": exploreExtra})
	p := headless.Explore(db, testgame.State(db, 1), db.Events["end_game"].Do, headless.Config{})[0]
	if !p.Cleared || slices.Contains(p.Lines, "never") || p.Taken[len(p.Taken)-1] != "clear" {
		t.Errorf("cleared %v lines %v taken %v", p.Cleared, p.Lines, p.Taken)
	}
}

// TestExploreGameOverRunsTheLossEventOnce: a lost battle where losing is not allowed ends the path as game over,
// and the game's game_over event runs once.
func TestExploreGameOverRunsTheLossEventOnce(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.Game + "game_over: {event: retried}\n", "data/z.yaml": exploreExtra})
	paths := headless.Explore(db, testgame.State(db, 1), db.Events["lose_it"].Do, headless.Config{})
	var lost, won *headless.Path
	for _, p := range paths {
		switch {
		case p.Over:
			lost = p
		case slices.Contains(p.Battles, "boss:won"):
			won = p
		}
	}
	if lost == nil || won == nil {
		t.Fatalf("a win path and a game over path: %d paths", len(paths))
	}
	if !slices.Contains(lost.Lines, "Back on your feet") || slices.Contains(lost.Lines, "after") {
		t.Errorf("the game over path runs the loss event and not the rest: %v", lost.Lines)
	}
	if !slices.Contains(won.Lines, "after") {
		t.Errorf("the win path goes on: %v", won.Lines)
	}
	if lost.Taken[len(lost.Taken)-1] != "game over" {
		t.Errorf("taken %v", lost.Taken)
	}
}

func TestExploreOptions(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": exploreExtra})
	st := testgame.State(db, 1)
	if ps := headless.Explore(db, st, nil, headless.Config{MaxPaths: -1}); len(ps) != 1 || len(ps[0].Errs) != 1 || !strings.Contains(ps[0].Errs[0].Error(), "negative") {
		t.Errorf("a negative limit is an error: %+v", ps)
	}
	if ps := headless.Explore(db, st, nil, headless.Config{MaxSteps: -1}); len(ps) != 1 || len(ps[0].Errs) != 1 {
		t.Errorf("a negative limit is an error: %+v", ps)
	}
	loop := headless.Explore(db, st, db.Events["forever"].Do, headless.Config{MaxSteps: 50})
	if len(loop) != 1 || len(loop[0].Errs) == 0 {
		t.Errorf("a call loop is reported: %+v", loop)
	}
	// scripted answers: Choose and Fight stop the branching and are recorded
	root := db.Events["scripted"].Do
	scripted := headless.Explore(db, st, root, headless.Config{
		Choose: func(*rpg.State, event.Choice) int { return 1 },
		Fight:  func(*rpg.State, event.Battle) int { return event.BattleWon },
	})
	if len(scripted) != 1 {
		t.Fatalf("scripted answers leave one path, got %d", len(scripted))
	}
	if got := scripted[0].Picks; !slices.Equal(got, []int{1, event.BattleWon}) {
		t.Errorf("picks %v", got)
	}
	// Outcomes narrows the results tried; Prepare changes the starting state; CheckEvery checks at every request
	checks := 0
	narrowed := headless.Explore(db, st, []data.Command{{Battle: "one"}}, headless.Config{
		Outcomes:   func(*rpg.State, event.Battle) []int { return []int{event.BattleLost} },
		Prepare:    func(st *rpg.State) { st.Gold = 777 },
		Check:      func(*rpg.State) error { checks++; return nil },
		CheckEvery: true,
	})
	if len(narrowed) != 1 || !slices.Contains(narrowed[0].Battles, "one:lost") || narrowed[0].St.Gold != 777 || checks < 2 {
		t.Errorf("paths %d battles %v checks %d", len(narrowed), narrowed[0].Battles, checks)
	}
	empty := headless.Explore(db, st, []data.Command{{Battle: "one"}}, headless.Config{Outcomes: func(*rpg.State, event.Battle) []int { return nil }})
	if len(empty) != 1 || len(empty[0].Errs) != 1 || !strings.Contains(empty[0].Errs[0].Error(), "no battle outcomes") {
		t.Errorf("no outcomes is an error: %+v", empty)
	}
	// a state that cannot be saved is reported rather than explored
	broken := testgame.State(db, 1)
	broken.DB = nil
	if ps := headless.Explore(db, broken, nil, headless.Config{}); len(ps) != 1 || len(ps[0].Errs) == 0 {
		t.Errorf("an unsavable state: %+v", ps)
	}
}

func TestExploreReportsMissingResources(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": exploreExtra})
	st := testgame.State(db, 1)
	cmds := []data.Command{
		{Say: &data.Say{Text: "hello", Face: "chars/missing.png"}},
		{Battle: "ghost"},
	}
	p := headless.Explore(db, st, cmds, headless.Config{})[0]
	joined := ""
	for _, e := range p.Errs {
		joined += e.Error() + "\n"
	}
	if !strings.Contains(joined, "chars/missing.png") || !strings.Contains(joined, "ghost") {
		t.Errorf("missing face and troop are reported: %v", p.Errs)
	}
}

// TestExploreChecksAPathThatEndsInGameOver: the state check runs at the end of every path, game over included
// (Config.Check: "called at the end of a path").
func TestExploreChecksAPathThatEndsInGameOver(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": exploreExtra})
	calls := 0
	paths := headless.Explore(db, testgame.State(db, 1), []data.Command{{Battle: "boss"}}, headless.Config{
		Outcomes: func(*rpg.State, event.Battle) []int { return []int{event.BattleLost} },
		Check:    func(*rpg.State) error { calls++; return errors.New("invariant broken") },
	})
	if len(paths) != 1 || !paths[0].Over {
		t.Fatalf("%+v", paths)
	}
	if calls == 0 || len(paths[0].Errs) != 1 || !strings.Contains(paths[0].Errs[0].Error(), "invariant broken") {
		t.Errorf("the check was not run at the end of a game over path: calls %d errs %v", calls, paths[0].Errs)
	}
}

func TestExploreLimitsPathsAndSaysHowManyAreLeft(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  wide:
    do:
      - choice: [{label: A, do: []}, {label: B, do: []}, {label: C, do: []}, {label: D, do: []}, {label: E, do: []}]
`})
	paths := headless.Explore(db, testgame.State(db, 1), db.Events["wide"].Do, headless.Config{MaxPaths: 2})
	if len(paths) != 2 || paths[1].Left != 3 {
		t.Errorf("%d paths, left %d (want 2 and 3)", len(paths), paths[len(paths)-1].Left)
	}
	all := headless.Explore(db, testgame.State(db, 1), db.Events["wide"].Do, headless.Config{})
	if len(all) != 5 || all[len(all)-1].Left != 0 {
		t.Errorf("all five options are tried: %d", len(all))
	}
}
