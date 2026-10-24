package dungeon

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// spaceStep resolves topology without committing state. Intermediate stairs stay
// on their lower level; only the final cell crosses to the upper landing.
func spaceStep(s *data.Space, p rpg.Pos) (rpg.Pos, bool) {
	out := p
	out.X, out.Y = Rel(p, 1, 0)
	for _, stair := range s.Stairs {
		if stair.Level-1 == p.Level {
			if n, on := data.StairOffset(stair, p.X, p.Y); on && n > 0 && n < stair.Length {
				if next, along := data.StairOffset(stair, out.X, out.Y); !along {
					return out, false
				} else if next == stair.Length {
					out.Level++
				}
			}
			if n, on := data.StairOffset(stair, out.X, out.Y); on && n > 0 && n < stair.Length {
				if from, along := data.StairOffset(stair, p.X, p.Y); !along || from == stair.Length {
					return out, false
				}
			}
		}
		if stair.Level == p.Level {
			n, on := data.StairOffset(stair, p.X, p.Y)
			next, along := data.StairOffset(stair, out.X, out.Y)
			if on && n == stair.Length && along && next == stair.Length-1 {
				out.Level--
			}
		}
	}
	return out, data.SpaceCell(s, out.Level, out.X, out.Y) != data.CellNone
}
