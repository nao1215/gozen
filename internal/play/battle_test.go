package play

import (
	"image"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// btlAttackUntilOver attacks the first living foe with every member until the battle screen
// is gone, and returns the screen.
func btlAttackUntilOver(t *testing.T, g *Game, s *battleScene) {
	t.Helper()
	for range 20000 {
		if g.currentBattle() != s {
			return
		}
		if s.phase == bpInput && s.target == nil && s.sub == nil {
			btlKey(g, btnOK) // Attack
			btlKey(g, btnOK) // the first foe
			continue
		}
		if s.phase == bpResult {
			btlFinish(t, g, s)
			return
		}
		btlFrame(t, g)
	}
	t.Fatalf("battle never ended, phase %d", s.phase)
}

// TestBattleManualWinAndTrace: a player attacks with the first command, aims at the foe and
// wins; the trace carries the capture, the steps and the result of the battle.
func TestBattleManualWinAndTrace(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	if s.phase != bpInput || len(s.members) != 2 {
		t.Fatalf("phase %d members %d", s.phase, len(s.members))
	}
	for range 2 {
		btlKey(g, btnOK) // Attack
		if s.target == nil || !s.target.enemies {
			t.Fatalf("no foe to aim at after Attack: %+v", s.target)
		}
		btlKey(g, btnOK)
	}
	if s.phase != bpPlay || len(s.actions) != 2 {
		t.Fatalf("phase %d actions %d", s.phase, len(s.actions))
	}
	btlFinish(t, g, s)
	if s.b.Result != battle.Won {
		t.Fatalf("result %v", s.b.Result)
	}
	if n := len(btlLines(t, out, "battle_capture")); n != 1 {
		t.Fatalf("%d captures", n)
	}
	if r := btlLines(t, out, "battle_end"); len(r) != 1 || r[0]["result"] != "won" {
		t.Fatalf("battle_end %v", r)
	}
	if len(btlLines(t, out, "battle_step")) == 0 || len(btlLines(t, out, "battle_result")) != 1 {
		t.Fatal("no steps or no result in the trace")
	}
	if g.st.Gold != 107 {
		t.Fatalf("gold %d, want 100 + 7", g.st.Gold)
	}
}

// TestBattleCommandMenus: the command list has six rows, the skill list shows what a
// member knows and costs, the item list the bag, B steps back, and an item used on an ally
// is spent at the end of the turn.
func TestBattleCommandMenus(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	if len(s.cmd.items) != 6 || s.cmd.items[5].disabled {
		t.Fatalf("commands %+v", s.cmd.items)
	}
	btlChoose(g, s, 1) // Skills
	if s.sub == nil || s.subKind != "skill" || !slices.Contains(s.subIDs, "fire") {
		t.Fatalf("skill list %v (%q)", s.subIDs, s.subKind)
	}
	btlKey(g, btnCancel)
	if s.sub != nil {
		t.Fatal("cancel did not close the skill list")
	}
	s.cmd.sel = 3
	btlKey(g, btnOK) // Items
	if s.sub == nil || s.subKind != "item" || s.subIDs[0] != "potion" {
		t.Fatalf("item list %v", s.subIDs)
	}
	btlKey(g, btnOK)
	if s.target == nil || s.target.enemies || len(s.target.pool) != 2 {
		t.Fatalf("an item for one ally needs an ally target: %+v", s.target)
	}
	btlKey(g, btnDown)
	btlKey(g, btnLeft)
	btlKey(g, btnRight)
	btlKey(g, btnCancel)
	if s.target != nil {
		t.Fatal("cancel did not close the targeting")
	}
	btlKey(g, btnOK) // the item again
	btlKey(g, btnOK) // the first ally
	if s.mi != 1 || s.actions[0].Item != "potion" {
		t.Fatalf("mi %d actions %+v", s.mi, s.actions)
	}
	btlKey(g, btnCancel) // back to the first member
	if s.mi != 0 || len(s.actions) != 0 {
		t.Fatalf("cancel did not step back: mi %d actions %d", s.mi, len(s.actions))
	}
}

// TestBattleNoItemsBuzzes: with an empty bag the item command only buzzes.
func TestBattleNoItemsBuzzes(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	g.st.Items = map[string]int{}
	s := btlAsk(t, g)
	s.cmd.sel = 3
	btlKey(g, btnOK)
	if s.sub != nil || s.phase != bpInput {
		t.Fatalf("an empty bag opened a list: %v", s.sub)
	}
}

// TestBattleSkillOnAlliesAndDead: a skill for one ally lists the living allies, and one for a
// fallen ally lists only the fallen; with none fallen it buzzes.
func TestBattleSkillOnAlliesAndDead(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	s.choose(g, battle.Action{User: s.members[0], Skill: "raise_all"})
	if s.target != nil {
		t.Fatal("nobody has fallen, but a target list opened")
	}
	s.b.Party[1].HP = 0
	s.choose(g, battle.Action{User: s.members[0], Skill: "raise_all"})
	if s.target == nil || len(s.target.pool) != 1 || s.target.pool[0] != s.b.Party[1] {
		t.Fatalf("pool %+v", s.target)
	}
}

// TestBattleEscape: the escape command ends the battle as Fled; a troop that cannot be
// fled from has the command disabled.
func TestBattleEscape(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	btlChoose(g, s, 5)
	btlFinish(t, g, s)
	if s.b.Result != battle.Fled {
		t.Fatalf("result %v", s.b.Result)
	}
	if r := btlLines(t, out, "battle_end"); len(r) != 1 || r[0]["result"] != "fled" {
		t.Fatalf("battle_end %v", r)
	}

	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2, _ := btlOpen(t, Options{Battle: "doom"}, "", "")
	s2 := btlAsk(t, g2)
	if !s2.cmd.items[5].disabled {
		t.Fatal("escape is open in a battle that cannot be fled")
	}
	btlChoose(g2, s2, 5)
	if s2.phase != bpInput {
		t.Fatal("a disabled command was taken")
	}
}

// TestBattleAutoCommandAndCancel: the AUTO command makes the party fight by itself and X
// takes it back.
func TestBattleAutoCommandAndCancel(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	btlChoose(g, s, 4)
	if !s.auto || !g.autoBattle || !s.b.Party[0].Auto {
		t.Fatalf("auto %v %v", s.auto, g.autoBattle)
	}
	for range 3 {
		btlFrame(t, g)
	}
	g.in.pressed[btnCancel] = true
	s.tick(g)
	g.in.pressed = [numButtons]bool{}
	if s.auto || g.autoBattle || s.b.Party[0].Auto {
		t.Fatal("cancel did not stop the auto battle")
	}
}

// TestBattleAutoStartsAndWins: with auto_start the party never asks for a command and the
// battle ends in a win in the open stack without input.
func TestBattleAutoStartsAndWins(t *testing.T) {
	g, out := btlOpen(t, Options{}, "", "")
	g.db.Game.Battle.AutoTempo = 2
	g.autoBattle = true
	g.startBattle("pack", func(int) {})
	s := btlBattle(t, g)
	if !s.auto {
		t.Fatal("not on auto")
	}
	for range 6000 {
		if g.currentBattle() != s {
			break
		}
		btlFrame(t, g)
	}
	if g.currentBattle() == s || s.b.Result != battle.Won {
		t.Fatalf("result %v phase %d results %v endT %d stack %T", s.b.Result, s.phase, s.results, s.endT, g.stack[len(g.stack)-1])
	}
	if len(btlLines(t, out, "battle_close")) == 0 {
		t.Fatal("battle_close not traced")
	}
}

// TestBattleGuardianFight: a guardian opens with a banner, charges its big move with a
// cut-in, and its defeat is stamped on the screen.
func TestBattleGuardianFight(t *testing.T) {
	covSlow(t)
	g, out := btlOpen(t, Options{Battle: "guardian"}, "", "")
	s := btlBattle(t, g)
	if s.boss != "Warden" || s.bossTitle != "Gate keeper" {
		t.Fatalf("boss %q title %q", s.boss, s.bossTitle)
	}
	btlAttackUntilOver(t, g, s)
	if s.b.Result != battle.Won {
		t.Fatalf("result %v", s.b.Result)
	}
	var charged, finished, cut bool
	for _, l := range btlLines(t, out, "battle_fx") {
		charged = charged || l["what"] == "charge"
		finished = finished || (l["what"] == "finish" && l["boss"] == true)
	}
	for _, l := range btlLines(t, out, "battle") {
		cut = cut || l["cutin"] == "Nova"
	}
	if !charged || !finished || !cut {
		t.Fatalf("charge %v finish %v cutin %v", charged, finished, cut)
	}
	if !s.dr.bossWon {
		t.Fatal("the guardian's defeat was not stamped")
	}
}

// TestBattleFastSkipsCharge: holding fast-forward leaves out the boss's charge.
func TestBattleFastSkipsCharge(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "guardian"}, "", "")
	s := btlBattle(t, g)
	g.in.fast = true
	for range 20 {
		if s.phase == bpInput {
			break
		}
		s.tick(g)
	}
	g.in.fast = false
	for _, l := range btlLines(t, out, "battle_fx") {
		if l["what"] == "charge" {
			t.Fatal("charged while fast-forwarding")
		}
	}
}

// TestBattleLossAndRetry: a lost battle asks to retry from before it when the game allows,
// and answering yes starts the same battle again from the saved state.
func TestBattleLossAndRetry(t *testing.T) {
	covSlow(t)
	g, out := btlOpen(t, Options{Battle: "doom", Level: 1}, "", "")
	g.db.Game.Battle.Retry = true
	s := btlAsk(t, g)
	snap, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	s.retry = snap
	btlChoose(g, s, 2) // guard
	btlChoose(g, s, 2)
	for range 3000 {
		if _, ok := g.stack[len(g.stack)-1].(*confirm); ok {
			break
		}
		if s.phase == bpInput {
			btlChoose(g, s, 2)
			btlChoose(g, s, 2)
		}
		if s.phase == bpResult {
			g.in.pressed[btnOK] = true
			g.stack[len(g.stack)-1].update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	q, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok {
		t.Fatalf("no retry question, top is %T (result %v)", g.stack[len(g.stack)-1], s.b.Result)
	}
	if s.b.Result != battle.Lost {
		t.Fatalf("result %v", s.b.Result)
	}
	for range 6 {
		q.update(g)
	}
	g.in.pressed[btnOK] = true
	q.update(g)
	g.in.pressed = [numButtons]bool{}
	n := g.currentBattle()
	if n == nil || n == s || n.b.Result != battle.Ongoing {
		t.Fatal("retry did not start a new battle")
	}
	if r := btlLines(t, out, "request"); len(r) == 0 || r[len(r)-1]["what"] != "retry" {
		t.Fatalf("requests %v", r)
	}
}

// TestBattleLossWithoutRetryEnds: without the retry setting a loss ends the battle screen
// and reports a loss to the event.
func TestBattleLossWithoutRetryEnds(t *testing.T) {
	covSlow(t)
	g, out := btlOpen(t, Options{Battle: "doom"}, "", "")
	s := btlBattle(t, g)
	for range 6000 {
		if g.currentBattle() != s {
			break
		}
		if s.phase == bpInput && s.target == nil {
			btlChoose(g, s, 2)
			btlChoose(g, s, 2)
			continue
		}
		if s.phase == bpResult {
			g.in.pressed[btnOK] = true
			g.stack[len(g.stack)-1].update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	if s.b.Result != battle.Lost {
		t.Fatalf("result %v", s.b.Result)
	}
	if r := btlLines(t, out, "battle_end"); len(r) != 1 || r[0]["result"] != "lost" {
		t.Fatalf("battle_end %v", r)
	}
}

// TestFoeBoxesFitTheScreen: however many foes there are, their boxes stay on the screen,
// keep to the right of the party's menu column and do not overlap.
func TestFoeBoxesFitTheScreen(t *testing.T) {
	for _, troop := range []string{"one", "two", "pack", "guardian"} {
		g, _ := btlOpen(t, Options{Battle: troop}, "", "")
		s := btlBattle(t, g)
		sw, sh := g.screen()
		boxes := s.foeBoxes(g)
		if len(boxes) != len(s.b.Foes) {
			t.Fatalf("%s: %d boxes for %d foes", troop, len(boxes), len(s.b.Foes))
		}
		var rs []foeBox
		for _, f := range s.b.Foes {
			b := boxes[f]
			if b.rect.Min.X < 0 || float64(b.rect.Max.X) > sw || b.rect.Min.Y < 0 || float64(b.rect.Max.Y) > sh {
				t.Fatalf("%s: box %v outside %vx%v", troop, b.rect, sw, sh)
			}
			for _, o := range rs {
				if b.rect.Overlaps(o.rect) {
					t.Fatalf("%s: boxes %v and %v overlap", troop, b.rect, o.rect)
				}
			}
			rs = append(rs, b)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestBattleWeaknessIsLearned: hitting a foe with an element it is weak to remembers it, and
// the aim line then names that element.
func TestBattleWeaknessIsLearned(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	foe := s.b.Foes[0]
	if line := s.known(g, foe); line != "" {
		t.Fatalf("known before any hit: %q", line)
	}
	btlChoose(g, s, 1) // Skills
	idx := slices.Index(s.subIDs, "fire")
	for s.sub.sel < idx {
		btlKey(g, btnDown)
	}
	btlKey(g, btnOK)
	btlKey(g, btnOK) // the foe
	btlChoose(g, s, 2)
	btlUntil(t, g, "the hit", 2000, func() bool { return g.st.Vars["known_mote_fire"] != nil })
	if line := s.known(g, foe); !strings.Contains(line, "Warmth") {
		t.Fatalf("the learned weakness is not shown: %q", line)
	}
}

// TestBattleCriticalAndWeakPopups: a critical hit and a weak hit stop the screen and tag
// the number; the hit stop is shorter than the finishing stop.
func TestBattleCriticalAndWeakPopups(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "pack"}, "", "")
	g.db.Game.Battle.Critical = data.Expr{Expr: testgame.Expr("1")}
	s := btlAsk(t, g)
	for range 2 {
		btlKey(g, btnOK)
		btlKey(g, btnOK)
	}
	btlUntil(t, g, "a critical number", 3000, func() bool {
		for _, p := range s.pops {
			if p.big {
				return true
			}
		}
		return false
	})
	var tagged bool
	for _, p := range s.pops {
		tagged = tagged || strings.Contains(p.text, "CRITICAL")
	}
	if !tagged {
		t.Fatalf("no CRITICAL tag in %+v", s.pops)
	}
}

func TestBattleTextCanHideTacticLog(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	st := battle.Step{Kind: battle.Act, Text: "Maren: Strike! [Maren row 3: Always -> Foe]"}
	if got := s.battleText(g, st); got != st.Text {
		t.Fatalf("development log lost: %q", got)
	}
	s.hideTacticLog = true
	if got := s.battleText(g, st); got != "Maren: Strike! " {
		t.Fatalf("capture text %q", got)
	}
	st.Kind = battle.Message
	if got := s.battleText(g, st); got != st.Text {
		t.Fatalf("non-action text changed: %q", got)
	}
}

func TestBattleTextHidesTacticLogWithoutHistory(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "ui: {battle_ai_history: false}\n", "")
	s := btlAsk(t, g)
	st := battle.Step{Kind: battle.Act, Text: "Maren: Strike! [Maren row 3: Always -> Foe]"}
	if got := s.battleText(g, st); got != "Maren: Strike! " {
		t.Fatalf("explanation shown without a history to read: %q", got)
	}
}

func TestBattleAIHistoryAuthorVisibility(t *testing.T) {
	if !slices.Contains(keys[btnLog], ebiten.KeyB) || !slices.Contains(pads[btnLog], ebiten.StandardGamepadButtonFrontTopLeft) {
		t.Fatal("keyboard/controller history bindings changed")
	}

	for _, mode := range []string{"default", "true", "false"} {
		t.Run(mode, func(t *testing.T) {
			config := ""
			if mode != "default" {
				config = "ui: {battle_ai_history: " + mode + "}\n"
			}
			g, _ := btlOpen(t, Options{Battle: "one"}, config, "")
			s := btlAsk(t, g)
			if s.tacticHistoryVisible(g) != (mode != "false") {
				t.Fatal("button visibility disagrees with configuration")
			}
			for _, control := range []string{"log", "pointer"} {
				sysKey(g)
				if control == "log" {
					g.in.pressed[btnLog] = true
				} else {
					g.in.click = true
					box := s.reasonButton(g)
					g.in.mouse = box.Min.Add(image.Pt(5, 5))
				}
				s.update(g)
				_, opened := g.stack[len(g.stack)-1].(*tacticReasons)
				if opened != (mode != "false") {
					t.Fatalf("mode %s control %s: history opened %v", mode, control, opened)
				}
				if opened {
					g.pop()
				}
			}
		})
	}
}

func TestBattleAIHistoryDoesNotChangeDecisionsOrReplay(t *testing.T) {
	var baseline []traceLine
	for _, mode := range []string{"true", "false"} {
		config := "ui: {battle_ai_history: " + mode + "}\n"
		g, out := btlOpen(t, Options{Battle: "two", Location: "camp", Script: "press ok 160\nquit\n", ShotDir: t.TempDir(), Seed: 11}, config, "")
		btlRunToEnd(t, g, 8000)
		lines := btlLines(t, out, "ai_decision", "battle_step", "battle_result")
		if len(btlLines(t, out, "ai_decision")) == 0 || len(btlLines(t, out, "battle_result")) != 1 {
			t.Fatal("missing decisions or result")
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		if baseline == nil {
			baseline = lines
		} else if !reflect.DeepEqual(baseline, lines) {
			t.Fatal("hiding AI history changed decisions, steps or rewards")
		}
		replay, err := New(Options{FS: btlFiles(t, config, ""), NoSound: true, SaveDir: t.TempDir(), Replay: btlWriteTrace(t, out.Bytes()), ReplayBattle: 1, Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		btlRunToEnd(t, replay, 12000)
		if ReplayReport == "" || strings.Contains(ReplayReport, "diverged") {
			t.Fatalf("mode %s replay: %s", mode, ReplayReport)
		}
		if err := replay.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBattleDefeatRecoveryPolicy(t *testing.T) {
	for _, noRecovery := range []bool{true, false} {
		t.Run(map[bool]string{true: "preserve", false: "recover"}[noRecovery], func(t *testing.T) {
			g, _ := btlOpen(t, Options{}, "", "")
			scoutHP := g.st.Actors["scout"].HP
			g.begin(event.Battle{Troop: "doom", Fighters: []string{"guide"}, CanLose: true, NoRetry: true, NoRecovery: noRecovery})
			s := g.currentBattle()
			if s == nil || len(s.b.Party) != 1 || s.b.Party[0].Actor.ID != "guide" {
				t.Fatal("wrong participants")
			}
			callbacks := 0
			end := s.onEnd
			s.onEnd = func(res int) {
				callbacks++
				if res != event.BattleLost {
					t.Fatalf("result %d", res)
				}
				want := 1
				if noRecovery {
					want = 0
				}
				if g.st.Actors["guide"].HP != want || g.st.Actors["scout"].HP != scoutHP {
					t.Fatal("wrong recovery")
				}
				if g.st.LastBattle == nil || g.st.LastBattle.HP["guide"] != 0 || len(g.st.LastBattle.HP) != 1 {
					t.Fatal("wrong snapshot")
				}
				end(res)
			}
			btlAttackUntilOver(t, g, s)
			if callbacks != 1 {
				t.Fatalf("callbacks %d", callbacks)
			}
		})
	}
}

func TestBattleRetryKeepsFightersAndRecovery(t *testing.T) {
	g, _ := btlOpen(t, Options{}, "", "")
	g.db.Game.Battle.Retry = true
	g.begin(event.Battle{Troop: "doom", Fighters: []string{"guide"}, CanLose: true, NoRecovery: true})
	s := g.currentBattle()
	s.b.Party[0].HP = 0
	s.b.Result = battle.Lost
	s.b.End()
	s.phase, s.endT = bpResult, 10
	s.result(g)
	q, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok {
		t.Fatal("retry question missing")
	}
	for range 6 {
		q.update(g)
	}
	g.in.pressed[btnOK] = true
	q.update(g)
	g.in.pressed = [numButtons]bool{}
	n := g.currentBattle()
	if n == nil || n == s || len(n.b.Party) != 1 || n.b.Party[0].Actor.ID != "guide" || !n.noRecovery {
		t.Fatal("retry lost fighter selection or recovery policy")
	}
	if g.st.LastBattle != nil {
		t.Fatal("retry retained the discarded outcome")
	}
}
