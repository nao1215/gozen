package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestBattleLargestTroopHitTargets(t *testing.T) {
	for _, width := range []int{1280, 1920} {
		g := &Game{db: &data.DB{Game: data.Game{Screen: data.Screen{Width: width, Height: 720}}}, pics: newPictures(nil), ui: 1}
		g.pics.imgs["neutral.png"] = &picEntry{img: missing}
		s := &battleScene{b: &battle.Battle{}}
		for range 32 {
			s.b.Foes = append(s.b.Foes, &battle.Unit{Enemy: true, Def: &data.Enemy{Image: data.Picture{File: "neutral.png"}}})
		}
		boxes := s.foeBoxes(g)
		right := 0
		for i, u := range s.b.Foes {
			r := boxes[u].rect
			if r.Empty() || r.Min.X < right || r.Max.X > width {
				t.Fatalf("width=%d enemy=%d rectangle=%v", width, i, r)
			}
			right = r.Max.X
		}
	}
}
