package play

import (
	"os"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Every sample troop must fit at both documented screen sizes, regardless of
// the source pictures' pixel dimensions.
func TestGoZenTroopPictureLayout(t *testing.T) {
	root := "../../examples/go-zen/game"
	raw, err := os.ReadFile(root + "/data/enemies.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var sample struct {
		Enemies map[string]*data.Enemy `yaml:"enemies"`
		Troops  map[string]*data.Troop `yaml:"troops"`
	}
	if err := yaml.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(root)
	for _, size := range [][2]int{{1280, 720}, {1920, 1080}} {
		g := &Game{db: &data.DB{Game: data.Game{Screen: data.Screen{Width: size[0], Height: size[1]}}}, pics: newPictures(fsys), ui: 1}
		for id, troop := range sample.Troops {
			s := &battleScene{b: &battle.Battle{}}
			for _, member := range troop.Members {
				def := sample.Enemies[member]
				if def == nil {
					t.Fatalf("%s: unknown enemy %s", id, member)
				}
				if def.ImageHeight <= 0 {
					t.Errorf("%s: %s has no image_height", id, member)
				}
				s.b.Foes = append(s.b.Foes, &battle.Unit{Enemy: true, Def: def})
			}
			boxes := s.foeBoxes(g)
			lastRight := 0
			for i, foe := range s.b.Foes {
				b := boxes[foe]
				if b.h <= 0 || b.rect.Empty() || b.rect.Min.X < 300 || b.rect.Max.X > size[0]-36 || b.rect.Min.Y < 150 || b.bottom > float64(size[1]) || b.rect.Min.X < lastRight {
					t.Errorf("%dx%d %s foe %d: invalid box %+v after x=%d", size[0], size[1], id, i, b, lastRight)
				}
				lastRight = b.rect.Max.X
			}
		}
		for _, tc := range []struct {
			id     string
			lo, hi float64
		}{{"succubus", .64, .80}, {"drip_slime", .25, .30}, {"king_slime", .50, .60}} {
			def := sample.Enemies[tc.id]
			s := &battleScene{b: &battle.Battle{Foes: []*battle.Unit{{Enemy: true, Def: def}}}}
			b := s.foeBoxes(g)[s.b.Foes[0]]
			ratio := b.h / float64(size[1])
			if ratio < tc.lo || ratio > tc.hi {
				t.Errorf("%dx%d %s height ratio %.3f", size[0], size[1], tc.id, ratio)
			}
		}
	}
}

func TestBattleTextCanHideTacticLog(t *testing.T) {
	s := &battleScene{}
	st := battle.Step{Kind: battle.Act, Text: "Maren: Strike! [Maren row 3: Always -> Foe]"}
	if got := s.battleText(st); got != st.Text {
		t.Fatalf("development log lost: %q", got)
	}
	s.hideTacticLog = true
	if got := s.battleText(st); got != "Maren: Strike! " {
		t.Fatalf("capture text %q", got)
	}
	st.Kind = battle.Message
	if got := s.battleText(st); got != st.Text {
		t.Fatalf("non-action text changed: %q", got)
	}
}
