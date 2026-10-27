package gamedata

import (
	"cmp"
	"crypto/sha1" //nolint:gosec // A file name for a line, not security.
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// Streams is the live-stream file named by game.yaml streams (schema_version 9, ADR 0048):
// the channels, their episodes and the rules of the stream screen.
type Streams struct {
	// Logo is the picture under assets/ drawn before the game's title in the bar over the
	// stream screens (a colored square when empty).
	Logo string `yaml:"logo"`
	// Voices is a folder under assets/ of recorded lines: a folder for each channel, the
	// line it says in VOICEKEY.ogg (VoiceKey). A line without its file is read by speech
	// synthesis in a browser build and only captioned elsewhere.
	Voices string `yaml:"voices"`
	// Timing is how long each kind of beat lasts.
	Timing StreamTiming `yaml:"timing"`
	// Slot is how long a channel streams when it is live, its episodes one after another.
	Slot StreamSlot `yaml:"slot"`
	// Live decides who is live when the game opens.
	Live StreamLive `yaml:"live"`
	// Away is the gold earned while the game is closed.
	Away StreamAway `yaml:"away"`
	// Tips are the amounts the player can tip, smallest first.
	Tips []int `yaml:"tips"`
	// Affection is how a viewer earns the affection of a channel.
	Affection StreamAffection `yaml:"affection"`
	// AnswerChance is the chance, by affection step, that a streamer picks the player's
	// answer to a question (one for each step).
	AnswerChance []float64 `yaml:"answer_chance"`
	// Echo is when a streamer reads back a comment it found nothing in.
	Echo StreamEcho `yaml:"echo"`
	// Bounce jiggles a streamer's puppet on the lines that match.
	Bounce StreamBounce `yaml:"bounce"`
	// Crowd is the bot viewers of the chat.
	Crowd StreamCrowd `yaml:"crowd"`
	// Channels are the streamers (the lobby lists the live ones by their viewers).
	Channels []*Channel `yaml:"channels"`
	// Duos are the episodes two channels stream together.
	Duos []*Duo `yaml:"duos"`

	// file is the path of the streams file, for diagnostics.
	file string
	// bounce is Bounce.On compiled (nil: no line bounces).
	bounce *regexp.Regexp
}

// StreamAffection is the points of the affection steps and what earns them.
type StreamAffection struct {
	// Steps are the points each step starts at, from 0, increasing.
	Steps []float64 `yaml:"steps"`
	// Comment is earned by a comment, Reply more when the streamer answers it, Answer by
	// an answer the streamer picks, Stayed by watching to the end, and a point by every
	// TipPer gold of a tip.
	Comment float64 `yaml:"comment"`
	Reply   float64 `yaml:"reply"`
	Answer  float64 `yaml:"answer"`
	TipPer  float64 `yaml:"tip_per"`
	Stayed  float64 `yaml:"stayed"`
}

// StreamSlot is the length of a stream: a live channel plays its episodes one after
// another, from the next the player has not seen, until Minutes have passed, and ends with
// the episode it is in then (60 when left out). With Rest, the channel starts its next
// stream Rest minutes after one ends (0: at once); without it, a channel whose stream
// ended stays offline until the game opens again.
type StreamSlot struct {
	Minutes float64  `yaml:"minutes"`
	Rest    *float64 `yaml:"rest"`
}

// StreamEcho reads back a comment the streamer found nothing in (the reaction book's
// dodge.unknown): only a comment of at most MaxRunes characters, with the chance Chance.
type StreamEcho struct {
	MaxRunes int     `yaml:"max_runes"`
	Chance   float64 `yaml:"chance"`
}

// StreamBounce is a regular expression (On) of the lines that jiggle the streamer's
// puppet, and the strength of the jiggle (Amount; a tip jiggles it too).
type StreamBounce struct {
	On     string  `yaml:"on"`
	Amount float64 `yaml:"amount"`
}

// Bounces reports whether a line jiggles the streamer's puppet.
func (s *Streams) Bounces(line string) bool { return s.bounce != nil && s.bounce.MatchString(line) }

// Files are the streams file and its episodes.
func (s *Streams) Files() []string {
	out := []string{s.file}
	for _, c := range s.Channels {
		for _, e := range c.Shows {
			out = append(out, e.file)
		}
	}
	for _, d := range s.Duos {
		for _, e := range d.Shows {
			out = append(out, e.file)
		}
	}
	return out
}

// MaxStep is the highest affection step.
func (s *Streams) MaxStep() int { return len(s.Affection.Steps) - 1 }

// StreamTiming is in seconds: a line lasts Say plus PerChar a character, a chat beat
// Chat, and a question Ask plus its longest answer.
type StreamTiming struct {
	Say     float64 `yaml:"say"`
	PerChar float64 `yaml:"per_char"`
	Chat    float64 `yaml:"chat"`
	Ask     float64 `yaml:"ask"`
}

// StreamLive is the chance of each channel to be live (Share), the least part of the
// channels live (MinShare), and the chance of a duo when both of its hosts are live.
type StreamLive struct {
	Share     float64 `yaml:"share"`
	MinShare  float64 `yaml:"min_share"`
	DuoChance float64 `yaml:"duo_chance"`
}

// StreamAway is the gold earned an hour while the game is closed, for at most CapHours,
// raised by StarBonus for every affection step of every channel.
type StreamAway struct {
	PerHour   float64 `yaml:"per_hour"`
	CapHours  float64 `yaml:"cap_hours"`
	StarBonus float64 `yaml:"star_bonus"`
}

// StreamCrowd is the bot viewers: their names, what they chat, what they write with a
// tip, and how many messages a second they post on average.
type StreamCrowd struct {
	Names    []string   `yaml:"names"`
	Lines    CrowdLines `yaml:"lines"`
	TipLines []string   `yaml:"tip_lines"`
	Rate     float64    `yaml:"rate"`
}

// CrowdLines are what the bots chat at any hour, and only in the morning (5 to 11
// o'clock) or the evening (17 to 5 o'clock). A plain list is Any.
type CrowdLines struct {
	Any     []string `yaml:"any"`
	Morning []string `yaml:"morning"`
	Evening []string `yaml:"evening"`
}

// UnmarshalYAML reads a list of lines or {any, morning, evening}.
func (l *CrowdLines) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal(&l.Any); err == nil {
		return nil
	}
	type plain CrowdLines
	return unmarshal((*plain)(l))
}

// At are the lines of the hour (0 to 23).
func (l CrowdLines) At(hour int) []string {
	switch {
	case hour >= 5 && hour < 11:
		return slices.Concat(l.Any, l.Morning)
	case hour >= 17 || hour < 5:
		return slices.Concat(l.Any, l.Evening)
	}
	return l.Any
}

// Channel is one streamer.
type Channel struct {
	ID string `yaml:"id"`
	// Actor is the streamer's actor: its name, its puppet and its reaction book.
	Actor string `yaml:"actor"`
	// Tag is the category shown under the name, Tagline a line about the channel.
	Tag     string `yaml:"tag"`
	Tagline string `yaml:"tagline"`
	// Color is the channel's color: its name in the chat and its frame in the lobby.
	Color Color `yaml:"color"`
	// Room is the background picture under assets/.
	Room string `yaml:"room"`
	// Frame is the part of the puppet shown, in puppet pixels.
	Frame StreamFrame `yaml:"frame"`
	// Voice is the speech synthesis of a browser build.
	Voice StreamVoice `yaml:"voice"`
	// Guest marks a streamer of no channel of its own: no stream, no card in the lobby; it
	// streams only as the other host of a duo (its with), and needs no room or episodes.
	Guest bool `yaml:"guest"`
	// Episodes are the episode files, relative to the game.
	Episodes []string `yaml:"episodes"`
	// Greet are the lines said to a returning viewer, one list per affection step 0..4.
	Greet [][]string `yaml:"greet"`
	// First is said to a first-time viewer, Streak to one who came days in a row ({days}),
	// Stayed to one who watched to the end; one line of each is chosen.
	First  []string `yaml:"first"`
	Streak []string `yaml:"streak"`
	Stayed []string `yaml:"stayed"`
	// Bridges are said between two episodes of a stream, one in turn (none when empty).
	Bridges []string `yaml:"bridges"`

	// Shows are the loaded episodes, in the order of Episodes.
	Shows []*Episode `yaml:"-"`
}

// StreamFrame is the part of the puppet a stream shows: centered on CX, from Top to Bottom.
type StreamFrame struct {
	CX     float64 `yaml:"cx"`
	Top    float64 `yaml:"top"`
	Bottom float64 `yaml:"bottom"`
}

// StreamVoice is the pitch and rate of speech synthesis (1 when left out).
type StreamVoice struct {
	Pitch float64 `yaml:"pitch"`
	Rate  float64 `yaml:"rate"`
}

// Duo is the episodes Host streams with With, once each of their own streams has been
// watched Unlock minutes (30 when left out).
type Duo struct {
	Host     string   `yaml:"host"`
	With     string   `yaml:"with"`
	Unlock   *float64 `yaml:"unlock"`
	Episodes []string `yaml:"episodes"`

	// Shows are the loaded episodes, in the order of Episodes.
	Shows []*Episode `yaml:"-"`
}

// Episode is one stream: its title (shown under the watch view), the short lines of its
// lobby card (the channel's tagline or tag when there are none) and its parts. Family marks
// an episode a family is in (a child among the guests): a comment on it gives the reaction
// book value.family 1, for the book's rules to keep to lines fit for it.
type Episode struct {
	Title  string   `yaml:"title"`
	Lobby  []string `yaml:"lobby"`
	Family bool     `yaml:"family"`
	Parts  [][]Beat `yaml:"parts"`

	// file is the episode's path, for diagnostics.
	file string
}

// Beats are the beats of every part, in order.
func (e *Episode) Beats() []Beat { return slices.Concat(e.Parts...) }

// Beat is one step of an episode: a line, a chat message or a question.
type Beat struct {
	// Line is said by the host (say:), or by the channel Speaker (the other host of a
	// duo, written with its channel id as the key).
	Line    *StreamLine `yaml:"say"`
	Speaker string      `yaml:"-"`
	// Chat is a viewer's message [name, text].
	Chat []string `yaml:"chat"`
	// Ask is a question the viewers answer.
	Ask *StreamAsk `yaml:"ask"`
}

// StreamLine is a line with the face it is said with.
type StreamLine struct {
	Face string `yaml:"face"`
	Text string `yaml:"text"`
}

// UnmarshalYAML reads "text" or {face, text}.
func (l *StreamLine) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal(&l.Text); err == nil {
		return nil
	}
	type plain StreamLine
	return unmarshal((*plain)(l))
}

// StreamAsk is a question: Who asks Q (a channel id, the host when left out), the viewers
// choose among Options, and NPC are the answers of bot viewers [name, option index].
type StreamAsk struct {
	Who     string         `yaml:"who"`
	Q       string         `yaml:"q"`
	Options []StreamOption `yaml:"options"`
	NPC     []NPCAnswer    `yaml:"npc"`
}

// StreamOption is an answer (C) and the lines said when it is chosen (R, {from} is who chose it).
type StreamOption struct {
	C string   `yaml:"c"`
	R []string `yaml:"r"`
}

// NPCAnswer is a bot viewer's answer: [name, option index from 0].
type NPCAnswer struct {
	Name   string `yaml:"-"`
	Option int    `yaml:"-"`
}

// UnmarshalYAML reads [name, option].
func (a *NPCAnswer) UnmarshalYAML(unmarshal func(any) error) error {
	var v []any
	if err := unmarshal(&v); err != nil || len(v) != 2 {
		return errors.New("an npc answer is [name, option]")
	}
	name, ok := v[0].(string)
	n, isInt := v[1].(uint64)
	if !ok || !isInt {
		return errors.New("an npc answer is [name, option]")
	}
	a.Name, a.Option = name, int(min(n, 1<<20))
	return nil
}

// UnmarshalYAML reads a one-key map: say, chat, ask, or a channel id.
func (b *Beat) UnmarshalYAML(unmarshal func(any) error) error {
	var m map[string]any
	if err := unmarshal(&m); err != nil || len(m) != 1 {
		return errors.New("a beat is one of say:, chat:, ask: or a channel id")
	}
	var key string
	for k := range m {
		key = k
	}
	switch key {
	case "chat":
		var v struct {
			Chat []string `yaml:"chat"`
		}
		if err := unmarshal(&v); err != nil || len(v.Chat) != 2 {
			return errors.New("chat is [name, text]")
		}
		b.Chat = v.Chat
	case "ask":
		var v struct {
			Ask *StreamAsk `yaml:"ask"`
		}
		if err := unmarshal(&v); err != nil {
			return err
		}
		if v.Ask == nil {
			return errors.New("ask is empty")
		}
		b.Ask = v.Ask
	default:
		var v map[string]StreamLine
		if err := unmarshal(&v); err != nil {
			return err
		}
		l := v[key]
		b.Line = &l
		if key != "say" {
			b.Speaker = key
		}
	}
	return nil
}

// LineSeconds is how long a line of text lasts.
func (t StreamTiming) LineSeconds(text string) float64 {
	return t.Say + t.PerChar*float64(utf8.RuneCountInString(text))
}

// Seconds is how long the beat lasts: a question waits Ask and then says its longest answer.
func (t StreamTiming) Seconds(b Beat) float64 {
	switch {
	case b.Line != nil:
		return t.LineSeconds(b.Line.Text)
	case b.Ask != nil:
		longest := 0.0
		for _, o := range b.Ask.Options {
			sum := 0.0
			for _, r := range o.R {
				sum += t.LineSeconds(r)
			}
			longest = max(longest, sum)
		}
		return t.LineSeconds(b.Ask.Q) + t.Ask + longest
	}
	return t.Chat
}

// VoiceKey names the recorded voice of a stream line: the first 16 hexadecimal digits of
// the SHA-1 of the line in UTF-8, as shown (translated) and before its {fields} are filled
// (a reaction book's line too).
func VoiceKey(line string) string {
	sum := sha1.Sum([]byte(line)) //nolint:gosec // A file name for a line, not security.
	return hex.EncodeToString(sum[:])[:16]
}

// Voice is the file, under the game's files, of the recorded voice of the line the
// channel says (VoiceKey), or "" when the streams have no voices folder or the line is
// empty.
func (s *Streams) Voice(channel, line string) string {
	if s.Voices == "" || line == "" {
		return ""
	}
	return "assets/" + path.Join(s.Voices, channel, VoiceKey(line)+".ogg")
}

// duoUnlock is the minutes of each host's own streams watched a duo needs when unlock is
// left out.
const duoUnlock = 30

// UnlockSeconds is how long each host's own streams must have been watched before the duo
// streams.
func (d *Duo) UnlockSeconds() float64 {
	if d.Unlock == nil {
		return duoUnlock * 60
	}
	return *d.Unlock * 60
}

// Channel is the channel with the id, or nil.
func (s *Streams) Channel(id string) *Channel {
	for _, c := range s.Channels {
		if c != nil && c.ID == id {
			return c
		}
	}
	return nil
}

// readStrict decodes YAML strictly into v, with the error at its line.
func (db *DB) readStrict(name string, v any) error {
	b, err := fs.ReadFile(db.FS, name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	doc, err := db.readSource(b, name)
	if err != nil {
		return err
	}
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, v, yaml.Strict()) }); err != nil {
		return doc.yamlError(err)
	}
	return nil
}

// loadStreams reads the streams file and its episodes. It returns the files read, which
// are not data files.
func (db *DB) loadStreams() ([]string, error) {
	name := db.Game.Streams
	if name == "" {
		return nil, nil
	}
	if !fs.ValidPath(name) || path.Ext(name) != ".yaml" {
		return nil, coded(db.diagnostic("game.yaml streams", fmt.Sprintf("want a relative .yaml file, not %q", name)), diag.Shape)
	}
	s := &Streams{file: name}
	if err := db.readStrict(name, s); err != nil {
		return nil, err
	}
	s.defaults()
	if s.Bounce.On != "" {
		re, err := regexp.Compile(s.Bounce.On)
		if err != nil {
			return nil, coded(db.diagnostic(name+" bounce.on", err.Error()), diag.Shape)
		}
		s.bounce = re
	}
	files := []string{name}
	var errs []error
	load := func(where string, eps []string) []*Episode {
		var out []*Episode
		for i, ep := range eps {
			p := ep
			if !fs.ValidPath(ep) || !strings.HasPrefix(ep, "streams/") || path.Ext(ep) != ".yaml" {
				errs = append(errs, coded(db.diagnostic(fmt.Sprintf("%s %s.episodes[%d]", name, where, i), fmt.Sprintf("want a .yaml file under streams/, not %q", ep)), diag.Shape))
				continue
			}
			e := &Episode{file: p}
			if err := db.readStrict(p, e); err != nil {
				errs = append(errs, err)
				continue
			}
			files = append(files, p)
			out = append(out, e)
		}
		return out
	}
	for i, c := range s.Channels {
		if c != nil {
			c.Shows = load(fmt.Sprintf("channels[%d]", i), c.Episodes)
		}
	}
	for i, d := range s.Duos {
		if d != nil {
			d.Shows = load(fmt.Sprintf("duos[%d]", i), d.Episodes)
		}
	}
	if len(errs) > 0 {
		return nil, joinDiagnostics(errs)
	}
	db.Streams = s
	return files, nil
}

// defaults fills in the affection, the answer chances, the slot and the echo when left out.
func (s *Streams) defaults() {
	a := &s.Affection
	if len(a.Steps) == 0 {
		a.Steps = []float64{0, 30, 150, 600, 3000}
	}
	for _, d := range []struct {
		p   *float64
		def float64
	}{{&a.Comment, 1}, {&a.Reply, 2}, {&a.Answer, 5}, {&a.TipPer, 100}, {&a.Stayed, 10}} {
		if *d.p == 0 {
			*d.p = d.def
		}
	}
	if len(s.AnswerChance) == 0 {
		s.AnswerChance = []float64{0.6, 0.75, 0.9, 1, 1}
	}
	if s.Slot.Minutes == 0 {
		s.Slot.Minutes = 60
	}
	if s.Echo.MaxRunes == 0 {
		s.Echo = StreamEcho{MaxRunes: 24, Chance: cmp.Or(s.Echo.Chance, 0.35)}
	}
}

// minBeat is the fewest seconds a line, a chat beat or a question's options last.
const minBeat = 0.1

// minEpisodeSeconds is the length below which an episode is warned of.
const minEpisodeSeconds = 10 * 60

// streams checks the streams file and its episodes.
func (c *checker) streams() {
	s := c.db.Streams
	if s == nil {
		return
	}
	if !c.needsVersion("game.yaml streams", "streams", SchemaVersion9) {
		return
	}
	at := func(p string, args ...any) string { return s.file + " " + fmt.Sprintf(p, args...) }
	c.file(at("logo"), s.Logo)
	t := s.Timing
	// a beat of less than minBeat would have a stream that ends at once start again so
	// often that catching the streams up would not end
	if !(t.Say >= minBeat) || !(t.PerChar >= 0) || !(t.Chat >= minBeat) || !(t.Ask >= minBeat) {
		c.errorf(diag.OutOfRange, at("timing"), "say, chat and ask need at least 0.1 seconds, per_char 0 or more")
	}
	if m := s.Slot.Minutes; !(m > 0 && m <= 24*60) {
		c.errorf(diag.OutOfRange, at("slot.minutes"), "want more than 0 and at most 1440 minutes, not %g", m)
	}
	if r := s.Slot.Rest; r != nil && !(*r >= 0 && *r <= 24*60) {
		c.errorf(diag.OutOfRange, at("slot.rest"), "want 0 to 1440 minutes, not %g", *r)
	}
	l := s.Live
	for _, v := range []float64{l.Share, l.MinShare, l.DuoChance} {
		if !(v >= 0 && v <= 1) {
			c.errorf(diag.OutOfRange, at("live"), "share, min_share and duo_chance are from 0 to 1")
			break
		}
	}
	if a := s.Away; !(a.PerHour >= 0) || !(a.CapHours >= 0) || !(a.StarBonus >= 0) {
		c.errorf(diag.OutOfRange, at("away"), "per_hour, cap_hours and star_bonus are 0 or more")
	}
	if len(s.Tips) == 0 || s.Tips[0] <= 0 || !slices.IsSorted(s.Tips) || len(slices.Compact(slices.Clone(s.Tips))) != len(s.Tips) {
		c.errorf(diag.OutOfRange, at("tips"), "want one or more amounts above 0, smallest first, each once")
	}
	af := s.Affection
	if af.Steps[0] != 0 || !slices.IsSorted(af.Steps) || len(slices.Compact(slices.Clone(af.Steps))) != len(af.Steps) || af.Comment < 0 || af.Reply < 0 || af.Answer < 0 || af.TipPer <= 0 || af.Stayed < 0 {
		c.errorf(diag.OutOfRange, at("affection"), "steps start at 0 and increase; the points are 0 or more and tip_per above 0")
	}
	if len(s.AnswerChance) != len(af.Steps) || slices.ContainsFunc(s.AnswerChance, func(v float64) bool { return !(v >= 0 && v <= 1) }) {
		c.errorf(diag.OutOfRange, at("answer_chance"), "want a chance from 0 to 1 for each of the %d affection steps", len(af.Steps))
	}
	if s.Echo.MaxRunes < 0 || !(s.Echo.Chance >= 0 && s.Echo.Chance <= 1) {
		c.errorf(diag.OutOfRange, at("echo"), "max_runes is 0 or more and chance from 0 to 1")
	}
	if s.Bounce.Amount < 0 || s.Bounce.Amount > 10 {
		c.errorf(diag.OutOfRange, at("bounce.amount"), "want 0 to 10")
	}
	cr := s.Crowd
	if len(cr.Names) == 0 || slices.Contains(cr.Names, "") || len(cr.Lines.Any) == 0 || len(cr.TipLines) == 0 {
		c.errorf(diag.Shape, at("crowd"), "the crowd needs names, lines (any) and tip_lines")
	}
	if !(cr.Rate > 0 && cr.Rate <= 100) {
		c.errorf(diag.OutOfRange, at("crowd.rate"), "want more than 0 and at most 100 messages a second")
	}
	if len(s.Channels) == 0 {
		c.errorf(diag.Shape, at("channels"), "no channels")
	}
	ids := map[string]bool{}
	for i, ch := range s.Channels {
		w := at("channels[%d]", i)
		if ch == nil || ch.ID == "" || ids[ch.ID] || ch.ID == "say" || ch.ID == "chat" || ch.ID == "ask" {
			c.errorf(diag.Duplicate, w+".id", "a channel needs an id of its own (not say, chat or ask)")
			continue
		}
		ids[ch.ID] = true
		a := c.db.Actors[ch.Actor]
		c.ref(w+".actor", "actor", ch.Actor, a != nil)
		if a != nil && a.Puppet == "" {
			c.errorf(diag.Shape, w+".actor", "actor %q has no puppet to stream with", ch.Actor)
		}
		if !ch.Guest || ch.Room != "" {
			c.file(w+".room", ch.Room)
		}
		if !(ch.Frame.Bottom > ch.Frame.Top) || ch.Frame.Top < 0 {
			c.errorf(diag.OutOfRange, w+".frame", "bottom must be below top, and top 0 or more")
		}
		if v := ch.Voice; v.Pitch < 0 || v.Pitch > 2 || v.Rate < 0 || v.Rate > 10 {
			c.errorf(diag.OutOfRange, w+".voice", "pitch is 0 to 2 and rate 0 to 10 (0 is 1)")
		}
		if len(ch.Greet) > len(af.Steps) {
			c.errorf(diag.OutOfRange, w+".greet", "one list for each affection step 0 to %d, not %d", s.MaxStep(), len(ch.Greet))
		}
		switch {
		case ch.Guest && len(ch.Episodes) > 0:
			c.errorf(diag.Shape, w+".episodes", "a guest has no episodes: it streams only in a duo")
		case !ch.Guest && len(ch.Episodes) == 0:
			c.errorf(diag.Shape, w+".episodes", "a channel needs an episode")
		}
		for _, lines := range append(slices.Clone(ch.Greet), ch.First, ch.Stayed) {
			c.streamLines(w, lines, "from")
		}
		c.streamLines(w+".streak", ch.Streak, "from", "days")
		c.streamLines(w+".bridges", ch.Bridges)
		if slices.Contains(ch.Bridges, "") {
			c.errorf(diag.Shape, w+".bridges", "a bridge needs text")
		}
		for _, e := range ch.Shows {
			c.episode(e, []string{ch.ID})
		}
	}
	c.streamVoices()
	pairs := map[[2]string]bool{}
	for i, d := range s.Duos {
		w := at("duos[%d]", i)
		if d == nil {
			continue
		}
		c.ref(w+".host", "channel", d.Host, ids[d.Host])
		if h := s.Channel(d.Host); h != nil && h.Guest {
			c.errorf(diag.Shape, w+".host", "%s is a guest: a guest hosts no duo, it is its with", d.Host)
		}
		c.ref(w+".with", "channel", d.With, ids[d.With])
		if d.Host == d.With || pairs[[2]string{d.Host, d.With}] {
			c.errorf(diag.Duplicate, w, "a duo is two channels, written once")
		}
		pairs[[2]string{d.Host, d.With}] = true
		if len(d.Episodes) == 0 {
			c.errorf(diag.Shape, w+".episodes", "a duo needs an episode")
		}
		if u := d.Unlock; u != nil && !(*u >= 0 && *u <= 100*60) {
			c.errorf(diag.OutOfRange, w+".unlock", "want 0 to 6000 minutes of each host's streams watched, not %g", *u)
		}
		for _, e := range d.Shows {
			c.episode(e, []string{d.Host, d.With})
		}
	}
}

// streamLines reports a {field} of a line other than those given.
func (c *checker) streamLines(where string, lines []string, fields ...string) {
	for _, l := range lines {
		for _, f := range sortedInterpolations(l) {
			if !slices.Contains(fields, f) {
				c.errorf(diag.UnknownReference, where, "line %q: unknown {%s} (want %v)", l, f, fields)
			}
		}
	}
}

// episode checks an episode streamed by the channels hosts (the first is the host).
func (c *checker) episode(e *Episode, hosts []string) {
	at := func(p string, args ...any) string { return e.file + " " + fmt.Sprintf(p, args...) }
	if e.Title == "" {
		c.errorf(diag.Shape, at("title"), "an episode needs a title")
	}
	if len(e.Beats()) == 0 {
		c.errorf(diag.Shape, at("parts"), "an episode needs beats")
	}
	speaker := func(w, id, face string) {
		if id == "" {
			id = hosts[0]
		}
		if !slices.Contains(hosts, id) {
			c.errorf(diag.UnknownReference, w, "%q is not a host of this episode (%v)", id, hosts)
			return
		}
		ch := c.db.Streams.Channel(id)
		if ch == nil { // a duo's unknown channel, reported with the duo
			return
		}
		a := c.db.Actors[ch.Actor]
		if face == "" || a == nil {
			return
		}
		if faces, ok := c.puppetFaces(ch.Actor, a); ok && !slices.Contains(faces, face) {
			c.errorf(diag.UnknownReference, w+".face", "%s has no face %q", a.Puppet, face)
		}
	}
	total := 0.0
	for i, part := range e.Parts {
		for j, b := range part {
			w := at("parts[%d][%d]", i, j)
			total += c.db.Streams.Timing.Seconds(b)
			switch {
			case b.Line != nil:
				speaker(w, b.Speaker, b.Line.Face)
				if b.Line.Text == "" {
					c.errorf(diag.Shape, w, "a line needs text")
				}
			case b.Ask != nil:
				q := b.Ask
				speaker(w+".ask.who", q.Who, "")
				if len(q.Options) < 2 || len(q.Options) > 4 {
					c.errorf(diag.OutOfRange, w+".ask.options", "want 2 to 4 options, not %d", len(q.Options))
				}
				if len(q.NPC) == 0 {
					c.errorf(diag.Shape, w+".ask.npc", "a question needs a bot viewer's answer")
				}
				for k, n := range q.NPC {
					if n.Option >= len(q.Options) {
						c.errorf(diag.OutOfRange, fmt.Sprintf("%s.ask.npc[%d]", w, k), "option %d is not one of the %d options (from 0)", n.Option, len(q.Options))
					}
				}
				for k, o := range q.Options {
					c.streamLines(fmt.Sprintf("%s.ask.options[%d].r", w, k), o.R, "from")
				}
			}
		}
	}
	if total < minEpisodeSeconds {
		c.warnf(diag.ShortEpisode, at("parts"), "the episode lasts %.0f minutes, less than 10", total/60)
	}
}

// streamVoices warns of a voices folder that is missing, of what in it is not a channel's
// folder, and of a file that is not the voice of a line of its channel (StreamVoice).
func (c *checker) streamVoices() {
	s := c.db.Streams
	if s.Voices == "" {
		return
	}
	where := s.file + " voices"
	dir := "assets/" + s.Voices
	entries, err := fs.ReadDir(c.db.FS, dir)
	if err != nil {
		c.warnf(diag.StreamVoice, where, "no folder %s: the lines are read by speech synthesis in a browser and only captioned elsewhere", dir)
		return
	}
	for _, e := range entries {
		ch := s.Channel(e.Name())
		if !e.IsDir() || ch == nil {
			c.warnf(diag.StreamVoice, where, "%s/%s is not the folder of a channel", dir, e.Name())
			continue
		}
		keys := c.voiceKeys(ch)
		files, err := fs.ReadDir(c.db.FS, dir+"/"+ch.ID)
		if err != nil {
			c.warnf(diag.StreamVoice, where, "%s/%s: %v", dir, ch.ID, err)
			continue
		}
		for _, f := range files {
			if key, ok := strings.CutSuffix(f.Name(), ".ogg"); !ok || f.IsDir() || !keys[key] {
				c.warnf(diag.StreamVoice, where, "%s/%s/%s is the voice of no line of %s", dir, ch.ID, f.Name(), ch.ID)
			}
		}
	}
}

// VoiceWarnings are the warnings of the recorded stream voices of lines that gozen cannot
// play, not Ogg Vorbis (GZN0120). gozen check reads the head of each voice for them, which
// loading the game does not: a browser fetches a voice only when its line comes (ADR 0051).
func (db *DB) VoiceWarnings() []error {
	s := db.Streams
	if s == nil || s.Voices == "" {
		return nil
	}
	c := &checker{db: db}
	dir := "assets/" + s.Voices
	for _, ch := range s.Channels {
		if ch == nil {
			continue
		}
		files, err := fs.ReadDir(db.FS, dir+"/"+ch.ID)
		if err != nil {
			continue
		}
		keys := c.voiceKeys(ch)
		for _, f := range files {
			name := dir + "/" + ch.ID + "/" + f.Name()
			if key, ok := strings.CutSuffix(f.Name(), ".ogg"); !ok || f.IsDir() || !keys[key] {
				continue // GZN0119
			}
			if format := oggCodec(db.FS, name); format != "vorbis" {
				c.warnf(diag.VoiceFormat, s.file+" voices", "%s is %s: gozen plays Ogg Vorbis; convert it", name, format)
			}
		}
	}
	return sortedDiagnostics(c.warns)
}

// oggCodec is "vorbis" for an Ogg Vorbis file, else what the file is ("Ogg Opus", "not an
// Ogg file"), read from its first page: the identification header of the first packet.
func oggCodec(fsys fs.FS, name string) string {
	f, err := fsys.Open(name)
	if err != nil {
		return "unreadable"
	}
	defer f.Close() //nolint:errcheck // Read only.
	b := make([]byte, 27+255+8)
	n, err := io.ReadFull(f, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) { // a short file is read whole
		return "unreadable"
	}
	b = b[:n]
	if len(b) < 27 || string(b[:4]) != "OggS" {
		return "not an Ogg file"
	}
	packet := b[min(len(b), 27+int(b[26])):]
	switch {
	case strings.HasPrefix(string(packet), "\x01vorbis"):
		return "vorbis"
	case strings.HasPrefix(string(packet), "OpusHead"):
		return "Ogg Opus"
	}
	return "an Ogg file of another codec"
}

// voiceKeys are the voice keys of the lines the channel says, in every language offered:
// its greetings, its lines and questions in its episodes and duos, and its reaction book's.
func (c *checker) voiceKeys(ch *Channel) map[string]bool {
	s := c.db.Streams
	lines := slices.Concat(ch.First, ch.Streak, ch.Stayed, ch.Bridges)
	for _, g := range ch.Greet {
		lines = append(lines, g...)
	}
	says := func(eps []*Episode, host string) {
		for _, e := range eps {
			for _, b := range e.Beats() {
				switch {
				case b.Line != nil && cmp.Or(b.Speaker, host) == ch.ID:
					lines = append(lines, b.Line.Text)
				case b.Ask != nil && cmp.Or(b.Ask.Who, host) == ch.ID:
					lines = append(lines, b.Ask.Q)
					for _, o := range b.Ask.Options {
						lines = append(lines, o.R...)
					}
				}
			}
		}
	}
	says(ch.Shows, ch.ID)
	for _, d := range s.Duos {
		if d != nil && (d.Host == ch.ID || d.With == ch.ID) {
			says(d.Shows, d.Host)
		}
	}
	if b := c.db.ReactionBooks[ch.Actor]; b != nil {
		lines = append(lines, b.Lines(ch.Actor)...)
	}
	keys := map[string]bool{}
	for _, code := range slices.Concat(c.db.Game.Offered(), []string{c.db.Game.Written()}) {
		for _, l := range lines {
			keys[VoiceKey(c.db.TrIn(code, l))] = true
		}
	}
	return keys
}
