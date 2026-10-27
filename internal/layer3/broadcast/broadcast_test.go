package broadcast

import (
	"math/rand/v2"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

func line(text string) data.Beat { return data.Beat{Line: &data.StreamLine{Text: text}} }

// streams has three channels of two episodes and a duo of the first two.
func streams() *data.Streams {
	ep := func(title string) *data.Episode {
		return &data.Episode{Title: title, Parts: [][]data.Beat{
			{line("abcd"), {Chat: []string{"viewer", "hi"}}},
			{{Ask: &data.StreamAsk{Q: "ab", Options: []data.StreamOption{{C: "x", R: []string{"abcdef"}}, {C: "y", R: []string{"a", "b"}}}}}, line("")},
		}}
	}
	d := &data.Streams{
		Timing:       data.StreamTiming{Say: 1, PerChar: 0.5, Chat: 2, Ask: 10},
		Live:         data.StreamLive{Share: 0.5, MinShare: 0.6, DuoChance: 1},
		Away:         data.StreamAway{PerHour: 100, CapHours: 24, StarBonus: 0.1},
		Affection:    data.StreamAffection{Steps: []float64{0, 30, 150, 600, 3000}},
		AnswerChance: []float64{0.6, 0.75, 0.9, 1, 1},
	}
	for _, id := range []string{"a", "b", "c"} {
		d.Channels = append(d.Channels, &data.Channel{ID: id, Shows: []*data.Episode{ep(id + "1"), ep(id + "2")}})
	}
	d.Duos = []*data.Duo{{Host: "a", With: "b", Shows: []*data.Episode{ep("ab1")}}}
	return d
}

var noon = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func TestTimesAndBeatAt(t *testing.T) {
	d := streams()
	s := newShow(d, d.Channels[:1], "a", d.Channels[0].Shows, 0)
	s.Start = noon
	// say 1+4*0.5, chat 2, ask (1+2*0.5)+10+max(1+6*0.5, 2*1+2*0.5), say 1
	want := []float64{0, 3, 5, 21, 22}
	if len(s.Times) != len(want) {
		t.Fatalf("times %v", s.Times)
	}
	for i := range want {
		if s.Times[i] != want[i] {
			t.Fatalf("times %v, want %v", s.Times, want)
		}
	}
	for _, tc := range []struct {
		at   float64
		beat int
		live bool
	}{{-5, 0, true}, {0, 0, true}, {2.9, 0, true}, {3, 1, true}, {20, 2, true}, {21.5, 3, true}, {22, 4, false}, {100, 4, false}} {
		now := noon.Add(time.Duration(tc.at * float64(time.Second)))
		if got := s.BeatAt(now); got != tc.beat {
			t.Errorf("%gs: beat %d, want %d", tc.at, got, tc.beat)
		}
		if got := s.Live(now); got != tc.live {
			t.Errorf("%gs: live %v", tc.at, got)
		}
	}
}

func hosts(shows []*Show) []string {
	var out []string
	for _, s := range shows {
		out = append(out, s.Key)
	}
	return out
}

func TestLineup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		open  int64
		share float64
		duo   float64
		want  []string
	}{
		{"first opening: everyone, a duo", 0, 0, 1, []string{"a+b", "c"}},
		{"first opening: everyone, no duo", 0, 0, 0, []string{"a", "b", "c"}},
		{"later, nobody drawn: min_share fills in order", 1, 0, 0, []string{"a", "b"}},
		{"later, everyone drawn", 1, 1, 0, []string{"a", "b", "c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := streams()
			d.Live.Share, d.Live.DuoChance = tc.share, tc.duo
			shows := Lineup(d, &rpg.Streams{Open: tc.open}, noon, rand.New(rand.NewPCG(1, 2))) //nolint:gosec // reproducible draws
			if got := hosts(shows); len(got) != len(tc.want) || func() bool {
				for i := range got {
					if got[i] != tc.want[i] {
						return true
					}
				}
				return false
			}() {
				t.Fatalf("live %v, want %v", got, tc.want)
			}
			for _, s := range shows {
				if late := noon.Sub(s.Start); late < 0 || late > maxLate || late.Seconds() > s.Seconds()/2 {
					t.Errorf("%s started %v before", s.Key, late)
				}
			}
		})
	}
}

func TestNextEpisodeAndSeen(t *testing.T) {
	d := streams()
	d.Live.DuoChance = 0
	p := &rpg.Streams{}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible draws
	shows := Lineup(d, p, noon, rng)
	if shows[0].Episodes[0].Title != "a1" {
		t.Fatalf("first episode %q", shows[0].Episodes[0].Title)
	}
	MarkSeen(p, shows[0], 0)
	MarkSeen(p, shows[0], 0) // seen again: no farther
	if p.Channels["a"].Next != 1 {
		t.Fatalf("next %d after the first seen twice", p.Channels["a"].Next)
	}
	MarkSeen(p, &Show{Hosts: shows[0].Hosts, Key: "a", First: 1}, 1) // the third, the second missed
	if got := Lineup(d, p, noon, rng)[0].Episodes[0].Title; got != "a2" || p.Channels["a"].Next != 3 {
		t.Errorf("after the third, the episode %q (two wrap round)", got)
	}
	duo := &Show{Hosts: []*data.Channel{d.Channels[0], d.Channels[1]}, Key: DuoKey("a", "b")}
	MarkSeen(p, duo, 0)
	if p.Duos["a+b"] != 1 || p.Channels["a"].Next != 3 {
		t.Errorf("a duo moves on by itself: %v %v", p.Duos, p.Channels["a"])
	}
	for _, tc := range []struct {
		watched, from, total int
		seen                 bool
	}{{6, 0, 10, true}, {5, 0, 10, false}, {3, 5, 10, true}, {2, 5, 10, false}, {0, 10, 10, true}} {
		if got := Seen(tc.watched, tc.from, tc.total); got != tc.seen {
			t.Errorf("Seen(%d, %d, %d) = %v", tc.watched, tc.from, tc.total, got)
		}
	}
}

func TestStepAndAnswers(t *testing.T) {
	d := streams()
	for _, tc := range []struct {
		points float64
		step   int
	}{{0, 0}, {29.9, 0}, {30, 1}, {149, 1}, {150, 2}, {600, 3}, {2999, 3}, {3000, 4}, {1e9, 4}} {
		if got := Step(d, tc.points); got != tc.step {
			t.Errorf("Step(%g) = %d, want %d", tc.points, got, tc.step)
		}
	}
	if PicksAnswer(d, 0, 0.6) || !PicksAnswer(d, 0, 0.59) || !PicksAnswer(d, 4, 0.999) {
		t.Error("answer chances")
	}
}

// TestVisitCountsDaysInARow: every visit is counted, the days in a row grow on the next
// day, stay on the same day and start again after a day missed.
func TestVisitCountsDaysInARow(t *testing.T) {
	c := &rpg.ChannelProgress{}
	day := func(n int, h int) time.Time { return noon.AddDate(0, 0, n).Add(time.Duration(h) * time.Hour) }
	for _, tc := range []struct {
		at    time.Time
		first bool
		run   int
	}{
		{day(0, 0), true, 1},
		{day(0, 5), false, 1},
		{day(1, -11), false, 2},
		{day(2, 0), false, 3},
		{day(4, 0), false, 1},
	} {
		if first := Visit(c, tc.at); first != tc.first || c.Run != tc.run {
			t.Errorf("%v: first %v run %d, want %v %d", tc.at, first, c.Run, tc.first, tc.run)
		}
	}
	if c.Visits != 5 {
		t.Errorf("visits %d, want 5", c.Visits)
	}
}

func TestSettle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		away   time.Duration
		points float64
		gold   int
	}{
		{"first opening", -1, 0, 0},
		{"under a minute", 59 * time.Second, 0, 0},
		{"two hours", 2 * time.Hour, 0, 200},
		{"capped at a day", 48 * time.Hour, 0, 2400},
		{"two steps of bonus", 2 * time.Hour, 150, 240},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &rpg.Streams{Channels: map[string]*rpg.ChannelProgress{"a": {Points: tc.points}}}
			if tc.away >= 0 {
				p.Open = noon.Add(-tc.away).Unix()
			}
			if got := Settle(streams(), p, noon); got != tc.gold {
				t.Errorf("gold %d, want %d", got, tc.gold)
			}
			if p.Open != noon.Unix() {
				t.Errorf("open %d", p.Open)
			}
		})
	}
}

// TestSlot: a stream plays the episodes from the next unseen one after another, wrapping
// round, a bridge of the host before each but the first, until the slot has passed, and
// ends with the episode it is in then; Part and PartEnd find the episode of a beat.
func TestSlot(t *testing.T) {
	d := streams()
	d.Slot.Minutes = 1 // 60 s: three episodes of 22 s (and a bridge of 1 s before the 2nd and 3rd)
	c := d.Channels[0]
	c.Bridges = []string{"", ""}
	s := newShow(d, d.Channels[:1], "a", c.Shows, 1)
	titles := []string{}
	for _, e := range s.Episodes {
		titles = append(titles, e.Title)
	}
	if len(titles) != 3 || titles[0] != "a2" || titles[1] != "a1" || titles[2] != "a2" || s.First != 1 {
		t.Fatalf("episodes %v from %d", titles, s.First)
	}
	if len(s.Starts) != 3 || s.Starts[1] != 4 || s.Beats[4].Line == nil || s.Starts[2] != 9 || len(s.Beats) != 14 || s.Seconds() != 22*3+2 {
		t.Fatalf("starts %v, %d beats, %g s", s.Starts, len(s.Beats), s.Seconds())
	}
	for beat, part := range map[int]int{0: 0, 3: 0, 4: 1, 8: 1, 9: 2, 13: 2} {
		if got := s.Part(beat); got != part {
			t.Errorf("beat %d in episode %d, want %d", beat, got, part)
		}
	}
	if s.PartEnd(0) != 4 || s.PartEnd(2) != 14 {
		t.Errorf("ends %d %d", s.PartEnd(0), s.PartEnd(2))
	}
	c.Bridges = nil
	if s := newShow(d, d.Channels[:1], "a", c.Shows, 0); len(s.Beats) != 12 || s.Seconds() != 66 {
		t.Errorf("without bridges: %d beats, %g s", len(s.Beats), s.Seconds())
	}
}

// TestAgain: without slot.rest a stream that ended stays over; with it, the next stream of
// the same channel or duo starts rest minutes after the end, from the episode the player has
// not seen by then, and not before.
func TestAgain(t *testing.T) {
	d := streams()
	p := &rpg.Streams{}
	s := newShow(d, d.Channels[:1], "a", d.Channels[0].Shows, 0)
	s.Start = noon
	end := noon.Add(22 * time.Second)
	if Again(d, p, s, end.Add(time.Hour)) != nil {
		t.Fatal("a stream starts again without slot.rest")
	}
	rest := 2.0
	d.Slot.Rest = &rest
	if Again(d, p, s, end.Add(119*time.Second)) != nil {
		t.Fatal("a stream starts again before its rest has passed")
	}
	MarkSeen(p, s, 0)
	n := Again(d, p, s, end.Add(3*time.Minute))
	if n == nil || !n.Start.Equal(end.Add(2*time.Minute)) || n.Episodes[0].Title != "a2" || n.Key != "a" {
		t.Fatalf("the next stream %+v", n)
	}
	duo := newShow(d, []*data.Channel{d.Channels[0], d.Channels[1]}, DuoKey("a", "b"), d.Duos[0].Shows, 0)
	duo.Start = noon
	if n := Again(d, p, duo, end.Add(2*time.Minute)); n == nil || len(n.Hosts) != 2 || n.Episodes[0].Title != "ab1" {
		t.Fatalf("the duo's next stream %+v", n)
	}
}

// TestMarkSeenAtTheLimit: past the highest next episode a save keeps, a channel or a duo
// takes its episodes round from the one after the episode seen, so its save loads again and
// it still goes on to the next episode.
func TestMarkSeenAtTheLimit(t *testing.T) {
	d := streams()
	key := DuoKey("a", "b")
	p := &rpg.Streams{Channels: map[string]*rpg.ChannelProgress{"a": {Next: rpg.MaxNextEpisode}}, Duos: map[string]int{key: rpg.MaxNextEpisode}}
	solo := newShow(d, d.Channels[:1], "a", d.Channels[0].Shows, rpg.MaxNextEpisode)
	duo := newShow(d, []*data.Channel{d.Channels[0], d.Channels[1]}, key, d.Duos[0].Shows, rpg.MaxNextEpisode)
	MarkSeen(p, solo, 0)
	MarkSeen(p, duo, 0)
	n, nd := len(d.Channels[0].Shows), len(d.Duos[0].Shows)
	if got := p.Channels["a"].Next; got != (rpg.MaxNextEpisode+1)%n {
		t.Errorf("the channel goes on to %d, not %d", got, (rpg.MaxNextEpisode+1)%n)
	}
	if got := p.Duos[key]; got != (rpg.MaxNextEpisode+1)%nd {
		t.Errorf("the duo goes on to %d, not %d", got, (rpg.MaxNextEpisode+1)%nd)
	}
	if next := newShow(d, d.Channels[:1], "a", d.Channels[0].Shows, p.Channels["a"].Next); next.Episodes[0] != d.Channels[0].Shows[(rpg.MaxNextEpisode+1)%n] {
		t.Errorf("the next stream starts with %q", next.Episodes[0].Title)
	}
}
