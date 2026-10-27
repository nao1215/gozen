package play

import (
	"image"
	"image/color"
	"log"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// textField is a line typed with the keyboard or an input method: the memo's line and
// the stream's comment and name.
type textField struct {
	text, comp string
	committed  bool
	composer   textinput.Composer
}

// start connects the input method; caret is where the caret is, in UI units, for the
// input method's window.
func (f *textField) start(g *Game, caret func() (x, y float64)) {
	f.composer.OnNewSession = func() *textinput.SessionOptions {
		x, y := caret()
		return &textinput.SessionOptions{CaretBounds: image.Rect(int(x*g.ui), int(y*g.ui), int(x*g.ui)+2, int((y+sizeM*1.3)*g.ui)), TextBeforeCaret: f.text}
	}
	f.composer.OnComposition = func(c *textinput.Composition) { f.comp = c.Text() }
	f.composer.OnCommit = func(c *textinput.Commit) {
		f.committed = true
		if before, after := c.IsSurroundingTextReplaced(); before || after {
			b, a := c.SurroundingText()
			f.text = b + c.Text() + a
			return
		}
		f.text += c.Text()
	}
}

// readTyping reads one tick of typing from the keyboard and the input method. It reports
// whether the tick's keys were text, when Enter and Escape are not the screen's.
// Backspace deletes the last character.
func (f *textField) readTyping() bool {
	handled, err := f.readInput(f.composer.Update, ebiten.AppendInputChars)
	if err != nil {
		log.Print(err)
	}
	if handled {
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && f.text != "" {
		f.composer.Confirm()
		_, n := utf8.DecodeLastRuneInString(f.text)
		f.text = f.text[:len(f.text)-n]
	}
	return false
}

// readInput gives priority to IME composition confirmation and avoids duplicating the plain character path.
// In a frame that received a character, Enter and Backspace are not used for save and delete.
func (f *textField) readInput(update func() (bool, error), chars func([]rune) []rune) (bool, error) {
	f.committed = false
	handled, err := update()
	if handled || f.committed || f.comp != "" {
		return true, err
	}
	if rs := chars(nil); len(rs) > 0 {
		f.text += string(rs)
		return true, err
	}
	return false, err
}

// take confirms the composition and returns the text, emptying the field.
func (f *textField) take() string {
	f.composer.Confirm()
	s := f.text
	f.text, f.comp = "", ""
	return s
}

// draw writes the text and the composition at x, y within room, its end kept in view,
// with the composition underlined and, when caret, a blinking caret.
func (f *textField) draw(dst *ebiten.Image, x, y, room, size float64, c, caretColor color.Color, caret bool, age int) {
	s := f.text + f.comp
	for s != "" && gfx.TextWidth(s, size) > room {
		_, n := utf8.DecodeRuneInString(s)
		s = s[n:]
	}
	gfx.DrawTextLeft(dst, s, x, y, size, c)
	tw := gfx.TextWidth(s, size)
	if f.comp != "" {
		cw := gfx.TextWidth(f.comp, size)
		rect(dst, x+tw-cw, y+size*1.4, cw, 2, c)
	}
	if caret && age/30%2 == 0 {
		rect(dst, x+2+tw, y, 2, size*1.2, caretColor)
	}
}

// caretX is where the caret is, for the input method's window.
func (f *textField) caretX(x, size float64) float64 { return x + gfx.TextWidth(f.text, size) }
