package play

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	lang "github.com/nao1215/gozen/internal/base/text"
	"github.com/nao1215/gozen/internal/drive/itemuse"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/rules/world"
	"github.com/nao1215/gozen/internal/screen/gfx"
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
	g.in.observing = true
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
	if g.tr != nil {
		if err != nil {
			g.emit("persistence_error", nil)
		} else {
			t, known := g.storeTime(path)
			g.emit("save_write", traceLine{"file": path, "time": t, "time_known": known})
		}
	}
	if backup != "" {
		log.Printf("save backup: %s", backup)
		g.emit("save_backup", traceLine{"file": path, "backup": backup})
	}
	return err
}

func (g *Game) loadSavedGame(st *rpg.State) {
	if st.Resume != nil && len(st.Resume.Cursor.Inputs) > 0 {
		probe, replay, root, err := g.savedScreens(st)
		if err != nil {
			g.failure = err
			return
		}
		g.installScreenReplay(probe, replay, root)
		return
	}
	if err := g.checkSavedReplay(st); err != nil {
		g.failure = err
		return
	}
	r := st.Resume
	st.Rand = g.stateRng()
	saved := st.SavedRand
	if r != nil {
		saved = r.Rand
	}
	if len(saved) > 0 {
		if p := rand.NewPCG(1, 2); p.UnmarshalBinary(saved) == nil {
			g.pcg, st.Rand = p, rand.New(p) //nolint:gosec // Restores deterministic gameplay randomness, not secrets.
		}
	}
	g.loadGame(st)
	if r != nil {
		g.resume(r)
	}
}

// checkSavedReplay probes parsed saves on private state before either slot
// selection or direct installation can replace the live game. It runs no
// presentation, input, autosave or external effects.
func (g *Game) checkSavedReplay(st *rpg.State) error {
	if st.Resume == nil {
		return nil
	}
	c := st.Resume.Cursor
	if len(c.Inputs) > 0 {
		_, _, _, err := g.savedScreens(st)
		return err
	}
	// Freeze the fallback once so every probe and installation uses the same
	// legacy random stream, including direct loads that bypass slot selection.
	if len(st.Resume.Rand) == 0 {
		rnd, err := g.newPCG().MarshalBinary()
		if err != nil {
			return err
		}
		st.Resume.Rand = rnd
	}
	// Preserve the legacy missing-root fallback used by resume.
	if !c.Arrival && st.DB.NamedCommands(c.Event) == nil {
		return nil
	}
	if !c.Arrival && c.Line == 0 { // a checkpoint save begins its event from the start
		return nil
	}
	probe, err := g.newSavedReplay(st)
	if err != nil {
		return err
	}
	_, err = probe.runner.ReplayTo(c.Line, c.Answers)
	return err
}

// savedReplay owns private reconstruction state until installation succeeds.
type savedReplay struct {
	st      *rpg.State
	pcg     *rand.PCG
	runner  *event.Runner
	target  *Game
	picture data.CG
	effects []event.Effect
}

func (g *Game) newSavedReplay(st *rpg.State) (*savedReplay, error) {
	b, err := st.Save()
	if err != nil {
		return nil, err
	}
	p := rand.NewPCG(1, 2)
	if err := p.UnmarshalBinary(st.Resume.Rand); err != nil {
		return nil, err
	}
	state, err := rpg.Parse(data.Fork(st.DB), b, rand.New(p)) //nolint:gosec // Private deterministic gameplay replay, not secrets.
	if err != nil {
		return nil, err
	}
	state.Calm = g.opts.Calm
	probe := &savedReplay{st: state, pcg: p}
	runner := event.New(state.DB, state, event.HostFunc(func(e event.Effect) {
		if probe.target != nil {
			probe.target.Handle(e)
			return
		}
		if image, ok := e.(event.ImageEffect); ok {
			probe.picture = image.Picture
		}
		if len(st.Resume.Cursor.Inputs) > 0 {
			probe.effects = append(probe.effects, e)
		}
	}))
	probe.runner = runner
	runner.Sortie, runner.Dev = true, g.opts.Development
	runner.UseItem = itemuse.For(state.DB, state)
	runner.BlockAside = func() bool {
		if probe.target != nil {
			return probe.target.AsideBlocked()
		}
		return probe.picture.File != "" && !probe.picture.Faces
	}
	runner.Log = func(err error) {
		if probe.target != nil {
			probe.target.screenTrace.blocked = true
			log.Print(err)
		}
	}
	c := st.Resume.Cursor
	if c.Arrival {
		runner.Start(world.Arrive(state.DB, state, true))
	} else if len(c.Args) > 0 {
		if err := runner.StartBound(c.Event, c.Args); err != nil {
			return nil, err
		}
	} else {
		runner.Start(state.DB.NamedCommands(c.Event))
	}
	return probe, nil
}

func (g *Game) savedScreens(st *rpg.State) (*savedReplay, event.ScreenReplay, screenTranscript, error) {
	var replay event.ScreenReplay
	root := screenTranscript{cursor: st.Resume.Cursor, rnd: st.Resume.Rand, inputs: st.Resume.Cursor.Inputs, saw: true}
	root.cursor.Inputs = nil
	var err error
	root.snap, err = st.Save()
	if err != nil {
		return nil, replay, root, err
	}
	probe, err := g.newSavedReplay(st)
	if err != nil {
		return nil, replay, root, err
	}
	var pending event.Request
	if root.cursor.Line > 0 {
		pending, err = probe.runner.ReplayTo(root.cursor.Line, root.cursor.Answers)
		if err != nil {
			return nil, replay, root, err
		}
		if _, ok := pending.(event.Choice); !ok {
			pending = nil
		}
	}
	replay, err = probe.runner.ReplayScreens(root.inputs, pending)
	if err == nil {
		for _, frame := range replay.Frames {
			def := st.DB.Game.Screens[frame.Name]
			if def.Kind != "" || def.View.Fullscreen {
				return nil, replay, root, errors.New("load: planning screen continuation is not supported")
			}
		}
	}
	return probe, replay, root, err
}
