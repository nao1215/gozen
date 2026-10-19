package play

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/internal/layer1/memo"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/savefile"
	lang "github.com/nao1215/gozen/text"
)

type Options struct {
	Development      bool
	Report           io.Writer
	CollectBuildInfo bool
	ExternalInput    bool
	FS               fs.FS

	Seed uint64

	SaveDir string

	Script  string
	ShotDir string

	NoSound bool

	Window float64

	Fullscreen bool

	GalleryAll bool

	Location string
	Party    []string
	Level    int
	Flags    []string
	Gold     int

	Items []string

	Floor int

	Calm bool

	Autopilot *Autopilot

	Video io.Writer
	Audio io.Writer

	Trace io.Writer

	TraceShots bool

	Battle string

	Speed int

	Replay string

	ReplayBattle int

	MemoDir string

	FromMemo string

	AllowLegacySave bool

	TracePath string

	Until int

	Scene   string
	Event   string
	Line    int
	Picture string
	Answers []int

	LiveDir string

	Review        bool
	ReviewLog     string
	ReviewSources string
}

var ReplayReport string

func Run(ctx context.Context, opts Options) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	g, err := New(opts)
	if err == nil {
		g.cancel()
		g.context, g.cancel = context.WithCancel(ctx)
		defer func() { err = errors.Join(err, g.Close()) }()
	}
	if err != nil {
		return err
	}
	opts = g.opts
	sw, sh := g.pixels()
	ebiten.SetWindowTitle(g.db.Game.Title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	scale := opts.Window
	if scale == 0 {
		scale = 1

		if monitor := ebiten.Monitor(); monitor != nil {
			mw, mh := monitor.Size()
			if mw > 0 && mh > 0 {
				scale = math.Min(1, math.Min(float64(mw)*0.9/sw, float64(mh)*0.85/sh))
			}
		}
	}
	ebiten.SetWindowSize(max(1, int(sw*scale)), max(1, int(sh*scale)))

	if opts.Fullscreen {
		g.cfg.Fullscreen = true
	}
	ebiten.SetFullscreen(g.cfg.Fullscreen)
	if opts.Script != "" || opts.Autopilot != nil {

		ebiten.SetVsyncEnabled(false)
		ebiten.SetRunnableOnUnfocused(true)
	}
	if opts.Speed > 1 {
		ebiten.SetTPS(60 * opts.Speed)
	}
	if opts.Video != nil || opts.Audio != nil {

		ebiten.SetTPS(ebiten.SyncWithFPS)
	}
	unwatched := opts.Script != "" || opts.Autopilot != nil
	g.rendering = true
	g.startLive()
	err = ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{InitUnfocused: unwatched})
	g.stopLive()
	if errors.Is(err, errQuit) {
		err = nil
	}
	if err == nil && g.tr != nil {
		err = g.tr.err
	}
	return errors.Join(err, g.failure)
}

var activeGame atomic.Bool

var errQuit = ebiten.Termination

type scene interface {
	update(g *Game)
	draw(g *Game, dst *ebiten.Image)
}

type overlay interface{ overlay() bool }

type modal interface{ modal() bool }

type Game struct {
	context       context.Context
	cancel        context.CancelFunc
	failure       error
	closed        bool
	saveDirectory string

	ui       float64
	cfg      config
	windowA  uint8
	auto     *autopilot
	autoShot string
	db       *data.DB
	spUsed   *bool
	opts     Options
	st       *rpg.State
	pics     *pictures
	puppets  puppetSet
	in       input
	th       theme
	tick     int

	stack []scene

	loadNotice *loadNotice

	bannerFloor float64

	events *event.Runner
	req    event.Request
	talk   talk
	waitT  int

	notices noticeBoard
	tapT    int
	tapAuto float64

	cg     data.CG
	cgAge  int
	cgOld  data.CG
	cgOldT int
	cgOut  int

	cgUnder  data.CG
	cgUnderA float64

	dream dreamState

	shake int

	fanfareSong string
	fanfareT    int
	flash       int
	fadeT       int
	enterAt     string

	baseSong, baseMood string
	songOverride       string
	moodOverride       string
	lastMood           string

	quit      bool
	rendering bool

	backlog  []logLine
	logView  *backlogView
	autoRead bool

	tr  *tracer
	rep *replay

	stage      [2]stageActor
	stageOut   stageActor
	stageOutT  int
	stageLeft  []leftActor
	stageLeftT int
	stageK     float32
	stageGen   int
	sides      map[string]float64
	unlocked   map[string]bool
	replaying  bool
	autoBattle bool
	read       map[uint64]bool
	readDirty  bool
	autoKey    string
	offered    string

	unlockReadErr     error
	unlockNeedsBackup bool

	walkGoal [3]int
	lastView *ebiten.Image
	freeze   *ebiten.Image
	freezeT  int

	frame     []byte
	lastFrame int

	memo *memoBox

	tag        *cgReview
	memoToast  string
	memoToastT int

	memoUnfocused bool
	started       time.Time

	pcg       *rand.PCG
	evSnap    []byte
	evRand    []byte
	evName    string
	evLines   int
	evAnswers []int
	fast      *fastTo
	untilShot bool
	tmpSave   string
	quick     []*memoBox
	quickN    int

	live liveState

	graphs graphPanel
}

func New(opts Options) (_ *Game, err error) {
	if opts.FS == nil {
		return nil, errors.New("play: nil filesystem")
	}
	if math.IsNaN(opts.Window) || math.IsInf(opts.Window, 0) || opts.Window < 0 || opts.Window > 4 {
		return nil, errors.New("play: window scale must be between 0 and 4")
	}
	if !activeGame.CompareAndSwap(false, true) {
		return nil, errors.New("play: another game is active; close it first")
	}
	defer func() {
		if err != nil {
			activeGame.Store(false)
		}
	}()

	ReplayReport = ""
	if opts.ReplayBattle < 0 || (opts.ReplayBattle > 0 && (opts.Replay == "" || opts.Until != 0)) {
		return nil, errors.New(lang.T("--replay-battle は --replay と正の戦闘番号を指定し、--until と併用しない"))
	}
	if err := data.UseWords(opts.FS); err != nil {
		return nil, err
	}
	db, err := data.Load(opts.FS)
	if err != nil {
		return nil, err
	}
	if db.Game.Screen.Width <= 0 || db.Game.Screen.Height <= 0 {
		return nil, errors.New("play: screen dimensions must be positive")
	}
	if err := lang.Use(db.Game.Language, db.Game.Text); err != nil {
		return nil, err
	}
	var rep *replay
	if opts.Replay != "" {
		if rep, err = loadReplay(opts.Replay, &opts); err != nil {
			return nil, err
		}
	}
	if opts.Trace != nil && opts.Seed == 0 {
		opts.Seed = newSeed()
	}
	runctx, cancel := context.WithCancel(context.Background())
	g := &Game{context: runctx, cancel: cancel, db: db, opts: opts, pics: newPictures(opts.FS), rep: rep, stageK: 1, started: time.Now()}
	defer func() {
		if err != nil {
			err = errors.Join(err, g.Close())
		}
	}()
	if rep != nil {
		if err := rep.prepare(g); err != nil {
			return nil, err
		}
	}
	if (opts.FromMemo != "" || opts.Script != "" || opts.Autopilot != nil || opts.Replay != "" || opts.Location != "" || opts.Scene != "" || opts.Event != "") && opts.SaveDir == "" {
		d, err := os.MkdirTemp("", "gozen-session-")
		if err != nil {
			return nil, err
		}
		g.opts.SaveDir, g.tmpSave = d, d
	}
	if g.opts.SaveDir == "" {
		g.saveDirectory, err = savefile.Prepare(db.Game.ID, savefile.Config{})
		if err != nil {
			return nil, err
		}
	}

	g.ui = UIScale(&db.Game)
	gfx.SetScale(g.ui)
	g.in.ui = g.ui
	g.loadUnlocks()
	g.setFaceBoxes()
	if opts.Trace != nil || rep != nil {
		g.tr = newTracer(opts.Trace)
		if rep != nil {
			g.in.replay = rep
		}
	}
	g.loadTheme()
	if err := LoadFonts(db, opts.FS); err != nil {
		return nil, err
	}
	f := db.Game.Fonts
	var titleFont []byte
	if f.Title != "" {
		var err error
		if titleFont, err = fs.ReadFile(opts.FS, "assets/"+f.Title); err != nil {
			return nil, fmt.Errorf("title font: %w", err)
		}
	}
	if err := gfx.LoadTitleFont(titleFont); err != nil {
		return nil, fmt.Errorf("title font: %w", err)
	}
	if opts.Autopilot != nil {
		g.auto = newAutopilot(*opts.Autopilot)
	}
	if opts.Script != "" {
		if opts.ShotDir == "" {
			return nil, errors.New("play: scripted runs require a screenshot directory")
		}
		s, err := parseScript(opts.Script)
		if err != nil {
			return nil, err
		}
		s.idle = func() bool { return !g.busy() }
		s.waiting = g.waiting
		s.walk = g.walkStep
		s.arrived = func() bool { a := g.walkArrived(); g.walkGoal = [3]int{}; return a }
		s.battleAuto = func() { g.act("battle_auto", nil) }
		s.inBattle = func() bool { return g.currentBattle() != nil }
		s.typeText = g.typeMemo
		s.memoNote = func(kind, text string) { g.quickMemo(kind, text, memo.LLM) }
		g.in.script = s
	}
	if opts.Audio != nil {
		sound.InitRecord(opts.Audio, 60)
	} else if !opts.NoSound {
		if err := sound.Init(); err != nil {
			return nil, err
		}
	}

	silentBGM := opts.Audio == nil && opts.NoSound && opts.Trace != nil
	if opts.Audio != nil || !opts.NoSound || silentBGM {
		if !silentBGM {
			sound.LoadEffects(opts.FS)
			sound.SetVary(db.Game.UI.SfxVary.Values())
		}
		songs := songsOf(db)
		if len(songs) > 0 {
			first := db.Game.TitleBGM.Song
			if first == "" {
				first = songs[0].Name
			}
			sound.LoadBGM(opts.FS, songs, first, db.Music.OneShot, db.Music.Volume)

			b := db.Game.Battle.Music.Song
			if b == "" {
				b = "battle"
			}
			if _, ok := db.Music.Songs[b]; ok {
				sound.Keep(b)
			}

			for _, l := range db.Locations {
				keep := []data.MusicCue{l.BattleMusic}
				if l.Dungeon != nil {
					for _, lk := range l.Dungeon.Looks {
						keep = append(keep, lk.BattleMusic)
					}
				}
				for _, c := range keep {
					if _, ok := db.Music.Songs[c.Song]; ok && c.Song != b {
						sound.Keep(c.Song)
					}
				}
			}
		}
	}

	g.loadConfig()
	if rep != nil && rep.version == traceVersion {
		g.cfg, g.read, g.unlocked = rep.setup.Config, rep.setup.Read, rep.setup.Unlocked
		g.applyConfig()
	}
	if err := g.traceStart(); err != nil {
		return nil, err
	}
	g.stack = []scene{newTitle(g)}
	if rep != nil && rep.capture != nil {
		if err := g.restoreBattleCapture(); err != nil {
			return nil, err
		}
		return g, nil
	}
	if opts.Location != "" || len(opts.Party) > 0 {
		if err := g.testStart(); err != nil {
			return nil, err
		}
		if t := opts.Battle; t != "" {
			if g.db.Troops[t] == nil {
				return nil, fmt.Errorf("unknown troop %q", t)
			}
			var snap []byte
			if g.db.Game.Battle.Retry {
				snap, err = g.st.Save()
				if err != nil {
					return nil, err
				}
			}
			g.startBattle(t, func(int) {}).retry = snap
		}
	}
	if opts.Review {
		g.opts.GalleryAll = true
		g.push(newCGReview(g))
	}
	if opts.FromMemo != "" {
		if err := g.startFromMemo(opts.FromMemo); err != nil {
			return nil, err
		}
	}
	if opts.Scene != "" || opts.Event != "" {
		if err := g.startAt(); err != nil {
			return nil, err
		}
	}
	if g.failure != nil {
		return nil, g.failure
	}
	return g, nil
}

func (g *Game) screen() (float64, float64) {
	w, h := g.pixels()
	k := g.ui
	if k == 0 {
		k = 1
	}
	return w / k, h / k
}

func (g *Game) pixels() (float64, float64) {
	return float64(g.db.Game.Screen.Width), float64(g.db.Game.Screen.Height)
}

func (g *Game) Layout(int, int) (int, int) {
	return g.db.Game.Screen.Width, g.db.Game.Screen.Height
}

func (g *Game) stateRng() *rand.Rand {
	g.pcg = g.newPCG()
	return rand.New(g.pcg) //nolint:gosec // Gameplay randomness is not used for security.
}

func (g *Game) newPCG() *rand.PCG {
	if g.opts.Seed != 0 {
		return rand.NewPCG(g.opts.Seed, g.opts.Seed*7+1)
	}
	return rand.NewPCG(rand.Uint64(), rand.Uint64()) //nolint:gosec // Gameplay randomness is not used for security.
}

func (g *Game) rng() *rand.Rand {
	if g.opts.Seed != 0 {
		return rand.New(rand.NewPCG(g.opts.Seed, g.opts.Seed*7+1)) //nolint:gosec // Gameplay randomness is not used for security.
	}
	return nil
}

func (g *Game) Update() error {
	if g.closed {
		return errors.New("play: game is closed")
	}
	if g.failure != nil {
		return g.failure
	}
	if g.tr != nil && g.tr.err != nil {
		return g.tr.err
	}
	if g.context != nil {
		if err := g.context.Err(); err != nil {
			return err
		}
	}

	if s := g.in.script; g.rendering && s != nil && s.shot != "" {
		return nil
	}
	if g.rep != nil && !g.rep.partial && g.tick >= g.rep.limit {
		g.quit = true
	}
	if g.quit || (g.in.script != nil && g.in.script.quit) {
		g.flushRead()
		g.traceEnd()
		return errQuit
	}

	if u := g.opts.Until; u > 0 && g.tick+1 >= u {
		switch {
		case !g.untilShot:
			g.untilShot = true
			g.autoShot = fmt.Sprintf("until_%06d", u)
		case g.autoShot == "":
			g.quit = true
		}
		return nil
	}
	g.tick++
	g.in.tick = g.tick
	if g.noticesShown() {
		g.notices.update()
	} else if g.noticesDropped() {
		g.notices.list = nil
	}
	if err := sound.Tick(); err != nil {
		return err
	}
	if g.opts.NoSound && g.tr != nil {
		sound.Pump(ebiten.TPS())
	}
	if g.fast != nil {
		if g.fast.page {
			g.seekReplayPage()
		} else {
			g.fastStep()
		}
	} else {
		g.in.update()
	}
	g.memoToastT = max(0, g.memoToastT-1)
	g.stepMemoFocus()

	if g.opts.MemoDir != "" && g.memo == nil && g.in.pressed[btnMemo] && g.rep == nil && g.auto == nil && g.db != nil && !g.memoKeyTaken() {
		g.openMemo()
	}
	if g.memo != nil {
		g.updateMemo()
		g.tick--
		return nil
	}
	if g.updateTag() {
		g.tick--
		return nil
	}
	if g.updateLoadNotice() {
		g.traceInput()
		g.traceTick()
		return nil
	}
	g.updateLive()
	if g.opts.Development {
		g.updateGraphs()
	}
	if g.auto != nil && len(g.stack) > 0 {
		g.auto.step(g)
	}
	if g.rep != nil {
		g.rep.doActs(g)
	}
	g.traceInput()
	if g.opts.CollectBuildInfo && os.Getenv("GOZEN_DEBUG") != "" && g.tick%30 == 0 && g.in.script != nil {
		log.Printf("tick %d pc %d wait %d stack %d busy %v", g.tick, g.in.script.pc, g.in.script.wait, len(g.stack), g.busy())
	}
	altEnter := inpututil.IsKeyJustPressed(ebiten.KeyEnter) && (ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight))
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) || altEnter {
		g.toggleFullscreen()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		sound.ToggleMute()
	}
	if g.st != nil {
		g.st.Frames++
	}
	g.shake = max(0, g.shake-1)
	g.flash = max(0, g.flash-1)
	g.stepFanfare()
	g.ageCG()
	g.freezeT = max(0, g.freezeT-1)
	if g.fadeT > 0 {
		g.fadeT--
		return nil
	}
	if g.logView != nil {
		g.logView.update(g)
	} else if len(g.stack) > 0 {
		g.stack[len(g.stack)-1].update(g)
	}
	g.traceTick()
	if g.tick%30 == 0 {
		g.checkUnlocks()
	}
	if g.tick%10 == 0 {
		g.idleChores()
	}
	if g.tick%600 == 0 {
		g.flushRead()
	}
	if g.tick%20 == 0 {
		g.updateMusic()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.rendering = true
	if g.opts.CollectBuildInfo && os.Getenv("GOZEN_DEBUG") != "" {
		t0 := time.Now()
		defer func() {
			if g.tick%30 == 0 {
				log.Printf("draw %v fps %.1f tps %.1f", time.Since(t0), ebiten.ActualFPS(), ebiten.ActualTPS())
			}
		}()
	}
	pw, ph := g.pixels()
	dst := screen
	var shaken *ebiten.Image
	if g.shake > 0 {
		shaken = ebiten.NewImage(int(pw), int(ph))
		dst = shaken
	}
	g.traceFrameStart()
	defer g.traceFrameEnd(screen)
	from := 0
	for i := len(g.stack) - 1; i >= 0; i-- {
		if o, ok := g.stack[i].(overlay); !ok || !o.overlay() {
			from = i
			break
		}
	}
	var top []scene
	for _, s := range g.stack[from:] {
		if m, ok := s.(modal); ok && m.modal() {
			top = append(top, s)
			continue
		}
		s.draw(g, dst)
	}
	if g.cgUnder.File != "" {
		g.drawCG(dst, g.cgUnder, float32(g.cgUnderA))
	} else if g.cgOldT > 0 {
		g.drawCG(dst, g.cgOld, float32(g.cgOldT)/float32(g.cgOut))
	}
	if g.cg.File != "" {
		alpha := 1.0
		if ticks := g.cg.FadeTicks(); ticks > 0 {
			alpha = math.Min(1, float64(g.cgAge)/float64(ticks))
		}
		g.drawCG(dst, g.cg, float32(alpha))
	}

	if g.fadeT > 30 && g.freeze != nil {
		dst.DrawImage(g.freeze, nil)
	} else if g.freezeT > 0 && g.freeze != nil {
		op := &ebiten.DrawImageOptions{}
		op.ColorScale.ScaleAlpha(float32(g.freezeT) / moveFade)
		dst.DrawImage(g.freeze, op)
	}
	if g.fadeT == 0 && g.freezeT == 0 {
		if g.lastView == nil {
			b := dst.Bounds()
			g.lastView = ebiten.NewImage(b.Dx(), b.Dy())
		}
		g.lastView.Clear()
		g.lastView.DrawImage(dst, nil)
	}
	g.talk.draw(g, dst)
	if g.noticesShown() {
		g.notices.draw(g, dst)
	}
	if g.logView != nil {
		g.logView.draw(g, dst)
	}
	for _, s := range top {
		s.draw(g, dst)
	}
	if shaken != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(math.Sin(float64(g.shake)*1.7)*float64(g.shake)*0.8*g.ui, math.Cos(float64(g.shake)*2.3)*float64(g.shake)*0.5*g.ui)
		screen.DrawImage(shaken, op)
		shaken.Deallocate()
	}
	if g.flash > 0 {
		fill(screen, withAlpha(colorWhite, uint8(min(255, g.flash*12))))
	}
	if g.fadeT > 0 {
		a := 1 - math.Abs(float64(g.fadeT)-30)/30
		fill(screen, withAlpha(colorBlack, uint8(255*a)))
	}
	if g.opts.Video != nil {
		if err := g.recordFrame(screen); err != nil {
			g.failure = err
			g.quit = true
		}
	}
	if g.autoShot != "" {
		g.saveShot(screen, g.autoShot)
		g.autoShot = ""
	}
	if g.memo != nil {
		g.drawMemo(screen)
	}
	g.drawTag(screen)
	g.drawMemoToast(screen)
	g.drawGraphs(screen)
	g.drawLiveNotice(screen)
	g.drawLoadNotice(screen)
	if len(g.quick) > 0 {
		g.drawQuickMemos(screen)
	}
	if s := g.in.script; s != nil && s.shot != "" {
		g.saveShot(screen, s.shot)
		s.shot = ""
	}
}

func (g *Game) shotDir() string {
	if g.opts.ShotDir == "" {
		return "."
	}
	return g.opts.ShotDir
}

func (g *Game) saveShot(screen *ebiten.Image, name string) {
	g.saveShotQuiet(screen, name)
	g.emit("shot", traceLine{"file": filepath.Join(g.shotDir(), name+".png")})
}

func (g *Game) saveShotQuiet(screen *ebiten.Image, name string) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
		g.failure = errors.New("screenshot: invalid basename")
		return
	}

	dir := g.shotDir()
	b := screen.Bounds()
	img := image.NewNRGBA(b)
	screen.ReadPixels(img.Pix)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		g.failure = errors.Join(g.failure, err)
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png")) //nolint:gosec // The output directory is explicit and the basename is validated above.
	if err != nil {
		g.failure = errors.Join(g.failure, err)
		return
	}
	if err := errors.Join(png.Encode(f, img), f.Close()); err != nil {
		g.failure = errors.Join(g.failure, err)
	}
}

func (g *Game) drawCG(dst *ebiten.Image, c data.CG, a float32) {
	sw, sh := g.screen()
	pic := c.Picture()
	defer g.drawCGOver(dst, c, a)
	if c.Look == "dream" {
		g.drawDream(dst, c, a)
		return
	}
	if c.Fit == "cover" {
		g.drawPicture(dst, pic, 0, 0, sw, sh, a)
		return
	}
	back := pic
	back.Blur = true
	back.Bright = 0.55 * max(0.01, pic.Bright)
	if pic.Bright == 0 {
		back.Bright = 0.55
	}
	if g.db.Game.SchemaVersion == data.CurrentSchemaVersion {
		back.Bright = 0.55 * pic.Brightness()
	}
	g.drawPicture(dst, back, 0, 0, sw, sh, a)
	img := g.pics.get(c.File)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	s := math.Min(sw/iw, sh/ih)
	g.drawPicture(dst, pic, (sw-iw*s)/2, (sh-ih*s)/2, iw*s, ih*s, a)
}

func (g *Game) drawCGOver(dst *ebiten.Image, c data.CG, a float32) {
	if c.Over.File == "" {
		return
	}
	sw, sh := g.screen()
	img := g.pics.get(c.Over.File)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	if iw == 0 || ih == 0 {
		return
	}
	size := c.OverSize
	if size <= 0 {
		size = 0.55
	}
	h := sh * size
	w := iw * h / ih
	if w > sw*0.8 {
		w, h = sw*0.8, ih*sw*0.8/iw
	}
	bottom := sh * 0.74
	g.drawPicture(dst, c.Over, (sw-w)/2, bottom-h, w, h, a)
}

func (g *Game) push(s scene) { g.stack = append(g.stack, s) }

func (g *Game) pop() {
	if len(g.stack) > 1 {
		g.stack = g.stack[:len(g.stack)-1]
	}
}

func (g *Game) replace(s scene) {
	g.stack = []scene{s}
	g.talk = talk{}
	g.cg, g.cgOldT, g.cgUnder = data.CG{}, 0, data.CG{}
}

func (g *Game) sfx(name string) {
	if !g.live.quiet {
		sound.Play(name)
	}
}

func (g *Game) newGame() {
	g.st = rpg.New(g.db, g.stateRng())
	g.bannerFloor = 0
	g.startState()
	if ev := g.db.Game.Start.Event; ev != "" {
		g.events.Start(g.db.Events[ev].Do)
	}
}

func (g *Game) testStart() error {
	o := g.opts
	g.st = rpg.New(g.db, g.stateRng())
	g.bannerFloor = 0
	if o.Location != "" {
		if g.db.Locations[o.Location] == nil {
			return fmt.Errorf("unknown location %q", o.Location)
		}
		g.st.Location = o.Location
	}
	if len(o.Party) > 0 {
		g.st.Party = nil
		for _, id := range o.Party {
			if g.db.Actors[id] == nil {
				return fmt.Errorf("unknown actor %q", id)
			}
			g.st.Join(id)
		}
	}
	for _, a := range g.st.Members() {
		if o.Level > a.Level {
			a.GainExp(a.ExpFor(o.Level) - a.Exp)
			a.SpendSP()
		}
		a.Recover()
	}
	for _, f := range o.Flags {

		if name, v, ok := strings.Cut(f, "="); ok && strings.HasPrefix(name, "var.") {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				g.st.Vars[strings.TrimPrefix(name, "var.")] = n
				continue
			}
		}
		g.st.Flags[f] = true
	}
	if o.Gold > 0 {
		g.st.Gold = o.Gold
	}
	for _, it := range o.Items {
		id, n := it, 1
		if name, v, ok := strings.Cut(it, "="); ok {
			id = name
			if k, err := strconv.Atoi(v); err == nil {
				n = k
			}
		}
		if g.db.Items[id] == nil {
			return fmt.Errorf("unknown item %q", id)
		}
		g.st.AddItem(id, n)
	}
	if o.Floor > 0 && o.Location != "" && g.db.Locations[o.Location].Dungeon != nil && g.db.Locations[o.Location].Dungeon.Generate != nil {
		dungeon.Dive(g.db, g.st, o.Location, o.Floor, true)
	}
	g.startState()
	return nil
}

func (g *Game) restoreState(b []byte) bool {
	st, err := rpg.Parse(g.db, b, g.st.Rand)
	if err != nil {
		log.Printf("retry: %v", err)
		return false
	}
	st.Calm, st.Fighters = g.st.Calm, g.st.Fighters
	*g.st = *st
	g.reportLoad(st.LoadReport)
	return true
}

func (g *Game) loadGame(st *rpg.State) {
	g.st = st
	g.bannerFloor = 0
	g.startState()
	g.reportLoad(st.LoadReport)
}

func (g *Game) startState() {
	g.st.Calm = g.opts.Calm
	g.backlog, g.logView = nil, nil
	g.events = event.New(g.db, g.st, event.HostFunc(g.Handle))
	g.events.BlockAside = g.AsideBlocked
	g.events.OnStart = g.onEventStart
	g.events.Log = func(err error) {
		log.Print(err)
		if g.auto != nil {
			g.auto.errs = append(g.auto.errs, err.Error())
		}
	}
	g.req = nil
	g.replace(g.newPlace())
	g.songOverride, g.moodOverride = "", ""
	g.playLocationMusic()
	var faces []string
	for _, a := range g.st.Members() {
		faces = append(faces, g.faceOf(a.ID, a.Def().Face))
	}
	g.pics.preload(faces...)
}

func (g *Game) faceOf(actor, face string) string {
	a := g.db.Actors[actor]
	if a == nil || a.Faces == "" {
		return ""
	}
	if face == "" {
		face = a.Face
	}

	if face != a.Face && !g.db.Exists("assets/"+a.Faces+"/"+face+".png") {
		face = a.Face
	}
	return a.Faces + "/" + face + ".png"
}

func (g *Game) Move(loc string) {
	g.st.Location = loc
	g.st.Steps++
	g.enterAt = loc
	g.stage = [2]stageActor{}
	g.stageOut = stageActor{}
	if g.fadeT == 0 {
		g.freezeView(moveFade)
	}
	g.sides = nil
	g.stageGen++
	g.songOverride = ""
	g.playLocationMusic()
	inDungeon := g.db.Locations[loc].Dungeon != nil
	if inDungeon {

		dungeon.Reveal(g.db, g.st)
	}
	switch s := g.stack[0].(type) {
	case *field:
		if inDungeon {
			g.stack[0] = newDungeonScene(g)
		} else {
			s.arrive(g)
		}
	case *dungeonScene:
		if inDungeon {
			s.arrive()
		} else {
			g.bannerFloor = 0
			f := &field{dirty: true}
			f.arrive(g)
			g.stack[0] = f
		}
	}
}

func (g *Game) newPlace() scene {
	if dungeon.Of(g.db, g.st) != nil {
		return newDungeonScene(g)
	}
	return newField(g)
}

func (g *Game) Music(c data.MusicCue) {
	g.songOverride = c.Song
	mood := c.Mood
	if mood == "" {
		mood = sound.Neutral
	}
	g.moodOverride = ""
	g.baseSong, g.baseMood = c.Song, mood
	sound.PlaySong(c.Song, mood)
}

func (g *Game) Mood(m string) {
	g.moodOverride = m
	sound.Request(m)
}

func (g *Game) Image(c data.CG) {
	if !c.Faces && g.db.StandIn(c.File) {

		c.Faces = true
	}
	if c.Same(g.cg) {
		if c.Faces {

			g.leaveStage()
		}
		return
	}
	if g.cg.File != "" {
		g.cgOld, g.cgOut = g.cg, g.cg.FadeTicks()
		if c.File == "" && c.HasFade() {
			g.cgOut = c.FadeTicks()
		}

		in := 1.0
		if f := g.cg.FadeTicks(); g.cgAge < f {
			in = float64(g.cgAge) / float64(f)
		}
		g.cgOldT = int(math.Round(float64(g.cgOut) * in))
		switch {
		case c.File == "":
			g.cgUnder = data.CG{}
		case g.cgUnder.File != "":

			u := g.cgUnderA * (1 - in)
			if in >= u {
				g.cgUnder = g.cg
			}
			g.cgUnderA = in + u
		default:
			g.cgUnder, g.cgUnderA = g.cg, in
		}
	}
	if data.PageBase(c.File) != data.PageBase(g.cg.File) {

		g.leaveStage()
	}
	g.cg, g.cgAge = c, 0
	g.dreamShown(c)
	if c.File != "" {
		g.pics.get(c.File)
		g.seenPicture(c.File)
	}
}

func (g *Game) ageCG() {
	if g.cg.File != "" {
		g.cgAge++
	}
	if g.cgUnder.File != "" && (g.cg.File == "" || g.cgAge >= g.cg.FadeTicks()) {
		g.cgUnder = data.CG{}
	}
	g.cgOldT = max(0, g.cgOldT-1)
}

type leftActor struct {
	stageActor
	x   float64
	dim bool
}

func (g *Game) leaveStage() {
	g.stageLeft = g.stageLeft[:0]
	for i, a := range g.stage {
		if a.id == "" || (i == 0 && a.id == g.stage[1].id) {
			continue
		}
		if x, ok := g.sides[a.id]; ok {
			g.stageLeft = append(g.stageLeft, leftActor{stageActor: a, x: x, dim: i == 0})
		}
	}
	g.stageLeftT = g.tick
	g.stage, g.stageOut, g.sides = [2]stageActor{}, stageActor{}, nil
	g.stageGen++
}

func (g *Game) SFX(name string) { g.sfx(name) }

func (g *Game) Effect(name string) {
	switch name {
	case "shake":
		g.shake = 24
	case "flash":
		g.flash = 20
	case "fade":
		g.fadeT = 60
		g.freezeView(0)

		g.stage, g.stageOut, g.sides = [2]stageActor{}, stageActor{}, nil
		g.stageGen++
	}
}

const moveFade = 18

func (g *Game) freezeView(t int) {
	if g.lastView == nil {
		return
	}
	if g.freeze == nil {
		b := g.lastView.Bounds()
		g.freeze = ebiten.NewImage(b.Dx(), b.Dy())
	}
	g.freeze.Clear()
	g.freeze.DrawImage(g.lastView, nil)
	g.freezeT = t
}

func (g *Game) noticesShown() bool {
	if g.talk.active || g.noticesDropped() {
		return false
	}
	for _, s := range g.stack {
		if _, ok := s.(*battleScene); ok {
			return false
		}
	}
	return true
}

func (g *Game) noticesDropped() bool {
	if f := g.db.Game.UI.FieldNotices; f == nil || *f || len(g.stack) == 0 {
		return false
	}
	_, ok := g.stack[0].(*field)
	return ok
}

func (g *Game) busy() bool {
	return g.req != nil || (g.events != nil && g.events.Running()) || g.talk.active || len(g.stack) > 1
}

func (g *Game) waiting() (string, int) {
	if len(g.stack) > 0 {
		if _, ok := g.stack[len(g.stack)-1].(*confirm); ok {
			return "choice", 0
		}
	}
	if g.talk.active {
		switch {
		case g.talk.choice != nil:
			return "choice", 0
		case g.talk.shown >= g.talk.total:
			return "text", g.talk.full
		}
		return "", 0
	}
	if g.tapWaiting() {
		if g.tapT <= 0 {
			return "text", -g.tapT
		}
		return "", 0
	}
	b := g.currentBattle()
	if b == nil || g.stack[len(g.stack)-1] != b {
		return "", 0
	}
	switch {
	case b.phase == bpInput:
		return "command", 0
	case b.phase == bpResult && len(b.results) > 0 && b.endT > 20:
		return "text", b.endT - 20
	}
	return "", 0
}

func (g *Game) pump() bool {
	if g.events == nil {
		return false
	}
	if g.req == nil {
		if !g.events.Running() {
			if g.sides != nil || g.stage != [2]stageActor{} {
				g.stageGen++
			}
			g.stage = [2]stageActor{}
			g.stageOut = stageActor{}
			g.sides = nil
			if g.moodOverride != "" {
				g.moodOverride = ""
			}
			return false
		}
		g.req = g.events.Next()
		if g.req == nil {
			return g.events.Running()
		}
		g.begin(g.req)
		return true
	}
	switch g.req.(type) {
	case event.Say, event.Choice:
		if done, answer := g.talk.update(g); done {
			if answer >= 0 {
				g.emit("answer", traceLine{"index": answer})
				g.evAnswers = append(g.evAnswers, answer)
				g.events.Answer(answer)
			}
			g.req = nil

			if g.events.Running() {
				if g.req = g.events.Next(); g.req != nil {
					g.begin(g.req)
				}
			}
		}
	case event.Wait:
		g.waitT--
		if g.waitT <= 0 {
			g.req = nil
		}
	case event.Tap:
		if g.updateTap() {
			g.req = nil
		}
	}
	return true
}

func (g *Game) tapWaiting() bool {
	if _, ok := g.req.(event.Tap); ok {
		return true
	}
	if b := g.currentBattle(); b != nil && b.phase == bpEvent && g.stack[len(g.stack)-1] == b {
		_, ok := b.req.(event.Tap)
		return ok
	}
	return false
}

func (g *Game) beginTap(r event.Tap) {
	g.tapT = g.cg.FadeTicks()
	g.tapAuto = r.Auto
	g.talk.hidden = false
}

func (g *Game) updateTap() bool {
	if g.novelInput() {
		return false
	}
	g.tapT--
	auto := g.autoRead && float64(-g.tapT) >= 120*g.cfg.autoScale()

	slide := g.tapAuto > 0 && float64(-g.tapT) >= g.tapAuto*60
	return g.tapT <= 0 && (g.in.advance() || g.in.fast || auto || slide)
}

func (g *Game) begin(r event.Request) {
	g.traceRequest(r)
	switch r.(type) {
	case event.Say, event.Choice:
		g.evLines++
	}
	done := func() { g.req = nil }
	switch r := r.(type) {
	case event.Say:
		g.talk.open(g, r)
	case event.Choice:
		g.talk.ask(g, r)
	case event.Wait:
		g.waitT = int(r.Seconds * 60)
	case event.Notice:
		g.notices.add(r.Text)
		done()
	case event.Tap:
		g.beginTap(r)
	case event.Battle:
		if g.replaying {
			g.events.Answer(event.BattleWon)
			done()
			return
		}
		g.st.Fighters = r.Fighters
		var snap []byte
		if g.db.Game.Battle.Retry && !r.NoRetry && !g.isAutopilot() {
			var err error
			snap, err = g.st.Save()
			if err != nil {
				g.failure = err
				return
			}
		}
		s := g.startBattle(r.Troop, func(res int) {
			g.st.Fighters = nil
			g.events.Answer(res)
			done()
		})
		s.retry = snap
	case event.Shop:
		if g.replaying {
			done()
			return
		}
		g.push(newShop(g, r.ID, done))
	case event.Save:
		if g.replaying {
			done()
			return
		}
		g.push(newSaveMenu(g, true, done))
	case event.Open:
		g.openScreen(r.Screen, done)
	case event.Clear:
		g.req = nil
		g.events.Stop()
		g.replace(newEnding(g))
	case event.GameOver:
		g.req = nil
		g.events.Stop()
		g.replace(newGameOver(g))
	}
}

func (g *Game) playLocationMusic() {
	loc := g.db.Locations[g.st.Location]
	if loc == nil {
		return
	}
	cue := loc.Music
	if m := dungeon.Music(g.db, g.st); m.Song != "" {
		cue = m
	}
	if cue.Song == "" {
		return
	}
	mood := cue.Mood
	if mood == "" {
		mood = sound.Neutral
	}
	g.baseSong, g.baseMood = cue.Song, mood
	sound.PlaySong(cue.Song, mood)
}

func (g *Game) hasMood(m string) bool {
	if s, ok := g.db.Music.Songs[g.baseSong]; ok {
		_, has := s.Moods[m]
		return has
	}
	return sound.HasMood(m)
}

func (g *Game) updateMusic() {
	if g.st == nil || g.baseSong == "" {
		return
	}
	song, mood := g.baseSong, g.baseMood
	now := false
	if g.moodOverride != "" {
		mood = g.moodOverride
	} else {
		env := g.st.Env()
		ctx := "field"
		early := false
		if b := g.currentBattle(); b != nil {
			env, ctx = b.b.Env(nil), "battle"
			early = b.phase != bpResult
		}
		for _, r := range g.db.Music.Rules[ctx] {
			if early && slices.Contains(g.db.Music.OneShot, r.Mood) {
				continue
			}
			ok, err := r.When.Bool(env)
			if err != nil || !ok {
				continue
			}
			if r.Play != "" {
				song, mood = r.Play, sound.Neutral
				if r.Mood != "" {
					mood = r.Mood
				}
				break
			}
			if r.Song != "" && r.Song != song {
				continue
			}
			if g.hasMood(r.Mood) {
				mood, now = r.Mood, r.Now
				break
			}
		}
	}
	if w := sound.WantedSong(); w != "" && song != w {
		g.lastMood = mood
		sound.PlaySong(song, mood)
		return
	}
	if mood != g.lastMood {
		g.lastMood = mood
		if now {
			sound.Jump(mood)
		} else {
			sound.Request(mood)
		}
	}
}

func (g *Game) currentBattle() *battleScene {
	for i := len(g.stack) - 1; i >= 0; i-- {
		if b, ok := g.stack[i].(*battleScene); ok {
			return b
		}
	}
	return nil
}

func (g *Game) saveDir() string {
	if g.opts.SaveDir != "" {
		return g.opts.SaveDir
	}
	if g.saveDirectory != "" {
		return g.saveDirectory
	}
	return g.saveDirectory
}

const saveSlots = 3

func (g *Game) slotPath(i int) string {
	if i == autoSlot {
		return g.autoPath()
	}
	return filepath.Join(g.saveDir(), fmt.Sprintf("save%d.json", i+1))
}

func (g *Game) save(i int) error {
	if g.testRun() && g.opts.SaveDir == "" {

		return nil
	}
	return g.writeSave(g.slotPath(i))
}

func (g *Game) load(i int) (*rpg.State, error) {
	b, err := os.ReadFile(g.slotPath(i))
	if err != nil {
		return nil, err
	}
	st, err := rpg.ParseWithConfig(g.db, b, g.rng(), rpg.Config{AllowLegacy: g.opts.AllowLegacySave})
	if err != nil {
		return nil, err
	}
	resumeAt(g.db, st)
	return st, nil
}

func resumeAt(db *data.DB, st *rpg.State) {
	if l := db.Locations[st.Location]; l != nil && l.ResumeTo != "" && db.Locations[l.ResumeTo] != nil {
		st.Location = l.ResumeTo
	}
}

func (g *Game) anySave() bool {
	for i := range saveSlots + 1 {

		if _, err := os.Stat(g.slotPath(i)); err == nil {
			return true
		}
	}
	return false
}

func slotName(i int) string {
	if i == autoSlot {
		return lang.T("オート")
	}
	return fmt.Sprint(i + 1)
}

func (g *Game) slotLabel(i int) (string, bool) {
	st, err := g.load(i)
	if err != nil {
		if _, serr := os.Stat(g.slotPath(i)); serr == nil {
			return slotName(i) + lang.T(": ---- 読めないセーブ ----"), false
		}
		return slotName(i) + lang.T(": ---- からっぽ ----"), false
	}
	loc := g.db.Locations[st.Location].Name
	if l := g.db.Locations[st.Location]; l != nil && l.Dungeon != nil && l.Dungeon.Generate != nil {
		if f, ok := st.Vars[dungeon.VarFloor].(float64); ok {
			loc += fmt.Sprintf(" B%dF", int(f))
		}
	}
	lead := st.Members()[0]
	sec := st.Frames / 60
	when := ""
	if fi, err := os.Stat(g.slotPath(i)); err == nil {
		when = fi.ModTime().Format("01/02 15:04")
	}

	return fmt.Sprintf("%s: %s  %s%d  %s  %d:%02d:%02d  %s", slotName(i), lead.Name(), g.db.Game.Terms.Level, lead.Level, loc, sec/3600, sec/60%60, sec%60, when), true
}

var (
	colorWhite = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	colorBlack = color.NRGBA{0, 0, 0, 0xff}
)

// Handle adapts the lower-layer event contract to the screen.
func (g *Game) Handle(e event.Effect) {
	switch e := e.(type) {
	case event.MoveEffect:
		g.Move(e.Location)
	case event.MusicEffect:
		g.Music(e.Cue)
	case event.MoodEffect:
		g.Mood(e.Name)
	case event.ImageEffect:
		g.Image(e.Picture)
	case event.SoundEffect:
		g.SFX(e.Name)
	case event.VisualEffect:
		g.Effect(e.Name)
	}
}

// Close releases this run's resources. Call on the game thread after its loop stops.
func (g *Game) Close() error {
	if g.closed {
		return nil
	}
	g.closed = true
	if g.cancel != nil {
		g.cancel()
	}
	g.stopLive()
	g.flushRead()
	g.traceEnd()
	err := errors.Join(g.failure, sound.Close())
	if g.tr != nil {
		err = errors.Join(err, g.tr.err)
	}
	if g.tmpSave != "" {
		err = errors.Join(err, os.RemoveAll(g.tmpSave))
		g.tmpSave = ""
	}
	g.closePuppets()
	if g.pics != nil {
		g.pics.close()
	}
	gfx.Reset()
	activeGame.Store(false)
	return err
}
