package sound

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestFadeInAndStop: a song played with a fade comes in over the frames while the one
// before goes out over the same frames; a stop with a fade goes out over its frames.
func TestFadeInAndStop(t *testing.T) {
	m := NewModular("a", nil)
	a, b := make([]int16, 32), make([]int16, 32)
	for i := range a {
		a[i], b[i] = 16384, 8192
	}
	m.AddSong(Song{Name: "a", File: "a.ogg"}, map[string][]int16{Neutral: a})
	m.AddSong(Song{Name: "b", File: "b.ogg"}, map[string][]int16{Neutral: b})
	m.PlaySong("a", Neutral)
	if _, err := m.Read(make([]byte, 8)); err != nil {
		t.Fatal(err)
	}
	left := func(n int) []float32 {
		out := make([]float32, n)
		p := make([]byte, 8*n)
		if _, err := m.Read(p); err != nil {
			t.Fatal(err)
		}
		for i := range n {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(p[i*8:]))
		}
		return out
	}
	near := func(got, want []float32) bool {
		for i := range want {
			if math.Abs(float64(got[i]-want[i])) > 1e-6 {
				return false
			}
		}
		return true
	}
	m.PlaySongFade("b", Neutral, 4.0/SampleRate)
	if got, want := left(5), []float32{0.5, 0.4375, 0.375, 0.3125, 0.25}; !near(got, want) {
		t.Fatalf("crossfade %v, want %v", got, want)
	}
	m.StopFade(4.0 / SampleRate)
	if got, want := left(5), []float32{0.25, 0.1875, 0.125, 0.0625, 0}; !near(got, want) {
		t.Fatalf("fade out %v, want %v", got, want)
	}
}
