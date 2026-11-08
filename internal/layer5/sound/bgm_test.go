package sound

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"log"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// tinySong is a module song whose modules are 4 frames long.
func tinySong(name string, moods map[string]string) Song {
	return Song{Name: name, Dir: "bgm", BPM: SampleRate * 60 / 4, Bars: 1, Beats: 1, Moods: moods}
}

func readTone(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// syncBuf is a log sink that background goroutines may write to.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func captureLog(t *testing.T) *syncBuf {
	t.Helper()
	sb := &syncBuf{}
	old := log.Writer()
	log.SetOutput(sb)
	t.Cleanup(func() { log.SetOutput(old) })
	return sb
}

// freshSession ends any session left by an earlier test, and ends this test's at its end.
func freshSession(t *testing.T) {
	t.Helper()
	end := func() {
		if err := testSession.Close(); err != nil {
			t.Error(err)
		}
	}
	end()
	t.Cleanup(end)
}

var (
	quickOnce    sync.Once
	quickEffects map[string][]byte
)

// recordQuickly is InitRecord with the built-in effects made once for the whole test run
// (making them takes a second or more): for tests that are about something else.
func recordQuickly(w io.Writer, tps int) {
	quickOnce.Do(func() { quickEffects = testSession.synthEffects() })
	testSession.rec = &recorder{w: w, tps: tps}
	testSession.mu.Lock()
	maps.Copy(testSession.effects, quickEffects)
	testSession.mu.Unlock()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func idle(m *Modular, song string) func() bool {
	return func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.songs[song] != nil && !m.loading[song]
	}
}

// lefts reads n frames and returns the left channel.
func lefts(t *testing.T, m *Modular, n int) []float32 {
	t.Helper()
	p := make([]byte, 8*n)
	if got, err := m.Read(p); err != nil || got != len(p) {
		t.Fatalf("read %d %v", got, err)
	}
	out := make([]float32, n)
	for i := range n {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(p[i*8:]))
	}
	return out
}

func constPCM(v int16, frames int) []int16 {
	d := make([]int16, frames*2)
	for i := range d {
		d[i] = v
	}
	return d
}

func equalF(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-6 {
			return false
		}
	}
	return true
}

func TestSongBasics(t *testing.T) {
	if !(Song{File: "a.ogg"}).Track() || (Song{Dir: "d"}).Track() {
		t.Fatal("Track is File != empty")
	}
	// Defaults: 8 bars of 4 beats; a BPM of 0 never divides by zero.
	if got, want := (Song{BPM: 120}).ModuleFrames(), SampleRate*60*8*4/120; got != want {
		t.Fatalf("default module %d, want %d", got, want)
	}
	if got, want := (Song{BPM: 60, Bars: 2, Beats: 3}).ModuleFrames(), SampleRate*60*6/60; got != want {
		t.Fatalf("module %d, want %d", got, want)
	}
	if got, want := (Song{}).ModuleFrames(), SampleRate*60*32; got != want {
		t.Fatalf("zero BPM module %d, want %d", got, want)
	}
	files := Song{Dir: "x/y", Moods: map[string]string{"calm": "c", "fight": "f"}}.files()
	if len(files) != 2 || files["calm"] != "x/y/c.ogg" || files["fight"] != "x/y/f.ogg" {
		t.Fatalf("mood files %v", files)
	}
	if tf := (Song{File: "t.ogg", Moods: map[string]string{"ignored": "i"}}).files(); len(tf) != 1 || tf[Neutral] != "t.ogg" {
		t.Fatalf("track files %v", tf)
	}
	if AssetPath("") != "" || AssetPath("bgm/a.ogg") != "assets/bgm/a.ogg" {
		t.Fatal("AssetPath")
	}
}

func TestDecodeOgg(t *testing.T) {
	pcm, err := decode(readTone(t))
	if err != nil {
		t.Fatal(err)
	}
	// 0.6 s of stereo: two samples per frame.
	if frames := len(pcm) / 2; frames < SampleRate*55/100 || frames > SampleRate*65/100 {
		t.Fatalf("%d frames for a 0.6 s tone", frames)
	}
	if slices.Max(pcm) < 1000 {
		t.Fatal("decoded tone is silent")
	}
	if _, err := decode([]byte("not ogg")); err == nil {
		t.Fatal("garbage decoded")
	}
}

func TestSongLoadSkipsBrokenFiles(t *testing.T) {
	logs := captureLog(t)
	fsys := fstest.MapFS{
		"bgm/good.ogg": {Data: readTone(t)},
		"bgm/bad.ogg":  {Data: []byte("junk")},
	}
	s := Song{Name: "s", Dir: "bgm", Moods: map[string]string{"good": "good", "bad": "bad", "gone": "gone"}}
	mods := s.load(fsys)
	if len(mods) != 1 || len(mods["good"]) == 0 {
		t.Fatalf("loaded moods %v", mods)
	}
	if out := logs.String(); !strings.Contains(out, "bgm s:") {
		t.Fatalf("broken moods are not logged: %q", out)
	}
}

// chunkReader returns at most n bytes per Read, to split 16-bit samples in the middle.
type chunkReader struct {
	b []byte
	n int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.b) == 0 {
		return 0, io.EOF
	}
	k := min(c.n, len(c.b), len(p))
	copy(p, c.b[:k])
	c.b = c.b[k:]
	return k, nil
}

func TestPCMFillReassemblesSplitSamples(t *testing.T) {
	want := []int16{1, -2, 300, -32768, 32767, 0}
	var raw []byte
	for _, v := range want {
		raw = binary.LittleEndian.AppendUint16(raw, uint16(v)) //nolint:gosec // two's complement bits
	}
	p := &pcm{data: make([]int16, len(want))}
	calls := 0
	if err := p.fill(&chunkReader{b: raw, n: 3}, func() { calls++ }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.data, want) || p.ready() != len(want) {
		t.Fatalf("data %v ready %d", p.data, p.ready())
	}
	if !p.done.Load() || calls == 0 {
		t.Fatal("fill did not finish or report progress")
	}
}

func TestPCMDropsSamplesPastTheLength(t *testing.T) {
	logs := captureLog(t)
	p := &pcm{data: make([]int16, 2)}
	raw := make([]byte, 12)
	if err := p.fill(bytes.NewReader(raw), nil); err != nil {
		t.Fatal(err)
	}
	if p.ready() != 2 || !strings.Contains(logs.String(), "4 samples past") {
		t.Fatalf("ready %d log %q", p.ready(), logs.String())
	}
}

func TestPCMPlayableAndSize(t *testing.T) {
	p := &pcm{data: make([]int16, 4*startFrames)}
	if p.playable() || p.size() != len(p.data) {
		t.Fatal("an unfilled stream is not playable and reports its capacity")
	}
	p.n.Store(2*startFrames - 1)
	if p.playable() {
		t.Fatal("one sample short of the start buffer")
	}
	p.n.Store(2 * startFrames)
	if !p.playable() {
		t.Fatal("start buffer filled but not playable")
	}
	p.n.Store(10)
	p.done.Store(true)
	if !p.playable() || p.size() != 10 {
		t.Fatal("a finished stream is playable and sized by what was decoded")
	}
}

func TestMoodMovesAtTheModuleBoundary(t *testing.T) {
	m := NewModular("s", nil)
	m.AddSong(tinySong("s", map[string]string{Neutral: "n", "fight": "f"}), map[string][]int16{
		Neutral: constPCM(8192, 4), "fight": constPCM(16384, 4),
	})
	m.PlaySong("s", Neutral)
	if got := lefts(t, m, 2); !equalF(got, []float32{0.25, 0.25}) {
		t.Fatalf("start %v", got)
	}
	m.Request("fight")
	// The module finishes its 4 frames, then the requested mood takes over.
	if got := lefts(t, m, 6); !equalF(got, []float32{0.25, 0.25, 0.5, 0.5, 0.5, 0.5}) {
		t.Fatalf("boundary %v", got)
	}
	if !m.HasMood("fight") || m.HasMood("missing") {
		t.Fatal("HasMood")
	}
	if name, file := m.Current(); name != "s" || file != "f" {
		t.Fatalf("current %q %q", name, file)
	}
}

func TestJumpChangesAtOnceAndFadesTheOld(t *testing.T) {
	m := NewModular("s", nil)
	m.AddSong(tinySong("s", map[string]string{Neutral: "n", "fight": "f"}), map[string][]int16{
		Neutral: constPCM(8192, 4), "fight": constPCM(16384, 4),
	})
	m.PlaySong("s", Neutral)
	lefts(t, m, 1)
	m.Jump("fight")
	got := lefts(t, m, 1)
	// New mood 0.5 plus the old mood still audible while it fades out.
	if got[0] <= 0.5 || got[0] > 0.75+1e-6 {
		t.Fatalf("jump frame %v", got)
	}
}

func TestOneShotMoodReturnsToNeutral(t *testing.T) {
	m := NewModular("s", []string{"sting"})
	m.AddSong(tinySong("s", map[string]string{Neutral: "n", "sting": "s"}), map[string][]int16{
		Neutral: constPCM(8192, 4), "sting": constPCM(16384, 4),
	})
	m.PlaySong("s", Neutral)
	lefts(t, m, 1)
	m.Jump("sting")
	lefts(t, m, 1) // the jump frame itself carries the fade tail
	// The sting plays through once (3 more frames, the first two still carrying the fading
	// tail of the old mood) and neutral returns.
	got := lefts(t, m, 6)
	if got[0] < 0.5 || got[1] < 0.5 || got[2] != 0.5 || !equalF(got[3:], []float32{0.25, 0.25, 0.25}) {
		t.Fatalf("one-shot %v", got)
	}
}

func TestJumpLastOneShotEndsTheMusic(t *testing.T) {
	m := NewModular("s", []string{"sting"})
	m.AddSong(tinySong("s", map[string]string{Neutral: "n", "sting": "s"}), map[string][]int16{
		Neutral: constPCM(8192, 4), "sting": constPCM(16384, 4),
	})
	m.PlaySong("s", Neutral)
	m.JumpLast("sting")
	lefts(t, m, 8)
	if m.Wanted() != "" {
		t.Fatalf("music still wanted: %q", m.Wanted())
	}
	if name, _ := m.Current(); name != "" {
		t.Fatalf("still playing %q", name)
	}
}

func TestStopFadesOut(t *testing.T) {
	m := NewModular("s", nil)
	m.AddSong(Song{Name: "s", File: "s.ogg"}, map[string][]int16{Neutral: constPCM(16384, 5000)})
	m.PlaySong("s", Neutral)
	lefts(t, m, 2)
	m.Stop()
	got := lefts(t, m, 200)
	if got[0] <= got[199] || got[199] <= 0 {
		t.Fatalf("stop did not fade: %v ... %v", got[0], got[199])
	}
	if m.Wanted() != "" {
		t.Fatal("stop leaves a wanted song")
	}
}

func TestPlayOnceEndsWithItsTail(t *testing.T) {
	m := NewModular("s", nil)
	m.AddSong(Song{Name: "s", File: "s.ogg"}, map[string][]int16{Neutral: constPCM(16384, 3)})
	m.PlayOnce("s", Neutral)
	lefts(t, m, 8)
	if m.Wanted() != "" {
		t.Fatal("a played-once song stays wanted")
	}
	if got := lefts(t, m, 2); !equalF(got, []float32{0, 0}) {
		t.Fatalf("silence expected after the song, got %v", got)
	}
}

func TestTrackLoopStartAndShortFrames(t *testing.T) {
	m := NewModular("s", nil)
	d := []int16{2000, 2000, 4000, 4000, 6000, 6000, 8000, 8000}
	m.AddSong(Song{Name: "s", File: "s.ogg", LoopStart: 2.0 / SampleRate}, map[string][]int16{Neutral: d})
	m.PlaySong("s", Neutral)
	got := lefts(t, m, 7)
	// After the last frame the track loops back to LoopStart (frame 2).
	want := []float32{2000, 4000, 6000, 8000, 6000, 8000, 6000}
	for i := range want {
		want[i] /= 32768
	}
	if !equalF(got, want) {
		t.Fatalf("loop %v, want %v", got, want)
	}
}

func TestTrimEvictsTheOldestUnprotectedSong(t *testing.T) {
	m := NewModular("a", nil)
	m.Lazy(nil, nil, 100) // 100 bytes; each song below holds 2*cap = 40 bytes
	add := func(n string) {
		m.AddSong(Song{Name: n, File: n + ".ogg"}, map[string][]int16{Neutral: make([]int16, 20)})
	}
	add("a")
	add("b")
	add("c")
	if got := m.Loaded(); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("after budget overflow %v", got)
	}
	m.Keep("c")
	add("d")
	if got := m.Loaded(); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("a kept song was evicted or the new one was not %v", got)
	}
}

func TestLazyStreamsMoodsAndKeepsQueued(t *testing.T) {
	logs := captureLog(t)
	tone := readTone(t)
	fsys := fstest.MapFS{
		"bgm/n.ogg": {Data: tone}, "bgm/f.ogg": {Data: tone}, "bgm/broken.ogg": {Data: []byte("x")},
		"bgm/k.ogg": {Data: tone},
	}
	m := NewModular("s", nil)
	m.Lazy(fsys, []Song{
		tinySong("s", map[string]string{Neutral: "n", "fight": "f", "broken": "broken", "gone": "gone"}),
		{Name: "k", File: "bgm/k.ogg"},
	}, 0)
	m.PlaySong("s", "fight")
	m.Keep("k") // waits for the song wanted now
	m.mu.Lock()
	queued := slices.Contains(m.later, "k")
	m.mu.Unlock()
	if !queued {
		t.Fatal("Keep while loading did not queue the song")
	}
	waitFor(t, "both songs", func() bool { return idle(m, "s")() && idle(m, "k")() })
	if got := m.Loaded(); !slices.Equal(got, []string{"k", "s"}) {
		t.Fatalf("loaded %v", got)
	}
	out := logs.String()
	if !strings.Contains(out, "bgm s:") {
		t.Fatalf("broken or missing mood files are not logged: %q", out)
	}
	m.PlaySong("s", "fight")
	var peak float32
	for _, v := range lefts(t, m, 4000) {
		peak = max(peak, v)
	}
	if peak < 0.01 {
		t.Fatal("a streamed song is silent")
	}
}

func TestPlaySongForUnknownSongIsHarmless(t *testing.T) {
	m := NewModular("s", nil)
	m.Lazy(fstest.MapFS{}, nil, 0)
	m.PlaySong("nowhere", Neutral)
	if got := lefts(t, m, 3); !equalF(got, []float32{0, 0, 0}) {
		t.Fatalf("expected silence, got %v", got)
	}
	if m.HasMood(Neutral) {
		t.Fatal("unknown song has a mood")
	}
	if name, file := m.Current(); name != "" || file != "" {
		t.Fatalf("current %q %q", name, file)
	}
}

func TestMoodFallbacks(t *testing.T) {
	m := NewModular("s", nil)
	s := &loadedSong{Song: Song{Name: "s", Dir: "d", Moods: map[string]string{"a": "a"}}, mods: map[string]*pcm{"a": fullPCM(make([]int16, 4))}}
	if got := m.mood(s, "unlisted"); got != "a" {
		t.Fatalf("falls back to any playable mood, got %q", got)
	}
	s.mods[Neutral] = fullPCM(make([]int16, 4))
	if got := m.mood(s, "unlisted"); got != Neutral {
		t.Fatalf("neutral preferred, got %q", got)
	}
	empty := &loadedSong{Song: Song{Name: "e"}, mods: map[string]*pcm{}}
	if got := m.mood(empty, "x"); got != Neutral {
		t.Fatalf("nothing playable gives %q", got)
	}
}

func TestPackageControlsWithoutMusic(t *testing.T) {
	freshSession(t)
	if testSession.Active() || testSession.WantedSong() != "" || testSession.HasMood("x") {
		t.Fatal("no music loaded but reported active")
	}
	if song, mod := testSession.Playing(); song != "" || mod != "" {
		t.Fatal("playing without music")
	}
	// None of these may panic without a loaded modular.
	testSession.Request("x")
	testSession.PlaySong("a", "x")
	testSession.PlaySongFade("a", "x", 1)
	testSession.StopFade(1)
	testSession.Stop()
	testSession.Jump("x")
	testSession.JumpLast("x")
	testSession.PlayOnce("a", "x")
	testSession.Keep("a")
	testSession.Relearn(nil, nil)
}

func TestLoadBGMControlsAndVolumes(t *testing.T) {
	freshSession(t)
	tone := readTone(t)
	fsys := fstest.MapFS{"bgm/a.ogg": {Data: tone}, "bgm/b.ogg": {Data: tone}}
	songs := []Song{{Name: "a", File: "bgm/a.ogg"}, {Name: "b", File: "bgm/b.ogg"}}
	testSession.LoadBGM(fsys, songs, "a", nil, 0.5)
	if !testSession.Active() || testSession.WantedSong() != "a" {
		t.Fatalf("active %v wanted %q", testSession.Active(), testSession.WantedSong())
	}
	if testSession.bgmVolume() != 0.5 || math.Abs(testSession.seVolume()-0.7) > 1e-12 {
		t.Fatalf("volumes %v %v", testSession.bgmVolume(), testSession.seVolume())
	}
	testSession.Request("calm")
	if m := testSession.current(); m.want != "calm" {
		t.Fatalf("Request: %q", m.want)
	}
	testSession.Jump("calm")
	if m := testSession.current(); !m.jump || m.last {
		t.Fatal("Jump")
	}
	testSession.JumpLast("calm")
	if m := testSession.current(); !m.jump || !m.last {
		t.Fatal("JumpLast")
	}
	testSession.PlayOnce("b", Neutral)
	if testSession.WantedSong() != "b" || testSession.current().onceSong != "b" {
		t.Fatal("PlayOnce")
	}
	testSession.Stop()
	if testSession.WantedSong() != "" {
		t.Fatal("Stop")
	}
	testSession.PlaySong("a", Neutral)
	if testSession.HasMood("calm") {
		t.Fatal("a track has no moods")
	}
	waitFor(t, "song a", func() bool { return idle(testSession.current(), "a")() })
	if song, _ := testSession.Playing(); song != "" && song != "a" {
		t.Fatalf("playing %q", song)
	}
	testSession.SetVolumes(0.5, 0.25)
	if testSession.bgmVolume() != 0.25 || math.Abs(testSession.seVolume()-0.175) > 1e-12 {
		t.Fatalf("scaled volumes %v %v", testSession.bgmVolume(), testSession.seVolume())
	}
	if !testSession.ToggleMute() || testSession.bgmVolume() != 0 || testSession.seVolume() != 0 {
		t.Fatal("mute")
	}
	if testSession.ToggleMute() || testSession.bgmVolume() != 0.25 || math.Abs(testSession.seVolume()-0.175) > 1e-12 {
		t.Fatal("unmute")
	}
	testSession.PlaySongFade("b", Neutral, 0.5)
	if testSession.WantedSong() != "b" {
		t.Fatalf("wanted %q", testSession.WantedSong())
	}
	waitFor(t, "song b", func() bool { m := testSession.current(); return idle(m, "b")() })
	testSession.StopFade(0.1)
	if testSession.WantedSong() != "" {
		t.Fatal("StopFade keeps a wanted song")
	}
	testSession.PlaySong("a", Neutral)
	waitFor(t, "song a", func() bool { m := testSession.current(); return idle(m, "a")() })
	testSession.Keep("b")
	// Relearn drops loaded songs that are neither playing nor wanted nor loading, then
	// reloads the kept ones from the new files.
	testSession.Relearn(fstest.MapFS{"bgm/a.ogg": {Data: tone}, "bgm/b.ogg": {Data: tone}}, songs)
	waitFor(t, "kept song b again", func() bool { m := testSession.current(); return idle(m, "b")() })
	testSession.Pump(60)
	if err := testSession.Close(); err != nil {
		t.Fatal(err)
	}
	if testSession.Active() || testSession.bgmVolume() != 0.7 {
		t.Fatalf("Close leaves the session: active %v volume %v", testSession.Active(), testSession.bgmVolume())
	}
}

func TestPumpAdvancesMusicWithoutADevice(t *testing.T) {
	freshSession(t)
	m := NewModular("s", nil)
	m.AddSong(Song{Name: "s", File: "s.ogg"}, map[string][]int16{Neutral: constPCM(100, 5000)})
	m.PlaySong("s", Neutral)
	testSession.mu.Lock()
	testSession.bgm = m
	testSession.mu.Unlock()
	testSession.Pump(0) // a non-positive rate does nothing
	if m.pos != 0 {
		t.Fatal("Pump(0) advanced")
	}
	testSession.Pump(60)
	if m.pos != SampleRate/60 {
		t.Fatalf("pos %d, want %d", m.pos, SampleRate/60)
	}
}

func TestRecordingMixesMusicAndEffects(t *testing.T) {
	freshSession(t)
	var out bytes.Buffer
	testSession.InitRecord(&out, 60)
	frame := SampleRate / 60
	if err := testSession.Tick(); err != nil || out.Len() != frame*8 {
		t.Fatalf("silent tick wrote %d bytes, %v", out.Len(), err)
	}
	if out.Bytes()[0] != 0 || slices.Max(out.Bytes()) != 0 {
		t.Fatal("no music and no effect must record silence")
	}
	if testSession.EffectSeconds("cursor") < 0.04 || testSession.EffectSeconds("cursor") > 0.06 || testSession.EffectSeconds("nope") != 0 {
		t.Fatalf("effect length %v", testSession.EffectSeconds("cursor"))
	}
	testSession.Play("cursor")
	testSession.Play("not-a-sound")
	testSession.Play("cursor")
	if len(testSession.rec.voices) != 2 {
		t.Fatalf("voices %d", len(testSession.rec.voices))
	}
	out.Reset()
	if err := testSession.Tick(); err != nil {
		t.Fatal(err)
	}
	loud := false
	for i := 0; i+4 <= out.Len(); i += 4 {
		if math.Float32frombits(binary.LittleEndian.Uint32(out.Bytes()[i:])) != 0 {
			loud = true
		}
	}
	if !loud {
		t.Fatal("effect missing from the recording")
	}
	// The 0.05 s effect is over after about three ticks and its voices are dropped.
	for range 4 {
		if err := testSession.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if len(testSession.rec.voices) != 0 {
		t.Fatalf("finished voices kept: %d", len(testSession.rec.voices))
	}
	testSession.ToggleMute()
	testSession.Play("cursor")
	out.Reset()
	if err := testSession.Tick(); err != nil {
		t.Fatal(err)
	}
	if slices.Max(out.Bytes()) != 0 {
		t.Fatal("muted recording is not silent")
	}
}

func TestRecordingLoadsAllSongsAtOnce(t *testing.T) {
	freshSession(t)
	var out bytes.Buffer
	recordQuickly(&out, 30)
	fsys := fstest.MapFS{"bgm/a.ogg": {Data: readTone(t)}}
	testSession.LoadBGM(fsys, []Song{{Name: "a", File: "bgm/a.ogg"}}, "a", nil, 1)
	if got := testSession.current().Loaded(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("recording loads songs synchronously, got %v", got)
	}
	if err := testSession.Tick(); err != nil || out.Len() != SampleRate/30*8 {
		t.Fatalf("tick %d %v", out.Len(), err)
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestRecordingWriteErrorSticks(t *testing.T) {
	freshSession(t)
	want := errors.New("disk full")
	recordQuickly(failWriter{want}, 60)
	if err := testSession.Tick(); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if err := testSession.Tick(); !errors.Is(err, want) {
		t.Fatal("a failed recording must keep failing")
	}
}

func TestPlayVoiceWhileRecording(t *testing.T) {
	freshSession(t)
	recordQuickly(io.Discard, 60)
	testSession.PlayVoice(make([]byte, 64), 1)
	testSession.PlayVoice(nil, 1)
	if len(testSession.rec.voices) != 1 {
		t.Fatalf("voices %d", len(testSession.rec.voices))
	}
}

var testSession = NewSession()

func TestSessionIsolation(t *testing.T) {
	parent, child := NewSession(), NewSession()
	t.Cleanup(func() {
		if err := errors.Join(parent.Close(), child.Close()); err != nil {
			t.Error(err)
		}
	})
	var hostAudio, childAudio bytes.Buffer
	parent.InitRecord(&hostAudio, 60)
	child.InitRecord(&childAudio, 60)
	parent.SetVolumes(0.2, 0.3)
	child.SetVolumes(0.8, 0.9)
	parent.SetVary(0, 0)
	child.SetVary(0.5, 0.4)
	parent.Play("decide")
	if len(parent.rec.voices) == 0 || len(child.rec.voices) != 0 {
		t.Fatal("effect crossed session boundary")
	}
	if err := child.Tick(); err != nil {
		t.Fatal(err)
	}
	if hostAudio.Len() != 0 || childAudio.Len() == 0 {
		t.Fatal("recorder crossed session boundary")
	}
	parent.effects["private"] = []byte{1, 2}
	parent.voiceCache = []voiceClip{{path: "same.ogg", pcm: []byte{3}}}
	if child.effects["private"] != nil || len(child.voiceCache) != 0 {
		t.Fatal("asset cache crossed session boundary")
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if parent.rec == nil || parent.bgmScale != 0.2 || parent.seScale != 0.3 || parent.varyPitch != 0 || len(parent.voiceCache) != 1 || parent.effects["private"] == nil {
		t.Fatal("closing child changed parent resources")
	}
	if err := parent.Tick(); err != nil {
		t.Fatal(err)
	}
	if hostAudio.Len() == 0 {
		t.Fatal("parent recorder did not resume")
	}
}

type heldAudioFS struct {
	fs.FS
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *heldAudioFS) Open(name string) (fs.File, error) {
	f.once.Do(func() { close(f.started) })
	<-f.release
	return f.FS.Open(name)
}

func TestSessionCloseJoinsBorrowedAudioReads(t *testing.T) {
	gate := &heldAudioFS{FS: fstest.MapFS{"tone.ogg": {Data: readTone(t)}}, started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(gate.release) }) })
	session := NewSession()
	session.LoadBGM(gate, []Song{{Name: "one", File: "tone.ogg"}}, "one", nil, 1)
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("audio loader did not start")
	}
	done := make(chan error, 1)
	go func() { done <- session.Close() }()
	select {
	case err := <-done:
		t.Fatalf("Close returned while borrowed FS read was active: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release.Do(func() { close(gate.release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not join completed read")
	}
}
