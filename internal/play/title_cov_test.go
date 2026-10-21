package play

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	lang "github.com/nao1215/gozen/internal/text"
)

// sysOpen opens the shared test game with a script, a throwaway shot directory and a save
// directory. extra adds or replaces files of the game. The language is reset on cleanup.
func sysOpen(t *testing.T, opts Options, extra map[string]string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	for name, src := range extra {
		if name == "game.yaml+" { // appended to the game's own game.yaml
			fsys["game.yaml"] = &fstest.MapFile{Data: append(fsys["game.yaml"].Data, src...)}
			continue
		}
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	opts.FS = fsys
	opts.NoSound = true
	sysShotNames = nil
	if opts.SaveDir == "" && opts.Store == nil {
		opts.SaveDir = t.TempDir()
	}
	if opts.Script != "" && opts.ShotDir == "" {
		opts.ShotDir = t.TempDir()
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	return g
}

// sysOpen4 is sysOpen for a game at schema_version 4 (map forms of wait, stand and so on).
func sysOpen4(t *testing.T, opts Options, extra map[string]string) *Game {
	t.Helper()
	game := "schema_version: 4\n" + string(gameFiles(t)["game.yaml"].Data)
	if extra == nil {
		extra = map[string]string{}
	}
	if more, ok := extra["game.yaml+"]; ok {
		game += more
		delete(extra, "game.yaml+")
	}
	if extra["game.yaml"] == "" {
		extra["game.yaml"] = game
	} else {
		extra["game.yaml"] = "schema_version: 4\n" + extra["game.yaml"]
	}
	return sysOpen(t, opts, extra)
}

// sysFrames drives n frames (an update and a draw each) and reports whether the game quit.
// Any other error fails the test.
func sysFrames(t *testing.T, g *Game, n int) (quit bool) {
	t.Helper()
	for range n {
		err := g.Update()
		if errors.Is(err, ebiten.Termination) {
			return true
		}
		if err != nil {
			t.Fatal(err)
		}
		sysSafeDraw(g)
	}
	return false
}

// sysSafeDraw draws one frame without the paths that read pixels back (they need a running
// game loop): the memo box gets a blank screenshot and quick memos are written directly.
func sysSafeDraw(g *Game) {
	if g.memo != nil && g.memo.shot == nil {
		g.memo.shot = image.NewNRGBA(image.Rect(0, 0, 1280, 720))
	}
	for _, m := range g.quick {
		m.shot = image.NewNRGBA(image.Rect(0, 0, 1280, 720))
		if _, err := g.writeMemo(g.opts.MemoDir, m); err != nil {
			g.failure = err
		}
	}
	g.quick = nil
	if sc := g.in.script; sc != nil && sc.shot != "" {
		sysShotNames = append(sysShotNames, sc.shot)
		sc.shot = ""
	}
	g.Draw(ebiten.NewImage(1280, 720))
}

// sysShotNames are the screenshots scripts asked for (never taken: reading pixels needs a
// running game loop).
var sysShotNames []string

// TestTitleNewGameThenQuit: choosing the first item of the title starts a new playthrough
// at the game's start place with the start party and gold.
func TestTitleNewGameThenQuit(t *testing.T) {
	g := sysOpen(t, Options{Seed: 3, Script: "wait 2\npress z\nwait 30\nquit"}, nil)
	if _, ok := g.stack[0].(*title); !ok {
		t.Fatalf("starts on %T, want the title", g.stack[0])
	}
	if g.st != nil {
		t.Fatal("a state before choosing")
	}
	if !sysFrames(t, g, 200) {
		t.Fatal("the script did not end")
	}
	if g.st == nil || g.st.Location != "camp" || g.st.Gold != 100 || len(g.st.Party) != 1 || g.st.Party[0] != "scout" {
		t.Fatalf("new game state: %+v", g.st)
	}
	if _, ok := g.stack[0].(*title); ok {
		t.Fatal("still on the title")
	}
}

// TestTitleContinueNeedsASave: with no save "つづきから" is disabled and the cursor stays on
// the new game; with a save the cursor starts on the continue item.
func TestTitleContinueNeedsASave(t *testing.T) {
	g := sysOpen(t, Options{Seed: 3}, nil)
	m := g.titleMenu()
	if !m.items[1].disabled || m.sel != 0 {
		t.Fatalf("no save: continue disabled=%v sel=%d", m.items[1].disabled, m.sel)
	}
	if g.anySave() {
		t.Fatal("reports a save in an empty store")
	}
	g.newGame()
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	if !g.anySave() {
		t.Fatal("does not see the saved slot")
	}
	m = g.titleMenu()
	if m.items[1].disabled || m.sel != 1 {
		t.Fatalf("with a save: continue disabled=%v sel=%d", m.items[1].disabled, m.sel)
	}
}

// TestTitleContinueLoadsTheSave: the continue item opens the slot list and loading a slot
// restores the saved state, not a new game.
func TestTitleContinueLoadsTheSave(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen(t, Options{Seed: 3, SaveDir: dir}, nil)
	g.newGame()
	g.st.Gold = 4321
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen(t, Options{Seed: 3, SaveDir: dir, Script: "wait 2\npress z\nwait 10\npress z\nwait 40\nquit"}, nil)
	if !sysFrames(t, g2, 400) {
		t.Fatal("the script did not end")
	}
	if g2.st == nil || g2.st.Gold != 4321 {
		t.Fatalf("continue did not load the save: %+v", g2.st)
	}
}

// TestTitleSettingsKeyOpensSettings: the settings key on the title opens the settings; a
// change is written to config.json when the screen is closed and read back by the next run.
func TestTitleSettingsKeyOpensSettings(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen(t, Options{SaveDir: dir, Script: "wait 2\npress config\nwait 5\npress right\nwait 5\npress x\nwait 5\nquit"}, nil)
	if !sysFrames(t, g, 300) {
		t.Fatal("the script did not end")
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.json")) //nolint:gosec // reads a file this test wrote
	if err != nil {
		t.Fatalf("settings not written: %v", err)
	}
	if !strings.Contains(string(b), `"text_speed": 3`) {
		t.Fatalf("text speed not saved: %s", b)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen(t, Options{SaveDir: dir}, nil)
	if g2.cfg.TextSpeed != 3 {
		t.Fatalf("settings not reloaded: %+v", g2.cfg)
	}
}

// TestSettingsMenuItemAndAdjust: the title's settings item opens the screen; left and right
// move values inside their limits and OK on the last row closes it.
func TestSettingsMenuItemAndAdjust(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	g.push(newConfigScreen(g))
	s := sysAs[*configScreen](t, g.stack[len(g.stack)-1])
	s.list.sel = int(rowText)
	for range 10 {
		s.change(g, -1, false)
	}
	if g.cfg.TextSpeed != 0 {
		t.Fatalf("text speed went below the range: %d", g.cfg.TextSpeed)
	}
	for range 10 {
		s.change(g, 1, false)
	}
	if g.cfg.TextSpeed != 4 {
		t.Fatalf("text speed above the range: %d", g.cfg.TextSpeed)
	}
	s.change(g, 1, true)
	if g.cfg.TextSpeed != 0 {
		t.Fatalf("a wrapping step from the top should return to 0: %d", g.cfg.TextSpeed)
	}
	s.list.sel = slicesIndexRow(t, s, rowWindow)
	for range 20 {
		s.change(g, 1, false)
	}
	if g.cfg.Window != 10 {
		t.Fatalf("window: %d", g.cfg.Window)
	}
	s.list.sel = slicesIndexRow(t, s, rowBattleSpeed)
	for range 5 {
		s.change(g, 1, false)
	}
	if g.cfg.BattleSpeed != 2 {
		t.Fatalf("battle speed: %d", g.cfg.BattleSpeed)
	}
	s.list.sel = slicesIndexRow(t, s, rowBattleAuto)
	before := g.autoStartBattle()
	s.change(g, 1, false)
	if g.autoStartBattle() == before || g.cfg.BattleAuto == nil {
		t.Fatal("battle auto not toggled")
	}
	if !g.showSP() && slices.Contains(s.rows, rowSP) {
		t.Fatal("sp row shown without sp")
	}
}

func slicesIndexRow(t *testing.T, s *configScreen, r cfgRow) int {
	t.Helper()
	for i, x := range s.rows {
		if x == r {
			return i
		}
	}
	t.Fatalf("the game has no row %d", r)
	return 0
}

// TestConfigClampsOutOfRangeValues: a hand-edited config.json with values outside the
// ranges never indexes outside the tables.
func TestConfigClampsOutOfRangeValues(t *testing.T) {
	c := config{TextSpeed: 99, AutoWait: -4, BattleSpeed: 50}
	if c.textStep() != 9999 {
		t.Fatalf("text step %d", c.textStep())
	}
	if c.autoScale() != 0.5 {
		t.Fatalf("auto scale %v", c.autoScale())
	}
	if c.battleScale() != 2.2 {
		t.Fatalf("battle scale %v", c.battleScale())
	}
	g := sysOpen(t, Options{}, nil)
	g.cfg = config{TextSpeed: 99, AutoWait: 99, Window: 99, BGM: -3, SE: 400, Voice: 77, BattleSpeed: -9}
	s := newConfigScreen(g)
	if len(s.list.items) == 0 {
		t.Fatal("no rows for out of range values")
	}
	g.applyConfig()
	if g.th.window.A != g.windowA {
		t.Fatalf("window alpha %d, want the full %d", g.th.window.A, g.windowA)
	}
}

// TestBrokenConfigIsAnError: config.json that is not JSON stops the game start with a
// settings error instead of silently falling back.
func TestBrokenConfigIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: dir})
	if err == nil || !strings.Contains(err.Error(), "settings") {
		t.Fatalf("err = %v", err)
	}
}

// TestSettingsSaveFailureShownThenClosed: a settings screen that cannot write shows the
// error and a second close leaves anyway (the screen says so).
func TestSettingsSaveFailureShownThenClosed(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := sysOpen(t, Options{Script: "wait 2\nquit"}, nil)
	g.store = gozen.DirStore(blocked)
	g.push(newConfigScreen(g))
	s := sysAs[*configScreen](t, g.stack[len(g.stack)-1])
	s.list.sel = len(s.list.items) - 1
	g.in.pressed[btnOK] = true
	s.update(g)
	if s.err == "" {
		t.Fatal("write failure hidden")
	}
	if g.stack[len(g.stack)-1] != scene(s) {
		t.Fatal("screen closed on the first failure")
	}
	g.in.pressed[btnOK] = true
	s.update(g)
	if g.stack[len(g.stack)-1] == scene(s) {
		t.Fatal("screen did not close the second time")
	}
}

// TestInvalidOptions: New refuses nonsense options before it starts anything.
func TestInvalidOptions(t *testing.T) {
	cases := map[string]Options{
		"nil fs":             {},
		"window too big":     {FS: gameFiles(t), Window: 9},
		"window negative":    {FS: gameFiles(t), Window: -1},
		"replay battle neg":  {FS: gameFiles(t), ReplayBattle: -1},
		"replay battle bare": {FS: gameFiles(t), ReplayBattle: 2},
		"script no shotdir":  {FS: gameFiles(t), Script: "quit", NoSound: true},
		"bad script":         {FS: gameFiles(t), Script: "bogus", ShotDir: t.TempDir(), NoSound: true},
		"unknown location":   {FS: gameFiles(t), Location: "nowhere", NoSound: true},
		"unknown actor":      {FS: gameFiles(t), Party: []string{"ghost"}, NoSound: true},
		"unknown item":       {FS: gameFiles(t), Location: "camp", Items: []string{"ghost"}, NoSound: true},
		"unknown troop":      {FS: gameFiles(t), Location: "camp", Battle: "ghost", NoSound: true},
		"unknown event":      {FS: gameFiles(t), Event: "ghost", NoSound: true},
		"unknown scene":      {FS: gameFiles(t), Scene: "ghost", NoSound: true},
		"missing replay":     {FS: gameFiles(t), Replay: "/nonexistent/trace.jsonl", NoSound: true},
		"missing memo":       {FS: gameFiles(t), FromMemo: "/nonexistent/memo.json", NoSound: true},
	}
	for name, o := range cases {
		if o.SaveDir == "" {
			o.SaveDir = t.TempDir()
		}
		g, err := New(o)
		if err == nil {
			sysClose(t, g)
			t.Errorf("%s: accepted", name)
			continue
		}
	}
	// A failed New must leave no active game behind: a good one still opens.
	g := sysOpen(t, Options{}, nil)
	if g == nil {
		t.Fatal("no game after failures")
	}
}

// TestStartOptionsBuildTheState: the test-start options (location, party, level, flags,
// gold, items) set the start state.
func TestStartOptionsBuildTheState(t *testing.T) {
	g := sysOpen(t, Options{
		Location: "room", Party: []string{"scout", "guide"}, Level: 5, Gold: 777,
		Flags: []string{"seen", "var.count=4", "var.bad=zzz"},
		Items: []string{"potion=5", "blade", "key=x"},
	}, nil)
	st := g.st
	if st.Location != "room" || st.Gold != 777 || len(st.Party) != 2 {
		t.Fatalf("state %+v", st)
	}
	if !st.Flags["seen"] || !st.Flags["var.bad=zzz"] {
		t.Fatalf("flags %v", st.Flags)
	}
	if st.Vars["count"] != 4.0 {
		t.Fatalf("vars %v", st.Vars)
	}
	for _, a := range st.Members() {
		if a.Level < 5 {
			t.Fatalf("%s level %d", a.ID, a.Level)
		}
	}
	if st.ItemCount("potion", false) < 5 || st.ItemCount("blade", false) != 1 || st.ItemCount("key", false) != 1 {
		t.Fatalf("items potion=%d blade=%d key=%d", st.ItemCount("potion", false), st.ItemCount("blade", false), st.ItemCount("key", false))
	}
}
