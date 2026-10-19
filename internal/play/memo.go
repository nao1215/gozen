package play

import (
	"cmp"
	"fmt"
	"image"
	"log"

	"path/filepath"

	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/layer1/memo"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/text"
)

const DefaultMemoDir = memo.DefaultDir

type memoBox struct {
	kinds     []data.MemoKind
	list      *list
	kind      int
	typed     bool
	text      string
	comp      string
	age       int
	committed bool

	composer textinput.Composer
	ext      chan extInput
	shot     *image.NRGBA
	info     memo.Memo
	save     []byte
}

func (g *Game) memoKinds() []data.MemoKind {
	if ks := g.db.Game.Memo.Kinds; len(ks) > 0 {
		return ks
	}
	return []data.MemoKind{{ID: "note", Label: lang.T("メモ")}}
}

type extInput struct {
	text string
	ok   bool
}

func (g *Game) openMemo() {
	m := &memoBox{kinds: g.memoKinds(), info: g.memoInfo()}
	m.save = g.memoResume(&m.info)
	var items []listItem
	for _, k := range m.kinds {
		items = append(items, listItem{label: k.Label, id: k.ID})
	}
	m.list = newList(items, 0, 0, 420, len(items))
	m.composer.OnNewSession = func() *textinput.SessionOptions {
		x, y := g.memoCaret()
		return &textinput.SessionOptions{CaretBounds: image.Rect(int(x*g.ui), int(y*g.ui), int(x*g.ui)+2, int((y+sizeM*1.3)*g.ui)), TextBeforeCaret: m.text}
	}
	m.composer.OnComposition = func(c *textinput.Composition) { m.comp = c.Text() }
	m.composer.OnCommit = func(c *textinput.Commit) {
		m.committed = true
		if before, after := c.IsSurroundingTextReplaced(); before || after {
			b, a := c.SurroundingText()
			m.text = b + c.Text() + a
			return
		}
		m.text += c.Text()
	}
	g.memo = m
	g.sfx("decide")
}

func shownPicture(c data.CG) string {
	if len(c.Files) > 0 {
		i := max(1, c.Page) - 1
		if i >= len(c.Files) {
			i = len(c.Files) - 1
		}
		return c.Files[i]
	}
	return c.File
}

func (g *Game) memoInfo() memo.Memo {
	now := time.Now()
	m := memo.Memo{Source: memo.Human, Time: now.Format(time.RFC3339), Tick: g.tick, Seed: g.opts.Seed, Game: g.db.Game.Title, Language: cmp.Or(g.db.Game.Language, lang.English)}
	if !g.started.IsZero() {
		m.Seconds = float64(now.Sub(g.started).Milliseconds()) / 1000
	}
	for _, s := range g.stack {
		m.Scenes = append(m.Scenes, sceneName(s))
	}
	if g.talk.active {
		m.Who, m.Line = g.talk.say.Name, g.talk.say.Text
		if g.talk.choice != nil {
			for _, it := range g.talk.choice.items {
				m.Choices = append(m.Choices, it.label)
			}
		}
	} else if n := len(g.backlog); n > 0 {
		m.Who, m.Line = g.backlog[n-1].name, g.backlog[n-1].text
	}
	m.Event = g.currentEvent()
	if g.currentBattle() != nil && g.tr != nil && g.tr.enc != nil && g.tr.battles > 0 {
		m.Data = map[string]any{"battle_capture": g.tr.battles}
	}
	m.Song = g.baseSong
	m.Picture = shownPicture(g.cg)
	st := g.st
	if st == nil {
		return m
	}
	m.Play = float64(st.Frames) / 60
	m.Location = st.Location
	if loc := g.db.Locations[st.Location]; loc != nil {
		m.Place = loc.Name
		if loc.Dungeon != nil {
			if f, ok := st.Vars[dungeon.VarFloor].(float64); ok {
				m.Floor = f
			}
			m.Pos, m.Facing = []int{st.Pos.X, st.Pos.Y}, data.Facings[st.Pos.Facing&3]
			if d := dungeon.Of(g.db, st); d != nil {
				for y, row := range d.Grid {
					m.Map = append(m.Map, string(row))
					seen := make([]byte, len(row))
					for x := range row {
						seen[x] = '0'
						if dungeon.Seen(g.db, st, st.Location, x, y) {
							seen[x] = '1'
						}
					}
					m.Seen = append(m.Seen, string(seen))
				}
			}
		}
	}
	for _, v := range g.db.Game.Memo.Vars {
		if val, err := v.Value.Eval(st.Env()); err == nil {
			if m.Vars == nil {
				m.Vars = map[string]any{}
			}
			m.Vars[v.Name] = val
		}
	}
	for _, a := range st.Members() {
		m.Party = append(m.Party, fmt.Sprintf("%s Lv%d HP%d/%d", a.Name(), a.Level, a.HP, a.Stat("mhp")))
	}
	return m
}

func (g *Game) currentEvent() string {
	if g.events == nil {
		return ""
	}
	frames, _ := g.events.Frames()
	for i := len(frames) - 1; i >= 0; i-- {
		if name := g.commandsName(frames[i]); name != "" {
			return name
		}
	}
	return ""
}

func (g *Game) knownCommands(f func(name string, cmds []data.Command) bool) {
	for id, ev := range g.db.Events {
		if f(id, ev.Do) {
			return
		}
	}
	for id, loc := range g.db.Locations {
		for _, a := range loc.Actions {
			if f(id+"."+a.Label, a.Do) {
				return
			}
		}
		for _, p := range loc.People {
			if f(id+"."+p.Name, p.Talk) {
				return
			}
		}
		if loc.Dungeon != nil {
			for c, t := range loc.Dungeon.Tiles {
				if t != nil && f(id+".tile."+c, t.Do) {
					return
				}
			}
		}
	}
}

func (g *Game) commandsName(cmds []data.Command) string {
	name := ""
	g.knownCommands(func(n string, c []data.Command) bool {
		if len(c) > 0 && len(cmds) > 0 && &c[0] == &cmds[0] {
			name = n
			return true
		}
		return false
	})
	return name
}

func (g *Game) commandsNamed(name string) []data.Command {
	var out []data.Command
	g.knownCommands(func(n string, c []data.Command) bool {
		if n == name {
			out = c
			return true
		}
		return false
	})
	return out
}

func (g *Game) onEventStart(cmds []data.Command) {
	g.evLines, g.evAnswers = 0, nil
	g.evName = g.commandsName(cmds)
	g.evSnap, g.evRand = nil, nil
	if g.st == nil {
		return
	}
	var err error
	g.evSnap, err = g.st.Save()
	if err != nil {
		g.failure = err
		return
	}
	if g.pcg != nil {
		r, err := g.pcg.MarshalBinary()
		if err != nil {
			g.failure = err
			return
		}
		g.evRand = r
	}
}

func (g *Game) memoResume(info *memo.Memo) []byte {
	if g.opts.CollectBuildInfo {
		info.Trace = filepath.Base(g.opts.TracePath)
	}
	info.Version = memo.Version(g.opts.CollectBuildInfo)
	if g.st == nil {
		return nil
	}
	now := func() []byte {
		b, err := g.st.Save()
		if err != nil {
			g.failure = err
			return nil
		}
		if g.pcg != nil {
			r, err := g.pcg.MarshalBinary()
			if err != nil {
				g.failure = err
				return nil
			}
			info.Rand = r
		}
		return b
	}
	inEvent := g.events != nil && (g.events.Running() || g.req != nil)
	b := g.currentBattle()
	switch {
	case inEvent && g.evSnap != nil && g.evName != "":
		info.Resume = &memo.Resume{From: "event", Event: g.evName, Line: g.evLines, Answers: g.evAnswers, InBattle: b != nil,
			Note: lang.T("イベントの始まりから始め直し、メモの台詞まで読み進める")}
		info.Rand = g.evRand
		return g.evSnap
	case inEvent && g.evSnap != nil:
		info.Resume = &memo.Resume{From: "arrival", Line: g.evLines, Answers: g.evAnswers, InBattle: b != nil,
			Note: lang.T("名前の分からないイベント（場所に着いた時など）の始まりの状態から。着いた時のイベントはそこから始まる")}
		info.Rand = g.evRand
		return g.evSnap
	case b != nil:
		info.Resume = &memo.Resume{From: "battle", Battle: b.troop, Note: lang.T("メモの時の状態で同じ戦いを始める（戦いの途中の HP は戻らない）")}
		return now()
	}
	info.Resume = &memo.Resume{From: "here", Note: lang.T("メモの瞬間の状態そのまま")}
	return now()
}

func (g *Game) updateMemo() {
	m := g.memo
	m.age++
	in := &g.in
	if !m.typed {
		if m.age < 4 {
			return
		}
		chose, cancel := m.list.update(g)
		switch {
		case cancel:
			g.closeMemo()
		case chose:
			m.kind, m.typed = m.list.sel, true
			g.sfx("decide")
			if in.script == nil {
				ch := make(chan extInput, 1)
				if g.opts.ExternalInput && externalEntry(g.context, lang.T("テストのメモ"), lang.T("一言（空でもよい。OK で保存、取消でやめる）")+"\n"+m.kinds[m.kind].Label, ch) {
					m.ext = ch
					g.keepRunningUnfocused()
				}
			}
		}
		return
	}
	if m.ext != nil {
		select {
		case r := <-m.ext:
			m.ext = nil
			if !r.ok {
				g.closeMemo()
				return
			}
			m.text = r.text
			g.saveMemo()
		default:
		}
		return
	}
	scripted := in.script != nil
	if !scripted {
		handled, err := m.readInput(m.composer.Update, ebiten.AppendInputChars)
		if err != nil {
			log.Print(err)
		}
		if handled {
			return
		}
	}
	save := in.script != nil && in.pressed[btnOK]
	cancel := in.script != nil && in.cancel()
	if !scripted {
		save = inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)
		cancel = inpututil.IsKeyJustPressed(ebiten.KeyEscape)
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && m.text != "" {
			m.composer.Confirm()
			_, n := utf8.DecodeLastRuneInString(m.text)
			m.text = m.text[:len(m.text)-n]
		}
	}
	switch {
	case cancel:
		m.composer.Cancel()
		g.closeMemo()
	case save:
		m.composer.Confirm()
		g.saveMemo()
	}
}

// readInput は変換確定を優先し、通常文字の経路との重複を避ける。
// 文字を受け取ったフレームでは Enter・Backspace を保存・削除に使わない。
func (m *memoBox) readInput(update func() (bool, error), chars func([]rune) []rune) (bool, error) {
	m.committed = false
	handled, err := update()
	if handled || m.committed || m.comp != "" {
		return true, err
	}
	if rs := chars(nil); len(rs) > 0 {
		m.text += string(rs)
		return true, err
	}
	return false, err
}

func (g *Game) closeMemo() {
	g.sfx("cancel")
	g.memo = nil
}

func (g *Game) typeMemo(s string) {
	if g.memo != nil && g.memo.typed {
		g.memo.text += s
	}
	if r := g.reviewOpen(); g.memo == nil && r != nil && r.box != nil {
		r.box.text += s
	}
}

func (g *Game) saveMemo() {
	m := g.memo
	g.memo = nil
	k := m.kinds[m.kind]
	m.info.Kind, m.info.Label, m.info.Text = k.ID, k.Label, strings.TrimSpace(m.text)
	root := g.opts.MemoDir
	if root == "" {
		root = DefaultMemoDir
	}
	path, err := g.writeMemo(root, m)
	if err != nil {
		log.Printf("memo: %v", err)
		g.memoToast, g.memoToastT = lang.T("メモを保存できなかった"), 150
		return
	}
	g.memoToast, g.memoToastT = fmt.Sprintf(lang.T("メモを残した（%s）"), filepath.Base(path)), memoToastTicks
	g.sfx("decide")
}

const memoToastTicks = 90

func (g *Game) keepRunningUnfocused() {
	if !ebiten.IsRunnableOnUnfocused() {
		ebiten.SetRunnableOnUnfocused(true)
		g.memoUnfocused = true
	}
}

func (g *Game) stepMemoFocus() {
	if g.memoUnfocused && g.memo == nil && g.memoToastT == 0 {
		ebiten.SetRunnableOnUnfocused(false)
		g.memoUnfocused = false
	}
}

func (g *Game) writeMemo(root string, m *memoBox) (string, error) {
	info := &m.info
	base, err := memo.Next(root, info)
	if err != nil {
		return "", err
	}
	var shot image.Image
	if m.shot != nil {
		shot = m.shot
	}
	path, err := memo.WriteAt(base, info, shot, m.save)
	if err != nil {
		return "", err
	}
	g.emit("memo", traceLine{"kind": info.Kind, "text": info.Text, "file": path, "source": info.Source, "location": info.Location, "event": info.Event})
	return path, nil
}

func (g *Game) quickMemo(kind, text, source string) {
	if g.opts.MemoDir == "" || g.db == nil || g.quickN >= 40 {
		return
	}
	g.quickN++
	m := &memoBox{info: g.memoInfo()}
	m.save = g.memoResume(&m.info)
	m.info.Source = source
	m.info.Kind, m.info.Label = kind, kind
	if kind == memo.Bug {
		m.info.Label = lang.T("不具合")
	}
	for _, k := range g.memoKinds() {
		if k.ID == kind || k.Label == kind {
			m.info.Kind, m.info.Label = k.ID, k.Label
		}
	}
	m.info.Text = text
	g.quick = append(g.quick, m)
}

func (g *Game) drawQuickMemos(screen *ebiten.Image) {
	for _, m := range g.quick {
		b := screen.Bounds()
		m.shot = image.NewNRGBA(b)
		screen.ReadPixels(m.shot.Pix)
		if _, err := g.writeMemo(g.opts.MemoDir, m); err != nil {
			log.Printf("memo: %v", err)
		}
	}
	g.quick = nil
}

func (g *Game) memoBoxRect() (x, y, w, h float64) {
	sw, sh := g.screen()
	w, h = 760, 226+float64(len(g.memo.kinds))*44
	return sw/2 - w/2, sh/2 - h/2, w, h
}

func (g *Game) memoCaret() (x, y float64) {
	bx, by, _, bh := g.memoBoxRect()
	return bx + 40 + gfx.TextWidth(g.memo.text, sizeM), by + bh - 102
}

func (g *Game) drawMemo(screen *ebiten.Image) {
	m := g.memo
	if m.shot == nil {
		b := screen.Bounds()
		m.shot = image.NewNRGBA(b)
		screen.ReadPixels(m.shot.Pix)
	}
	fill(screen, withAlpha(colorBlack, 0xa0))
	x, y, w, h := g.memoBoxRect()
	g.window(screen, x, y, w, h)
	g.text(screen, lang.T("テストのメモ"), x+32, y+20, sizeL, g.th.accent)
	where := m.info.Place
	if m.info.Pos != nil {
		where += fmt.Sprintf(lang.T(" %d階 (%d,%d)"), int(m.info.Floor), m.info.Pos[0], m.info.Pos[1])
	}
	gfx.DrawTextRight(screen, where, x+w-32, y+28, sizeS, g.th.dim)
	m.list.x, m.list.y, m.list.w = x+24, y+70, w-48
	m.list.drawRows(g, screen, !m.typed)
	if m.typed {

		r := m.list.rowRect(m.kind)
		g.text(screen, "●", float64(r.Min.X)+16, float64(r.Min.Y)+8, sizeS, g.th.cursor)
	}
	fy := y + h - 112
	gfx.FillRoundRect(screen, float32(x+28), float32(fy), float32(w-56), 52, 8, withAlpha(colorBlack, 0x90))
	s := m.text + m.comp
	room := w - 80
	for s != "" && gfx.TextWidth(s, sizeM) > room {
		_, n := utf8.DecodeRuneInString(s)
		s = s[n:]
	}
	switch {
	case !m.typed:
		g.text(screen, lang.T("（種類を選ぶと一言を書ける）"), x+40, fy+10, sizeM, g.th.dim)
	case m.ext != nil:
		g.text(screen, lang.T("（別の窓で一言を書いている）"), x+40, fy+10, sizeM, g.th.dim)
	default:
		g.text(screen, s, x+40, fy+10, sizeM, g.th.text)
		if m.comp != "" {
			cw := gfx.TextWidth(m.comp, sizeM)
			tw := gfx.TextWidth(s, sizeM)
			rect(screen, x+40+tw-cw, fy+44, cw, 2, g.th.text)
		}
		if m.age/30%2 == 0 {
			rect(screen, x+42+gfx.TextWidth(s, sizeM), fy+10, 2, sizeM*1.2, g.th.cursor)
		}
	}
	help := lang.T("↑↓で選んで決定　Escでやめる")
	if m.typed {
		help = lang.T("一言を書いてEnterで保存（空でもよい）　Escでやめる")
	}
	if m.ext != nil {
		help = lang.T("別の窓で書いて OK で保存、取消でやめる（窓が見えなければ Alt+Tab）")
	}
	g.text(screen, help, x+32, y+h-46, sizeS, g.th.dim)
}

func (g *Game) drawMemoToast(screen *ebiten.Image) {
	if g.memoToastT <= 0 {
		return
	}
	sw, _ := g.screen()
	w := gfx.TextWidth(g.memoToast, sizeS) + 40
	a := uint8(min(255, g.memoToastT*8))
	x := sw - w - 24
	gfx.FillRoundRect(screen, float32(x), 24, float32(w), 44, 10, withAlpha(colorBlack, a*3/4))
	gfx.DrawText(screen, g.memoToast, x+w/2, 32, sizeS, withAlpha(g.th.text, a))
}
