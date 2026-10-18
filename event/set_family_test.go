package event_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

func TestSetFamilyIndependentOfDisplayName(t *testing.T) {
	for _, tc := range []struct {
		name, base, upper string
		count             int
	}{
		{"同一IDで別名", "family: frost, name: Frost", "family: frost, name: Pure Frost", 1},
		{"別IDで同名", "family: frost, name: Frost", "family: other, name: Frost", 2},
		{"旧形式で同名", "name: Frost", "name: Frost", 1},
		{"旧形式で別名", "name: Frost", "name: Pure Frost", 2},
		{"旧名と明示IDは別系列", "family: Frost, name: Frost", "name: Frost", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(stoneData, "frost: {name: Frost", "frost: {"+tc.base, 1)
			src = strings.Replace(src, "frost_up: {name: Frost", "frost_up: {"+tc.upper, 1)
			db := testgame.Load(t, map[string]string{
				"data/stones.yaml": src,
				"game.yaml": testgame.Game + `
equip_slots:
  - {id: weapon, name: W}
  - {id: armor, name: A}
  - {id: stone1, name: S1}
  - {id: stone2, name: S2}
  - {id: stone3, name: S3}
`,
			})
			st := testgame.State(db, 1)
			st.Join("monk")
			for _, id := range []string{"white", "white2", "blue"} {
				st.AddItem(id, 1)
			}
			wantFamily := []string{"frost"}
			wantSets := []string{"frost", "frost_up"}
			if tc.count == 1 {
				wantFamily = append(wantFamily, "frost_up")
				wantSets = wantSets[:1]
			}
			if got := st.SetFamily("frost"); !reflect.DeepEqual(got, wantFamily) {
				t.Fatalf("系列の順序 %v、期待 %v", got, wantFamily)
			}
			if offers := st.StoneOffers(); len(offers) != tc.count {
				t.Fatalf("候補 %v、期待する数 %d", offers, tc.count)
			}
			monk := st.Actors["monk"]
			before := monk.Stat("mdf")
			for i, id := range []string{"white", "white2", "blue"} {
				if err := st.Equip(monk, []string{"stone1", "stone2", "stone3"}[i], id); err != nil {
					t.Fatal(err)
				}
			}
			if got := monk.Sets(); !reflect.DeepEqual(got, wantSets) {
				t.Fatalf("成立と先勝ち %v、期待 %v", got, wantSets)
			}
			if got := monk.Stat("mdf") - before; got != 4*tc.count {
				t.Fatalf("セットの能力加算 %d、期待 %d", got, 4*tc.count)
			}
			if err := st.Equip(monk, "stone1", ""); err != nil {
				t.Fatal(err)
			}
			if got := st.SetHolds("frost"); got != (tc.count == 1) {
				t.Fatalf("上位石だけでの系列の成立 %v", got)
			}
			if got := rpg.SetsOf(db, "sage", []string{"white", "white2", "blue"}); len(got) != 0 {
				t.Fatalf("専用でない相手に成立 %v", got)
			}
		})
	}
}
