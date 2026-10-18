package play

import (
	"fmt"
	"io/fs"
	"math"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

func messageMetrics(gm *data.Game) (size, width float64) {
	sw := logicalWidth(gm)
	if gm.UI.Message == "novel" {
		return novelSize, sw - 300
	}
	return talkSize, sw - 80 - 100
}

func MessagePages(gm *data.Game, text string) ([][]string, float64) {
	size, width := messageMetrics(gm)
	return gfx.Paginate(text, size, width, talkLines)
}

func MessageLines() int { return talkLines }

func MessageWrap(gm *data.Game, text string) []string {
	size, width := messageMetrics(gm)
	return gfx.Wrap(text, size, width)
}

func choiceWidth(gm *data.Game, options []string) float64 {
	w := 300.0
	for _, o := range options {
		w = math.Max(w, gfx.TextWidth(o, sizeM)+100)
	}
	const minTextW = 380.0
	sw := logicalWidth(gm)
	maxW := (sw - 80) * 0.6
	if gm.UI.Message == "novel" {
		maxW = sw - 40 - 180 - minTextW
	}
	return math.Min(w, math.Max(300, maxW))
}

func PromptLines(gm *data.Game, text string, options []string) ([]string, float64, bool) {
	size, width := messageMetrics(gm)
	return gfx.FitLinesCut(text, size, width, talkLines)
}

func UIScale(gm *data.Game) float64 {
	if gm.Screen.Height <= 0 {
		return 1
	}
	return float64(gm.Screen.Height) / gfx.BaseHeight
}

func logicalWidth(gm *data.Game) float64 { return float64(gm.Screen.Width) / UIScale(gm) }

func LoadFonts(db *data.DB, fsys fs.FS) error {
	gfx.SetScale(UIScale(&db.Game))
	f := db.Game.Fonts
	reg, err := fs.ReadFile(fsys, "assets/"+f.Regular)
	if err != nil {
		return fmt.Errorf("font: %w", err)
	}
	var pop []byte
	if f.Pop != "" {
		pop, err = fs.ReadFile(fsys, "assets/"+f.Pop)
		if err != nil {
			return fmt.Errorf("font: %w", err)
		}
	}
	if err := gfx.LoadFonts(reg, pop); err != nil {
		return err
	}
	gfx.SetScale(UIScale(&db.Game))
	return nil
}
