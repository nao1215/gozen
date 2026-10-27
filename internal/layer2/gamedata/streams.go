package gamedata

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// Streams is the live-stream file named by game.yaml streams (schema_version 9, ADR 0048):
// the channels, their episodes and the rules of the stream screen.
type Streams struct {
	// Timing is how long each kind of beat lasts.
	Timing StreamTiming `yaml:"timing"`
	// Live decides who is live when the game opens.
	Live StreamLive `yaml:"live"`
	// Away is the gold earned while the game is closed.
	Away StreamAway `yaml:"away"`
	// Tips are the amounts the player can tip, smallest first.
	Tips []int `yaml:"tips"`
	// Crowd is the bot viewers of the chat.
	Crowd StreamCrowd `yaml:"crowd"`
	// Channels are the streamers, in the order the lobby lists them.
	Channels []*Channel `yaml:"channels"`
	// Duos are the episodes two channels stream together.
	Duos []*Duo `yaml:"duos"`

	// file is the path of the streams file, for diagnostics and the episodes' directory.
	file string
}

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
	Names    []string `yaml:"names"`
	Lines    []string `yaml:"lines"`
	TipLines []string `yaml:"tip_lines"`
	Rate     float64  `yaml:"rate"`
}

// Channel is one streamer.
type Channel struct {
	ID string `yaml:"id"`
	// Actor is the streamer's actor: its name, its puppet and its reaction book.
	Actor string `yaml:"actor"`
	// Tag is the category shown under the name.
	Tag string `yaml:"tag"`
	// Room is the background picture under assets/.
	Room string `yaml:"room"`
	// Frame is the part of the puppet shown, in puppet pixels.
	Frame StreamFrame `yaml:"frame"`
	// Voice is the speech synthesis of a browser build.
	Voice StreamVoice `yaml:"voice"`
	// Episodes are the episode files, relative to the streams file.
	Episodes []string `yaml:"episodes"`
	// Greet are the lines said to a returning viewer, one list per affection step 0..4.
	Greet [][]string `yaml:"greet"`
	// First is said to a first-time viewer, Streak to one who came days in a row ({days}),
	// Stayed to one who watched to the end; one line of each is chosen.
	First  []string `yaml:"first"`
	Streak []string `yaml:"streak"`
	Stayed []string `yaml:"stayed"`

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

// Duo is the episodes Host streams with With.
type Duo struct {
	Host     string   `yaml:"host"`
	With     string   `yaml:"with"`
	Episodes []string `yaml:"episodes"`

	// Shows are the loaded episodes, in the order of Episodes.
	Shows []*Episode `yaml:"-"`
}

// Episode is one stream: its title, the short lines of its lobby card and its parts.
type Episode struct {
	Title string   `yaml:"title"`
	Lobby []string `yaml:"lobby"`
	Parts [][]Beat `yaml:"parts"`

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

// StreamOption is an answer (C) and the lines said when it is chosen (R, {name} is who chose it).
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

// Channel is the channel with the id, or nil.
func (s *Streams) Channel(id string) *Channel {
	for _, c := range s.Channels {
		if c.ID == id {
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
	files := []string{name}
	var errs []error
	load := func(where string, eps []string) []*Episode {
		var out []*Episode
		for i, ep := range eps {
			p := path.Join(path.Dir(name), ep)
			if !fs.ValidPath(ep) || path.Ext(ep) != ".yaml" {
				errs = append(errs, coded(db.diagnostic(fmt.Sprintf("%s %s.episodes[%d]", name, where, i), fmt.Sprintf("want a relative .yaml file, not %q", ep)), diag.Shape))
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

// minEpisodeSeconds is the length below which an episode is warned of.
const minEpisodeSeconds = 20 * 60

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
	t := s.Timing
	if !(t.Say > 0) || !(t.PerChar >= 0) || !(t.Chat > 0) || !(t.Ask > 0) {
		c.errorf(diag.OutOfRange, at("timing"), "say, chat and ask need more than 0 seconds, per_char 0 or more")
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
	cr := s.Crowd
	if len(cr.Names) == 0 || slices.Contains(cr.Names, "") || len(cr.Lines) == 0 || len(cr.TipLines) == 0 {
		c.errorf(diag.Shape, at("crowd"), "the crowd needs names, lines and tip_lines")
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
		c.file(w+".room", ch.Room)
		if !(ch.Frame.Bottom > ch.Frame.Top) || ch.Frame.Top < 0 {
			c.errorf(diag.OutOfRange, w+".frame", "bottom must be below top, and top 0 or more")
		}
		if v := ch.Voice; v.Pitch < 0 || v.Pitch > 2 || v.Rate < 0 || v.Rate > 10 {
			c.errorf(diag.OutOfRange, w+".voice", "pitch is 0 to 2 and rate 0 to 10 (0 is 1)")
		}
		if len(ch.Greet) > 5 {
			c.errorf(diag.OutOfRange, w+".greet", "one list for each affection step 0 to 4, not %d", len(ch.Greet))
		}
		if len(ch.Episodes) == 0 {
			c.errorf(diag.Shape, w+".episodes", "a channel needs an episode")
		}
		for _, lines := range append(slices.Clone(ch.Greet), ch.First, ch.Stayed) {
			c.streamLines(w, lines, "from")
		}
		c.streamLines(w+".streak", ch.Streak, "from", "days")
		for _, e := range ch.Shows {
			c.episode(e, []string{ch.ID})
		}
	}
	pairs := map[[2]string]bool{}
	for i, d := range s.Duos {
		w := at("duos[%d]", i)
		if d == nil {
			continue
		}
		c.ref(w+".host", "channel", d.Host, ids[d.Host])
		c.ref(w+".with", "channel", d.With, ids[d.With])
		if d.Host == d.With || pairs[[2]string{d.Host, d.With}] {
			c.errorf(diag.Duplicate, w, "a duo is two channels, written once")
		}
		pairs[[2]string{d.Host, d.With}] = true
		if len(d.Episodes) == 0 {
			c.errorf(diag.Shape, w+".episodes", "a duo needs an episode")
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
	if len(e.Lobby) == 0 {
		c.errorf(diag.Shape, at("lobby"), "an episode needs lobby lines for its card")
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
					c.streamLines(fmt.Sprintf("%s.ask.options[%d].r", w, k), o.R, "name")
				}
			}
		}
	}
	if total < minEpisodeSeconds {
		c.warnf(diag.ShortEpisode, at("parts"), "the episode lasts %.0f minutes, less than 20", total/60)
	}
}
