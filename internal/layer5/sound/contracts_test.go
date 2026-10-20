package sound

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

func TestTrackLoopAndOneShot(t *testing.T) {
	m := NewModular("tone", nil)
	m.AddSong(Song{Name: "tone", File: "tone.ogg"}, map[string][]int16{Neutral: {16384, -16384, 8192, -8192}})
	m.PlaySong("tone", Neutral)
	p := make([]byte, 24)
	n, err := m.Read(p)
	if err != nil || n != len(p) {
		t.Fatalf("read %d %v", n, err)
	}
	sample := func(i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(p[i*4:])) }
	if sample(0) != 0.5 || sample(1) != -0.5 || sample(4) != 0.5 {
		t.Fatalf("loop samples %v %v %v", sample(0), sample(1), sample(4))
	}
	once := NewModular("tone", nil)
	once.AddSong(Song{Name: "tone", File: "tone.ogg"}, map[string][]int16{Neutral: {16384, 16384}})
	once.PlayOnce("tone", Neutral)
	if _, err := once.Read(p); err != nil {
		t.Fatal(err)
	}
	if once.Wanted() != "" {
		t.Fatal("one-shot did not stop")
	}
	if n, err := once.Read(p[:7]); err != nil || n != 0 {
		t.Fatalf("partial frame %d %v", n, err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }
func TestRecordingShortWriteAndReset(t *testing.T) {
	InitRecord(shortWriter{}, 60)
	if err := Tick(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Tick(); err != nil {
		t.Fatal("recorder retained failure")
	}
}

func TestPCMReadFailure(t *testing.T) {
	want := errors.New("broken stream")
	p := &pcm{data: make([]int16, 10)}
	if err := p.fill(errorReader{want}, nil); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if !p.done.Load() {
		t.Fatal("failed decode never completed")
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
