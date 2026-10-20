// Package sound plays the background music and sound effects.
package sound

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
)

const SampleRate = 44100

const Neutral = "neutral"

type Song struct {
	Name string

	File      string
	LoopStart float64
	Dir       string
	BPM       int
	Bars      int
	Beats     int
	Moods     map[string]string
}

func (s Song) Track() bool { return s.File != "" }

func (s Song) files() map[string]string {
	files := map[string]string{}
	if s.Track() {
		files[Neutral] = s.File
	} else {
		for mood, file := range s.Moods {
			files[mood] = s.Dir + "/" + file + ".ogg"
		}
	}
	return files
}

func (s Song) load(fsys fs.FS) map[string][]int16 {
	mods := map[string][]int16{}
	for mood, path := range s.files() {
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			log.Printf("bgm %s: %v", s.Name, err)
			continue
		}
		pcm, err := decode(b)
		if err != nil {
			log.Printf("bgm %s: %v", s.Name, err)
			continue
		}
		mods[mood] = pcm
	}
	return mods
}

func (s Song) ModuleFrames() int {
	bars, beats := s.Bars, s.Beats
	if bars == 0 {
		bars = 8
	}
	if beats == 0 {
		beats = 4
	}
	return SampleRate * 60 * bars * beats / max(1, s.BPM)
}

const songFade = SampleRate * 7 / 10

var (
	ctx     *audio.Context
	bgm     *Modular
	bgmPlay *audio.Player
	muted   bool
	volume  = 0.7
	mu      sync.Mutex
)

func Init() error {
	ctx = audio.CurrentContext()
	if ctx == nil {
		ctx = audio.NewContext(SampleRate)
	}
	if ctx.SampleRate() != SampleRate {
		return fmt.Errorf("sound: audio context requires sample rate %d", SampleRate)
	}
	makeEffects()
	return nil
}

const bgmBudget = 64 << 20

const startFrames = SampleRate / 4

const playerBuffer = 100 * time.Millisecond

func LoadBGM(fsys fs.FS, songs []Song, first string, oneShot []string, vol float64) {
	m := NewModular(first, oneShot)
	mu.Lock()
	bgm = m
	volume = vol
	mu.Unlock()
	if rec != nil {
		recordBGM(m, fsys, songs)
		return
	}
	m.Lazy(fsys, songs, bgmBudget)
	m.PlaySong(first, Neutral)
	if ctx == nil {
		return
	}
	p, err := ctx.NewPlayerF32(m)
	if err != nil {
		log.Printf("bgm: %v", err)
		return
	}
	p.SetBufferSize(playerBuffer)

	mu.Lock()
	p.SetVolume(bgmVolume())
	bgmPlay = p
	mu.Unlock()
	p.Play()
}

func decode(b []byte) ([]int16, error) {
	p, st, err := openPCM(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if err := p.fill(st, nil); err != nil {
		return nil, err
	}
	return p.data[:p.ready()], nil
}

func openPCM(r io.ReadSeeker) (*pcm, io.Reader, error) {
	st, err := vorbis.DecodeWithSampleRate(SampleRate, r)
	if err != nil {
		return nil, nil, err
	}
	return &pcm{data: make([]int16, max(0, st.Length()/2))}, st, nil
}

type pcm struct {
	data []int16
	n    atomic.Int64
	done atomic.Bool
}

func fullPCM(d []int16) *pcm {
	p := &pcm{data: d}
	p.n.Store(int64(len(d)))
	p.done.Store(true)
	return p
}

func (p *pcm) ready() int { return int(p.n.Load()) }

func (p *pcm) size() int {
	if p.done.Load() {
		return p.ready()
	}
	return len(p.data)
}

func (p *pcm) playable() bool {
	return p.done.Load() || p.ready() >= 2*startFrames
}

func (p *pcm) fill(r io.Reader, progress func()) error {
	defer p.done.Store(true)
	buf := make([]byte, 16<<10)
	odd := 0
	i, extra := 0, 0
	for {
		n, err := r.Read(buf[odd:])
		n += odd
		even := n &^ 1
		for j := 0; j < even; j += 2 {
			if i < len(p.data) {
				//nolint:gosec // PCM reinterprets a signed two-complement 16-bit sample.
				p.data[i] = int16(binary.LittleEndian.Uint16(buf[j:]))
				i++
			} else {
				extra++
			}
		}
		odd = n - even
		if odd > 0 {
			buf[0] = buf[even]
		}
		p.n.Store(int64(i))
		if progress != nil {
			progress()
		}
		if err == io.EOF {
			if extra > 0 {
				log.Printf("bgm: %d samples past the stream's length dropped", extra)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func current() *Modular {
	mu.Lock()
	defer mu.Unlock()
	return bgm
}

func Request(mood string) {
	if m := current(); m != nil {
		m.Request(mood)
	}
}

func PlaySong(song, mood string) {
	if m := current(); m != nil {
		m.PlaySong(song, mood)
	}
}

func Stop() {
	if m := current(); m != nil {
		m.Stop()
	}
}

func Jump(mood string) {
	if m := current(); m != nil {
		m.Jump(mood)
	}
}

func JumpLast(mood string) {
	if m := current(); m != nil {
		m.JumpLast(mood)
	}
}

func PlayOnce(song, mood string) {
	if m := current(); m != nil {
		m.PlayOnce(song, mood)
	}
}

func Keep(songs ...string) {
	if m := current(); m != nil {
		m.Keep(songs...)
	}
}

func Playing() (song, module string) {
	if m := current(); m != nil {
		return m.Current()
	}
	return "", ""
}

func Active() bool { return current() != nil }

func WantedSong() string {
	if m := current(); m != nil {
		return m.Wanted()
	}
	return ""
}

func HasMood(mood string) bool {
	if m := current(); m != nil {
		return m.HasMood(mood)
	}
	return false
}

func ToggleMute() bool {
	mu.Lock()
	defer mu.Unlock()
	muted = !muted
	if bgmPlay != nil {
		bgmPlay.SetVolume(bgmVolume())
	}
	return muted
}

func Muted() bool {
	mu.Lock()
	defer mu.Unlock()
	return muted
}

func bgmVolume() float64 {
	if muted {
		return 0
	}
	return volume * bgmScale
}

func seVolume() float64 {
	if muted {
		return 0
	}
	return 0.7 * seScale
}

var bgmScale, seScale = 1.0, 1.0

func SetVolumes(bgm, se float64) {
	mu.Lock()
	bgmScale, seScale = bgm, se
	if bgmPlay != nil {
		bgmPlay.SetVolume(bgmVolume())
	}
	mu.Unlock()
}

type tail struct {
	pcm        *pcm
	pos        int
	fade, left int
}

type loadedSong struct {
	Song
	mods map[string]*pcm
}

func (s *loadedSong) frames() int {
	if !s.Track() {
		return s.ModuleFrames()
	}
	if p := s.mods[Neutral]; p != nil {
		return p.size() / 2
	}
	return 0
}

func (s *loadedSong) bytes() int {
	n := 0
	for _, p := range s.mods {
		n += 2 * cap(p.data)
	}
	return n
}

type Modular struct {
	mu    sync.Mutex
	songs map[string]*loadedSong

	defs     map[string]Song
	fsys     fs.FS
	loading  map[string]bool
	kept     map[string]bool
	budget   int
	used     map[string]int
	clock    int
	oneShot  map[string]bool
	song     string
	wantSong string
	cur      string
	want     string
	jump     bool
	stop     bool
	last     bool
	onceSong string
	pos      int
	tails    []tail
}

func NewModular(first string, oneShot []string) *Modular {
	m := &Modular{songs: map[string]*loadedSong{}, oneShot: map[string]bool{}, song: first, wantSong: first, cur: Neutral, want: Neutral}
	for _, o := range oneShot {
		m.oneShot[o] = true
	}
	return m
}

func (m *Modular) AddSong(s Song, mods map[string][]int16) {
	ls := &loadedSong{Song: s, mods: map[string]*pcm{}}
	for mood, d := range mods {
		ls.mods[mood] = fullPCM(d)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.songs[s.Name] = ls
	delete(m.loading, s.Name)
	m.touch(s.Name)
	m.trim()
}

func (m *Modular) Lazy(fsys fs.FS, songs []Song, budget int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fsys, m.budget = fsys, budget
	m.defs = map[string]Song{}
	for _, s := range songs {
		m.defs[s.Name] = s
	}
}

func (m *Modular) Loaded() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for name := range m.songs {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (m *Modular) Keep(songs ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kept == nil {
		m.kept = map[string]bool{}
	}
	for _, s := range songs {
		m.kept[s] = true
		m.fetch(s, Neutral)
	}
}

func (m *Modular) fetch(song, mood string) {
	m.touch(song)
	s, ok := m.defs[song]
	if !ok || m.songs[song] != nil || m.loading[song] || m.fsys == nil {
		return
	}
	if m.loading == nil {
		m.loading = map[string]bool{}
	}
	m.loading[song] = true
	go m.stream(s, mood, m.fsys)
}

func (m *Modular) stream(s Song, first string, fsys fs.FS) {
	files := s.files()
	order := slices.Sorted(func(yield func(string) bool) {
		for mood := range files {
			if !yield(mood) {
				return
			}
		}
	})
	rank := func(mood string) int {
		switch mood {
		case first:
			return 0
		case Neutral:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(order, func(a, b string) int { return rank(a) - rank(b) })
	ls := &loadedSong{Song: s, mods: map[string]*pcm{}}
	shown := false
	show := func() {
		if !shown {
			shown = true
			m.songs[s.Name] = ls
			m.trim()
		}
	}
	for _, mood := range order {
		b, err := fs.ReadFile(fsys, files[mood])
		if err != nil {
			log.Printf("bgm %s: %v", s.Name, err)
			continue
		}
		p, st, err := openPCM(bytes.NewReader(b))
		if err != nil {
			log.Printf("bgm %s: %v", s.Name, err)
			continue
		}
		m.mu.Lock()
		ls.mods[mood] = p
		m.mu.Unlock()
		err = p.fill(st, func() {
			if !shown && p.playable() {
				m.mu.Lock()
				show()
				m.mu.Unlock()
			}
		})
		if err != nil {
			log.Printf("bgm %s: %v", s.Name, err)
		}
	}
	m.mu.Lock()
	show()
	delete(m.loading, s.Name)
	m.trim()
	m.mu.Unlock()
}

func (m *Modular) touch(song string) {
	if m.used == nil {
		m.used = map[string]int{}
	}
	m.clock++
	m.used[song] = m.clock
}

func (m *Modular) trim() {
	if m.budget <= 0 {
		return
	}
	total := 0
	for _, s := range m.songs {
		total += s.bytes()
	}
	for total > m.budget {
		oldest, at := "", 0
		for name := range m.songs {
			if name == m.song || name == m.wantSong || m.kept[name] || m.loading[name] {
				continue
			}
			if oldest == "" || m.used[name] < at {
				oldest, at = name, m.used[name]
			}
		}
		if oldest == "" {
			return
		}
		total -= m.songs[oldest].bytes()
		delete(m.songs, oldest)
	}
}

func (m *Modular) Request(mood string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want = mood
}

func (m *Modular) PlaySong(song, mood string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wantSong, m.want = song, mood
	if song != m.song {
		m.last = false
	}
	m.onceSong = ""
	m.fetch(song, mood)
}

func (m *Modular) PlayOnce(song, mood string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wantSong, m.want, m.jump, m.last, m.onceSong = song, mood, true, false, song
	m.fetch(song, mood)
}

func (m *Modular) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wantSong, m.want, m.jump, m.stop, m.last, m.onceSong = "", Neutral, false, true, false, ""
}

func (m *Modular) Jump(mood string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want, m.jump, m.last = mood, true, false
}

func (m *Modular) JumpLast(mood string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.want, m.jump, m.last = mood, true, true
}

func (m *Modular) Wanted() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wantSong
}

func (m *Modular) HasMood(mood string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var moods map[string]string
	if s := m.songs[m.wantSong]; s != nil {
		moods = s.Moods
	} else if d, ok := m.defs[m.wantSong]; ok {
		moods = d.Moods
	}
	_, ok := moods[mood]
	return ok
}

func (m *Modular) Current() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.songs[m.song]
	if s == nil {
		return "", ""
	}
	return s.Name, s.Moods[m.mood(s, m.cur)]
}

func (m *Modular) mood(s *loadedSong, want string) string {
	if p := s.mods[want]; p != nil && p.playable() {
		if _, ok := s.Moods[want]; ok || s.Track() {
			return want
		}
	}
	if p := s.mods[Neutral]; p != nil && p.playable() {
		return Neutral
	}
	for mood, p := range s.mods {
		if p.playable() {
			return mood
		}
	}
	return Neutral
}

func (m *Modular) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	frames := len(p) / 8
	for f := range frames {
		if m.stop {
			m.fadeOut()
			m.song, m.cur, m.pos, m.stop = "", Neutral, 0, false
		}
		if (m.wantSong != m.song || m.jump) && m.songs[m.wantSong] != nil {
			m.changeSong()
		}
		s := m.songs[m.song]
		var l, r float32
		next := true
		if s != nil {
			if m.pos >= s.frames() {
				m.advance(s)
				s = m.songs[m.song]
			}
		}
		if s != nil {
			if pcm := s.mods[m.mood(s, m.cur)]; pcm != nil {
				if i := 2 * m.pos; i+1 < pcm.ready() {
					l, r = float32(pcm.data[i])/32768, float32(pcm.data[i+1])/32768
				} else if !pcm.done.Load() {
					next = false
				}
			}
		}
		live := m.tails[:0]
		for _, t := range m.tails {
			if t.pos+1 >= t.pcm.size() || (t.fade > 0 && t.left <= 0) {
				continue
			}
			g := float32(1)
			if t.fade > 0 {
				g = float32(t.left) / float32(t.fade)
				t.left--
			}
			if t.pos+1 < t.pcm.ready() {
				l += float32(t.pcm.data[t.pos]) / 32768 * g
				r += float32(t.pcm.data[t.pos+1]) / 32768 * g
			}
			t.pos += 2
			live = append(live, t)
		}
		clear(m.tails[len(live):])
		m.tails = live
		if next {
			m.pos++
		}
		binary.LittleEndian.PutUint32(p[f*8:], math.Float32bits(clamp(l)))
		binary.LittleEndian.PutUint32(p[f*8+4:], math.Float32bits(clamp(r)))
	}
	return frames * 8, nil
}

func (m *Modular) changeSong() {
	m.fadeOut()
	m.song = m.wantSong
	m.cur = m.mood(m.songs[m.song], m.want)
	m.pos = 0
	m.jump = false
	m.trim()
}

func (m *Modular) fadeOut() {
	if s := m.songs[m.song]; s != nil {
		if pcm := s.mods[m.mood(s, m.cur)]; pcm != nil && 2*m.pos < pcm.size() {
			m.tails = append(m.tails, tail{pcm: pcm, pos: 2 * m.pos, fade: songFade, left: songFade})
		}
	}
	for i := range m.tails {
		if m.tails[i].fade == 0 {
			m.tails[i].fade, m.tails[i].left = songFade, songFade
		}
	}
}

func (m *Modular) advance(s *loadedSong) {
	frames := s.frames()
	if m.onceSong != "" && m.song == m.onceSong && !m.jump {
		if pcm := s.mods[m.mood(s, m.cur)]; pcm != nil && pcm.size() > 2*frames {
			m.tails = append(m.tails, tail{pcm: pcm, pos: 2 * frames})
		}
		m.song, m.wantSong, m.cur, m.want, m.pos, m.last, m.onceSong = "", "", Neutral, Neutral, 0, false, ""
		return
	}
	if s.Track() {
		m.cur = Neutral
		m.pos = min(int(s.LoopStart*SampleRate), max(0, frames-1))
		return
	}
	if pcm := s.mods[m.mood(s, m.cur)]; pcm != nil && pcm.size() > 2*frames {
		m.tails = append(m.tails, tail{pcm: pcm, pos: 2 * frames})
	}
	if m.last && m.oneShot[m.cur] {
		m.song, m.wantSong, m.cur, m.want, m.pos, m.last = "", "", Neutral, Neutral, 0, false
		return
	}
	next := m.mood(s, m.want)
	if m.oneShot[m.cur] && next == m.cur {
		next = Neutral
		m.want = Neutral
	}
	m.cur = next
	m.pos = 0
}

func clamp(v float32) float32 {
	return max(-1, min(1, v))
}

func Decode(b []byte) ([]int16, error) { return decode(b) }

func AssetPath(file string) string {
	if file == "" {
		return ""
	}
	return "assets/" + file
}

func Relearn(fsys fs.FS, songs []Song) {
	m := current()
	if m == nil {
		return
	}
	m.mu.Lock()
	budget := m.budget
	m.mu.Unlock()
	m.Lazy(fsys, songs, budget)
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.songs {
		if name != m.song && name != m.wantSong && !m.loading[name] {
			delete(m.songs, name)
		}
	}
	for name := range m.kept {
		m.fetch(name, Neutral)
	}
}
