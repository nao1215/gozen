package play

import (
	"fmt"
	"log"
	"math/rand/v2"

	"github.com/nao1215/gozen/internal/layer1/memo"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

type fastTo struct {
	lines   int
	answers []int
	battle  bool
	ticks   int

	picture string
	seen    int
	seenAt  int
	page    bool
	// scene is scene skip: it stops before a line not read, a choice or a battle (skipBefore).
	scene bool
}

func (g *Game) startFromMemo(path string) error {
	m, save, err := memo.Read(path)
	if err != nil {
		return err
	}
	p := rand.NewPCG(1, 2)
	if len(m.Rand) == 0 || p.UnmarshalBinary(m.Rand) != nil {
		p = g.newPCG()
	}
	st, err := rpg.ParseWithConfig(g.db, save, rand.New(p), rpg.Config{AllowLegacy: g.opts.AllowLegacySave}) //nolint:gosec // Restores deterministic gameplay randomness, not secrets.
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	g.pcg = p
	g.loadGame(st)
	if f, ok := st.Vars[dungeon.VarFloor].(float64); ok {
		g.bannerFloor = f
	}
	r := m.Resume
	if r == nil {
		r = &memo.Resume{From: "here"}
	}
	if r.From != "arrival" {
		g.enterAt = ""
	}
	switch r.From {
	case "event":
		cmds := g.commandsNamed(r.Event)
		if cmds == nil {
			log.Printf("from memo: no event %q (the data changed?): starting where it began", r.Event)
			break
		}
		g.events.Start(cmds)
		g.fast = &fastTo{lines: r.Line, answers: r.Answers, battle: r.InBattle}
	case "arrival":
		g.fast = &fastTo{lines: r.Line, answers: r.Answers, battle: r.InBattle}
	case "battle":
		if g.db.Troops[r.Battle] != nil {
			g.startBattle(r.Battle, func(int) {})
		}
	}
	g.emit("from_memo", traceLine{"file": path, "resume": r.From, "event": r.Event, "line": r.Line, "memo_version": m.Version})
	return nil
}

func (g *Game) fastStep() {
	in := &g.in
	in.pressed, in.held = [numButtons]bool{}, [numButtons]bool{}
	in.click, in.rclick, in.moved, in.wheel, in.fast, in.esc, in.shift = false, false, false, 0, false, false, false
	f := g.fast
	f.ticks++
	stop := func(why string) {
		if f.scene {
			g.endSceneSkip(why)
			return
		}
		g.fast = nil
		if g.opts.capture != nil {
			g.opts.capture.seek = why
		}
		g.emit("from_memo_ready", traceLine{"lines": g.evLines, "want": f.lines, "why": why, "picture": g.cg.File})
		g.landedVoice()
	}
	reached := g.evLines >= f.lines
	what, t := g.waiting()
	if f.picture != "" {
		if (what == "text" || what == "choice") && g.talk.active && g.cg.File == f.picture && g.evLines != f.seenAt {
			f.seen, f.seenAt = f.seen+1, g.evLines
		}
		reached = f.seen >= max(1, f.lines) && g.evLines == f.seenAt
	}
	switch {
	case f.page && what == "choice" && (g.talk.choice == nil || len(g.evAnswers) >= len(f.answers) || f.answers[len(g.evAnswers)] < 0 || f.answers[len(g.evAnswers)] >= len(g.talk.choice.items)):
		stop("choice")
		return
	case f.ticks > 60*300:
		if g.opts.capture == nil {
			log.Printf("from memo: gave up reading on to line %d (at %d)", f.lines, g.evLines)
		}
		stop("gave up")
		return
	case reached && f.battle && g.currentBattle() != nil:
		stop("battle")
		return
	case reached && !f.battle && (what == "text" || what == "choice") && g.talk.active:
		stop("line")
		return
	case f.ticks > 30 && !g.busy():
		stop("no event")
		return
	}
	switch what {
	case "text":
		if t >= 1 {
			in.pressed[btnOK] = true
		}
	case "choice":
		if g.talk.active && g.talk.choice != nil {
			if k := len(g.evAnswers); k < len(f.answers) && f.answers[k] >= 0 && f.answers[k] < len(g.talk.choice.items) {
				g.talk.choice.sel = f.answers[k]
			}
		}
		if f.ticks%4 == 0 {
			in.pressed[btnOK] = true
		}
	case "command":
		g.act("battle_auto", nil)
	default:
		if g.talk.active && g.talk.shown < g.talk.total {
			in.pressed[btnOK] = true
		}
	}
}
