package trial

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const pic = "synthetic picture sentinel"

func TestForbiddenOutputAndUnresolvedBranches(t *testing.T) {
	src := fstest.MapFS{
		"assets/NOTICE.txt": {Data: []byte("Original neutral test fixture.")},
		"game.yaml":         {Data: []byte("schema_version: 8\nid: narrative-trial\nstart: {location: station, event: opening}\n")},
		"data/story.yaml":   {Data: []byte("locations: {station: {name: Station}}\nevents:\n  opening:\n    do:\n      - if: chance(0.5)\n        then: [{call: ending}]\n        else: [{say: Welcome}]\n  ending: {do: [{say: Later chapter}]}\n")},
	}
	res, err := Build(src, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unresolved) == 0 || !slices.Contains(res.Kept["events"], "ending") {
		t.Fatal("unresolved branch was not reported and retained")
	}
	if _, err := Build(src, &Config{Forbid: []string{"events.ending"}}); err == nil || !strings.Contains(err.Error(), "forbidden content remains: events.ending") {
		t.Fatalf("forbidden ending: %v", err)
	}
	res, err = Build(src, &Config{Cut: []string{"ending"}, Forbid: []string{"events.ending"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Check(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Files["data/story.yaml"]), "Later chapter") {
		t.Fatal("cut content survived")
	}
}

func TestNarrativeDependenciesAndAliasCut(t *testing.T) {
	src := fstest.MapFS{
		"game.yaml":         {Data: []byte("schema_version: 8\nid: trial-story\nstart: {location: station, event: opening}\nverbs: {greet: greeting}\nreactions: {rowan: rowan.yaml}\nscreens: {journal: {title: Journal, entries: [{label: Note, text: Welcome}]}}\n")},
		"data/story.yaml":   {Data: []byte("actors: {rowan: {name: Rowan}}\nlocations: {station: {name: Station}}\nevents:\n  opening: {do: [{greet: true}, {open: journal}]}\n  greeting: {do: [{respond: {who: rowan, text: Hello}}]}\n")},
		"assets/rowan.yaml": {Data: []byte("people: [{id: rowan, rules: [{id: hello, say: [Welcome]}]}]\n")},
	}
	r, err := Build(src, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.Assets, "rowan.yaml") {
		t.Fatal("reaction book dropped")
	}
	if _, err := Build(src, &Config{Forbid: []string{"assets/rowan.yaml"}}); err == nil {
		t.Fatal("forbidden book retained")
	}
	r, err = Build(src, &Config{Cut: []string{"greeting"}, Forbid: []string{"events.greeting"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(r.Files["game.yaml"]), "verbs:") {
		t.Fatal("expanded aliases left behind")
	}
}

// trialGame is a small campaign with a gallery, asides, songs, a cut ending and a dungeon
// whose stairs go on without end.
func trialGame(extra map[string]string) fstest.MapFS {
	m := fstest.MapFS{
		"game.yaml": {Data: []byte(`schema_version: 1
id: trial-sample
title: Full sample
start: {location: camp, party: [scout]}
battle: {variance: 0, critical: 0, escape: 1}
text: {a: one, b: two}
cg_review: {tags: [{id: ok, label: OK}]}
scene_review: {}
gallery:
  cgs:
    - {title: Kept, file: cg/kept.jpg}
    - {title: Dropped, file: cg/dropped.jpg}
    - {title: Stages, files: [cg/stage.*.jpg, cg/absent.jpg]}
  scenes:
    - {title: Opening, event: opening, location: camp, flag: saw_opening}
    - {title: Ending, event: ending, flag: saw_ending}
    - {title: Locked, event: locked, flag: saw_locked}
    - {title: Gone, event: nowhere, flag: saw_gone}
    - {title: Seen, event: seen, flag: saw_seen, seen: cg/kept.jpg}
    - {title: Unseen, event: seen, flag: saw_unseen, seen: cg/dropped.jpg}
  music: theme
  music_pictures:
    theme: [cg/kept.jpg, cg/dropped.jpg]
    lost: [cg/kept.jpg]
asides:
  - {actor: scout, call: chat}
  - {name: Guide, call: haunt}
  - {name: Ghost, call: ghostly}
  - {face: faces/ghost*, call: faces_call}
  - {name: Never, when: "flag.never", call: hidden}
`)},
		"data/world.yaml": {Data: []byte(`classes:
  walker: {name: Walker, stats: {mhp: 30, mmp: 5, atk: 6, def: 2, mat: 1, mdf: 1, agi: 5, luk: 1}, exp_for: level * 10}
actors: {scout: {name: Scout, class: walker}}
skills:
  attack: {name: Tap, effects: [{damage: 6, variance: 0}]}
  guard: {name: Wait, scope: self}
  flourish: {name: Flourish, scope: one_enemy, effects: [{damage: 9}]}
enemies:
  mote: {name: Mote, image: foe.png, stats: {mhp: 6, mmp: 0, atk: 1, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}, ai: [{use: attack}]}
  giant: {name: Giant, image: foe.png, stats: {mhp: 60, mmp: 0, atk: 5, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}, ai: [{use: flourish}]}
troops:
  small: {members: [mote]}
  large: {members: [giant]}
locations:
  camp:
    name: Camp
    people:
      - name: Guide
        talk:
          - say: {text: Welcome, who: Guide}
          - call: opening
          - if: "flag.locked_door"
            then: [{call: locked}]
            else: [{say: The door is shut.}]
    actions:
      - {label: Descend, do: [{dive: depths, floor: 1}]}
      - {label: Finish, do: [{call: finale}]}
      - {label: Shop, when: "var.gold_seen > 3", do: [{call: seen}]}
    exits: [{to: hall, label: Hall}]
  hall:
    name: Hall
    encounters: {rate: 0.5, troops: [{troop: small}]}
    exits: [{to: camp}]
  depths:
    name: Depths
    dungeon:
      generate:
        width: 11
        height: 11
        start: S
        place: [{char: '>', far: true}, {char: R, count: "var.rare_count"}]
      tiles:
        S: {start: true}
        '>': {do: [{dive: depths, floor: var.floor + 1}]}
        R: {name: Relic, do: [{call: relic}]}
        X: {name: Unused, do: [{call: neverused}]}
  vault:
    name: Vault
    exits: [{to: camp, when: "flag.locked_door"}]
events:
  opening: {do: [{say: The road begins.}, {image: cg/kept.jpg}, {set: {flag.locked_door: false, flag.saw_opening: true}}]}
  ending: {do: [{say: The end.}, {clear: true}]}
  locked: {do: [{say: Open now.}, {image: cg/dropped.jpg}]}
  seen: {do: [{say: Seen it.}]}
  chat: {do: [{say: Hello}]}
  haunt: {do: [{say: Boo}]}
  faces_call: {do: [{say: Face}]}
  hidden: {do: [{say: Never}]}
  ghostly: {do: [{say: Whoo}]}
  relic: {do: [{say: A relic.}, {image: cg/stage.1.jpg}]}
  neverused: {do: [{say: nope}]}
  finale: {do: [{say: Final scene.}, {call: ending}]}
  hold: {trigger: enter, location: hall, once: true, when: "flag.hall_once", do: [{say: Echo}]}
  greet: {trigger: enter, location: hall, once: true, do: [{say: Welcome to the hall.}, {set: {flag.saw_seen: true, flag.saw_unseen: true}}]}
`)},
		"data/music.yaml": {Data: []byte(`music:
  songs:
    theme: {dir: bgm/theme, bpm: 100, moods: {calm: calm, tense: tense}}
    battle: {file: bgm/battle.ogg}
    unused: {file: bgm/unused.ogg}
`)},
		"assets/foe.png":             {Data: []byte(pic)},
		"assets/cg/kept.jpg":         {Data: []byte(pic)},
		"assets/cg/dropped.jpg":      {Data: []byte(pic)},
		"assets/cg/stage.1.jpg":      {Data: []byte(pic)},
		"assets/cg/stage.2.jpg":      {Data: []byte(pic)},
		"assets/bgm/theme/calm.ogg":  {Data: []byte("ogg")},
		"assets/bgm/theme/tense.ogg": {Data: []byte("ogg")},
		"assets/bgm/battle.ogg":      {Data: []byte("ogg")},
		"assets/bgm/unused.ogg":      {Data: []byte("ogg")},
		"assets/sfx/step.wav":        {Data: []byte("wav")},
		"assets/notes.txt":           {Data: []byte("# keep\nhelp.png\ncg/kept.jpg\nplain line\n")},
		"assets/spare.png":           {Data: []byte(pic)},
	}
	for k, v := range extra {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func build(t *testing.T, config string, extra map[string]string) *Result {
	t.Helper()
	cfg, err := Parse([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(trialGame(extra), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func kept(r *Result, sec string) []string { return r.Kept[sec] }

func TestBuildKeepsOnlyWhatTheRangeReaches(t *testing.T) {
	res := build(t, `
assume: {flag.locked_door: false, flag.hall_once: false, var.rare_count: 0, var.gold_seen: 0}
cut: [finale, ending]
dive_max: {depths: 3}
blocked: [{say: This is as far as the trial goes.}]
`, nil)
	if err := res.Check(); err != nil {
		t.Fatalf("the trial copy must load: %v", err)
	}
	// An event is kept only when something in range reaches it.
	for _, id := range []string{"opening", "chat", "haunt", "greet"} {
		if !slices.Contains(kept(res, "events"), id) {
			t.Errorf("event %s was dropped (kept %v)", id, kept(res, "events"))
		}
	}
	for _, id := range []string{"finale", "ending", "neverused", "hidden", "hold", "relic", "locked", "faces_call", "ghostly"} {
		if slices.Contains(kept(res, "events"), id) {
			t.Errorf("event %s must not be kept", id)
		}
		if !slices.Contains(res.Removed["events"], id) {
			t.Errorf("event %s must be listed as removed: %v", id, res.Removed["events"])
		}
	}
	if !slices.Contains(res.Removed["locations"], "vault") || !slices.Contains(kept(res, "locations"), "depths") {
		t.Errorf("locations kept %v removed %v", kept(res, "locations"), res.Removed["locations"])
	}
	if slices.Contains(kept(res, "troops"), "large") || !slices.Contains(kept(res, "troops"), "small") {
		t.Errorf("troops %v", kept(res, "troops"))
	}
	if slices.Contains(kept(res, "skills"), "flourish") || !slices.Contains(kept(res, "skills"), "attack") || !slices.Contains(kept(res, "skills"), "guard") {
		t.Errorf("skills %v: the engine's own skills stay, an unused one goes", kept(res, "skills"))
	}
	if !slices.Contains(kept(res, "songs"), "theme") || !slices.Contains(kept(res, "songs"), "battle") || slices.Contains(kept(res, "songs"), "unused") {
		t.Errorf("songs %v", kept(res, "songs"))
	}
	if res.Gates < 3 {
		t.Errorf("the cut calls and the dive limit are gates (%d)", res.Gates)
	}
	if res.Passes < 1 {
		t.Errorf("passes %d", res.Passes)
	}
	files := res.FS()
	game := string(files["game.yaml"].Data)
	for _, gone := range []string{"cg_review", "scene_review"} {
		if strings.Contains(game, gone) {
			t.Errorf("%s must not reach a trial copy", gone)
		}
	}
	if !strings.Contains(string(files["data/world.yaml"].Data), "This is as far as the trial goes.") {
		t.Error("the blocked commands replace a cut call")
	}
	world := string(files["data/world.yaml"].Data)
	if !strings.Contains(world, "(var.floor + 1) > 3") {
		t.Errorf("a dive below the limit is guarded by an if:\n%s", world)
	}
	if strings.Contains(world, "neverused") || strings.Contains(world, "Unused") {
		t.Errorf("an unused tile stays in the copy:\n%s", world)
	}
	if strings.Contains(world, "var.gold_seen") {
		t.Errorf("an action whose condition cannot hold is dropped:\n%s", world)
	}
	if strings.Contains(world, "The door is shut.") == false || strings.Contains(world, "call: locked") {
		t.Errorf("an if that cannot be true keeps only its else:\n%s", world)
	}
	// Assets.
	for _, a := range []string{"foe.png", "cg/kept.jpg", "bgm/theme/calm.ogg", "bgm/theme/tense.ogg", "bgm/battle.ogg", "sfx/step.wav"} {
		if !slices.Contains(res.Assets, a) {
			t.Errorf("asset %s dropped (kept %v)", a, res.Assets)
		}
	}
	for _, a := range []string{"cg/dropped.jpg", "bgm/unused.ogg", "spare.png", "cg/stage.1.jpg"} {
		if slices.Contains(res.Assets, a) {
			t.Errorf("asset %s must not be kept", a)
		}
	}
}

func TestBuildGalleryAndAsides(t *testing.T) {
	res := build(t, `assume: {flag.locked_door: false, var.rare_count: 0, var.gold_seen: 0}
cut: [ending, finale]
keep: ["cg/stage.*.jpg"]
`, nil)
	game := string(res.FS()["game.yaml"].Data)
	for _, want := range []string{"title: Kept", "title: Opening", "title: Seen"} {
		if !strings.Contains(game, want) {
			t.Errorf("gallery lacks %q:\n%s", want, game)
		}
	}
	for _, gone := range []string{"title: Dropped", "title: Gone", "title: Unseen", "name: Never", "call: hidden"} {
		if strings.Contains(game, gone) {
			t.Errorf("gallery still has %q:\n%s", gone, game)
		}
	}
	if !strings.Contains(game, "call: chat") || !strings.Contains(game, "call: haunt") {
		t.Errorf("asides of a kept speaker are kept:\n%s", game)
	}
	if res.Gallery.CGs != 2 || res.Gallery.Scenes < 2 {
		t.Errorf("gallery counts: %+v", res.Gallery)
	}
	if strings.Contains(game, "lost:") || !strings.Contains(game, "theme:") {
		t.Errorf("music_pictures must follow the songs that stay:\n%s", game)
	}
	if !slices.Contains(res.Assets, "cg/stage.1.jpg") || !slices.Contains(res.Assets, "cg/stage.2.jpg") {
		t.Errorf("keep patterns not honoured: %v", res.Assets)
	}
}

func TestBuildListsMissingAssetsAndRefusesToWrite(t *testing.T) {
	res := build(t, `assume: {flag.locked_door: false}
cut: [ending, finale]
keep: ["missing/one.png"]
`, nil)
	if !slices.Equal(res.Missing, []string{"missing/one.png"}) {
		t.Fatalf("missing: %v", res.Missing)
	}
	err := res.Write("unused", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "missing assets") {
		t.Fatalf("a copy that lacks an asset must not be written: %v", err)
	}
}

func TestResultWriteProducesALoadableCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.CopyFS(src, trialGame(nil)); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]byte("assume: {flag.locked_door: false, var.rare_count: 0, var.gold_seen: 0}\ncut: [ending, finale]\nkeep: [notes.txt]\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(os.DirFS(src), cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := res.Write(src, out); err != nil {
		t.Fatal(err)
	}
	if _, err := data.LoadDir(os.DirFS(out), "."); err != nil {
		t.Fatalf("the written trial copy does not load: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "assets", "notes.txt")) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	// A text list drops the lines that name a picture the copy lacks, and keeps the rest.
	if got := string(b); strings.Contains(got, "help.png") || !strings.Contains(got, "cg/kept.jpg") || !strings.Contains(got, "plain line") || !strings.Contains(got, "# keep") {
		t.Errorf("notes.txt: %q", got)
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "spare.png")); err == nil {
		t.Error("an unreferenced asset was copied")
	}
	// A path that leaves the output is refused.
	bad := &Result{Files: map[string][]byte{"../escape.yaml": nil}}
	if err := bad.Write(src, filepath.Join(dir, "bad")); err == nil || !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("escaping file: %v", err)
	}
	bad = &Result{Assets: []string{"../escape.png"}}
	if err := bad.Write(src, filepath.Join(dir, "bad2")); err == nil || !strings.Contains(err.Error(), "invalid asset path") {
		t.Errorf("escaping asset: %v", err)
	}
	bad = &Result{Assets: []string{"absent.png"}, src: os.DirFS(src)}
	if err := bad.Write(src, filepath.Join(dir, "bad3")); err == nil {
		t.Error("an asset that does not exist in the source must fail the write")
	}
}

func TestGameOverridesMergeTextAndReplaceOtherKeys(t *testing.T) {
	res := build(t, `assume: {flag.locked_door: false}
cut: [ending, finale]
game:
  title: Trial edition
  text: {b: changed, c: added}
`, nil)
	game := string(res.FS()["game.yaml"].Data)
	if !strings.Contains(game, "title: Trial edition") || strings.Contains(game, "Full sample") {
		t.Errorf("title not replaced:\n%s", game)
	}
	for _, want := range []string{"a: one", "b: changed", "c: added"} {
		if !strings.Contains(game, want) {
			t.Errorf("text override lacks %q:\n%s", want, game)
		}
	}
}

func TestConfigParseRejectsBadSettings(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"unknown key", "surprise: 1\n", "surprise"},
		{"not yaml", "assume: [", "trial:"},
		{"assume name", "assume: {gold: 3}\n", "var.名前 か flag.名前"},
		{"range length", "assume: {var.x: [1, 2, 3]}\n", "2 つ"},
		{"range type", "assume: {var.x: [1, a]}\n", "数で書く"},
		{"assume map", "assume: {var.x: {a: 1}}\n", "前提に書けない"},
		{"dive_max without blocked", "dive_max: {depths: 3}\n", "blocked"},
		{"bad pattern", "cut: ['[']\n", "trial pattern"},
		{"bad unknown pattern", "unknown: ['[']\n", "trial pattern"},
		{"unsupported forbidden section", "forbid: [actors.rowan]\n", "removable definition section"},
		{"bad forbidden pattern", "forbid: ['assets/[']\n", "trial pattern"},
		{"bad keep pattern", "keep: ['[']\n", "trial pattern"},
		{"blocked entry is no command", "blocked: [0]\n", "blocked: each entry is a command"},
		{"blocked entry is a list", "blocked: [[say]]\n", "blocked: each entry is a command"},
	} {
		_, err := Parse([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v (want %q)", tc.name, err, tc.want)
		}
	}
	for _, ok := range []string{
		"assume: {var.x: 3, var.y: [0, inf], var.z: ['-inf', 5], flag.a: true, flag.b: false, var.s: text, var.n: null}\n",
		"assume: {var.x: 2.5, var.y: 7}\n",
		"{}",
	} {
		if _, err := Parse([]byte(ok)); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}

func TestLoadReadsConfigFromTheFS(t *testing.T) {
	fsys := fstest.MapFS{"trial.yaml": {Data: []byte("cut: [x]\n")}}
	c, err := Load(fsys, "trial.yaml")
	if err != nil || !slices.Equal(c.Cut, []string{"x"}) {
		t.Fatalf("%v %v", c, err)
	}
	if _, err := Load(fsys, "absent.yaml"); err == nil {
		t.Fatal("absent config must fail")
	}
}

func TestBuildFailsWithoutAGameOrWithBrokenYAML(t *testing.T) {
	cfg, err := Parse([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(fstest.MapFS{}, cfg); err == nil {
		t.Error("a directory without game.yaml must fail")
	}
	broken := trialGame(map[string]string{"data/broken.yaml": "a: [1, 2\n"})
	if _, err := Build(broken, cfg); err == nil || !strings.Contains(err.Error(), "data/broken.yaml") {
		t.Errorf("broken data file: %v", err)
	}
}
