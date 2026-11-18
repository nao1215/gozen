package play

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math"
	"strings"
	"time"

	lang "github.com/nao1215/gozen/internal/base/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/screen/gfx"
	"github.com/nao1215/gozen/internal/screen/picdecode"
)

type saveMenu struct {
	saving bool
	inPlay bool
	slots  *list
	// index maps a row to its slot: the load screen lists the quick slot only while the
	// game offers it or a quick save exists.
	index []int
	done  func()
	msg   string
	msgT  int
}

func newSaveMenu(g *Game, saving bool, done func()) *saveMenu {
	s := &saveMenu{saving: saving, done: done}
	s.refresh(g)
	return s
}

func (s *saveMenu) overlay() bool { return true }

// modal draws the save screen over the novel window and the stage: opened from the
// novel band, it was drawn under the portraits standing in front of the scene.
func (s *saveMenu) modal() bool { return true }

// Rows of the save screen: a row with a picture of the save is taller, so the picture
// reads as the scene it is.
const (
	saveRowH     = 60.0
	saveRowPicH  = 96.0
	saveListTop  = 110.0
	saveListRoom = 24 + 20 + 64 + 16 // the window's pad, the gap and the status window
)

func (s *saveMenu) refresh(g *Game) {
	s.index = s.index[:0]
	for i := range g.saveSlots() {
		s.index = append(s.index, i)
	}
	if !s.saving {
		s.index = append(s.index, g.autoSlot())
		if g.db.Game.UI.QuickSave() || g.storeHas(g.slotPath(g.quickSlot())) {
			s.index = append(s.index, g.quickSlot())
		}
	}
	var items []listItem
	pics := false
	for _, i := range s.index {
		label, ok := g.slotLabel(i)
		it := listItem{label: label, disabled: !s.saving && !ok}
		if ok {
			it.pic = g.slotPics.picture(g, g.slotPath(i))
			pics = pics || it.pic != nil
		}
		items = append(items, it)
	}
	sw, sh := g.screen()
	sel := 0
	if s.slots != nil {
		sel = s.slots.sel
	} else if !s.saving {
		newest, _ := g.newestSave()
		for row, i := range s.index {
			if i == newest {
				sel = row
			}
		}
	}
	// A game that keeps pictures lays the rows out for them from its first save on, so
	// the first picture does not move the rows.
	pics = pics || g.slotPics.keeps(g)
	width := math.Min(800, sw-32)
	if pics {
		width = math.Min(1120, sw-32)
	}
	_, _, titleGap := s.heading(width)
	y := math.Max(sh/2-saveListTop, 16+titleGap)
	n := len(items)
	want := saveRowH
	if pics {
		want = saveRowPicH
		y = math.Max(16+titleGap, math.Min(y, sh*0.2))
	}
	// Reserve the status window even before a write, so saving does not move
	// the selected slot and short landscape views retain every row.
	room := sh - y - saveListRoom
	rows := max(1, min(n, int(room/want)))
	rowHeight := math.Min(want, room/float64(rows))
	s.slots = newList(items, (sw-width)/2, y, width, rows)
	s.slots.rowH = math.Max(24, rowHeight)
	s.slots.size = math.Min(sizeM, s.slots.rowH/1.5)
	if pics {
		s.slots.size = math.Min(sizeM, saveRowH/1.5)
		s.slots.picW = (s.slots.rowH - 12) * 16 / 9
	}
	s.slots.sel = min(sel, n-1)
	s.slots.scroll()
}

func (s *saveMenu) update(g *Game) {
	if s.msgT > 0 {
		s.msgT--
		if s.msgT == 0 || g.in.advance() {
			s.msgT = 0
			g.leave(s.done)
		}
		return
	}
	chose, cancel := s.slots.update(g)
	if cancel {
		g.leave(s.done)
		return
	}
	if !chose {
		return
	}
	i := s.index[s.slots.sel]
	if s.saving {
		if g.storeHas(g.slotPath(i)) {
			g.push(newConfirm(fmt.Sprintf(lang.T("スロット%sに 上書きしますか？"), g.slotName(i)), func() { s.write(g, i) }))
			return
		}
		s.write(g, i)
		return
	}
	st, err := g.load(i)
	if err != nil {
		s.msg = lang.T("ロードできなかった: ") + err.Error()
		s.msgT = 90
		return
	}
	if s.inPlay {
		g.push(newConfirm(fmt.Sprintf(lang.T("スロット%sを ロードしますか？（セーブしていない 進み具合は 消えます）"), g.slotName(i)), func() { g.loadSavedGame(st) }))
		return
	}
	g.loadSavedGame(st)
}

func (s *saveMenu) write(g *Game, i int) {
	if err := g.save(i); err != nil {
		s.msg = lang.T("セーブできなかった: ") + err.Error()
	} else {
		s.msg = fmt.Sprintf(lang.T("スロット%sに セーブしました。"), g.slotName(i))
		g.sfx("item")
	}
	s.refresh(g)
	s.msgT = 80
}

// heading fits the prompt above the slots without changing ordinary layouts.
func (s *saveMenu) heading(width float64) ([]string, float64, float64) {
	title := lang.T("どこに セーブしますか？")
	if !s.saving {
		title = lang.T("どの きろくから はじめますか？")
	}
	lines, size := gfx.FitLines(title, sizeL, width, 2)
	return lines, size, math.Max(70, float64(len(lines))*gfx.LineHeight(size)+16)
}

func (s *saveMenu) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0x90))
	lines, size, gap := s.heading(s.slots.w)
	for i, line := range lines {
		gfx.DrawTextOutlineColor(dst, line, sw/2, s.slots.y-gap+float64(i)*gfx.LineHeight(size), size, g.th.text, g.th.window, 1)
	}
	s.slots.draw(g, dst, s.msgT == 0)
	if s.msgT > 0 {
		width := math.Min(600, s.slots.w)
		y := s.slots.y + s.slots.height() + 20
		g.window(dst, (sw-width)/2, y, width, 64)
		lines, size := gfx.FitLines(s.msg, sizeM, width-32, 2)
		top := y + (64-float64(len(lines))*gfx.LineHeight(size))/2
		// Preserve the established single-line baseline on ordinary screens.
		if len(lines) == 1 && size == sizeM {
			top = y + 16
		}
		for i, line := range lines {
			gfx.DrawText(dst, line, sw/2, top+float64(i)*gfx.LineHeight(size), size, g.th.text)
		}
	}
}

// quickSave writes the quick slot without a menu (ui.saves.quick) and says so over the
// scene; a scene that refuses saving says why instead.
func (g *Game) quickSave() {
	if why := g.saveBlocked(); why != "" {
		g.sfx("buzzer")
		g.slotPics.say(why)
		return
	}
	if err := g.save(g.quickSlot()); err != nil {
		g.sfx("buzzer")
		g.slotPics.say(lang.T("セーブできなかった: ") + err.Error())
		return
	}
	g.sfx("item")
	g.slotPics.say(lang.T("クイックセーブしました。"))
}

// quickLoad reads the quick slot back after one question, as loading from the menu does.
func (g *Game) quickLoad() {
	st, err := g.load(g.quickSlot())
	if err != nil {
		g.sfx("buzzer")
		if g.storeHas(g.slotPath(g.quickSlot())) {
			g.slotPics.say(lang.T("ロードできなかった: ") + err.Error())
		} else {
			g.slotPics.say(lang.T("クイックセーブが ありません。"))
		}
		return
	}
	g.push(newConfirm(lang.T("クイックセーブを ロードしますか？（セーブしていない 進み具合は 消えます）"), func() { g.loadSavedGame(st) }))
}

// saveShots keeps a small copy of the last frame the player saw with nothing over the
// scene, so a save made from a menu shows the scene rather than the menu, and the
// pictures the save screen shows. It also carries the short note a quick save leaves.
type saveShots struct {
	frame *ebiten.Image
	// pics are the decoded pictures of the slots, by the save's file and its time, so
	// the save screen decodes a picture once and again only after a new save.
	pics  map[string]savePic
	note  string
	noteT int
}

type savePic struct {
	at  time.Time
	img *ebiten.Image
}

// The picture of a save is 16:9, 320 pixels wide: enough for a row of the save screen
// at 3840x2160, small enough for a browser's storage.
const (
	shotW = 320
	shotH = 180
)

// keep copies the frame when nothing covers the scene, then draws a quick save's note
// over it. It runs at the end of Draw, once a frame; a trial run that keeps no save
// skips the copy.
func (s *saveShots) keep(g *Game, screen *ebiten.Image) {
	if s.uncovered(g) {
		s.copy(screen)
	}
	if s.noteT > 0 {
		s.drawNote(g, screen)
		s.noteT--
	}
}

// keeps reports whether the game keeps a picture of its saves. Pixels can be read back
// only inside the Ebitengine loop (a game drawn offscreen by a test has none), and a trial
// run that keeps no save keeps no picture.
func (s *saveShots) keeps(g *Game) bool { return g.loopStarted && (!g.testRun() || g.storeGiven) }

func (s *saveShots) uncovered(g *Game) bool {
	if !s.keeps(g) || g.logView != nil || g.talk.hidden || len(g.stack) == 0 {
		return false
	}
	// A screen over the scene (a menu, the character book) is not the scene the save
	// returns to.
	top := g.stack[len(g.stack)-1]
	if o, ok := top.(overlay); ok && o.overlay() {
		return false
	}
	m, ok := top.(modal)
	return !ok || !m.modal()
}

func (s *saveShots) copy(screen *ebiten.Image) {
	if s.frame == nil {
		s.frame = ebiten.NewImage(shotW, shotH)
	}
	b := screen.Bounds()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(shotW/float64(b.Dx()), shotH/float64(b.Dy()))
	s.frame.Clear()
	s.frame.DrawImage(screen, op)
}

// write stores the picture of the save path beside it (shotName). A save
// with no frame yet (nothing drawn) has no picture; an old picture is removed so it does
// not stand for the new save.
func (s *saveShots) write(g *Game, path string) {
	name := shotName(path)
	delete(s.pics, name)
	if s.frame == nil {
		s.drop(g, name)
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, shotW, shotH))
	s.frame.ReadPixels(img.Pix)
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70})
	if err == nil {
		// A browser keeps saves as text, so the picture is written as base64, as the
		// phone's packages are.
		err = g.store.Write(name, []byte(base64.StdEncoding.EncodeToString(buf.Bytes())))
	}
	if err != nil {
		log.Printf("save picture %s: %v", name, err)
		s.drop(g, name)
	}
}

// drop removes the picture of a save, so the row goes without one rather than show an
// older save's. The save itself is written already.
func (s *saveShots) drop(g *Game, name string) {
	if err := g.store.Remove(name); err != nil {
		log.Printf("save picture %s: %v", name, err)
	}
}

// picture is the picture of the save at path, or nil.
func (s *saveShots) picture(g *Game, path string) *ebiten.Image {
	name := shotName(path)
	at, _ := g.storeTime(path)
	if p, ok := s.pics[name]; ok && p.at.Equal(at) {
		return p.img
	}
	var img *ebiten.Image
	if b, err := g.storeRead(name); err == nil {
		if raw, err := base64.StdEncoding.DecodeString(string(b)); err == nil {
			if pic, err := picdecode.Decode(raw); err == nil {
				img = ebiten.NewImageFromImage(pic)
			}
		}
	}
	if s.pics == nil {
		s.pics = map[string]savePic{}
	}
	s.pics[name] = savePic{at, img}
	return img
}

// shotName is the file of the picture of the save at path: save1.json → save1.jpg.b64.
func shotName(path string) string { return strings.TrimSuffix(path, ".json") + ".jpg.b64" }

func (s *saveShots) say(text string) { s.note, s.noteT = text, 90 }

func (s *saveShots) drawNote(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	lines, size := gfx.FitLines(s.note, sizeM, math.Min(600, sw-32)-32, 2)
	w := 32.0
	for _, l := range lines {
		w = math.Max(w, gfx.TextWidth(l, size)+48)
	}
	h := float64(len(lines))*gfx.LineHeight(size) + 28
	g.window(dst, (sw-w)/2, 24, w, h)
	for i, l := range lines {
		gfx.DrawText(dst, l, sw/2, 38+float64(i)*gfx.LineHeight(size), size, g.th.text)
	}
}
