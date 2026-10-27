// Package broadcast is the rules of the live streams (ADR 0048), without drawing: when
// each beat of an episode plays, who is live, which episode each channel plays, when an
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

// Show is an episode on the air.
type Show struct {
	// Hosts are the channels streaming it, the host first (two for a duo).
	Hosts []*data.Channel
	// Key is the progress key: the channel id, or DuoKey for a duo.
	Key     string
	Episode *data.Episode
	// Beats are the episode's beats and Times when each starts, in seconds after
	// Start; Times has one more entry, the end of the episode.
	Beats []data.Beat
	Times []float64
	Start time.Time
}

// DuoKey is the progress key of a duo.
func DuoKey(host, with string) string { return host + "+" + with }

// newShow times the episode of the key that the player has not seen yet.
func newShow(d *data.Streams, hosts []*data.Channel, key string, shows []*data.Episode, next int, start time.Time) *Show {
	ep := shows[next%len(shows)]
	s := &Show{Hosts: hosts, Key: key, Episode: ep, Beats: ep.Beats(), Start: start}
	t := 0.0
	for _, b := range s.Beats {
		s.Times = append(s.Times, t)
		t += d.Timing.Seconds(b)
	}
	s.Times = append(s.Times, t)
	return s
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
// together with the chance live.duo_chance. Each show plays the next episode the
// player has not seen and started up to four minutes ago. The shows are in the order of
// their hosts.
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
	start := func() time.Time { return now.Add(-time.Duration(rng.Float64() * float64(maxLate))) }
	paired := map[string]*Show{}
	for _, duo := range d.Duos {
		if !live[duo.Host] || !live[duo.With] || paired[duo.Host] != nil || paired[duo.With] != nil || len(duo.Shows) == 0 || rng.Float64() >= d.Live.DuoChance {
			continue
		}
		key := DuoKey(duo.Host, duo.With)
		s := newShow(d, []*data.Channel{d.Channel(duo.Host), d.Channel(duo.With)}, key, duo.Shows, p.Duos[key], start())
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
			out = append(out, newShow(d, []*data.Channel{c}, c.ID, c.Shows, Progress(p, c.ID).Next, start()))
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

// SeenShare is the part of the beats from where the player came in that marks an
// episode seen.
const SeenShare = 0.6

// Seen reports whether watching watched beats of an episode of total beats, coming in at
// beat from, sees it.
func Seen(watched, from, total int) bool {
	return float64(watched) >= SeenShare*float64(total-from)
}

// MarkSeen moves the show's channel or duo on to its next episode.
func MarkSeen(p *rpg.Streams, s *Show) {
	if len(s.Hosts) > 1 {
		if p.Duos == nil {
			p.Duos = map[string]int{}
		}
		p.Duos[s.Key]++
		return
	}
	Progress(p, s.Key).Next++
}

// Points a viewer earns with a channel.
const (
	// CommentPoints is for a comment; ReplyPoints more when the streamer answers it.
	CommentPoints = 1
	ReplyPoints   = 2
	// AnswerPoints is for an answer the streamer picks; StayPoints for watching an
	// episode to its end.
	AnswerPoints = 5
	StayPoints   = 10
	// TipPerPoint is the gold of a tip that earns one point.
	TipPerPoint = 100
)

// steps are the points each affection step starts at.
var steps = [...]float64{0, 30, 150, 600, 3000}

// MaxStep is the highest affection step.
const MaxStep = len(steps) - 1

// Step is the affection step of the points, 0 to MaxStep.
func Step(points float64) int {
	s := 0
	for i, at := range steps {
		if points >= at {
			s = i
		}
	}
	return s
}

// answerChance is the chance a streamer picks the player's answer, by affection step.
var answerChance = [...]float64{0.6, 0.75, 0.9, 1, 1}

// PicksAnswer reports whether a streamer of the affection step picks the player's
// answer to a question, with the draw roll in [0, 1).
func PicksAnswer(step int, roll float64) bool { return roll < answerChance[step] }

// Day is the calendar day of t where t is, counted from 1970-01-01.
func Day(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// Visit records a visit to a channel at now: the days in a row grow on the next day,
// start again after a day missed, and stay on the same day. It reports whether it is
// the first visit ever.
func Visit(c *rpg.ChannelProgress, now time.Time) bool {
	first := c.Day == 0
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
			stars += Step(pr.Points)
		}
	}
	return int(d.Away.PerHour * min(away, d.Away.CapHours) * (1 + d.Away.StarBonus*float64(stars)))
}
