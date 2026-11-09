package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/base/traceformat"
)

type metricPoint struct {
	Tick  float64 `json:"tick"`
	Event string  `json:"event,omitempty"`
}

type readSpan struct {
	From       metricPoint `json:"from"`
	To         metricPoint `json:"to"`
	Ticks      float64     `json:"ticks"`
	Seconds    float64     `json:"seconds"`
	Characters int         `json:"characters"`
	Lines      int         `json:"lines"`
	Decision   string      `json:"decision"`
}

type repeatedLine struct {
	Text       string    `json:"text"`
	Count      int       `json:"count"`
	FirstTicks []float64 `json:"first_ticks"`
}

type metricSignal struct {
	At      metricPoint `json:"at"`
	Kinds   []string    `json:"kinds"`
	Ticks   *float64    `json:"interval_ticks,omitempty"`
	Seconds *float64    `json:"interval_seconds,omitempty"`
}

type metricStall struct {
	From    metricPoint    `json:"from"`
	To      metricPoint    `json:"to"`
	Ticks   float64        `json:"ticks"`
	Seconds float64        `json:"seconds"`
	Kind    string         `json:"kind"`
	Inputs  int            `json:"inputs"`
	Screen  map[string]any `json:"screen"`
}

type playMetrics struct {
	Decisions    []readSpan       `json:"text_per_decision"`
	Trailing     readSpan         `json:"trailing_text"`
	Longest      readSpan         `json:"longest_read_only"`
	Repeated     []repeatedLine   `json:"repeated_text"`
	SignalsUsed  []string         `json:"signals_used"`
	Signals      []metricSignal   `json:"climax_spacing"`
	Stalls       []metricStall    `json:"stalls"`
	Sections     []map[string]any `json:"sections"`
	Notes        []string         `json:"notes"`
	StallSeconds float64          `json:"stall_seconds"`
}

type metricObservation struct {
	start  metricPoint
	screen map[string]any
	seen   bool
}

type metricReader struct {
	report                                          playMetrics
	at, begin                                       metricPoint
	seen                                            bool
	chars, lines                                    int
	repeats                                         map[string]*repeatedLine
	last                                            map[string]map[string]any
	screen                                          map[string]any
	state, display, progress, input, rejected, auto metricObservation
	inputs                                          int
	lastDecision                                    float64
	phoneAction                                     bool
	waitReason                                      string
}

// measureTrace retains aggregates and report rows, not input frames. Exact repeat
// counts require one entry per distinct dialogue line; first occurrences are capped.
func measureTrace(r io.Reader, top int, stallSeconds float64) (playMetrics, error) {
	m := playMetrics{
		Decisions: []readSpan{}, Repeated: []repeatedLine{}, Signals: []metricSignal{},
		Stalls: []metricStall{}, Sections: []map[string]any{}, StallSeconds: stallSeconds,
		SignalsUsed: []string{"battle scene entry", "battle cut-in", "audible song/module change", "stand CG change", "screen background change", "screen phone_call entry"},
		Notes: []string{
			"Facts, never verdicts. Seconds are simulation ticks / 60, not wall time.",
			"Text is nonempty say lines: Unicode code points excluding line separators; choice labels and screen text are excluded to avoid duplicate/typewriter counts. Display does not prove reading.",
			"Decisions are answer records, accepted menu decision records and acknowledged scripted phone actions; same-tick records count once. Older traces without decision records cannot measure menu selections.",
			"Phone results without recorded actions remain result boundaries and can include automatic timeouts.",
			"Unknown event names are omitted. The trailing span ends at the last observed tick, not an inferred completion.",
			"Stalls describe unchanged observations, not faults. Inputs without observable progress do not prove rejection; explicit input_result and autoplay_wait records are reported when available.",
			"Missing screen records are null. Animations can change screens without changing game state. Repeated text includes at most the first three occurrence ticks.",
			"Signal spacing is between observed signals, not a judgment of intensity; coincident signals share one moment. Initial music, CG and backgrounds establish baselines.",
		},
	}
	reader := metricReader{report: m, repeats: map[string]*repeatedLine{}, last: map[string]map[string]any{}, lastDecision: -1}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<24)
	for sc.Scan() {
		l, err := decodeTrace(sc.Bytes())
		if err != nil {
			return m, err
		}
		if err := reader.record(l); err != nil {
			return m, err
		}
	}
	if err := sc.Err(); err != nil {
		return m, err
	}
	reader.finish(top)
	return reader.report, nil
}

func (r *metricReader) span(decision string) readSpan {
	d := r.at.Tick - r.begin.Tick
	return readSpan{r.begin, r.at, d, d / 60, r.chars, r.lines, decision}
}

func (r *metricReader) longest(s readSpan) {
	if s.Ticks > r.report.Longest.Ticks || s.Ticks == r.report.Longest.Ticks && s.Characters > r.report.Longest.Characters {
		r.report.Longest = s
	}
}

func (r *metricReader) observation() metricObservation {
	return metricObservation{r.at, r.screen, true}
}

func (r *metricReader) stall(o metricObservation, kind string, inputs int) {
	d := r.at.Tick - o.start.Tick
	if o.seen && d/60 >= r.report.StallSeconds {
		r.report.Stalls = append(r.report.Stalls, metricStall{o.start, r.at, d, d / 60, kind, inputs, o.screen})
	}
}

func (r *metricReader) record(l map[string]any) error {
	tick := traceformat.Number(l["tick"])
	if r.seen && tick < r.at.Tick {
		return diag.TraceBroken.Errorf("metrics require nondecreasing trace ticks")
	}
	r.at.Tick = tick
	if event, ok := l["event"].(string); ok {
		r.at.Event = event
	}
	if !r.seen {
		r.begin, r.progress, r.seen = r.at, r.observation(), true
	}
	k := str(l["t"])
	facts := maps.Clone(l)
	delete(facts, "tick")
	delete(facts, "event")
	if k == "state" {
		delete(facts, "terminal")
	}
	previous, exists := r.last[k]
	changed := !exists || !reflect.DeepEqual(previous, facts)
	r.signals(k, l, previous, exists)
	progress := r.consume(k, l, changed)
	switch k {
	case "state", "screen", "scene", "music", "stand", "menu", "request", "act", "battle_end", "battle_result", "jump", "resume":
		r.last[k] = facts
	}
	if progress {
		r.stall(r.progress, "no_observed_progress", 0)
		r.stall(r.input, "input_without_observed_progress", r.inputs)
		r.input.seen, r.inputs = false, 0
		r.progress = r.observation()
	}
	return nil
}

func (r *metricReader) consume(kind string, l map[string]any, changed bool) bool {
	switch kind {
	case "say":
		r.phoneAction = false
		r.text(str(l["text"]))
		return true
	case "answer", "decision", "input_result":
		return r.decision(kind, l)
	case "state":
		if changed {
			r.stall(r.state, "no_state_change", 0)
			r.state = r.observation()
		}
		return changed
	case "screen":
		if str(l["phone"]) == "" {
			r.phoneAction = false
		}
		if changed {
			r.stall(r.display, "same_screen", 0)
			r.screen = l
			r.display = r.observation()
		}
		return changed
	case "battle":
		return true
	case "input":
		if len(traceformat.Strings(l["press"])) > 0 || l["click"] == true || l["esc"] == true || traceformat.Number(l["digit"]) > 0 {
			if r.inputs == 0 {
				r.input = r.observation()
			}
			r.inputs++
		}
	case "autoplay_wait":
		r.stall(r.auto, "autoplay_wait:"+r.waitReason, 0)
		r.waitReason = str(l["waiting"])
		r.auto = r.observation()
		r.auto.seen = r.waitReason != ""
	case "section":
		r.report.Sections = append(r.report.Sections, l)
	case "scene", "menu", "request", "act", "battle_end", "battle_result", "jump", "resume":
		return changed
	}
	return false
}

func (r *metricReader) text(text string) {
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		r.chars += utf8.RuneCountInString(line)
		r.lines++
		entry := r.repeats[line]
		if entry == nil {
			entry = &repeatedLine{Text: line}
			r.repeats[line] = entry
		}
		entry.Count++
		if len(entry.FirstTicks) < 3 {
			entry.FirstTicks = append(entry.FirstTicks, r.at.Tick)
		}
	}
}

func (r *metricReader) decision(kind string, l map[string]any) bool {
	label := str(l["kind"])
	if kind == "input_result" {
		if l["accepted"] == false && !r.rejected.seen {
			r.rejected = r.observation()
		}
		if l["accepted"] == true && r.rejected.seen {
			r.stall(r.rejected, "input_not_acknowledged", 0)
			r.rejected.seen = false
		}
		if l["accepted"] != true || l["phone"] != true {
			return false
		}
		r.phoneAction = true
		label = "phone_action"
	}
	if kind == "answer" {
		label = "choice"
		if l["phone"] == true {
			if r.phoneAction {
				r.phoneAction = false
				return false
			}
			label = "phone_result"
		}
	}
	if r.at.Tick != r.lastDecision {
		s := r.span(label)
		r.report.Decisions = append(r.report.Decisions, s)
		r.longest(s)
		r.begin, r.chars, r.lines, r.lastDecision = r.at, 0, 0, r.at.Tick
	}
	return true
}

func (r *metricReader) signal(kind string) {
	n := len(r.report.Signals)
	if n > 0 && r.report.Signals[n-1].At.Tick == r.at.Tick {
		if !slices.Contains(r.report.Signals[n-1].Kinds, kind) {
			r.report.Signals[n-1].Kinds = append(r.report.Signals[n-1].Kinds, kind)
		}
		return
	}
	s := metricSignal{At: r.at, Kinds: []string{kind}}
	if n > 0 {
		d := r.at.Tick - r.report.Signals[n-1].At.Tick
		sec := d / 60
		s.Ticks, s.Seconds = &d, &sec
	}
	r.report.Signals = append(r.report.Signals, s)
}

func (r *metricReader) signals(kind string, l, previous map[string]any, exists bool) {
	switch kind {
	case "screen":
		if exists && !reflect.DeepEqual(previous["backgrounds"], l["backgrounds"]) {
			r.signal("background")
		}
		if l["phone_call"] == true && previous["phone_call"] != true {
			r.signal("phone_call")
		}
	case "scene":
		if slices.Contains(traceformat.Strings(l["stack"]), "battle") && !slices.Contains(traceformat.Strings(previous["stack"]), "battle") {
			r.signal("battle")
		}
	case "battle":
		if str(l["cutin"]) != "" {
			r.signal("cut_in")
		}
	case "music":
		if exists && (str(previous["song"]) != str(l["song"]) || str(previous["module"]) != str(l["module"])) {
			r.signal("music")
		}
	case "stand":
		if exists && str(previous["cg"]) != str(l["cg"]) {
			r.signal("cg")
		}
	}
}

func (r *metricReader) finish(top int) {
	r.stall(r.auto, "autoplay_wait:"+r.waitReason, 0)
	r.stall(r.rejected, "input_not_acknowledged", 0)
	r.report.Trailing = r.span("trace_end")
	r.longest(r.report.Trailing)
	r.stall(r.state, "no_state_change", 0)
	r.stall(r.display, "same_screen", 0)
	r.stall(r.input, "input_without_observed_progress", r.inputs)
	r.stall(r.progress, "no_observed_progress", 0)
	for _, entry := range r.repeats {
		if entry.Count > 1 {
			r.report.Repeated = append(r.report.Repeated, *entry)
		}
	}
	slices.SortFunc(r.report.Repeated, func(a, b repeatedLine) int {
		if n := cmp.Compare(b.Count, a.Count); n != 0 {
			return n
		}
		return cmp.Compare(a.Text, b.Text)
	})
	if len(r.report.Repeated) > top {
		r.report.Repeated = r.report.Repeated[:top]
	}
}

func (c *command) printTraceMetrics(m playMetrics) error {
	if c.report.Machine() {
		return c.report.Result(c.out, "ok", m)
	}
	c.out.Println("Play metrics (facts, never verdicts)")
	for _, note := range m.Notes {
		c.out.Println(note)
	}
	c.out.Println("Text per decision:")
	printSpan := func(s readSpan) {
		c.out.Printf("  %g (%s) -> %g (%s): %g ticks / %.2fs, %d characters, %d lines; %s\n",
			s.From.Tick, s.From.Event, s.To.Tick, s.To.Event, s.Ticks, s.Seconds, s.Characters, s.Lines, s.Decision)
	}
	for _, s := range m.Decisions {
		printSpan(s)
	}
	c.out.Println("Trailing text:")
	printSpan(m.Trailing)
	c.out.Println("Longest read-only stretch:")
	printSpan(m.Longest)
	c.out.Println("Repeated text:")
	for _, line := range m.Repeated {
		c.out.Printf("  %dx %q; first ticks %v\n", line.Count, line.Text, line.FirstTicks)
	}
	c.out.Println("Climax spacing signals:", strings.Join(m.SignalsUsed, ", "))
	for _, signal := range m.Signals {
		c.out.Printf("  tick %g: %s", signal.At.Tick, strings.Join(signal.Kinds, ", "))
		if signal.Ticks != nil {
			c.out.Printf("; interval %g ticks / %.2fs", *signal.Ticks, *signal.Seconds)
		}
		c.out.Println()
	}
	c.out.Printf("Stalls (observation threshold %.2fs):\n", m.StallSeconds)
	for _, stall := range m.Stalls {
		b, err := json.Marshal(stall.Screen)
		if err != nil {
			return err
		}
		c.out.Printf("  %g -> %g: %s, %g ticks / %.2fs, %d inputs; screen %s\n",
			stall.From.Tick, stall.To.Tick, stall.Kind, stall.Ticks, stall.Seconds, stall.Inputs, b)
	}
	c.out.Println("Recorded sections:")
	for _, section := range m.Sections {
		b, err := json.Marshal(section)
		if err != nil {
			return err
		}
		c.out.Println(string(b))
	}
	return nil
}
