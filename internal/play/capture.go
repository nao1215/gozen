package play

import (
	"context"
	"errors"
	"fmt"
	"image/gif"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/picturelimit"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// CaptureJob selects a deterministic view for the CLI's image batch.
// Script is preparation input, not a filename. WaitTicks follows that input.
type CaptureJob struct {
	ID                           string
	Start, Scene, Event, Picture string
	Line                         int
	Answers                      []int
	Party, Flags                 []string
	Level, Floor                 int
	Battle                       string
	Calm                         bool
	Seed                         uint64
	Script                       string
	Setup                        string
	WaitTicks                    int
	GIF                          *CaptureGIF
}

// CaptureConfig names a private staging directory owned by the caller.
// Timeout is a positive cooperative per-job limit. CheckAsset checks each asset
// in every sampled frame; nil accepts assets that render without load failures.
type CaptureConfig struct {
	Output     string
	Timeout    time.Duration
	CheckAsset func(string) error
}

// CaptureResult describes one job in input order. Err marks a failed job; only
// successful jobs have an Artifact relative to CaptureConfig.Output.
type CaptureResult struct {
	ID, Artifact          string
	Width, Height, Frames int
	Seed                  uint64
	Pictures              []string
	Err                   error
}

// Capture runs a batch in one graphics loop on the main thread. Every job loads
// fresh definitions and uses private memory saves. Per-job failures are results;
// a returned error means the batch itself could not complete reliably.
func Capture(ctx context.Context, fsys fs.FS, jobs []CaptureJob, cfg CaptureConfig) (results []CaptureResult, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if fsys == nil || cfg.Output == "" || cfg.Timeout <= 0 || len(jobs) == 0 {
		return nil, errors.New("capture requires jobs, staging output and a positive timeout")
	}
	seen := map[string]bool{}
	if err := data.UseWords(fsys); err != nil {
		return nil, err
	}
	db, err := data.Load(fsys)
	if err != nil {
		return nil, err
	}
	if err := picturelimit.Dimensions(db.Game.Screen.Width, db.Game.Screen.Height); err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if j.ID == "" || j.ID == "." || strings.ContainsAny(j.ID, "/\\\x00") || seen[j.ID] {
			return nil, fmt.Errorf("invalid or duplicate capture ID %q", j.ID)
		}
		seen[j.ID] = true
		if j.WaitTicks < 0 {
			return nil, fmt.Errorf("capture %s: wait must be nonnegative", j.ID)
		}
		script, err := parseScript(captureScript(j))
		if err != nil {
			return nil, fmt.Errorf("capture %s: %w", j.ID, err)
		}
		for _, cmd := range script.cmds {
			if cmd.verb == "press" && (cmd.arg == "memo" || cmd.arg == "tag") {
				return nil, fmt.Errorf("capture %s: note-writing buttons are unavailable in capture scripts", j.ID)
			}
			switch cmd.verb {
			case "shot", "lines", "play", "until", "memo", "skip":
				return nil, fmt.Errorf("capture %s: script command %s can write extra images; use capture jobs instead", j.ID, cmd.verb)
			}
			if cmd.verb == "wait" && cmd.n < 0 {
				return nil, fmt.Errorf("capture %s: negative script wait", j.ID)
			}
		}
		if j.GIF != nil {
			if _, err := newCaptureFrames(*j.GIF, db.Game.Screen.Width, db.Game.Screen.Height); err != nil {
				return nil, fmt.Errorf("capture %s: %w", j.ID, err)
			}
		}
		if j.Setup != "" {
			if ds := data.CheckScript(db, j.Setup, true); len(ds) > 0 {
				return nil, fmt.Errorf("capture %s setup: %s", j.ID, ds.Error())
			}
		}
	}
	b := &captureBatch{ctx: ctx, fsys: fsys, jobs: jobs, cfg: cfg}
	defer func() {
		if b.game != nil {
			err = errors.Join(err, b.game.Close())
		}
	}()
	// Initialize before RunGame so Layout has the game's actual dimensions.
	for b.game == nil && b.next < len(jobs) {
		b.begin()
	}
	if b.game == nil {
		return b.results, nil
	}
	ebiten.SetWindowTitle("Gozen capture")
	ebiten.SetWindowSize(b.width, b.height)
	ebiten.SetTPS(60)
	ebiten.SetVsyncEnabled(false)
	ebiten.SetRunnableOnUnfocused(true)
	err = ebiten.RunGameWithOptions(b, &ebiten.RunGameOptions{InitUnfocused: true})
	if errors.Is(err, ebiten.Termination) {
		err = nil
	}
	return b.results, err
}

func capturePreparation(j CaptureJob) string {
	return fmt.Sprintf("%s\nwait %d\n", j.Script, j.WaitTicks)
}

func captureScript(j CaptureJob) string {
	s := capturePreparation(j)
	if j.GIF != nil {
		s += j.GIF.Script + "\n"
	}
	return s + "wait 2147483647\n"
}

type captureBatch struct {
	ctx                     context.Context
	fsys                    fs.FS
	jobs                    []CaptureJob
	cfg                     CaptureConfig
	next, width, height     int
	game                    *Game
	results                 []CaptureResult
	result                  CaptureResult
	frames                  *captureFrames
	pictures                map[string]bool
	prepare                 int
	start, tick             int
	sampling, due, finished bool
	jobErr                  error
}

func (b *captureBatch) begin() {
	j := b.jobs[b.next]
	b.next++
	if j.Seed == 0 {
		j.Seed = 1
	}
	b.result = CaptureResult{ID: j.ID, Seed: j.Seed, Pictures: []string{}}
	b.frames, b.pictures = nil, map[string]bool{}
	b.sampling, b.due, b.finished, b.jobErr = false, false, false, nil
	b.tick, b.start = 0, 0
	prep, err := parseScript(capturePreparation(j))
	if err != nil {
		b.result.Err = err
		b.results = append(b.results, b.result)
		return
	}
	b.prepare = len(prep.cmds)
	g, err := New(Options{
		FS: b.fsys, Seed: j.Seed, NoSound: true, Development: true,
		capture: &captureStartup{setup: j.Setup},
		Script:  captureScript(j), ShotDir: b.cfg.Output, Trace: io.Discard,
		Location: j.Start, Party: j.Party, Level: j.Level, Floor: j.Floor, Flags: j.Flags,
		Battle: j.Battle, Calm: j.Calm, Scene: j.Scene, Event: j.Event, Picture: j.Picture,
		Line: j.Line, Answers: j.Answers,
	})
	if err != nil {
		b.result.Err = err
		b.results = append(b.results, b.result)
		return
	}
	b.game = g
	g.cancel()
	g.context, g.cancel = context.WithTimeout(b.ctx, b.cfg.Timeout)
	g.rendering = true
	w, h := g.Layout(0, 0)
	b.width, b.height = w, h
	if err := picturelimit.Dimensions(w, h); err != nil {
		b.finish(err)
		return
	}
	b.result.Width, b.result.Height = w, h
	if j.GIF != nil {
		b.frames, err = newCaptureFrames(*j.GIF, w, h)
		if err != nil {
			b.finish(err)
			return
		}
		b.result.Width, b.result.Height = b.frames.Config.Width, b.frames.Config.Height
	}
}

func (b *captureBatch) Update() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.game == nil {
		if b.next == len(b.jobs) {
			return ebiten.Termination
		}
		b.begin()
		return nil
	}
	if err := b.game.context.Err(); err != nil {
		b.finish(err)
		return nil
	}
	if b.finished {
		b.finish(b.jobErr)
		return nil
	}
	if b.due {
		return nil
	}
	s := b.game.in.script
	j := b.jobs[b.next-1]
	if (j.Scene != "" || j.Event != "") && b.game.fast == nil && b.game.opts.capture.seek != "line" {
		b.finish(fmt.Errorf("capture did not reach the requested line: %s", b.game.opts.capture.seek))
		return nil
	}
	if !b.sampling && b.game.fast == nil && s.pc >= b.prepare && s.wait == 0 {
		b.sampling, b.due, b.start = true, true, b.tick
		return nil
	}
	if err := b.game.Update(); err != nil {
		if errors.Is(err, errQuit) {
			err = errors.New("capture script ended before its image was captured")
		}
		b.finish(err)
		return nil
	}
	// Walking through an event can request automatic review shots. This batch
	// owns image sampling and publication, so those implicit shots are disabled.
	b.game.in.script.shot = ""
	b.tick++
	if b.sampling && b.frames != nil && b.tick-b.start >= b.frames.due() {
		b.due = true
	}
	return nil
}

func (b *captureBatch) Draw(screen *ebiten.Image) {
	if b.game == nil {
		return
	}
	b.game.Draw(screen)
	if !b.due || b.finished {
		return
	}
	b.due = false
	for _, p := range sortedKeys(b.game.tr.drawn) {
		b.pictures[p] = true
		if b.game.tr.missing[p] {
			b.jobErr = fmt.Errorf("capture: missing image %s", p)
			break
		}
		if b.cfg.CheckAsset != nil {
			if err := b.cfg.CheckAsset(p); err != nil {
				b.jobErr = err
				break
			}
		}
	}
	if b.jobErr != nil {
		b.finished = true
		return
	}
	pixels := b.game.shots.pixels(screen.Bounds())
	screen.ReadPixels(pixels.Pix)
	if b.frames == nil {
		b.game.shots.write(filepath.Join(b.cfg.Output, b.result.ID+".png"), pixels)
		b.result.Frames, b.finished = 1, true
		return
	}
	b.frames.add(pixels)
	b.game.shots.pool.Put(pixels)
	b.result.Frames = len(b.frames.Image)
	b.finished = b.result.Frames == b.frames.count
}

func (b *captureBatch) Layout(int, int) (int, int) { return b.width, b.height }

type captureStartup struct{ setup, seek string }

func (g *Game) captureSetup(defaults []data.Command) error {
	src := g.opts.capture.setup
	if src != "" {
		cmds, ds := data.CompileScript(g.db, src, true)
		if len(ds) > 0 {
			return fmt.Errorf("capture setup: %s", ds.Error())
		}
		defaults = append(defaults, cmds...)
	}
	if len(defaults) == 0 {
		return nil
	}
	r := event.New(g.db, g.st, event.HostFunc(g.Handle))
	r.Dev = true
	var failure error
	r.Log = func(err error) { failure = errors.Join(failure, err) }
	r.Start(defaults)
	for range 10000 {
		request := r.Next()
		if failure != nil {
			return fmt.Errorf("capture setup: %w", failure)
		}
		if request != nil {
			return fmt.Errorf("capture setup cannot wait for player input: %T", request)
		}
		if !r.Running() {
			return nil
		}
	}
	return errors.New("capture setup exceeded its execution limit")
}

func (b *captureBatch) finish(err error) {
	err = errors.Join(err, b.game.Close())
	b.game = nil
	ext := ".png"
	if b.frames != nil {
		ext = ".gif"
		if err == nil {
			f, openErr := os.Create(filepath.Join(b.cfg.Output, b.result.ID+ext))
			if openErr != nil {
				err = openErr
			} else {
				err = errors.Join(gif.EncodeAll(f, &b.frames.GIF), f.Close())
			}
		}
	}
	if err == nil {
		b.result.Artifact = b.result.ID + ext
	} else {
		if removeErr := os.Remove(filepath.Join(b.cfg.Output, b.result.ID+ext)); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}
	b.result.Err, b.result.Pictures = err, sortedKeys(b.pictures)
	b.results = append(b.results, b.result)
	b.frames = nil
}
