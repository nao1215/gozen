package play

import "testing"

func TestDungeonSnapSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := wldWorld(t, "fx_maze", Options{SaveDir: dir})
	if g.cfg.DungeonSnap {
		t.Fatal("existing settings must retain animated transitions")
	}
	s := newConfigScreen(g)
	s.list.sel = slicesIndexRow(t, s, rowDungeonSnap)
	s.change(g, 1, false)
	if !g.cfg.DungeonSnap {
		t.Fatal("settings did not enable instant transitions")
	}
	if err := g.saveConfig(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	restored := wldWorld(t, "fx_maze", Options{SaveDir: dir})
	if !restored.cfg.DungeonSnap {
		t.Fatal("instant transitions did not survive restarting the game")
	}
}
