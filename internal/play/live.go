package play

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"log"
	"math/rand/v2"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/rpg"
	lang "github.com/nao1215/gozen/text"
)

const liveEvery = 2 * time.Second

type liveState struct {
	mu      sync.Mutex
	base    string
	changed atomic.Bool
	stop    chan struct{}
	pending bool
	msg     string
	msgT    int
	quiet   bool
}

func (l *liveState) setBase(fp string) {
	l.mu.Lock()
	l.base = fp
	l.mu.Unlock()
	l.changed.Store(false)
}

func (l *liveState) baseNow() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.base
}

var liveMedia = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".ogg": true, ".wav": true, ".mp3": true, ".flac": true}

func liveFingerprint(dir string) string {
	d, a := fnv.New64a(), fnv.New64a()
	for _, sub := range []string{"game.yaml", "data", "assets"} {
		walkErr := filepath.WalkDir(filepath.Join(dir, sub), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			info, err := e.Info()
			if err != nil {
				return nil
			}
			h := d
			if sub == "assets" && liveMedia[strings.ToLower(filepath.Ext(p))] {
				h = a
			}
			_, _ = fmt.Fprintf(h, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano()) //nolint:errcheck // hash.Hash.Write never returns an error.
			return nil
		})
		if walkErr != nil {
			return "unreadable"
		}
	}
	return fmt.Sprintf("%016x/%016x", d.Sum64(), a.Sum64())
}

func liveDataPart(fp string) string {
	d, _, _ := strings.Cut(fp, "/")
	return d
}

func (g *Game) startLive() {
	dir := g.opts.LiveDir
	if dir == "" || (g.opts.Script != "" && !strings.Contains(g.opts.Script, "press reload")) {
		return
	}
	l := &g.live
	l.setBase(liveFingerprint(dir))
	l.stop = make(chan struct{})
	go func(stop chan struct{}) {
		t := time.NewTicker(liveEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				l.changed.Store(liveFingerprint(dir) != l.baseNow())
			}
		}
	}(l.stop)
}

func (g *Game) stopLive() {
	if g.live.stop != nil {
		close(g.live.stop)
		g.live.stop = nil
	}
}

func (g *Game) liveSay(msg string, t int) {
	g.live.msg, g.live.msgT = msg, t
}

func (g *Game) updateLive() {
	if !g.opts.Development {
		return
	}
	g.live.msgT = max(0, g.live.msgT-1)
	if g.in.pressed[btnReload] {
		g.askReload()
		return
	}
	if g.live.pending && g.reloadWait() == "" && !g.busy() {
		g.live.pending = false
		g.reload()
	}
}

func (g *Game) askReload() {
	if g.opts.LiveDir == "" {
		g.liveSay(lang.T("埋め込みのデータで動いている（--dir で起動すると読み直せる）"), 180)
		return
	}
	now := liveFingerprint(g.opts.LiveDir)
	if liveDataPart(now) == liveDataPart(g.live.baseNow()) {
		g.reloadAssets()
		g.live.setBase(now)
		g.live.pending = false
		g.liveSay(lang.T("絵と音を読み直した"), 150)
		g.emit("live_reload", traceLine{"ok": true, "kind": "assets"})
		return
	}
	if why := g.reloadWait(); why != "" {
		g.reloadAssets()
		g.live.pending = true
		g.liveSay(why, 180)
		g.emit("live_reload", traceLine{"ok": true, "kind": "assets", "wait": why})
		return
	}
	g.reload()
}

type liveKeeper interface {
	liveKeep(g *Game) scene
}

func (g *Game) reloadWait() string {
	switch {
	case g.currentBattle() != nil:
		return lang.T("戦闘の後で読み直す")
	case g.fast != nil:
		return lang.T("読み進めの後で読み直す")
	case g.replaying:
		if r, _ := g.replayScene(); r == nil || r.saved != nil {
			return lang.T("回想の後で読み直す")
		}
		return ""
	case g.st == nil || len(g.stack) == 0:
		return ""
	}
	switch g.stack[0].(type) {
	case *field, *dungeonScene:
	default:
		return lang.T("この画面を出てから読み直す")
	}
	for _, s := range g.stack[1:] {
		if _, ok := s.(liveKeeper); !ok {
			return lang.T("画面を閉じたら読み直す")
		}
	}
	return ""
}

func (g *Game) replayScene() (*sceneReplay, int) {
	for i, s := range g.stack {
		if r, ok := s.(*sceneReplay); ok {
			return r, i
		}
	}
	return nil, -1
}

func (g *Game) reloadAssets() {
	if f, ok := g.opts.FS.(interface{ Forget() }); ok {
		f.Forget()
	}
	g.pics.reset()
	if !g.opts.NoSound || g.opts.Audio != nil {
		sound.LoadEffects(g.opts.FS)
		sound.SetVary(g.db.Game.UI.SfxVary.Values())
		sound.Relearn(g.opts.FS, songsOf(g.db))
	}
	for _, s := range g.stack {
		if gs, ok := s.(*galleryScreen); ok {
			gs.refresh(g)
		}
	}
}

func (g *Game) reload() {
	db, err := data.Load(g.opts.FS)

	if err == nil && (db.Game.ID != g.db.Game.ID || db.Game.DataVersion < g.db.Game.DataVersion) {
		err = fmt.Errorf(lang.T("作品 ID またはデータ版が互換でないため読み直せない（%q/%d → %q/%d）"), g.db.Game.ID, g.db.Game.DataVersion, db.Game.ID, db.Game.DataVersion)
	}
	var reports []rpg.LoadReport
	rp, ri := g.replayScene()
	inEvent := rp == nil && g.st != nil && g.events != nil && (g.events.Running() || g.req != nil) && g.evSnap != nil
	var now, save []byte
	if err == nil && rp == nil && g.st != nil {
		now, err = g.st.Save()
		save = now
		if inEvent {
			save = g.evSnap
		}
	}
	for _, b := range [][]byte{save, now} {
		if err == nil && b != nil {
			var st *rpg.State
			st, err = rpg.Parse(db, b, rand.New(rand.NewPCG(1, 2))) //nolint:gosec // Deterministic validation of a saved gameplay state.
			if err == nil {
				reports = append(reports, st.LoadReport)
			}
		}
	}
	if err != nil {
		log.Printf("live reload: the data has errors, kept the old: %v", err)
		text := liveErrText(err.Error())
		g.liveSay(lang.T("直しに誤りがあるので読み直さない")+"\n"+text, 600)
		g.emit("live_reload", traceLine{"ok": false, "error": err.Error()})
		return
	}
	if g.opts.LiveDir != "" {
		g.live.setBase(liveFingerprint(g.opts.LiveDir))
	}
	g.live.pending = false
	name, lines, answers, rnd := g.evName, g.evLines, g.evAnswers, g.evRand
	g.db = db
	g.unlockNeedsBackup = db.Game.Gallery.MigrateUnlocks(g.unlocked) || g.unlockNeedsBackup
	g.spUsed = nil
	if err := lang.Use(db.Game.Language, db.Game.Text); err != nil {
		log.Printf("live reload: %v", err)
	}
	g.setFaceBoxes()
	g.reloadAssets()
	msg := lang.T("読み直した")
	switch {
	case rp != nil:
		msg = g.reloadReplay(rp, ri)
	case g.st == nil:
		g.keepScenes(g.stack)
	default:
		msg = g.reloadState(now, save, inEvent, name, lines, answers, rnd)
	}
	for _, report := range reports {
		g.reportLoad(report)
	}
	t := 150
	g.liveSay(msg, t)
	g.emit("live_reload", traceLine{"ok": true, "kind": "data", "event": name, "line": lines, "in_event": inEvent, "replay": rp != nil, "at": g.evLines})
}

func (g *Game) keepScenes(ss []scene) {
	for i, s := range ss {
		if k, ok := s.(liveKeeper); ok {
			ss[i] = k.liveKeep(g)
		}
	}
}

func keepers(ss []scene) []scene {
	var out []scene
	for _, s := range ss {
		if _, ok := s.(liveKeeper); ok {
			out = append(out, s)
		}
	}
	return out
}

func (g *Game) reloadState(now, save []byte, inEvent bool, name string, lines int, answers []int, rnd []byte) string {
	uppers := keepers(g.stack[1:])
	banner := g.bannerFloor
	baseRng := g.st.Rand
	start := func(b []byte, fromEvent bool) bool {
		rng := baseRng
		if fromEvent && rnd != nil {
			if p := rand.NewPCG(1, 2); p.UnmarshalBinary(rnd) == nil {
				g.pcg, rng = p, rand.New(p) //nolint:gosec // Restores deterministic gameplay randomness, not secrets.
			}
		}
		st, err := rpg.Parse(g.db, b, rng)
		if err != nil {
			log.Printf("live reload: %v", err)
			return false
		}
		g.loadGame(st)
		g.bannerFloor = banner
		return true
	}
	msg := lang.T("読み直した")
	var cmds []data.Command
	if inEvent && name != "" {
		if cmds = g.commandsNamed(name); cmds == nil {
			log.Printf("live reload: no event %q in the new data: going on from now", name)
			inEvent, msg = false, lang.T("読み直した（イベントが無いので今の所から）")
		}
	}
	defer func() {
		for _, s := range uppers {
			if keeper, ok := s.(liveKeeper); ok {
				g.push(keeper.liveKeep(g))
			}
		}
	}()
	if !inEvent || lines == 0 {
		b := now
		if inEvent {
			b = save
		}
		if !start(b, inEvent) {
			return msg
		}
		g.enterAt = ""
		if inEvent && name != "" {
			g.events.Start(cmds)
		}
		return msg
	}
	begin := func() bool {
		if !start(save, true) {
			return false
		}
		if name != "" {
			g.enterAt = ""
			g.events.Start(cmds)
		}
		return true
	}
	if g.liveReplayTo(begin, lines, answers, nil) {
		msg = lang.T("読み直した（行が減ったので最後の行）")
	}
	return msg
}

func (g *Game) reloadReplay(r *sceneReplay, at int) string {
	lines, answers := g.evLines, g.evAnswers
	autoRead := g.autoRead
	uppers := keepers(g.stack[at+1:])
	g.keepScenes(g.stack[:at])
	msg := lang.T("読み直した")
	defer func() {
		for _, s := range uppers {
			if keeper, ok := s.(liveKeeper); ok {
				g.push(keeper.liveKeep(g))
			}
		}
	}()
	var sc data.GalleryScene
	found := false
	for _, s := range g.db.Game.Gallery.Scenes {
		if s.Key() == r.sc.Key() || slices.Contains(s.LegacyKeys, r.sc.Key()) || (!found && s.Event == r.sc.Event && s.Title == r.sc.Title) {
			sc, found = s, true
		}
	}
	if !found || g.db.Events[sc.Event] == nil {
		g.dropReplay(r, at)
		if c := g.db.Game.TitleBGM; c.Song != "" {
			g.Music(c)
		}
		return lang.T("読み直した（回想が無いのでギャラリーへ）")
	}
	var cur *sceneReplay
	begin := func() bool {
		g.dropReplay(r, at)
		cur = newSceneReplay(g, sc)
		g.autoRead = autoRead
		g.push(cur)
		return true
	}
	alive := func() bool { return len(g.stack) > at && g.stack[at] == cur }
	if lines == 0 {
		begin()
		return msg
	}
	if g.liveReplayTo(begin, lines, answers, alive) {
		msg = lang.T("読み直した（行が減ったので最後の行）")
	}
	if !alive() {
		msg = lang.T("読み直した（回想が終わった）")
	}
	return msg
}

func (g *Game) dropReplay(r *sceneReplay, at int) {
	g.stack = g.stack[:at]
	g.st, g.events, g.req = r.saved, r.events, nil
	g.backlog, g.logView = r.log, nil
	g.replaying = false
	g.autoRead = r.autoRead
	g.talk = talk{}
	g.cg, g.cgOldT, g.cgUnder = data.CG{}, 0, data.CG{}
	g.stage, g.stageOut, g.sides = [2]stageActor{}, stageActor{}, nil
}

func (g *Game) liveReplayTo(begin func() bool, lines int, answers []int, alive func() bool) (short bool) {
	if !begin() {
		return false
	}
	name := g.evName
	seen := g.liveCatchUp(&fastTo{lines: lines, answers: answers}, name, alive)
	if g.evLines >= lines && g.talk.active && (alive == nil || alive()) {
		return false
	}
	if seen <= 0 || seen >= lines || !begin() {
		return false
	}
	g.liveCatchUp(&fastTo{lines: seen, answers: answers}, name, alive)
	return true
}

func (g *Game) liveCatchUp(f *fastTo, name string, alive func() bool) (seen int) {
	g.fast = f
	g.live.quiet = true
	defer func() {
		g.live.quiet = false
		g.fast = nil
		g.shake, g.flash, g.freezeT = 0, 0, 0
		g.in.pressed, g.in.held = [numButtons]bool{}, [numButtons]bool{}

		if g.cg.File != "" {
			g.cgAge = max(g.cgAge, g.cg.FadeTicks())
		}
		g.cgUnder, g.cgOldT = data.CG{}, 0
		g.stageLeftT, g.stageOutT = min(g.stageLeftT, g.tick-12), min(g.stageOutT, g.tick-10)
	}()
	for g.fast != nil {
		if g.evName == name {
			seen = max(seen, g.evLines)
		}
		if alive != nil && !alive() {
			break
		}
		g.tick++
		g.in.tick = g.tick
		g.fastStep()
		if g.fast == nil {
			break
		}
		g.ageCG()
		g.freezeT = max(0, g.freezeT-1)
		if g.fadeT > 0 {
			g.fadeT--
			continue
		}
		if len(g.stack) > 0 {
			g.stack[len(g.stack)-1].update(g)
		}
	}
	if g.evName == name {
		seen = max(seen, g.evLines)
	}
	return seen
}

var liveErrPos = regexp.MustCompile(`^(\S+): \[(\d+):\d+\] (.*)$`)

var liveErrUnknown = regexp.MustCompile(`^unknown field "([^"]+)"`)

func liveErrText(msg string) string {
	first, rest, _ := strings.Cut(msg, "\n")
	clip := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n]) + "..."
		}
		return s
	}
	m := liveErrPos.FindStringSubmatch(first)
	if m == nil {
		return clip(first, 70)
	}
	what := m[3]
	hint := ""
	if u := liveErrUnknown.FindStringSubmatch(what); u != nil {
		what = fmt.Sprintf(lang.T("知らない項目「%s」"), u[1])
		hint = lang.T("本体が古い。作り直すと読める") + lang.T("（動いている本体が知らない書き方）")
	}
	out := []string{fmt.Sprintf(lang.T("%s %s行目"), m[1], m[2]) + "  " + clip(what, 50)}
	for _, l := range strings.Split(rest, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, ">") {
			out = append(out, clip(strings.TrimSpace(strings.TrimPrefix(t, ">")), 60))
			break
		}
	}
	if hint != "" {
		out = append(out, hint)
	}
	return strings.Join(out, "\n")
}

func (g *Game) setFaceBoxes() {
	for _, a := range g.db.Actors {
		if a.Faces != "" && len(a.FaceBox) == 3 {
			g.pics.boxes[a.Faces] = a.FaceBox
		}
		if a.Faces != "" && (a.FaceShade > 0 || a.FaceShadeTop > 0) {
			top := a.FaceShadeTop
			if top == 0 && g.db.Game.SchemaVersion == data.LegacySchemaVersion {
				top = a.FaceShade
			}
			g.pics.shades[a.Faces] = [2]float64{top, a.FaceShade}
		}
	}
}

func songsOf(db *data.DB) []sound.Song {
	var songs []sound.Song
	for name, s := range db.Music.Songs {
		songs = append(songs, sound.Song{Name: name, File: sound.AssetPath(s.File), LoopStart: s.LoopStart, Dir: "assets/" + s.Dir, BPM: s.BPM, Bars: db.Music.Bars, Beats: db.Music.Beats, Moods: s.Moods})
	}
	return songs
}

func (g *Game) liveNotice() string {
	switch {
	case g.live.msgT > 0:
		return g.live.msg
	case g.live.pending:
		return lang.T("終わったら読み直す")
	}

	return ""
}

func (g *Game) drawLiveNotice(screen *ebiten.Image) {
	msg := g.liveNotice()
	if msg == "" {
		return
	}
	lines := strings.Split(msg, "\n")
	w := 0.0
	for _, l := range lines {
		w = max(w, gfx.TextWidth(l, sizeS))
	}
	w += 40
	h := float64(20 + 28*len(lines))
	sw, _ := g.screen()

	x, y := (float64(sw)-w)/2, 24.0
	if g.memoToastT > 0 {
		y += 52
	}

	gfx.FillRoundRect(screen, float32(x), float32(y), float32(w), float32(h), 10, withAlpha(colorBlack, 0xe8))
	for i, l := range lines {
		gfx.DrawText(screen, l, x+w/2, y+8+float64(28*i), sizeS, g.th.text)
	}
}
