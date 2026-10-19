package runner

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
	"github.com/nao1215/gozen/internal/play"
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
// Options follow the GNU style (--name value or --name=value; single-dash long
// options are rejected). Development options require --dev. See
// docs/development/graphics.md.
func Run(ctx context.Context, embedded fs.FS, sub string, cfg Config) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	out := cfg.Output
	if out == nil {
		out = io.Discard
	}
	fl := clihelp.NewFlagSet("game")
	dev := fl.Bool("dev", false, "enable the author tools; scripted runs isolate saves unless --save-dir is given")
	collect := fl.Bool("build-info", false, "with --dev, include the embedded revision metadata in notes")
	external := fl.Bool("external-input", false, "with --dev, permit Linux zenity for text input, falling back to in-window input")
	dir := fl.String("dir", "", "read the game from this `DIR` instead of the embedded one")
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
	review := fl.Bool("review", false, "start in the review of every event picture, tagging them one by one (implies --gallery-all); needs --review-log")
	reviewLog := fl.String("review-log", "", "write the review's tags to this JSON Lines `FILE` (the picture, its source, the tag, and the time)")
	reviewSources := fl.String("review-sources", "", "a JSON `FILE` mapping each picture page to the picture it was made from, so the review keeps its tags by that")
	help, parseErr := clihelp.Parse(fl, cfg.Args)
	if parseErr != nil {
		return parseErr
	}
	if help {
		_, err := io.WriteString(out, page.String(fl))
		return err
	}
	if fl.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fl.Args())
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

var page = clihelp.Page{
	Name:    "game",
	Summary: "a game built with the Gozen runner",
	Usage:   []string{"game [OPTIONS]"},
	About: "Starts the game. Players need no option. The options below choose the window and the saves, " +
		"and, with --dev, start a test play at a place, script the game, play by itself, record, trace, " +
		"and replay. Every option other than --dir, --seed, --mute, --fullscreen, --window, and --save-dir " +
		"needs --dev. The examples use the Wayfinder sample of examples/play.",
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
