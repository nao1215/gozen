package play

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/layer5/sound"
)

const sysMusicGame = `music:
  songs:
    town: {file: town.ogg}
    fight: {file: fight.ogg}
    hush: {file: hush.ogg}
  rules:
    field:
      - {when: "flag.alarm", play: fight}
events:
  raise:
    do:
      - set: {flag.alarm: true}
      - say: "Alarm."
  calm:
    do:
      - set: {flag.alarm: false}
      - say: "Calm."
  swap:
    do:
      - music: {song: hush}
      - say: "Hush."
locations:
  hall:
    name: Hall
    music: town
`

// sysMusicGame opens the game with three one-second Ogg songs, recording the trace so that the
// songs are loaded without a sound device.
func sysMusicOpen(t *testing.T, opts Options) *Game {
	t.Helper()
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	wav := string(tone)
	opts.Trace = &bytes.Buffer{}
	return sysOpen4(t, opts, map[string]string{
		"data/music.yaml":  sysMusicGame,
		"assets/town.ogg":  wav,
		"assets/fight.ogg": wav,
		"assets/hush.ogg":  wav,
	})
}

// sysPlaying runs frames until the module plays song (songs load in the background).
func sysPlaying(t *testing.T, g *Game, song string) {
	t.Helper()
	var got string
	for range 400 {
		sysFrames(t, g, 1)
		if got, _ = sound.Playing(); got == song {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("playing %q, want %q", got, song)
}

// TestLocationMusicPlaysAndRulesChangeIt: the place's song plays; a rule that holds changes
// to its song and the place's own song comes back when it stops holding.
func TestLocationMusicPlaysAndRulesChangeIt(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall"})
	sysPlaying(t, g, "town")
	if g.baseSong != "town" {
		t.Fatalf("base song %q", g.baseSong)
	}
	g.st.Flags["alarm"] = true
	sysPlaying(t, g, "fight")
	g.st.Flags["alarm"] = false
	sysPlaying(t, g, "town")
}

// TestMusicCommandOverridesThePlace: the event's music command plays that song whatever the
// place sets.
func TestMusicCommandOverridesThePlace(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall", Event: "swap", Line: 1})
	sysPlaying(t, g, "hush")
	if g.songOverride != "hush" || g.baseSong != "hush" {
		t.Fatalf("override %q base %q", g.songOverride, g.baseSong)
	}
	g.Move("camp")
	if g.songOverride != "" {
		t.Fatalf("a move should drop the override: %q", g.songOverride)
	}
}

// TestMoodNeedsASongWithIt: a track has no moods, a request for one is answered false.
func TestMoodNeedsASongWithIt(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall"})
	if g.hasMood("tense") {
		t.Fatal("a plain track has no mood")
	}
	g.Mood("tense")
	if g.moodOverride != "tense" {
		t.Fatalf("override %q", g.moodOverride)
	}
}
