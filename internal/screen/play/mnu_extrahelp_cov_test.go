package play

import (
	"strings"
	"testing"
	"testing/fstest"
)

const mnuExtraData = `
classes:
  mage:
    name: Mage
    sp: 1
    start_sp: 2
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "5", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    tree:
      - {id: spark, skill: fire, max: 2, col: 0, row: 0}
      - {id: glow, name: Glow, desc: A steady glow., stats: {atk: 1}, max: 2, col: 1, row: 0, requires: [{node: spark}]}
      - {id: mend, skill: heal, col: 1, row: 1, requires: [{node: spark, level: 2}]}
      - {id: far, name: Far, stats: {def: 1}, max: 1, col: 3, row: 0, requires: [{node: glow, level: 2}]}
      - {id: back, name: Back, stats: {def: 1}, max: 1, col: 3, row: 1, requires: [{node: glow, level: 1}, {node: mend}]}
actors:
  sage: {name: Sage, class: mage}
  third: {name: Third, class: ranger}
  fourth: {name: Fourth, class: ranger}
  fifth: {name: Fifth, class: ranger}
items:
  shard: {name: Shard, price: 1}
  gem: {name: Gem, price: 1}
  crystal: {name: Crystal, price: 1}
fusions:
  - {from: [shard, shard], to: gem, price: 30}
  - {from: [gem, shard], to: crystal, price: 5}
`

// mnuExtraGame is the camp game with a mage's skill tree, a fusion table and extra actors.
func mnuExtraGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/extra.yaml"] = &fstest.MapFile{Data: []byte(mnuExtraData)}
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "battle:\n", "battle:\n  passives: [{actor: sage, state: poison, when: \"flag.awake\", level: \"2\"}, {actor: sage, state: sleep}]\n", 1))
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Location: "camp", Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}
