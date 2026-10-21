package main

import (
	"github.com/nao1215/gozen/internal/clihelp"
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
	Summary: "check, simulate, document, and package Gozen RPG games",
	Usage:   []string{"gozen <command> [<sub-command>] [OPTIONS] [ARGUMENTS]", "gozen help [<command> ...]", "gozen help <code>"},
	About: "gozen reads a game directory (game.yaml, data/, assets/) and checks the definitions, " +
		"measures play without a window, reads and shoots real plays, documents the events, builds " +
		"release and trial trees, and runs the picture and asset tools. Results go to stdout, " +
		"diagnostics to stderr.",
	Commands: [][2]string{
		{"check", "Check a game directory: definitions, references, expressions, assets, text layout"},
		{"sim", "Measure play without a window: battle, dive, floors, design, campaign"},
		{"play", "Read and shoot real plays: trace, notes, shots"},
		{"book", "Write the definitions and events as Markdown"},
		{"pack", "Build what you ship: release, trial"},
		{"art", "Picture and asset tools: prompt, mosaic, fetch, faces, puppet"},
		{"theme", "Dungeon themes: add the pictures of a theme to a game, list the themes"},
		{"version", "Print the gozen version"},
		{"help", "Show the help of a command, or explain a diagnostic code"},
	},
	Examples: []clihelp.Example{
		{"check a game directory", "gozen check game"},
		{"fight 100 battles against the troop lesson", "gozen sim battle game lesson -n 100"},
		{"print the trace of a play", "gozen play trace run.jsonl"},
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
		"Requirements: check, sim, book, pack, play trace, play notes, and art prompt need nothing beyond " +
			"the gozen binary; cgo is not required. play shots launches a graphics game, and unattended " +
			"capture on Linux needs Xvfb. art mosaic inference and art faces need a gozen built with cgo, " +
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
		},
		Examples: []clihelp.Example{
			{"fight 100 battles", "gozen sim battle game lesson -n 100"},
			{"simulate three floors of five battles", "gozen sim dive game grotto --floors 3 --battles 5"},
			{"compare three seeds on floor 1", "gozen sim floors game grotto --floor 1 --try 3"},
			{"write a design report to a new directory", "gozen sim design game -o report"},
		},
		Groups: []clihelp.Group{general},
		Notes: []string{"--seed, --turns, and --format mean the same in every command: the same --seed " +
			"gives the same result, --turns ends a battle as a draw, and --format json prints one JSON " +
			"document for a tool or an agent.", noRequirements},
		Hint: `Run "gozen sim <command> --help" for a command's options and examples.`,
	},
	"play": {
		Summary: "read and shoot real plays",
		Usage:   []string{"gozen play <command> [OPTIONS] ..."},
		About: "Reads and takes pictures of real plays; the game binary itself is run by its own --dev. " +
			"trace reads the JSON Lines a game writes with --trace, notes turns the F9 notes into HTML, " +
			"and shots launches the game binary to take a list of screenshots.",
		Commands: [][2]string{
			{"trace", "Print or check a JSON Lines trace of a play"},
			{"notes", "Turn test-play notes into HTML with thumbnails"},
			{"shots", "Launch the game and take the screenshots of a list"},
		},
		Examples: []clihelp.Example{
			{"print a trace", "gozen play trace run.jsonl"},
			{"write build/memos-html", "gozen play notes build/memos"},
			{"take the shots with the original assets", "gozen play shots game shots.yaml --bin ./mygame --raw -o shots"},
		},
		Groups: []clihelp.Group{general},
		Notes: []string{"Requirements: trace and notes need nothing beyond the gozen binary. shots needs a " +
			"game binary built with the Gozen runner and its graphics runtime (a display and the platform " +
			"graphics and audio libraries); unattended capture on Linux needs Xvfb (sudo apt-get install " +
			"xvfb xauth). See " + clihelp.README},
		Hint: `Run "gozen play <command> --help" for a command's options and examples.`,
	},
	"pack": {
		Summary: "build what you ship",
		Usage:   []string{"gozen pack <command> [OPTIONS] <game-dir> <out-dir>"},
		About: "Copies the game to a new directory: release with approved assets only, trial with the " +
			"reachable part of the game. --overlay takes replaced assets from a directory, and --inspect " +
			"reads the approval records that every asset of the overlay must have.",
		Commands: [][2]string{
			{"release", "Build a release tree with approved assets only"},
			{"trial", "Build a trial tree with the reachable part of the game"},
		},
		Examples: []clihelp.Example{
			{"build a release tree", "gozen pack release game release --inspect inspect.jsonl"},
			{"count what a trial would keep, writing nothing", "gozen pack trial game -n"},
		},
		Groups: []clihelp.Group{general},
		Notes:  []string{"The output directory must not exist yet and must not be inside an input.", noRequirements},
		Hint:   `Run "gozen pack <command> --help" for a command's options and examples.`,
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
			{"add the ice theme to a game", "gozen theme add ice game"},
		},
		Groups: []clihelp.Group{general},
		Notes:  []string{noRequirements},
		Hint:   `Run "gozen theme <command> --help" for a command's options and examples.`,
	},
	"art": {
		Summary: "picture and asset tools",
		Usage:   []string{"gozen art <command> [OPTIONS] ..."},
		About: "Tools for the pictures of a game: assemble image-generation text from a catalog, pixelate " +
			"regions of images, download the detection models, find the faces of standing pictures for " +
			"the balloons, and check a moving portrait.",
		Commands: [][2]string{
			{"prompt", "Assemble image-generation text from a catalog"},
			{"mosaic", "Pixelate regions of images, with optional ONNX detection"},
			{"fetch", "Download the pinned detection models into the cache"},
			{"faces", "Find the faces of the standing pictures for the balloons"},
			{"puppet", "Check a moving portrait and cut its difference pictures"},
		},
		Examples: []clihelp.Example{
			{"check a catalog", "gozen art prompt lint --config catalog.yaml"},
			{"list the models and whether each is fetched", "gozen art fetch --list"},
			{"check a puppet and show the positions", "gozen art puppet streamer/puppet.yaml"},
		},
		Groups: []clihelp.Group{general},
		Notes: []string{"Requirements: prompt, fetch, and puppet need nothing beyond the gozen binary " +
			"(fetch needs the network). mosaic inference and faces need a gozen built with cgo " +
			"(CGO_ENABLED=1 and a C compiler), an ONNX Runtime shared library, and user-supplied or fetched " +
			"models; a build without cgo says so and exits 1. Models are never bundled. See " + clihelp.README},
		Hint: `Run "gozen art <command> --help" for a command's options and examples.`,
	},
}

var pages = map[string]clihelp.Page{
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
		Usage:   []string{"gozen theme add [OPTIONS] <name> <game-dir>"},
		About: "Copies the wall, floor, ceiling and door pictures and the colors of the theme to " +
			"assets/dungeon/themes/<name>/ of the game, so that theme: <name> in a dungeon or one of its " +
			"looks fills the walls, floor, ceiling, door and colors. A file the game already has is kept; " +
			"--force replaces it. The pictures come with the gozen binary: no network is used.",
		Examples: []clihelp.Example{
			{"add the cave theme", "gozen theme add cave game"},
			{"restore the pictures of the cave theme", "gozen theme add cave game --force"},
		},
		Groups: []clihelp.Group{{Title: "Files", Flags: []string{"force"}}, general},
		Notes:  []string{"The theme's pictures are CC BY 4.0; see ASSETS.md of the gozen repository.", noRequirements},
	},
	"theme list": {
		Summary: "list the dungeon themes",
		Usage:   []string{"gozen theme list"},
		About:   "Prints the name of each theme the gozen binary carries, with a line on how it looks.",
		Examples: []clihelp.Example{
			{"list the themes", "gozen theme list"},
		},
		Groups: []clihelp.Group{general},
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
		Usage:   []string{"gozen sim battle [OPTIONS] <game-dir> <troop>"},
		About: "Fights the troop with the starting party (or --party, or a saved state) under AUTO and " +
			"prints the win, loss, flight, and draw rates and the average turns. The numbers are the " +
			"same as the development balance panel (F7) of the game. The same --seed gives the same result.",
		Examples: []clihelp.Example{
			{"fight 100 battles", "gozen sim battle game lesson -n 100"},
			{"choose the party, its level, and another seed", "gozen sim battle game lesson -n 100 --party scout --level 5 --seed 7"},
			{"start from a saved test-play note and show the HP left after a win", "gozen sim battle game lesson --state memo.save --hp"},
			{"keep the saved HP and count the wins in a row, up to 10", "gozen sim battle game lesson --state memo.save --as-is --streak 10"},
			{"watch the messages of the first battle", "gozen sim battle game lesson -n 1 -v"},
		},
		Groups: []clihelp.Group{
			{Title: "Party", Flags: []string{"party", "level", "gear", "no-tree", "state", "as-is"}},
			{Title: "Battle", Flags: []string{"battles", "turns", "seed", "streak"}},
			{Title: "Output", Flags: []string{"hp", "verbose", "format"}},
			general,
		},
	},
	"sim dive": {
		Summary: "simulate a dive floor by floor",
		Usage:   []string{"gozen sim dive [OPTIONS] <game-dir> <location>"},
		About: "Takes the starting party down the dungeon location, fighting --battles battles on each " +
			"floor, buying better equipment from --shops, resting, and fighting the bosses. Prints the " +
			"level, HP, gold, and win rates of each floor.",
		Examples: []clihelp.Example{
			{"simulate three floors of five battles", "gozen sim dive game grotto --floors 3 --battles 5"},
			{"add a boss on floor 3 and grind until it is won 80% of the time", "gozen sim dive game grotto --floors 3 --boss 3:lesson --grind 0.8"},
			{"rest only every two floors, and fight 50 battles for each boss win rate", "gozen sim dive game grotto --floors 4 --rest 2 --boss 2:lesson --tries 50"},
		},
		Groups: []clihelp.Group{
			{Title: "Dive", Flags: []string{"floors", "battles", "boss", "guards", "retreat", "guards-only", "tries", "grind", "turns", "seed"}},
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
	"sim routes": {
		Summary: "find unreachable endings and choice options",
		Usage:   []string{"gozen sim routes [OPTIONS] <game-dir>"},
		About: "Explores event choices without a screen, counting paths to clear and game-over endings, " +
			"and listing authored choice options that were never offered. Limits bound paths, event requests " +
			"per path, and elapsed time. Findings do not fail unless --fail-unreachable is set.",
		Examples: []clihelp.Example{
			{"explore a visual novel", "gozen sim routes examples/novel/game"},
			{"write JSON and fail CI on unreachable content", "gozen sim routes game --format json --fail-unreachable"},
		},
		Groups: []clihelp.Group{{Title: "Exploration", Flags: []string{"paths", "depth", "timeout"}}, {Title: "Output", Flags: []string{"format", "fail-unreachable"}}, general},
		Notes:  []string{noRequirements},
	},
	"sim floors": {
		Summary: "show generated floors: maps, steps, and encounters",
		Usage:   []string{"gozen sim floors [OPTIONS] <game-dir> <location>"},
		About: "Generates the floors of the dungeon location and prints each map with the steps to the " +
			"stairs and the expected encounters. With --try, compares seeds on one floor, one line each.",
		Examples: []clihelp.Example{
			{"show floors 1 to 3", "gozen sim floors game grotto --from 1 --to 3"},
			{"compare three seeds on floor 1", "gozen sim floors game grotto --floor 1 --try 3"},
			{"print the numbers only", "gozen sim floors game grotto --no-map"},
		},
		Groups: []clihelp.Group{{Title: "Floors", Flags: []string{"from", "to", "floor", "try"}}, {Title: "Output", Flags: []string{"no-map"}}, general},
	},
	"sim design": {
		Summary: "play the game with personas and write a report",
		Usage:   []string{"gozen sim design [OPTIONS] <game-dir>"},
		About: "Plays the game headless with the personas of designeval.yaml (players, rewards, and time " +
			"weights) and writes summary.json, the timelines, band.png, and report.md to a new directory. " +
			"It does not run Git or collect anything from the environment. A simulation error keeps the " +
			"report and exits 1.",
		Examples: []clihelp.Example{
			{"write a report to a new directory", "gozen sim design game -o report"},
			{"run two personas with their logs", "gozen sim design game -o report --only walker -v"},
			{"use another configuration and keep notes", "gozen sim design game --config game/designeval.yaml --memos notes"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "only", "seed", "list"}},
			{Title: "Output", Flags: []string{"out", "root", "memos", "verbose", "format"}},
			general,
		},
		Notes: []string{"The output directory must not exist yet."},
	},
	"play trace": {
		Summary: "print or check a JSON Lines trace of a play",
		Usage:   []string{"gozen play trace [OPTIONS] <trace.jsonl>"},
		About: "Prints a trace written by the game's --trace in time order. With --check, lists the " +
			"faults of the standing pictures and the music instead and exits 1 if there are any.",
		Examples: []clihelp.Example{
			{"print a trace", "gozen play trace run.jsonl"},
			{"print only dialogue and battles between seconds 10 and 60", "gozen play trace run.jsonl --only say,battle --from 10 --to 60"},
			{"check pictures and music", "gozen play trace run.jsonl --check"},
		},
		Groups: []clihelp.Group{
			{Title: "Filter", Flags: []string{"only", "from", "to", "input"}},
			{Title: "Check", Flags: []string{"check", "music-late"}},
			general,
		},
	},
	"play notes": {
		Summary: "turn test-play notes into HTML with thumbnails",
		Usage:   []string{"gozen play notes [OPTIONS] <memo-dir>"},
		About:   "Reads the notes the game writes with F9 and builds an HTML page with thumbnails in a new directory. The notes are not changed.",
		Examples: []clihelp.Example{
			{"write build/memos-html", "gozen play notes build/memos"},
			{"choose the output directory", "gozen play notes build/memos -o notes-html"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"out"}}, general},
		Notes:  []string{"The output directory must not exist yet."},
	},
	"play shots": {
		Summary: "launch the game and take the screenshots of a list",
		Usage:   []string{"gozen play shots --bin FILE (--raw | --overlay DIR) [OPTIONS] <game-dir> <list>"},
		About: "Launches the game binary once per entry of the list and writes PNG, JPEG, an HTML index, " +
			"and shots.json to a new directory. Saves go to a work tree per shot. With --overlay, approval " +
			"is checked and a pending approval exits 1. A failed launch is recorded in the work tree and exits 1.",
		Examples: []clihelp.Example{
			{"take the shots with the original assets", "gozen play shots game shots.yaml --bin ./mygame --raw -o shots"},
			{"take only one of them, with a shorter time limit", "gozen play shots game shots.yaml --bin ./mygame --raw --only camp --timeout 1m"},
			{"run unattended on Linux", "xvfb-run -a gozen play shots game shots.yaml --bin ./mygame --raw"},
		},
		Groups: []clihelp.Group{
			{Title: "Game", Flags: []string{"bin", "raw", "overlay", "timeout"}},
			{Title: "Output", Flags: []string{"out", "only"}},
			general,
		},
	},
	"book": {
		Summary: "write the definitions and events as Markdown",
		Usage:   []string{"gozen book [OPTIONS] <game-dir>"},
		About:   "Writes the locations, events, scenes, troops, and items of the game to stdout as one Markdown document.",
		Examples: []clihelp.Example{
			{"write the book to a file", "gozen book game > book.md"},
			{"read it in a pager", "gozen book game | less"},
		},
		Groups: []clihelp.Group{general},
	},
	"pack release": {
		Summary: "build a release tree with approved assets only",
		Usage:   []string{"gozen pack release [OPTIONS] --inspect FILE <game-dir> <out-dir>"},
		About: "Checks the approval records and the assets the game refers to, then copies the game to a " +
			"new directory, taking replaced assets from the overlay. Any unapproved reference fails the build.",
		Examples: []clihelp.Example{
			{"build a release tree", "gozen pack release game release --inspect inspect.jsonl"},
			{"take replaced assets from an overlay directory", "gozen pack release game release --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{{Title: "Assets", Flags: []string{"inspect", "overlay", "prefix"}}, general},
	},
	"pack trial": {
		Summary: "build a trial tree with the reachable part",
		Usage:   []string{"gozen pack trial [OPTIONS] <game-dir> <out-dir>", "gozen pack trial --dry-run [OPTIONS] <game-dir>"},
		About: "Keeps only the definitions and assets reachable within the range of trial.yaml and writes " +
			"them to a new directory. --overlay and --inspect go together.",
		Examples: []clihelp.Example{
			{"count what would be kept, writing nothing", "gozen pack trial game -n"},
			{"build a trial tree and list what was kept and dropped", "gozen pack trial game trial --list trial.txt"},
			{"take replaced assets from an overlay", "gozen pack trial game trial --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "overlay", "inspect"}},
			{Title: "Output", Flags: []string{"list", "dry-run"}},
			general,
		},
		Notes: []string{"The --list file must not exist yet. --dry-run writes nothing, even with --list."},
	},
	"art puppet": {
		Summary: "check a moving portrait and cut its pictures",
		Usage:   []string{"gozen art puppet [OPTIONS] <puppet.yaml>"},
		About: "Loads a puppet (a moving portrait for the puppet package) with the same checks as puppet.Load " +
			"and prints where each face, closed-eyes, and mouth picture lies on the base. A difference " +
			"picture as large as the base is cut down to the part that differs. With --out, writes the " +
			"puppet YAML and the cut pictures (each with at) to a new directory, so the game ships small " +
			"pictures. The written YAML has no comments.",
		Examples: []clihelp.Example{
			{"check a puppet and show the positions", "gozen art puppet streamer/puppet.yaml"},
			{"write it with small difference pictures", "gozen art puppet streamer/puppet.yaml --out streamer-packed"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"out"}}, general},
		Notes:  []string{"The output directory must not exist yet.", noRequirements},
	},
	"art faces": {
		Summary: "find the faces for the balloons and icons",
		Usage:   []string{"gozen art faces [OPTIONS] <game-dir>"},
		About: "Finds the face in each standing picture a painted scene (stage:) cuts a balloon face from: " +
			"the pictures of the people placed in a stage and the actors' face pictures, and, in a game with a " +
			"character book (profiles), the actors' and the profiles' pictures its face icons are cut from. Uses the anime face " +
			"detector that gozen art fetch face downloads, and writes the boxes, the pictures' sizes and " +
			"SHA-256 to assets/faces.json (replacing it). The game reads that file and runs no model, so a web " +
			"build uses it too. A doll's icon (a face drawn for the balloons) is not searched; it and face_at come first; without a box the face is guessed. gozen check " +
			"warns of a picture with no box or changed since.",
		Examples: []clihelp.Example{
			{"download the model once", "gozen art fetch face"},
			{"find the faces", "ONNXRUNTIME_LIB=/path/libonnxruntime.so gozen art faces game"},
			{"and look at what was found", "gozen art faces game --preview faces-check"},
		},
		Groups: []clihelp.Group{{Title: "Detection", Flags: []string{"model", "ort", "min"}}, {Title: "Output", Flags: []string{"preview"}}, general},
		Notes: []string{"Requirements: a gozen built with cgo, an ONNX Runtime shared library, and the face model " +
			"(gozen art fetch face). A build without cgo says so and fails. The preview directory must not exist yet. See " + clihelp.README},
	},
	"version": {
		Summary:  "print the gozen version",
		Usage:    []string{"gozen version", "gozen --version"},
		About:    "Prints the version recorded by go install, or devel for a build from source.",
		Examples: []clihelp.Example{{"print the version", "gozen version"}, {"the same with the option", "gozen -v"}},
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
