package runner

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/play"
	lang "github.com/nao1215/gozen/text"
)

// Config supplies explicit arguments and the destination for help and reports.
// Nil Args means no flags (os.Args is never read). Nil Output discards help and
// reports. The caller owns Output. Errors are returned, never fatal to the process.
type Config struct {
	Args   []string
	Output io.Writer
}

// Run starts the game in embedded's relative directory sub. --dir overrides it.
// --help succeeds without reading the game. Run must execute on the main thread;
// Ebitengine supports one game loop per process. ctx cancellation is returned.
// Development flags require --dev. See docs/development/graphics.md.
func Run(ctx context.Context, embedded fs.FS, sub string, cfg Config) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	out := cfg.Output
	if out == nil {
		out = io.Discard
	}
	var help bytes.Buffer
	fl := flag.NewFlagSet("game", flag.ContinueOnError)
	fl.SetOutput(&help)
	dev := fl.Bool("dev", false, "enable author tools; scripted runs isolate saves unless --save-dir is explicit")
	collect := fl.Bool("build-info", false, "with --dev: include embedded revision metadata in notes")
	external := fl.Bool("external-input", false, "with --dev: permit Linux zenity, falling back to in-window input")
	fl.Usage = func() {
		help.WriteString("Usage: game [flags]\n") //nolint:errcheck // The in-memory writer cannot fail.
		fl.PrintDefaults()
		help.WriteString("\nRequirements: a graphics display and platform graphics/audio libraries.\nRecording requires ffmpeg; external input optionally uses Linux zenity.\nEbitengine 2.10 supports pure Go builds; race tests require a C compiler. Linux display tests: Xvfb.\nSee https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md\n") //nolint:errcheck // The in-memory writer cannot fail.
	}
	dir := fl.String("dir", "", "read the game from this directory instead of the embedded one")
	seed := fl.Uint64("seed", 0, "fix the random numbers")
	scriptFile := fl.String("script", "", "drive the game with a script (see docs/development/graphics.md)")
	shots := fl.String("shots", "", "where the screenshots go")
	record := fl.String("record", "", "with --script, --autoplay or --replay: record the play to this MP4 (needs ffmpeg)")
	mute := fl.Bool("mute", false, "no sound")
	fullscreen := fl.Bool("fullscreen", false, "start in full screen (F11 or Alt+Enter switches)")
	galleryAll := fl.Bool("gallery-all", false, "debug: open every picture and scene of the gallery (not saved), to look at all of them")
	windowScale := fl.Float64("window", 0, "the window's size, times the game's (0: the game's size, or smaller to fit the monitor)")
	saveDir := fl.String("save-dir", "", "where to keep the saves")
	start := fl.String("start", "", "test play: start a new game at this location")
	fight := fl.String("battle", "", "test play: begin with a battle against this troop (with --start; a boss, a cut-in to look at)")
	party := fl.String("party", "", "test play: the party (actor ids, comma separated)")
	level := fl.Int("level", 0, "test play: the party's level")
	flags := fl.String("flags", "", "test play: flags to set (comma separated; var.name=number sets a variable)")
	gold := fl.Int("gold", 0, "test play: the money")
	items := fl.String("items", "", "test play: items to put in the bag (id or id=count, comma separated)")
	floor := fl.Int("floor", 0, "test play: with --start a made dungeon, start on this floor")
	calm := fl.Bool("calm", false, "test play: no random battles")
	auto := fl.Int("autoplay", 0, "play by itself for this many seconds (the debug play)")
	report := fl.String("report", "", "with --autoplay: write the report here too")
	shotEvery := fl.Int("shot-every", 0, "with --autoplay: a screenshot every this many seconds (in --shots)")
	autoFloors := fl.Int("auto-floors", 0, "with --autoplay: go back to town after this many floors in a dive (default 3)")
	speed := fl.Int("speed", 1, "with --autoplay: run this many times faster (for balancing; mute it)")
	trace := fl.String("trace", "", "write what happens as JSON lines to this file (see docs/development/graphics.md)")
	replay := fl.String("replay", "", "play this trace again and check that the game goes the same way")
	replayBattle := fl.Int("replay-battle", 0, lang.T("--replay の戦闘番号（１から）を開始直前から結果まで再現して照合する"))
	memos := fl.String("memos", "", "where the test player's notes (the F9 key) go: <dir>/<date>/<number>.png and .json")
	fromMemo := fl.String("from-memo", "", "start from a test player's note (its .json or .save): the event it was in begun again and read on to its line")
	allowLegacySave := fl.Bool("allow-legacy-save", false, lang.T("同じ作品と確認した版なしの外部メモを移行する（--from-memo）"))
	until := fl.Int("until", 0, "stop before this tick, saving the screen as until_<tick>.png in --shots (with --replay: to a note's tick)")
	traceShots := fl.Bool("trace-shots", false, "with --trace: save the screen (in --shots) each time what is shown changes")
	scene := fl.String("scene", "", "start a scene of the gallery at once (its title, or its number from 1) and read on to --line")
	event := fl.String("event", "", "test play: start this event at once (an event's id, or location.action as the notes name them) and read on to --line")
	line := fl.Int("line", 0, "with --scene or --event: stop at this line (the lines and questions from its start; default 1)")
	picture := fl.String("picture", "", "with --scene or --event: stop at the --line-th line shown over this picture (a page's file, cg/x.002.jpg)")
	answers := fl.String("answers", "", "with --scene or --event: the answers to the questions on the way (0 is the first; comma separated)")
	review := fl.Bool("review", false, "debug: start in the review of every event picture (one by one, tagging each; implies --gallery-all)")
	reviewLog := fl.String("review-log", "", "where the review's tags go (JSON lines: the picture, its source, the tag, the time)")
	reviewSources := fl.String("review-sources", "", "a JSON of each picture page to the picture it was made from (the review keeps its tags by that)")
	parseErr := fl.Parse(cfg.Args)
	if help.Len() > 0 {
		if _, err := io.Copy(out, &help); err != nil {
			return err
		}
	}
	if errors.Is(parseErr, flag.ErrHelp) {
		return nil
	}
	if parseErr != nil {
		return parseErr
	}
	if fl.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fl.Args())
	}
	var devFlag string
	fl.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "dev", "dir", "seed", "mute", "fullscreen", "window", "save-dir":
		default:
			devFlag = f.Name
		}
	})
	if devFlag != "" && !*dev {
		return fmt.Errorf("--%s requires --dev", devFlag)
	}
	if *review && *reviewLog == "" {
		return errors.New("--review requires --review-log")
	}
	if *scriptFile != "" && *shots == "" {
		return errors.New("--script requires --shots")
	}

	fsys, err := gameFS(embedded, sub, *dir)
	if err != nil {
		return err
	}
	opts := play.Options{Development: *dev, CollectBuildInfo: *collect, ExternalInput: *external, Report: out, FS: fsys, Seed: *seed, NoSound: *mute, SaveDir: *saveDir, ShotDir: *shots,
		Location: *start, Battle: *fight, Party: split(*party), Level: *level, Flags: split(*flags), Gold: *gold, Items: split(*items), Calm: *calm, Floor: *floor,
		Replay: *replay, ReplayBattle: *replayBattle, TraceShots: *traceShots, Fullscreen: *fullscreen, Window: *windowScale, GalleryAll: *galleryAll, MemoDir: *memos,
		FromMemo: *fromMemo, AllowLegacySave: *allowLegacySave, TracePath: *trace, Until: *until, Scene: *scene, Event: *event, Line: *line, Picture: *picture,
		LiveDir: "", Review: *review, ReviewLog: *reviewLog, ReviewSources: *reviewSources}
	if *dev {
		opts.LiveDir = *dir
	}
	for _, a := range split(*answers) {
		n, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil {
			return fmt.Errorf("--answers %q: %w", *answers, err)
		}
		opts.Answers = append(opts.Answers, n)
	}
	if *auto > 0 {
		opts.Autopilot = &play.Autopilot{Seconds: *auto, Report: *report, ShotEvery: *shotEvery, Floors: *autoFloors}
		opts.Speed = *speed
	}
	if *scriptFile != "" {
		b, err := os.ReadFile(*scriptFile)
		if err != nil {
			return err
		}
		opts.Script = string(b)
	}
	if *trace != "" {
		if d := filepath.Dir(*trace); d != "." {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return err
			}
		}
		f, err := os.Create(*trace)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, f.Close()) }()
		opts.Trace = f
	}
	if *record != "" {
		err = play.Record(ctx, opts, *record)
	} else {
		err = play.Run(ctx, opts)
	}
	if play.ReplayReport != "" {
		if _, writeErr := fmt.Fprintln(out, play.ReplayReport); writeErr != nil {
			err = errors.Join(err, writeErr)
		}
		if err == nil && strings.HasPrefix(play.ReplayReport, "replay diverged") {
			err = errors.New(play.ReplayReport)
		}
	}
	return err
}

func gameFS(embedded fs.FS, sub, dir string) (fs.FS, error) {
	var fsys fs.FS
	if dir != "" {
		fsys = os.DirFS(dir)
	} else {
		if embedded == nil || !fs.ValidPath(sub) {
			return nil, &fs.PathError{Op: "sub", Path: sub, Err: fs.ErrInvalid}
		}
		s, err := fs.Sub(embedded, sub)
		if err != nil {
			return nil, err
		}
		fsys = s
	}
	return fsys, nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
