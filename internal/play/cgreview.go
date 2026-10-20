package play

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

const DefaultReviewLog = "picture-review.jsonl"

type reviewItem struct {
	page  string
	slot  string
	title string
	src   string
	group int
}

func (it reviewItem) key() string {
	if it.src != "" {
		return it.src
	}
	return it.page
}

type reviewLine struct {
	Time  string `json:"time"`
	Src   string `json:"src,omitempty"`
	Page  string `json:"page"`
	Slot  string `json:"slot"`
	Tag   string `json:"tag"`
	Label string `json:"label"`
	OK    bool   `json:"ok,omitempty"`
	Off   bool   `json:"off,omitempty"`
	Note  string `json:"note,omitempty"`
}

const reviewNoteTag = "note"

type cgReview struct {
	all     []reviewItem
	items   []reviewItem
	at      int
	viewing bool
	menu    *list
	byName  bool
	skipOK  bool
	skipAny bool
	groups  []bool
	marks   map[string]map[string]bool
	inPlay  bool
	notes   map[string]string
	box     *reviewBox
	hide    bool
	toast   string
	toastT  int
	started bool
}

type reviewBox struct {
	list *list
	text string
	ext  chan extInput
	ag   int
}

func (g *Game) cgReviewEnabled() bool {
	return g.opts.ReviewLog != "" && (g.opts.Review || g.db.Game.CGReview.Configured())
}

func newCGReview(g *Game) *cgReview {
	r := &cgReview{skipOK: true, groups: make([]bool, len(g.db.Game.CGReview.Groups))}
	r.all = reviewPages(g)
	r.load(g)
	r.buildMenu(g)
	return r
}

func (r *cgReview) takesMemoKey() bool { return true }

func reviewPages(g *Game) []reviewItem {
	srcs := loadReviewSources(g.opts.ReviewSources)
	cfg := g.db.Game.CGReview
	seen := map[string]bool{}
	var out []reviewItem
	add := func(page, title string) {
		if page == "" || seen[page] || g.db.StandIn(page) || strings.HasPrefix(srcs[page], "stand_in:") {
			return
		}
		if _, err := fs.Stat(g.db.FS, "assets/"+page); err != nil {
			return
		}
		seen[page] = true
		it := reviewItem{page: page, slot: reviewSlot(page), title: title, src: srcs[page], group: -1}
		for i, gr := range cfg.Groups {
			if data.MatchAny(gr.Files, page) {
				it.group = i
				break
			}
		}
		out = append(out, it)
	}
	for _, c := range g.db.Game.Gallery.CGs {
		for _, p := range c.Pages(g.db.FS) {
			add(p, c.Title)
		}
	}
	rest := make([]string, 0, len(srcs))
	for p := range srcs {
		rest = append(rest, p)
	}
	sort.Strings(rest)
	for _, p := range rest {
		add(p, "")
	}
	for _, pat := range cfg.Extra {
		m, err := fs.Glob(g.db.FS, "assets/"+pat)
		if err != nil {
			g.failure = err
			return nil
		}
		sort.Strings(m)
		for _, p := range m {
			add(strings.TrimPrefix(p, "assets/"), "")
		}
	}
	return out
}

func reviewSlot(page string) string {
	b := path.Base(data.PageBase(page))
	return strings.TrimSuffix(b, path.Ext(b))
}

func loadReviewSources(file string) map[string]string {
	out := map[string]string{}
	if file == "" {
		return out
	}
	b, err := os.ReadFile(file) //nolint:gosec // The host explicitly selects this optional review source file.
	if err != nil {
		log.Printf("review sources: %v", err)
		return out
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Printf("review sources: %v", err)
		return out
	}
	for k, v := range raw {
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = s
			continue
		}
		var o struct {
			Src string `json:"src"`
		}
		if json.Unmarshal(v, &o) == nil {
			out[k] = o.Src
		}
	}
	return out
}

func (g *Game) reviewLog() string {
	if g.opts.ReviewLog != "" {
		return g.opts.ReviewLog
	}
	return DefaultReviewLog
}

func (r *cgReview) load(g *Game) {
	r.marks, r.notes = map[string]map[string]bool{}, map[string]string{}
	f, err := os.Open(g.reviewLog())
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			g.failure = err
		}
	}()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var l reviewLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		r.apply(l)
	}
}

func (r *cgReview) apply(l reviewLine) {
	key := l.Src
	if key == "" {
		key = l.Page
	}
	if l.Tag == reviewNoteTag {
		r.notes[key] = l.Note
		return
	}
	m := r.marks[key]
	if m == nil {
		m = map[string]bool{}
		r.marks[key] = m
	}
	if l.Off {
		delete(m, l.Tag)
	} else {
		m[l.Tag] = true
	}
}

func (r *cgReview) okTag(g *Game) string {
	for _, t := range g.db.Game.CGReview.Tags {
		if t.OK {
			return t.ID
		}
	}
	return ""
}

func (r *cgReview) filter(g *Game) []reviewItem {
	ok := r.okTag(g)
	var out []reviewItem
	for _, it := range r.all {
		if it.group >= 0 && !r.groups[it.group] {
			continue
		}
		m := r.marks[it.key()]
		if r.skipOK && ok != "" && m[ok] {
			continue
		}
		if r.skipAny && len(m) > 0 {
			continue
		}
		out = append(out, it)
	}
	if r.byName {
		sort.SliceStable(out, func(i, j int) bool { return out[i].page < out[j].page })
	}
	return out
}

const (
	reviewRowStart = iota
	reviewRowResume
	reviewRowOrder
	reviewRowSkipOK
	reviewRowSkipAny
)

func (r *cgReview) rows() []int {
	rows := []int{reviewRowStart}
	if r.started && r.at > 0 && r.at < len(r.items) {
		rows = append(rows, reviewRowResume)
	}
	return append(rows, reviewRowOrder, reviewRowSkipOK, reviewRowSkipAny)
}

func (r *cgReview) buildMenu(g *Game) {
	sel := 0
	if r.menu != nil {
		sel = r.menu.sel
	}
	yes := func(b bool) string {
		if b {
			return lang.T("する")
		}
		return lang.T("しない")
	}
	n := len(r.filter(g))
	var items []listItem
	for _, row := range r.rows() {
		switch row {
		case reviewRowStart:
			items = append(items, listItem{label: lang.T("はじめから流し見る"), right: fmt.Sprintf(lang.T("%d 枚"), n), disabled: n == 0})
		case reviewRowResume:
			items = append(items, listItem{label: lang.T("続きから"), right: fmt.Sprintf("%d / %d", r.at+1, len(r.items))})
		case reviewRowOrder:
			o := lang.T("場面の順")
			if r.byName {
				o = lang.T("名前の順")
			}
			items = append(items, listItem{label: lang.T("並び"), right: o})
		case reviewRowSkipOK:
			items = append(items, listItem{label: lang.T("OK 済みを除く"), right: yes(r.skipOK)})
		case reviewRowSkipAny:
			items = append(items, listItem{label: lang.T("判定済み（札のある絵）を除く"), right: yes(r.skipAny)})
		}
	}
	for _, gr := range g.db.Game.CGReview.Groups {
		items = append(items, listItem{label: fmt.Sprintf(lang.T("「%s」を入れる"), gr.Label)})
	}
	for i := range g.db.Game.CGReview.Groups {
		items[len(r.rows())+i].right = yes(r.groups[i])
	}
	items = append(items, listItem{label: lang.T("もどる")})
	sw, _ := g.screen()
	r.menu = newList(items, sw/2-380, 170, 760, len(items))
	r.menu.sel = min(sel, len(items)-1)
}

func (r *cgReview) update(g *Game) {
	if r.toastT > 0 {
		r.toastT--
	}
	if r.box != nil {
		r.updateBox(g)
		return
	}
	if r.viewing {
		r.updateView(g)
		return
	}
	rows := r.rows()
	toggle := g.in.left() || g.in.right()
	chose, cancel := r.menu.update(g)
	sel := r.menu.sel
	if cancel || (chose && sel == len(r.menu.items)-1) {
		g.pop()
		return
	}
	if !chose && !toggle {
		return
	}
	if sel < len(rows) {
		switch rows[sel] {
		case reviewRowStart:
			if !chose {
				return
			}
			r.items = r.filter(g)
			if len(r.items) == 0 {
				return
			}
			r.at, r.viewing, r.started = 0, true, true
			r.shown(g)
			return
		case reviewRowResume:
			if !chose {
				return
			}
			r.viewing = true
			r.shown(g)
			return
		case reviewRowOrder:
			r.byName = !r.byName
		case reviewRowSkipOK:
			r.skipOK = !r.skipOK
		case reviewRowSkipAny:
			r.skipAny = !r.skipAny
		}
	} else if i := sel - len(rows); i < len(r.groups) {
		r.groups[i] = !r.groups[i]
	}
	g.sfx("cursor")
	r.buildMenu(g)
}

func (r *cgReview) shown(g *Game) {
	it := r.items[r.at]
	g.emit("cg_review", traceLine{"page": it.page, "src": it.src, "at": r.at + 1, "of": len(r.items)})
	if r.at+1 < len(r.items) && g.pics != nil {
		g.pics.preload(r.items[r.at+1].page)
	}
}

func (r *cgReview) next(g *Game) {
	if r.at+1 >= len(r.items) {
		r.viewing = false
		r.at = len(r.items)
		r.say(lang.T("最後まで見た"))
		r.buildMenu(g)
		g.sfx("decide")
		return
	}
	r.at++
	g.sfx("cursor")
	r.shown(g)
}

func (r *cgReview) say(s string) { r.toast, r.toastT = s, 90 }

func (r *cgReview) updateView(g *Game) {
	in := &g.in
	switch {
	case in.digit > 0:
		tags := g.db.Game.CGReview.Tags
		if in.digit <= len(tags) {
			r.toggle(g, tags[in.digit-1])
		}
	case in.pressed[btnMemo] || in.pressed[btnNext]:
		r.openBox(g)
	case in.pressed[btnHide]:
		r.hide = !r.hide
	case in.cancel():
		r.viewing = false
		r.buildMenu(g)
		g.sfx("cancel")
	case in.left() || in.up():
		if r.at > 0 {
			r.at--
			g.sfx("cursor")
			r.shown(g)
		}
	case in.ok() || in.click || in.right() || in.down():
		r.next(g)
	}
}

func (r *cgReview) toggle(g *Game, t data.CGReviewTag) {
	it := r.items[r.at]
	m := r.marks[it.key()]
	on := !m[t.ID]
	if on {
		for id := range m {
			if t.OK || id == r.okTag(g) {
				r.write(g, it, r.tagOf(g, id), true, "")
			}
		}
	}
	r.write(g, it, t, !on, "")
	if on {
		r.say(lang.T("札") + ": " + t.Label)
		g.sfx("decide")
	} else {
		r.say(lang.T("外した") + ": " + t.Label)
		g.sfx("cancel")
	}
	if on && t.OK {
		r.box = nil
		if !r.inPlay {
			r.next(g)
		}
	}
}

func (r *cgReview) tagOf(g *Game, id string) data.CGReviewTag {
	for _, t := range g.db.Game.CGReview.Tags {
		if t.ID == id {
			return t
		}
	}
	return data.CGReviewTag{ID: id, Label: id}
}

func (r *cgReview) write(g *Game, it reviewItem, t data.CGReviewTag, off bool, note string) {
	l := reviewLine{Time: time.Now().Format(time.RFC3339), Src: it.src, Page: it.page, Slot: it.slot,
		Tag: t.ID, Label: t.Label, OK: t.OK, Off: off, Note: note}
	r.apply(l)
	g.emit("cg_review_tag", traceLine{"page": it.page, "src": it.src, "tag": t.ID, "off": off, "note": note})
	b, err := json.Marshal(l)
	if err != nil {
		g.failure = err
		return
	}
	file := g.reviewLog()
	if dir := filepath.Dir(file); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			g.failure = err
			return
		}
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // The host explicitly selects the review output directory.
	if err == nil {
		_, err = f.Write(append(b, '\n'))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		log.Printf("review: %v", err)
		r.say(lang.T("記録を書けなかった"))
	}
}

func (r *cgReview) openBox(g *Game) {
	b := &reviewBox{}
	r.box = b
	r.refreshBox(g)
	g.sfx("decide")
}

func (r *cgReview) refreshBox(g *Game) {
	b := r.box
	it := r.items[r.at]
	m := r.marks[it.key()]
	var items []listItem
	for i, t := range g.db.Game.CGReview.Tags {
		key := ""
		if i < 10 {
			key = itoa((i + 1) % 10)
		}
		li := listItem{label: t.Label, right: key, id: t.ID}
		if m[t.ID] {
			li.note, li.noteColor = "✓", color.NRGBA{0x7c, 0xe0, 0xa8, 0xff}
		}
		items = append(items, li)
	}
	items = append(items, listItem{label: lang.T("一言を足す"), id: reviewNoteTag})
	sel := 0
	if b.list != nil {
		sel = b.list.sel
	}
	_, sh := g.screen()
	rows := min(len(items), int((sh-170)/30))
	b.list = newList(items, 0, 0, 560, rows)
	b.list.rowH, b.list.size = 30, sizeS*0.9
	b.list.sel = min(sel, len(items)-1)
	b.list.scroll()
}

func (r *cgReview) updateBox(g *Game) {
	b := r.box
	b.ag++
	in := &g.in
	if b.ext != nil {
		select {
		case res := <-b.ext:
			b.ext = nil
			if res.ok && strings.TrimSpace(res.text) != "" {
				r.write(g, r.items[r.at], data.CGReviewTag{ID: reviewNoteTag, Label: lang.T("一言")}, false, strings.TrimSpace(res.text))
				r.say(lang.T("一言を残した"))
			}
		default:
		}
		return
	}
	if in.digit > 0 {
		tags := g.db.Game.CGReview.Tags
		if in.digit <= len(tags) {
			r.toggle(g, tags[in.digit-1])
			if r.box != nil {
				r.refreshBox(g)
			}
		}
		return
	}
	if in.pressed[btnMemo] || in.pressed[btnNext] || in.pressed[btnTag] {
		r.box = nil
		g.sfx("cancel")
		return
	}
	chose, cancel := b.list.update(g)
	switch {
	case cancel:
		r.box = nil
	case chose:
		id := b.list.items[b.list.sel].id
		if id == reviewNoteTag {
			if in.script != nil || b.text != "" {
				if s := strings.TrimSpace(b.text); s != "" {
					r.write(g, r.items[r.at], data.CGReviewTag{ID: reviewNoteTag, Label: lang.T("一言")}, false, s)
					r.say(lang.T("一言を残した"))
				}
				b.text = ""
				return
			}
			ch := make(chan extInput, 1)
			if g.opts.ExternalInput && externalEntry(g.context, lang.T("一枚絵の一言"), lang.T("一言（OK で残す、取消でやめる）")+"\n"+r.items[r.at].page, ch) {
				b.ext = ch
			}
			return
		}
		r.toggle(g, r.tagOf(g, id))
		if r.box != nil {
			r.refreshBox(g)
		}
	}
}

func (r *cgReview) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if r.viewing || r.box != nil {
		fill(dst, colorBlack)
		it := r.items[r.at]
		g.drawCG(dst, data.CG{File: it.page}, 1)
		if !r.hide {
			r.drawInfo(g, dst, it)
		}
		if r.box != nil {
			r.drawBox(g, dst, it)
		}
		r.drawToast(g, dst)
		return
	}
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xb0))
	gfx.DrawTextOutlineColor(dst, lang.T("一枚絵の流し見"), sw/2, 60, sizeL, g.th.accent, g.th.window, 1)
	head := gfx.Ellipsis(fmt.Sprintf(lang.T("全部 %d 枚　記録 %s"), len(r.all), g.reviewLog()), sizeS*0.8, sw-80)
	gfx.DrawTextOutlineColor(dst, head, sw/2, 118, sizeS*0.8, g.th.dim, g.th.window, 1)
	r.menu.draw(g, dst, true)
	help := lang.T("Z: 決める　←→: 切り替え　X: もどる")
	gfx.DrawTextOutlineColor(dst, help, sw/2, sh-90, sizeS, g.th.text, g.th.window, 1)
	keys := lang.T("流し見の間: Z・Enter・→ 次　← 前　1〜9・0 札　F9・Tab 札の一覧　V 字を消す　X もどる")
	gfx.DrawTextOutlineColor(dst, keys, sw/2, sh-52, sizeS*0.85, g.th.dim, g.th.window, 1)
	r.drawToast(g, dst)
}

func (r *cgReview) drawInfo(g *Game, dst *ebiten.Image, it reviewItem) {
	_, sh := g.screen()
	sz := sizeS * 0.7
	src := it.src
	if src == "" {
		src = lang.T("（対応なし）")
	}
	slot := it.slot
	if it.title != "" {
		slot += fmt.Sprintf(lang.T("（%s）"), it.title)
	}
	var tags []string
	for _, t := range g.db.Game.CGReview.Tags {
		if r.marks[it.key()][t.ID] {
			tags = append(tags, t.Label)
		}
	}
	mark := lang.T("札なし")
	var mc color.Color = g.th.dim
	if len(tags) > 0 {
		mark, mc = lang.T("札")+": "+strings.Join(tags, "、"), color.NRGBA{0xff, 0xa0, 0x70, 0xff}
		if slices.Contains(tags, r.tagOf(g, r.okTag(g)).Label) {
			mc = color.NRGBA{0x7c, 0xe0, 0xa8, 0xff}
		}
	}
	lines := []struct {
		s string
		c color.Color
	}{
		{fmt.Sprintf("%d / %d　%s　%s", r.at+1, len(r.items), slot, it.page), g.th.text},
		{lang.T("元") + ": " + src, g.th.text},
		{mark, mc},
	}
	if n := r.notes[it.key()]; n != "" {
		lines = append(lines, struct {
			s string
			c color.Color
		}{lang.T("一言") + ": " + n, g.th.dim})
	}
	sw, _ := g.screen()
	room, w := sw*0.75, 0.0
	for i := range lines {
		lines[i].s = gfx.Ellipsis(lines[i].s, sz, room)
		w = max(w, gfx.TextWidth(lines[i].s, sz))
	}
	lh := sz * 1.35
	y := sh - 10 - float64(len(lines))*lh
	gfx.FillRoundRect(dst, 6, float32(y-6), float32(w+20), float32(float64(len(lines))*lh+10), 8, withAlpha(colorBlack, 0x90))
	for _, l := range lines {
		g.text(dst, l.s, 16, y, sz, l.c)
		y += lh
	}
}

func (r *cgReview) drawBox(g *Game, dst *ebiten.Image, it reviewItem) {
	sw, sh := g.screen()
	b := r.box
	w, h := 600.0, b.list.height()+130
	x, y := sw-w-24, (sh-h)/2
	g.window(dst, x, y, w, h)
	g.text(dst, lang.T("一枚絵の札"), x+24, y+14, sizeM, g.th.accent)
	gfx.DrawTextRight(dst, it.slot, x+w-24, y+20, sizeS*0.8, g.th.dim)
	b.list.x, b.list.y, b.list.w = x+20, y+54, w-40
	b.list.drawRows(g, dst, b.ext == nil)
	help := lang.T("Z・数字: 付ける／外す（OK は次の絵へ）　F9・X: 閉じる")
	switch {
	case b.ext != nil:
		help = lang.T("別の窓で一言を書いて OK（窓が見えなければ Alt+Tab）")
	case b.text != "":
		help = lang.T("一言") + ": " + b.text
	}
	g.text(dst, help, x+24, y+h-44, sizeS*0.8, g.th.dim)
}

func (r *cgReview) drawToast(g *Game, dst *ebiten.Image) {
	if r.toastT <= 0 {
		return
	}
	sw, _ := g.screen()
	x := sw / 2
	if r.box != nil {
		x = 24 + (gfx.TextWidth(r.toast, sizeS)+40)/2
	}
	w := gfx.TextWidth(r.toast, sizeS) + 40
	gfx.FillRoundRect(dst, float32(x-w/2), 20, float32(w), 44, 10, withAlpha(colorBlack, 0xc0))
	gfx.DrawText(dst, r.toast, x, 28, sizeS, g.th.text)
}

func (g *Game) reviewOpen() *cgReview {
	if len(g.stack) == 0 {
		return nil
	}
	r, ok := g.stack[len(g.stack)-1].(*cgReview)
	if !ok {
		return nil
	}
	return r
}

func (g *Game) memoKeyTaken() bool {
	if len(g.stack) == 0 {
		return false
	}
	t, ok := g.stack[len(g.stack)-1].(interface{ takesMemoKey() bool })
	return ok && t.takesMemoKey()
}
