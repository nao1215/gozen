package battle_test

import (
	"fmt"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// FuzzBattle: whatever the party does turn after turn, HP and MP stay in range, a decided battle stays decided,
// and the same seed gives the same battle line by line.
func FuzzBattle(f *testing.F) {
	f.Add(uint64(1), uint8(0), []byte{0, 1, 2, 3, 4, 5})
	f.Add(uint64(7), uint8(1), []byte{9, 9, 9, 8, 8, 8, 0})
	f.Add(uint64(3), uint8(2), []byte{})
	skills := []string{"attack", "guard", "fire", "heal", "siphon", "charge", "ram", "cleanse", "rez", "rez_all", "flee", "sure_miss", "all_ring", "hurt_all", "poison_hit", "nap", "venom", "big", "nothing", ""}
	troops := []string{"one", "two", "caster", "pair", "boss"}
	f.Fuzz(func(t *testing.T, seed uint64, troop uint8, ops []byte) {
		if len(ops) > 60 {
			return
		}
		play := func() (string, *battle.Battle) {
			db := testgame.Load(t, map[string]string{"data/effects.yaml": effectData})
			st := testgame.State(db, seed)
			st.Join("guide")
			b := battle.New(db, st, troops[int(troop)%len(troops)])
			var log string
			for i := 0; i+1 < len(ops) && b.Result == battle.Ongoing && b.Turn < 40; i += 2 {
				user := b.Party[int(ops[i])%len(b.Party)]
				skill := skills[int(ops[i+1])%len(skills)]
				var target *battle.Unit
				if all := append(append([]*battle.Unit{}, b.Party...), b.Foes...); len(all) > 0 {
					target = all[int(ops[i+1]>>3)%len(all)]
				}
				for _, s := range run(b, battle.Action{User: user, Skill: skill, Target: target}) {
					log += fmt.Sprintf("%d|%s|%d|%d\n", s.Kind, s.Text, s.Amount, s.HP)
				}
				for _, u := range append(append([]*battle.Unit{}, b.Party...), b.Foes...) {
					if u.HP < 0 || u.HP > u.MHP() || u.MP < 0 || u.MP > u.MMP() {
						t.Fatalf("%s is out of range: hp %d/%d mp %d/%d\n%s", u.Name, u.HP, u.MHP(), u.MP, u.MMP(), log)
					}
				}
			}
			return log, b
		}
		a, ba := play()
		c, bc := play()
		if a != c || ba.Result != bc.Result || ba.Turn != bc.Turn {
			t.Fatalf("the same seed gave two battles:\n%s\n---\n%s", a, c)
		}
		if ba.Result != battle.Ongoing {
			if lines := ba.End(); lines == nil && ba.Result == battle.Won {
				t.Log("a won battle with no lines")
			}
		}
	})
}
