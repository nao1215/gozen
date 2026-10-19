package main

import (
	"github.com/nao1215/gozen/internal/clihelp"
)

// The help pages. The examples use the neutral fixture in e2e/atago/fixtures
// (a game directory "game" with the troop "lesson" and the dungeon "grotto").

const noRequirements = "Requirements: none beyond the gozen binary. No cgo, C compiler, ONNX Runtime, model, display, or sound device is needed. See " + clihelp.README

var general = clihelp.Group{Title: "General", Flags: []string{"help"}}

var topPage = clihelp.Page{
	Name:    "gozen",
	Summary: "check, simulate, document, and package Gozen RPG games",
	Usage:   []string{"gozen <command> [OPTIONS] [ARGUMENTS]", "gozen help <command>"},
	About: "gozen reads a game directory (game.yaml, data/, assets/) and checks the definitions, " +
		"fights AUTO battles, measures the growth curve, documents the events, builds release " +
		"and trial trees, takes screenshots, and checks moving portraits. Results go to stdout, diagnostics to stderr. " +
		"Exit status: 0 for success and help, 1 when a check, I/O, simulation, or capture fails, " +
		"2 for invalid arguments.",
	Commands: [][2]string{
		{"check", "Check definitions, references, expressions, assets, and text layout"},
		{"layout", "Check that dialogue and choices fit the message window"},
		{"sim", "Fight AUTO battles against a troop and print the win rate"},
		{"curve", "Simulate a dive floor by floor: growth, battles, and bosses"},
		{"floors", "Show generated dungeon floors: maps, steps, and encounters"},
		{"trace", "Print or check a JSON Lines trace of a play"},
		{"book", "Write the definitions and events as Markdown"},
		{"design", "Play the game with personas and write a design report"},
		{"memos", "Turn test-play notes into HTML with thumbnails"},
		{"release", "Build a release tree with approved assets only"},
		{"trial", "Build a trial tree with the reachable part of the game"},
		{"shots", "Launch the game and take the screenshots of a list"},
		{"puppet", "Check a moving portrait and cut its difference pictures"},
		{"prompt", "Assemble image-generation text from a catalog"},
		{"mosaic", "Pixelate regions of images, with optional ONNX detection"},
		{"version", "Print the gozen version"},
		{"help", "Show the help of gozen or of a command"},
	},
	Examples: []clihelp.Example{
		{"check a game directory", "gozen check game"},
		{"check the structure only, before the game has fonts", "gozen check --no-layout game"},
		{"fight 100 battles against the troop lesson", "gozen sim game lesson -n 100"},
		{"show the options of one command", "gozen sim --help"},
	},
	Groups: []clihelp.Group{{Title: "General", Flags: []string{"help", "version"}}},
	Notes: []string{
		"Options follow the GNU style: --name=value and --name value both work, a few common options " +
			"have a one-letter form (-n 100), options may come before or after the arguments, and -- ends " +
			"the options. Single-dash long options such as -seed are rejected.",
		"Requirements: checks, simulations, reports, and prompt need nothing beyond the gozen binary; " +
			"cgo is not required. shots launches a graphics game, and unattended capture on Linux needs Xvfb. " +
			"mosaic inference needs a gozen built with cgo, an ONNX Runtime shared library, and user-supplied " +
			"models. See " + clihelp.README,
	},
	Hint: `Run "gozen <command> --help" for a command's options and examples.`,
}

var pages = map[string]clihelp.Page{
	"check": {
		Summary: "check the definitions, assets, and text layout of a game",
		Usage:   []string{"gozen check [OPTIONS] <game-dir>"},
		About: "Loads every definition of the game, resolves references and expressions, checks that " +
			"the assets exist, and lays out dialogue with the game's font. Warnings go to stderr and do " +
			"not fail the check. With --no-layout, the font is not read and the text layout is not checked; " +
			"stdout says so.",
		Examples: []clihelp.Example{
			{"check a game", "gozen check game"},
			{"check the structure only, without fonts", "gozen check --no-layout game"},
			{"check a directory whose name starts with a dash", "gozen check -- -draft"},
		},
		Groups: []clihelp.Group{{Title: "Check", Flags: []string{"no-layout"}}, general},
		Notes:  []string{noRequirements},
	},
	"layout": {
		Summary: "check that dialogue and choices fit the message window",
		Usage:   []string{"gozen layout [OPTIONS] <game-dir>"},
		About: "Lays out every line of dialogue and every choice with the game's font and display rules " +
			"and reports the ones that overflow. Warnings alone exit 0.",
		Examples: []clihelp.Example{
			{"check the text layout of a game", "gozen layout game"},
			{"check definitions and layout together", "gozen check game"},
		},
		Groups: []clihelp.Group{general},
		Notes:  []string{noRequirements},
	},
	"sim": {
		Summary: "fight AUTO battles against a troop and print the win rate",
		Usage:   []string{"gozen sim [OPTIONS] <game-dir> <troop>"},
		About: "Fights the troop with the starting party (or --party, or a saved state) under AUTO and " +
			"prints the win, loss, flight, and draw rates and the average turns. The numbers are the " +
			"same as the development balance panel (F7) of the game. The same --seed gives the same result.",
		Examples: []clihelp.Example{
			{"fight 100 battles", "gozen sim game lesson -n 100"},
			{"choose the party, its level, and another seed", "gozen sim game lesson -n 100 --party scout --level 5 --seed 7"},
			{"start from a saved test-play note and show the HP left after a win", "gozen sim game lesson --state memo.save --hp"},
			{"keep the saved HP and count the wins in a row, up to 10", "gozen sim game lesson --state memo.save --as-is --streak 10"},
			{"watch the messages of the first battle", "gozen sim game lesson -n 1 -v"},
		},
		Groups: []clihelp.Group{
			{Title: "Party", Flags: []string{"party", "level", "gear", "no-tree", "state", "as-is"}},
			{Title: "Battle", Flags: []string{"battles", "turns", "seed", "streak"}},
			{Title: "Output", Flags: []string{"hp", "verbose"}},
			general,
		},
		Notes: []string{noRequirements},
	},
	"curve": {
		Summary: "simulate a dive floor by floor: growth, battles, bosses",
		Usage:   []string{"gozen curve [OPTIONS] <game-dir> <location>"},
		About: "Takes the starting party down the dungeon location, fighting --battles battles on each " +
			"floor, buying better equipment from --shops, resting, and fighting the bosses. Prints the " +
			"level, HP, gold, and win rates of each floor.",
		Examples: []clihelp.Example{
			{"simulate three floors of five battles", "gozen curve game grotto --floors 3 --battles 5"},
			{"add a boss on floor 3 and grind until it is won 80% of the time", "gozen curve game grotto --floors 3 --boss 3:lesson --grind 0.8"},
			{"rest only every two floors, and fight 50 battles for each boss win rate", "gozen curve game grotto --floors 4 --rest 2 --boss 2:lesson --tries 50"},
		},
		Groups: []clihelp.Group{
			{Title: "Dive", Flags: []string{"floors", "battles", "boss", "tries", "grind", "turns", "seed"}},
			{Title: "Town", Flags: []string{"shops", "rest", "supplies", "rest-cost", "rest-event", "back-item", "worn-hp", "worn-mp"}},
			{Title: "Events", Flags: []string{"events", "scene-event", "scene-when"}},
			general,
		},
		Notes: []string{noRequirements},
	},
	"floors": {
		Summary: "show generated floors: maps, steps, and encounters",
		Usage:   []string{"gozen floors [OPTIONS] <game-dir> <location>"},
		About: "Generates the floors of the dungeon location and prints each map with the steps to the " +
			"stairs and the expected encounters. With --try, compares seeds on one floor, one line each.",
		Examples: []clihelp.Example{
			{"show floors 1 to 3", "gozen floors game grotto --from 1 --to 3"},
			{"compare three seeds on floor 1", "gozen floors game grotto --floor 1 --try 3"},
			{"print the numbers only", "gozen floors game grotto --no-map"},
		},
		Groups: []clihelp.Group{{Title: "Floors", Flags: []string{"from", "to", "floor", "try"}}, {Title: "Output", Flags: []string{"no-map"}}, general},
		Notes:  []string{noRequirements},
	},
	"trace": {
		Summary: "print or check a JSON Lines trace of a play",
		Usage:   []string{"gozen trace [OPTIONS] <trace.jsonl>"},
		About: "Prints a trace written by the game's --trace in time order. With --check, lists the " +
			"faults of the standing pictures and the music instead and exits 1 if there are any.",
		Examples: []clihelp.Example{
			{"print a trace", "gozen trace run.jsonl"},
			{"print only dialogue and battles between seconds 10 and 60", "gozen trace run.jsonl --only say,battle --from 10 --to 60"},
			{"check pictures and music", "gozen trace run.jsonl --check"},
		},
		Groups: []clihelp.Group{
			{Title: "Filter", Flags: []string{"only", "from", "to", "input"}},
			{Title: "Check", Flags: []string{"check", "music-late"}},
			general,
		},
		Notes: []string{noRequirements},
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
		Notes:  []string{noRequirements},
	},
	"design": {
		Summary: "play the game with personas and write a design report",
		Usage:   []string{"gozen design [OPTIONS] <game-dir>"},
		About: "Plays the game headless with the personas of designeval.yaml (players, rewards, and time " +
			"weights) and writes summary.json, the timelines, band.png, and report.md to a new directory. " +
			"It does not run Git or collect anything from the environment. A simulation error keeps the " +
			"report and exits 1.",
		Examples: []clihelp.Example{
			{"write a report to a new directory", "gozen design game -o report"},
			{"run two personas with their logs", "gozen design game -o report --only walker -v"},
			{"use another configuration and keep notes", "gozen design game --config game/designeval.yaml --memos notes"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "only"}},
			{Title: "Output", Flags: []string{"out", "root", "memos", "verbose"}},
			general,
		},
		Notes: []string{"The output directory must not exist yet. " + noRequirements},
	},
	"memos": {
		Summary: "turn test-play notes into HTML with thumbnails",
		Usage:   []string{"gozen memos [OPTIONS] <memo-dir>"},
		About:   "Reads the notes the game writes with F9 and builds an HTML page with thumbnails in a new directory. The notes are not changed.",
		Examples: []clihelp.Example{
			{"write build/memos-html", "gozen memos build/memos"},
			{"choose the output directory", "gozen memos build/memos -o notes-html"},
		},
		Groups: []clihelp.Group{{Title: "Output", Flags: []string{"out"}}, general},
		Notes:  []string{"The output directory must not exist yet. " + noRequirements},
	},
	"release": {
		Summary: "build a release tree with approved assets only",
		Usage:   []string{"gozen release [OPTIONS] --inspect FILE <game-dir> <out-dir>"},
		About: "Checks the approval records and the assets the game refers to, then copies the game to a " +
			"new directory, taking replaced assets from the overlay. Any unapproved reference fails the build.",
		Examples: []clihelp.Example{
			{"build a release tree", "gozen release game release --inspect inspect.jsonl"},
			{"take replaced assets from an overlay directory", "gozen release game release --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{{Title: "Assets", Flags: []string{"inspect", "overlay", "prefix"}}, general},
		Notes:  []string{"The output directory must not exist yet and must not be inside an input. " + noRequirements},
	},
	"trial": {
		Summary: "build a trial tree with the reachable part of the game",
		Usage:   []string{"gozen trial [OPTIONS] <game-dir> <out-dir>", "gozen trial --dry-run [OPTIONS] <game-dir>"},
		About: "Keeps only the definitions and assets reachable within the range of trial.yaml and writes " +
			"them to a new directory. --overlay and --inspect go together.",
		Examples: []clihelp.Example{
			{"count what would be kept, writing nothing", "gozen trial game -n"},
			{"build a trial tree and list what was kept and dropped", "gozen trial game trial --list trial.txt"},
			{"take replaced assets from an overlay", "gozen trial game trial --overlay overlay --inspect inspect.jsonl"},
		},
		Groups: []clihelp.Group{
			{Title: "Input", Flags: []string{"config", "overlay", "inspect"}},
			{Title: "Output", Flags: []string{"list", "dry-run"}},
			general,
		},
		Notes: []string{"The output directory and the --list file must not exist yet. --dry-run writes nothing, even with --list. " + noRequirements},
	},
	"shots": {
		Summary: "launch the game and take the screenshots of a list",
		Usage:   []string{"gozen shots --bin FILE (--raw | --overlay DIR) [OPTIONS] <game-dir> <list>"},
		About: "Launches the game binary once per entry of the list and writes PNG, JPEG, an HTML index, " +
			"and shots.json to a new directory. Saves go to a work tree per shot. With --overlay, approval " +
			"is checked and a pending approval exits 1.",
		Examples: []clihelp.Example{
			{"take the shots with the original assets", "gozen shots game shots.yaml --bin ./mygame --raw -o shots"},
			{"take only one of them, with a shorter time limit", "gozen shots game shots.yaml --bin ./mygame --raw --only camp --timeout 1m"},
			{"run unattended on Linux", "xvfb-run -a gozen shots game shots.yaml --bin ./mygame --raw"},
		},
		Groups: []clihelp.Group{
			{Title: "Game", Flags: []string{"bin", "raw", "overlay", "timeout"}},
			{Title: "Output", Flags: []string{"out", "only"}},
			general,
		},
		Notes: []string{
			"Requirements: a game binary built with the Gozen runner, and its graphics runtime (a display " +
				"and the platform graphics and audio libraries). Unattended capture on Linux needs Xvfb " +
				"(sudo apt-get install xvfb xauth). A failed launch is recorded in the work tree and exits 1. " +
				"See " + clihelp.README,
		},
	},
	"puppet": {
		Summary: "check a moving portrait and cut its difference pictures",
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
		Notes:  []string{"The output directory must not exist yet. " + noRequirements},
	},
	"version": {
		Summary:  "print the gozen version",
		Usage:    []string{"gozen version", "gozen --version"},
		About:    "Prints the version recorded by go install, or devel for a build from source.",
		Examples: []clihelp.Example{{"print the version", "gozen version"}, {"the same with the option", "gozen -v"}},
		Groups:   []clihelp.Group{general},
		Notes:    []string{noRequirements},
	},
}

func init() {
	for name, p := range pages {
		p.Name = "gozen " + name
		pages[name] = p
	}
}
