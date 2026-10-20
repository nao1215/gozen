package sound

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestClipDecodeAndFailures(t *testing.T) {
	// Original two-frame stereo PCM WAV, generated entirely in the test.
	b := make([]byte, 52)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 44)
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 2)
	binary.LittleEndian.PutUint32(b[24:], SampleRate)
	binary.LittleEndian.PutUint32(b[28:], SampleRate*4)
	binary.LittleEndian.PutUint16(b[32:], 4)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 8)
	binary.LittleEndian.PutUint16(b[44:], 16384)
	fsys := fstest.MapFS{"tone.wav": {Data: b}, "bad.ogg": {Data: []byte("invalid")}}
	c, err := Load(fsys, "tone.wav")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.pcm) != 16 {
		t.Fatalf("PCM bytes %d", len(c.pcm))
	}
	if _, err := c.Play(nil); err == nil {
		t.Fatal("accepted nil audio context")
	}
	if _, err := Load(fsys, "missing.wav"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if c, err := Load(fsys, "bad.ogg"); err == nil || c != nil {
		t.Fatal("invalid stream accepted")
	}
	if _, err := Load(fsys, "tone.mp3"); err == nil {
		t.Fatal("unsupported extension accepted")
	}
}
