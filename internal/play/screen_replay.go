package play

import (
	"errors"
	"slices"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// screenTranscript retains the named root while child entries replace ev*.
// It records semantic requests and answers, never frames, callbacks or pixels.
type screenTranscript struct {
	cursor       rpg.Cursor
	snap, rnd    []byte
	inputs       []rpg.ReplayInput
	blocked, saw bool
}

func (g *Game) hasAuthorScreen() bool {
	for _, view := range g.stack {
		if _, ok := view.(*authorScreen); ok {
			return true
		}
	}
	return false
}

// homeCheckpoint reports whether a save now falls back to the last checkpoint:
// a home screen is open, its own replay cannot be saved, and a checkpoint exists.
func (g *Game) homeCheckpoint() bool {
	if g.st == nil || g.st.Checkpoint == nil || g.screenSaveAllowed() {
		return false
	}
	for _, view := range g.stack {
		if s, ok := view.(*authorScreen); ok {
			if def := g.db.Game.Screens[s.name]; def != nil && def.Home {
				return true
			}
		}
	}
	return false
}

// saveCheckpoint saves the last checkpoint: loading it starts the checkpoint's
// event again, as a retry would, and keeps the checkpoint for later retries.
func (g *Game) saveCheckpoint() ([]byte, error) {
	st, _, err := g.st.RetryCheckpoint()
	if err != nil {
		return nil, err
	}
	b, err := st.Save()
	if err != nil {
		return nil, err
	}
	c := rpg.Cursor{Event: st.Checkpoint.Event}
	if err := c.Seal(g.db); err != nil {
		return nil, err
	}
	return rpg.WithCursor(b, c, st.SavedRand)
}

func (g *Game) screenContinuation() bool {
	return g.hasAuthorScreen() || g.screenTrace.saw && g.events != nil && g.events.Running()
}

func (g *Game) screenSaveAllowed() bool {
	t := &g.screenTrace
	return g.db.Game.SchemaVersion >= data.SchemaVersion10 && !t.blocked && t.saw && t.cursor.Event != "" && !t.cursor.Arrival && len(t.inputs) > 0 && t.snap != nil
}

func (g *Game) recordScreenInput(input rpg.ReplayInput) {
	t := &g.screenTrace
	if t.blocked || g.db.Game.SchemaVersion < data.SchemaVersion10 {
		return
	}
	if len(t.inputs) >= 20000 {
		t.blocked = true
		t.inputs = nil
		return
	}
	t.inputs = append(t.inputs, input)
}

func (g *Game) recordScreenView(q event.ReplayView) {
	digest, err := q.Digest()
	if err != nil {
		g.screenTrace.blocked = true
		return
	}
	g.recordScreenInput(rpg.ReplayInput{Request: digest})
}

func (g *Game) recordScreenAnswer(n int) { g.recordScreenInput(rpg.ReplayInput{Answer: &n}) }

func (g *Game) recordScreen(s *authorScreen) {
	def := g.db.Game.Screens[s.name]
	if def.Kind != "" || def.View.Fullscreen || s.timelineContext != nil {
		g.screenTrace.blocked = true
		return
	}
	g.screenTrace.saw = true
	q := event.ReplayView{Kind: "open", ID: s.name, Text: s.title}
	for _, entry := range s.entries {
		q.Options = append(q.Options, entry.Label)
	}
	g.recordScreenView(q)
}

func (g *Game) saveScreen() ([]byte, error) {
	if !g.screenSaveAllowed() {
		return nil, errors.New("finish the event that opened the screen before saving")
	}
	t := &g.screenTrace
	c := t.cursor
	c.Inputs = t.inputs
	if err := c.Seal(g.db); err != nil {
		return nil, err
	}
	return rpg.WithCursor(t.snap, c, t.rnd)
}

// installScreenReplay only runs after reconstruction succeeded on private state.
func (g *Game) installScreenReplay(probe *savedReplay, replay event.ScreenReplay, root screenTranscript) {
	g.pcg = probe.pcg
	g.talk = talk{}
	g.loadGame(probe.st)
	g.enterAt = ""
	probe.target = g
	g.events = replay.Runner
	g.events.OnStart = g.onEventStart
	for i, frame := range replay.Frames {
		frame.Runner.OnStart = g.onEventStart
		s := newAuthorScreen(g, frame.Name, func() { g.req = nil }, frame.Args)
		s.origin = frame.Origin
		s.destination = frame.Destination
		s.running = i < len(replay.Frames)-1 || replay.View.Kind != "open"
		if s.running {
			s.parent = frame.Runner
			s.request = event.Open{Screen: frame.Name, Args: frame.Args}
		}
		g.push(s)
	}
	// Restore persistent presentation without replaying sounds or transient effects.
	for _, effect := range probe.effects {
		switch effect.(type) {
		case event.ImageEffect, event.StandEffect:
			g.Handle(effect)
		}
	}
	g.req = replay.Request
	if replay.Request != nil && replay.View.Kind != "open" {
		g.begin(replay.Request)
	}
	g.evNoBack = true
	g.screenTrace = root
	g.screenTrace.inputs = slices.Clone(root.inputs)
	g.emit("resume", traceLine{"event": root.cursor.Event, "screen": replay.View.ID})
}
