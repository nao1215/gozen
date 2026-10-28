package main

import (
	"context"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
)

// The help pages (ADR 0018, 0024). The examples use the neutral fixture in
// e2e/atago/fixtures (a game directory "game" with the troop "lesson" and the
// dungeon "grotto"). Requirements are written once: the top page holds the
// general ones, a group page what its commands share, and a leaf page only
// what differs from its group.

var general = clihelp.Group{Title: "General", Flags: []string{"help"}}

const noRequirements = "Requirements: none beyond the gozen binary. No cgo, C compiler, ONNX Runtime, model, display, or sound device is needed. See " + clihelp.README

var topPage = clihelp.Page{
	Name:    "gozen",
	Summary: "play, check, simulate, and package Gozen games",
	Usage:   []string{"gozen <command> [<sub-command>] [OPTIONS] [ARGUMENTS]", "gozen help [<command> ...]", "gozen help <code>"},
	About: "gozen reads a game directory (game.yaml, data/, assets/) and checks the definitions, " +
		"runs games in a window, measures play without rendering, inspects real plays, documents events, builds " +
		"release and trial trees, and runs the picture and asset tools. Results go to stdout, " +
		"diagnostics to stderr.",
	Commands: [][2]string{
		{"new", "Create a playable game from a YAML template"},
		{"run", "Run a game directory in a window"},
		{"check", "Check a game directory: definitions, references, expressions, assets, text layout"},
		{"sim", "Measure play without a window: battle, dive, floors, design, campaign"},
		{"trace", "Inspect a gameplay trace"},
		{"notes", "Build an HTML report from test-play notes"},
		{"release", "Build an approved asset tree"},
		{"trial", "Build a trial game tree"},
		{"capture", "Capture PNG images and animated GIFs"},
		{"book", "Write the definitions and events as Markdown"},
		{"export", "Export a playable desktop game or static website"},
		{"prompt", "Assemble image-generation text from a catalog"},
		{"mosaic", "Pixelate and review image regions"},
		{"models", "List or fetch user-selected inference models"},
		{"faces", "Find faces in standing pictures"},
		{"puppet", "Inspect a moving portrait"},
		{"theme", "Dungeon themes: add the pictures of a theme to a game, list the themes"},
		{"version", "Print the gozen version"},
		{"help", "Show the help of a command, or explain a diagnostic code"},
	},
	Examples: []clihelp.Example{
		{"check a game directory", "gozen check game"},
		{"fight 100 battles against the troop lesson", "gozen sim battle game --troop lesson -n 100"},
		{"print the trace of a play", "gozen trace run.jsonl"},
		{"show the options of one command", "gozen help sim dive"},
	},
	Groups: []clihelp.Group{{Title: "General", Flags: []string{"help", "version"}}},
	Notes: []string{
		"Options follow the GNU style: --name=value and --name value both work, a few common options " +
			"have a one-letter form (-n 100), options may come before or after the arguments, and -- ends " +
			"the options. Single-dash long options such as -seed are rejected.",
		"Exit status: 0 for success and help, 1 when a check, I/O, simulation, or capture fails, 2 for " +
			"invalid arguments. A diagnostic may end with a code such as [GZN1101] whose first digit is " +
			"the exit status; gozen help GZN1101 explains it.",
		"Requirements: check, sim, book, release, trial, trace, notes, and prompt need nothing beyond " +
			"the gozen binary; cgo is not required. export needs a matching prebuilt runtime bundle. run and capture launch a graphics game, and unattended " +
			"capture on Linux needs Xvfb. mosaic inference and faces need a gozen built with cgo, " +
			"an ONNX Runtime shared library, and user-supplied models. See " + clihelp.README,
	},
	Hint: `Run "gozen <command> --help" for a command's options and examples.`,
}

// groups are the pages of the commands that hold other commands.
var groups = map[string]clihelp.Page{
	"sim": {
		Summary: "measure play without a window",
		Usage:   []string{"gozen sim <command> [OPTIONS] <game-dir> ..."},
		About: "Plays the game by its rules alone, with no window or sound, and prints numbers: route exploration, the win " +
			"rate against a troop, a dive floor by floor, the generated floors, and a report of personas " +
			"playing the whole game.",
		Commands: [][2]string{
			{"battle", "Fight AUTO battles against a troop and print the win rate"},
			{"dive", "Simulate a dive floor by floor: growth, battles, and bosses"},
			{"floors", "Show generated dungeon floors: maps, steps, and encounters"},
			{"design", "Play the game with personas and write a design report"},
			{"campaign", "Play a campaign many times with a strategy and print what it came to"},
			{"routes", "Explore event choices and report reachable endings and options"},
			{"breed", "Show monsters reachable from captures and their shortest recipes"},
			{"script", "Play a run --script script without a window and write its trace"},
		},
		Examples: []clihelp.Example{
			{"fight 100 battles", "gozen sim battle game --troop lesson -n 100"},
			{"simulate three floors of five battles", "gozen sim dive game --location grotto --floors 3 --battles 5"},
			{"compare three seeds on floor 1", "gozen sim floors game --location grotto --floor 1 --try 3"},
			{"write a design report to a new directory", "gozen sim design game -o report"},
		},
		Groups: []clihelp.Group{general},
		Notes: []string{"--seed, --turns, and --format mean the same in every command: the same --seed " +
			"gives the same result, --turns ends a battle as a draw, and --format json prints one JSON " +
			"document for a tool or an agent.", noRequirements},
		Hint: `Run "gozen sim <command> --help" for a command's options and examples.`,
	},
	"theme": {
		Summary: "dungeon themes",
		Usage:   []string{"gozen theme <command> [OPTIONS] ..."},
		About: "A theme is a set of wall, floor, ceiling and door pictures with colors that go together. " +
			"add copies one into a game, list shows the names.",
		Commands: [][2]string{
			{"add", "Copy a theme's pictures into a game"},
			{"list", "List the themes"},
		},
		Examples: []clihelp.Example{
			{"list the themes", "gozen theme list"},
			{"add the ice theme to a game", "gozen theme add game --name ice"},
		},
		Groups: []clihelp.Group{general},
		Notes:  []string{noRequirements},
		Hint:   `Run "gozen theme <command> --help" for a command's options and examples.`,
	},
	"models": {Summary: "inference model cache", Usage: []string{"gozen models <list|fetch> [OPTIONS]"}, Commands: [][2]string{{"list", "List models and cache state"}, {"fetch", "Download models"}}, Examples: []clihelp.Example{{"list available models", "gozen models list"}}, Groups: []clihelp.Group{general}, Notes: []string{noRequirements}},
}

var pages = map[string]clihelp.Page{
	"new": {
		Summary:  "create a playable game without Go source",
		Usage:    []string{"gozen new <game-dir> [OPTIONS]"},
		About:    "Creates editable YAML and original placeholder assets. The destination must not exist. Each game gets a new save identity unless --id is supplied. No Go compiler is required.",
		Examples: []clihelp.Example{{"create an RPG", "gozen new my-game"}, {"create a novel", "gozen new my-story --template novel --title 'My story'"}, {"create a campaign", "gozen new my-campaign --template campaign"}},
		Groups:   []clihelp.Group{{Title: "Game", Flags: []string{"template", "id", "title"}}, {Title: "Output", Flags: []string{"format"}}, general},
		Notes:    []string{noRequirements},
	},

	"sim breed": {
		Summary:  "explore the monster breeding graph",
		Usage:    []string{"gozen sim breed [OPTIONS] <game-dir>"},
		About:    "Starts with every capturable monster and finds each species reachable by sex-compatible breeding, its fewest generations and one recipe with parent sexes. Also reports unreachable species.",
		Examples: []clihelp.Example{{"explore recipes", "gozen sim breed game"}, {"read the graph as JSON", "gozen sim breed game --format json"}},
		Groups:   []clihelp.Group{{Title: "Output", Flags: []string{"format"}}, general},
		Notes:    []string{noRequirements},
	},
	"theme add": {
		Summary: "copy a dungeon theme into a game",
		Usage:   []string{"gozen theme add <game-dir> --name NAME [OPTIONS]"},
		About: "Copies the wall, floor, ceiling and door pictures and the colors of the theme to " +
			"assets/dungeon/themes/<name>/ of the game, so that theme: <name> in a dungeon or one of its " +
			"looks fills the walls, floor, ceiling, door and colors. A file the game already has is kept; " +
			"--force replaces it. The pictures come with the gozen binary: no network is used.",
		Examples: []clihelp.Example{
			{"add the cave theme", "gozen theme add game --name cave"},
			{"restore the pictures of the cave theme", "gozen theme add game --name cave --force"},
		},
		Groups: []clihelp.Group{{Title: "Files", Flags: []string{"name", "force"}}, general},
		Notes:  []string{"The theme's pictures are CC BY 4.0; see ASSETS.md of the gozen repository.", noRequirements},
	},
	"theme list": {
		Summary: "list the dungeon themes",
		Usage:   []string{"gozen theme list"},
		About:   "Prints the name of each theme the gozen binary carries, with a line on how it looks.",
		Examples: []clihelp.Example{
			{"list the themes", "gozen theme list"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"format"}}, general},
		Notes:  []string{noRequirements},
	},
	"check": {
		Summary: "check the definitions, assets, and text layout of a game",
		Usage:   []string{"gozen check [OPTIONS] <game-dir>"},
		About: "Loads every definition of the game, resolves references and expressions, checks that " +
			"the assets exist, and lays out dialogue and choices with the game's font and display rules. " +
			"Warnings go to stderr and do not fail the check. --only structure does not read the font or " +
			"check the text layout, and stdout says so; --only layout checks the text layout alone. " +
			"A game without fonts.regular in game.yaml is laid out with the built-in Go Regular font, " +
			"which has Latin letters only; a warning lists the characters of the text it cannot draw.",
		Examples: []clihelp.Example{
			{"check a game", "gozen check game"},
			{"check the structure only, without fonts", "gozen check --only structure game"},
			{"check that dialogue and choices fit the message window", "gozen check --only layout game"},
			{"print the problems as JSON for a tool or an agent", "gozen check --format json game"},
		},
		Groups: []clihelp.Group{{Title: "Check", Flags: []string{"only", "format"}}, general},
	},
	"sim battle": {
		Summary: "fight AUTO battles against a troop",
		Usage:   []string{"gozen sim battle [OPTIONS] <game-dir> --troop ID"},
		About: "Fights the troop with the starting party (or --party, or a saved state) under AUTO and " +
			"prints the win, loss, flight, and draw rates and the average turns. The numbers are the " +
			"same as the development balance panel (F7) of the game. The same --seed gives the same result.",
		Examples: []clihelp.Example{
			{"fight 100 battles", "gozen sim battle game --troop lesson -n 100"},
			{"choose the party, its level, and another seed", "gozen sim battle game --troop lesson -n 100 --party scout --level 5 --seed 7"},
			{"start from a saved test-play note and show the HP left after a win", "gozen sim battle game --troop lesson --state memo.save --hp"},
			{"keep the saved HP and count the wins in a row, up to 10", "gozen sim battle game --troop lesson --state memo.save --as-is --streak 10"},
			{"watch the messages of the first battle", "gozen sim battle game --troop lesson -n 1 -v"},
		},
		Groups: []clihelp.Group{
			{Title: "Party", Flags: []string{"party", "level", "gear", "no-tree", "state", "as-is"}},
			{Title: "Battle", Flags: []string{"troop", "battles", "turns", "seed", "streak"}},
			{Title: "Output", Flags: []string{"hp", "verbose", "format"}},
			general,
		},
	},
	"sim dive": {
		Summary: "simulate a dive floor by floor",
		Usage:   []string{"gozen sim dive [OPTIONS] <game-dir> --location ID"},
		About: "Takes the starting party down the dungeon location, fighting --battles battles on each " +
			"floor, buying better equipment from --shops, resting, and fighting the bosses. Prints the " +
			"level, HP, gold, and win rates of each floor.",
		Examples: []clihelp.Example{
			{"simulate three floors of five battles", "gozen sim dive game --location grotto --floors 3 --battles 5"},
			{"add a boss on floor 3 and grind until it is won 80% of the time", "gozen sim dive game --location grotto --floors 3 --boss 3:lesson --grind 0.8"},
			{"rest only every two floors, and fight 50 battles for each boss win rate", "gozen sim dive game --location grotto --floors 4 --rest 2 --boss 2:lesson --tries 50"},
		},
		Groups: []clihelp.Group{
			{Title: "Dive", Flags: []string{"location", "floors", "battles", "boss", "guards", "retreat", "guards-only", "tries", "grind", "turns", "seed"}},
			{Title: "Town", Flags: []string{"shops", "rest", "supplies", "rest-cost", "rest-event", "back-item", "worn-hp", "worn-mp", "no-town"}},
			{Title: "Events", Flags: []string{"events", "scene-event", "scene-when", "floor-event"}},
			{Title: "Output", Flags: []string{"format", "arrival-saves"}},
			general,
		},
	},
	"sim campaign": {
		Summary: "play a campaign many times and measure it",
		Usage:   []string{"gozen sim campaign [OPTIONS] <game-dir>"},
		About: "Plays the game's campaign (game.yaml campaign, ADR 0030) --campaigns times, headless, " +
			"the player giving orders by --as: greedy marches when the clash's odds are good (its " +
			"formulas with rand() at its middle), else gives another order; random gives any order " +
			"it can; ai plays the player's faction by its own ai rules. Prints the win rate, the turns " +
			"the campaigns ended in, the regions each faction held turn by turn, the factions that " +
			"fell and when, and the orders given. Campaign i uses the seed --seed + i. With " +
			"--compare strategies|tactics, it plays every strategy or declared tactic on the same " +
			"seeds and reports summary and per-region balance numbers; --format json writes the same report as JSON.",
		Examples: []clihelp.Example{
			{"measure 200 campaigns", "gozen sim campaign game"},
			{"compare random play, as JSON", "gozen sim campaign game --as random -n 500 --format json"},
			{"march every general with one tactic", "gozen sim campaign game --tactic guard"},
			{"compare player strategies", "gozen sim campaign game --compare strategies"},
			{"compare declared tactics as JSON", "gozen sim campaign game --compare tactics --format json"},
		},
		Groups: []clihelp.Group{
			{Title: "Campaign", Flags: []string{"campaigns", "seed", "as", "tactic", "orders", "compare"}},
			{Title: "Output", Flags: []string{"format"}},
			general,
		},
	},
	"sim script": {
		Summary: "play a script without a window and write its trace",
		Usage:   []string{"gozen sim script [OPTIONS] <game-dir> --script FILE"},
		About: "Plays the game with the script of run --dev --script, tick by tick as fast as it goes, " +
			"without a window, sound, or screenshots, and writes the same JSON Lines trace (say, request, " +
			"order, dev_do, autosave, state, end). The same options and seed give the same game as run. " +
			"shot takes no picture, and the trace lines that come from drawing (shot, view, stand, menu) " +
			"are not written. With --script -, the script is read line by line from the standard input " +
			"and the trace goes to the standard output: before each line it writes a ready line and " +
			"flushes, so a program can read the trace and send the next command; snapshot writes the " +
			"whole state, and the end of the input ends the run.",
		Examples: []clihelp.Example{
			{"play a script and write its trace", "gozen sim script game --start camp --script check.txt --trace run.jsonl"},
			{"read the script from the standard input, the trace on the standard output", "gozen sim script game --start camp --script - < check.txt"},
		},
		Groups: []clihelp.Group{
			{Title: "Script", Flags: []string{"script", "trace", "seed", "save-dir"}},
			{Title: "Start", Flags: []string{"start", "battle", "party", "level", "flags", "gold", "items", "floor", "calm", "event", "line", "answers"}},
			general,
		},
		Notes: []string{noRequirements},
	},
	"sim routes": {
		Summary: "find unreachable endings and choice options",
		Usage:   []string{"gozen sim routes [OPTIONS] <game-dir>"},
		About: "Explores event choices without a screen, counting paths to clear and game-over endings, " +
			"and listing authored choice options that were never offered. Limits bound paths, event requests " +
			"per path, and elapsed time. Unreachable content fails only with --fail-unreachable. Runtime errors and timeouts always fail.",
		Examples: []clihelp.Example{
			{"explore a visual novel", "gozen sim routes examples/novel/game"},
			{"write JSON and fail CI on unreachable content", "gozen sim routes game --format json --fail-unreachable"},
		},
		Groups: []clihelp.Group{{Title: "Exploration", Flags: []string{"paths", "depth", "timeout"}}, {Title: "Output", Flags: []string{"format", "fail-unreachable"}}, general},
		Notes:  []string{noRequirements},
	},
	"sim floors": {
		Summary: "show generated floors: maps, steps, and encounters",
		Usage:   []string{"gozen sim floors [OPTIONS] <game-dir> --location ID"},
		About: "Generates the floors of the dungeon location and prints each map with the steps to the " +
			"stairs and the expected encounters. With --try, compares seeds on one floor, one line each.",
		Examples: []clihelp.Example{
			{"show floors 1 to 3", "gozen sim floors game --location grotto --from 1 --to 3"},
			{"compare three seeds on floor 1", "gozen sim floors game --location grotto --floor 1 --try 3"},
			{"print the numbers only", "gozen sim floors game --location grotto --no-map"},
		},
		Groups: []clihelp.Group{{Title: "Floors", Flags: []string{"location", "from", "to", "floor", "try"}}, {Title: "Output", Flags: []string{"no-map", "format"}}, general},
	},
	"sim design": {
		Summary: "play the game with personas and write a report",
		Usage:   []string{"gozen sim design [OPTIONS] <game-dir>"},
		About: "Plays the game headless with the personas of designeval.yaml (players, rewards, and time " +
			"weights). With --out, writes summary.json, timelines, band.png, and report.md to a new directory. " +
			"It does not run Git or collect anything from the environment. A simulation error keeps the " +
			"report and exits 1.",
		Examples: []clihelp.Example{
			{"write a report to a new directory", "gozen sim design game -o report"},
			{"run one persona with its log", "gozen sim design game -o report --only walker -v"},
			{"use another configuration and keep notes", "gozen sim design game --config game/designeval.yaml --out report --memos notes"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "only", "seed", "list"}},
			{Title: "Output", Flags: []string{"out", "root", "memos", "verbose", "format"}},
			general,
		},
		Notes: []string{"The output directory must not exist yet."},
	},
	"trace": {
		Summary: "print or check a JSON Lines trace of a play",
		Usage:   []string{"gozen trace [OPTIONS] <trace.jsonl>"},
		About: "Prints a trace written by the game's --trace in time order. With --check, lists the " +
			"faults of the standing pictures and the music instead and exits 1 if there are any. Use - for standard input. Check mode does not accept display filters.",
		Examples: []clihelp.Example{
			{"print a trace", "gozen trace run.jsonl"},
			{"print only dialog and battles between seconds 10 and 60", "gozen trace run.jsonl --only say --only battle --from 10 --to 60"},
			{"check pictures and music", "gozen trace run.jsonl --check"},
		},
		Groups: []clihelp.Group{
			{Title: "Filter", Flags: []string{"only", "from", "to", "input"}},
			{Title: "Check", Flags: []string{"check", "music-late"}},
			{Title: "Output", Flags: []string{"format"}},
			general,
		},
	},
	"notes": {
		Summary: "turn test-play notes into HTML with thumbnails",
		Usage:   []string{"gozen notes <memo-dir> --out DIR [OPTIONS]"},
		About:   "Reads the notes the game writes with F9 and builds an HTML page with thumbnails in a new directory. The notes are not changed.",
		Examples: []clihelp.Example{
			{"write build/memos-html", "gozen notes build/memos --out build/memos-html"},
			{"choose the output directory", "gozen notes build/memos -o notes-html"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"out"}}, general},
		Notes:  []string{"The output directory must not exist yet.", noRequirements},
	},
	"capture": {
		Summary: "capture PNG images and animated GIFs from a game",
		Usage:   []string{"gozen capture GAME --config FILE --out DIR [OPTIONS]"},
		About:   "Runs the YAML capture jobs in one graphics loop with isolated state and saves. Writes PNG or GIF images, shots.json and index.html. Failed jobs remain in the report and cause exit 1. Existing output directories are preserved.",
		Examples: []clihelp.Example{
			{"capture a list of views", "gozen capture game --config captures.yaml --out promo"},
			{"capture one job and report JSON", "gozen capture game --config captures.yaml --out promo --only opening --format json"},
			{"run unattended on Linux", "xvfb-run -a gozen capture game --config captures.yaml --out promo"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "only", "overlay", "raw", "timeout"}},
			{Title: "Output", Flags: []string{"out", "format"}},
			general,
		},
		Notes: []string{
			"Requirements: a display and platform graphics libraries; no game binary, Go toolchain or ffmpeg. See " + clihelp.README,
			"GIF jobs support 1–30 FPS, up to 10 seconds and 1280 pixels wide. Combined limits: 300 frames, 921600 pixels per frame and 64 MiB of paletted frames. Timeouts are cooperative.",
		},
	},
	"book": {
		Summary: "write the game synopsis as Markdown or structured data",
		Usage:   []string{"gozen book [OPTIONS] <game-dir>"},
		About:   "Writes the locations, events, scenes, troops, and items of the game to stdout as one Markdown document. Each monster page includes its lineage (how it is captured or bred). JSON contains the same complete synopsis; --section lineage selects only lineage in every format. JSONL emits one item per section and a final summary. --section voices exports a versioned recording inventory, including filled scripted lines and unresolved runtime fields; JSON and JSONL return one result.",
		Examples: []clihelp.Example{
			{"write the book to a file", "gozen book game > book.md"},
			{"read it in a pager", "gozen book game | less"},
			{"read each monster's family tree as JSON", "gozen book game --section lineage --format json"},
			{"list stream recordings", "gozen book game --section voices --format json"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"format", "section"}}, general},
	},
	"export": {
		Summary: "export a playable game with a prebuilt runtime",
		Usage:   []string{"gozen export <game-dir> --runtime DIR --out DIR [OPTIONS]"},
		About:   "Copies runtime game files, a matching player and license notices. --target web creates a static website; desktop targets create a directory with a native player. No compiler or network access is used.",
		Examples: []clihelp.Example{
			{"package for Windows", "gozen export game --out output --runtime runtime --target windows-amd64"},
			{"export a website", "gozen export game --out site --runtime web-runtime --target web"},
		},
		Groups: []clihelp.Group{{Title: "Runtime", Flags: []string{"runtime", "target"}}, {Title: "Output", Flags: []string{"out", "format"}}, general},
		Notes:  []string{"Requirements: an unpacked matching Gozen runtime bundle. Desktop play needs a display and platform graphics and audio libraries; web play needs an HTTP or HTTPS server. See " + clihelp.README, "The output must be new and outside the game and runtime directories. For games with an asset policy, prepare the input with gozen release first."},
	},
	"release": {
		Summary: "build a release tree with approved assets only",
		Usage:   []string{"gozen release <game-dir> --inspect FILE --out DIR [OPTIONS]"},
		About: "Checks the approval records and the assets the game refers to, then copies the game to a " +
			"new directory, taking replaced assets from the overlay. Any unapproved reference fails the build.",
		Examples: []clihelp.Example{
			{"build a release tree", "gozen release game --out release --inspect inspect.jsonl"},
			{"take replaced assets from an overlay directory", "gozen release game --out release --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{{Title: "Assets", Flags: []string{"out", "inspect", "overlay", "prefix"}}, general},
	},
	"trial": {
		Summary: "build a trial tree with the reachable part",
		Usage:   []string{"gozen trial <game-dir> --out DIR [OPTIONS]", "gozen trial --dry-run [OPTIONS] <game-dir>"},
		About: "Keeps only the definitions and assets reachable within the range of trial.yaml and writes " +
			"them to a new directory. --overlay and --inspect go together.",
		Examples: []clihelp.Example{
			{"count what would be kept, writing nothing", "gozen trial game -n"},
			{"build a trial tree and list what was kept and dropped", "gozen trial game --out trial --list trial.txt"},
			{"take replaced assets from an overlay", "gozen trial game --out trial --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "overlay", "inspect"}},
			{Title: "Output", Flags: []string{"out", "list", "dry-run"}},
			general,
		},
		Notes: []string{"The --list file must not exist yet. --dry-run writes nothing, even with --list. --config is relative to the working directory; when omitted, GAME/trial.yaml is used."},
	},
	"puppet": {
		Summary: "check a moving portrait and cut its pictures",
		Usage:   []string{"gozen puppet [OPTIONS] <puppet.yaml>"},
		About: "Loads a puppet (a moving portrait for the puppet package) with the same checks as puppet.Load " +
			"and prints where each face, closed-eyes, and mouth picture lies on the base. A difference " +
			"picture as large as the base is cut down to the part that differs. With --out, writes the " +
			"puppet YAML and the cut pictures (each with at) to a new directory, so the game ships small " +
			"pictures. The written YAML has no comments.",
		Examples: []clihelp.Example{
			{"check a puppet and show the positions", "gozen puppet streamer/puppet.yaml"},
			{"write it with small difference pictures", "gozen puppet streamer/puppet.yaml --out streamer-packed"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"out"}}, general},
		Notes:  []string{"The output directory must not exist yet.", noRequirements},
	},
	"faces": {
		Summary: "find the faces for the balloons and icons",
		Usage:   []string{"gozen faces [OPTIONS] <game-dir>"},
		About: "Finds the face in each standing picture a painted scene (stage:) cuts a balloon face from: " +
			"the pictures of the people placed in a stage and the actors' face pictures, and, in a game with a " +
			"character book (profiles), the actors' and the profiles' pictures its face icons are cut from. Uses the anime face " +
			"detector that gozen models fetch face downloads, and writes the boxes, the pictures' sizes and " +
			"SHA-256 to assets/faces.json (replacing it). The game reads that file and runs no model, so a web " +
			"build uses it too. A doll's icon (a face drawn for the balloons) is not searched; it and face_at come first; without a box the face is guessed. gozen check " +
			"warns of a picture with no box or changed since.",
		Examples: []clihelp.Example{
			{"download the model once", "gozen models fetch face"},
			{"find the faces", "ONNXRUNTIME_LIB=/path/libonnxruntime.so gozen faces game"},
			{"and look at what was found", "gozen faces game --preview faces-check"},
		},
		Groups: []clihelp.Group{{Title: "Detection", Flags: []string{"model", "ort", "min"}}, {Title: "Output", Flags: []string{"preview"}}, general},
		Notes: []string{"Requirements: a gozen built with cgo, an ONNX Runtime shared library, and the face model " +
			"(gozen models fetch face). A build without cgo says so and fails. The preview directory must not exist yet. See " + clihelp.README},
	},
	"version": {
		Summary:  "print the gozen version",
		Usage:    []string{"gozen version", "gozen --version"},
		About:    "Prints the version recorded by go install, or devel for a build from source.",
		Examples: []clihelp.Example{{"print the version", "gozen version"}, {"the same with the option", "gozen --version"}},
		Groups:   []clihelp.Group{general},
	},
}

func init() {
	for name, p := range pages {
		p.Name = "gozen " + name
		pages[name] = p
	}
	for name, p := range groups {
		p.Name = "gozen " + name
		groups[name] = p
	}
}

// helpOption is derived from the same registered flag used by execution.
type helpOption struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Type        string   `json:"type"`
	Default     string   `json:"default"`
	Value       string   `json:"value,omitempty"`
	Description string   `json:"description"`
	Group       string   `json:"group,omitempty"`
	Repeatable  bool     `json:"repeatable"`
	Choices     []string `json:"choices,omitempty"`
	Environment []string `json:"environment,omitempty"`
}
type helpCommand struct {
	Command     string            `json:"command"`
	Summary     string            `json:"summary"`
	Usage       []string          `json:"usage"`
	About       string            `json:"about,omitempty"`
	Subcommands []string          `json:"subcommands"`
	Options     []helpOption      `json:"options"`
	Examples    []clihelp.Example `json:"examples"`
	Notes       []string          `json:"notes"`
}

func describeCommand(path string, n node, registry map[string]node) (helpCommand, clihelp.Page, *pflag.FlagSet) {
	c := &command{registry: registry, path: path, out: &cliio.Output{Writer: io.Discard}, stderr: &cliio.Output{Writer: io.Discard}}
	var p clihelp.Page
	var f *pflag.FlagSet
	if n.build != nil {
		op := c.operation(n)
		p, f = op.Page, op.Flags
	} else {
		f = clihelp.NewFlagSet("gozen " + path)
		p = groups[path]
		if path == "" {
			f = clihelp.NewFlagSet("gozen")
			f.Bool("version", false, "print the gozen version and exit")
			p = topPage
		}
	}
	result := helpCommand{Command: path, Summary: p.Summary, Usage: append([]string{}, p.Usage...), About: p.About, Subcommands: append([]string{}, n.subs...), Options: []helpOption{}, Examples: append([]clihelp.Example{}, p.Examples...), Notes: append([]string{}, p.Notes...)}
	f.VisitAll(func(flag *pflag.Flag) {
		value, description := pflag.UnquoteUsage(flag)
		group := ""
		for _, g := range p.Groups {
			if slices.Contains(g.Flags, flag.Name) {
				group = g.Title
				break
			}
		}
		typ := flag.Value.Type()
		result.Options = append(result.Options, helpOption{Name: flag.Name, Short: flag.Shorthand, Type: typ, Default: flag.DefValue, Value: value, Description: description, Group: group, Repeatable: typ == "stringArray" || typ == "stringSlice" || typ == "coords", Choices: slices.Clone(flag.Annotations["choices"]), Environment: slices.Clone(flag.Annotations["env"])})
	})
	return result, p, f
}

func (c *command) helpCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen help")
	c.reporting(f)
	p := clihelp.Page{Name: "gozen help", Summary: "describe commands or diagnostic codes", Usage: []string{"gozen help [COMMAND ... | CODE] [--format text|json|jsonl]"}, Examples: []clihelp.Example{{"describe all commands", "gozen help --format json"}, {"describe one command", "gozen help capture --format json"}}, Groups: []clihelp.Group{{Title: "Output", Flags: []string{"format"}}, general}, Notes: []string{noRequirements}}
	return clihelp.Operation{Flags: f, Page: p, Run: func(context.Context) error {
		words := f.Args()
		for len(words) > 0 && words[0] == "help" {
			words = words[1:]
		}
		if len(words) == 1 && diag.Syntax(words[0]) {
			if !c.report.Machine() {
				return c.explain(words[0])
			}
			code, ok := diag.Parse(words[0])
			if !ok {
				return cliio.Invalid(diag.UnknownCode, "unknown diagnostic code %s", words[0])
			}
			entry, _ := diag.Lookup(code)
			return c.report.Result(c.out, "ok", map[string]any{"code": code.String(), "exit_code": code.ExitCode(), "summary": entry.Summary, "detail": entry.Detail, "fix": entry.Fix, "since": entry.Since})
		}
		path := strings.Join(words, " ")
		n, ok := c.registry[path]
		if !ok {
			return cliio.Invalid(diag.UnknownCommand, "unknown command %q; use gozen help", path)
		}
		if !c.report.Machine() {
			_, page, flags := describeCommand(path, n, c.registry)
			c.out.Print(page.String(flags))
			return nil
		}
		paths := []string{path}
		if path == "" {
			paths = slices.Sorted(maps.Keys(c.registry))
		}
		commands := make([]helpCommand, 0, len(paths))
		for _, path := range paths {
			entry, _, _ := describeCommand(path, c.registry[path], c.registry)
			commands = append(commands, entry)
			if c.report.Format == "jsonl" {
				if err := c.report.Item(c.out, path, "ok", entry); err != nil {
					return err
				}
			}
		}
		if c.report.Format == "jsonl" {
			return c.report.Result(c.out, "ok", map[string]int{"commands": len(commands)})
		}
		return c.report.Result(c.out, "ok", struct {
			Commands []helpCommand `json:"commands"`
		}{commands})
	}}
}
