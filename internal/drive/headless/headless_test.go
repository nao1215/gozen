package headless_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/drive/headless"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/battle"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestMoveRevealsDungeonEntrance records the entrance of the destination in the exploration record, the same as the screen.
func TestMoveRevealsDungeonEntrance(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/entry.yaml": `
locations:
  passage:
    name: 通路
    dungeon:
      map: |
        #####
        #S.T#
        #####
      tiles:
        S: {start: true, facing: east}
        T: {name: 奥}
`})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{Move: "passage"}}); err != nil {
		t.Fatal(err)
	}
	if len(st.Mapped["passage"]) != 15 || st.Mapped["passage"][6] != 1 || st.Vars["explored"] != float64(33) {
		t.Fatalf("到着した入口が踏破記録にない 地図%v 探索率%v", st.Mapped, st.Vars["explored"])
	}
	if err := d.Walk('T'); err != nil {
		t.Fatal(err)
	}
	if st.Vars["explored"] != float64(100) {
		t.Fatalf("入口から奥まで歩いても踏破にならない %v", st.Vars["explored"])
	}
}

// AutoBattle also runs the commands of in-battle events (gozen sim and curve use the same path).
func TestAutoBattleRunsTroopEvents(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	b := battle.New(db, st, "boss") // set flag.saw_two on turn 2
	headless.AutoBattle(db, st, b, nil, 3, nil)
	if b.Turn < 2 {
		t.Fatalf("2 ターン目まで続かなかった（turn %d, %v）", b.Turn, b.Result)
	}
	if !st.Flag("saw_two") {
		t.Error("戦闘中イベントの set が実行されていない")
	}
}

// battle.after_win works in headless too, for both event battles (ending with end) and Fights.
func TestAfterWin(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "  after_win: aw\n",
		"data/z.yaml": `
events:
  aw:
    do: [{set: {var.aw: "var.aw + 1"}}]
  fight:
    do:
      - battle: one
      - end: true
`,
	})
	st := testgame.State(db, 1)
	for _, a := range st.Members() {
		a.Level = 30
		a.Recover()
	}
	d := headless.New(db, st)
	if err := d.Run(context.Background(), db.Events["fight"].Do); err != nil {
		t.Fatal(err)
	}
	if d.Wins != 1 || st.Vars["aw"] != 1.0 {
		t.Fatalf("イベントの戦闘の後: wins %d, var.aw %v", d.Wins, st.Vars["aw"])
	}
	st.Location = "field"
	if err := d.Fights(1); err != nil {
		t.Fatal(err)
	}
	if d.Wins != 2 || st.Vars["aw"] != 2.0 {
		t.Fatalf("Fights の後: wins %d, var.aw %v", d.Wins, st.Vars["aw"])
	}
}

// The portrait over a provisional background is recorded as the same display as the screen, for both direct runs and exhaustive path search.
func TestStandInKeepsFaces(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":                     testgame.Game + "\nstand_ins: cg/stand_in.txt\n",
		"assets/cg/stand_in.txt":        "# 仮背景\ncg/placeholder.png\n",
		"assets/cg/placeholder.png":     "仮背景",
		"assets/cg/placeholder.002.png": "仮背景の差分",
		"assets/cg/finished.png":        "完成画像",
		"data/images.yaml": `
events:
  images:
    do:
      - image: cg/placeholder.png
      - image: {files: [cg/placeholder.png, cg/placeholder.*.png], page: 2}
      - image: cg/finished.png
      - image: {file: cg/finished.png, faces: true}
      - image: ""
`,
	})
	want := []headless.Step{
		{Kind: "image", File: "cg/placeholder.png", Faces: true},
		{Kind: "image", File: "cg/placeholder.002.png", Faces: true},
		{Kind: "image", File: "cg/finished.png"},
		{Kind: "image", File: "cg/finished.png", Faces: true},
		{Kind: "image"},
	}
	cmds := db.Events["images"].Do
	t.Run("直接実行", func(t *testing.T) {
		d := headless.New(db, testgame.State(db, 1))
		d.Record = true
		var observed []headless.Step
		d.OnStep = func(s headless.Step) { observed = append(observed, s) }
		if err := d.Run(context.Background(), cmds); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Steps, want) || !reflect.DeepEqual(observed, want) {
			t.Fatalf("表示記録 %v、観測 %v、期待 %v", d.Steps, observed, want)
		}
	})
	t.Run("全経路探索", func(t *testing.T) {
		paths := headless.Explore(db, testgame.State(db, 1), cmds, headless.Config{})
		if len(paths) != 1 || len(paths[0].Errs) != 0 {
			t.Fatalf("探索結果が不正 %v", paths)
		}
		if !reflect.DeepEqual(paths[0].Steps, want) {
			t.Fatalf("表示記録 %v、期待 %v", paths[0].Steps, want)
		}
	})
}

func TestDefeatRecoveryPolicy(t *testing.T) {
	for _, recoverHP := range []bool{true, false} {
		for _, supplied := range []bool{true, false} {
			db := testgame.Load(t, map[string]string{"data/loss.yaml": `
events:
  loss:
    do:
      - battle: boss
        fighters: [guide]
        retry: false
        on_lose: [{set: {var.losses: "var.losses + 1", var.final_hp: "last_battle.hp.guide"}}]
`})
			st := testgame.State(db, 1)
			st.Join("guide")
			st.Actors["scout"].HP = 0 // An absent casualty must never be revived by a duel.
			st.Actors["guide"].HP = 0
			cmds := db.Events["loss"].Do
			cmds[0].Recover = &recoverHP
			d := headless.New(db, st)
			if supplied {
				d.Result = func(*rpg.State, string) (int, bool) { return event.BattleLost, true }
			}
			if err := d.Run(t.Context(), cmds); err != nil {
				t.Fatal(err)
			}
			want := 0
			if recoverHP {
				want = 1
			}
			if st.Actors["guide"].HP != want || st.Actors["scout"].HP != 0 {
				t.Fatalf("recover=%v supplied=%v: guide %d scout %d", recoverHP, supplied, st.Actors["guide"].HP, st.Actors["scout"].HP)
			}
			if st.LastBattle == nil || st.LastBattle.Result != "lose" || len(st.LastBattle.HP) != 1 || st.LastBattle.HP["guide"] != 0 || st.Vars["losses"] != float64(1) {
				t.Fatalf("outcome %+v vars %v", st.LastBattle, st.Vars)
			}
		}
	}
}

func TestAtlasInstantActions(t *testing.T) {
	db := testgame.LoadScenes(t, map[string]string{"data/menu.yaml": testgame.AtlasMenuData, "assets/maps/menu.png": string(testgame.Picture(64, 36))})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(t.Context(), []data.Command{{Move: "selection"}}); err != nil {
		t.Fatal(err)
	}
	steps := st.Steps
	for _, label := range []string{"Town", "Town", "Quarry"} {
		if err := d.Do(label); err != nil {
			t.Fatal(err)
		}
	}
	if st.Location != "selection" || st.Spot != "town" || st.Steps != steps || st.Vars["town"] != 2.0 || st.Vars["quarry"] != 1.0 || st.Vars["road"] != nil || d.Wins != 0 {
		t.Fatalf("instant actions traveled: location=%s spot=%s steps=%d vars=%v wins=%d", st.Location, st.Spot, st.Steps, st.Vars, d.Wins)
	}
	if err := d.Do("Tower"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "selection" {
		t.Fatal("locked entrance entered")
	}
	st.Flags["heard"] = true
	if err := d.Do("Tower"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "camp" || st.Steps != steps+1 {
		t.Fatal("explicit entrance did not move directly")
	}
	// once applies to actions even though selection never changes the saved spot.
	p := db.Locations["selection"].Map.Points["quarry"]
	p.Once = true
	if err := d.Run(t.Context(), []data.Command{{Move: "selection"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Do("Quarry"); err != nil {
		t.Fatal(err)
	}
	if d.Has("Quarry") {
		t.Fatal("completed once action remains available")
	}
}

func TestAtlasHiddenPointUnlocksAfterAction(t *testing.T) {
	world := strings.Replace(testgame.AtlasMenuData, `do: [{set: {var.town: "var.town + 1"}}]`, `do: [{set: {flag.heard: true}}]`, 1)
	db := testgame.LoadScenes(t, map[string]string{"data/menu.yaml": world, "assets/maps/menu.png": string(testgame.Picture(64, 36))})
	a := db.Locations["selection"].Map
	a.Points["tower"].Hide = "closed"
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(t.Context(), []data.Command{{Move: "selection"}}); err != nil {
		t.Fatal(err)
	}
	if d.Has("Tower") {
		t.Fatal("hidden point offered")
	}
	if err := d.Do("Tower"); err == nil {
		t.Fatal("hidden point selected")
	}
	if err := d.Do("Town"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "selection" || !d.Has("Tower") {
		t.Fatal("point did not open in place")
	}
	if err := d.Do("Tower"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "camp" {
		t.Fatal("open point did not activate")
	}
}

// TestResetActorMatchesTheSession: reset_actor leaves the same start state through the
// headless driver as through a play session (see TestResetActorInASession).
func TestResetActorMatchesTheSession(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	scout := st.Actors["scout"]
	scout.GainExp(500)
	scout.HP, scout.MP = 1, 0
	scout.States["poison"] = rpg.StateForever
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{ResetActor: "scout", Level: 3}}); err != nil {
		t.Fatal(err)
	}
	if scout.Level != 3 || scout.Exp != 90 || scout.HP != scout.Stat("mhp") || scout.MP != scout.Stat("mmp") || len(scout.States) != 0 || scout.Equip["weapon"] != "stick" {
		t.Errorf("level %d exp %d hp %d mp %d states %v equip %v", scout.Level, scout.Exp, scout.HP, scout.MP, scout.States, scout.Equip)
	}
	if err := d.Run(context.Background(), []data.Command{{ResetActor: "scout"}}); err != nil {
		t.Fatal(err)
	}
	if scout.Level != 1 || scout.Exp != 0 || scout.HP != 50 {
		t.Errorf("level %d exp %d hp %d, want the start of level 1", scout.Level, scout.Exp, scout.HP)
	}
}

func TestDriverRejectsUnsupportedRequests(t *testing.T) {
	db := testgame.Load(t, nil)
	for _, tc := range []struct {
		name    string
		command data.Command
	}{
		{"open", data.Command{Open: data.ScreenCall{Screen: "status"}}},
		{"checkpoint", data.Command{Checkpoint: "main"}},
		{"save", data.Command{Save: true}},
		{"phone", data.Command{Phone: &data.PhoneCue{App: "page", Var: "var.result", Rows: []data.PhoneRow{{Button: "Open"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := testgame.State(db, 7)
			d := headless.New(db, st)
			err := d.Run(t.Context(), []data.Command{tc.command, {SetV: data.Assignments{"flag.after": data.MustExpr("true")}}})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.name) {
				t.Fatalf("unsupported request succeeded: %v", err)
			}
			if st.Flags["after"] {
				t.Fatal("ran beyond unsupported request")
			}
		})
	}
	d := headless.New(db, testgame.State(db, 7))
	if err := d.Run(t.Context(), []data.Command{{Phone: &data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}}}, {Say: &data.Say{Text: "After"}}}); err != nil {
		t.Fatal(err)
	}
	if len(d.Transcript) != 1 || d.Transcript[0] != "After" {
		t.Fatal(d.Transcript)
	}
}

func TestDriverRejectsExplicitInvalidAnswers(t *testing.T) {
	db := testgame.Load(t, nil)
	cmds := []data.Command{{Prompt: "Pick a route", Choice: []data.ChoiceOption{
		{Label: "North", Do: []data.Command{{SetV: data.Assignments{"var.route": data.MustExpr("1")}}}},
		{Label: "South", Do: []data.Command{{SetV: data.Assignments{"var.route": data.MustExpr("2")}}}},
	}}}
	for _, n := range []int{-1, 2, 99} {
		for _, policy := range []bool{false, true} {
			st := testgame.State(db, 7)
			d := headless.New(db, st)
			if policy {
				d.Choose = func(*rpg.State, event.Choice) int { return n }
			} else {
				d.Answers = []int{n}
			}
			err := d.Run(t.Context(), cmds)
			if err == nil || !strings.Contains(err.Error(), "Pick a route") || !strings.Contains(err.Error(), "North") || !strings.Contains(err.Error(), "request") {
				t.Fatalf("invalid answer %d: %v", n, err)
			}
			if st.Vars["route"] != nil {
				t.Fatal("ran a substituted branch", st.Vars)
			}
		}
	}
	d := headless.New(db, testgame.State(db, 7))
	if err := d.Run(t.Context(), cmds); err != nil || d.St.Vars["route"] != float64(1) {
		t.Fatalf("default answer: %v", err)
	}
}

func TestDriverErrorsBelongToEachOperation(t *testing.T) {
	for _, name := range []string{"start", "do", "run", "go", "fight", "walk"} {
		t.Run(name, func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"data/walk.yaml": `locations:
  path:
    dungeon:
      map: "#####\n#S.T#\n#####"
      tiles:
        S: {start: true, facing: east}
        T: {name: Target, do: [{say: Arrived}]}
`})
			st := testgame.State(db, 7)
			d := headless.New(db, st)
			if err := d.Run(t.Context(), []data.Command{{Gold: data.MustExpr("'bad'")}}); err == nil {
				t.Fatal("fixture did not fail")
			}
			var err error
			switch name {
			case "start":
				err = d.Start()
			case "do":
				err = d.Do("Rest")
			case "run":
				err = d.Run(t.Context(), []data.Command{{Say: &data.Say{Text: "Ready"}}})
			case "go":
				err = d.GoTo("field")
			case "fight":
				st.Location = "field"
				err = d.Fights(1)
			case "walk":
				// The move itself also settles, so use a separate successful driver to place the state.
				setup := headless.New(db, st)
				if e := setup.Run(t.Context(), []data.Command{{Move: "path"}}); e != nil {
					t.Fatal(e)
				}
				err = d.Walk('T')
			}
			if err != nil {
				t.Fatalf("successful %s repeated an old error: %v", name, err)
			}
		})
	}
}

const driverData = `
music:
  songs: {theme: {file: x.png}}
enemies:
  wall:
    name: Wall
    image: x.png
    stats: {mhp: 99999, mmp: 0, atk: 1, def: 99999, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 600, mmp: 0, atk: 400, def: 30, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack}]
troops:
  wall: {members: [wall]}
  brute: {members: [brute], can_escape: false}
  chatty:
    members: [wall]
    events:
      - when: "turn == 1"
        do:
          - choice: [{label: A, do: [{set: {var.chose: 1}}]}, {label: B, do: []}]
          - battle: one
          - mood: calm
          - sfx: ding
          - effect: flash
          - music: {song: theme}
          - set: {var.talked: 1}
locations:
  crypt:
    name: Crypt
    dungeon:
      map: |
        #######
        #S..G.#
        #######
      tiles:
        S: {start: true, facing: east}
        G: {name: Goal, do: [{set: {flag.goal: true}}]}
  pit:
    name: Pit
    encounters: {rate: "1", troops: [{troop: brute}]}
    exits: [{to: camp}]
events:
  opening: {do: [{say: Welcome}, {set: {flag.opened: true}}]}
  wake: {trigger: auto, once: true, when: "flag.go", do: [{set: {flag.woke: true}}]}
`

func TestStartRunsTheStartEvent(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": strings.Replace(testgame.Game, "start: {", "start: {event: opening, ", 1), "data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if !st.Flags["opened"] || len(d.Transcript) == 0 || d.Transcript[0] != "Welcome" {
		t.Errorf("flags %v transcript %v", st.Flags, d.Transcript)
	}
}

func TestAutoEventsRunWhenTheyHold(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	st.Flags["go"] = true
	if err := d.Run(context.Background(), db.Events["opening"].Do); err != nil {
		t.Fatal(err)
	}
	if !st.Flags["woke"] {
		t.Error("an auto event whose when holds ran after the commands")
	}
}

func TestRunNeedsAContextAndStopsWhenItIsDone(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(nil, db.Events["opening"].Do); err == nil { //nolint:staticcheck // A nil context is the case under test.
		t.Error("a nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Run(ctx, db.Events["opening"].Do); err == nil {
		t.Error("a cancelled context")
	}
	if st.Flags["opened"] {
		t.Error("a cancelled run changed the state")
	}
}

// TestBattleResults: a battle that cannot be finished within MaxTurns is a loss, a lost battle raises the party to 1 HP,
// and an escape is its own result.
func TestBattleResults(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	d.MaxTurns = 3
	if got := d.Fight("wall"); got != event.BattleLost || d.Losses != 1 || d.Wins != 0 {
		t.Errorf("a stalemate is a loss: result %d losses %d", got, d.Losses)
	}
	d.MaxTurns = 80
	for _, a := range st.Members() {
		a.HP = 0
	}
	if got := d.Fight("brute"); got != event.BattleLost {
		t.Errorf("result %d", got)
	}
	for _, a := range st.Members() {
		if a.HP < 1 {
			t.Errorf("%s was not raised after the loss", a.Name())
		}
	}
	if d.Battles != 2 {
		t.Errorf("battles %d", d.Battles)
	}
	if !strings.Contains(strings.Join(d.Transcript, "\n"), "[battle brute: lose in") {
		t.Errorf("the transcript says how it ended: %v", d.Transcript)
	}
}

func TestFightsAndGoToStopAtGameOver(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.GoTo("pit"); err == nil {
		t.Log("no way to the pit from the camp")
	}
	st.Location = "pit"
	err := d.Fights(3)
	if err == nil && !d.Over {
		t.Fatal("losing a battle that cannot be lost ends the game")
	}
	if err := d.Fights(1); err == nil || !strings.Contains(err.Error(), "game over") {
		t.Errorf("after the game is over: %v", err)
	}
	if err := d.GoTo("camp"); err == nil || !strings.Contains(err.Error(), "game over") {
		t.Errorf("walking after the game is over: %v", err)
	}
	// Restore starts again
	fresh := testgame.State(db, 2)
	d.Restore(fresh)
	if d.Over || d.St != fresh {
		t.Error("Restore forgets the game over and takes the new state")
	}
	if err := d.Run(context.Background(), db.Events["opening"].Do); err != nil || !fresh.Flags["opened"] {
		t.Errorf("a restored driver runs on the new state: %v", err)
	}
}

func TestWalkInADungeon(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{Move: "crypt"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Walk('Z'); err == nil || !strings.Contains(err.Error(), `no way to 'Z'`) {
		t.Errorf("a mark that is not there: %v", err)
	}
	if err := d.Walk('G'); err != nil || !st.Flags["goal"] {
		t.Errorf("walking to the goal runs its do: %v", err)
	}
	if err := d.Walk('G'); err != nil {
		t.Errorf("already there: %v", err)
	}
	if err := d.Walk('S'); err != nil {
		t.Errorf("back to the start: %v", err)
	}
	st.Location = "camp"
	if err := d.Walk('G'); err == nil {
		t.Error("there is nothing to walk to outside a dungeon")
	}
}

// TestBattleEventsAreAnsweredQuietly: commands in a troop's battle events run without a screen:
// the first option is taken, a battle inside a battle is won, and effects are ignored.
func TestBattleEventsAreAnsweredQuietly(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "chatty")
	headless.AutoBattle(db, st, b, nil, 2, nil)
	if st.Vars["chose"] != 1.0 || st.Vars["talked"] != 1.0 {
		t.Errorf("the battle event ran to its end: %v", st.Vars)
	}
	var seen int
	b2 := battle.New(db, st, "chatty")
	headless.AutoBattle(db, st, b2, nil, 2, func(steps []battle.Step) { seen += len(steps) })
	if seen == 0 {
		t.Error("the steps are handed to the observer")
	}
	if b2.Result != battle.Ongoing {
		t.Errorf("a battle cut off by the turn limit is left ongoing for the caller, got %v", b2.Result)
	}
}
