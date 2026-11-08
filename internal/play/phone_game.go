package play

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// phoneGameRuntime retains one child and its resources while its host is suspended.
// It never owns another window or polls physical devices.
type phoneGameRuntime struct {
	appID   string
	game    *Game
	screen  *ebiten.Image
	resume  func()
	input   input
	blocked bool
}

func (g *Game) startPhoneGame(p *phoneView, id string) error {
	if g.embedded {
		return errors.New("embedded games cannot launch another game")
	}
	if p.child != nil {
		return errors.New("a phone game is already running")
	}
	installed, err := g.phoneInstalled(id)
	if err != nil {
		return err
	}
	now, err := g.phoneGameClock(installed)
	if err != nil {
		return errors.Join(err, g.releasePhonePackage(id))
	}
	sum := sha256.Sum256([]byte(g.db.Game.ID + "\x00" + installed.Release.ID + "\x00" + installed.Release.Version))
	store := phoneGameStore{Store: g.store, prefix: "phone-save-" + hex.EncodeToString(sum[:]) + "-"}
	resumeAudio := g.audioSession().Suspend()
	resumeSpeech := g.suspendPhoneSpeech()
	resume := func() { resumeAudio(); resumeSpeech() }
	child, err := newGame(Options{FS: installed.FS, Store: store, Now: func() time.Time { return now }, Seed: g.opts.Seed, NoSound: g.opts.NoSound, Audio: g.opts.Audio, recordedAudio: g.tr != nil || g.opts.recordedAudio, headless: g.opts.headless}, true)
	if err != nil {
		resume()
		return errors.Join(err, g.releasePhonePackage(id))
	}
	if child.db.Streams != nil {
		g.emit("phone_streams", traceLine{"id": installed.Release.ID})
	}
	p.child = &phoneGameRuntime{appID: id, game: child, resume: resume, blocked: true}
	child.in.supplied = &p.child.input
	child.textInput = func() { g.markTextInput() }
	child.inputCaret = func(cx, cy float64) (float64, float64) {
		x, y, w, h := g.phoneScreenRect(p.side)
		vx, vy, vw, vh := phoneGameRect(child, w, h)
		sw, sh := child.screen()
		return (x + vx + cx*vw/sw) * g.ui, (y + vy + cy*vh/sh) * g.ui
	}
	p.turn(installed.Release.Orientation == "landscape")
	return nil
}

// phoneGameStore projects exact-release save names onto the host store. The
// prefix is immutable, and filenames are validated before the prefix is added.
type phoneGameStore struct {
	Store
	prefix string
}

func (s phoneGameStore) name(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) || !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return s.prefix + name, nil
}

func (s phoneGameStore) Read(name string) ([]byte, error) {
	n, e := s.name(name)
	if e != nil {
		return nil, e
	}
	return s.Store.Read(n)
}

func (s phoneGameStore) Write(name string, b []byte) error {
	n, e := s.name(name)
	if e != nil {
		return e
	}
	return s.Store.Write(n, b)
}

func (s phoneGameStore) Remove(name string) error {
	n, e := s.name(name)
	if e != nil {
		return e
	}
	return s.Store.Remove(n)
}

func (s phoneGameStore) List() ([]string, error) {
	names, e := s.Store.List()
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, n := range names {
		if name, ok := strings.CutPrefix(n, s.prefix); ok {
			out = append(out, name)
		}
	}
	return out, nil
}

const phoneGameBar = 36.0

// phoneGameRect returns the letterboxed child viewport within the phone screen.
func phoneGameRect(child *Game, w, h float64) (x, y, cw, ch float64) {
	pw, ph := child.pixels()
	k := math.Min(w/pw, math.Max(1, h-phoneGameBar)/ph)
	cw, ch = pw*k, ph*k
	return (w - cw) / 2, phoneGameBar + (h-phoneGameBar-ch)/2, cw, ch
}

func (g *Game) updatePhoneGame(p *phoneView) error {
	c := p.child
	if c == nil {
		return nil
	}
	if !p.resting() {
		p.turned++
		return g.phoneGameResult(p, c.game.tick, c.game.audioSession().Tick())
	}
	x, y, w, h := g.phoneScreenRect(p.side)
	px, py := float64(g.in.mouse.X)-x, float64(g.in.mouse.Y)-y
	if (g.in.esc || g.in.pressed[btnCancel]) && g.in.shift || g.in.click && px >= 0 && px < w && py >= 0 && py < phoneGameBar {
		g.in.query(true)
		return errors.Join(g.stopPhoneGame(p), g.audioSession().Tick())
	}
	// Entry and exit must not reuse the press that selected the app.
	if c.blocked {
		held := g.in.pheld && !g.in.pdown
		for b, v := range g.in.held {
			held = held || v && !g.in.pressed[b]
		}
		if held {
			return g.phoneGameResult(p, c.game.tick, c.game.audioSession().Tick())
		}
		c.blocked = false
	}
	c.input = g.in
	c.input.physical = g.in.devices()
	c.input.script, c.input.replay, c.input.supplied = nil, nil, nil
	c.input.esc = g.in.esc
	vx, vy, vw, vh := phoneGameRect(c.game, w, h)
	sw, sh := c.game.screen()
	c.input.mouse = image.Pt(int((px-vx)*sw/vw), int((py-vy)*sh/vh))
	c.input.touchFrom = image.Pt(int((float64(g.in.touchFrom.X)-x-vx)*sw/vw), int((float64(g.in.touchFrom.Y)-y-vy)*sh/vh))
	c.input.swipe = g.in.swipe * sh / vh
	transformRunnerPointers(&c.input, &g.in, x+vx, y+vy, vw, vh, sw, sh)
	if px < vx || px >= vx+vw || py < vy || py >= vy+vh {
		c.input.click, c.input.rclick, c.input.pdown, c.input.pheld, c.input.pup = false, false, false, false, false
		c.input.wheel = 0
	}
	beforeTick := c.game.tick
	err := c.game.Update()
	g.in.readInput = c.game.in.idleRead
	return g.phoneGameResult(p, beforeTick, err)
}

// phoneGameResult preserves the host for failures from both normal updates and
// audio-only entry frames while closing all resources owned by the child.
func (g *Game) phoneGameResult(p *phoneView, beforeTick int, err error) error {
	c := p.child
	if errors.Is(err, ebiten.Termination) {
		return errors.Join(g.stopPhoneGame(p), g.audioSession().Tick())
	}
	if err != nil {
		closeErr := g.stopPhoneGame(p)
		p.gameError = fmt.Sprintf("Game stopped: %v", errors.Join(err, closeErr))
		if p.store != nil {
			p.store.message = p.gameError
			p.store.ready = false
		}
		if p.cue.App == "game" {
			p.closing = 0
		}
		if c.game.tick == beforeTick {
			return g.audioSession().Tick()
		}
		return nil
	}
	return nil
}

func (g *Game) stopPhoneGame(p *phoneView) error {
	c := p.child
	if c == nil {
		return nil
	}
	p.child = nil
	err := errors.Join(c.game.Close(), g.releasePhonePackage(c.appID))
	if c.screen != nil {
		c.screen.Deallocate()
	}
	if !g.closed {
		c.resume()
	}
	g.in.pressed, g.in.held = [numButtons]bool{}, [numButtons]bool{}
	g.in.click, g.in.rclick, g.in.pdown, g.in.pheld, g.in.pup = false, false, false, false, false
	g.in.fast, g.in.esc = false, false
	p.turn(p.cue.Hold == "landscape")
	if p.cue.App == "game" {
		p.putDown(-4, -4, false)
	}
	return err
}

func (g *Game) drawPhoneGame(p *phoneView, dst *ebiten.Image, w, h float64) {
	c := p.child
	if c == nil {
		drawPhoneGameError(dst, w, h, p.gameError)
		return
	}
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), phoneBody, false)
	pw, ph := c.game.pixels()
	c.screen = sized(c.screen, int(pw), int(ph))
	c.screen.Clear()
	c.game.Draw(c.screen)
	x, y, cw, ch := phoneGameRect(c.game, w, h)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(cw/pw*gfx.Scale(), ch/ph*gfx.Scale())
	op.GeoM.Translate(x*gfx.Scale(), y*gfx.Scale())
	dst.DrawImage(c.screen, op)
	// Host controls are composed last and remain operable independently of child fonts.
	gfx.FillRect(dst, 0, 0, float32(w), float32(phoneGameBar), phoneBody, false)
	gfx.StrokeLine(dst, 20, 18, 28, 10, 2, phoneOnVideo, true)
	gfx.StrokeLine(dst, 20, 18, 28, 26, 2, phoneOnVideo, true)
	gfx.StrokeLine(dst, 20, 18, 40, 18, 2, phoneOnVideo, true)
	gfx.DrawText(dst, "Back (Shift+Esc)", w/2+12, 10, 16, phoneOnVideo)
}

func (g *Game) openPhoneGame(p *phoneView) {
	if p.gameOpened {
		return
	}
	p.gameOpened = true
	if err := g.startPhoneGame(p, p.cue.Game); err != nil {
		p.gameError = fmt.Sprintf("Cannot open game: %v", err)
	}
}

// addPhoneGameApps appends only installed, visible approved apps. Authored rows
// and their event result indices stay fixed, including the dismiss result.
func (g *Game) addPhoneGameApps(p *phoneView) {
	if p.appRows != nil || g.db.Game.Phone == nil {
		return
	}
	p.homeResults = p.cue.Results()
	p.appRows = map[int]string{}
	p.cue.Rows = append([]data.PhoneRow(nil), p.cue.Rows...)
	for _, app := range g.db.Game.Phone.Apps {
		release, err := g.phoneListed(app.ID)
		if err != nil {
			continue
		}
		p.appRows[len(p.cue.Rows)] = app.ID
		p.cue.Rows = append(p.cue.Rows, data.PhoneRow{Button: release.Title, Item: &data.PhoneItem{Icon: "installed:" + app.ID, Title: release.Title}})
	}
	p.rows = nil
}

func drawPhoneGameError(dst *ebiten.Image, w, h float64, message string) {
	if message == "" {
		return
	}
	lines, size := gfx.FitLines(message, 16, w-24, 3)
	top := h - float64(len(lines))*gfx.LineHeight(size) - 18
	gfx.FillRect(dst, 0, float32(top-8), float32(w), float32(h-top+8), phoneScreen, false)
	for i, line := range lines {
		gfx.DrawText(dst, line, w/2, top+float64(i)*gfx.LineHeight(size), size, phoneInk)
	}
}

// speechWatch finds this runtime's synthesized voice even under its own menu.
func (g *Game) speechWatch() *streamWatch {
	for _, scene := range slices.Backward(g.stack) {
		if w, ok := scene.(*streamWatch); ok {
			return w
		}
	}
	return nil
}

func (g *Game) stopOwnedSpeech() {
	if w := g.speechWatch(); w != nil && w.spoken != nil {
		done := w.spoken
		streamStopSpeak()
		done()
		w.spoken = nil
	}
}

// Browser speech cannot retain two paused utterances on one device. Restart the
// parent's current caption on return, only if that same watch still owns it.
func (g *Game) suspendPhoneSpeech() func() {
	w := g.speechWatch()
	if w == nil || w.spoken == nil {
		return func() {}
	}
	caption, who := w.caption, w.captionWho
	g.stopOwnedSpeech()
	return func() {
		if g.speechWatch() == w && w.caption == caption && !w.ended && g.desk != nil && g.desk.speak {
			w.readAloud(g, who, caption)
		}
	}
}
