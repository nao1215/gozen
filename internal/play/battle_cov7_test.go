package play

import (
	"math/rand/v2"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// TestFoeBoxesProperty: for any mix of up to eight foes of every size in the game, the boxes stay
// inside the screen, do not overlap and keep to the right of the party's command column.
func TestFoeBoxesProperty(t *testing.T) {
	g, _ := btlOpen(t, Options{}, "", "")
	kinds := []string{"mote", "hulk", "giant", "idol", "warden", "brute", "caller"}
	rng := rand.New(rand.NewPCG(7, 9)) //nolint:gosec // A fixed seed for a reproducible test.
	sw, sh := g.screen()
	for n := range 80 {
		var members []string
		for range 1 + rng.IntN(8) {
			members = append(members, kinds[rng.IntN(len(kinds))])
		}
		id := "mix"
		g.db.Troops[id] = &data.Troop{Members: members}
		s := g.startBattle(id, func(int) {})
		boxes := s.foeBoxes(g)
		if len(boxes) != len(s.b.Foes) {
			t.Fatalf("mix %d %v: %d boxes", n, members, len(boxes))
		}
		for i, f := range s.b.Foes {
			r := boxes[f].rect
			if r.Min.X < 0 || float64(r.Max.X) > sw || r.Min.Y < 0 || float64(r.Max.Y) > sh || r.Empty() {
				t.Fatalf("mix %d %v: box %d %v outside %vx%v", n, members, i, r, sw, sh)
			}
			for j := range i {
				if r.Overlaps(boxes[s.b.Foes[j]].rect) {
					t.Fatalf("mix %d %v: boxes %d and %d overlap: %v %v", n, members, i, j, r, boxes[s.b.Foes[j]].rect)
				}
			}
		}
		g.pop()
	}
}
