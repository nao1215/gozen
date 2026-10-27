package play

import (
	"fmt"
	"io/fs"
	"math"

	"golang.org/x/image/font/gofont/goregular"

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

func PromptLines(gm *data.Game, text string, _ []string) ([]string, float64, bool) {
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

// FontError is a font of game.yaml that cannot be used: Key is the field
// (fonts.regular or fonts.pop) and Err why.
type FontError struct {
	Key string
	Err error
}

func (e *FontError) Error() string { return e.Key + ": " + e.Err.Error() }

func (e *FontError) Unwrap() error { return e.Err }

// LoadFonts reads the fonts of game.yaml. Without fonts.regular the text is set
// in Go Regular, which Gozen carries (golang.org/x/image/font/gofont,
// BSD-3-Clause; Latin letters only). A failure is a *FontError.
func LoadFonts(db *data.DB, fsys fs.FS) error {
	gfx.SetScale(UIScale(&db.Game))
	f := db.Game.Fonts
	reg := goregular.TTF // the built-in font of a game without fonts.regular (Latin letters only)
	if f.Regular != "" {
		var err error
		if reg, err = fs.ReadFile(fsys, "assets/"+f.Regular); err != nil {
			return &FontError{Key: "fonts.regular", Err: err}
		}
	}
	if err := gfx.LoadFonts(reg, nil); err != nil {
		return &FontError{Key: "fonts.regular", Err: fmt.Errorf("assets/%s: %w", f.Regular, err)}
	}
	if f.Pop != "" {
		pop, err := fs.ReadFile(fsys, "assets/"+f.Pop)
		if err != nil {
			return &FontError{Key: "fonts.pop", Err: err}
		}
		if err := gfx.LoadFonts(reg, pop); err != nil {
			return &FontError{Key: "fonts.pop", Err: fmt.Errorf("assets/%s: %w", f.Pop, err)}
		}
	}
	gfx.SetScale(UIScale(&db.Game))
	return nil
}

// StreamCaptionMax is how many lines a stream caption takes before it hides the stage
// (gozen check warns of a longer line).
const StreamCaptionMax = 2

// StreamCaptionLines is how many lines a streamer's line takes in the watch view's caption.
func StreamCaptionLines(gm *data.Game, text string) int {
	return len(gfx.Wrap(text, captionSize, (logicalWidth(gm)-streamChatW)*captionShare))
}
