package play

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// btlScreen is the target every test frame is drawn to. Draw only queues commands, so no
// pixel is ever read back.
var btlScreen *ebiten.Image

func btlImage() *ebiten.Image {
	if btlScreen == nil {
		btlScreen = ebiten.NewImage(1280, 720)
	}
	return btlScreen
}

// btlData is extra battle content laid beside the shared test game: a guardian with a
// title, big moves with a cut-in, elements, states and a second party member that heals.
const btlData = `
skills:
  nova: {name: Nova, mp: 0, scope: all_enemies, cutin: true, anim: magic, fx: fire, effects: [{damage: "30", element: fire, variance: 0}]}
  smite: {name: Smite, scope: one_enemy, anim: slash, effects: [{damage: "40", variance: 0}]}
  frost: {name: Frost, mp: 2, scope: one_enemy, anim: magic, fx: ice, effects: [{damage: "10", element: ice, variance: 0}]}
  bolt: {name: Bolt, mp: 2, scope: one_enemy, anim: magic, fx: thunder, effects: [{damage: "10", variance: 0}]}
  weaken: {name: Weaken, scope: one_enemy, anim: debuff, effects: [{add_state: poison}]}
  bless: {name: Bless, scope: one_ally, anim: buff, effects: [{add_state: ward}]}
  cure: {name: Cure, scope: one_ally, where: [battle], anim: heal, effects: [{heal: "30"}]}
  drain: {name: Drain, scope: one_enemy, effects: [{damage: "10", stat: mp, variance: 0}]}
  hurt_all: {name: Quake, scope: all_enemies, effects: [{damage: "10", variance: 0}]}
  hit_party: {name: Roar, scope: all_allies, effects: [{damage: "6", variance: 0}]}
  hit_one: {name: Claw, scope: one_enemy, anim: slash, effects: [{damage: "12", variance: 0}]}
  raise_all: {name: Dawn, scope: dead_ally, effects: [{revive: 0.5}]}
  whiff: {name: Whiff, scope: one_enemy, hit: "0", effects: [{damage: "5"}]}
  call_mote: {name: Call, scope: self, effects: [{summon: mote}]}
  lunge: {name: Lunge, scope: one_enemy, anim: slash, effects: [{damage: "6", variance: 0}]}
  holy_light: {name: Light, scope: one_enemy, fx: holy, effects: [{damage: "5", variance: 0}]}
  shadow: {name: Shadow, scope: one_enemy, fx: dark, effects: [{damage: "5", variance: 0}]}
  pierce_all: {name: Pierce, scope: one_enemy, fx: pierce, effects: [{damage: "5", variance: 0}]}
  thump: {name: Thump, scope: one_enemy, fx: strike, effects: [{damage: "5", variance: 0}]}
  ember: {name: Ember, mp: 0, scope: one_enemy, anim: magic, fx: fire, desc: "A small flame that burns what it touches and then some more words to make the line long enough to wrap in the description box.", effects: [{damage: "9", element: fire, variance: 0}]}
  slash_fire: {name: Fire edge, scope: one_enemy, anim: slash, fx: slash, effects: [{damage: "9", element: fire, variance: 0}]}
  chill: {name: Chill, scope: one_enemy, anim: magic, fx: ice, effects: [{damage: "9", element: ice, variance: 0}]}
  dispel: {name: Dispel, scope: one_enemy, effects: [{remove_state: ward}]}
  hallow: {name: Hallow, scope: one_ally, anim: heal, fx: holy, effects: [{heal: "20"}]}
  mend_quiet: {name: Soothe, scope: one_ally, effects: [{heal: "5", quiet: true}]}
  freeze: {name: Freeze, scope: one_enemy, anim: magic, fx: ice, effects: [{add_state: ward}]}
  sting: {name: Sting, scope: one_enemy, anim: slash, effects: [{damage: "3", variance: 0}, {add_state: poison}]}
  quiet_hit: {name: Tap, scope: one_enemy, anim: none, effects: [{damage: "4", variance: 0}]}
  finisher: {name: Finisher, scope: one_enemy, cutin: true, effects: [{damage: "99", variance: 0}]}
states:
  ward: {name: Warded, turns: 3}
  regen: {name: Regen, turns: 3, on_turn_end: [{heal: "3", quiet: true}]}
  shield: {name: Shield, turns: 5, cover: "b.hp_rate < 2", messages: {cover: "{target} covers {user}"}}
actors:
  knight:
    name: Knight
    class: ranger
    faces: f
    face: normal
    cutin: x.png
    cutin_box: [0.5, 0.3, 0.4]
    cutin_lines: ["Hya!", "Take this!"]
    reactions: {act: grin, hit: hurt, heal: grin, win: grin, fall: hurt}
    face_rules: [{when: "self.hp_rate < 0.5", face: hurt}]
  weaver: {name: Weaver, class: hatter}
enemies:
  idol:
    name: Idol
    title: Painted one
    boss: true
    image: tall.png
    faces: {hurt: x.png, angry: x.png}
    face_rules: [{when: "self.hp_rate < 0.5", face: hurt}]
    reactions: {hit: angry}
    stats: {mhp: 80, mmp: 0, atk: 8, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: slap}]
  giant:
    name: Giant
    image: big.png
    resist: {ice: 0.5}
    stats: {mhp: 60, mmp: 0, atk: 8, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: slap}]
  warden:
    name: Warden
    title: Gate keeper
    boss: true
    image: x.png
    stats: {mhp: 60, mmp: 50, atk: 8, def: 2, mat: 1, mdf: 1, agi: 1, luk: 1}
    exp: 40
    gold: 20
    ai:
      - {when: "turn == 1", use: nova}
      - {use: slap}
  hulk:
    name: Hulk
    image: x.png
    scale: 1.5
    stats: {mhp: 25, mmp: 0, atk: 8, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    exp: 5
    gold: 3
    ai: [{use: hit_one}]
  caller:
    name: Caller
    image: x.png
    stats: {mhp: 40, mmp: 0, atk: 8, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai:
      - {when: "turn == 1", use: call_mote, priority: 5}
      - {use: lunge}
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 9000, mmp: 0, atk: 400, def: 0, mat: 1, mdf: 1, agi: 20, luk: 1}
    ai: [{use: hit_one}]
shops:
  kiosk: {name: Kiosk, goods: [potion]}
troops:
  pictured:
    members: [mote]
    can_escape: false
    events:
      - when: "turn == 1"
        do:
          - {image: {files: [x.png, x.png]}}
          - {shop: kiosk}
          - {set: {flag.shown: true}}
  titans: {members: [giant, giant, giant, giant, giant, giant]}
  guardian: {members: [warden]}
  pack: {members: [hulk, mote, hulk]}
  doom: {members: [brute], can_escape: false}
  caller: {members: [caller]}
  tall: {members: [idol, giant, hulk]}
  crowd: {members: [hulk, hulk, hulk, hulk, hulk]}
  chatty:
    members: [mote]
    can_escape: false
    events:
      - when: "turn == 1"
        do:
          - {notice: "Heads up"}
          - say: {who: guide, text: "Guide line", instead: [{who: scout, text: "Scout instead"}]}
          - say: {who: guide, text: "Lost line"}
          - {wait: 0.1}
          - choice:
              - {label: Left, do: [{set: {flag.left: true}}]}
              - {label: Right, do: [{set: {flag.right: true}}]}
`

// btlFiles is the shared test game with the battle content above and extra game.yaml text.
func btlFiles(t *testing.T, gameExtra, dataExtra string, extra ...map[string]string) fstest.MapFS {
	t.Helper()
	fsys := gameFiles(t)
	for name, size := range map[string][2]int{"assets/tall.png": {100, 700}, "assets/big.png": {300, 620}} {
		fsys[name] = &fstest.MapFile{Data: btlPNG(size[0], size[1])}
	}
	for _, face := range []string{"normal", "hurt", "grin"} {
		fsys["assets/f/"+face+".png"] = &fstest.MapFile{Data: btlPNG(32, 32)}
	}
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte(testgame.Game + "\nid: screen-test\nfonts: {regular: test.ttf}\n" + gameExtra)}
	fsys["data/btl.yaml"] = &fstest.MapFile{Data: []byte(btlData + dataExtra)}
	for _, files := range extra {
		for name, src := range files {
			fsys[name] = &fstest.MapFile{Data: []byte(src)}
		}
	}
	return fsys
}

// btlOpen opens the game with the extra content and returns it with its trace buffer.
func btlOpen(t *testing.T, opts Options, gameExtra, dataExtra string, extra ...map[string]string) (*Game, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	opts.FS = btlFiles(t, gameExtra, dataExtra, extra...)
	opts.NoSound = true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.Trace == nil {
		opts.Trace = &out
	}
	if opts.Seed == 0 {
		opts.Seed = 5
	}
	switch {
	case opts.Location == "-": // start at the title
		opts.Location = ""
	case opts.Location == "":
		opts.Location = "field"
		fallthrough
	default:
		if len(opts.Party) == 0 {
			opts.Party = []string{"scout", "guide"}
		}
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g, &out
}

// btlFrame runs one frame: an update and a draw.
func btlFrame(t *testing.T, g *Game) {
	t.Helper()
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	g.Draw(btlImage())
}

// btlKey presses a button for one frame of the top screen (what a player's key does) and
// draws it.
func btlKey(g *Game, bs ...button) {
	for _, b := range bs {
		g.in.pressed[b] = true
	}
	g.tick++
	g.stack[len(g.stack)-1].update(g)
	g.in.pressed = [numButtons]bool{}
	g.Draw(btlImage())
}

// btlUntil runs frames until the condition holds.
func btlUntil(t *testing.T, g *Game, what string, limit int, cond func() bool) {
	t.Helper()
	for range limit {
		if cond() {
			return
		}
		btlFrame(t, g)
	}
	t.Fatalf("never reached: %s", what)
}

// btlBattle is the running battle screen.
func btlBattle(t *testing.T, g *Game) *battleScene {
	t.Helper()
	s := g.currentBattle()
	if s == nil {
		t.Fatal("no battle on the stack")
	}
	return s
}

// btlAsk runs frames until the battle asks for a command.
func btlAsk(t *testing.T, g *Game) *battleScene {
	t.Helper()
	s := btlBattle(t, g)
	btlUntil(t, g, "command phase", 4000, func() bool { return s.phase == bpInput || s.phase == bpResult })
	return s
}

// btlFinish runs frames until the battle screen is gone; presses OK now and then so result
// lines move on.
func btlFinish(t *testing.T, g *Game, s *battleScene) {
	t.Helper()
	for i := range 6000 {
		if g.currentBattle() != s {
			return
		}
		if s.phase == bpResult && i%3 == 0 {
			g.in.pressed[btnOK] = true
			g.tick++
			g.stack[len(g.stack)-1].update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	t.Fatalf("battle did not end, phase %d", s.phase)
}

// btlChoose picks a command from the open menu by moving the cursor to the row.
func btlChoose(g *Game, s *battleScene, row int) {
	for s.cmd.sel < row {
		btlKey(g, btnDown)
	}
	btlKey(g, btnOK)
}

// btlLines are the trace lines of the given kinds.
func btlLines(t *testing.T, out *bytes.Buffer, kinds ...string) []traceLine {
	t.Helper()
	return traceLines(t, out.Bytes(), kinds...)
}

func btlAlive(us []*battle.Unit) int {
	n := 0
	for _, u := range us {
		if u.Alive() {
			n++
		}
	}
	return n
}

// btlPNG is a flat picture of the given size.
func btlPNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(60 + x%90), G: uint8(90 + y%120), B: 150, A: 255}) //nolint:gosec // in 0..255
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

var _ = strings.Contains
