package react

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Stimulus is what reaches a person: words, an event, numbers.
type Stimulus struct {
	// Kind is KindSay for words, or a game-defined kind such as "event.steal".
	Kind string
	// From names the sender (a player, a viewer); repeats are counted per sender.
	From string
	// Text is the words; it may be empty for events.
	Text string
	// Values are numbers the rules read as value.NAME (a tip, an item's price).
	Values map[string]float64
	// Tags are tags the game already knows (score 1), declared in Book.Tags.
	Tags []string
}

// Tag is a finding with a confidence Score in 0..1. Slot is the word that suggested
// it, for templates and memory.
type Tag struct {
	Name  string
	Score float64
	Slot  string
}

// Config holds a Mind's collaborators. The zero value works: a fixed random seed, no
// game clock, 400 runes and 8 remembered texts per sender.
type Config struct {
	// Rand chooses lines. Nil uses a fixed seed, so reactions are reproducible.
	Rand *rand.Rand
	// Clock is the game time, for memories with a Time limit. Nil is allowed only when
	// no rule of the person uses Time.
	Clock func() time.Duration
	// MaxRunes cuts longer text before analysis (0 means 400).
	MaxRunes int
	// History is how many recent texts per sender are kept to find repeats (0 means 8).
	History int
}

// Reaction is how the person reacted.
type Reaction struct {
	// ruleDef is the rule ID, "dodge.repeat", "dodge.unsure", "dodge.unknown",
	// "ask.ID" for Mind.Ask, or empty when the person had nothing to say.
	Rule string
	// Lines are the line said and, when the rule asks a question, the question.
	Lines []string
	// Written are the Lines as the book writes them, before their fields are filled, and
	// Fields what each line's fields were filled with, by name ("from", "slot.cat",
	// "mem.pet"): a game that shows a line in another language fills its translation with
	// them (Fill).
	Written []string
	Fields  []map[string]string
	Face    string
	// Delta is the mood change applied; Mood is the mood after it.
	Delta, Mood float64
	// Confidence is the highest tag score (0 when nothing was found).
	Confidence float64
	// Tags are the findings, the most confident first.
	Tags []Tag
	// question is the ID of the question now pending, if any.
	Ask string
}

type lexWord struct {
	norm, text   string
	weight       float64
	whole, exact bool
}

// lexRun is a lexicon tag ready to match: its words, and its except words normalized
// (for the normal words) and lowered (for the exact ones).
type lexRun struct {
	words           []lexWord
	exceptNorm, low []string
}

type entry struct {
	Value     any           `json:"v"`
	UntilTurn int           `json:"turn,omitempty"`
	UntilTime time.Duration `json:"time,omitempty"`
}

// Mind is the running state of one person. It is not safe for concurrent use.
type Mind struct {
	book *Book
	p    *personDef
	cfg  Config
	rng  *rand.Rand
	lex  map[string]*lexRun
	tags []string // lexicon tags, sorted

	mood    float64
	turn    int
	mem     map[string]entry
	cool    map[string]int // rule ID -> first turn it may react again
	asked   string
	askTill int
	// used are the lines each list (a rule, a question, a dodge) said last, by their first
	// line, which it does not say again for a while (pickEntry).
	used   map[string][]string
	recent map[string][]string
}

// New makes the running state of the person id in the book. The book must have been
// validated (Load and Parse do it) and must not change while the Mind is used.
func New(b *Book, id string, cfg Config) (*Mind, error) {
	p, ok := b.person(id)
	if !ok {
		return nil, fmt.Errorf("react: no person %q", id)
	}
	if cfg.Clock == nil {
		for _, r := range p.Rules {
			for _, m := range r.Remember {
				if m.Time > 0 {
					return nil, fmt.Errorf("react: %s.%s remembers %q by time but Config.Clock is nil", id, r.ID, m.Key)
				}
			}
		}
	}
	if cfg.MaxRunes <= 0 {
		cfg.MaxRunes = 400
	}
	if cfg.History <= 0 {
		cfg.History = 8
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: reproducible line choice, not security
	}
	m := &Mind{
		book: b, p: p, cfg: cfg, rng: rng, lex: map[string]*lexRun{},
		mem: map[string]entry{}, cool: map[string]int{}, recent: map[string][]string{}, used: map[string][]string{},
	}
	for tag, lt := range b.doc.Lexicon {
		m.tags = append(m.tags, tag)
		run := &lexRun{}
		for _, w := range lt.Words {
			run.words = append(run.words, lexWord{norm: Normalize(w.Text), text: w.Text, weight: w.Weight, whole: w.Whole, exact: w.Exact || w.Whole})
		}
		for _, x := range lt.Except {
			run.exceptNorm = append(run.exceptNorm, Normalize(x))
			run.low = append(run.low, strings.ToLower(x))
		}
		m.lex[tag] = run
	}
	sort.Strings(m.tags)
	lo, hi := p.bounds()
	m.mood = math.Max(lo, math.Min(hi, p.Mood))
	return m, nil
}

// Mood is the current mood.
func (m *Mind) Mood() float64 { return m.mood }

// Recall is a remembered value (a string or a float64); ok is false when it is absent
// or expired.
func (m *Mind) Recall(key string) (any, bool) {
	e, ok := m.mem[key]
	return e.Value, ok
}

// Reset starts a new conversation with the same person, such as the next stream
// or a later visit: the recent texts used to find repeats and the pending question are
// cleared, while the mood, the memory and the cooldowns are kept.
func (m *Mind) Reset() {
	m.recent = map[string][]string{}
	m.asked, m.askTill = "", 0
}

func (m *Mind) now() time.Duration {
	if m.cfg.Clock == nil {
		return 0
	}
	return m.cfg.Clock()
}

func (m *Mind) expire() {
	now := m.now()
	for k, e := range m.mem {
		if (e.UntilTurn > 0 && m.turn > e.UntilTurn) || (e.UntilTime > 0 && now > e.UntilTime) {
			delete(m.mem, k)
		}
	}
}

// match finds the lexicon tags: a normal word in norm, the normalized text, an exact or
// whole word in raw, the text as written. An occurrence inside an occurrence of one of
// the tag's except words is not counted. raw is empty for the acrostic, which has no
// words as written.
func (m *Mind) match(norm, raw string, prefix string, found map[string]Tag) {
	if norm == "" {
		return
	}
	low := strings.ToLower(raw)
	for _, tag := range m.tags {
		run := m.lex[tag]
		for _, w := range run.words {
			var hit bool
			if w.exact {
				hit = occurs(low, strings.ToLower(w.text), run.low, w.whole)
			} else {
				hit = occurs(norm, w.norm, run.exceptNorm, false)
			}
			if hit {
				add(found, Tag{Name: prefix + tag, Score: w.weight, Slot: w.text})
			}
		}
	}
}

// occurs reports whether text holds word somewhere not inside an occurrence of an
// except word, and, when whole, with no letter or digit right before or after it.
func occurs(text, word string, except []string, whole bool) bool {
	if word == "" {
		return false
	}
	edge := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		at, end := i+j, i+j+len(word)
		ok := true
		if whole {
			before, _ := utf8.DecodeLastRuneInString(text[:at])
			after, _ := utf8.DecodeRuneInString(text[end:])
			ok = (at == 0 || edge(before)) && (end == len(text) || edge(after))
		}
		if ok && !covered(text, at, end, except) {
			return true
		}
		_, n := utf8.DecodeRuneInString(text[at:])
		i = at + n
	}
}

// covered reports whether text[at:end] lies inside an occurrence of an except word.
func covered(text string, at, end int, except []string) bool {
	for _, x := range except {
		for from := max(0, end-len(x)); from <= at; {
			k := strings.Index(text[from:], x)
			if k < 0 || from+k > at {
				break
			}
			if from+k+len(x) >= end {
				return true
			}
			from += k + 1
		}
	}
	return false
}

// add keeps the most confident finding per tag.
func add(found map[string]Tag, t Tag) {
	t.Score = math.Max(0, math.Min(1, t.Score))
	if old, ok := found[t.Name]; !ok || t.Score > old.Score {
		found[t.Name] = t
	}
}

// React analyzes the stimulus, chooses a rule (or a dodge) and applies it to the mood
// and memory. An error means a condition failed to evaluate (for example comparing a
// remembered string with a number). The turn then still advances and memories whose
// limit has passed are dropped, as on every call; the mood, the remaining memory, the
// recent texts, the pending question and the cooldowns are unchanged, so retrying the
// same stimulus is not counted as a repeat.
func (m *Mind) React(s Stimulus) (Reaction, error) {
	m.turn++
	m.expire()
	if utf8.RuneCountInString(s.Text) > m.cfg.MaxRunes {
		s.Text = string([]rune(s.Text)[:m.cfg.MaxRunes])
	}
	text := s.Text
	norm := Normalize(text)
	found := map[string]Tag{}
	m.match(norm, text, "", found)
	if ls := lines(text); len(ls) >= 2 {
		var heads strings.Builder
		for _, l := range ls {
			r, _ := utf8.DecodeRuneInString(l)
			heads.WriteRune(r)
		}
		m.match(heads.String(), "", AcrosticPrefix, found)
	}
	for _, t := range s.Tags {
		add(found, Tag{Name: t, Score: 1})
	}
	if s.Kind == KindSay && norm == "" {
		add(found, Tag{Name: TagEmpty, Score: 1})
	}
	repeats := 0
	if norm != "" {
		for _, r := range m.recent[s.From] {
			if r == norm {
				repeats++
			}
		}
		if repeats > 0 {
			add(found, Tag{Name: TagRepeat, Score: 1})
		}
		keys := make([]string, 0, len(m.mem))
		for k := range m.mem {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, ok := m.mem[k].Value.(string)
			if n := Normalize(v); ok && meaningful(n) && strings.Contains(norm, n) {
				add(found, Tag{Name: TagRecall, Score: 1, Slot: v})
				break
			}
		}
	}
	answer := ""
	if q := m.ask(m.asked); q != nil {
		for _, t := range q.Expect {
			if f, ok := found[t]; ok {
				f.Score = 1
				found[t] = f
				if answer == "" {
					answer = t
				}
			}
		}
	}
	tags := make([]Tag, 0, len(found))
	for _, t := range found {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Score != tags[j].Score {
			return tags[i].Score > tags[j].Score
		}
		return tags[i].Name < tags[j].Name
	})
	conf := 0.0
	if len(tags) > 0 {
		conf = tags[0].Score
	}
	e := &env{m: m, s: s, found: found, answer: answer, repeats: repeats, length: utf8.RuneCountInString(norm), conf: conf}
	rule, err := m.choose(s, e)
	if err != nil {
		return Reaction{}, err
	}
	delta := 0.0
	for _, t := range tags {
		delta += m.p.Likes[t.Name] * t.Score
	}
	out := Reaction{Confidence: conf, Tags: tags}
	pending := m.asked
	if m.turn >= m.askTill {
		pending = ""
	}
	if rule != nil {
		delta += rule.Delta
		out.Rule, out.Face = rule.ID, rule.Face
		for _, l := range m.pickEntry(rule.ID, rule.Say) {
			out.say(m, l, e)
		}
		for _, r := range rule.Remember {
			m.write(r, e)
		}
		for _, k := range rule.Forget {
			delete(m.mem, k)
		}
		if rule.Cooldown > 0 {
			m.cool[rule.ID] = m.turn + rule.Cooldown + 1
		}
		if rule.Ask != "" {
			q := m.ask(rule.Ask)
			l := m.pick("ask."+q.ID, q.Say)
			out.say(m, l, e)
			if q.Face != "" {
				out.Face = q.Face
			}
			pending = m.pose(q)
		}
	} else {
		d := m.p.Dodge
		lines, name := d.Unknown, "unknown"
		switch {
		case found[TagRepeat].Score > 0 && len(d.Repeat) > 0:
			lines, name = d.Repeat, "repeat"
		case len(tags) > 0 && len(d.Unsure) > 0:
			lines, name = d.Unsure, "unsure"
		}
		if l := m.pick("dodge."+name, lines); l != "" {
			out.Rule, out.Face = "dodge."+name, d.Face
			out.say(m, l, e)
		}
	}
	m.asked = pending
	lo, hi := m.p.bounds()
	before := m.mood
	m.mood = math.Max(lo, math.Min(hi, m.mood+delta))
	out.Delta, out.Mood, out.Ask = m.mood-before, m.mood, m.asked
	m.remember(s.From, norm)
	return out, nil
}

// meaningful reports whether a remembered word is distinctive enough to count as a
// mention: two characters or more, or a single kanji.
func meaningful(n string) bool {
	if utf8.RuneCountInString(n) >= 2 {
		return true
	}
	r, _ := utf8.DecodeRuneInString(n)
	return unicode.Is(unicode.Han, r)
}

func (m *Mind) choose(s Stimulus, e *env) (*ruleDef, error) {
	for i := range m.p.Rules {
		r := &m.p.Rules[i]
		if (r.Kind != "" && r.Kind != s.Kind) || m.turn < m.cool[r.ID] {
			continue
		}
		ok, err := r.When.Bool(e)
		if err != nil {
			return nil, fmt.Errorf("react: %s.%s: %w", m.p.ID, r.ID, err)
		}
		if ok {
			return r, nil
		}
	}
	return nil, nil
}

// Ask has the person put the question id, as a reaction of its own (for example when
// the game wants the person to start a topic).
func (m *Mind) Ask(id string) (Reaction, error) {
	q := m.ask(id)
	if q == nil {
		return Reaction{}, fmt.Errorf("react: %s has no ask %q", m.p.ID, id)
	}
	e := &env{m: m, found: map[string]Tag{}}
	m.asked = m.pose(q)
	l := m.pick("ask."+q.ID, q.Say)
	var r Reaction
	r.say(m, l, e)
	r.Rule, r.Face, r.Mood, r.Ask = "ask."+id, q.Face, m.mood, id
	return r, nil
}

// say adds the line as the book writes it, filled, and what its fields were filled with.
func (r *Reaction) say(m *Mind, l string, e *env) {
	fields := m.fields(l, e)
	r.Lines = append(r.Lines, Fill(l, fields))
	r.Written = append(r.Written, l)
	r.Fields = append(r.Fields, fields)
}

func (m *Mind) ask(id string) *question {
	for i := range m.p.Asks {
		if id != "" && m.p.Asks[i].ID == id {
			return &m.p.Asks[i]
		}
	}
	return nil
}

func (m *Mind) pose(q *question) string {
	m.askTill = m.turn + max(1, q.Turns)
	return q.ID
}

func (m *Mind) remember(from, norm string) {
	if norm == "" {
		return
	}
	r := append(m.recent[from], norm)
	if len(r) > m.cfg.History {
		r = r[len(r)-m.cfg.History:]
	}
	m.recent[from] = r
}

func (m *Mind) write(r memoryWrite, e *env) {
	en := entry{}
	if r.Turns > 0 {
		en.UntilTurn = m.turn + r.Turns
	}
	if r.Time > 0 {
		en.UntilTime = m.now() + time.Duration(r.Time*float64(time.Second))
	}
	if r.Set != "" {
		en.Value = m.fill(r.Set, e)
	} else {
		cur := 0.0
		if f, ok := m.mem[r.Key].Value.(float64); ok {
			cur = f
		}
		en.Value = cur + r.Add
	}
	m.mem[r.Key] = en
}

// pick chooses a line of the list key as pickEntry does.
func (m *Mind) pick(key string, lines []string) string {
	entries := make([]sayEntry, len(lines))
	for i, l := range lines {
		entries[i] = sayEntry{l}
	}
	if e := m.pickEntry(key, entries); e != nil {
		return e[0]
	}
	return ""
}

// pickEntry chooses an entry of the list key (a rule's say, a question, a dodge), avoiding
// those it chose the last len(entries)/2 times (by their first line): a list of two
// alternates, one of ten says five others before it repeats one.
func (m *Mind) pickEntry(key string, entries []sayEntry) sayEntry {
	if len(entries) == 0 {
		return nil
	}
	used := m.used[key]
	choice := slices.DeleteFunc(slices.Clone(entries), func(e sayEntry) bool { return slices.Contains(used, e[0]) })
	if len(choice) == 0 {
		choice = entries
	}
	e := choice[m.rng.IntN(len(choice))]
	if keep := len(entries) / 2; keep > 0 {
		used = append(used, e[0])
		m.used[key] = slices.Clone(used[max(0, len(used)-keep):])
	}
	return e
}

// fill replaces {from}, {text}, {mood}, {slot.TAG} and {mem.KEY}.
func (m *Mind) fill(l string, e *env) string { return Fill(l, m.fields(l, e)) }

// fields are the values of the fields of l now: {from}, {text}, {mood}, {slot.TAG} and
// {mem.KEY} (an unknown field is empty).
func (m *Mind) fields(l string, e *env) map[string]string {
	out := map[string]string{}
	for {
		i := strings.IndexByte(l, '{')
		j := strings.IndexByte(l, '}')
		if i < 0 || j < i {
			return out
		}
		f := l[i+1 : j]
		switch {
		case f == "from":
			out[f] = e.s.From
		case f == "text":
			out[f] = e.s.Text
		case f == "mood":
			out[f] = fmt.Sprintf("%.0f", m.mood)
		case strings.HasPrefix(f, "slot."):
			out[f] = e.found[f[5:]].Slot
		case strings.HasPrefix(f, "mem."):
			if v, ok := m.mem[f[4:]]; ok {
				out[f] = format(v.Value)
			}
		}
		l = l[j+1:]
	}
}

// Fill replaces each {NAME} of l with fields[NAME] (empty when it has none): a line of a
// reaction, or its translation, with Reaction.Fields.
func Fill(l string, fields map[string]string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(l, '{')
		j := strings.IndexByte(l, '}')
		if i < 0 || j < i {
			b.WriteString(l)
			return b.String()
		}
		b.WriteString(l[:i])
		b.WriteString(fields[l[i+1:j]])
		l = l[j+1:]
	}
}

func format(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%g", f)
	}
	return fmt.Sprint(v)
}

// env gives conditions their names.
type env struct {
	m       *Mind
	s       Stimulus
	found   map[string]Tag
	answer  string
	repeats int
	length  int
	conf    float64
}

type fields func(string) (any, bool)

func (f fields) Field(name string) (any, bool) { return f(name) }

func (e *env) Lookup(name string) (any, bool) {
	switch name {
	case "tag":
		return fields(func(f string) (any, bool) { return e.found[f].Score, true }), true
	case "slot":
		return fields(func(f string) (any, bool) { return e.found[f].Slot, true }), true
	case "mem":
		return fields(func(f string) (any, bool) {
			if v, ok := e.m.mem[f]; ok {
				return v.Value, true
			}
			return 0.0, true
		}), true
	case "value":
		return fields(func(f string) (any, bool) { return e.s.Values[f], true }), true
	case "mood":
		return e.m.mood, true
	case "turn":
		return float64(e.m.turn), true
	case "kind":
		return e.s.Kind, true
	case "from":
		return e.s.From, true
	case "length":
		return float64(e.length), true
	case "asked":
		if e.m.turn > e.m.askTill {
			return "", true
		}
		return e.m.asked, true
	case "answer":
		return e.answer, true
	case "repeats":
		return float64(e.repeats), true
	case "confidence":
		return e.conf, true
	}
	return nil, false
}

// state is what Save keeps.
type state struct {
	Version int                 `json:"version"`
	Person  string              `json:"person"`
	Mood    float64             `json:"mood"`
	Turn    int                 `json:"turn"`
	Mem     map[string]entry    `json:"mem"`
	Cool    map[string]int      `json:"cool"`
	Asked   string              `json:"asked,omitempty"`
	AskTill int                 `json:"ask_till,omitempty"`
	Used    map[string][]string `json:"used,omitempty"`
	Recent  map[string][]string `json:"recent"`
}

// Save returns the mood, memory, cooldowns, pending question, recent texts and the lines
// said lately as JSON.
// The random generator's position is not saved.
func (m *Mind) Save() ([]byte, error) {
	return json.Marshal(state{
		Version: 1, Person: m.p.ID, Mood: m.mood, Turn: m.turn, Mem: m.mem, Cool: m.cool,
		Asked: m.asked, AskTill: m.askTill, Used: m.used, Recent: m.recent,
	})
}

// Restore replaces the state with one that Save returned for the same person. On an
// error the state is unchanged.
func (m *Mind) Restore(b []byte) error {
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("react: restore: %w", err)
	}
	if s.Version != 1 || s.Person != m.p.ID {
		return errors.New("react: restore: the data is not for this person or version")
	}
	if s.Asked != "" && m.ask(s.Asked) == nil {
		return fmt.Errorf("react: restore: unknown ask %q", s.Asked)
	}
	for k, e := range s.Mem {
		if f, ok := e.Value.(float64); ok {
			e.Value = f
		} else if _, ok := e.Value.(string); !ok {
			return fmt.Errorf("react: restore: memory %q is not a string or number", k)
		}
	}
	lo, hi := m.p.bounds()
	m.mood = math.Max(lo, math.Min(hi, s.Mood))
	m.turn, m.asked, m.askTill = s.Turn, s.Asked, s.AskTill
	m.mem, m.cool, m.recent, m.used = orEmpty(s.Mem), orEmpty(s.Cool), orEmpty(s.Recent), orEmpty(s.Used)
	return nil
}

func orEmpty[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}
