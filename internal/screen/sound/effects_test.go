package sound

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestBuiltInEffectsAreAllMadeAndBounded(t *testing.T) {
	made := testSession.synthEffects()
	for _, name := range Effects {
		b, ok := made[name]
		if !ok {
			t.Errorf("effect %q is listed but not made", name)
			continue
		}
		if len(b) == 0 || len(b)%8 != 0 {
			t.Errorf("effect %q has %d bytes, want whole stereo float32 frames", name, len(b))
			continue
		}
		for i := 0; i+4 <= len(b); i += 4 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
			if v < -1 || v > 1 || v != v {
				t.Errorf("effect %q sample %d out of range: %v", name, i/4, v)
				break
			}
		}
	}
	if len(made) != len(Effects) {
		t.Errorf("made %d effects, listed %d", len(made), len(Effects))
	}
}

func TestBuiltInEffectsAreDeterministic(t *testing.T) {
	a, b := testSession.synthEffects(), testSession.synthEffects()
	for _, name := range Effects {
		if !bytes.Equal(a[name], b[name]) {
			t.Errorf("effect %q differs between runs", name)
		}
	}
}

func TestEffectEnvelopes(t *testing.T) {
	if env(0, 0.1, 1) != 0 || env(0.05, 0.1, 1) != 0.5 || env(0.1, 0.1, 1) != 1 {
		t.Fatal("attack ramp")
	}
	if got := env(1.1, 0.1, 1); math.Abs(got-math.Exp(-1)) > 1e-12 {
		t.Fatalf("decay %v", got)
	}
	if square(0.1, 1) != 1 || square(0.6, 1) != -1 {
		t.Fatal("square")
	}
	if step(0) != 1 || step(-1e-9) != 0 {
		t.Fatal("step")
	}
	if math.Abs(tone(0.25, 1)-1) > 1e-12 {
		t.Fatal("tone")
	}
	// Notes start one gap apart, so before the second note only the first sounds.
	f := arpeggio([]float64{440, 660}, 0.1, 0.2, 1)
	if f(-0.01) != 0 {
		t.Fatal("arpeggio sounds before it starts")
	}
}

func TestSynthClampsToUnitRange(t *testing.T) {
	b := testSession.synth(0.002, func(float64) float64 { return 5 })
	if len(b) != 88*8 {
		t.Fatalf("length %d", len(b))
	}
	if v := math.Float32frombits(binary.LittleEndian.Uint32(b)); v != 1 {
		t.Fatalf("clamped to %v", v)
	}
	if l, r := binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:]); l != r {
		t.Fatal("effects are dual mono")
	}
}

func TestMakeEffectsGameFilesWin(t *testing.T) {
	freshSession(t)
	custom := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	testSession.mu.Lock()
	testSession.effects["cursor"] = custom
	testSession.mu.Unlock()
	testSession.makeEffects(false)
	testSession.mu.Lock()
	got, other := testSession.effects["cursor"], testSession.effects["decide"]
	testSession.mu.Unlock()
	if !bytes.Equal(got, custom) {
		t.Fatal("a game's own effect was replaced by the built-in one")
	}
	if len(other) == 0 {
		t.Fatal("built-in effects missing")
	}
}

func TestMakeEffectsInBackground(t *testing.T) {
	freshSession(t)
	testSession.makeEffects(true)
	waitFor(t, "background effects", func() bool {
		testSession.mu.Lock()
		defer testSession.mu.Unlock()
		return len(testSession.effects["fanfare"]) > SampleRate*8
	})
}

func stereo(vals ...float32) []byte {
	b := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(b[i*8:], math.Float32bits(v))
		binary.LittleEndian.PutUint32(b[i*8+4:], math.Float32bits(-v))
	}
	return b
}

func TestResample(t *testing.T) {
	src := stereo(0, 1, 2, 3)
	if got := resample(src, 1); &got[0] != &src[0] {
		t.Fatal("rate 1 must return the input")
	}
	if got := resample(src, 1+1e-6); &got[0] != &src[0] {
		t.Fatal("an inaudible change must return the input")
	}
	if got := resample(src, 0); &got[0] != &src[0] {
		t.Fatal("non-positive rate must return the input")
	}
	for _, rate := range []float64{math.NaN(), math.Inf(1), math.SmallestNonzeroFloat64} {
		if got := resample(src, rate); &got[0] != &src[0] {
			t.Fatalf("invalid or excessive rate %v must return the input", rate)
		}
	}
	if got := resample(src, math.MaxFloat64); len(got) != 8 {
		t.Fatalf("very fast rate produced %d bytes, want one frame", len(got))
	}
	one := stereo(1)
	if got := resample(one, 2); &got[0] != &one[0] {
		t.Fatal("a single frame cannot be resampled")
	}
	// Twice as fast: every other frame, half as long.
	fast := resample(src, 2)
	if len(fast)/8 != 2 {
		t.Fatalf("fast frames %d", len(fast)/8)
	}
	if l := math.Float32frombits(binary.LittleEndian.Uint32(fast[8:])); l != 2 {
		t.Fatalf("second frame left %v", l)
	}
	// Half speed: new frames between the old ones are interpolated; the channels stay mirrored.
	slow := resample(src, 0.5)
	if len(slow)/8 != 8 {
		t.Fatalf("slow frames %d", len(slow)/8)
	}
	l := math.Float32frombits(binary.LittleEndian.Uint32(slow[8:]))
	r := math.Float32frombits(binary.LittleEndian.Uint32(slow[12:]))
	if l != 0.5 || r != -0.5 {
		t.Fatalf("interpolated frame %v %v", l, r)
	}
}

func TestVaryOnlyChangesQuickRepeats(t *testing.T) {
	freshSession(t)
	t.Cleanup(func() { testSession.SetVary(0.03, 0.10) })
	src := stereo(0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	testSession.SetVary(0.2, 0.1)
	b, k := testSession.vary("a", src)
	if k != 1 || len(b) != len(src) {
		t.Fatal("first play must be untouched")
	}
	changed := false
	for range 20 {
		b, k = testSession.vary("a", src)
		if k < 0.9-1e-9 || k > 1.1+1e-9 {
			t.Fatalf("volume factor %v out of range", k)
		}
		if len(b) != len(src) {
			changed = true
		}
	}
	if !changed {
		t.Fatal("repeated plays never varied")
	}
	if b, k := testSession.vary("other", src); k != 1 || len(b) != len(src) {
		t.Fatal("another effect's first play must be untouched")
	}
	testSession.SetVary(-1, -1)
	if b, k := testSession.vary("a", src); k != 1 || len(b) != len(src) {
		t.Fatal("variation disabled must keep the effect as is")
	}
	if testSession.varyPitch != 0 || testSession.varyVolume != 0 {
		t.Fatal("negative variation is clamped to zero")
	}
}
