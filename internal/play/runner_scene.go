package play

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/casual"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// runnerScene owns presentation for one event invocation. The solver cannot
// write story state; leaving returns its result to the waiting event once.
type runnerScene struct {
	request                           event.RunGame
	rules                             *casual.Runner
	view                              casual.Snapshot
	controls                          runnerControls
	ready, paused, blocked, pauseExit bool
	done                              func()
	countdown                         int
	portrait, previousPortrait        string
	portraitTicks, portraitPriority   int
	portraitBlend                     float64
	preAction                         []string
	preActionAhead                    string
}

// The runner owns its canvas above retained story pictures and dialogue.
func (s *runnerScene) modal() bool { return true }

func (g *Game) runnerActive() *runnerScene {
	for _, scene := range slices.Backward(g.stack) {
		if s, ok := scene.(*runnerScene); ok {
			return s
		}
	}
	return nil
}

func (g *Game) beginRunner(q event.RunGame, done func()) {
	if g.fast != nil {
		f := g.fast
		if len(g.events.GameResults()) < len(f.games) {
			if err := g.events.ReplayGame(f.games); err != nil {
				g.failure = err
				return
			}
			done()
			return
		}
		g.fast = nil
		if f.strict {
			g.failure = errors.New("load: missing game outcome")
			return
		}
	}
	rules, err := casual.New(q.Definition, casual.Config{Seed: q.Identity.Seed})
	if err != nil {
		g.failure = err
		return
	}
	s := &runnerScene{request: q, rules: rules, view: rules.Snapshot(), ready: true, blocked: true, done: done}
	if v := q.Definition.Visuals; v != nil && v.ReadyTicks > 0 {
		s.ready = false
		s.countdown = v.ReadyTicks
	}
	g.talk = talk{}
	g.push(s)
	g.emit("runner", traceLine{"id": q.Identity.ID, "state": "ready", "seed": q.Identity.Seed})
}

func (s *runnerScene) leave(g *Game) {
	result := s.request.Identity
	result.CompletedStages, result.BestCompletedStages, result.Misses, result.Ticks = s.view.CompletedStages, s.view.BestCompletedStages, s.view.Misses, s.view.Ticks
	result.Distance, result.Collected, result.Streak, result.LifeProgress = s.view.Distance, s.view.Collected, s.view.Streak, s.view.LifeProgress
	result.Status, result.Score, result.Lives, result.Stock, result.Stage = s.view.Status, s.view.Score, s.view.Lives, s.view.Stock, s.view.Stage
	if err := g.events.AnswerGame(result); err != nil {
		g.failure = err
		return
	}
	// Accepted results must refresh the autosave even when the location is unchanged.
	g.autoKey = ""
	g.emit("runner", traceLine{"id": result.ID, "state": result.Status, "score": result.Score, "stage": result.Stage})
	s.controls.reset()
	g.in.pressed, g.in.held = [numButtons]bool{}, [numButtons]bool{}
	g.in.click, g.in.pdown, g.in.pheld, g.in.pup = false, false, false, false
	g.pop()
	s.done()
}

func (s *runnerScene) update(g *Game) {
	if s.blocked {
		if g.in.runnerPointerHeld() {
			return
		}
		for _, held := range g.in.held {
			if held {
				return
			}
		}
		s.blocked = false
		return
	}
	s.updatePortrait()
	sw, sh := g.screen()
	x, y := float64(g.in.mouse.X), float64(g.in.mouse.Y)
	pressed := g.in.pressed[btnOK] || g.in.click && x >= sw*.15 && x < sw*.85 && y >= sh-76 && y < sh-22
	back := g.in.esc || g.in.pressed[btnCancel]
	if s.ready {
		s.updateReady(g, pressed, back)
		return
	}
	if s.view.Status != "running" {
		s.updateResult(g, pressed, back)
		return
	}
	if s.updatePause(g, pressed, back) {
		return
	}
	if s.countdown > 0 {
		s.countdown--
		s.controls.reset()
		return
	}
	in := s.controls.update(g)
	if g.in.query(s.controls.pause) {
		s.paused = true
		s.rules.Pause(true)
		s.controls.reset()
		s.blocked = true
		return
	}
	before := s.view
	s.rules.Step(in)
	s.view = s.rules.Snapshot()
	if g.tr != nil {
		g.emit("runner", traceLine{"id": s.request.Identity.ID, "ticks": s.view.Ticks, "x": s.view.X, "scroll": s.view.Scroll, "stage": s.view.Stage, "score": s.view.Score, "lives": s.view.Lives, "stock": s.view.Stock, "status": s.view.Status, "clear_ticks": s.view.ClearTicks})
	}
	s.react(s.view.Events)
	if s.view.Stock < before.Stock {
		s.preAction = before.Rows
		s.preActionAhead = before.Ahead
	}
	d := s.request.Definition
	if (s.view.Collected != before.Collected || s.view.Lives > before.Lives) && d.PickupSound != "" {
		g.sfx(d.PickupSound)
	}
	if s.view.Stock < before.Stock && d.ActionSound != "" {
		g.sfx(d.ActionSound)
	}
	if s.view.Status != before.Status {
		s.blocked = true
		if s.view.Status == "completed" && d.FinishSound != "" {
			g.sfx(d.FinishSound)
		}
		if (s.view.Status == "missed" || s.view.Status == "failed") && d.FailSound != "" {
			g.sfx(d.FailSound)
		}
	}
}

func (s *runnerScene) updateReady(g *Game, pressed, back bool) {
	if g.in.query(back) {
		s.rules.Cancel()
		s.view = s.rules.Snapshot()
		s.leave(g)
		return
	}
	if g.in.query(pressed) {
		s.ready = false
		s.blocked = true
		s.controls.reset()
	}
}

func (s *runnerScene) updateResult(g *Game, pressed, back bool) {
	if g.in.query(back) {
		if s.view.Status == "missed" {
			s.rules.GiveUp()
		} else {
			s.rules.Cancel()
		}
		s.view = s.rules.Snapshot()
		s.leave(g)
		return
	}
	if !g.in.query(pressed) {
		return
	}
	if s.view.Status == "missed" && s.rules.Retry() {
		s.view = s.rules.Snapshot()
		s.blocked = true
		s.controls.reset()
		if v := s.request.Definition.Visuals; v != nil {
			s.countdown = v.ReadyTicks
		}
		s.react(s.view.Events)
		return
	}
	s.leave(g)
}

func (s *runnerScene) updatePause(g *Game, pressed, back bool) bool {
	sw, sh := g.screen()
	x, y := float64(g.in.mouse.X), float64(g.in.mouse.Y)
	if g.in.query(back || g.in.click && x >= sw-64 && x < sw && y >= 0 && y < 44) {
		s.paused = !s.paused
		s.pauseExit = false
		s.rules.Pause(s.paused)
		s.controls.reset()
		s.blocked = true
		return true
	}
	if !s.paused {
		return false
	}
	if g.in.query(g.in.pressed[btnUp] || g.in.pressed[btnDown]) {
		s.pauseExit = !s.pauseExit
	}
	if g.in.query(g.in.click && x >= sw*.15 && x < sw*.85 && y >= sh-122 && y < sh-82 || pressed && s.pauseExit) {
		s.rules.Cancel()
		s.view = s.rules.Snapshot()
		s.leave(g)
		return true
	}
	if g.in.query(pressed) {
		s.paused = false
		s.rules.Pause(false)
		s.controls.reset()
		s.blocked = true
	}
	return true
}

func (s *runnerScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	d := s.request.Definition
	canvas := color.NRGBA{R: 13, G: 21, B: 34, A: 255}
	if d.Visuals != nil {
		canvas = d.Visuals.CanvasColor.Or(canvas)
	}
	dst.Fill(canvas)
	board := runnerBoardRect(g, d)
	left, top, cell := board[0], board[1], board[2]/float64(d.Width)
	background := d.Background
	if b := d.Stages[s.view.Stage].Background; b != "" {
		background = b
	}
	if background != "" && (d.Visuals == nil || !d.Visuals.BoardBackground) {
		g.drawPicture(dst, data.Picture{File: background}, 0, 0, sw, sh, 1)
	}
	s.drawPortrait(g, dst)
	clip := runnerBoardClip(board, gfx.Scale(), dst.Bounds())
	if !clip.Empty() {
		if target, ok := dst.SubImage(clip).(*ebiten.Image); ok {
			if v := d.Visuals; v != nil {
				if v.BoardBackground && background != "" {
					// Keep the top of an illustration visible while covering the board.
					img := g.pics.get(background)
					iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
					scale := math.Max(board[2]/iw, board[3]/ih)
					gfx.DrawImageScaled(target, img, left+(board[2]-iw*scale)/2, top, scale, 1)
				}
				if v.BoardColor.Valid {
					gfx.FillRect(target, float32(left), float32(top), float32(board[2]), float32(board[3]), v.BoardColor.NRGBA, false)
				}
			}
			s.drawRows(g, target, left, top, cell)
			s.drawActionWalls(g, target, left, top, cell)
		}
	}
	player := runnerPlayerRect(d, s.view.X, left, top, cell)
	if d.PlayerImage != "" {
		g.drawPictureContain(dst, d.PlayerImage, player[0], player[1], player[2], player[3], 1)
	} else {
		gfx.FillRoundRect(dst, float32(player[0]), float32(player[1]), float32(player[2]), float32(player[3]), 4, d.PlayerColor.Or(color.NRGBA{R: 101, G: 236, B: 197, A: 255}))
	}
	s.drawCutin(g, dst)
	white := color.NRGBA{R: 242, G: 246, B: 252, A: 255}
	label := func(text string, y float64, size float64) {
		size = math.Min(size, size*(sw-24)/math.Max(1, gfx.TextWidth(text, size)))
		gfx.DrawText(dst, text, sw/2, y, size, white)
	}
	hud := color.NRGBA{R: 13, G: 21, B: 34, A: 255}
	gfx.FillRect(dst, 0, 0, float32(sw), 42, hud, false)
	gfx.FillRect(dst, 0, float32(sh-36), float32(sw), 36, hud, false)
	header := fmt.Sprintf("Score %d   Lives %d   Action %d", s.view.Score, s.view.Lives, s.view.Stock)
	size := math.Min(18, 18*math.Max(24, sw-104)/math.Max(1, gfx.TextWidth(header, 18)))
	gfx.DrawTextLeft(dst, header, 12, 12, size, white)
	label(fmt.Sprintf("Stage %d / %d   Steer / Boost / Action", s.view.Stage+1, len(d.Stages)), sh-27, 14)
	message, button := "", ""
	switch {
	case s.paused:
		message = "Paused"
		button = "Resume"
	case s.countdown > 0:
		message = "Ready"
		if s.countdown <= 35 {
			message = "Go!"
		}
	case s.ready:
		message = d.Title
		button = "Start"
	case s.view.Status == "missed":
		message = "Try again"
		button = "Retry"
	case s.view.Status == "completed":
		message = "Course complete"
		button = "Continue"
	case s.view.Status == "failed":
		message = "Game over"
		button = "Continue"
	}
	if message != "" {
		gfx.FillRect(dst, 0, float32(sh*.3), float32(sw), float32(sh*.3), color.NRGBA{R: 13, G: 21, B: 34, A: 238}, false)
		label(message, sh*.35, 28)
		if s.ready {
			label("Left / Right: steer   Up: boost", sh*.44, 18)
			label("Space / Enter: action   Esc: pause", sh*.5, 18)
			label("Touch: drag to steer, double-tap for action", sh*.56, 14)
		}
		if button != "" {
			gfx.FillRoundRect(dst, float32(sw*.15), float32(sh-76), float32(sw*.7), 54, 8, color.NRGBA{R: 34, G: 83, B: 102, A: 255})
			label(button, sh-63, 22)
		}
		if s.paused {
			exit := "Exit game"
			if s.pauseExit {
				exit = "> Exit game"
			}
			label(exit, sh-120, 18)
		}
	} else {
		gfx.DrawText(dst, "Pause", sw-32, 14, 14, white)
	}
}

func runnerBoardRect(g *Game, d *data.RunnerDefinition) [4]float64 {
	sw, sh := g.screen()
	cell := math.Max(1, math.Min((sw-24)/float64(d.Width), (sh-150)/float64(d.ViewRows)))
	left, top := (sw-cell*float64(d.Width))/2, 50.0
	if v := d.Visuals; v != nil && len(v.Board) == 4 {
		b := runnerAuthoredRect(g, v.Board)
		left, top = b[0], b[1]
		cell = math.Min(b[2]/float64(d.Width), b[3]/float64(d.ViewRows))
	}
	return [4]float64{left, top, cell * float64(d.Width), cell * float64(d.ViewRows)}
}

func runnerBoardClip(board [4]float64, scale float64, bounds image.Rectangle) image.Rectangle {
	return image.Rect(int(math.Floor(board[0]*scale)), int(math.Floor(board[1]*scale)), int(math.Ceil((board[0]+board[2])*scale)), int(math.Ceil((board[1]+board[3])*scale))).Intersect(bounds)
}

func runnerPlayerRect(d *data.RunnerDefinition, x, left, top, cell float64) [4]float64 {
	w, h, dx, dy := .6, .6, 0.0, 0.0
	if d.PlayerImage != "" {
		w, h = 1, 1
	}
	if v := d.Visuals; v != nil {
		if len(v.PlayerSize) == 2 {
			w, h = v.PlayerSize[0], v.PlayerSize[1]
		}
		if len(v.PlayerOffset) == 2 {
			dx, dy = v.PlayerOffset[0], v.PlayerOffset[1]
		}
	}
	return [4]float64{left + (x+dx-w/2)*cell, top + (float64(d.PlayerRow)+.5+dy-h/2)*cell, w * cell, h * cell}
}

func (s *runnerScene) drawRows(g *Game, dst *ebiten.Image, left, top, cell float64) {
	d := s.request.Definition
	for row := -1; row < len(s.view.Rows); row++ {
		line := s.view.Ahead
		if row >= 0 {
			line = s.view.Rows[row]
		}
		for col, key := range line {
			tile := d.Tiles[string(key)]
			if tile.Kind == "empty" {
				continue
			}
			x, y := left+float64(col)*cell, top+(float64(row)+s.view.Scroll)*cell
			if tile.Image != "" {
				g.drawPicture(dst, data.Picture{File: tile.Image}, x, y, cell, cell, 1)
				continue
			}
			clr := color.NRGBA{R: 91, G: 115, B: 142, A: 255}
			if tile.Kind != "wall" {
				clr = color.NRGBA{R: 244, G: 196, B: 74, A: 255}
			}
			gfx.FillRect(dst, float32(x+1), float32(y+1), float32(math.Max(1, cell-2)), float32(math.Max(1, cell-2)), tile.Color.Or(clr), false)
		}
	}
}

func (s *runnerScene) react(events []casual.Event) {
	v := s.request.Definition.Visuals
	if v == nil {
		return
	}
	for _, e := range events {
		for _, key := range slices.Sorted(maps.Keys(v.Reactions)) {
			reaction := v.Reactions[key]
			name := reaction.Event
			if name == "" {
				name = key
			}
			matches := name == e.Kind || name == e.Kind+":"+e.Tile
			if !matches || reaction.Every > 0 && (e.Streak < reaction.Every || e.Streak%reaction.Every != 0) || s.portraitTicks > 0 && reaction.Priority < s.portraitPriority {
				continue
			}
			index := 0
			if reaction.Every > 0 {
				index = (e.Streak/reaction.Every - 1) % len(reaction.Images)
			}
			s.setPortrait(reaction.Images[index])
			s.portraitTicks = reaction.Ticks
			s.portraitPriority = reaction.Priority
		}
	}
}

func (s *runnerScene) setPortrait(image string) {
	if image != s.portrait {
		s.previousPortrait = s.portrait
		s.portrait = image
		s.portraitBlend = 0
	}
}

func (s *runnerScene) updatePortrait() {
	v := s.request.Definition.Visuals
	if v == nil {
		return
	}
	if s.portraitTicks > 0 {
		s.portraitTicks--
	}
	if s.portraitTicks == 0 && len(v.Idle) > 0 {
		width := s.request.Definition.Width
		for row := max(0, s.request.Definition.PlayerRow-6); row < s.request.Definition.PlayerRow; row++ {
			count := 0
			for _, tile := range s.view.Rows[row] {
				if s.request.Definition.Tiles[string(tile)].Kind != "wall" {
					count++
				}
			}
			width = min(width, count)
		}
		danger := 0
		switch {
		case width <= 3:
			danger = 3
		case width <= 4:
			danger = 2
		case width <= 5:
			danger = 1
		}
		if s.view.Lives == 0 {
			danger++
		}
		s.setPortrait(v.Idle[min(danger, len(v.Idle)-1)])
	}
	rate := .06
	if s.portraitTicks > 0 {
		rate = .25
	}
	s.portraitBlend = min(1, s.portraitBlend+rate)
}

// runnerAuthoredRect fits explicit art coordinates from the author's canvas
// into the current UI viewport with one aspect-preserving scale for both axes.
func runnerAuthoredRect(g *Game, rect []float64) [4]float64 {
	sw, sh := g.screen()
	w, h := float64(g.db.Game.Screen.Width), float64(g.db.Game.Screen.Height)
	scale := math.Min(sw/w, sh/h)
	return [4]float64{(sw-w*scale)/2 + rect[0]*scale, (sh-h*scale)/2 + rect[1]*scale, rect[2] * scale, rect[3] * scale}
}

func (s *runnerScene) drawPortrait(g *Game, dst *ebiten.Image) {
	v := s.request.Definition.Visuals
	if v == nil || len(v.Portrait) != 4 {
		return
	}
	b := runnerAuthoredRect(g, v.Portrait)
	if s.previousPortrait != "" && s.portraitBlend < 1 {
		g.drawPicture(dst, data.Picture{File: s.previousPortrait}, b[0], b[1], b[2], b[3], float32(1-s.portraitBlend))
	}
	if s.portrait != "" {
		g.drawPicture(dst, data.Picture{File: s.portrait}, b[0], b[1], b[2], b[3], float32(s.portraitBlend))
	}
}

func (s *runnerScene) drawActionWalls(g *Game, dst *ebiten.Image, left, top, cell float64) {
	v := s.request.Definition.Visuals
	if v == nil || s.view.ClearTicks <= 0 {
		return
	}
	elapsed := s.request.Definition.ClearTicks - s.view.ClearTicks
	for row := -1; row < len(s.preAction); row++ {
		line := s.preActionAhead
		if row >= 0 {
			line = s.preAction[row]
		}
		for col, key := range line {
			tile := s.request.Definition.Tiles[string(key)]
			if tile.Kind != "wall" {
				continue
			}
			if elapsed >= v.CutinTicks+(len(s.preAction)-1-row)*3 {
				continue
			}
			x, y := left+float64(col)*cell, top+(float64(row)+s.view.Scroll)*cell
			if tile.Image != "" {
				g.drawPicture(dst, data.Picture{File: tile.Image}, x, y, cell, cell, 1)
			} else {
				gfx.FillRect(dst, float32(x+1), float32(y+1), float32(cell-2), float32(cell-2), tile.Color.Or(color.NRGBA{R: 91, G: 115, B: 142, A: 255}), false)
			}
		}
	}
}

func (s *runnerScene) drawCutin(g *Game, dst *ebiten.Image) {
	v := s.request.Definition.Visuals
	if v == nil || s.view.ClearTicks <= 0 {
		return
	}
	elapsed := s.request.Definition.ClearTicks - s.view.ClearTicks
	if elapsed < v.CutinTicks && v.Cutin != "" {
		sw, sh := g.screen()
		height := math.Min(360, sh*.4)
		gfx.FillRect(dst, 0, float32(sh*.56-height/2), float32(sw), float32(height), color.NRGBA{R: 13, G: 21, B: 34, A: 220}, false)
		g.drawPicture(dst, data.Picture{File: v.Cutin}, 0, sh*.56-height/2, sw, height, 1)
	}
}
