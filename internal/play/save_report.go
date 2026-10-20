package play

import (
	"fmt"
	"log"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

type loadNotice struct {
	lines []string
	list  *list
}

func (g *Game) reportLoad(r rpg.LoadReport) {
	if !r.Changed() {
		return
	}
	var lines []string
	if r.Legacy {
		lines = append(lines, lang.T("旧形式のセーブを読み込んだ。次の保存時に元ファイルの控えを残す"))
	}
	if r.SavedDataVersion != r.DataVersion {
		lines = append(lines, fmt.Sprintf(lang.T("データ版 %d から %d へ移行"), r.SavedDataVersion, r.DataVersion))
	}
	for _, c := range r.Changes {
		lines = append(lines, c.Path+" / "+c.ID+" — "+c.Reason)
	}
	g.emit("save_load", traceLine{"report": r})
	if g.loadNotice == nil {
		g.loadNotice = &loadNotice{}
	}
	for _, line := range lines {
		if !slices.Contains(g.loadNotice.lines, line) {
			log.Printf("save load: %s", line)
			g.loadNotice.lines = append(g.loadNotice.lines, line)
		}
	}
	g.loadNotice.list = nil
}

func (g *Game) updateLoadNotice() bool {
	n := g.loadNotice
	if n == nil || g.fast != nil || g.live.quiet {
		return false
	}
	if n.list == nil {
		sw, sh := g.screen()
		var items []listItem
		for _, line := range n.lines {
			for _, part := range gfx.Wrap(line, sizeS, sw-180) {
				items = append(items, listItem{label: part})
			}
		}
		n.list = newList(items, 60, 120, sw-120, max(1, int((sh-260)/40)))
		n.list.rowH, n.list.size = 40, sizeS
		return true
	}
	ok, cancel := n.list.update(g)
	if ok || cancel {
		g.loadNotice = nil
	}
	return true
}

func (g *Game) drawLoadNotice(dst *ebiten.Image) {
	n := g.loadNotice
	if n == nil || n.list == nil {
		return
	}
	fill(dst, withAlpha(colorBlack, 0xe8))
	g.text(dst, lang.T("セーブの移行・変更点"), 60, 65, sizeL, g.th.text)
	n.list.draw(g, dst, true)
	_, sh := g.screen()
	g.text(dst, lang.T("↑↓ 一覧を読む　決定・戻る 閉じる"), 60, sh-75, sizeS, g.th.text)
}

func (g *Game) writeSave(path string) error {
	backup, err := g.saveState(path)
	if backup != "" {
		log.Printf("save backup: %s", backup)
		g.emit("save_backup", traceLine{"file": path, "backup": backup})
	}
	return err
}

func (g *Game) loadSavedGame(st *rpg.State) {
	st.Rand = g.stateRng()
	g.loadGame(st)
}
