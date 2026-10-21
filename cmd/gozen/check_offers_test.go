package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

const offerWords = "A long recommendation keeps going and going, past the edge of the window "

// The stone shop's and the training hall's offers are lines too: a pitch that is longer than
// the window is reported with the offer it belongs to.
func TestCheckLayoutCoversTheStoneShopAndTheTrainingHallOffers(t *testing.T) {
	dir := fixture(t)
	game := string(read(t, filepath.Join(dir, "game.yaml")))
	write(t, filepath.Join(dir, "game.yaml"), strings.Replace(game, "start: {location: camp, party: [scout]}", `start: {location: camp, party: [scout]}
equip_slots:
  - {id: weapon, name: W}
  - {id: stone1, name: S1}
  - {id: stone2, name: S2}`, 1))
	world := filepath.Join(dir, "data", "world.yaml")
	text := string(read(t, world))
	text = strings.Replace(text, "    exp_for: level * 30\n", "    exp_for: level * 30\n    equip: [stone]\n    sp: 1\n    start_sp: 3\n    sp_pitch: \""+strings.Repeat(offerWords, 6)+"\"\n    tree:\n      - {id: jab, skill: attack}\n      - {id: guardup, skill: guard, requires: [{node: jab, level: 1}]}\n", 1)
	write(t, world, text)
	write(t, filepath.Join(dir, "data", "stones.yaml"), `items:
  white: {name: White Stone, short: White, kind: stone, slot: stone, stats: {def: 2}}
  blue: {name: Blue Stone, short: Blue, kind: stone, slot: stone, stats: {mat: 3}}
  red: {name: Red Stone, short: Red, kind: stone, slot: stone, stats: {atk: 3}}
stone_sets:
  frost: {name: Frost, stones: [white, blue], stats: {mdf: 4}, pitch: "`+strings.Repeat(offerWords, 4)+`"}
  ember: {name: Ember, stones: [red], stats: {atk: 5}, for: [scout], only: true, pitch: "`+strings.Repeat(offerWords, 4)+`"}
`)
	_, stderr := invoke(t, 0, "check", "--only", "layout", dir)
	if !strings.Contains(stderr, diag.TextOverflow.String()) {
		t.Fatalf("stderr: %q", stderr)
	}
	ps := checkJSONOut(t, 0, "--only", "layout", dir)
	var stones, training int
	for _, p := range ps {
		switch {
		case strings.Contains(p.File, "scout") && strings.Contains(p.File, "stone"):
			stones++
		case strings.Contains(p.File, "scout") && strings.Contains(p.File, "training"):
			training++
		}
		if p.Severity != "warning" || p.Code != diag.TextOverflow.String() {
			t.Errorf("%+v", p)
		}
	}
	if stones != 1 {
		t.Errorf("one stone offer for the one actor: %d in %+v", stones, ps)
	}
	if training != 1 {
		t.Errorf("one training offer for the one actor: %d in %+v", training, ps)
	}
}

// Each offered language is laid out too, translated; the file is tagged with its code.
func TestCheckLayoutLaysOutEveryOfferedLanguage(t *testing.T) {
	dir := fixture(t)
	game := string(read(t, filepath.Join(dir, "game.yaml")))
	game = strings.Replace(game, "schema_version: 1", "schema_version: 4", 1)
	write(t, filepath.Join(dir, "game.yaml"), strings.Replace(game, "id: cli-expedition", "id: cli-expedition\nlanguage: en\nwritten_in: en\nlanguages: [en, ja]", 1))
	addWorld(t, dir, "talk.yaml", "events:\n  chat:\n    do:\n      - say: Short.\n      - say:\n          text: Another short one.\n")
	if ps := checkJSONOut(t, 0, "--only", "layout", dir); len(ps) != 0 {
		t.Fatalf("short lines fit in every language: %+v", ps)
	}
	write(t, filepath.Join(dir, "lang", "ja.yaml"), "text:\n  \"Short.\": \""+strings.Repeat("とても長い文章がずっと続きます。", 30)+"\"\n  \"Another short one.\": \"短い。\"\n")
	ps := checkJSONOut(t, 0, "--only", "layout", dir)
	hit := 0
	for _, p := range ps {
		if p.Code == diag.TextOverflow.String() {
			hit++
			if p.File != "data/talk.yaml [ja]" || p.Line != 4 {
				t.Errorf("the Japanese line is tagged with its language and its line: %+v", p)
			}
		}
	}
	if hit != 1 {
		t.Errorf("only the translated line overflows, once: %+v", ps)
	}
	_, stderr := invoke(t, 0, "check", "--only", "layout", dir)
	if !strings.Contains(stderr, "data/talk.yaml [ja]") {
		t.Errorf("stderr: %q", stderr)
	}
	// A block-scalar line and a text: line are laid out like any other.
	addWorld(t, dir, "block.yaml", "events:\n  block:\n    do:\n      - say: |\n          "+strings.Repeat(offerWords, 10)+"\n      - say: {text: \""+strings.Repeat(offerWords, 10)+"\"}\n")
	hits := 0
	for _, p := range checkJSONOut(t, 0, "--only", "layout", dir) {
		if p.File == "data/block.yaml" {
			hits++
		}
	}
	if hits != 2 {
		t.Errorf("both long lines are reported: %d", hits)
	}
}
