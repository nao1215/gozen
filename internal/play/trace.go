package play

import (
	"bufio"
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"image"
	"io"
	"io/fs"
	"log"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/memo"
	"github.com/nao1215/gozen/internal/layer1/traceformat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/savefile"
	lang "github.com/nao1215/gozen/internal/text"
)

type tracer struct {
	autoWaiting string
	performance *tracePerformance
	// inAct suppresses redundant panel mutation records already replayed by act.
	inAct     bool
	focusLost bool
	focusTick int
	mouse     image.Point
	viewport  traceformat.Viewport
	enc       *lineWriter
	lines     int
	lastTick  int
	drawTick  int // Last drawn tick; zero is the state before the first update.
	err       error

	scenes string
	music  string

	wantSong string
	wantAt   int
	waiting  bool

	stand        *standFrame
	standPending string
	standN       int
	standAt      int
	standLast    string
	menu         string
	view         string

	// layout gathers the frame's figures and texts (trace_layout.go).
	layout        *layoutFrame
	layoutPending string
	layoutN       int
	layoutAt      int
	layoutLast    string
	screenSeed    maphash.Seed
	screenSeen    bool
	screenDigest  uint64

	pending   string
	pendingN  int
	pendingAt int

	drawn   map[string]bool
	missing map[string]bool
	lists   []traceList
	ended   bool
	battles int
}

type traceList struct {
	Items []string `json:"items"`
	Sel   int      `json:"sel"`
}

type traceLine = map[string]any

const traceVersion = 2

const initialSaveLimit = 8 << 20

func newTracer(w io.Writer) *tracer {
	t := &tracer{}
	if w != nil {
		t.enc = &lineWriter{w: w}
	}
	return t
}

type lineWriter struct{ w io.Writer }

func (lw *lineWriter) Encode(l traceLine) error {
	var b strings.Builder
	b.WriteString("{")
	keys := []string{"t", "tick"}
	for _, k := range sortedKeys(boolKeys(l)) {
		if k != "t" && k != "tick" {
			keys = append(keys, k)
		}
	}
	first := true
	for _, k := range keys {
		v, ok := l[k]
		if !ok {
			continue
		}
		var vb strings.Builder
		enc := json.NewEncoder(&vb)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return err
		}
		if !first {
			b.WriteString(",")
		}
		first = false
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		b.Write(kb)
		b.WriteString(":")
		b.WriteString(strings.TrimSuffix(vb.String(), "\n"))
	}
	b.WriteString("}\n")
	if b.Len() >= traceformat.LineLimit {
		return errors.New("trace record exceeds the 16 MiB line limit")
	}
	n, err := io.WriteString(lw.w, b.String())
	if err == nil && n != b.Len() {
		return io.ErrShortWrite
	}
	return err
}

func boolKeys(l traceLine) map[string]bool {
	m := map[string]bool{}
	for k := range l {
		m[k] = true
	}
	return m
}

func (g *Game) emit(kind string, fields traceLine) {
	if g.tr == nil {
		return
	}
	if g.tr.inAct && (kind == "dev_set" || kind == "dev_do" || kind == "jump") {
		return
	}
	line := traceLine{"t": kind, "tick": g.tick}
	maps.Copy(line, fields)
	// Editors can freeze the simulation after this update has emitted actions.
	// Keep their boundary and subsequent observations at the latest record tick.
	g.tr.lastTick = max(g.tr.lastTick, int(traceformat.Number(line["tick"])))
	line["tick"] = g.tr.lastTick
	g.tr.lines++
	if g.tr.enc != nil && g.tr.err == nil {
		g.tr.err = g.tr.enc.Encode(line)
	}
	if g.rep != nil {
		g.rep.check(g, line)
	}
}

func (g *Game) traceStart() error {
	if g.tr == nil {
		return nil
	}
	o := g.opts
	fingerprint, err := traceFingerprint(g.opts.FS)
	if err != nil {
		return err
	}
	setup := traceSetup{Config: g.cfg, Read: g.read, Unlocked: g.unlocked}
	setup.Resources, err = g.tracePhoneResources()
	if err != nil {
		return err
	}
	if g.meter.on {
		setup.Unsupported = append(setup.Unsupported, "frame meter")
	}
	setup.Saves, err = g.traceSaves()
	if err != nil {
		return err
	}
	if o.FromMemo != "" {
		m, b, err := memo.Read(o.FromMemo)
		if err != nil {
			return err
		}
		setup.Memo = &traceMemo{Memo: m, Save: b, Path: o.FromMemo, AllowLegacy: o.AllowLegacySave}
	}
	if g.opts.Trace != nil {
		setup.SaveData = map[string][]byte{}
		setup.SaveTimes = map[string]time.Time{}
		for name := range setup.Saves {
			b, err := g.traceSaveRead(name)
			if errors.Is(err, gamepackage.ErrSizeLimit) {
				setup.Unsupported = append(setup.Unsupported, fmt.Sprintf("initial phone package %q exceeds the 8 MiB encoded snapshot limit", name))
				continue
			}
			if err != nil {
				return err
			}
			setup.SaveData[name] = b
			if len(b) > initialSaveLimit {
				setup.Unsupported = append(setup.Unsupported, fmt.Sprintf("initial save %q exceeds the 8 MiB limit", name))
			}
			if t, ok := g.storeTime(name); ok {
				setup.SaveTimes[name] = t
			}
		}
	}
	if setup.Memo != nil && len(setup.Memo.Save) > initialSaveLimit {
		setup.Memo = nil
		setup.Unsupported = append(setup.Unsupported, "initial memo save exceeds the 8 MiB limit")
	}
	if o.Replay != "" {
		setup.Unsupported = append(setup.Unsupported, "recording a replay")
	}
	if o.State != nil {
		setup.Unsupported = append(setup.Unsupported, "injected state")
	}
	initial := setup.SaveData
	if initial != nil {
		setup.SaveData = map[string][]byte{}
	}
	g.emit("start", traceLine{
		"game_dir": o.GameDir, "scripted": o.Script != "", "locales": o.Locales, "book_as_played": o.BookAsPlayed,
		"exact_version": 3, "version": traceVersion, "game": g.db.Game.Title, "language": cmp.Or(g.db.Game.Language, lang.English), "seed": o.Seed,
		"window": o.Window, "fullscreen": g.cfg.Fullscreen, "development": o.Development,
		"screen":   []int{g.db.Game.Screen.Width, g.db.Game.Screen.Height},
		"location": o.Location, "party": o.Party, "level": o.Level, "flags": o.Flags, "gold": o.Gold, "items": o.Items, "calm": o.Calm,
		"floor": o.Floor, "battle": o.Battle, "speed": max(1, o.Speed), "gallery_all": o.GalleryAll,
		"scene": o.Scene, "event": o.Event, "line": o.Line, "picture": o.Picture, "answers": o.Answers,
		"autopilot": g.isAutopilot(), "fingerprint": fingerprint, "setup": setup,
	})
	if g.opts.Trace != nil {
		for _, name := range slices.Sorted(maps.Keys(initial)) {
			if len(initial[name]) <= initialSaveLimit {
				g.emit("initial_save", traceLine{"file": name, "data": initial[name]})
			}
		}
		g.store = &traceStore{Store: g.store, game: g, initial: initial, times: setup.SaveTimes}
	}
	return g.tr.err
}

// traceStore detects external slot edits at the I/O boundary. Only recordings
// install it; ordinary play pays no additional read or per-frame cost.
type traceStore struct {
	Store
	game    *Game
	initial map[string][]byte
	times   map[string]time.Time
}

func (s *traceStore) slot(name string) bool {
	for i := range saveSlots + 1 {
		if name == s.game.slotPath(i) {
			return true
		}
	}
	return false
}

func (s *traceStore) Read(name string) ([]byte, error) {
	b, err := s.Store.Read(name)
	if s.slot(name) {
		want, exists := s.initial[name]
		if (exists && (err != nil || !bytes.Equal(b, want))) ||
			(!exists && !errors.Is(err, fs.ErrNotExist)) {
			s.game.emit("external_save", nil)
		}
	}
	return b, err
}

func (s *traceStore) Write(name string, b []byte) error {
	err := s.Store.Write(name, b)
	if err == nil && s.slot(name) {
		s.initial[name] = bytes.Clone(b)
		if st, ok := s.Store.(interface {
			ModTime(string) (time.Time, error)
		}); ok {
			if stamp, err := st.ModTime(name); err == nil {
				s.times[name] = stamp
			} else {
				delete(s.times, name)
			}
		}
	}
	return err
}

func (s *traceStore) ModTime(name string) (time.Time, error) {
	var t time.Time
	err := fs.ErrNotExist
	if st, ok := s.Store.(interface {
		ModTime(string) (time.Time, error)
	}); ok {
		t, err = st.ModTime(name)
	}
	if s.slot(name) {
		want, exists := s.times[name]
		if (exists && (err != nil || !want.Equal(t) || want.Format("01/02 15:04") != t.Format("01/02 15:04"))) || (!exists && err == nil) {
			s.game.emit("external_save", nil)
		}
	}
	return t, err
}

func traceFingerprint(fsys fs.FS) (string, error) {
	if fsys == nil {
		return "", nil
	}
	h := sha256.New()
	add := func(name string) error {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(h, "%d:%s:%d:", len(name), name, len(b)); err != nil {
			return err
		}
		_, err = h.Write(b)
		return err
	}
	if err := add("game.yaml"); err != nil {
		return "", err
	}
	err := fs.WalkDir(fsys, "data", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			return add(name)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// traceResources fingerprints fonts, pictures, sounds, metadata and translations
// once at startup. These files must stay immutable during a recorded run.
func traceResources(fsys fs.FS) (string, error) {
	h := sha256.New()
	if fsys != nil {
		for _, root := range []string{"assets", "lang"} {
			err := fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
				if errors.Is(err, fs.ErrNotExist) && name == root {
					return nil
				}
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				b, err := fs.ReadFile(fsys, name)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(h, "%d:%s:%d:", len(name), name, len(b)); err != nil {
					return err
				}
				_, err = h.Write(b)
				return err
			})
			if err != nil {
				return "", err
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type traceMemo struct {
	Memo        memo.Memo `json:"memo"`
	Save        []byte    `json:"save"`
	Path        string    `json:"path"`
	AllowLegacy bool      `json:"allow_legacy,omitempty"`
}

type traceSetup struct {
	Resources   string               `json:"resources,omitempty"`
	SaveData    map[string][]byte    `json:"save_data"`
	SaveTimes   map[string]time.Time `json:"save_times,omitempty"`
	Memo        *traceMemo           `json:"memo,omitempty"`
	Config      config               `json:"config"`
	Read        map[uint64]bool      `json:"read"`
	Unlocked    map[string]bool      `json:"unlocked"`
	Saves       map[string]string    `json:"saves"`
	Unsupported []string             `json:"unsupported,omitempty"`
}

func (g *Game) traceSaves() (map[string]string, error) {
	out := map[string]string{}
	if g.db.FS == nil && g.opts.FS == nil {
		return out, nil
	}
	names := make([]string, 0, saveSlots+1)
	for i := range saveSlots + 1 {
		names = append(names, g.slotPath(i))
	}
	if g.db.Game.Phone != nil {
		extra, err := g.store.List()
		if err != nil {
			return nil, err
		}
		for _, name := range extra {
			if strings.HasPrefix(name, "phone-save-") || strings.HasPrefix(name, "phone-package-") || name == "phone-apps.json" || name == "phone-catalog.json" {
				names = append(names, name)
			}
		}
	}
	virtual, e := g.tracePhoneArchiveNames()
	if e != nil {
		return nil, e
	}
	names = append(names, virtual...)
	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		b, err := g.traceSaveRead(name)
		if errors.Is(err, gamepackage.ErrSizeLimit) {
			out[name] = "oversized phone package"
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	return out, nil
}

// buttonNames is the canonical table for trace encoding, validation and replay.
// TestTraceButtonsRoundTrip requires a unique nonempty name for every button.
var buttonNames = [numButtons]string{
	btnUp: "up", btnDown: "down", btnLeft: "left", btnRight: "right", btnOK: "ok", btnCancel: "cancel",
	btnMenu: "menu", btnLog: "log", btnAuto: "auto", btnHide: "hide", btnConfig: "config", btnNext: "next",
	btnMemo: "memo", btnReload: "reload", btnTag: "tag", btnGraphs: "graphs", btnSkip: "skip", btnSceneSkip: "scene_skip",
	btnBack: "back", btnSave: "save", btnLoad: "load", btnPanel: "panel",
	btnBook: "characters", btnBestiary: "bestiary", btnCaptions: "captions",
	btnMapZoomIn: "map_zoom_in", btnMapZoomOut: "map_zoom_out", btnMapOverview: "map_overview",
	btnMapFocus: "map_focus", btnMapPan: "map_pan", btnEndTurn: "end_turn_key", btnInbox: "inbox",
}

func (g *Game) traceInput(in *input) {
	if g.tr == nil {
		return
	}
	var press, held []string
	for b := range numButtons {
		if in.pressed[b] {
			press = append(press, buttonNames[b])
		}
		if in.held[b] {
			held = append(held, buttonNames[b])
		}
	}
	if press == nil && held == nil && !in.fast && !in.esc && !in.shift && !in.click && !in.rclick && !in.moved && in.wheel == 0 && in.digit == 0 && !in.pdown && !in.pheld && !in.pup && !in.longPress && in.mouse == g.tr.mouse {
		return
	}
	g.tr.mouse = in.mouse
	l := traceLine{"moved": in.moved, "mouse": []int{in.mouse.X, in.mouse.Y}}
	if in.longPress {
		l["long_press"] = true
	}
	if in.digit != 0 {
		l["digit"] = in.digit
	}
	if press != nil {
		l["press"] = press
	}
	if held != nil {
		l["held"] = held
	}
	if in.fast {
		l["fast"] = true
	}
	if in.esc {
		l["esc"] = true
	}
	if in.shift {
		l["shift"] = true
	}
	if in.click {
		l["click"] = true
	}
	if in.rclick {
		l["rclick"] = true
	}
	// A press goes down with a click unless a finger began it; only the
	// difference is written.
	if in.pdown != in.click {
		l["pointer_down"] = in.pdown
	}
	if in.pheld {
		l["pointer_held"] = true
	}
	if in.pup {
		l["pointer_up"] = true
	}
	if in.wheel != 0 {
		l["wheel"] = in.wheel
	}
	g.emit("input", l)
}

func sceneName(s scene) string {
	n := reflect.TypeOf(s).Elem().Name()
	return strings.TrimSuffix(n, "Scene")
}

func (g *Game) traceTick() {
	if g.tr == nil {
		return
	}
	var names []string
	for _, s := range g.stack {
		names = append(names, sceneName(s))
	}
	if g.logView != nil {
		names = append(names, "backlog")
	}
	if key := strings.Join(names, ">"); key != g.tr.scenes {
		g.tr.scenes = key
		l := traceLine{"stack": names}
		if g.st != nil {
			l["location"] = g.st.Location
		}
		g.emit("scene", l)
		g.traceState()
	} else if g.tick%600 == 0 {
		g.traceState()
	}

	mood := g.baseMood
	if g.moodOverride != "" {
		mood = g.moodOverride
	} else if g.lastMood != "" {
		mood = g.lastMood
	}
	song, module := g.audioSession().Playing()
	wanted := g.audioSession().WantedSong()
	tr := g.tr
	if wanted != tr.wantSong {
		tr.wantSong, tr.wantAt, tr.waiting = wanted, g.tick, wanted != "" && wanted != song
	}
	if key := song + "/" + module + "|" + g.baseSong + "/" + mood + "|" + wanted; key != tr.music {
		tr.music = key
		l := traceLine{"song": song, "module": module, "wanted": wanted, "asked": g.baseSong, "mood": mood}
		if wanted != "" {
			l["asked_at"] = tr.wantAt
		}
		if tr.waiting && song == wanted {
			tr.waiting = false
			l["started"] = g.tick
			l["wait"] = g.tick - tr.wantAt
		}
		g.emit("music", l)
	}
}

func (g *Game) traceState(terminal ...bool) {
	if g.st == nil {
		return
	}
	st := g.st
	var party []traceLine
	for _, a := range st.Members() {
		hp, mp, states := a.HP, a.MP, a.States
		if u := g.battleUnit(a.ID); u != nil {
			hp, mp, states = u.HP, u.MP, u.States
		}
		party = append(party, traceLine{
			"id": a.ID, "name": a.Name(), "lv": a.Level, "hp": hp, "mhp": a.Stat("mhp"),
			"mp": mp, "mmp": a.Stat("mmp"), "back": a.Back, "states": states,
		})
	}
	l := traceLine{"location": st.Location, "gold": st.Gold, "party": party, "summary": g.traceSummary()}
	if loc := g.db.Locations[st.Location]; loc != nil && loc.Dungeon != nil {
		l["pos"] = []int{st.Pos.X, st.Pos.Y}
		l["facing"] = data.Facings[st.Pos.Facing&3]
		if loc.Dungeon.Space != nil {
			l["level"] = st.Pos.Level
			if p := st.SpacePose; loc.Dungeon.Space.FreeMove != nil && p != nil {
				l["space_pose"] = traceLine{"x": p.X, "z": p.Z, "yaw": p.Yaw, "level": p.Level, "travel": p.Travel, "encounter": p.Encounter}
			}
		}
		if f, ok := st.Vars[dungeon.VarFloor]; ok {
			l["floor"] = f
		}
	}
	if len(terminal) > 0 {
		l["terminal"] = true
	}
	g.emit("state", l)
}

func (g *Game) traceRequest(r event.Request) { g.traceRequestIn(r, false) }

// traceBattleRequest traces a line, choice or notice of a troop event. Its say lines carry
// in_battle, so a replay of a trace written before battle lines were traced can tell them
// apart (replay.check).
func (g *Game) traceBattleRequest(r event.Request) { g.traceRequestIn(r, true) }

func (g *Game) traceRequestIn(r event.Request, inBattle bool) {
	if g.tr == nil {
		return
	}
	mark := func(l traceLine) traceLine {
		l["event"] = g.evName
		if inBattle {
			l["in_battle"] = true
		}
		return l
	}
	switch r := r.(type) {
	case event.Say:
		l := traceLine{"text": r.Text, "read": g.isRead(r)}
		if r.Name != "" {
			l["who"] = r.Name
		}
		if r.Face != "" {
			l["face"] = r.Face
		}
		g.emit("say", mark(l))
	case event.Choice:
		g.emit("say", mark(traceLine{"text": r.Prompt, "choices": r.Options}))
	case event.Notice:
		g.emit("notice", mark(traceLine{"text": r.Text}))
	case event.Battle:
		g.emit("request", traceLine{"what": "battle", "troop": r.Troop})
	case event.Shop:
		g.emit("request", traceLine{"what": "shop", "id": r.ID})
	case event.Save:
		g.emit("request", traceLine{"what": "save"})
	case event.Open:
		g.emit("request", traceLine{"what": "open", "screen": r.Screen})
	case event.Clear:
		g.emit("request", traceLine{"what": "clear"})
	case event.GameOver:
		g.emit("request", traceLine{"what": "game_over"})
	case event.ClashScene:
		var lost [2]int
		for side := range 2 {
			for _, u := range r.Clash.Sides[side] {
				lost[side] += int(u.Max - u.Troops)
			}
		}
		g.emit("clash", traceLine{"region": r.Clash.Region, "result": r.Clash.Result, "rounds": r.Clash.Rounds, "lost": lost[0], "foe_lost": lost[1]})
	}
}

// traceDrawGap writes only exceptional cadence, with no allocation on ordinary frames.
// through is the last tick that ran without drawing (inclusive).
func (g *Game) traceDrawGap(through int) {
	if g.tr == nil || g.tr.ended {
		return
	}
	if first := g.tr.drawTick + 1; first <= through {
		g.emit("draw_skip", traceLine{"first": first, "last": through})
	}
	g.tr.drawTick = g.tick
}

// skipDraw keeps draw-owned presentation transitions on their recorded ticks.
// Traces without draw_skip records retain the legacy draw-every-tick behavior.
func (r *replay) skipDraw(tick int) bool {
	i, _ := slices.BinarySearchFunc(r.drawSkips, tick, func(span [2]int, tick int) int {
		return cmp.Compare(span[1], tick)
	})
	return i < len(r.drawSkips) && r.drawSkips[i][0] <= tick
}

func (g *Game) traceFrameStart() {
	if g.tr == nil {
		return
	}
	g.tr.drawn, g.tr.missing, g.tr.lists = map[string]bool{}, map[string]bool{}, nil
	g.tr.stand = &standFrame{}
	if g.tr.enc != nil && (g.opts.capture == nil || g.opts.capture.structure) {
		g.tr.layout = &layoutFrame{}
	}
	g.pics.seen = func(path string, ok bool) {
		g.tr.drawn[path] = true
		if !ok {
			g.tr.missing[path] = true
		}
	}
}

func (g *Game) traceFrameEnd(screen *ebiten.Image) {
	if g.tr == nil {
		return
	}
	g.pics.seen = nil
	if g.tr.layout != nil {
		if g.tr.stand != nil {
			g.tr.layout.cast = g.tr.stand.figures
			g.tr.layout.speaker = g.tr.stand.speaker
		}
		if g.phone != nil {
			g.tr.layout.phone = g.phone.cue.App
			g.tr.layout.phoneCall = g.phone.calling()
		}
		if g.talk.active && !g.talk.hidden {
			g.tr.layout.speaker = g.talk.say.Name
		}
	}

	g.traceStandEnd()
	g.traceLayoutEnd(screen)

	drawn := sortedKeys(g.tr.drawn)
	key := strings.Join(drawn, "|")
	if key != g.tr.pending {
		g.tr.pending, g.tr.pendingN, g.tr.pendingAt = key, 0, g.tick
	}
	g.tr.pendingN++
	if g.tr.pendingN == 10 && key != g.tr.view {
		g.tr.view = key
		l := traceLine{"pictures": drawn, "since": g.tr.pendingAt}
		if len(g.tr.missing) > 0 {
			l["missing"] = sortedKeys(g.tr.missing)
		}
		if g.opts.TraceShots {

			name := fmt.Sprintf("view_%06d", g.tick)
			l["shot"] = g.traceShotPath(name)
			g.saveShotQuiet(screen, name)
		}
		g.emit("view", l)
	}

	var active *traceList
	if len(g.tr.lists) > 0 {
		active = &g.tr.lists[len(g.tr.lists)-1]
	}
	key = ""
	if active != nil {
		key = fmt.Sprint(active.Items, active.Sel)
	}
	if key != g.tr.menu {
		g.tr.menu = key
		if active != nil {
			g.emit("menu", traceLine{"items": active.Items, "sel": active.Sel})
		} else {
			g.emit("menu", traceLine{"items": nil})
		}
	}
}

func (g *Game) traceList(l *list) {
	if g.tr == nil || g.tr.drawn == nil {
		return
	}
	tl := traceList{Sel: l.sel}
	for _, it := range l.items {
		s := it.label
		if it.right != "" {
			s += " " + it.right
		}
		if it.disabled {
			s += " (x)"
		}
		tl.Items = append(tl.Items, s)
	}
	g.tr.lists = append(g.tr.lists, tl)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func (g *Game) isAutopilot() bool {
	return g.auto != nil || g.opts.Autopilot != nil || (g.rep != nil && g.rep.autopilot)
}

type replay struct {
	phoneLaunches   map[int]phoneLaunchClock
	startupViewport *traceformat.Viewport
	drawSkips       [][2]int
	saveWrites      map[int]map[string][]*time.Time
	scripted        bool
	// now is the time the recorded run read for its streams, at its offset from UTC (the
	// "now" line).
	now         time.Time
	picks       map[int]traceLine
	pickTick    int
	inputs      map[int]traceLine
	acts        map[int][]traceLine
	expect      map[string][]traceLine
	at          map[string]int
	matched     int
	diverge     string
	version     int
	partial     bool
	limit       int
	end         traceLine
	fingerprint string
	setup       traceSetup
	autopilot   bool
	finished    bool
	capture     *battleCapture
	battleSays  bool // the trace has say lines marked in_battle
}

// expectLine keeps a checked line of the trace to compare with the replay.
func (r *replay) expectLine(k string, l traceLine) {
	if k == "say" && l["in_battle"] == true {
		r.battleSays = true
	}
	r.expect[k] = append(r.expect[k], l)
}

var replayChecked = []string{"say", "scene", "battle", "screen_action"}

func (r *replay) checked(k string) bool {
	if r.capture != nil {
		return slices.Contains(battleReplayChecked, k)
	}
	return slices.Contains(replayChecked, k) || (r.version == traceVersion && k == "state")
}

func loadReplay(path string, o *Options) (_ *replay, err error) {
	if o.ReplayBattle > 0 {
		return loadBattleReplay(path, o)
	}
	f, err := os.Open(path) //nolint:gosec // The caller explicitly selects the trace input file.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	r := &replay{picks: map[int]traceLine{}, saveWrites: map[int]map[string][]*time.Time{}, inputs: map[int]traceLine{}, acts: map[int][]traceLine{}, expect: map[string][]traceLine{}, at: map[string]int{}, partial: o.Until > 0}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), traceformat.LineLimit)
	if o.Until < 0 {
		return nil, errors.New(lang.T("--until は正のティックを指定する"))
	}
	lines, lastTick := 0, 0
	var start traceLine
	var exact traceformat.ExactReplay
	var reloads []int
	var startOptions struct {
		Development  *bool       `json:"development"`
		Window       float64     `json:"window"`
		Fullscreen   bool        `json:"fullscreen"`
		Seed         uint64      `json:"seed"`
		Version      int         `json:"version"`
		Speed        int         `json:"speed"`
		Fingerprint  string      `json:"fingerprint"`
		Setup        *traceSetup `json:"setup"`
		Scene        string      `json:"scene"`
		Event        string      `json:"event"`
		Line         int         `json:"line"`
		Picture      string      `json:"picture"`
		Answers      []int       `json:"answers"`
		Autopilot    bool        `json:"autopilot"`
		GalleryAll   bool        `json:"gallery_all"`
		Scripted     bool        `json:"scripted"`
		Locales      []string    `json:"locales"`
		BookAsPlayed bool        `json:"book_as_played"`
	}
	for sc.Scan() {
		lines++
		var l traceLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("replay %s: %w", path, err)
		}
		var header struct {
			Kind string `json:"t"`
			Tick *int   `json:"tick"`
		}
		if err := json.Unmarshal(sc.Bytes(), &header); err != nil {
			return nil, err
		}
		if header.Tick == nil || *header.Tick < lastTick || header.Kind == "" || r.end != nil {
			return nil, fmt.Errorf(lang.T("replay %s の %d 行目で順序・終端・ティックが不正"), path, lines)
		}
		tick, k := *header.Tick, header.Kind
		lastTick = tick
		if lines == 1 && (k != "start" || tick != 0) {
			return nil, fmt.Errorf(lang.T("replay %s に先頭の開始行がない"), path)
		}
		if r.partial && tick < o.Until {
			if err := exact.Check(k, tick, sc.Bytes()); err != nil {
				return nil, err
			}
		}
		switch k {
		case "start":
			if lines != 1 {
				return nil, fmt.Errorf(lang.T("replay %s の開始行が重複"), path)
			}

			if err := json.Unmarshal(sc.Bytes(), &startOptions); err != nil {
				return nil, err
			}
			start = l
			r.autopilot = startOptions.Autopilot
			r.scripted = startOptions.Scripted
			r.version = startOptions.Version
			if r.version != 1 && r.version != traceVersion {
				return nil, fmt.Errorf(lang.T("replay の記録版 %d には対応していない"), r.version)
			}
			if r.version == 1 && !r.partial {
				return nil, errors.New(lang.T("旧記録は前提条件を検証できない。--until で区間を指定する"))
			}
			if r.version == traceVersion {
				if startOptions.Setup == nil {
					return nil, errors.New(lang.T("replay の開始設定が欠落"))
				}
				r.setup, r.fingerprint = *startOptions.Setup, startOptions.Fingerprint
				if len(r.setup.Unsupported) > 0 {
					return nil, fmt.Errorf(lang.T("replay で再現できない開始条件 %v"), r.setup.Unsupported)
				}
			}
		case "viewport":
			if r.partial && tick <= 1 && tick < o.Until {
				var viewport traceformat.Viewport
				if err := json.Unmarshal(sc.Bytes(), &viewport); err != nil {
					return nil, err
				}
				r.startupViewport = &viewport
			}
		case "draw_skip":
			var span struct {
				First int `json:"first"`
				Last  int `json:"last"`
			}
			if err := json.Unmarshal(sc.Bytes(), &span); err != nil {
				return nil, err
			}
			if span.First < 1 || span.Last < span.First || span.Last > tick ||
				(len(r.drawSkips) > 0 && span.First <= r.drawSkips[len(r.drawSkips)-1][1]) {
				return nil, errors.New("invalid draw_skip interval")
			}
			r.drawSkips = append(r.drawSkips, [2]int{span.First, span.Last})
		case "initial_save":
			var save struct {
				File string `json:"file"`
				Data []byte `json:"data"`
			}
			if err := json.Unmarshal(sc.Bytes(), &save); err != nil {
				return nil, err
			}
			if tick != 0 || r.setup.SaveData == nil || len(save.Data) > initialSaveLimit {
				return nil, errors.New("invalid initial save record")
			}
			if _, exists := r.setup.SaveData[save.File]; exists {
				return nil, errors.New("duplicate initial save record")
			}
			r.setup.SaveData[save.File] = save.Data
		case "unsupported_script", "battle_setup":
			if !r.partial {
				return nil, fmt.Errorf("cannot reproduce exactly: %s at tick %d", k, tick)
			}
		case "live_reload":
			reloads = append(reloads, tick)
		case "end":
			r.end = l
			if r.version == traceVersion {
				var end struct {
					Lines *int            `json:"lines"`
					State json.RawMessage `json:"state"`
				}
				if err := json.Unmarshal(sc.Bytes(), &end); err != nil {
					return nil, err
				}
				if end.Lines == nil || *end.Lines != lines {
					return nil, errors.New(lang.T("replay の総行数が終端と一致しない"))
				}
				if len(end.State) == 0 {
					return nil, errors.New(lang.T("replay の終端状態が欠落"))
				}
			}
		case "input":
			if tick == 0 {
				return nil, errors.New(lang.T("replay の入力は１ティック以降でなければならない"))
			}
			for _, field := range []string{"press", "held"} {
				for _, name := range traceformat.Strings(l[field]) {
					if name == "" {
						return nil, fmt.Errorf("replay: empty button name in %s at tick %d; older recordings lost the button identity; record a new trace", field, tick)
					}
					if !slices.Contains(buttonNames[:], name) {
						return nil, fmt.Errorf(lang.T("replay の未知のボタン %q"), name)
					}
				}
			}
			if _, exists := r.inputs[tick]; exists {
				return nil, fmt.Errorf(lang.T("replay の入力ティック %d が重複"), tick)
			}
			r.inputs[tick] = l
		case "pick":
			index, ok := l["index"].(float64)
			on, onOK := l["on"].(string)
			_, labelOK := l["label"].(string)
			if tick <= 0 || !ok || index < 0 || index != float64(int(index)) || !onOK || on == "" || !labelOK || r.picks[tick] != nil {
				return nil, errors.New("replay: invalid pick record")
			}
			r.picks[tick] = l
		case "act":
			if tick == 0 {
				return nil, errors.New(lang.T("replay の直接操作は１ティック以降でなければならない"))
			}
			name, ok := l["name"].(string)
			if !ok {
				return nil, errors.New("replay: action name must be a string")
			}
			if name == "window" {
				if !r.partial || tick < o.Until {
					return nil, errors.New("cannot reproduce exactly: the recording changed the window size")
				}
				continue
			}
			if !traceformat.SupportsAction(name) {
				return nil, fmt.Errorf(lang.T("replay の未知の直接操作 %q"), name)
			}
			r.acts[tick] = append(r.acts[tick], l)
		case "save_write":
			var write struct {
				File  string    `json:"file"`
				Time  time.Time `json:"time"`
				Known bool      `json:"time_known"`
			}
			if err := json.Unmarshal(sc.Bytes(), &write); err != nil {
				return nil, err
			}
			if r.saveWrites[tick] == nil {
				r.saveWrites[tick] = map[string][]*time.Time{}
			}
			var stamp *time.Time
			if write.Known {
				stamp = &write.Time
			}
			r.saveWrites[tick][write.File] = append(r.saveWrites[tick][write.File], stamp)
		case "phone_game":
			var launch phoneLaunchClock
			if err := json.Unmarshal(sc.Bytes(), &launch); err != nil {
				return nil, err
			}
			if launch.ID == "" || launch.Version == "" || len(launch.Digest) != 64 || launch.Offset < -86400 || launch.Offset > 86400 {
				return nil, errors.New("invalid phone game clock record")
			}
			if r.phoneLaunches == nil {
				r.phoneLaunches = map[int]phoneLaunchClock{}
			}
			if _, exists := r.phoneLaunches[tick]; exists {
				return nil, errors.New("duplicate phone game clock record")
			}
			r.phoneLaunches[tick] = launch
		case "now":
			r.now = time.UnixMilli(int64(traceformat.Number(l["unix_ms"]))).In(time.FixedZone("", int(traceformat.Number(l["utc_offset"]))))
		default:
			if r.checked(k) {
				r.expectLine(k, l)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lines == 0 {
		return nil, fmt.Errorf(lang.T("replay %s に開始行がない"), path)
	}
	if r.end == nil && !r.partial {
		return nil, errors.New(lang.T("replay の終端がない（途中で切れた記録）"))
	}
	r.limit = lastTick
	if r.partial {
		if o.Until-1 > lastTick {
			return nil, errors.New(lang.T("--until が記録の末尾を超えている"))
		}
		r.limit = o.Until - 1
		for k, ls := range r.expect {
			r.expect[k] = slices.DeleteFunc(ls, func(l traceLine) bool {
				return int(traceformat.Number(l["tick"])) > r.limit || (k == "state" && l["terminal"] == true)
			})
		}
	}
	for _, tick := range reloads {
		if tick <= r.limit {
			return nil, errors.New(lang.T("replay は途中の素材・データ再読込に対応していない"))
		}
	}
	if startOptions.Development != nil {
		o.Development = *startOptions.Development
	}
	// Older traces omit these fields; only recorded values replace caller options.
	if _, ok := start["window"]; ok {
		o.Window = startOptions.Window
	}
	if _, ok := start["fullscreen"]; ok {
		o.Fullscreen = startOptions.Fullscreen
	}
	o.Seed = startOptions.Seed
	// A recorded null locale list deliberately clears the caller's locales.
	if _, ok := start["locales"]; ok {
		o.Locales = startOptions.Locales
	}
	if _, ok := start["book_as_played"]; ok {
		o.BookAsPlayed = startOptions.BookAsPlayed
	}

	if value, ok := start["location"].(string); ok {
		o.Location = value
	} else if start["location"] != nil {
		return nil, errors.New("replay: invalid location")
	}
	o.Party, o.Flags, o.Items = traceformat.Strings(start["party"]), traceformat.Strings(start["flags"]), traceformat.Strings(start["items"])
	o.Level, o.Gold, o.Floor = int(traceformat.Number(start["level"])), int(traceformat.Number(start["gold"])), int(traceformat.Number(start["floor"]))
	if value, ok := start["calm"].(bool); ok {
		o.Calm = value
	} else if start["calm"] != nil {
		return nil, errors.New("replay: invalid calm")
	}
	if value, ok := start["battle"].(string); ok {
		o.Battle = value
	} else if start["battle"] != nil {
		return nil, errors.New("replay: invalid battle")
	}
	o.Speed = max(1, startOptions.Speed)
	o.Scene, o.Event, o.Line, o.Picture, o.Answers, o.GalleryAll = startOptions.Scene, startOptions.Event, startOptions.Line, startOptions.Picture, startOptions.Answers, startOptions.GalleryAll
	o.Autopilot, o.Script, o.FromMemo, o.Review = nil, "", "", false
	return r, nil
}

func (r *replay) prepare(g *Game) error {
	if r.capture != nil {
		return r.capture.validate(g)
	}
	if r.version != traceVersion {
		return nil
	}
	fingerprint, err := traceFingerprint(g.opts.FS)
	if err != nil {
		return err
	}
	if g.opts.Seed == 0 {
		return errors.New(lang.T("シード０の記録は乱数の前提を再現できない"))
	}
	if fingerprint == "" || fingerprint != r.fingerprint {
		return errors.New(lang.T("replay のデータの指紋が一致しない"))
	}
	if r.setup.Resources != "" {
		resources, err := g.tracePhoneResources()
		if err != nil {
			return err
		}
		if resources != r.setup.Resources {
			return errors.New("replay: assets or translations changed")
		}
	}
	saves, err := g.traceSaves()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(saves, r.setup.Saves) {
		return errors.New(lang.T("replay の前提となるセーブスロットが一致しない"))
	}
	return nil
}

func (r *replay) feed(in *input, tick int) {
	in.pressed, in.held = [numButtons]bool{}, [numButtons]bool{}
	in.fast, in.click, in.rclick, in.moved, in.wheel = false, false, false, false, 0
	in.pdown, in.pheld, in.pup = false, false, false
	in.longPress = false
	in.esc, in.shift = false, false
	in.digit = 0
	l, ok := r.inputs[tick]
	if !ok {
		return
	}
	for _, n := range traceformat.Strings(l["press"]) {
		if i := slices.Index(buttonNames[:], n); i >= 0 {
			in.pressed[i] = true
		}
	}
	for _, n := range traceformat.Strings(l["held"]) {
		if i := slices.Index(buttonNames[:], n); i >= 0 {
			in.held[i] = true
		}
	}
	if r.picks[tick] != nil {
		in.pressed[btnOK] = false
	}
	in.fast = l["fast"] == true
	in.esc = l["esc"] == true
	in.shift = l["shift"] == true
	in.click = l["click"] == true
	in.rclick = l["rclick"] == true
	in.pdown, in.pheld, in.pup = in.click, l["pointer_held"] == true, l["pointer_up"] == true
	if down, ok := l["pointer_down"].(bool); ok {
		in.pdown = down
	}
	in.longPress = l["long_press"] == true
	in.wheel = traceformat.Number(l["wheel"])
	in.digit = int(traceformat.Number(l["digit"]))
	if m, ok := l["mouse"].([]any); ok && len(m) == 2 {
		in.mouse.X, in.mouse.Y = int(traceformat.Number(m[0])), int(traceformat.Number(m[1]))
		// Ordinary replay retains support for older coordinate-only records.
		in.moved = true
	}
	if moved, ok := l["moved"].(bool); ok {
		in.moved = moved
	}
}

func (r *replay) doActs(g *Game) {
	for _, l := range r.acts[g.tick] {
		name, ok := l["name"].(string)
		if !ok {
			g.failure = errors.New("replay: action name must be a string")
			return
		}
		args, ok := l["args"].(map[string]any)
		if !ok {
			args = nil
		}
		g.act(name, args)
	}
}

func (r *replay) check(g *Game, line traceLine) {
	k, ok := line["t"].(string)
	if !ok {
		g.failure = errors.New("trace: missing record type")
		return
	}
	if r.diverge != "" || !r.checked(k) || (r.partial && k == "state" && line["terminal"] == true) {
		return
	}
	if k == "say" && line["in_battle"] == true && !r.battleSays {
		// The trace was written before battle lines were traced and has none to compare.
		return
	}
	i := r.at[k]
	r.at[k]++
	if i >= len(r.expect[k]) {
		r.diverge = fmt.Sprintf(lang.T("ティック %d で余分な %s 行（%d 行目）"), g.tick, k, i+1)
		return
	}
	if _, ok := r.expect[k][i]["read"]; k == "say" && !ok {
		// A trace from before read marks (ADR 0026) has no read field to compare.
		line = maps.Clone(line)
		delete(line, "read")
	}
	want, got := comparableLine(r.expect[k][i]), comparableLine(line)
	if r.capture != nil {
		want, got = battleComparable(want), battleComparable(got)
	}
	if want != got {
		r.diverge = fmt.Sprintf(lang.T("ティック %d の %s 行（%d 行目）が不一致\n  記録 %s\n  再生 %s"), g.tick, k, i+1, want, got)
		g.emit("diverge", traceLine{"kind": k, "trace": r.expect[k][i], "replay": line})
		return
	}
	r.matched++
}

func (r *replay) finish(g *Game) {
	r.finished = true
	if r.diverge != "" {
		return
	}
	if g.tick != r.limit {
		r.diverge = fmt.Sprintf(lang.T("終端に未到達（%d / %d ティック）"), g.tick, r.limit)
		return
	}
	kinds := []string{"say", "scene", "battle", "state"}
	if r.capture != nil {
		kinds = battleReplayChecked
	}
	for _, k := range kinds {
		if r.at[k] != len(r.expect[k]) {
			r.diverge = fmt.Sprintf(lang.T("%s の期待行が %d 行未消化"), k, len(r.expect[k])-r.at[k])
			return
		}
	}
	if !r.partial && r.capture == nil {
		want, exists := r.end["state"]
		if !exists || comparableLine(traceLine{"state": want}) != comparableLine(traceLine{"state": g.traceSummary()}) {
			r.diverge = lang.T("終端のゲーム状態が一致しない")
		}
	}
}

func (r *replay) report() string {
	if r.diverge != "" {
		return "replay diverged: " + r.diverge
	}
	if !r.finished {
		return "replay diverged: " + lang.T("完了検証が行われていない")
	}
	if r.partial {
		return fmt.Sprintf(lang.T("replay 区間一致: %d 行一致（%d ティックまで）"), r.matched, r.limit)
	}
	if r.capture != nil {
		return fmt.Sprintf(lang.T("replay 戦闘一致: %d 行一致（行動・判断理由・報酬・乱数検証済み）"), r.matched)
	}
	return fmt.Sprintf(lang.T("replay 完全一致: %d 行一致（終端・状態検証済み）"), r.matched)
}

func (g *Game) traceSummary() any {
	if g.st == nil {
		return nil
	}
	b, err := json.Marshal(g.st)
	if err != nil {
		return traceLine{"error": err.Error()}
	}
	var out traceLine
	if err := json.Unmarshal(b, &out); err != nil {
		return traceLine{"error": err.Error()}
	}
	delete(out, "frames")
	return out
}

func comparableLine(l traceLine) string {
	c := traceLine{}
	for k, v := range l {
		if k != "tick" {
			c[k] = v
		}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func newSeed() uint64 { return rand.Uint64()>>12 | 1 } //nolint:gosec // Gameplay seed, not a security token.

func (g *Game) traceEnd() {
	if g.tr == nil || g.tr.ended {
		return
	}
	g.traceDrawGap(g.tick)
	g.tr.ended = true
	g.tracePerformanceEnd()
	g.traceState(true)
	if g.rep != nil {
		g.rep.finish(g)
	}
	l := traceLine{"lines": g.tr.lines + 1, "state": g.traceSummary()}
	if g.rep != nil {
		ReplayReport = g.rep.report()
		l["replay"] = ReplayReport
	}
	g.emit("end", l)
}

// traceAct writes a direct action to the trace, for a replay to do it again (replay.doActs).
func (g *Game) traceAct(name string, args traceLine) {
	l := traceLine{"name": name}
	if len(args) > 0 {
		l["args"] = args
	}
	g.emit("act", l)
}

func (g *Game) act(name string, args traceLine) bool {
	g.traceAct(name, args)
	if g.tr != nil {
		previous := g.tr.inAct
		g.tr.inAct = true
		defer func() { g.tr.inAct = previous }()
	}
	s := func(k string) string {
		v, ok := args[k].(string)
		if !ok {
			return ""
		}
		return v
	}
	n := func(k string) int { return int(traceformat.Number(args[k])) }
	st := g.st
	switch name {
	case "dev_do":
		g.devDo(s("do"))
		return true
	case "dev_jump":
		g.devJump(s("event"), n("line"))
		return true
	case "guides":
		g.guideScripted = args["on"] == true
		return true
	case "window":
		w, h := n("width"), n("height")
		if w <= 0 || h <= 0 {
			g.failure = errors.New("replay: invalid window dimensions")
			return false
		}
		if !g.opts.headless {
			ebiten.SetWindowSize(w, h)
		}
		return true
	case "walk":
		// A script's walk on a map or in a painted scene (sceneWalk); in a dungeon the walk
		// presses keys, which the trace has as input.
		_, _, ok := g.sceneWalk(s("to"), args["face"] == true)
		return ok
	case "battle_auto":
		if b := g.currentBattle(); b != nil && b.phase == bpInput {
			b.auto = true
			for _, m := range b.b.Party {
				m.Auto = true
			}
			b.b.StartTurn(b.actions)
			b.phase = bpPlay
			return true
		}
	case "run":
		loc := g.db.Locations[s("location")]
		if loc != nil && n("action") < len(loc.Actions) {
			st.Location = s("location")
			g.events.Start(loc.Actions[n("action")].Do)
			return true
		}
	case "use_item":
		it := g.db.Items[s("item")]
		if it != nil && g.db.Events[it.Event] != nil {
			if it.Consumed() {
				st.AddItem(s("item"), -1)
			}
			g.events.Start(g.db.Events[it.Event].Do)
			return true
		}
	case "raise":
		if m := st.Actors[s("actor")]; m != nil {
			return m.Raise(s("node"))
		}
	case "equip_best":
		if m := st.Actors[s("actor")]; m != nil {
			st.EquipBest(m)
			return true
		}
	case "buy":
		if sh := s("shop"); sh != "" {
			return st.Buy(sh, s("item"), n("count"))
		}
		st.Gold -= n("price") * n("count")
		st.AddItem(s("item"), n("count"))
		return true
	}
	log.Printf("act %s %v: cannot", name, args)
	return false
}

func (g *Game) traceBattleDecisions(b *battle.Battle) {
	if g.tr != nil && (g.tr.enc != nil || g.rep != nil && g.rep.capture != nil) {
		b.OnDecision = func(d battle.Decision) {
			g.emit("ai_decision", traceLine{"decision": d})
		}
	}
}

// replayStore preserves slot ordering for Continue while isolating replay writes.
type replayStore struct {
	game   *Game
	writes map[int]map[string][]*time.Time
	Store
	times map[string]time.Time
}

func (s *replayStore) ModTime(name string) (time.Time, error) {
	t, ok := s.times[name]
	if !ok {
		return time.Time{}, fs.ErrNotExist
	}
	return t, nil
}

func (s *replayStore) Write(name string, b []byte) error {
	if s.game != nil && s.game.rep != nil && s.game.rep.partial {
		for i := range saveSlots + 1 {
			if name == s.game.slotPath(i) {
				if len(s.writes[s.game.tick][name]) == 0 {
					s.game.failure = fmt.Errorf("cannot reproduce exactly: unrecorded save write at tick %d", s.game.tick)
					return s.game.failure
				}
			}
		}
	}
	if err := s.Store.Write(name, b); err != nil {
		return err
	}
	// A recorded write preserves both the local offset and timestamp availability.
	if s.game != nil {
		if stamps := s.writes[s.game.tick][name]; len(stamps) > 0 {
			stamp := stamps[0]
			s.writes[s.game.tick][name] = stamps[1:]
			if stamp == nil {
				delete(s.times, name)
			} else {
				s.times[name] = *stamp
			}
			return nil
		}
	}
	// Older ordinary replays have no clock record; exact replay refuses them.
	s.times[name] = time.Unix(0, 0).UTC()
	return nil
}

func (r *replay) restoreSaves(g *Game) error {
	if r.setup.SaveData == nil {
		return nil
	}
	s := &replayStore{game: g, writes: r.saveWrites, Store: savefile.MemStore(), times: maps.Clone(r.setup.SaveTimes)}
	if s.times == nil {
		s.times = map[string]time.Time{}
	}
	for name, b := range r.setup.SaveData {
		if fmt.Sprintf("%x", sha256.Sum256(b)) != r.setup.Saves[name] {
			return fmt.Errorf("replay: save digest mismatch: %s", name)
		}
		if err := s.Store.Write(name, b); err != nil {
			return err
		}
	}
	g.store, g.storeGiven = s, true
	return nil
}

func (g *Game) traceShotPath(name string) string {
	path := filepath.Join(g.shotDir(), name+".png")
	abs, err := filepath.Abs(path)
	if err != nil {
		g.tr.err = err
		return path
	}
	return abs
}

// traceInputOutcome runs before finishOffer clears the receiver's acknowledgement.
// It is installed only while tracing, so ordinary updates do no additional work.
func (g *Game) traceInputOutcome(phone bool) {
	if o := g.in.offer; o != nil && o.offered {
		g.emit("input_result", traceLine{"accepted": o.accepted, "command": o.command.verb, "phone": phone, "event": g.evName})
	}
	if g.auto != nil {
		what, _ := g.waiting()
		if what != g.tr.autoWaiting {
			g.tr.autoWaiting = what
			g.emit("autoplay_wait", traceLine{"waiting": what, "event": g.evName})
		}
	}
}

// tracePhoneResources includes the exact bundled packages and trusted catalog in
// replay prerequisites. Optional installed archives are included in traceSaves.
func (g *Game) tracePhoneResources() (string, error) {
	base, err := traceResources(g.opts.FS)
	if err != nil {
		return "", err
	}
	if g.db == nil || g.db.Game.Phone == nil {
		return base, nil
	}
	h := sha256.New()
	_, _ = h.Write([]byte(base))
	paths := []string{g.db.Game.Phone.Catalog}
	for _, app := range g.db.Game.Phone.Apps {
		if app.Install == "bundled" {
			paths = append(paths, app.Path)
		}
	}
	slices.Sort(paths)
	for _, name := range slices.Compact(paths) {
		f, err := g.opts.FS.Open(name)
		if err != nil {
			return "", err
		}
		info, err := f.Stat()
		if err != nil {
			return "", errors.Join(err, f.Close())
		}
		if _, err = fmt.Fprintf(h, "%d:%s:%d:", len(name), name, info.Size()); err != nil {
			return "", errors.Join(err, f.Close())
		}
		_, err = io.Copy(h, f)
		err = errors.Join(err, f.Close())
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// phoneLaunchClock fixes the wall clock separately for each embedded attempt.
// The approved digest also prevents a replay from opening a different release.
type phoneLaunchClock struct {
	ID      string `json:"id"`
	Version string `json:"package_version"`
	Digest  string `json:"sha256"`
	UnixMS  int64  `json:"unix_ms"`
	Offset  int    `json:"utc_offset"`
}

func (g *Game) phoneGameClock(installed *gamepackage.Installed) (time.Time, error) {
	r := installed.Release
	var launch phoneLaunchClock
	if g.rep != nil {
		var ok bool
		launch, ok = g.rep.phoneLaunches[g.tick]
		if !ok || launch.ID != r.ID || launch.Version != r.Version || launch.Digest != r.SHA256 {
			return time.Time{}, errors.New("cannot reproduce phone game: missing clock or changed package")
		}
	} else {
		now := time.Now()
		if g.opts.Now != nil {
			now = g.opts.Now()
		}
		_, offset := now.Zone()
		launch = phoneLaunchClock{r.ID, r.Version, r.SHA256, now.UnixMilli(), offset}
	}
	g.emit("phone_game", traceLine{"id": launch.ID, "package_version": launch.Version, "sha256": launch.Digest, "unix_ms": launch.UnixMS, "utc_offset": launch.Offset})
	return time.UnixMilli(launch.UnixMS).In(time.FixedZone("", launch.Offset)), nil
}
