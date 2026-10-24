package play

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
)

// EntryConfig supplies the host's output, save store and CLI operand mode.
type EntryConfig struct {
	Store     func(string) (Store, error)
	Mute      bool
	Output    io.Writer
	Directory bool
}

// ArgumentError identifies invalid startup arguments before the game runs.
type ArgumentError struct{ Err error }

func (e *ArgumentError) Error() string { return e.Err.Error() }
func (e *ArgumentError) Unwrap() error { return e.Err }

// Main parses startup options for the private player and test host.
// Directory mode requires one game directory operand and enables live reload there.
func Main(ctx context.Context, fsys fs.FS, dir string, args []string, cfg EntryConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	op := Entry(fsys, dir, cfg)
	help, err := clihelp.Parse(op.Flags, args)
	if err != nil {
		if cfg.Directory {
			return &ArgumentError{Err: err}
		}
		if inner := errors.Unwrap(err); inner != nil {
			return inner
		}
		return err
	}
	if help {
		out := cfg.Output
		if out == nil {
			out = io.Discard
		}
		_, err := io.WriteString(out, op.Page.String(op.Flags))
		return err
	}
	return op.Run(ctx)
}

// Entry registers the shared player's real startup flags without reading game
// data or initializing graphics. Main and the CLI dispatcher parse these flags;
// the CLI help catalog reads the same registration without starting a game.
func Entry(fsys fs.FS, dir string, cfg EntryConfig) clihelp.Operation {
	name := "game"
	if cfg.Directory {
		name = "gozen run"
	}
	fl := clihelp.NewFlagSet(name)
	dev := fl.Bool("dev", false, "enable the author tools; scripted runs isolate saves unless --save-dir is given")
	collect := fl.Bool("build-info", false, "with --dev, include the embedded revision metadata in notes")
	external := fl.Bool("external-input", false, "with --dev, permit Linux zenity for text input, falling back to in-window input")
	dirFlag := new(string)
	if !cfg.Directory {
		dirFlag = fl.String("dir", "", "read the game from this `DIR` instead of the embedded one")
	}
	seed := fl.Uint64("seed", 0, "fix the random numbers with this `SEED`")
	scriptFile := fl.String("script", "", "drive the game with this script `FILE` (see docs/development/graphics.md); needs --shots")
	shots := fl.String("shots", "", "write screenshots to this `DIR`")
	record := fl.String("record", "", "with --script, --autoplay, or --replay, record the play to this MP4 `FILE` (needs ffmpeg)")
	mute := fl.Bool("mute", false, "play no sound")
	fullscreen := fl.Bool("fullscreen", false, "start in full screen (F11 or Alt+Enter switches)")
	galleryAll := fl.Bool("gallery-all", false, "open every picture and scene of the gallery for this run (not saved)")
	windowScale := fl.Float64("window", 0, "the window size as a `SCALE` of the game's size; 0 uses the game's size, or smaller to fit the monitor")
	saveDir := fl.String("save-dir", "", "keep the saves in this `DIR`")
	start := fl.String("start", "", "start a new game at this `LOCATION`")
	fight := fl.String("battle", "", "with --start, begin with a battle against this `TROOP`")
	party := fl.String("party", "", "the party, as actor `IDS` separated by commas")
	level := fl.Int("level", 0, "the party's `LEVEL`")
	flags := fl.String("flags", "", "the `FLAGS` to set, separated by commas; var.name=number sets a variable")
	gold := fl.Int("gold", 0, "the money, in `GOLD`")
	items := fl.String("items", "", "the `ITEMS` to put in the bag, as id or id=count separated by commas")
	floor := fl.Int("floor", 0, "with --start at a generated dungeon, start on this `FLOOR`")
	calm := fl.Bool("calm", false, "meet no random battles")
	auto := fl.Int("autoplay", 0, "play by itself for these `SECONDS`")
	report := fl.String("report", "", "with --autoplay, also write the report to this `FILE`")
	shotEvery := fl.Int("shot-every", 0, "with --autoplay, take a screenshot in --shots every these `SECONDS`")
	autoFloors := fl.Int("auto-floors", 0, "with --autoplay, go back to town after these `FLOORS` of a dive (default: 3)")
	speed := fl.Int("speed", 1, "with --autoplay, run this many `TIMES` faster (for balancing; use --mute)")
	trace := fl.String("trace", "", "write what happens as JSON Lines to this `FILE` (see docs/development/graphics.md)")
	replay := fl.String("replay", "", "play this trace `FILE` again and check that the game goes the same way")
	replayBattle := fl.Int("replay-battle", 0, "with --replay, replay battle `N` (from 1) from just before it begins to its result and compare")
	memos := fl.String("memos", "", "write the test player's notes (F9) to this `DIR` as <dir>/<date>/<number>.png and .json")
	fromMemo := fl.String("from-memo", "", "start from a test player's note `FILE` (.json or .save): its event begun again and read on to its line")
	allowLegacySave := fl.Bool("allow-legacy-save", false, "with --from-memo, migrate an external note without a version, after checking that it is from the same game")
	until := fl.Int("until", 0, "stop before this `TICK`, saving the screen as until_<tick>.png in --shots (with --replay, up to a note's tick)")
	traceShots := fl.Bool("trace-shots", false, "with --trace, save the screen in --shots each time what is shown changes")
	scene := fl.String("scene", "", "start this gallery `SCENE` at once (its title, or its number from 1) and read on to --line")
	event := fl.String("event", "", "start this `EVENT` at once (an event id, or location.action as the notes name it) and read on to --line")
	line := fl.Int("line", 0, "with --scene or --event, stop at this `LINE`, counting lines and questions from its start (default: 1)")
	picture := fl.String("picture", "", "with --scene or --event, stop at the --line-th line shown over this picture `FILE` (a page's file, such as cg/x.002.jpg)")
	answers := fl.String("answers", "", "with --scene or --event, the `ANSWERS` to the questions on the way, separated by commas (0 is the first)")
	review := fl.Bool("review", false, "start a review of the pictures of every event: show them one by one and tag each (implies --gallery-all); needs --review-log")
	reviewLog := fl.String("review-log", "", "with --review, write the tags to this JSON Lines `FILE` (the picture, its source, the tag, and the time)")
	reviewSources := fl.String("review-sources", "", "with --review, a JSON `FILE` mapping each picture to the one it was made from, so a tag follows the source picture")
	return clihelp.Operation{Flags: fl, Page: entryPage(cfg.Directory), Run: func(ctx context.Context) (err error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		argument := func(err error) error {
			if cfg.Directory {
				return &ArgumentError{Err: err}
			}
			return err
		}

		out := cfg.Output
		if out == nil {
			out = io.Discard
		}
		if cfg.Directory {
			if fl.NArg() != 1 {
				return argument(errors.New("run expects one game directory"))
			}
			*dirFlag = fl.Arg(0)
			if *dirFlag == "" {
				return argument(errors.New("game directory must not be empty"))
			}
		} else if fl.NArg() != 0 {
			return argument(fmt.Errorf("unexpected positional arguments: %v", fl.Args()))
		}
		var devFlag string
		fl.Visit(func(f *pflag.Flag) {
			switch f.Name {
			case "dev", "dir", "seed", "mute", "fullscreen", "window", "save-dir":
			default:
				devFlag = f.Name
			}
		})
		if devFlag != "" && !*dev {
			return argument(fmt.Errorf("--%s requires --dev", devFlag))
		}
		if *review && *reviewLog == "" {
			return argument(errors.New("--review requires --review-log"))
		}
		if *scriptFile != "" && *shots == "" {
			return argument(errors.New("--script requires --shots"))
		}

		game, err := gameFS(fsys, dir, *dirFlag)
		if err != nil {
			return err
		}
		opts := Options{
			Development: *dev, CollectBuildInfo: *collect, ExternalInput: *external, Report: out, FS: game, Seed: *seed, NoSound: *mute || cfg.Mute, SaveDir: *saveDir, ShotDir: *shots,
			Location: *start, Battle: *fight, Party: split(*party), Level: *level, Flags: split(*flags), Gold: *gold, Items: split(*items), Calm: *calm, Floor: *floor,
			Replay: *replay, ReplayBattle: *replayBattle, TraceShots: *traceShots, Fullscreen: *fullscreen, Window: *windowScale, GalleryAll: *galleryAll, MemoDir: *memos,
			FromMemo: *fromMemo, AllowLegacySave: *allowLegacySave, TracePath: *trace, Until: *until, Scene: *scene, Event: *event, Line: *line, Picture: *picture,
			LiveDir: "", Review: *review, ReviewLog: *reviewLog, ReviewSources: *reviewSources,
		}
		if *dev {
			opts.LiveDir = *dirFlag
		}
		if cfg.Store != nil {
			opts.StoreFor = func(id string) (Store, error) { return cfg.Store(id) }
		}
		for _, a := range split(*answers) {
			n, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				return argument(fmt.Errorf("--answers %q: %w", *answers, err))
			}
			opts.Answers = append(opts.Answers, n)
		}
		if *auto > 0 {
			opts.Autopilot = &Autopilot{Seconds: *auto, Report: *report, ShotEvery: *shotEvery, Floors: *autoFloors}
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
			err = Record(ctx, opts, *record)
		} else {
			err = Run(ctx, opts)
		}
		if ReplayReport != "" {
			if _, writeErr := fmt.Fprintln(out, ReplayReport); writeErr != nil {
				err = errors.Join(err, writeErr)
			}
			if err == nil && strings.HasPrefix(ReplayReport, "replay diverged") {
				err = errors.New(ReplayReport)
			}
		}
		return err
	}}
}

// gameFS is the --dir directory, or dir within the FS that was passed in.
func gameFS(fsys fs.FS, dir, override string) (fs.FS, error) {
	if override != "" {
		return os.DirFS(override), nil
	}
	if fsys == nil || !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}
	return fs.Sub(fsys, dir)
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

var runnerPage = clihelp.Page{
	Name:    "game",
	Summary: "a game built with Gozen",
	Usage:   []string{"game [OPTIONS]"},
	About: "Starts the game. Players need no option. The options below choose the window and the saves, " +
		"and, with --dev, start a test play at a place, script the game, play by itself, record, trace, " +
		"and replay. Every option other than --dir, --seed, --mute, --fullscreen, --window, and --save-dir " +
		"needs --dev. In the examples, game is the name of the game's program and camp and walker " +
		"are a place and a character of the game.",
	Examples: []clihelp.Example{
		{"play in full screen without sound", "game --fullscreen --mute"},
		{"play by itself for a minute and write a trace", "game --dev --autoplay 60 --trace run.jsonl"},
		{"start a test play in a place with a party", "game --dev --start camp --party walker --level 5 --calm"},
		{"take screenshots with a script", "game --dev --script check.txt --shots shots"},
		{"replay a trace and check that the game goes the same way", "game --dev --replay run.jsonl"},
	},
	Groups: []clihelp.Group{
		{Title: "Game", Flags: []string{"dir", "seed", "mute", "fullscreen", "window", "save-dir", "dev"}},
		{Title: "Test play (--dev)", Flags: []string{"start", "battle", "party", "level", "flags", "gold", "items", "floor", "calm", "scene", "event", "line", "picture", "answers"}},
		{Title: "Autoplay (--dev)", Flags: []string{"autoplay", "report", "shot-every", "auto-floors", "speed"}},
		{Title: "Scripts and records (--dev)", Flags: []string{"script", "shots", "record", "trace", "trace-shots", "replay", "replay-battle", "until"}},
		{Title: "Notes and review (--dev)", Flags: []string{"memos", "from-memo", "allow-legacy-save", "gallery-all", "review", "review-log", "review-sources", "build-info", "external-input"}},
		{Title: "General", Flags: []string{"help"}},
	},
	Notes: []string{
		"Requirements: a graphics display and the platform graphics and audio libraries. Recording needs " +
			"ffmpeg; text input with --external-input optionally uses Linux zenity. Unattended runs on Linux " +
			"need Xvfb. See https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md",
	},
}

func entryPage(directory bool) clihelp.Page {
	p := runnerPage
	if directory {
		p.Name = "gozen run"
		p.Summary = "run a game directory in a window"
		p.Usage = []string{"gozen run <game-dir> [OPTIONS]"}
		p.About = "Runs game.yaml, data/ and assets/ directly, without a Go wrapper. Options may precede or follow the directory. Author tools require --dev. Relative output paths are resolved from the working directory."
		p.Examples = []clihelp.Example{
			{"play a game", "gozen run game"},
			{"play in full screen without sound", "gozen run game --fullscreen --mute"},
			{"develop with live reload", "gozen run game --dev"},
			{"take screenshots with a script", "gozen run game --dev --script check.txt --shots shots"},
		}
	}
	return p
}
