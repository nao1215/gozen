package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// The music and sound words of staging (ADR 0028): music takes a fade and a stop, sfx can wait for
// the sound, a portrait shakes by name, and the screen fades through a color.

const wldMusicData = `music:
  songs:
    calm: {name: Calm, file: bgm/calm.ogg}
    tense: {name: Tense, file: bgm/tense.ogg}
events:
  ev_music:
    do:
      - music: calm
      - music: {song: tense, fade: 1.5}
      - sfx: {name: decide, wait: true}
      - wait: {seconds: 0.2}
      - music: {stop: true, fade: 1}
      - say: "Quiet."
`

func wldMusicGame(t *testing.T) *Game {
	t.Helper()
	extra := map[string]string{
		"data/music_cov.yaml":  wldMusicData,
		"assets/bgm/calm.ogg":  "not decoded without sound",
		"assets/bgm/tense.ogg": "not decoded without sound",
	}
	g := wldOpenWith(t, Options{Location: "camp", FS: wldWorldFiles(t, "", extra)})
	wldFrames(t, g, 10)
	return g
}

func TestWldMusicWordsSetTheSongAsked(t *testing.T) {
	g := wldMusicGame(t)
	g.music(data.MusicCue{Song: "calm"})
	if g.baseSong != "calm" || g.songOverride != "calm" {
		t.Fatalf("song %q override %q", g.baseSong, g.songOverride)
	}
	// a fade crossfades to the song asked for; the song asked for is the new one at once
	g.music(data.MusicCue{Song: "tense", Mood: "fight", Fade: 1.5})
	if g.baseSong != "tense" || g.baseMood != "fight" {
		t.Fatalf("song %q mood %q after a fade", g.baseSong, g.baseMood)
	}
	// stop fades out and leaves no song asked for
	g.music(data.MusicCue{Stop: true, Fade: 1})
	if g.baseSong != "" || g.songOverride != "" || g.moodOverride != "" {
		t.Fatalf("after stop: song %q override %q mood %q", g.baseSong, g.songOverride, g.moodOverride)
	}
}

func TestWldMusicWordsRunInAnEvent(t *testing.T) {
	g := wldMusicGame(t)
	wldStartEvent(t, g, "ev_music")
	wldFrames(t, g, 60)
	if g.baseSong != "" {
		t.Fatalf("the event ends with the music stopped, the song asked for is %q", g.baseSong)
	}
	if !g.talk.active || g.talk.say.Text != "Quiet." {
		t.Fatalf("the event did not get to the line after the words: %+v", g.talk.say)
	}
	if soundTicks("no such sound") != 0 {
		t.Fatal("a sound that does not exist lasts no time")
	}
}

func TestWldScreenFadesThroughAColorAndBack(t *testing.T) {
	g := wldMusicGame(t)
	red := data.Color{Valid: true}
	red.R = 0xff
	g.visual(event.VisualEffect{Name: "fade", Cue: data.EffectCue{Name: "fade", Color: red, Seconds: 0.25, Hold: 0.1}})
	if g.fadeColor == nil {
		t.Fatal("no fade started")
	}
	total := 2*g.fadeColor.out + g.fadeColor.hold
	for range total - 1 {
		g.stepStaging()
	}
	if g.fadeColor == nil {
		t.Fatal("the fade ended early")
	}
	g.drawFadeColor(wldScreen(g))
	g.stepStaging()
	if g.fadeColor != nil {
		t.Fatalf("the fade did not end after %d ticks", total)
	}
	// a plain fade word is the screen's own fade
	g.visual(event.VisualEffect{Name: "fade", Cue: data.EffectCue{Name: "fade"}})
	if g.fadeColor != nil {
		t.Fatal("a plain fade started a color fade")
	}
}
