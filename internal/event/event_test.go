package event_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

type host struct{ moved, image, mood string }

func (h *host) Move(l string)       { h.moved = l }
func (h *host) Music(data.MusicCue) {}
func (h *host) Mood(m string)       { h.mood = m }
func (h *host) Image(c data.CG)     { h.image = c.File }
func (h *host) SFX(string)          {}
func (h *host) Effect(string)       {}

const script = `
events:
  main:
    do:
      - say: {who: scout, face: happy, text: "I have {gold}G"}
      - choice:
          - label: Yes
            do: [{set: {flag.yes: true, var.n: "var.n + 2"}}]
          - label: Hidden
            when: "flag.never"
            do: []
          - label: No
            do: [{set: {flag.no: true}}]
        prompt: "Well?"
      - if: "flag.yes"
        then: [{give: {item: key}}, {gold: "-30"}]
        else: [{gold: "1000"}]
      - call: sub
      - battle: one
        on_win: [{set: {flag.won: true}}]
        on_lose: [{set: {flag.lost: true}}]
      - join: guide
      - move: field
      - image: cg.png
      - mood: calm
      - end: true
      - say: "never shown"
`

func TestRunner(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": script, "assets/cg.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)

	say, ok := r.Next().(event.Say)
	if !ok || say.Name != "Scout" || say.Text != "I have 100G" || say.Face != "" {
		t.Fatalf("say = %+v", say)
	}
	ch, ok := r.Next().(event.Choice)
	if !ok || len(ch.Options) != 2 || ch.Options[1] != "No" || ch.Prompt != "Well?" {
		t.Fatalf("choice = %+v (the hidden option must not show)", ch)
	}
	r.Answer(0)
	bt, ok := r.Next().(event.Battle)
	if !ok || bt.Troop != "one" || !bt.CanLose {
		t.Fatalf("battle = %+v", bt)
	}
	r.Answer(event.BattleWon)
	join, ok := r.Next().(event.Say)
	if !ok || join.Text == "" {
		t.Fatalf("no note for the join: %+v", join)
	}
	if req := r.Next(); req != nil {
		t.Fatalf("after end: %+v", req)
	}
	if !st.Flag("yes") || st.Vars["n"] != 2.0 || st.Items["key"] != 1 || st.Gold != 75 || !st.Flag("won") {
		t.Errorf("state: flags %v vars %v items %v gold %d", st.Flags, st.Vars, st.Items, st.Gold)
	}
	if len(st.Party) != 2 || h.moved != "field" || h.image != "cg.png" || h.mood != "calm" {
		t.Errorf("party %v host %+v", st.Party, h)
	}
	if r.Running() {
		t.Error("still running")
	}
}

func TestLosingWithoutOnLoseIsGameOver(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e: {do: [{battle: one}, {say: after}]}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Battle); !ok {
		t.Fatal("no battle")
	}
	r.Answer(event.BattleLost)
	if _, ok := r.Next().(event.GameOver); !ok {
		t.Fatal("no game over")
	}
	if r.Running() {
		t.Error("the event went on after the game over")
	}
}

// files shows the pictures one after another, a tap between them; a pattern takes the
// files it matches in name order, and the last picture stays for the line after it.
func TestImageFiles(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/a.png, cg/night_*.png], fade: 1}
      - say: "after"
`, "assets/cg/a.png": "x", "assets/cg/night_2.png": "x", "assets/cg/night_1.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)
	for _, want := range []string{"cg/a.png", "cg/night_1.png"} {
		if _, ok := r.Next().(event.Tap); !ok || h.image != want {
			t.Fatalf("want a tap on %s, shown %s", want, h.image)
		}
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "after" || h.image != "cg/night_2.png" {
		t.Fatalf("after the pictures: %+v, shown %s", say, h.image)
	}
}

// page shows just one stage of files and moves to the next statement without waiting (written per stage between passages of text).
// A missing stage stays as the last picture.
func TestImagePage(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/x.png, cg/x.*.png], page: 1}
      - say: one
      - image: {files: [cg/x.png, cg/x.*.png], page: 2}
      - say: two
      - image: {files: [cg/x.png, cg/x.*.png], page: 3}
      - say: three
`, "assets/cg/x.png": "x", "assets/cg/x.002.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)
	for _, c := range []struct{ text, image string }{{"one", "cg/x.png"}, {"two", "cg/x.002.png"}, {"three", "cg/x.002.png"}} {
		if say, ok := r.Next().(event.Say); !ok || say.Text != c.text || h.image != c.image {
			t.Fatalf("want %q over %s, got %+v over %s", c.text, c.image, say, h.image)
		}
	}
}

func TestImageFilesMatchingNothingIsAnError(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/none_*.png]}
`})
	if err == nil {
		t.Fatal("a pattern matching no picture must be reported")
	}
}

// hurt reduces HP outside battle (1 remains). affect applies or cures a state.
func TestHurtAndAffect(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - hurt: {who: all, amount: "self.mhp * 0.5"}
      - affect: {who: all, add: poison}
  more:
    do:
      - hurt: {who: all, amount: "99999"}
      - affect: {who: all, remove: bad}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	a := st.Members()[0]
	full := a.HP
	r.Start(db.Events["main"].Do)
	for r.Next() != nil {
	}
	if a.HP != full-full/2 || len(a.States) != 1 {
		t.Fatalf("after main: hp %d of %d, states %v", a.HP, full, a.States)
	}
	r.Start(db.Events["more"].Do)
	for r.Next() != nil {
	}
	if a.HP != 1 || len(a.States) != 0 {
		t.Fatalf("after more: hp %d (want 1), states %v", a.HP, a.States)
	}
}

// TestAffectForever checks that a state with no expiry (turns: 0) applied by affect is held as
// "no expiry", the same as a grant during battle, and appears in the state list.
func TestAffectForever(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - affect: {who: all, add: poison}
      - affect: {who: all, add: sleep, turns: 3}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	for r.Next() != nil {
	}
	a := st.Members()[0]
	if a.States["poison"] != rpg.StateForever {
		t.Errorf("poison = %d, want %d (期限なし)", a.States["poison"], rpg.StateForever)
	}
	if a.States["sleep"] != 3 {
		t.Errorf("sleep = %d, want 3", a.States["sleep"])
	}
}

// TestLaterSurvivesEnd checks that after_win scheduled by Won is not discarded by the original event's end and runs later.
func TestLaterSurvivesEnd(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "  after_win: aw\n",
		"data/z.yaml": `
events:
  aw:
    do: [{say: "after"}]
  e:
    do:
      - battle: one
      - end: true
      - say: "never"
`,
	})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Battle); !ok {
		t.Fatal("no battle")
	}
	r.Answer(event.BattleWon)
	r.Won()
	say, ok := r.Next().(event.Say)
	if !ok || say.Text != "after" {
		t.Fatalf("after_win が走らない: %+v", say)
	}
	if req := r.Next(); req != nil || r.Running() {
		t.Fatalf("残り: %+v", req)
	}
}

// TestSayInstead checks that say's instead (the replacement line when the speaker is down in battle) is held in the same way as the original line.
func TestSayInstead(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - say: {who: scout, text: "I have {gold}G", instead: [{who: guide, text: "Scout has {gold}G", side: right}, {who: guide, text: ""}]}
`})
	r := event.New(db, testgame.State(db, 1), &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	say, ok := r.Next().(event.Say)
	if !ok || say.Actor != "scout" || len(say.Instead) != 1 {
		t.Fatalf("say = %+v (the empty instead must be dropped)", say)
	}
	if alt := say.Instead[0]; alt.Actor != "guide" || alt.Name != "Guide" || alt.Text != "Scout has 100G" || !alt.Right {
		t.Errorf("instead = %+v", alt)
	}
}

// TestNotice checks that notice returns Notice rather than the conversation window (Say), and moves on without waiting for an answer.
func TestNotice(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - set: {var.n: 3}
      - notice: "{var.n}G落とした"
      - say: "next"
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	n, ok := r.Next().(event.Notice)
	if !ok || n.Text != "3G落とした" {
		t.Fatalf("notice が返らない: %+v", n)
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" {
		t.Fatalf("notice の後が進まない: %+v", say)
	}
}

func TestOptionalImageFilesMayMatchNothing(t *testing.T) {
	if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/none_*.png], optional: true}
`}); err != nil {
		t.Fatal(err)
	}
}

func (h *host) Handle(e event.Effect) {
	switch e := e.(type) {
	case event.MoveEffect:
		h.Move(e.Location)
	case event.MusicEffect:
		h.Music(e.Cue)
	case event.MoodEffect:
		h.Mood(e.Name)
	case event.ImageEffect:
		h.Image(e.Picture)
	case event.SoundEffect:
		h.SFX(e.Name)
	case event.VisualEffect:
		h.Effect(e.Name)
	}
}

func TestCaptureAfterVictoryIsAChoice(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	for _, answer := range []int{0, 1} {
		st := testgame.State(db, 1)
		r := event.New(db, st, &host{})
		r.Log = func(err error) { t.Error(err) }
		r.Start([]data.Command{{Battle: "one"}})
		if _, ok := r.Next().(event.Battle); !ok {
			t.Fatal("no battle request")
		}
		r.Answer(event.BattleWon)
		line, ok := r.Next().(event.Say)
		if !ok || !strings.Contains(line.Text, "rises") || line.Recruit == nil || line.Recruit.Image == "" {
			t.Fatalf("no staged rising line: %+v", line)
		}
		choice, ok := r.Next().(event.Choice)
		if !ok || len(choice.Options) != 2 || choice.Options[0] != "Yes" || choice.Recruit == nil || choice.Recruit.Sex != line.Recruit.Sex {
			t.Fatalf("join request = %+v", choice)
		}
		if !strings.Contains(choice.Prompt, "♂") && !strings.Contains(choice.Prompt, "♀") {
			t.Fatalf("the offer has no sex sign: %q", choice.Prompt)
		}
		r.Answer(answer)
		for r.Running() {
			r.Next()
		}
		want := float64(1 - answer)
		if got := st.Vars["bestiary.mote.caught"]; answer == 0 && got != want || answer == 1 && got != nil {
			t.Fatalf("answer %d caught %v", answer, got)
		}
		if st.Vars["bestiary.mote.defeated"] != float64(1) {
			t.Fatalf("defeated = %v", st.Vars)
		}
		if answer == 0 {
			if got := st.Actors["monster_1"]; got == nil || got.Sex != choice.Recruit.Sex {
				t.Fatalf("joined monster sex differs from invitation: %+v", got)
			}
		}
	}
}

func TestRecruitOfferDoesNotShiftBattleRandomStream(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	plainDB := data.Fork(db)
	plainDB.Enemies["mote"].Recruit = nil
	withOffer, plain := testgame.State(db, 8), testgame.State(plainDB, 8)
	for _, tc := range []struct {
		game *data.DB
		st   *rpg.State
	}{
		{db, withOffer}, {plainDB, plain},
	} {
		r := event.New(tc.game, tc.st, &host{})
		r.Start([]data.Command{{Battle: "one"}})
		if _, ok := r.Next().(event.Battle); !ok {
			t.Fatal("no battle")
		}
		r.Answer(event.BattleWon)
		for r.Running() {
			if _, ok := r.Next().(event.Choice); ok {
				r.Answer(1) // Decline the optional recruit.
			}
		}
	}
	if got, want := withOffer.Rand.Uint64(), plain.Rand.Uint64(); got != want {
		t.Fatalf("adding a recruit offer shifted the next unrelated draw: %d != %d", got, want)
	}
}

func TestCaptureResultNamesReserveAndFirstCompanion(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 1)
	db.Game.MaxParty = len(st.Party)
	r := event.New(db, st, &host{})
	r.Start([]data.Command{{Capture: "mote"}})
	r.Next() // The saved-instance note precedes the staged result line.
	line, ok := r.Next().(event.Say)
	if !ok || !strings.Contains(line.Text, "joined your reserve") || !strings.Contains(line.Text, "First companion of this species") {
		t.Fatalf("reserve result = %+v", line)
	}
	if len(st.ReserveIDs()) != 1 {
		t.Fatalf("reserve = %v", st.ReserveIDs())
	}
	r.Start([]data.Command{{Capture: "mote"}})
	r.Next()
	line, ok = r.Next().(event.Say)
	if !ok || strings.Contains(line.Text, "First companion of this species") {
		t.Fatalf("repeat result = %+v", line)
	}
}

func TestStrictRecruitmentAdmission(t *testing.T) {
	for _, source := range []string{"capture", "battle"} {
		t.Run(source, func(t *testing.T) {
			files := testgame.MonsterFiles()
			for name, body := range files {
				files[name] = strings.Replace(body, "recruit: {chance: 1,", "recruit: {admission: party_only, chance: 1,", 1)
			}
			db := testgame.Load(t, files)
			st := testgame.State(db, 1)
			db.Game.MaxParty = len(st.Party) + 1
			if source == "battle" {
				db.Troops["one"].Members = []string{"mote", "mote"}
			}
			r := event.New(db, st, &host{})
			if source == "battle" {
				r.Start([]data.Command{{Battle: "one"}})
				if _, ok := r.Next().(event.Battle); !ok {
					t.Fatal("missing battle")
				}
				r.Answer(event.BattleWon)
			} else {
				r.Start([]data.Command{{Capture: "mote"}, {Capture: "mote"}})
			}
			rejected := false
			for steps := 0; r.Running() && steps < 100; steps++ {
				switch request := r.Next().(type) {
				case event.Choice:
					r.Answer(0)
				case event.Say:
					rejected = rejected || strings.Contains(request.Text, "party is full")
				}
			}
			if r.Running() {
				t.Fatal("runner did not finish")
			}
			if !rejected || st.Vars["capture_result"] != "party_full" || st.Vars["bestiary.mote.caught"] != float64(1) || st.Vars["monster.next"] != float64(1) || len(st.ReserveIDs()) != 0 {
				t.Fatalf("rejected=%v party=%v reserve=%v vars=%v", rejected, st.Party, st.ReserveIDs(), st.Vars)
			}
			// The shared state mutation rejects a full party before allocating or drawing sex.
			before, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := st.Recruit("mote", 1, 0, nil); ok {
				t.Fatal("state admitted a full-party recruit")
			}
			after, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("rejected recruitment changed saved state")
			}
		})
	}
}

func TestStrictRecruitmentRechecksPendingOffer(t *testing.T) {
	files := testgame.MonsterFiles()
	for name, body := range files {
		files[name] = strings.Replace(body, "recruit: {chance: 1,", "recruit: {admission: party_only, chance: 1,", 1)
	}
	db := testgame.Load(t, files)
	st := testgame.State(db, 1)
	db.Game.MaxParty = len(st.Party) + 1
	r := event.New(db, st, &host{})
	r.Start([]data.Command{{Battle: "one"}})
	r.Next()
	r.Answer(event.BattleWon)
	r.Next()
	if _, ok := r.Next().(event.Choice); !ok {
		t.Fatal("missing pending offer")
	}
	if !st.JoinStory("guide") {
		t.Fatal("could not fill last slot")
	}
	r.Answer(0)
	for r.Running() {
		r.Next()
	}
	if st.Vars["capture_result"] != "party_full" || st.Vars["monster.next"] != nil || st.Vars["bestiary.mote.caught"] != nil || len(st.ReserveIDs()) != 0 {
		t.Fatalf("pending offer admitted after party filled: %v", st.Vars)
	}
}

func TestStrictRecruitmentDoesNotBlockBreeding(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 1)
	db.Game.MaxParty = 2
	a, _ := st.RecruitWithSex("mote", 2, 0, nil, "male")
	b, _ := st.RecruitWithSex("mote", 2, 0, nil, "female")
	db.Enemies["boss"].Recruit = &data.Recruit{Admission: "party_only"}
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start([]data.Command{{Breed: &data.BreedCmd{Parents: []string{a, b}}}})
	for r.Running() {
		r.Next()
	}
	if st.Actors[a] != nil || st.Actors[b] != nil || st.Actors["monster_3"] == nil || st.Party[1] != "monster_3" {
		t.Fatalf("breeding lost its parent slot: %v", st.Party)
	}
}

// A won battle counts in the monster book and offers to recruit the foes the battle beat
// (its last_kill counts), not every member the troop began with: one of two motes fled.
func TestWonBattleCountsTheFoesBeaten(t *testing.T) {
	files := testgame.MonsterFiles()
	files["data/z.yaml"] = "events:\n  fight: {do: [{battle: two}]}\n"
	db := testgame.Load(t, files)
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["fight"].Do)
	if _, ok := r.Next().(event.Battle); !ok {
		t.Fatal("no battle")
	}
	// what a battle that beat one mote and saw the other flee leaves behind
	st.RecordBattle("won", st.Members())
	st.Vars["last_kill_mote"] = 1.0
	r.Answer(event.BattleWon)
	if n := st.Vars["bestiary.mote.defeated"]; n != 1.0 {
		t.Errorf("the book counts %v motes defeated, want 1", n)
	}
	offers := 0
	for req := r.Next(); req != nil; req = r.Next() {
		if s, ok := req.(event.Say); ok && strings.Contains(s.Text, "rises") {
			offers++
		}
		if c, ok := req.(event.Choice); ok {
			r.Answer(len(c.Options) - 1)
		}
	}
	if offers != 1 {
		t.Errorf("%d recruit offers, want 1", offers)
	}
}

func TestSayCarriesEnsembleWithoutChangingText(t *testing.T) {
	db := testgame.Load(t, nil)
	r := event.New(db, testgame.State(db, 1), &host{})
	r.Start([]data.Command{{Say: &data.Say{Who: data.IDRef{ID: "scout"}, Text: "Welcome, everyone.", Ensemble: "gathering"}}})
	got, ok := r.Next().(event.Say)
	if !ok || got.Ensemble != "gathering" || got.Actor != "scout" || got.Text != "Welcome, everyone." {
		t.Fatalf("line lost presentation or identity: %+v", got)
	}
}

func TestInvalidEventBindingLeavesStateAndRNG(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: bindings\n" + testgame.Game, "data/bindings.yaml": `events:
  add:
    params: {amount: number}
    do: [{gold: arg.amount}]
`})
	for _, value := range []string{"true", "nil", "pow(10, 309)", "rand(10)"} {
		t.Run(value, func(t *testing.T) {
			st, control := testgame.State(db, 17), testgame.State(db, 17)
			before, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			runner := event.New(db, st, &host{})
			var problems []error
			runner.Log = func(err error) { problems = append(problems, err) }
			runner.Start([]data.Command{{Call: data.EventCall{Event: "add", Args: map[string]data.Expr{"amount": data.MustExpr(value)}}}})
			if q := runner.Next(); q != nil {
				t.Fatalf("invalid call produced %v", q)
			}
			after, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			if len(problems) != 1 || !bytes.Equal(before, after) {
				t.Fatalf("invalid binding changed state: %v", problems)
			}
			if st.Rand.Uint64() != control.Rand.Uint64() {
				t.Fatal("invalid binding consumed RNG")
			}
			runner.Start([]data.Command{{Call: data.EventCall{Event: "add", Args: map[string]data.Expr{"amount": data.MustExpr("2")}}}})
			runner.Next()
			if st.Gold != control.Gold+2 {
				t.Fatal("runner unusable after rejection")
			}
		})
	}
}

func TestPlanningRuleShadowsEventArguments(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": "schema_version: 10\nid: bindings\n" + testgame.Game + `planning:
  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [day]}
  attributes: {vigor: {type: number, initial: 0}}
  entities: {potter: {name: Potter}}
  rules:
    boost:
      params: [amount]
      do: [{op: set, entity: potter, attribute: vigor, value: arg.amount}]
`,
		"data/bindings.yaml": `events:
  encourage:
    params: {amount: number}
    do:
    - plan: {op: rule, rule: boost, args: {amount: 'arg.amount + 1'}}
    - say: '{arg.amount}/{entity.potter.vigor}'
`,
	})
	st := testgame.State(db, 5)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	args := map[string]any{"amount": 2.0}
	if err := r.StartBound("encourage", args); err != nil {
		t.Fatal(err)
	}
	args["amount"] = 99.0
	line, ok := r.Next().(event.Say)
	if !ok || line.Text != "2/3" {
		t.Fatalf("rule leaked or root retained input map: %+v", line)
	}
}

func TestScreenBindingFailureAndOwnership(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": "schema_version: 10\nid: screen-bindings\n" + testgame.Game + `screens:
  stock:
    params: {amount: number}
    title: Stock
    entries:
    - label: Take
      do:
      - say: '{arg.amount}'
      - set: {var.total: 'arg.amount'}
`,
		"data/bindings.yaml": `events:
  open_stock:
    do:
    - open: {screen: stock, args: {amount: 'var.amount'}}
    - say: Returned.
`,
	})
	st := testgame.State(db, 5)
	control := testgame.State(db, 5)
	r := event.New(db, st, &host{})
	var problems []error
	r.Log = func(err error) { problems = append(problems, err) }
	st.Vars["amount"] = "invalid"
	r.Start(db.Events["open_stock"].Do)
	line, ok := r.Next().(event.Say)
	if !ok || line.Text != "Returned." || len(problems) != 1 || st.Vars["total"] != nil {
		t.Fatalf("invalid open changed state or ran a screen: %+v, %v", line, problems)
	}
	if st.Rand.IntN(1000) != control.Rand.IntN(1000) {
		t.Fatal("invalid open consumed RNG")
	}
	r.Stop()
	st.Vars["amount"] = float64(2)
	r.Start(db.Events["open_stock"].Do)
	opened, ok := r.Next().(event.Open)
	if !ok {
		t.Fatal("missing open")
	}
	st.Vars["amount"] = float64(99)
	r.Stop()
	if err := r.StartBound("screen:stock:0", opened.Args); err != nil {
		t.Fatal(err)
	}
	opened.Args["amount"] = float64(88)
	line, ok = r.Next().(event.Say)
	if !ok || line.Text != "2" {
		t.Fatalf("entry replay lost frozen args: %+v", line)
	}
	if q := r.Next(); q != nil || st.Vars["total"] != float64(2) {
		t.Fatalf("entry result: %v, %v", q, st.Vars)
	}
	for _, name := range []string{"screen:stock:1", "screen:stock:00", "screen:missing:0"} {
		if err := r.StartBound(name, map[string]any{"amount": float64(3)}); err == nil || r.Running() {
			t.Fatalf("accepted invalid root %s", name)
		}
	}
}

func TestReplayTo(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/replay.yaml": `events:
  receipt:
    do:
    - choice:
      - label: Collect
        do: [{set: {var.boards: 2}}]
      - label: Cancel
    - say: Receipt.
    - set: {flag.finished: true}
`})
	for _, tc := range []struct {
		name    string
		line    int
		answers []int
		valid   bool
	}{
		{"preview", 1, nil, true},
		{"answered", 1, []int{0}, true},
		{"receipt", 2, []int{0}, true},
		{"cancelled", 2, []int{1}, true},
		{"missing", 2, nil, false},
		{"extra", 2, []int{0, 1}, false},
		{"out of range", 2, []int{2}, false},
		{"unreachable", 3, []int{0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := testgame.State(db, 7)
			r := event.New(db, st, &host{})
			r.Start(db.Events["receipt"].Do)
			q, err := r.ReplayTo(tc.line, tc.answers)
			if (err == nil) != tc.valid {
				t.Fatalf("request %v, error %v", q, err)
			}
			if !tc.valid {
				return
			}
			if st.Flags["finished"] {
				t.Fatal("replay ran beyond the receipt")
			}
			if tc.name == "answered" {
				if q != nil {
					t.Fatal("answered choice displayed again")
				}
				q = r.Next()
			}
			if tc.name != "preview" {
				if say, ok := q.(event.Say); !ok || say.Text != "Receipt." {
					t.Fatal(q)
				}
				var want any = float64(2)
				if tc.name == "cancelled" {
					want = nil
				}
				if st.Vars["boards"] != want {
					t.Fatal(st.Vars)
				}
			}
		})
	}
	for _, command := range []data.Command{{Move: "camp"}, {Open: data.ScreenCall{Screen: "status"}}, {Save: true}} {
		st := testgame.State(db, 7)
		r := event.New(db, st, &host{})
		r.Start(append([]data.Command{command}, db.Events["receipt"].Do...))
		if _, err := r.ReplayTo(1, nil); err == nil {
			t.Fatal("accepted boundary", command)
		}
	}
}

func TestScreenEntriesVisibilityAndOwnership(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Screens = map[string]*data.AuthorScreen{"stock": {Entries: []data.ScreenEntry{
		{Label: "Hidden", When: data.Expr{Expr: testgame.Expr("false")}},
		{Label: "Broken", When: data.Expr{Expr: testgame.Expr("missing()")}},
		{Label: "Stock {amount}"},
		{Label: "After {amount}", When: data.Expr{Expr: testgame.Expr("amount == 3")}},
	}}}
	env := expr.Map{"amount": float64(2)}
	var problems []error
	report := func(err error) { problems = append(problems, err) }
	var labels []string
	for index, entry := range event.ScreenEntries(db, "stock", env, report) {
		labels = append(labels, entry.Label)
		if index == 2 {
			// Frontend detail evaluation must finish before the next guard is read.
			env["amount"] = float64(3)
			entry.Label = "Caller copy"
		}
	}
	if len(problems) != 1 || strings.Join(labels, "/") != "Stock 2/After 3" || db.Game.Screens["stock"].Entries[2].Label != "Stock {amount}" {
		t.Fatalf("errors=%v labels=%v definition=%v", problems, labels, db.Game.Screens["stock"])
	}
	for _, name := range []string{"missing", "stock"} {
		count := 0
		for range event.ScreenEntries(db, name, env, nil) {
			count++
			break
		}
		if name == "missing" && count != 0 || name == "stock" && count != 1 {
			t.Fatal("missing screen or stopped iteration")
		}
	}
}

func TestIndexedVariableAssignment(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: counters\n" + testgame.Game, "data/counters.yaml": `events:
  count:
    params: {resident: string, day: number}
    do:
    - set: {'var["stock_" + arg.resident + "_" + arg.day]': 'var["stock_" + arg.resident + "_" + arg.day] + 2'}
    - set: {'flag["visited_" + arg.resident]': '!flag["visited_" + arg.resident]'}
  visit:
    do:
    - call: {event: count, args: {resident: '"potter"', day: '1'}}
    - call: {event: count, args: {resident: '"weaver"', day: '2'}}
    - call: {event: count, args: {resident: '"potter"', day: '1'}}
    - set: {var.key: '"stock_next"', 'var[var.key]': '5'}
`})
	st := testgame.State(db, 17)
	runner := event.New(db, st, &host{})
	runner.Log = func(err error) { t.Error(err) }
	runner.Start(db.Events["visit"].Do)
	runner.Next()
	if st.Vars["stock_potter_1"] != 4.0 || st.Vars["stock_weaver_2"] != 2.0 || st.Vars["stock_next"] != 5.0 {
		t.Fatalf("counters: %v", st.Vars)
	}
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, saved, testgame.State(db, 17).Rand)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Flags["visited_potter"] || !loaded.Flags["visited_weaver"] {
		t.Fatalf("loaded visit flags: %v", loaded.Flags)
	}
	if loaded.Vars["stock_potter_1"] != 4.0 || loaded.Vars["stock_weaver_2"] != 2.0 {
		t.Fatalf("loaded counters: %v", loaded.Vars)
	}
}

func TestInvalidIndexedTargetPreservesStateAndRandomness(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: counters\n" + testgame.Game})
	for _, target := range []string{`var[true]`, `var[""]`, `var["nested.key"]`, `var["bad key"]`, `var[rand(10)]`, `var["x"`, `var[nil]`, `flag[true]`, `flag[""]`, `flag["nested.key"]`, `flag[rand(10)]`, `flag["x"`, `flag[nil]`} {
		t.Run(target, func(t *testing.T) {
			st, control := testgame.State(db, 17), testgame.State(db, 17)
			before, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			runner := event.New(db, st, &host{})
			var problems []error
			runner.Log = func(err error) { problems = append(problems, err) }
			runner.Start([]data.Command{{SetV: data.Assignments{target: data.MustExpr("rand(100)")}}})
			runner.Next()
			after, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			if len(problems) != 1 || !bytes.Equal(before, after) {
				t.Fatalf("invalid target changed state: %v", problems)
			}
			if st.Rand.Uint64() != control.Rand.Uint64() {
				t.Fatal("invalid target consumed randomness")
			}
		})
	}
}

func TestRepeatedCallKeepsOrderAndEvaluatesArgumentsAtEachCall(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: bindings\n" + testgame.Game, "data/bindings.yaml": `events:
  visit:
    params: {worker: string, amount: number, note: string}
    do:
    - set: {var.total: arg.amount}
    - say: '{arg.note} {arg.worker}: {arg.amount}'
  tour:
    params: {prefix: string}
    do:
    - set: {var.total: '0'}
    - call:
        event: visit
        each: {param: worker, items: [second, first, second]}
        args: {amount: 'var.total+1', note: arg.prefix}
    - say: '{arg.prefix} Finished {var.total}'
`})
	r := event.New(db, testgame.State(db, 17), &host{})
	r.Log = func(err error) { t.Fatal(err) }
	if err := r.StartBound("tour", map[string]any{"prefix": "Outer"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Outer second: 1", "Outer first: 2", "Outer second: 3", "Outer Finished 3"} {
		q, ok := r.Next().(event.Say)
		if !ok || q.Text != want {
			t.Fatalf("got %+v; want %q", q, want)
		}
	}
	if q := r.Next(); q != nil {
		t.Fatalf("unexpected continuation: %+v", q)
	}
}

func TestRepeatedCallNestedBudgetStopsExecution(t *testing.T) {
	db := testgame.Load(t, nil)
	items := make([]string, 4096)
	for i := range items {
		items[i] = "unit"
	}
	db.Events["leaf"] = &data.Event{Params: map[string]string{"item": "string"}, Do: []data.Command{{SetV: data.Assignments{"var.visits": data.MustExpr("var.visits+1")}}}}
	db.Events["branch"] = &data.Event{Params: map[string]string{"item": "string"}, Do: []data.Command{{Call: data.EventCall{Event: "leaf", Each: &data.CallEach{Param: "item", Items: items}}}}}
	r := event.New(db, testgame.State(db, 17), &host{})
	var failure error
	r.Log = func(err error) { failure = err }
	r.Start([]data.Command{{Call: data.EventCall{Event: "branch", Each: &data.CallEach{Param: "item", Items: items}}}})
	if q := r.Next(); q != nil || r.Running() || failure == nil || !strings.Contains(failure.Error(), "65536") {
		t.Fatalf("unbounded execution: %v %v", q, failure)
	}
}

func TestSpeakerIdentityAcrossCalls(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(map[bool]string{false: "literal", true: "expression"}[dynamic], func(t *testing.T) {
			who := "scout"
			if dynamic {
				who = "{expr: arg.speaker}"
			}
			db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: speakers\n" + testgame.Game, "data/speakers.yaml": `events:
  speak:
    params: {speaker: actor}
    do:
    - say: {who: ` + who + `, text: 'Ready {gold}.'}
  visit:
    do:
    - call: {event: speak, args: {speaker: '"scout"'}}
    - call: {event: speak, args: {speaker: '"guide"'}}
`})
			r := event.New(db, testgame.State(db, 1), &host{})
			r.Log = func(err error) { t.Error(err) }
			r.Start(db.Events["visit"].Do)
			for _, actor := range []string{"scout", "guide"} {
				if !dynamic {
					actor = "scout"
				}
				line, ok := r.Next().(event.Say)
				if !ok || line.Actor != actor || line.Key != event.SourceKey(actor, "Ready {gold}.") || line.Text != "Ready 100." {
					t.Fatalf("speaker identity: %+v", line)
				}
			}
		})
	}
}

func TestScreenExpressionTarget(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: targets\n" + testgame.Game + `planning:
  calendar: {start: "2030-06-01", end: "2030-06-30", phases: [morning]}
  attributes:
    portrait: {type: string, initial: ''}
  entities:
    potter: {name: Potter}
screens:
  pick:
    title: Pick
    params: {resident: entity}
    view: {portrait: portrait}
    entries:
    - label: Visit
      target: {expr: arg.resident}
`})
	env := expr.Map{"arg": expr.Map{"resident": "potter"}}
	count := 0
	for index, entry := range event.ScreenEntries(db, "pick", env, func(err error) { t.Error(err) }) {
		count++
		if index != 0 || entry.Label != "Visit" || entry.Target.ID != "potter" || entry.Target.Expr.Set() {
			t.Fatal(index, entry)
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
	if !db.Game.Screens["pick"].Entries[0].Target.Expr.Set() {
		t.Fatal("definition mutated")
	}
	env["arg"] = expr.Map{"resident": "missing"}
	problems := 0
	for range event.ScreenEntries(db, "pick", env, func(error) { problems++ }) {
		t.Fatal("invalid target shown")
	}
	if problems != 1 {
		t.Fatal(problems)
	}
}

func TestInvalidSpeakerDoesNotConsumeRandomness(t *testing.T) {
	db := testgame.Load(t, nil)
	for _, value := range []string{`"missing"`, `""`, `42`, `rand(3)`} {
		t.Run(value, func(t *testing.T) {
			st, control := testgame.State(db, 17), testgame.State(db, 17)
			r := event.New(db, st, &host{})
			var problems []error
			r.Log = func(err error) { problems = append(problems, err) }
			r.Start([]data.Command{{Say: &data.Say{Who: data.IDRef{Expr: data.MustExpr(value)}, Text: "Ready"}}})
			if got := r.Next(); got != nil || len(problems) != 1 {
				t.Fatalf("request=%v errors=%v", got, problems)
			}
			if st.Rand.Uint64() != control.Rand.Uint64() {
				t.Fatal("speaker consumed randomness")
			}
		})
	}
}

func TestSpeakerVoiceAndPortraitUseResolvedActor(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": "schema_version: 10\nid: voices\n" + testgame.Game,
		"data/voice.yaml": `actors:
  potter: {name: Potter, faces: potter, voices: potter}
events:
  greeting:
    params: {speaker: actor}
    do:
    - say: {who: {expr: arg.speaker}, face: smile, voice: hello, text: Welcome}
`,
		"assets/potter/smile.png": "stub", "assets/potter/normal.png": "stub", "assets/potter/hello.wav": "stub",
	})
	var effects []event.Effect
	r := event.New(db, testgame.State(db, 1), event.HostFunc(func(e event.Effect) { effects = append(effects, e) }))
	r.Log = func(err error) { t.Error(err) }
	if err := r.StartBound("greeting", map[string]any{"speaker": "potter"}); err != nil {
		t.Fatal(err)
	}
	line, ok := r.Next().(event.Say)
	if !ok || line.Actor != "potter" || line.Face != "potter/smile.png" || line.Voice != "potter/hello.wav" {
		t.Fatal(line)
	}
	if len(effects) != 1 {
		t.Fatal(effects)
	}
	voice, ok := effects[0].(event.VoiceEffect)
	if !ok || voice.Who != "potter" || voice.File != line.Voice {
		t.Fatal(effects)
	}
}

func TestSelectedSourceKeepsLiteralReadIdentity(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		field := `text: 'Potter has {gold} coins.'`
		if dynamic {
			field = `text_expr: 'arg.name + " has {gold} coins."'`
		}
		db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: sources\n" + testgame.Game, "data/source.yaml": `events:
  speak:
    params: {name: string}
    do:
    - say: {who: scout, ` + field + `}
  visit:
    do:
    - call: {event: speak, args: {name: '"Potter"'}}
`})
		for _, gold := range []int{5, 9} {
			st := testgame.State(db, 17)
			st.Gold = gold
			r := event.New(db, st, &host{})
			r.Log = func(err error) { t.Error(err) }
			r.Start(db.Events["visit"].Do)
			line := testgame.As[event.Say](t, r.Next())
			if line.Key != event.SourceKey("scout", "Potter has {gold} coins.") || line.Text != fmt.Sprintf("Potter has %d coins.", gold) {
				t.Fatalf("source identity: %+v", line)
			}
		}
	}
}

func TestInvalidSelectedSourceDoesNotConsumeRandomness(t *testing.T) {
	db := testgame.Load(t, nil)
	for _, source := range []string{`42`, `rand()`, `"x" + missing.name`} {
		st, control := testgame.State(db, 17), testgame.State(db, 17)
		r := event.New(db, st, &host{})
		var failures int
		r.Log = func(error) { failures++ }
		r.Start([]data.Command{{Say: &data.Say{TextExpr: data.MustExpr(source)}}})
		if got := r.Next(); got != nil || failures != 1 {
			t.Fatalf("invalid source %s: %v, failures %d", source, got, failures)
		}
		a, b := st.Rand.Uint64(), control.Rand.Uint64()
		if a != b {
			t.Fatalf("source consumed randomness: %v != %v", a, b)
		}
	}
}
