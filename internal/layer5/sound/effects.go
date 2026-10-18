package sound

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

var effectPlayers []*audio.Player

var effects = map[string][]byte{}

var Effects = []string{"cursor", "decide", "cancel", "buzzer", "hit", "crit", "magic", "heal", "buff", "debuff", "item", "fall", "win", "escape", "step",

	"cutin", "encounter", "boss", "stairs", "chest",

	"blast", "miss",

	"switch", "solve", "push", "thud", "unlock", "door", "punch", "tink", "break", "throw", "lift", "hurt",
	"gameover", "pop", "boom", "charge", "shoot", "key", "heart", "fanfare",

	"levelup",

	"treasure"}

func Play(name string) {
	b := effects[name]
	if rec != nil && b != nil {
		rec.voices = append(rec.voices, voice{b: b})
		return
	}
	mu.Lock()
	off, vol := muted, seVolume()
	mu.Unlock()
	if ctx == nil || off || b == nil {
		return
	}
	b, k := vary(name, b)
	p := ctx.NewPlayerF32FromBytes(b)
	p.SetVolume(min(1, vol*k))
	p.Play()
	effectPlayers = slices.DeleteFunc(effectPlayers, func(p *audio.Player) bool { return !p.IsPlaying() })
	effectPlayers = append(effectPlayers, p)
}

func synth(seconds float64, f func(t float64) float64) []byte {
	n := int(seconds * SampleRate)
	b := make([]byte, n*8)
	for i := range n {
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

func makeEffects() {
	//nolint:gosec // Deterministic synthetic audio noise, not a secret.
	rng := rand.New(rand.NewPCG(11, 12))
	noise := func() float64 { return rng.Float64()*2 - 1 }
	effects["cursor"] = synth(0.05, func(t float64) float64 { return tone(t, 1320) * env(t, 0.002, 0.012) * 0.25 })
	effects["decide"] = synth(0.18, func(t float64) float64 {
		return (tone(t, 880)*step(0.06-t) + tone(t, 1320)*step(t-0.06)) * env(t, 0.003, 0.08) * 0.3
	})
	effects["cancel"] = synth(0.14, func(t float64) float64 {
		return (tone(t, 660)*step(0.05-t) + tone(t, 440)*step(t-0.05)) * env(t, 0.003, 0.06) * 0.3
	})
	effects["buzzer"] = synth(0.25, func(t float64) float64 { return square(t, 140) * env(t, 0.005, 0.12) * 0.18 })
	lp := 0.0
	effects["hit"] = synth(0.2, func(t float64) float64 {
		lp += (noise() - lp) * 0.35
		return (lp*0.8 + tone(t, 90*math.Exp(-t*8))*0.7) * env(t, 0.001, 0.05)
	})
	effects["crit"] = synth(0.35, func(t float64) float64 {
		return (noise()*0.6 + tone(t, 70*math.Exp(-t*5))*0.9 + tone(t, 1800)*0.2) * env(t, 0.001, 0.09)
	})
	effects["magic"] = synth(0.6, func(t float64) float64 {
		f := 600 + 900*t
		return (tone(t, f)*0.5 + tone(t, f*1.5)*0.3) * env(t, 0.02, 0.2) * (0.7 + 0.3*tone(t, 18)) * 0.35
	})
	effects["heal"] = synth(0.9, arpeggio([]float64{783.99, 987.77, 1174.66, 1567.98}, 0.07, 0.25, 0.16))
	effects["buff"] = synth(0.5, func(t float64) float64 {
		return tone(t, 440*math.Pow(2, t*2)) * env(t, 0.01, 0.2) * 0.3
	})
	effects["debuff"] = synth(0.5, func(t float64) float64 {
		return tone(t, 660*math.Pow(2, -t*2)) * env(t, 0.01, 0.2) * 0.3
	})
	effects["item"] = synth(1.2, arpeggio([]float64{1046.5, 1318.5, 1568, 2093}, 0.08, 0.2, 0.14))
	effects["fall"] = synth(0.5, func(t float64) float64 {
		return math.Sin(2*math.Pi*110*math.Exp(-t*3)*t) * env(t, 0.003, 0.15) * 0.7
	})
	effects["win"] = synth(1.4, arpeggio([]float64{587.33, 739.99, 880, 1174.66}, 0.09, 0.35, 0.18))
	effects["escape"] = synth(0.4, func(t float64) float64 {
		lp += (noise() - lp) * (0.1 + t)
		return lp * math.Sin(math.Pi*t/0.4) * 0.6
	})
	effects["step"] = synth(0.12, func(t float64) float64 {
		lp += (noise() - lp) * 0.2
		return lp * env(t, 0.005, 0.03) * 0.5
	})
	effects["cutin"] = synth(0.9, func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + 0.4*math.Min(1, t*3))
		whoosh := lp * math.Sin(math.Pi*math.Min(1, t/0.45)) * 0.7 * step(0.45-t)
		ring := arpeggio([]float64{1318.5, 1760, 2637}, 0.05, 0.3, 0.12)(math.Max(0, t-0.3)) * step(t-0.3)
		return whoosh + ring
	})
	effects["encounter"] = synth(0.8, func(t float64) float64 {
		f := 220 * math.Pow(2, t*3)
		glass := (tone(t, f) + tone(t, f*1.51)*0.5) * env(t, 0.01, 0.25) * 0.22
		lp += (noise() - lp) * 0.6
		crack := lp * env(math.Max(0, t-0.35), 0.001, 0.08) * step(t-0.35) * 0.8
		return glass + crack
	})
	effects["boss"] = synth(1.6, func(t float64) float64 {
		lp += (noise() - lp) * 0.03
		drum := tone(t, 50*math.Exp(-t*1.5)) * env(t, 0.002, 0.5)
		low := (square(t, 55)*0.15 + tone(t, 82.4)*0.3) * env(t, 0.05, 0.6)
		return (drum*0.8 + low + lp*1.5*env(t, 0.01, 0.4)) * 0.6
	})
	effects["stairs"] = synth(0.7, func(t float64) float64 {
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
	effects["chest"] = synth(1.0, func(t float64) float64 {
		creak := square(t, 180+t*300) * env(t, 0.01, 0.08) * 0.08 * step(0.2-t)
		return creak + arpeggio([]float64{1046.5, 1568, 2093, 2637}, 0.06, 0.25, 0.13)(math.Max(0, t-0.15))*step(t-0.15)
	})
	effects["blast"] = synth(0.45, func(t float64) float64 {
		lp += (noise() - lp) * (0.5 - 0.4*math.Min(1, t*3))
		return (lp*0.7 + tone(t, 120*math.Exp(-t*4))*0.6 + tone(t, 900+400*t)*0.15) * env(t, 0.002, 0.12)
	})
	effects["miss"] = synth(0.22, func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + t*2)
		return lp * math.Sin(math.Pi*t/0.22) * 0.5
	})
	makeActionEffects(noise)
}

func makeActionEffects(noise func() float64) {
	lp := 0.0
	sweep := func(t, f0, f1, dur float64) float64 {
		k := math.Log(f1/f0) / dur
		return math.Sin(2 * math.Pi * f0 * (math.Exp(k*t) - 1) / k)
	}
	effects["switch"] = synth(0.16, func(t float64) float64 {
		return (square(t, 1200)*step(0.04-t) + square(t, 1800)*step(t-0.04)) * env(t, 0.002, 0.05) * 0.12
	})

	effects["solve"] = synth(1.6, arpeggio([]float64{783.99, 739.99, 622.25, 440, 415.30, 659.25, 830.61, 1046.5}, 0.11, 0.35, 0.13))
	effects["push"] = synth(0.32, func(t float64) float64 {
		lp += (noise() - lp) * 0.05
		return (lp*1.6 + tone(t, 55)*0.3) * math.Sin(math.Pi*t/0.32) * 0.8
	})
	effects["thud"] = synth(0.25, func(t float64) float64 {
		lp += (noise() - lp) * 0.15
		return (lp*0.6 + tone(t, 70*math.Exp(-t*6))) * env(t, 0.002, 0.06) * 0.8
	})
	effects["unlock"] = synth(0.7, func(t float64) float64 {
		v := (tone(t, 2600) + tone(t, 3900)*0.5) * env(t, 0.001, 0.015) * 0.3
		u := t - 0.12
		if u > 0 {
			v += (tone(u, 2200) + tone(u, 3300)*0.5) * env(u, 0.001, 0.02) * 0.3
		}
		return v + arpeggio([]float64{880, 1108.73, 1318.51}, 0.07, 0.2, 0.1)(math.Max(0, t-0.25))*step(t-0.25)
	})
	effects["door"] = synth(0.6, func(t float64) float64 {
		lp += (noise() - lp) * 0.08
		return (lp + square(t, 60+t*80)*0.15) * math.Sin(math.Pi*t/0.6) * 0.7
	})
	effects["punch"] = synth(0.13, func(t float64) float64 {
		lp += (noise() - lp) * (0.1 + t*4)
		return lp * math.Sin(math.Pi*t/0.13) * 0.7
	})
	effects["tink"] = synth(0.18, func(t float64) float64 {
		return (tone(t, 2400) + tone(t, 3610)*0.6) * env(t, 0.001, 0.04) * 0.25
	})
	effects["break"] = synth(0.4, func(t float64) float64 {
		lp += (noise() - lp) * 0.6
		v := lp * env(t, 0.001, 0.08) * 0.8
		for i, at := range []float64{0.05, 0.11, 0.19} {
			if u := t - at; u > 0 {
				v += tone(u, 1800+float64(i)*700) * env(u, 0.001, 0.02) * 0.2
			}
		}
		return v
	})
	effects["throw"] = synth(0.25, func(t float64) float64 {
		lp += (noise() - lp) * (0.05 + t)
		return lp * math.Sin(math.Pi*t/0.25) * 0.6
	})
	effects["lift"] = synth(0.14, func(t float64) float64 { return sweep(t, 300, 700, 0.14) * env(t, 0.005, 0.07) * 0.3 })
	effects["hurt"] = synth(0.3, func(t float64) float64 {
		f := 600 * math.Exp(-t*4)
		return square(t, f) * env(t, 0.002, 0.12) * 0.16
	})
	effects["gameover"] = synth(2.0, arpeggio([]float64{659.25, 587.33, 523.25, 493.88, 392}, 0.22, 0.5, 0.15))
	effects["pop"] = synth(0.2, func(t float64) float64 {
		lp += (noise() - lp) * 0.3
		return (sweep(t, 400, 1400, 0.2)*0.4 + lp*0.3) * env(t, 0.002, 0.06)
	})
	effects["boom"] = synth(1.4, func(t float64) float64 {
		lp += (noise() - lp) * 0.04
		return (lp*2.2 + tone(t, 45*math.Exp(-t))*0.6) * env(t, 0.005, 0.4)
	})
	effects["charge"] = synth(0.35, func(t float64) float64 { return square(t, 180+t*900) * env(t, 0.01, 0.2) * 0.1 })
	effects["shoot"] = synth(0.18, func(t float64) float64 { return sweep(t, 1400, 300, 0.18) * env(t, 0.002, 0.08) * 0.3 })
	effects["key"] = synth(0.6, arpeggio([]float64{1567.98, 2093}, 0.08, 0.15, 0.14))
	effects["heart"] = synth(0.5, arpeggio([]float64{1046.5, 1318.51, 1567.98}, 0.05, 0.12, 0.12))
	effects["levelup"] = synth(1.1, func(t float64) float64 {
		v := arpeggio([]float64{523.25, 659.25, 783.99, 1046.5}, 0.07, 0.3, 0.13)(t)
		if u := t - 0.3; u > 0 {
			v += (tone(u, 1046.5) + tone(u, 1318.51)*0.6 + tone(u, 1567.98)*0.5) * env(u, 0.01, 0.4) * 0.09
		}
		return v
	})
	effects["treasure"] = synth(1.1, func(t float64) float64 {
		v := arpeggio([]float64{523.25, 659.25, 783.99, 1046.5, 1318.51}, 0.055, 0.14, 0.12)(t)
		if u := t - 0.3; u > 0 {
			v += (tone(u, 523.25) + tone(u, 783.99)*0.7 + tone(u, 1046.5)*0.5) * env(u, 0.01, 0.3) * 0.08
		}
		return v
	})
	effects["fanfare"] = synth(1.8, func(t float64) float64 {
		v := arpeggio([]float64{440, 554.37, 659.25, 880}, 0.12, 0.6, 0.12)(t)
		if u := t - 0.55; u > 0 {
			v += (tone(u, 880) + tone(u, 1108.73)*0.6 + tone(u, 1318.51)*0.5) * env(u, 0.01, 0.5) * 0.1
		}
		return v
	})
}
