// Package broadcast is the rules of the live streams (ADR 0048), without drawing: the
// episodes a stream plays one after another (ADR 0052), when each beat plays, who is live, which episode each channel plays, when an
// episode counts as seen, the affection steps, the days in a row and the gold earned
// while the game was closed. The clock and the random source are passed in, so the
// rules run headless. The stream screen of internal/play uses them.
package broadcast

import (
	"math"
	"math/rand/v2"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// Show is a stream on the air: the episodes of a slot one after another.
type Show struct {
	// Hosts are the channels streaming it, the host first (two for a duo).
	Hosts []*data.Channel
	// Key is the progress key: the channel id, or DuoKey for a duo.
	Key string
	// Episodes are the episodes it plays in order, the first the one at First among the
	// channel's (or the duo's) episodes, the next ones following it round.
	Episodes []*data.Episode
	First    int
	// Beats are the beats of the episodes, a bridge line of the host before each episode
	// but the first, Starts the beat each episode begins at (its bridge, when it has one),
	// and Times when each beat starts, in seconds after Start; Times has one more entry,
	// the end of the stream.
	Beats  []data.Beat
	Starts []int
	Times  []float64
	Start  time.Time
}

// DuoKey is the progress key of a duo.
func DuoKey(host, with string) string { return host + "+" + with }

// newShow times the slot of the key: its episodes from the next the player has not seen,
// one after another with the host's bridges between them, until slot.minutes have passed;
// the stream ends with the episode it is in then.
func newShow(d *data.Streams, hosts []*data.Channel, key string, shows []*data.Episode, next int) *Show {
	s := &Show{Hosts: hosts, Key: key, First: next}
	t := 0.0
	add := func(b data.Beat) {
		s.Beats = append(s.Beats, b)
		s.Times = append(s.Times, t)
		t += d.Timing.Seconds(b)
	}
	bridges := hosts[0].Bridges
	for i := 0; i == 0 || t < d.Slot.Minutes*60; i++ {
		before := t
		s.Starts = append(s.Starts, len(s.Beats))
		if i > 0 && len(bridges) > 0 {
			add(data.Beat{Line: &data.StreamLine{Text: bridges[(next+i-1)%len(bridges)]}})
		}
		ep := shows[(next+i)%len(shows)]
		s.Episodes = append(s.Episodes, ep)
		for _, b := range ep.Beats() {
			add(b)
		}
		if t == before { // an episode without beats would never fill the slot
			break
		}
	}
	s.Times = append(s.Times, t)
	return s
}

// Part is the episode playing at the beat, an index of Episodes.
func (s *Show) Part(beat int) int {
	i := 0
	for i+1 < len(s.Starts) && s.Starts[i+1] <= beat {
		i++
	}
	return i
}

// PartEnd is the beat after the last of the episode part.
func (s *Show) PartEnd(part int) int {
	if part+1 < len(s.Starts) {
		return s.Starts[part+1]
	}
	return len(s.Beats)
}

// Seconds is how long the episode lasts.
func (s *Show) Seconds() float64 { return s.Times[len(s.Times)-1] }

// Live reports whether the episode is still on at now.
func (s *Show) Live(now time.Time) bool { return now.Sub(s.Start).Seconds() < s.Seconds() }

// BeatAt is the beat playing at now: 0 before the start, len(Beats) after the end.
func (s *Show) BeatAt(now time.Time) int {
	t := now.Sub(s.Start).Seconds()
	i := 0
	for i < len(s.Beats) && s.Times[i+1] <= t {
		i++
	}
	return i
}

// maxLate is how long before the game opens an episode may have started.
const maxLate = 4 * time.Minute

// Lineup decides who is live at now: every channel with an episode when the game opens
// for the first time (p.Open is 0), else each with the chance live.share and at least
// live.min_share of them, in the order written. Two live hosts of a duo stream it
// together with the chance live.duo_chance. Each show plays its slot from the next
// episode the player has not seen and started up to four minutes ago, before its middle.
// The shows are in the order of their hosts.
func Lineup(d *data.Streams, p *rpg.Streams, now time.Time, rng *rand.Rand) []*Show {
	first := p.Open == 0
	live := map[string]bool{}
	n := 0
	for _, c := range d.Channels {
		if len(c.Shows) > 0 && (first || rng.Float64() < d.Live.Share) {
			live[c.ID] = true
			n++
		}
	}
	want := int(math.Ceil(d.Live.MinShare * float64(len(d.Channels))))
	for _, c := range d.Channels {
		if n < want && !live[c.ID] && len(c.Shows) > 0 {
			live[c.ID] = true
			n++
		}
	}
	// late starts a show up to four minutes ago, and before its middle.
	late := func(s *Show) *Show {
		s.Start = now.Add(-time.Duration(rng.Float64() * min(float64(maxLate), s.Seconds()/2*float64(time.Second))))
		return s
	}
	paired := map[string]*Show{}
	for _, duo := range d.Duos {
		if !live[duo.Host] || !live[duo.With] || paired[duo.Host] != nil || paired[duo.With] != nil || len(duo.Shows) == 0 || rng.Float64() >= d.Live.DuoChance {
			continue
		}
		key := DuoKey(duo.Host, duo.With)
		s := late(newShow(d, []*data.Channel{d.Channel(duo.Host), d.Channel(duo.With)}, key, duo.Shows, p.Duos[key]))
		paired[duo.Host], paired[duo.With] = s, s
	}
	var out []*Show
	for _, c := range d.Channels {
		switch s := paired[c.ID]; {
		case !live[c.ID]:
		case s != nil:
			if s.Hosts[0] == c {
				out = append(out, s)
			}
		default:
			out = append(out, late(newShow(d, []*data.Channel{c}, c.ID, c.Shows, Progress(p, c.ID).Next)))
		}
	}
	return out
}

// Progress is the channel's progress in p, made when there is none.
func Progress(p *rpg.Streams, id string) *rpg.ChannelProgress {
	if p.Channels == nil {
		p.Channels = map[string]*rpg.ChannelProgress{}
	}
	c := p.Channels[id]
	if c == nil {
		c = &rpg.ChannelProgress{}
		p.Channels[id] = c
	}
	return c
}

// SeenShare is the part of an episode's beats from where the player came in that marks
// it seen.
const SeenShare = 0.6

// Seen reports whether watching watched beats of an episode of total beats, coming in at
// beat from, sees it.
func Seen(watched, from, total int) bool {
	return float64(watched) >= SeenShare*float64(total-from)
}

// MarkSeen marks the episode part of the show seen: the channel or the duo goes on to the
// episode after it (the episodes before it that the player came in too late for stay
// behind, as a stream does not wait).
func MarkSeen(p *rpg.Streams, s *Show, part int) {
	next := s.First + part + 1
	if len(s.Hosts) > 1 {
		if p.Duos == nil {
			p.Duos = map[string]int{}
		}
		p.Duos[s.Key] = max(p.Duos[s.Key], next)
		return
	}
	pr := Progress(p, s.Key)
	pr.Next = max(pr.Next, next)
}

// Step is the affection step of the points, 0 to the streams file's MaxStep.
func Step(d *data.Streams, points float64) int {
	s := 0
	for i, at := range d.Affection.Steps {
		if points >= at {
			s = i
		}
	}
	return s
}

// PicksAnswer reports whether a streamer of the affection step picks the player's
// answer to a question (answer_chance), with the draw roll in [0, 1).
func PicksAnswer(d *data.Streams, step int, roll float64) bool { return roll < d.AnswerChance[step] }

// Day is the calendar day of t where t is, counted from 1970-01-01.
func Day(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// Visit records a visit to a channel at now: it counts the visit, and the days in a row
// grow on the next day, start again after a day missed, and stay on the same day. It
// reports whether it is the first visit ever.
func Visit(c *rpg.ChannelProgress, now time.Time) bool {
	first := c.Day == 0
	c.Visits++
	switch today := Day(now); {
	case today == c.Day:
	case c.Day > 0 && today == c.Day+1:
		c.Day, c.Run = today, c.Run+1
	default:
		c.Day, c.Run = today, 1
	}
	return first
}

// StreakDays is the days in a row from which a streamer mentions the streak.
const StreakDays = 3

// Settle is the gold earned while the game was closed, from p.Open to now: away.per_hour
// an hour for at most away.cap_hours, raised by away.star_bonus for every affection step
// of every channel. Nothing is earned on the first opening or after less than a minute.
// It sets p.Open to now.
func Settle(d *data.Streams, p *rpg.Streams, now time.Time) int {
	last := p.Open
	p.Open = now.Unix()
	away := float64(p.Open-last) / 3600
	if last == 0 || away < 1.0/60 {
		return 0
	}
	stars := 0
	for _, c := range d.Channels {
		if pr := p.Channels[c.ID]; pr != nil {
			stars += Step(d, pr.Points)
		}
	}
	return int(d.Away.PerHour * min(away, d.Away.CapHours) * (1 + d.Away.StarBonus*float64(stars)))
}
