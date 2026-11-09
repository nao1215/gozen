package hygiene

// Level is one rank of the dependency order (ADR 0064), from the primitive
// foundation up to the entry points the author runs. It plays the part of a
// Linux initcall level: the rank has a name, and its place in Levels is its
// number, written once here.
//
// A component of .go-arch-lint.yml may depend only on components of its own
// level or of a lower one. Dir is the directory that holds the level's
// packages; the application level has none, because its commands and tools
// keep the places other decisions give them (cmd/, internal/promptgen,
// internal/mosaic, internal/dev, internal/testgame).
type Level struct {
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	Components []string `json:"components"`
}

// Levels returns the levels from the lowest to the highest. The hygiene tests
// (TestLevelOrder) check the architecture policy against it; nothing else reads
// it, so the order cannot drift between two copies.
func Levels() []Level {
	return []Level{
		// Values, expressions, texts, save bytes, chat and conversation rules,
		// diagnostics and the CLI and browser I/O they share. Nothing of a game.
		{"base", "internal/base", []string{"base", "expr", "text", "savefile", "chat", "react", "diag", "clihelp", "cliio", "webfs"}},
		// The game's definitions (YAML types, their checks and loading) and its state.
		{"model", "internal/model", []string{"model", "rpg"}},
		// The rules that change the state: battles, events, places, campaigns, dungeons.
		{"rules", "internal/rules", []string{"rules", "battle", "event", "world"}},
		// Running the rules without a screen: progress, sessions and simulation.
		{"drive", "internal/drive", []string{"drive", "headless", "session"}},
		// Drawing, sound, portraits and the player.
		{"screen", "internal/screen", []string{"screen", "play"}},
		// Entry points, authoring tools and maintainer tooling.
		{"app", "", []string{"commands", "repro", "dango", "player", "promptgen", "promptcmd", "mosaic", "mosaiccmd", "experiments", "examples", "dev", "testgame"}},
	}
}
