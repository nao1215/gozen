package testgame

import (
	"encoding/binary"
	"math"
)

// Wav is a short tone made here, a stand-in for a recorded voice: 16-bit mono PCM at
// 44100 Hz, seconds long.
func Wav(seconds float64) []byte {
	const rate = 44100
	n := uint32(seconds * rate)
	le := binary.LittleEndian
	b := []byte("RIFF")
	b = le.AppendUint32(b, 36+n*2)
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1) // PCM
	b = le.AppendUint16(b, 1) // mono
	b = le.AppendUint32(b, rate)
	b = le.AppendUint32(b, rate*2)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = le.AppendUint32(b, n*2)
	for i := range n {
		v := int16(math.Sin(float64(i)*2*math.Pi*440/rate) * 8000)
		b = le.AppendUint16(b, uint16(v)) //nolint:gosec // A signed sample written as its two's-complement bits.
	}
	return b
}
