package play

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/text"
)

const DefaultSceneReviewLog = "scene-review.jsonl"

func (g *Game) sceneReviewLog() string {
	return filepath.Join(filepath.Dir(g.opts.ReviewLog), DefaultSceneReviewLog)
}

const galleryCheckTab = 3

type sceneReviewLine struct {
	Time  string `json:"time"`
	Event string `json:"event"`
	Title string `json:"title"`
	Group string `json:"group,omitempty"`
	Tag   string `json:"tag"`
	Label string `json:"label"`
	OK    bool   `json:"ok,omitempty"`
}

type sceneCheck struct {
	rows     []int
	onlyTodo bool
	verdict  map[string]sceneReviewLine
	pending  int
	box      *list
	boxFor   int
	toast    string
	toastT   int
}

func (s *galleryScreen) takesTagKey() bool { return s.tab == galleryCheckTab }

func sceneKey(sc data.GalleryScene) string { return sc.Event + "|" + sc.Title }

func sceneGroup(sc data.GalleryScene) string {
	if sc.Group != "" {
		return sc.Group
	}
	return lang.T("その他")
}

func sceneTags(g *Game) []data.CGReviewTag {
	if t := g.db.Game.SceneReview.Tags; len(t) > 0 {
		return t
	}
	return []data.CGReviewTag{{ID: "ok", Label: lang.T("OK"), OK: true}, {ID: "ng", Label: lang.T("NG")}}
}

func (c *sceneCheck) load(g *Game) {
	c.verdict = map[string]sceneReviewLine{}
	f, err := os.Open(g.sceneReviewLog())
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
		var l sceneReviewLine
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			if l.Tag == "" {
				delete(c.verdict, l.Event+"|"+l.Title)
			} else {
				c.verdict[l.Event+"|"+l.Title] = l
			}
		}
	}
}

func checkGroups(g *Game) ([]string, map[string][]int) {
	var order []string
	by := map[string][]int{}
	for i, sc := range g.db.Game.Gallery.Scenes {
		k := sceneGroup(sc)
		if _, ok := by[k]; !ok {
			order = append(order, k)
		}
		by[k] = append(by[k], i)
	}
	rank := map[string]int{}
	for i, k := range g.db.Game.SceneReview.Groups {
		rank[k] = i + 1
	}
	r := func(k string) int {
		if n := rank[k]; n > 0 {
			return n
		}
		return len(rank) + 1
	}
	sort.SliceStable(order, func(a, b int) bool { return r(order[a]) < r(order[b]) })
	return order, by
}

func (s *galleryScreen) buildCheck(g *Game) {
	sw, sh := g.screen()
	if s.check == nil {
		s.check = &sceneCheck{pending: -1}
		s.check.load(g)
	}
	c := s.check
	scenes := g.db.Game.Gallery.Scenes
	order, by := checkGroups(g)
	var items []listItem
	c.rows = c.rows[:0]
	green, orange := color.NRGBA{0x7c, 0xe0, 0xa8, 0xff}, color.NRGBA{0xff, 0xa0, 0x70, 0xff}
	for _, gr := range order {
		ok, ng := 0, 0
		for _, i := range by[gr] {
			if v, has := c.verdict[sceneKey(scenes[i])]; has {
				if v.OK {
					ok++
				} else {
					ng++
				}
			}
		}
		n := len(by[gr])
		if c.onlyTodo && ok+ng == n {
			continue
		}
		items = append(items, listItem{label: "■ " + gr, right: fmt.Sprintf(lang.T("OK %d　NG %d　未 %d"), ok, ng, n-ok-ng), disabled: true, color: g.th.accent})
		c.rows = append(c.rows, -1)
		for _, i := range by[gr] {
			v, has := c.verdict[sceneKey(scenes[i])]
			if c.onlyTodo && has {
				continue
			}
			it := listItem{label: "　" + scenes[i].Title}
			switch {
			case !has:
				it.note, it.noteColor = lang.T("未"), g.th.dim
			case v.OK:
				it.note, it.noteColor = v.Label, green
			default:
				it.note, it.noteColor = v.Label, orange
			}
			items = append(items, it)
			c.rows = append(c.rows, i)
		}
	}
	s.menu = newList(items, 40, 130, sw*0.46, int((sh-260)/40))
	s.menu.rowH = 40
	for s.menu.sel < len(c.rows)-1 && c.rows[s.menu.sel] < 0 {
		s.menu.sel++
	}
}

func (s *galleryScreen) updateCheckModal(g *Game) bool {
	c := s.check
	if c == nil {
		return false
	}
	c.toastT = max(0, c.toastT-1)
	if c.pending >= 0 {
		s.openVerdict(g, c.pending)
		c.pending = -1
		return true
	}
	if c.box == nil {
		return false
	}
	chose, cancel := c.box.update(g)
	switch {
	case cancel || g.in.pressed[btnTag]:
		c.box = nil
		g.sfx("cancel")
	case chose:
		tags := sceneTags(g)
		if c.box.sel < len(tags) {
			s.writeVerdict(g, c.boxFor, tags[c.box.sel])
		}
		c.box = nil
		sel := s.menu.sel
		s.buildCheck(g)
		s.menu.sel = min(sel, max(0, len(s.menu.items)-1))
		s.menu.scroll()
	}
	return true
}

func (s *galleryScreen) updateCheck(g *Game) {
	c := s.check
	if g.in.pressed[btnNext] {
		c.onlyTodo = !c.onlyTodo
		g.sfx("cursor")
		s.buildCheck(g)
		return
	}
	cur := -1
	if s.menu.sel < len(c.rows) {
		cur = c.rows[s.menu.sel]
	}
	if g.in.pressed[btnTag] && cur >= 0 {
		s.openVerdict(g, cur)
		return
	}
	chose, _ := s.menu.update(g)
	if !chose || cur < 0 || s.menu.sel >= len(c.rows) || c.rows[s.menu.sel] < 0 {
		return
	}
	sc := g.db.Game.Gallery.Scenes[c.rows[s.menu.sel]]
	c.pending = c.rows[s.menu.sel]
	g.emit("gallery", traceLine{"scene": sc.Event, "check": true})
	g.push(newSceneReplay(g, sc))
}

func (s *galleryScreen) openVerdict(g *Game, i int) {
	c := s.check
	var items []listItem
	for _, t := range sceneTags(g) {
		items = append(items, listItem{label: t.Label, id: t.ID})
	}
	items = append(items, listItem{label: lang.T("後で"), id: ""})
	c.box = newList(items, 0, 0, 560, min(len(items), 12))
	c.box.rowH = 34
	c.boxFor = i
	g.sfx("decide")
}

func (s *galleryScreen) writeVerdict(g *Game, i int, t data.CGReviewTag) {
	c := s.check
	sc := g.db.Game.Gallery.Scenes[i]
	l := sceneReviewLine{Time: time.Now().Format(time.RFC3339), Event: sc.Event, Title: sc.Title, Group: sc.Group, Tag: t.ID, Label: t.Label, OK: t.OK}
	c.verdict[sceneKey(sc)] = l
	g.emit("scene_review", traceLine{"event": sc.Event, "title": sc.Title, "tag": t.ID})
	b, err := json.Marshal(l)
	if err != nil {
		g.failure = err
		return
	}
	if err := os.MkdirAll(filepath.Dir(g.sceneReviewLog()), 0o700); err != nil {
		g.failure = err
		return
	}
	f, err := os.OpenFile(g.sceneReviewLog(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = f.Write(append(b, '\n'))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		log.Printf("scene review: %v", err)
		c.toast, c.toastT = lang.T("記録を書けなかった"), 120
		return
	}
	c.toast, c.toastT = sc.Title+"　→ "+t.Label, 120
}

func (s *galleryScreen) drawCheck(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	c := s.check
	s.menu.draw(g, dst, c.box == nil)
	scenes := g.db.Game.Gallery.Scenes
	ok, ng := 0, 0
	for _, v := range c.verdict {
		if v.OK {
			ok++
		} else {
			ng++
		}
	}
	x, y, w := sw*0.52, 130.0, sw*0.44
	h := w * 9 / 16
	g.window(dst, x-8, y-8, w+16, h+16)
	cur := -1
	if s.menu.sel < len(c.rows) {
		cur = c.rows[s.menu.sel]
	}
	if cur >= 0 && scenes[cur].Picture != "" {
		g.drawPicture(dst, data.Picture{File: scenes[cur].Picture}, x, y, w, h, 1)
	}
	if cur >= 0 {
		ls, sz := gfx.FitLines(scenes[cur].Title, sizeM, w, 1)
		if len(ls) > 0 {
			gfx.DrawTextOutlineColor(dst, ls[0], x+w/2, y+h+20, sz, g.th.text, g.th.window, 1)
		}
		if v, has := c.verdict[sceneKey(scenes[cur])]; has {
			gfx.DrawTextOutlineColor(dst, fmt.Sprintf(lang.T("判定: %s（%s）"), v.Label, v.Time[:min(len(v.Time), 16)]), x+w/2, y+h+60, sizeS*0.85, g.th.dim, g.th.window, 1)
		}
	}
	all := len(scenes)
	sum := fmt.Sprintf(lang.T("全 %d　OK %d　NG %d　未 %d"), all, ok, ng, all-ok-ng)
	if c.onlyTodo {
		sum += lang.T("（未だけ）")
	}
	gfx.DrawTextOutlineColor(dst, sum, x+w/2, y+h+100, sizeM*0.9, g.th.accent, g.th.window, 1)
	hint := lang.T("Z: 見る（見終わると判定）   F10: 見ずに判定   Tab: 未だけ／全部   ←→: 欄   X: もどる")
	gfx.DrawTextOutlineColor(dst, hint, sw/2, sh-44, sizeS*0.9, g.th.text, g.th.window, 1)
	if c.box != nil && c.boxFor >= 0 {
		fill(dst, withAlpha(colorBlack, 0xc0))
		bw, bh := 600.0, c.box.height()+110
		bx, by := (sw-bw)/2, (sh-bh)/2
		g.window(dst, bx, by, bw, bh)
		t, sz := gfx.FitLines(scenes[c.boxFor].Title, sizeM, bw-48, 1)
		if len(t) > 0 {
			g.text(dst, t[0], bx+24, by+14, sz, g.th.accent)
		}
		c.box.x, c.box.y, c.box.w = bx+20, by+60, bw-40
		c.box.drawRows(g, dst, true)
		g.text(dst, lang.T("Z: 決める　X・F10: 閉じる"), bx+24, by+bh-40, sizeS*0.8, g.th.dim)
	}
	if c.toastT > 0 {
		tw := gfx.TextWidth(c.toast, sizeS) + 40
		gfx.FillRoundRect(dst, float32(sw/2-tw/2), 20, float32(tw), 44, 10, withAlpha(colorBlack, 0xc0))
		gfx.DrawText(dst, c.toast, sw/2, 28, sizeS, g.th.text)
	}
}
