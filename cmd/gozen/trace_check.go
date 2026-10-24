package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/diag"

	lang "github.com/nao1215/gozen/internal/text"
)

type traceIssue struct {
	tick int
	kind string
	text string
}

type traceChecker struct {
	musicLate float64
	speed     float64
	issues    []traceIssue

	sayWho, sayText string
	sayTick         int
	sayN            int

	reported map[string]bool

	pos map[string]float64

	bg          string
	hasBG       bool
	carried     map[string]bool
	spoke       map[string]bool
	lastFigures []standFigure

	wantSong string
	wantAt   int
	waiting  bool
	stands   int

	// skipping is skip mode or scene skip, from the trace's skip lines (ADR 0026).
	skipping bool
}

type standFigure struct {
	ID       string  `json:"id"`
	Face     string  `json:"face"`
	Side     string  `json:"side"`
	X        float64 `json:"x"`
	Alpha    float64 `json:"alpha"`
	Speaking bool    `json:"speaking"`
	Leaving  bool    `json:"leaving"`
}

type standLine struct {
	Tick     int           `json:"tick"`
	Since    int           `json:"since"`
	Idle     bool          `json:"idle"`
	Set      int           `json:"set"`
	Speaker  string        `json:"speaker"`
	Face     string        `json:"face"`
	Standing bool          `json:"standing"`
	Figures  []standFigure `json:"figures"`
	Location string        `json:"location"`
	CG       string        `json:"cg"`
	CGPage   string        `json:"cg_page"`
	Hides    bool          `json:"hides"`
	CGBG     bool          `json:"cg_bg"`
	Over     bool          `json:"over"`
	Fading   bool          `json:"fading"`
}

func (c *traceChecker) add(tick int, kind, format string, a ...any) {
	c.issues = append(c.issues, traceIssue{tick, kind, fmt.Sprintf(format, a...)})
}

func (c *traceChecker) say() string {
	if c.sayWho != "" {
		return c.sayWho + fmt.Sprintf(lang.T("「%s」"), c.sayText)
	}
	return c.sayText
}

func who(key string) string {
	if _, v, ok := strings.Cut(key, ":"); ok {
		return v
	}
	return key
}

func (c *traceChecker) line(l map[string]any, raw []byte) error {
	k := str(l["t"])
	tick := int(num(l["tick"]))
	switch k {
	case "start":
		if err := useTraceLanguage(l); err != nil {
			return err
		}
		if s := num(l["speed"]); s > 1 {
			c.speed = s
		}
	case "say":
		c.sayWho, c.sayText, c.sayTick = str(l["who"]), str(l["text"]), tick
		c.sayN++
		if read, ok := l["read"].(bool); ok && !read && c.skipping {
			c.add(tick, "skip-unread", lang.T("スキップ中に読んでいない台詞を飛ばした（%s）"), c.say())
		}
	case "skip":
		c.skipping = l["on"] == true
	case "voice":
		switch {
		case c.skipping:
			c.add(tick, "skip-voice", lang.T("スキップ中にボイス %s が鳴った"), str(l["file"]))
		case l["missing"] == true:
			c.add(tick, "voice", lang.T("ボイス %s が読めない（%s）"), str(l["file"]), c.say())
		}
	case "rollback":
		if from, to := num(l["from"]), num(l["to"]); l["back_into"] != true && to >= from {
			c.add(tick, "rollback", lang.T("戻る操作で行が戻っていない（%v 行目から %v 行目）"), from, to)
		}
	case "stand":
		var s standLine
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		c.stands++
		c.stand(s)
	case "music":
		c.music(l, tick)
	case "end":
		c.musicEnd(tick)
	}
	return nil
}

func (c *traceChecker) stand(s standLine) {
	if s.Idle {
		c.lastFigures = nil
		c.pos = map[string]float64{}
		return
	}

	pos := map[string]float64{}
	for _, f := range s.Figures {
		if f.Leaving || f.Alpha < 0.5 {
			continue
		}
		key := fmt.Sprintf("%d|%s", s.Set, f.ID)
		if x, ok := c.pos[key]; ok && x != f.X {
			c.add(s.Since, "moved", lang.T("%s の立ち位置が会話の途中で %.2f から %.2f に動いた（直前の台詞: %s）"), who(f.ID), x, f.X, c.say())
		}
		pos[key] = f.X
	}
	c.pos = pos

	bg := s.Location + "|" + s.CGPage
	if !c.hasBG || bg != c.bg {
		if c.hasBG {
			c.carried = map[string]bool{}
			for _, f := range c.lastFigures {
				if !f.Leaving && f.Alpha >= 0.5 {
					c.carried[f.ID] = true
				}
			}
		}
		c.bg, c.hasBG = bg, true
		c.spoke = map[string]bool{}
	}
	if s.Speaker != "" {
		c.spoke[s.Speaker] = true
	}
	for _, f := range s.Figures {
		if !f.Leaving && f.Alpha >= 0.5 && c.carried[f.ID] && !c.spoke[f.ID] {
			c.add(s.Since, "left-over", lang.T("背景が替わったのに %s が舞台に残っている（背景 %s、直前の台詞: %s）"), who(f.ID), bgName(s), c.say())
			delete(c.carried, f.ID)
		}
	}
	c.lastFigures = s.Figures

	if s.Over {
		var ids []string
		for _, f := range s.Figures {
			ids = append(ids, who(f.ID))
		}
		c.add(s.Since, "over-cg", lang.T("一枚絵 %s の上に立ち絵（%s）が見えた（直前の台詞: %s）"), s.CG, strings.Join(ids, lang.T("・")), c.say())
	}

	if s.Speaker != "" && s.Standing && !s.Fading {
		shown := false
		for _, f := range s.Figures {
			if f.ID == s.Speaker && f.Speaking && f.Alpha >= 0.5 {
				shown = true
			}
		}
		key := strconv.Itoa(c.sayN)
		if !shown && !c.reported[key] {
			switch {
			case !s.Hides:
				c.reported[key] = true
				c.add(s.Since, "no-stand", lang.T("話している %s の立ち絵が出ていない（%s）"), who(s.Speaker), c.say())
			case s.CGBG:
				c.reported[key] = true
				c.add(s.Since, "no-stand", lang.T("場所の背景 %s が立ち絵を隠したまま %s が話す（faces: true が要る。%s）"), s.CG, who(s.Speaker), c.say())
			}
		}
	}
}

func bgName(s standLine) string {
	if s.CG != "" {
		return s.CG
	}
	return "@" + s.Location
}

func (c *traceChecker) late(ticks int) bool {
	return float64(ticks)/60/c.speed > c.musicLate
}

func (c *traceChecker) music(l map[string]any, tick int) {
	wanted := str(l["wanted"])
	if wanted != c.wantSong {
		if c.waiting && c.late(tick-c.wantAt) {
			c.add(c.wantAt, "music-late", lang.T("曲 %s を頼んでから %.1f 秒鳴らないまま次の曲 %q を頼んだ"), c.wantSong, float64(tick-c.wantAt)/60/c.speed, wanted)
		}
		c.wantSong = wanted
		c.wantAt = int(num(l["asked_at"]))
		if _, ok := l["asked_at"]; !ok {
			c.wantAt = tick
		}
		c.waiting = wanted != "" && str(l["song"]) != wanted
	}
	if w, ok := l["wait"]; ok {
		c.waiting = false
		if c.late(int(num(w))) {
			c.add(c.wantAt, "music-late", lang.T("曲 %s を頼んでから鳴り始めるまで %.1f 秒（直前の台詞: %s）"), str(l["song"]), num(w)/60/c.speed, c.say())
		}
	}
}

func (c *traceChecker) musicEnd(tick int) {
	if c.waiting && c.late(tick-c.wantAt) {
		c.add(c.wantAt, "music-late", lang.T("曲 %s を頼んだが終わりまで鳴らなかった（%.1f 秒）"), c.wantSong, float64(tick-c.wantAt)/60/c.speed)
	}
	c.waiting = false
}

func checkTrace(path string, musicLate float64, w io.Writer) (n int, err error) {
	f, err := os.Open(path) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	output := &cliio.Output{Writer: w}
	c := &traceChecker{
		musicLate: musicLate, speed: 1, reported: map[string]bool{}, pos: map[string]float64{},
		carried: map[string]bool{}, spoke: map[string]bool{},
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	last := 0
	for sc.Scan() {
		l, err := decodeTrace(sc.Bytes())
		if err != nil {
			return 0, err
		}
		last = int(num(l["tick"]))
		if err := c.line(l, sc.Bytes()); err != nil {
			return 0, err
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	c.musicEnd(last)
	slices.SortStableFunc(c.issues, func(a, b traceIssue) int { return a.tick - b.tick })
	for _, is := range c.issues {
		sec := float64(is.tick) / 60
		code := diag.PictureFault
		switch {
		case strings.HasPrefix(is.kind, "music"):
			code = diag.MusicFault
		case strings.HasPrefix(is.kind, "skip"):
			code = diag.SkipFault
		case is.kind == "rollback":
			code = diag.RollbackFault
		case is.kind == "voice":
			code = diag.VoiceFault
		}
		output.Println(code.Mark(fmt.Sprintf("%02d:%05.2f %-10s %s", int(sec)/60, sec-float64(int(sec)/60*60), is.kind, is.text)))
	}
	if c.stands == 0 && c.sayN > 0 {
		output.Println(lang.T("stand の行が無い（古い trace か、会話の窓を描いていない）。立ち絵は調べていない"))
	}
	output.Printf(lang.T("check: %d 件（台詞 %d、stand %d）\n"), len(c.issues), c.sayN, c.stands)
	return len(c.issues), output.Err
}
