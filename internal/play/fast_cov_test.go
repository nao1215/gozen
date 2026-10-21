package play

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestScriptSkipFilmsEveryTickWhenNamed: `skip N NAME` films each tick as NAME_001... and puts the
// game speed back when it is done.
func TestScriptSkipFilmsEveryTickWhenNamed(t *testing.T) {
	tps := ebiten.TPS()
	t.Cleanup(func() { ebiten.SetTPS(tps) })
	g := sysOpen(t, Options{Location: "camp", Script: "skip 4 spin\nquit"}, nil)
	if !sysFrames(t, g, 50) {
		t.Fatal("the script did not end")
	}
	for _, want := range []string{"spin_001", "spin_002", "spin_003", "spin_004"} {
		if !slices.Contains(sysShotNames, want) {
			t.Fatalf("shots %v lack %s", sysShotNames, want)
		}
	}
	if ebiten.TPS() != tps {
		t.Fatalf("the game speed %d was not put back to %d", ebiten.TPS(), tps)
	}
}

// TestFastStopsForTheReasonItHas: reading on stops when the line is reached, when nothing is
// running, at a choice with no answer given (replay pages), or after a very long time.
func TestFastStopsForTheReasonItHas(t *testing.T) {
	var trace bytes.Buffer
	g := sysNovel(t, Options{Seed: 1, Location: "camp", Event: "story", Line: 3, Trace: &trace})
	g.fast.ticks = 60*300 + 1
	g.fast.lines = 99
	sysFrames(t, g, 1)
	if g.fast != nil {
		t.Fatal("reading on never gives up")
	}
	if ev := traceLines(t, trace.Bytes(), "from_memo_ready"); len(ev) != 1 || ev[0]["why"] != "gave up" {
		t.Fatalf("trace %v", ev)
	}
}

// TestFastPageSeekStopsAtAChoiceWithoutAnAnswer.
func TestFastPageSeekStopsAtAChoiceWithoutAnAnswer(t *testing.T) {
	g2 := sysNovel(t, Options{Seed: 1, Location: "camp", Event: "story", Line: 4})
	g2.fast.page = true
	g2.fast.lines = 99
	for range 5000 {
		if g2.fast == nil {
			break
		}
		sysFrames(t, g2, 1)
	}
	if g2.fast != nil || g2.talk.choice == nil {
		t.Fatalf("fast %v choice %v", g2.fast, g2.talk.choice)
	}
}

// TestFastSceneSkipEndsByItsOwnReport.
func TestFastSceneSkipEndsByItsOwnReport(t *testing.T) {
	var trace3 bytes.Buffer
	g3 := sysNovel(t, Options{Seed: 1, Location: "camp", Event: "story", Line: 2, Trace: &trace3})
	g3.fast.scene = true
	g3.fast.lines = 1
	for range 3000 {
		if g3.fast == nil {
			break
		}
		sysFrames(t, g3, 1)
	}
	if g3.fast != nil {
		t.Fatal("scene skip did not stop")
	}
	if sk := traceLines(t, trace3.Bytes(), "skip"); len(sk) == 0 || sk[len(sk)-1]["scene"] != true || sk[len(sk)-1]["on"] != false {
		t.Fatalf("scene skip trace %v", sk)
	}
}

// TestFastToAPictureStopsOnTheLineThatShowsIt: --picture reads on until the picture has been
// under that many lines.
func TestFastToAPictureStopsOnTheLineThatShowsIt(t *testing.T) {
	events := `events:
  show:
    do:
      - say: "Before."
      - image: {file: x.png}
      - say: "Under one."
      - say: "Under two."
`
	g := sysOpen(t, Options{Seed: 1, Location: "camp", Event: "show", Line: 2, Picture: "x.png"}, map[string]string{"data/show.yaml": events})
	sysRunFast(t, g, 4000)
	if g.talk.say.Text != "Under two." || g.cg.File != "x.png" {
		t.Fatalf("stopped at %q with picture %q", g.talk.say.Text, g.cg.File)
	}
}

// TestFastFightsBattlesWhenAsked: a note taken in a battle reads on through the event and
// stops when the battle is on screen.
func TestFastFightsBattlesWhenAsked(t *testing.T) {
	events := `events:
  fight:
    do:
      - say: "Before."
      - battle: one
      - say: "After."
`
	g := sysOpen(t, Options{Seed: 1, Location: "camp", Event: "fight", Line: 1}, map[string]string{"data/fight.yaml": events})
	g.fast.battle = true
	sysRunFast(t, g, 20000)
	if g.currentBattle() == nil {
		t.Fatal("the battle is not on screen")
	}
}

// TestStoreOptionIsUsedAsGiven: a store given in the options takes the saves, and a store that
// fails on write makes the save fail with its error.
func TestStoreOptionIsUsedAsGiven(t *testing.T) {
	mem := gozen.MemStore()
	g, err := New(Options{FS: gameFiles(t), NoSound: true, Store: mem, Location: "camp"})
	if err != nil {
		t.Fatal(err)
	}
	if !g.storeGiven {
		t.Fatal("a given store is not explicit")
	}
	if err := g.save(2); err != nil {
		t.Fatal(err)
	}
	if b, err := mem.Read("save3.json"); err != nil || len(b) == 0 {
		t.Fatalf("the given store did not get the save: %v", err)
	}
	want := errors.New("storage full")
	g.store = sysWriteFails{Store: mem, err: want}
	if err := g.save(1); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	// an unreadable file with a backup that cannot be written fails the save too.
	g.store = sysWriteFails{Store: mem, err: want, readOnly: true}
	if err := mem.Write("save2.json", []byte("junk")); err != nil {
		t.Fatal(err)
	}
	if err := g.save(1); !errors.Is(err, want) {
		t.Fatalf("backup err = %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
}

// sysWriteFails is a store that cannot write (except, with readOnly, nothing at all).
type sysWriteFails struct {
	gozen.Store
	err      error
	readOnly bool
}

func (s sysWriteFails) Write(string, []byte) error { return s.err }

// TestVoiceClipPlaysOrIsReportedMissing: a voice that exists is traced with its length, one that
// is missing is traced as missing and the game goes on; any voiced line makes the voice rows show.
func TestVoiceClipPlaysOrIsReportedMissing(t *testing.T) {
	events := `events:
  hear:
    do:
      - say: {text: "Present", voice: a.wav}
      - say: {text: "Absent", voice: b.wav}
`
	var trace bytes.Buffer
	fsys := map[string]string{"data/hear.yaml": events, "assets/a.wav": string(testgame.Wav(0.25)), "assets/b.wav": string(testgame.Wav(0.25))}
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "hear", Line: 2, Trace: &trace}, fsys)
	sysRunFast(t, g, 2000)
	if !g.usesVoices() {
		t.Fatal("a game with voiced lines has no voice rows")
	}
	g.fast = nil
	v := traceLines(t, trace.Bytes(), "voice")
	// fast reading passes lines silently; open them again as the player would.
	_ = v
	g.talk.voiceT = 0
	gone := g.talk.say
	gone.Voice = "never.wav"
	g.playVoice(&g.talk, gone)
	if g.talk.voiceT != 0 {
		t.Fatalf("a missing clip has a length: %d", g.talk.voiceT)
	}
	v = traceLines(t, trace.Bytes(), "voice")
	if len(v) == 0 || v[len(v)-1]["missing"] != true {
		t.Fatalf("voice lines %v", v)
	}
	present := g.talk.say
	present.Voice = "a.wav"
	g.playVoice(&g.talk, present)
	if g.talk.voiceT < 14 || g.talk.voiceT > 16 {
		t.Fatalf("a quarter second clip lasts %d ticks", g.talk.voiceT)
	}
	g.cfg.VoiceBy = map[string]int{"scout": 3}
	if g.voiceGain("scout") != 0.3 || g.voiceGain("other") != 1 {
		t.Fatalf("gain %v %v", g.voiceGain("scout"), g.voiceGain("other"))
	}
}
