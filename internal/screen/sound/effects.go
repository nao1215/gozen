package sound

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// builtinEffectSeconds is shared by synthesis and silent simulation.
var builtinEffectSeconds = map[string]float64{
	"cursor":    0.05,
	"decide":    0.18,
	"cancel":    0.14,
	"buzzer":    0.25,
	"hit":       0.2,
	"crit":      0.35,
	"magic":     0.6,
	"heal":      0.9,
	"buff":      0.5,
	"debuff":    0.5,
	"item":      1.2,
	"fall":      0.5,
	"win":       1.4,
	"escape":    0.4,
	"step":      0.12,
	"cutin":     0.9,
	"encounter": 0.8,
	"boss":      1.6,
	"stairs":    0.7,
	"chest":     1.0,
	"blast":     0.45,
	"miss":      0.22,
	"switch":    0.16,
	"solve":     1.6,
	"push":      0.32,
	"thud":      0.25,
	"unlock":    0.7,
	"door":      0.6,
	"punch":     0.13,
	"tink":      0.18,
	"break":     0.4,
	"throw":     0.25,
	"lift":      0.14,
	"hurt":      0.3,
	"gameover":  2.0,
	"pop":       0.2,
	"boom":      1.4,
	"charge":    0.35,
	"shoot":     0.18,
	"key":       0.6,
	"heart":     0.5,
	"levelup":   1.1,
	"treasure":  1.1,
	"tip":       0.9,
	"fanfare":   1.8,
}

var Effects = []string{
	"cursor", "decide", "cancel", "buzzer", "hit", "crit", "magic", "heal", "buff", "debuff", "item", "fall", "win", "escape", "step",

	"cutin", "encounter", "boss", "stairs", "chest",

	"blast", "miss",

	"switch", "solve", "push", "thud", "unlock", "door", "punch", "tink", "break", "throw", "lift", "hurt",
	"gameover", "pop", "boom", "charge", "shoot", "key", "heart", "fanfare",

	"levelup",

	"treasure",

	"tip",
}

func (session *Session) Play(name string) {
	session.mu.Lock()
	b := session.effects[name]
	off, vol := session.muted, session.seVolume()
	session.mu.Unlock()
	if session.rec != nil && b != nil {
		session.rec.voices = append(session.rec.voices, voice{b: b})
		return
	}
	if session.ctx == nil || off || b == nil {
		return
	}
	b, k := session.vary(name, b)
	p := session.ctx.NewPlayerF32FromBytes(b)
	p.SetVolume(min(1, vol*k))
	p.Play()
	session.effectPlayers = slices.DeleteFunc(session.effectPlayers, func(p *audio.Player) bool { return !p.IsPlaying() })
	session.effectPlayers = append(session.effectPlayers, p)
}

func (session *Session) synth(seconds float64, f func(t float64) float64) []byte {
	n := int(seconds * SampleRate)
	b := make([]byte, n*8)
	for i := range n {
		if i%1024 == 0 {
			session.synthPace.Breathe()
		}
		v := float32(max(-1, min(1, f(float64(i)/SampleRate))))
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(b[i*8:], bits)
		binary.LittleEndian.PutUint32(b[i*8+4:], bits)
	}
	return b
}

func env(t, attack, decay float64) float64 {
	if t < attack {
		return t / attack
	}
	return math.Exp(-(t - attack) / decay)
}

func tone(t, freq float64) float64 { return math.Sin(2 * math.Pi * freq * t) }

func square(t, freq float64) float64 {
	if math.Mod(t*freq, 1) < 0.5 {
		return 1
	}
	return -1
}

func step(x float64) float64 {
	if x >= 0 {
		return 1
	}
	return 0
}

func arpeggio(notes []float64, gap, decay, vol float64) func(float64) float64 {
	return func(t float64) float64 {
		v := 0.0
		for i, fr := range notes {
			st := float64(i) * gap
			if t >= st {
				u := t - st
				v += (tone(u, fr) + 0.3*tone(u, fr*2.01)) * env(u, 0.004, decay)
			}
		}
		return v * vol
	}
}

// makeEffects makes the built-in effects: in a browser in the background, so that the
// first frames do not wait for them (an effect played before is not heard).
func (session *Session) makeEffects(background bool) {
	session.mu.Lock()
	gen := session.effectsGen
	session.mu.Unlock()
	run := func() {
		made := session.synthEffects()
		session.mu.Lock()
		defer session.mu.Unlock()
		if gen != session.effectsGen {
			return
		}
		for name, b := range made {
			if _, ok := session.effects[name]; !ok {
				session.effects[name] = b
			}
		}
	}
	if background {
		go run()
		return
	}
	run()
}

func (session *Session) synthEffects() map[string][]byte {
	effects := map[string][]byte{}
	//nolint:gosec // Deterministic synthetic audio noise, not a secret.
	rng := rand.New(rand.NewPCG(11, 12))
	noise := func() float64 { return rng.Float64()*2 - 1 }
	effects["cursor"] = session.synth(builtinEffectSeconds["cursor"], func(t float64) float64 { return tone(t, 1320) * env(t, 0.002, 0.012) * 0.25 })
	effects["decide"] = session.synth(builtinEffectSeconds["decide"], func(t float64) float64 {
		return (tone(t, 880)*step(0.06-t) + tone(t, 1320)*step(t-0.06)) * env(t, 0.003, 0.08) * 0.3
	})
	effects["cancel"] = session.synth(builtinEffectSeconds["cancel"], func(t float64) float64 {
		return (tone(t, 660)*step(0.05-t) + tone(t, 440)*step(t-0.05)) * env(t, 0.003, 0.06) * 0.3
	})
	effects["buzzer"] = session.synth(builtinEffectSeconds["buzzer"], func(t float64) float64 { return square(t, 140) * env(t, 0.005, 0.12) * 0.18 })
	lp := 0.0
	effects["hit"] = session.synth(builtinEffectSeconds["hit"], func(t float64) float64 {
		lp += (noise() - lp) * 0.35
		return (lp*0.8 + tone(t, 90*math.Exp(-t*8))*0.7) * env(t, 0.001, 0.05)
	})
	effects["crit"] = session.synth(builtinEffectSeconds["crit"], func(t float64) float64 {
		return (noise()*0.6 + tone(t, 70*math.Exp(-t*5))*0.9 + tone(t, 1800)*0.2) * env(t, 0.001, 0.09)
	})
	effects["magic"] = session.synth(builtinEffectSeconds["magic"], func(t float64) float64 {
		f := 600 + 900*t
		return (tone(t, f)*0.5 + tone(t, f*1.5)*0.3) * env(t, 0.02, 0.2) * (0.7 + 0.3*tone(t, 18)) * 0.35
	})
	effects["heal"] = session.synth(builtinEffectSeconds["heal"], arpeggio([]float64{783.99, 987.77, 1174.66, 1567.98}, 0.07, 0.25, 0.16))
	effects["buff"] = session.synth(builtinEffectSeconds["buff"], func(t float64) float64 {
		return tone(t, 440*math.Pow(2, t*2)) * env(t, 0.01, 0.2) * 0.3
	})
	effects["debuff"] = session.synth(builtinEffectSeconds["debuff"], func(t float64) float64 {
		return tone(t, 660*math.Pow(2, -t*2)) * env(t, 0.01, 0.2) * 0.3
	})
	effects["item"] = session.synth(builtinEffectSeconds["item"], arpeggio([]float64{1046.5, 1318.5, 1568, 2093}, 0.08, 0.2, 0.14))
	effects["fall"] = session.synth(builtinEffectSeconds["fall"], func(t float64) float64 {
		return math.Sin(2*math.Pi*110*math.Exp(-t*3)*t) * env(t, 0.003, 0.15) * 0.7
	})
	effects["win"] = session.synth(builtinEffectSeconds["win"], arpeggio([]float64{587.33, 739.99, 880, 1174.66}, 0.09, 0.35, 0.18))
	effects["escape"] = session.synth(builtinEffectSeconds["escape"], func(t float64) float64 {
		lp += (noise() - lp) * (0.1 + t)
		return lp * math.Sin(math.Pi*t/0.4) * 0.6
	})
	effects["step"] = session.synth(builtinEffectSeconds["step"], func(t float64) float64 {
		lp += (noise() - lp) * 0.2
		return lp * env(t, 0.005, 0.03) * 0.5
	})
	effects["cutin"] = session.synth(builtinEffectSeconds["cutin"], func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + 0.4*math.Min(1, t*3))
		whoosh := lp * math.Sin(math.Pi*math.Min(1, t/0.45)) * 0.7 * step(0.45-t)
		ring := arpeggio([]float64{1318.5, 1760, 2637}, 0.05, 0.3, 0.12)(math.Max(0, t-0.3)) * step(t-0.3)
		return whoosh + ring
	})
	effects["encounter"] = session.synth(builtinEffectSeconds["encounter"], func(t float64) float64 {
		f := 220 * math.Pow(2, t*3)
		glass := (tone(t, f) + tone(t, f*1.51)*0.5) * env(t, 0.01, 0.25) * 0.22
		lp += (noise() - lp) * 0.6
		crack := lp * env(math.Max(0, t-0.35), 0.001, 0.08) * step(t-0.35) * 0.8
		return glass + crack
	})
	effects["boss"] = session.synth(builtinEffectSeconds["boss"], func(t float64) float64 {
		lp += (noise() - lp) * 0.03
		drum := tone(t, 50*math.Exp(-t*1.5)) * env(t, 0.002, 0.5)
		low := (square(t, 55)*0.15 + tone(t, 82.4)*0.3) * env(t, 0.05, 0.6)
		return (drum*0.8 + low + lp*1.5*env(t, 0.01, 0.4)) * 0.6
	})
	effects["stairs"] = session.synth(builtinEffectSeconds["stairs"], func(t float64) float64 {
		v := 0.0
		for i := range 4 {
			u := t - float64(i)*0.15
			if u > 0 {
				lp += (noise() - lp) * 0.2
				v += (lp*0.5 + tone(u, 120-float64(i)*12)*0.5) * env(u, 0.003, 0.04)
			}
		}
		return v * 0.6
	})
	effects["chest"] = session.synth(builtinEffectSeconds["chest"], func(t float64) float64 {
		creak := square(t, 180+t*300) * env(t, 0.01, 0.08) * 0.08 * step(0.2-t)
		return creak + arpeggio([]float64{1046.5, 1568, 2093, 2637}, 0.06, 0.25, 0.13)(math.Max(0, t-0.15))*step(t-0.15)
	})
	effects["blast"] = session.synth(builtinEffectSeconds["blast"], func(t float64) float64 {
		lp += (noise() - lp) * (0.5 - 0.4*math.Min(1, t*3))
		return (lp*0.7 + tone(t, 120*math.Exp(-t*4))*0.6 + tone(t, 900+400*t)*0.15) * env(t, 0.002, 0.12)
	})
	effects["miss"] = session.synth(builtinEffectSeconds["miss"], func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + t*2)
		return lp * math.Sin(math.Pi*t/0.22) * 0.5
	})
	session.makeActionEffects(effects, noise)
	return effects
}

func (session *Session) makeActionEffects(effects map[string][]byte, noise func() float64) {
	lp := 0.0
	sweep := func(t, f0, f1, dur float64) float64 {
		k := math.Log(f1/f0) / dur
		return math.Sin(2 * math.Pi * f0 * (math.Exp(k*t) - 1) / k)
	}
	effects["switch"] = session.synth(builtinEffectSeconds["switch"], func(t float64) float64 {
		return (square(t, 1200)*step(0.04-t) + square(t, 1800)*step(t-0.04)) * env(t, 0.002, 0.05) * 0.12
	})

	effects["solve"] = session.synth(builtinEffectSeconds["solve"], arpeggio([]float64{783.99, 739.99, 622.25, 440, 415.30, 659.25, 830.61, 1046.5}, 0.11, 0.35, 0.13))
	effects["push"] = session.synth(builtinEffectSeconds["push"], func(t float64) float64 {
		lp += (noise() - lp) * 0.05
		return (lp*1.6 + tone(t, 55)*0.3) * math.Sin(math.Pi*t/0.32) * 0.8
	})
	effects["thud"] = session.synth(builtinEffectSeconds["thud"], func(t float64) float64 {
		lp += (noise() - lp) * 0.15
		return (lp*0.6 + tone(t, 70*math.Exp(-t*6))) * env(t, 0.002, 0.06) * 0.8
	})
	effects["unlock"] = session.synth(builtinEffectSeconds["unlock"], func(t float64) float64 {
		v := (tone(t, 2600) + tone(t, 3900)*0.5) * env(t, 0.001, 0.015) * 0.3
		u := t - 0.12
		if u > 0 {
			v += (tone(u, 2200) + tone(u, 3300)*0.5) * env(u, 0.001, 0.02) * 0.3
		}
		return v + arpeggio([]float64{880, 1108.73, 1318.51}, 0.07, 0.2, 0.1)(math.Max(0, t-0.25))*step(t-0.25)
	})
	effects["door"] = session.synth(builtinEffectSeconds["door"], func(t float64) float64 {
		lp += (noise() - lp) * 0.08
		return (lp + square(t, 60+t*80)*0.15) * math.Sin(math.Pi*t/0.6) * 0.7
	})
	effects["punch"] = session.synth(builtinEffectSeconds["punch"], func(t float64) float64 {
		lp += (noise() - lp) * (0.1 + t*4)
		return lp * math.Sin(math.Pi*t/0.13) * 0.7
	})
	effects["tink"] = session.synth(builtinEffectSeconds["tink"], func(t float64) float64 {
		return (tone(t, 2400) + tone(t, 3610)*0.6) * env(t, 0.001, 0.04) * 0.25
	})
	effects["break"] = session.synth(builtinEffectSeconds["break"], func(t float64) float64 {
		lp += (noise() - lp) * 0.6
		v := lp * env(t, 0.001, 0.08) * 0.8
		for i, at := range []float64{0.05, 0.11, 0.19} {
			if u := t - at; u > 0 {
				v += tone(u, 1800+float64(i)*700) * env(u, 0.001, 0.02) * 0.2
			}
		}
		return v
	})
	effects["throw"] = session.synth(builtinEffectSeconds["throw"], func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + t)
		return lp * math.Sin(math.Pi*t/0.25) * 0.6
	})
	effects["lift"] = session.synth(builtinEffectSeconds["lift"], func(t float64) float64 { return sweep(t, 300, 700, 0.14) * env(t, 0.005, 0.07) * 0.3 })
	effects["hurt"] = session.synth(builtinEffectSeconds["hurt"], func(t float64) float64 {
		f := 600 * math.Exp(-t*4)
		return square(t, f) * env(t, 0.002, 0.12) * 0.16
	})
	effects["gameover"] = session.synth(builtinEffectSeconds["gameover"], arpeggio([]float64{659.25, 587.33, 523.25, 493.88, 392}, 0.22, 0.5, 0.15))
	effects["pop"] = session.synth(builtinEffectSeconds["pop"], func(t float64) float64 {
		lp += (noise() - lp) * 0.3
		return (sweep(t, 400, 1400, 0.2)*0.4 + lp*0.3) * env(t, 0.002, 0.06)
	})
	effects["boom"] = session.synth(builtinEffectSeconds["boom"], func(t float64) float64 {
		lp += (noise() - lp) * 0.04
		return (lp*2.2 + tone(t, 45*math.Exp(-t))*0.6) * env(t, 0.005, 0.4)
	})
	effects["charge"] = session.synth(builtinEffectSeconds["charge"], func(t float64) float64 { return square(t, 180+t*900) * env(t, 0.01, 0.2) * 0.1 })
	effects["shoot"] = session.synth(builtinEffectSeconds["shoot"], func(t float64) float64 { return sweep(t, 1400, 300, 0.18) * env(t, 0.002, 0.08) * 0.3 })
	effects["key"] = session.synth(builtinEffectSeconds["key"], arpeggio([]float64{1567.98, 2093}, 0.08, 0.15, 0.14))
	effects["heart"] = session.synth(builtinEffectSeconds["heart"], arpeggio([]float64{1046.5, 1318.51, 1567.98}, 0.05, 0.12, 0.12))
	effects["levelup"] = session.synth(builtinEffectSeconds["levelup"], func(t float64) float64 {
		v := arpeggio([]float64{523.25, 659.25, 783.99, 1046.5}, 0.07, 0.3, 0.13)(t)
		if u := t - 0.3; u > 0 {
			v += (tone(u, 1046.5) + tone(u, 1318.51)*0.6 + tone(u, 1567.98)*0.5) * env(u, 0.01, 0.4) * 0.09
		}
		return v
	})
	effects["treasure"] = session.synth(builtinEffectSeconds["treasure"], func(t float64) float64 {
		v := arpeggio([]float64{523.25, 659.25, 783.99, 1046.5, 1318.51}, 0.055, 0.14, 0.12)(t)
		if u := t - 0.3; u > 0 {
			v += (tone(u, 523.25) + tone(u, 783.99)*0.7 + tone(u, 1046.5)*0.5) * env(u, 0.01, 0.3) * 0.08
		}
		return v
	})
	// a stream's tip: two bright coin pings and a shimmer
	effects["tip"] = session.synth(builtinEffectSeconds["tip"], func(t float64) float64 {
		v := arpeggio([]float64{1567.98, 2093}, 0.07, 0.25, 0.13)(t)
		if u := t - 0.16; u > 0 {
			v += (tone(u, 2637.02) + tone(u, 3135.96)*0.5) * env(u, 0.005, 0.45) * 0.06
		}
		return v
	})
	effects["fanfare"] = session.synth(builtinEffectSeconds["fanfare"], func(t float64) float64 {
		v := arpeggio([]float64{440, 554.37, 659.25, 880}, 0.12, 0.6, 0.12)(t)
		if u := t - 0.55; u > 0 {
			v += (tone(u, 880) + tone(u, 1108.73)*0.6 + tone(u, 1318.51)*0.5) * env(u, 0.01, 0.5) * 0.1
		}
		return v
	})
}
