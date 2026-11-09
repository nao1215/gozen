package sound

import (
	"encoding/binary"
	"io"
	"io/fs"
	"math"
)

type recorder struct {
	w      io.Writer
	tps    int
	voices []voice
	buf    []byte
	err    error
}

type voice struct {
	b   []byte
	pos int
}

func (session *Session) InitRecord(w io.Writer, tps int) {
	session.rec = &recorder{w: w, tps: tps}
	session.makeEffects(false)
}

func recordBGM(m *Modular, fsys fs.FS, songs []Song) {
	for _, s := range songs {
		m.AddSong(s, s.load(fsys))
	}
}

func (session *Session) Tick() error {
	r := session.rec
	if r == nil {
		return nil
	}
	if r.err != nil {
		return r.err
	}
	frames := SampleRate / r.tps
	if len(r.buf) != frames*8 {
		r.buf = make([]byte, frames*8)
	}
	clear(r.buf)
	if m := session.current(); m != nil {
		if _, err := m.Read(r.buf); err != nil {
			return err
		}
	}

	session.mu.Lock()
	vol, se := float32(session.bgmVolume()), float32(session.seVolume())
	session.mu.Unlock()
	for f := range frames * 2 {
		v := math.Float32frombits(binary.LittleEndian.Uint32(r.buf[f*4:])) * vol
		for i := range r.voices {
			vc := &r.voices[i]
			if p := vc.pos + f*4; p+4 <= len(vc.b) {
				v += math.Float32frombits(binary.LittleEndian.Uint32(vc.b[p:])) * se
			}
		}
		binary.LittleEndian.PutUint32(r.buf[f*4:], math.Float32bits(clamp(v)))
	}
	live := r.voices[:0]
	for _, vc := range r.voices {
		vc.pos += frames * 8
		if vc.pos < len(vc.b) {
			live = append(live, vc)
		}
	}
	r.voices = live
	var n int
	n, r.err = r.w.Write(r.buf)
	if r.err == nil && n != len(r.buf) {
		r.err = io.ErrShortWrite
	}
	return r.err
}

func (session *Session) Pump(tps int) {
	session.mu.Lock()
	m, play := session.bgm, session.bgmPlay
	session.mu.Unlock()
	if session.rec != nil || play != nil || m == nil || tps <= 0 {
		return
	}
	frames := SampleRate / tps
	if len(session.silentBuf) != frames*8 {
		session.silentBuf = make([]byte, frames*8)
	}
	if _, err := m.Read(session.silentBuf); err != nil {
		m.Stop()
	}
}
