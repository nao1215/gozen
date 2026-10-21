package play

import (
	"bytes"
	"os"
	"testing"

	"github.com/nao1215/gozen/internal/layer5/sound"
)

// TestTitleMusicPlaysOnTheTitle: the song game.yaml names for the title plays when the title opens
// and is the song the game falls back to.
func TestTitleMusicPlaysOnTheTitle(t *testing.T) {
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	g := sysOpen4(t, Options{Seed: 1, Trace: &bytes.Buffer{}}, map[string]string{
		"data/music.yaml": sysMusicGame, "assets/town.ogg": string(tone), "assets/fight.ogg": string(tone), "assets/hush.ogg": string(tone),
		"game.yaml+": "title_music: {song: hush}\n",
	})
	sysPlaying(t, g, "hush")
	if g.baseSong != "hush" || g.baseMood != sound.Neutral {
		t.Fatalf("base song %q mood %q", g.baseSong, g.baseMood)
	}
}

// TestTitleLetteringFollowsTheGame: the title and subtitle are lettered with the sizes and
// spacing the game gives; a title with figures that give no height or bottom still draws them.
func TestTitleLetteringFollowsTheGame(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1}, map[string]string{
		"game.yaml+": "subtitle: Part one\ntitle_text: {size: 70, tracking: 0.2, subtitle_size: 24, subtitle_tracking: 0.1}\ntitle_figures: [{file: x.png, x: 0.3}, {file: x.png, x: 0.6, bottom: 1, height: 0.7}]\n",
	})
	sysFrames(t, g, 60)
	if g.failure != nil {
		t.Fatal(g.failure)
	}
	if g.pics.imgs["x.png"] == nil {
		t.Fatal("the figures were not loaded")
	}
	if g.titleCentered() {
		t.Fatal("a title with figures is not centred")
	}
}

// TestLegacyTitleFiguresStandOnTheBottom: legacy games put figures at the bottom of the screen when they give no bottom.
func TestLegacyTitleFiguresStandOnTheBottom(t *testing.T) {
	g2 := sysOpen(t, Options{}, map[string]string{"game.yaml+": "title_figures: [{file: x.png, x: 0.3}]\n"})
	sysFrames(t, g2, 5)
	if g2.pics.imgs["x.png"] == nil {
		t.Fatal("the legacy figure was not loaded")
	}
}
