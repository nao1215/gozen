package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The optional parts of a game each have a line in the book when the game uses them.
func TestBookShowsDescriptionsRoamersLooksMusicAndBranches(t *testing.T) {
	dir := designGame(t)
	world := filepath.Join(dir, "data", "world.yaml")
	edit(t, world, "    name: Camp\n", "    name: Camp\n    desc: A quiet clearing.\n")
	edit(t, world, "      tiles:\n        S: {start: true}", `      roamers:
        - name: Ghoul
          picture: mote.png
          move: chase
          appear: "flag.met"
          every: "3"
          troops: [{troop: lesson}]
          meet: [{say: Boo}]
        - name: Brute
          picture: mote.png
          move: wander
          count: "1"
          troops: [{troop: pair}]
      looks:
        - {when: "var.floor >= 2", wall: mote.png, battle_background: mote.png, area: Deep}
      tiles:
        S: {start: true}`)
	edit(t, world, "events:\n", `events:
  duel:
    do:
      - battle: rival
        fighters: [scout]
        on_win: [{say: Won}]
        on_lose: [{say: Lost}]
  watch: {trigger: enter, location: camp, when: "gold > 0", do: [{say: Welcome back.}]}
`)
	write(t, filepath.Join(dir, "assets", "calm.ogg"), "sentinel")
	write(t, filepath.Join(dir, "data", "music.yaml"), `music:
  songs:
    theme: {file: calm.ogg, name: Theme}
    wind: {dir: wind, bpm: 90, moods: {calm: calm, tense: tense}}
  rules:
    field:
      - {when: "gold > 100", play: theme, now: true}
      - {when: "gold <= 100", mood: calm}
`)
	write(t, filepath.Join(dir, "assets", "wind", "calm.ogg"), "sentinel")
	write(t, filepath.Join(dir, "assets", "wind", "tense.ogg"), "sentinel")
	out, _ := invoke(t, 0, "book", dir)
	for _, want := range []string{
		"A quiet clearing.",
		"  - Chaser Ghoul (chase, appears flag.met, 1 cell per 3 steps)",
		"    - Boo",
		"  - Strong foe Brute (wander, count 1): [pair]",
		"  - Look var.floor >= 2: wall mote.png, battle background mote.png",
		"- battle rival (only [scout])",
		"- on a win\n  - Won",
		"- on a loss\n  - Lost",
		"### watch\n\n- Welcome back.",
		"at_camp -->|enter gold > 0| watch",
		"## Music",
		"- theme (Theme): ",
		"- wind (): calm tense",
		"- Rule field: gold > 100 → theme (now)",
		"- Rule field: gold <= 100 → calm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the book lacks %q", want)
		}
	}
}
