package play

import (
	"bufio"

	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// Bump when playback semantics change; build metadata remains opt-in.
const battleEngine = "gozen-play-v1"

type battleCapture struct {
	Version     int        `json:"version"`
	Tick        int        `json:"tick"`
	Troop       string     `json:"troop"`
	Save        []byte     `json:"save"`
	Rand        []byte     `json:"rand"`
	Fighters    []string   `json:"fighters"`
	Calm        bool       `json:"calm"`
	Auto        bool       `json:"auto"`
	AutoRead    bool       `json:"auto_read"`
	Setup       traceSetup `json:"setup"`
	Fingerprint string     `json:"fingerprint"`
	Engine      string     `json:"engine"`
	Fade        int        `json:"fade"`
}

var battleReplayChecked = []string{"say", "battle", "battle_step", "ai_decision", "battle_result"}

func (g *Game) traceBattleCapture(troop string) {
	if g.tr == nil || g.tr.enc == nil || g.tr.err != nil {
		return
	}
	c := battleCapture{Version: 1, Tick: g.tick, Troop: troop,
		Fighters: slices.Clone(g.st.Fighters), Calm: g.st.Calm, Auto: g.autoBattle,
		Setup: traceSetup{Config: g.cfg, Read: g.read, Unlocked: g.unlocked}, AutoRead: g.autoRead, Engine: battleEngine, Fade: g.fadeT}
	var err error
	c.Save, err = g.st.Save()
	if err == nil && g.pcg != nil {
		c.Rand, err = g.pcg.MarshalBinary()
	}
	if err == nil {
		c.Fingerprint, err = traceFingerprint(g.opts.FS)
	}
	if err != nil {
		g.tr.err = err
		return
	}
	g.tr.battles++
	g.emit("battle_capture", traceLine{"number": g.tr.battles, "capture": c})
}

func (c *battleCapture) validate(g *Game) error {
	fingerprint, err := traceFingerprint(g.opts.FS)
	if err != nil {
		return err
	}
	if c.Version != 1 || len(c.Save) == 0 || len(c.Rand) == 0 || c.Tick < 0 || c.Fade < 0 || g.db.Troops[c.Troop] == nil {
		return errors.New(lang.T("戦闘記録の版・開始状態・乱数・敵集団が不正"))
	}
	if c.Fingerprint == "" || c.Fingerprint != fingerprint {
		return errors.New(lang.T("戦闘記録のデータの指紋が一致しない。同一再現の前提が崩れている"))
	}
	if c.Engine == "" || c.Engine != battleEngine {
		return errors.New(lang.T("戦闘記録のエンジン版が一致しない。同一再現の前提が崩れている"))
	}
	for _, id := range c.Fighters {
		if g.db.Actors[id] == nil {
			return errors.New(lang.T("戦闘記録の参加者が存在しない"))
		}
	}
	return nil
}

func (g *Game) restoreBattleCapture() error {
	c := g.rep.capture
	p := rand.NewPCG(1, 2)
	if err := p.UnmarshalBinary(c.Rand); err != nil {
		return fmt.Errorf(lang.T("戦闘記録の乱数状態が不正: %w"), err)
	}
	st, err := rpg.Parse(g.db, c.Save, rand.New(p)) //nolint:gosec // Reproducible gameplay, not cryptographic randomness.
	if err != nil {
		return err
	}
	if st.LoadReport.Changed() || g.db.Locations[st.Location] == nil {
		return errors.New(lang.T("戦闘記録の開始状態は補正なしで復元できない"))
	}
	g.pcg, g.st, g.tick = p, st, c.Tick
	g.cfg, g.read, g.unlocked = c.Setup.Config, c.Setup.Read, c.Setup.Unlocked
	g.applyConfig()
	g.startState()
	g.enterAt = ""
	g.st.Calm, g.st.Fighters = c.Calm, slices.Clone(c.Fighters)
	g.autoBattle, g.fadeT = c.Auto, c.Fade
	g.autoRead = c.AutoRead
	g.startBattle(c.Troop, func(int) {})
	return nil
}

func loadBattleReplay(path string, o *Options) (_ *replay, err error) {
	f, err := os.Open(path) //nolint:gosec // The caller explicitly selects the replay file.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	r := &replay{inputs: map[int]traceLine{}, acts: map[int][]traceLine{}, expect: map[string][]traceLine{}, at: map[string]int{}, version: traceVersion}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	n, previous, first := 0, -1, true
	for sc.Scan() {
		var l traceLine
		var h struct {
			Kind    string         `json:"t"`
			Tick    *int           `json:"tick"`
			Version int            `json:"version"`
			Capture *battleCapture `json:"capture"`
		}
		if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
			return nil, err
		}
		if h.Tick == nil || *h.Tick < 0 || *h.Tick < previous || h.Kind == "" {
			return nil, errors.New(lang.T("戦闘記録のティック・順序が不正"))
		}
		previous = *h.Tick
		if first {
			first = false
			if h.Kind != "start" || *h.Tick != 0 || h.Version != traceVersion {
				return nil, errors.New(lang.T("戦闘再現には版２のトレースが必要"))
			}
		}
		if h.Kind == "end" {
			return nil, errors.New(lang.T("指定した戦闘の開始記録または結果がない"))
		}
		if h.Kind == "battle_capture" {
			n++
			if r.capture != nil {
				return nil, errors.New(lang.T("戦闘記録の結果より先に次の戦闘がある"))
			}
			if n == o.ReplayBattle {
				if h.Capture == nil || h.Capture.Tick != *h.Tick {
					return nil, errors.New(lang.T("戦闘記録の開始状態が欠落・不正"))
				}
				r.capture = h.Capture
				r.setup = r.capture.Setup
			}
			continue
		}
		if r.capture == nil {
			continue
		}
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, err
		}
		switch h.Kind {
		case "reload":
			return nil, errors.New(lang.T("戦闘中にデータを再読込した記録には対応していない"))
		case "input":
			if *h.Tick <= r.capture.Tick {
				continue
			}
			if _, exists := r.inputs[*h.Tick]; exists {
				return nil, errors.New(lang.T("戦闘記録の入力ティックが重複"))
			}
			for _, key := range []string{"press", "held"} {
				for _, name := range strs(l[key]) {
					if !slices.Contains(buttonNames[:], name) {
						return nil, errors.New(lang.T("戦闘記録に未知のボタンがある"))
					}
				}
			}
			r.inputs[*h.Tick] = l
		case "act":
			name, ok := l["name"].(string)
			if !ok {
				return nil, fmt.Errorf("replay: action has no name")
			}
			if name != "battle_auto" || *h.Tick <= r.capture.Tick {
				return nil, errors.New(lang.T("戦闘記録に再現できない直接操作がある"))
			}
			r.acts[*h.Tick] = append(r.acts[*h.Tick], l)
		default:
			if r.checked(h.Kind) {
				r.expect[h.Kind] = append(r.expect[h.Kind], l)
			}
			if h.Kind == "battle_result" {
				r.limit = *h.Tick
				o.Location, o.Battle, o.Scene, o.Event, o.FromMemo, o.Script = "", "", "", "", "", ""
				o.Party, o.Autopilot, o.Review, o.GalleryAll = nil, nil, false, false
				return r, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New(lang.T("指定した戦闘の開始記録または結果がない"))
}

func (g *Game) traceBattleStep(s battle.Step) {
	if g.tr == nil {
		return
	}
	unit := func(u *battle.Unit) any {
		if u == nil {
			return nil
		}
		return traceLine{"id": u.ID(), "enemy": u.Enemy, "index": u.Index}
	}
	g.emit("battle_step", traceLine{"kind": s.Kind, "user": unit(s.User), "target": unit(s.Target),
		"skill": s.Skill, "amount": s.Amount, "stat": s.Stat, "hp": s.HP, "mp": s.MP,
		"crit": s.Crit, "weak": s.Weak, "resisted": s.Resisted, "state": s.State, "element": s.Element})
}

func (g *Game) traceBattleResult(s *battleScene) {
	if g.tr == nil {
		return
	}
	var rng []byte
	if g.pcg != nil {
		var err error
		if rng, err = g.pcg.MarshalBinary(); err != nil {
			g.failure = err
			return
		}
	}
	result := map[battle.Result]string{battle.Won: "won", battle.Lost: "lost", battle.Fled: "fled"}[s.b.Result]
	g.emit("battle_result", traceLine{"result": result, "gains": s.b.Gains, "summary": g.traceSummary(), "rand": rng})
}

func battleComparable(s string) string {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return s
	}
	return string(b)
}
