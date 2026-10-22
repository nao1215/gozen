package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestBattleFormTransitionDrawsInExistingScreen(t *testing.T) {
	covSlow(t)
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "")
	s := btlBattle(t, g)
	old := s.b.Foes[0]
	use := s.b.DB.Skills["call_mote"]
	use.Effects[0].Summon = ""
	use.Effects[0].Transform = "mote"
	old.HP = old.MHP() / 2
	btlUntil(t, g, "input", 4000, func() bool { return s.phase == bpInput })
	btlGuardAll(g, s)
	btlUntil(t, g, "form", 4000, func() bool { return old.EID == "mote" })
	if s.b.Foes[0] != old {
		t.Fatal("form changed the battle slot")
	}
	btlUntil(t, g, "transition", 4000, func() bool { return s.foeFade[old].ticks > 0 })
	if s.foeFade[old].from == "" || !s.foeFade[old].form {
		t.Fatal("old picture was not kept for crossfade")
	}
	for range 30 {
		btlFrame(t, g)
	}
}

func TestEnemyPuppetUsesActorRigFormat(t *testing.T) {
	covSlow(t)
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "", map[string]string{
		"assets/p/rig.yaml": "base: base.png\nfaces: {normal: {}}\n",
		"assets/p/base.png": string(btlPNG(32, 32)),
	})
	s := btlBattle(t, g)
	u := s.b.Foes[0]
	u.Def.Puppet = "p/rig.yaml"
	p := g.enemyPuppet(u, "normal.png", u.HP)
	if p == nil {
		t.Fatal("enemy puppet was not loaded")
	}
	if g.puppets.live["enemy."+u.EID] == nil {
		t.Fatal("enemy puppet was not retained")
	}
	before := p.Motion.Pose()
	s.reactTo(g, u, "hit", 40)
	for range 5 {
		p.Motion.Step()
	}
	if p.Motion.Pose() == before {
		t.Fatal("hit did not move the enemy puppet")
	}
}

func TestBattleHPPictureCrossfades(t *testing.T) {
	covSlow(t)
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "", map[string]string{"assets/hurt.png": string(btlPNG(32, 32))})
	s := btlBattle(t, g)
	u := s.b.Foes[0]
	u.Def.Images = []data.HPImage{{Below: 0.5, File: "hurt.png"}}
	u.Def.Motion = "breathe"
	s.shownHP[u] = u.MHP()
	s.drawFoes(g, btlImage())
	if got := s.foeImages[u]; got != u.Def.Image.File {
		t.Fatalf("healthy picture %q", got)
	}
	s.shownHP[u] = u.MHP() / 4
	s.drawFoes(g, btlImage())
	if got := s.foeImages[u]; got != "hurt.png" || s.foeFade[u].ticks == 0 {
		t.Fatalf("hurt picture %q fade %+v", got, s.foeFade[u])
	}
}

func TestPartyHPFaceCrossfades(t *testing.T) {
	covSlow(t)
	g, _ := btlOpen(t, Options{Battle: "one", Party: []string{"knight"}}, "", "")
	s := btlBattle(t, g)
	u := s.b.Party[0]
	u.Actor.Def().FaceRule = nil
	u.Actor.Def().Images = []data.HPImage{{Below: 0.4, Face: "grin"}}
	s.shownHP[u] = u.MHP()
	s.drawParty(g, btlImage())
	s.shownHP[u] = u.MHP() / 4
	s.drawParty(g, btlImage())
	if s.faceOf(u) != "grin" || s.foeFade[u].ticks == 0 {
		t.Fatalf("face=%q fade=%+v", s.faceOf(u), s.foeFade[u])
	}
}
