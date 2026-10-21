package play

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func btlSongs(g *Game, rule bool) {
	g.db.Music.Songs = map[string]data.Song{
		"battle":  {BPM: 120, Moods: map[string]string{"fight": "a", "win": "b"}},
		"victory": {BPM: 120},
	}
	g.db.Music.Rules = nil
	if rule {
		g.db.Music.Rules = map[string][]data.MoodRule{"battle": {{When: data.Expr{Expr: testgame.Expr("true")}, Mood: "win"}}}
	}
}

// TestBattleMusicAtTheEnd: when a battle is won the music goes to the mood the game's rules name
// for it; with none, a fanfare plays; a guardian's defeat plays the game's boss song once and
// the field music waits for that song to finish.
func TestBattleMusicAtTheEnd(t *testing.T) {
	covSlow(t)
	g, out := btlOpen(t, Options{}, "", "")
	btlSongs(g, true)
	s := g.startBattle("one", func(int) {})
	if g.baseSong != "battle" {
		t.Fatalf("the battle song was not started: %q", g.baseSong)
	}
	btlAttackUntilOver(t, g, s)
	last := btlLines(t, out, "battle_music")
	if len(last) != 1 || last[0]["mood"] != "win" || last[0]["fanfare"] != false {
		t.Fatalf("music lines %v", last)
	}

	g.db.Music.Rules = nil
	s = g.startBattle("one", func(int) {})
	btlAttackUntilOver(t, g, s)
	lines := btlLines(t, out, "battle_music")
	if l := lines[len(lines)-1]; l["mood"] != "" || l["fanfare"] != true {
		t.Fatalf("no rule: %v", l)
	}

	g.db.Game.Battle.BossWinMusic = data.MusicCue{Song: "victory"}
	s = g.startBattle("guardian", func(int) {})
	btlAttackUntilOver(t, g, s)
	if s.dr.winSong != "victory" {
		t.Fatalf("the boss song was not chosen: %q", s.dr.winSong)
	}
	var seen bool
	for _, l := range btlLines(t, out, "battle_music") {
		seen = seen || l["win_song"] == "victory"
	}
	if !seen {
		t.Fatal("the boss song was not traced")
	}
	if g.fanfareSong != "victory" || g.fanfareT != g.songTicks("victory") {
		t.Fatalf("fanfare %q for %d ticks", g.fanfareSong, g.fanfareT)
	}
	// The field music waits for the song, then comes back.
	for g.fanfareT > 0 {
		before := g.fanfareT
		g.stepFanfare()
		if g.fanfareT != before-1 {
			t.Fatalf("fanfare time %d -> %d", before, g.fanfareT)
		}
	}
	g.stepFanfare()
	if g.fanfareSong != "" {
		t.Fatal("the song did not end")
	}
	// A different base song ends the wait at once.
	g.fanfareSong, g.fanfareT, g.baseSong = "victory", 50, "other"
	g.stepFanfare()
	if g.fanfareSong != "" || g.fanfareT != 0 {
		t.Fatalf("fanfare after the song changed: %q %d", g.fanfareSong, g.fanfareT)
	}
	g.stepFanfare() // nothing to wait for
}

// TestBossWinSongChoice: a troop's own win song takes precedence over the game's, an unknown
// song is left out, and a song without a cue is none.
func TestBossWinSongChoice(t *testing.T) {
	g, _ := btlOpen(t, Options{}, "", "")
	btlSongs(g, false)
	s := g.startBattle("guardian", func(int) {})
	if s.bossWinMusic(g) {
		t.Fatal("a song with no cue was played")
	}
	g.db.Game.Battle.BossWinMusic = data.MusicCue{Song: "missing"}
	if s.bossWinMusic(g) {
		t.Fatal("an unknown song was played")
	}
	g.db.Game.Battle.BossWinMusic = data.MusicCue{Song: "battle", Mood: "fight"}
	g.db.Troops["guardian"].WinMusic = data.MusicCue{Song: "victory"}
	if !s.bossWinMusic(g) || s.dr.winSong != "victory" || g.baseMood != sound.Neutral {
		t.Fatalf("troop song: %q mood %q", s.dr.winSong, g.baseMood)
	}
	g.db.Troops["guardian"].WinMusic = data.MusicCue{}
	if !s.bossWinMusic(g) || s.dr.winSong != "battle" || g.baseMood != "fight" {
		t.Fatalf("game song: %q mood %q", s.dr.winSong, g.baseMood)
	}
}

// TestSongTicks: a track or an unknown song is held for twenty seconds, a modular song for its
// module length (bars x beats at the tempo) and a little more.
func TestSongTicks(t *testing.T) {
	g, _ := btlOpen(t, Options{}, "", "")
	g.db.Music.Songs = map[string]data.Song{
		"track":   {File: "x.png"},
		"nobpm":   {},
		"modular": {BPM: 120},
		"slow":    {BPM: 60},
	}
	for id, want := range map[string]int{"track": 1200, "nobpm": 1200, "missing": 1200, "modular": 32*30 + 30, "slow": 32*60 + 30} {
		if got := g.songTicks(id); got != want {
			t.Errorf("songTicks(%s) = %d, want %d", id, got, want)
		}
	}
	g.db.Music.Bars, g.db.Music.Beats = 2, 3
	if got := g.songTicks("slow"); got != 6*60+30 {
		t.Errorf("custom module: %d", got)
	}
}

// TestBattleDramaTimings: the freeze stops the battle clock, slow motion skips every second tick,
// a charge counts down while shaking the foe, and the multipliers of the game's settings scale
// them (0 turns an effect off; on auto only what the game allows is kept).
func TestBattleDramaTimings(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "guardian"}, "", "")
	s := btlBattle(t, g)
	s.dr.freeze = 2
	first, second, third := s.stepDrama(g), s.stepDrama(g), s.stepDrama(g)
	if !first || !second || third {
		t.Fatalf("a freeze of two ticks held %v %v %v", first, second, third)
	}
	s.dr.slow = 4
	var got []bool
	for range 4 {
		got = append(got, s.stepDrama(g))
	}
	if got[0] != true || got[1] != false || got[2] != true || got[3] != false {
		t.Fatalf("slow motion pattern %v", got)
	}
	foe := s.b.Foes[0]
	s.dr.chargeU, s.dr.chargeLen, s.dr.charge = foe, 20, 20
	age := s.age
	s.tick(g)
	if s.dr.charge != 19 || s.shakeU[foe] < 1 || s.age != age+1 {
		t.Fatalf("charge %d shake %d", s.dr.charge, s.shakeU[foe])
	}
	if s.chargeScale(foe) <= 1 || s.chargeScale(s.b.Party[0]) != 1 {
		t.Fatalf("charge scale %v / %v", s.chargeScale(foe), s.chargeScale(s.b.Party[0]))
	}
	s.dr.charge = 0
	s.chargeTick(g)
	if s.dr.chargeU != nil {
		t.Fatal("the charge did not end")
	}
	s.chargeTick(g) // nothing charging

	// The settings.
	fx := &g.db.Game.UI.BattleFx
	if g.holdTicks(10, 1) != g.fxTicks(10) || g.holdTicks(10, 0) != 0 {
		t.Fatalf("holdTicks %d %d", g.holdTicks(10, 1), g.holdTicks(10, 0))
	}
	g.in.fast = true
	if g.holdTicks(10, 1) != 0 {
		t.Fatal("fast-forward holds the screen")
	}
	g.in.fast = false
	if k := s.dramaK(g, fx.Hitstop, fx.Auto.Hitstop, 0); k != 1 {
		t.Fatalf("manual k %v", k)
	}
	s.auto = true
	if k := s.dramaK(g, fx.Hitstop, fx.Auto.Hitstop, 0); k != 0 {
		t.Fatalf("auto k %v: hitstop is off on auto unless the game says otherwise", k)
	}
	fx.Auto.Hitstop = new(0.5)
	if k := s.dramaK(g, fx.Hitstop, fx.Auto.Hitstop, 0); k != 0.5 {
		t.Fatalf("auto k with a setting %v", k)
	}
	s.auto = false
	if g.fxk(nil) != 1 || g.fxk(new(-3.0)) != 0 || g.fxk(new(2.5)) != 2.5 {
		t.Fatal("fxk")
	}
	if g.flashAlpha(0.5) != 0.5 {
		t.Fatalf("flash %v", g.flashAlpha(0.5))
	}
	fx.Flash = new(0.0)
	if g.flashAlpha(0.5) != 0 {
		t.Fatal("flash off")
	}
}

// TestBattleHitStopOnCriticalAndWeak: a critical or a weak hit freezes the screen for a moment and
// shakes it more than a plain hit; the setting 0 turns the stop off.
func TestBattleHitStopOnCriticalAndWeak(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   battle.Step
		stop int
	}{
		{"critical", battle.Step{Crit: true}, hitstopCrit},
		{"weak", battle.Step{Weak: true}, hitstopWeak},
		{"plain", battle.Step{}, 0},
	} {
		g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
		s := btlBattle(t, g)
		s.hitDrama(g, tc.st)
		want := 0
		if tc.stop > 0 {
			want = g.fxTicks(tc.stop)
		}
		if s.dr.freeze != want {
			t.Errorf("%s: freeze %d, want %d", tc.name, s.dr.freeze, want)
		}
		if (tc.stop > 0) != (g.shake > 0) {
			t.Errorf("%s: shake %d", tc.name, g.shake)
		}
		g.db.Game.UI.BattleFx.Hitstop = new(0.0)
		s.dr.freeze = 0
		s.hitDrama(g, tc.st)
		if s.dr.freeze != 0 {
			t.Errorf("%s: a hit stop with the setting off", tc.name)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestBattleFinishingBlow: the last foe falling stops and slows the screen longer for a guardian.
func TestBattleFinishingBlow(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.finishDrama(g, s.b.Foes[0])
	plainStop, plainSlow := s.dr.freeze, s.dr.slow
	if !btlHas(btlLines(t, out, "battle_fx"), "what", "finish") {
		t.Fatal("not traced")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2, _ := btlOpen(t, Options{Battle: "guardian"}, "", "")
	boss := btlBattle(t, g2)
	boss.finishDrama(g2, boss.b.Foes[0])
	if boss.dr.freeze <= plainStop || boss.dr.slow <= plainSlow {
		t.Fatalf("plain %d/%d, boss %d/%d", plainStop, plainSlow, boss.dr.freeze, boss.dr.slow)
	}
	if err := g2.Close(); err != nil {
		t.Fatal(err)
	}
	// Others still standing: the foe is not the last.
	g3, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s3 := btlBattle(t, g3)
	if s3.lastFoe(s3.b.Foes[0]) {
		t.Fatal("the first of two is the last")
	}
	s3.shownHP[s3.b.Foes[1]] = 0
	if !s3.lastFoe(s3.b.Foes[0]) {
		t.Fatal("the only one standing is not the last")
	}
	s3.shownHP[s3.b.Foes[1]] = 5
	s3.gone[s3.b.Foes[1]] = 3
	if !s3.lastFoe(s3.b.Foes[0]) {
		t.Fatal("a foe that has gone counts as standing")
	}
}

// TestBattleKnownLines: what the player has learned of a foe is a line of weak and strong
// elements; a state at its forever-value shows without turns; party members have no line.
func TestBattleKnownLines(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "tall"}, "", "")
	s := btlBattle(t, g)
	giant := s.b.Foes[1]
	g.db.Enemies["giant"].Resist["fire"] = 2
	g.st.Vars["known_giant_ice"] = 1.0
	if line := s.known(g, giant); !strings.Contains(line, "Frost") || strings.Contains(line, "Warmth") {
		t.Fatalf("strong only: %q", line)
	}
	g.st.Vars["known_giant_fire"] = 1.0
	line := s.known(g, giant)
	if !strings.Contains(line, "Warmth") || !strings.Contains(line, "Frost") || !strings.Contains(line, " / ") {
		t.Fatalf("both: %q", line)
	}
	if s.known(g, s.b.Party[0]) != "" {
		t.Fatal("a member has a known line")
	}
	giant.States["poison"] = rpg.StateForever
	s.sync()
	if got := s.stateTurns(g, giant); got != "Poisoned" {
		t.Fatalf("a state without turns: %q", got)
	}
	giant.States["poison"] = 2
	s.sync()
	if got := s.stateTurns(g, giant); !strings.Contains(got, "2") {
		t.Fatalf("a state with turns: %q", got)
	}
	if s.usage("") != nil || s.usage("nope") != nil || s.usage("potion") == nil || s.usage("fire") == nil {
		t.Fatal("usage lookup")
	}
}

// TestBattleCutInOfAnItem: an item whose use has a cut-in shows the item's name in it, and a
// member with no picture to show gets none.
func TestBattleCutInOfAnItem(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one", Party: []string{"knight", "weaver"}}, "", "")
	s := btlBattle(t, g)
	g.db.Items["potion"].Use.Cutin = true
	if !s.startCutIn(g, battle.Step{Kind: battle.Act, User: s.b.Party[0], Skill: "potion"}) {
		t.Fatal("no cut-in for an item")
	}
	if s.cut.name != "Tonic" {
		t.Fatalf("cut-in name %q", s.cut.name)
	}
	g.db.Skills["finisher"].Cutin = true
	if s.startCutIn(g, battle.Step{Kind: battle.Act, User: s.b.Party[1], Skill: "finisher"}) {
		t.Fatal("a member with no picture got a cut-in")
	}
	if s.startCutIn(g, battle.Step{Kind: battle.Act, Skill: "finisher"}) {
		t.Fatal("a cut-in without a user")
	}
}
