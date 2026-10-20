package sound

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

var (
	varyMu     sync.Mutex
	varyPitch  = 0.03
	varyVolume = 0.10
	varyWindow = 400 * time.Millisecond
	lastPlayed = map[string]time.Time{}
)

func SetVary(pitch, volume float64) {
	varyMu.Lock()
	defer varyMu.Unlock()
	varyPitch, varyVolume = max(0, pitch), max(0, volume)
}

func vary(name string, b []byte) ([]byte, float64) {
	varyMu.Lock()
	now := time.Now()
	last, seen := lastPlayed[name]
	lastPlayed[name] = now
	pitch, volume := varyPitch, varyVolume
	varyMu.Unlock()
	if !seen || now.Sub(last) > varyWindow || (pitch == 0 && volume == 0) {
		return b, 1
	}
	//nolint:gosec // Audio variation, not security-sensitive randomness.
	v := 1 + volume*(2*rand.Float64()-1)
	//nolint:gosec // Audio variation, not security-sensitive randomness.
	r := 1 + pitch*(2*rand.Float64()-1)
	return resample(b, r), v
}

func resample(b []byte, r float64) []byte {
	n := len(b) / 8
	if n < 2 || r <= 0 || math.Abs(r-1) < 1e-4 {
		return b
	}

	m := int(math.Ceil(float64(n) / r))
	out := make([]byte, m*8)
	sample := func(i, ch int) float64 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i*8+ch*4:])))
	}
	for j := range m {
		x := float64(j) * r
		i := int(x)
		f := x - float64(i)
		for ch := range 2 {
			v := sample(i, ch)*(1-f) + sample(min(i+1, n-1), ch)*f
			binary.LittleEndian.PutUint32(out[j*8+ch*4:], math.Float32bits(float32(v)))
		}
	}
	return out
}
