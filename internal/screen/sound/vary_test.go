package sound

import (
	"encoding/binary"
	"math"
	"testing"
)

// The last output frame reads at float64(j)*r, which can round up to exactly n
// (the frame after the clip) although ceil(n/r) frames were asked for: 9 frames
// at this rate give 11 output frames and 10*r == 9.0. That frame is the clip's
// last one, not a read past it.
func TestResampleStaysOnTheClip(t *testing.T) {
	const n = 9
	src := make([]byte, n*8)
	for i := range n {
		for ch := range 2 {
			binary.LittleEndian.PutUint32(src[i*8+ch*4:], math.Float32bits(float32(i+1)))
		}
	}
	got := resample(src, 0.8999999999999999)
	if len(got) != 11*8 {
		t.Fatalf("resampled to %d bytes", len(got))
	}
	if last := math.Float32frombits(binary.LittleEndian.Uint32(got[10*8:])); last != n {
		t.Fatalf("the last frame is %v, want the clip's last (%d)", last, n)
	}
}
