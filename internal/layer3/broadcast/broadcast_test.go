package broadcast

import (
	"math/rand/v2"
	"slices"
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
	unlocked := 0.0
	d.Duos = []*data.Duo{{Host: "a", With: "b", Unlock: &unlocked, Shows: []*data.Episode{ep("ab1")}}}
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

// TestTurn: without slot.rest a stream that ended stays over; with it, a channel's next
// stream starts rest minutes after the end, from the episode the player has not seen by
// then, and not before; a duo it may stream with (unlocked, its other host on the air on
// its own, by live.duo_chance) takes both their places, and when the duo ends each host
// streams on its own again.
func TestTurn(t *testing.T) {
	d := streams()
	p := &rpg.Streams{}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible draws
	solo := func(i int) *Show {
		s := newShow(d, d.Channels[i:i+1], d.Channels[i].ID, d.Channels[i].Shows, 0)
		s.Start = noon
		return s
	}
	end := noon.Add(22 * time.Second)
	shows := []*Show{solo(0), solo(1), solo(2)}
	if got := hosts(Turn(d, p, shows, end.Add(time.Hour), rng)); len(got) != 3 || got[0] != "a" {
		t.Fatalf("a stream starts again without slot.rest: %v", got)
	}
	rest := 2.0
	d.Slot.Rest = &rest
	d.Live.DuoChance = 0
	if got := Turn(d, p, shows, end.Add(119*time.Second), rng); got[0] != shows[0] {
		t.Fatal("a stream starts again before its rest has passed")
	}
	MarkSeen(p, shows[0], 0)
	got := Turn(d, p, shows, end.Add(3*time.Minute), rng)
	if a := got[slices.IndexFunc(got, func(s *Show) bool { return s.Key == "a" })]; !a.Start.Equal(end.Add(2*time.Minute)) || a.Episodes[0].Title != "a2" {
		t.Fatalf("the next stream of a %+v", a)
	}
	// a duo: a's stream ends while b streams on its own; the duo takes both their places
	d.Live.DuoChance = 1
	shows = []*Show{solo(0), solo(1), solo(2)}
	shows[1].Start = end // b is on the air when a starts again
	got = Turn(d, p, shows, end.Add(2*time.Minute), rng)
	if h := hosts(got); len(h) != 2 || !slices.Contains(h, "a+b") || !slices.Contains(h, "c") {
		t.Fatalf("the duo does not take a's and b's places: %v", h)
	}
	duo := got[slices.IndexFunc(got, func(s *Show) bool { return s.Key == "a+b" })]
	if !duo.Start.Equal(end.Add(2*time.Minute)) || len(duo.Hosts) != 2 || duo.Episodes[0].Title != "ab1" {
		t.Fatalf("the duo's stream %+v", duo)
	}
	// the duo ends: each host on its own again
	after := duo.Start.Add(time.Duration((duo.Seconds() + rest*60 + 1) * float64(time.Second)))
	if h := hosts(Turn(d, p, []*Show{duo}, after, rng)); len(h) != 2 || !slices.Contains(h, "a") || !slices.Contains(h, "b") {
		t.Fatalf("after the duo: %v", h)
	}
}

// TestDuoUnlock: a duo streams only once each of its hosts' own streams have been watched
// its unlock minutes (30 when left out): not at the opening, nor when a stream ends.
func TestDuoUnlock(t *testing.T) {
	d := streams()
	d.Duos[0].Unlock = nil // 30 minutes
	p := &rpg.Streams{}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible draws
	if h := hosts(Lineup(d, p, noon, rng)); slices.Contains(h, "a+b") {
		t.Fatalf("a locked duo streams at the opening: %v", h)
	}
	Progress(p, "a").Watched, Progress(p, "b").Watched = 30*60, 29*60
	if Unlocked(d, p, d.Duos[0]) {
		t.Fatal("unlocked with b watched 29 minutes")
	}
	Progress(p, "b").Watched = 30 * 60
	if !Unlocked(d, p, d.Duos[0]) || !slices.Contains(hosts(Lineup(d, p, noon, rng)), "a+b") {
		t.Fatal("not unlocked with both watched 30 minutes")
	}
}

// TestTurnGoesOn: a stream of no length (timings too small to count) with no rest does not
// start again at its own start, which would start it again for ever.
func TestTurnGoesOn(t *testing.T) {
	d := streams()
	d.Timing = data.StreamTiming{Say: 1e-12, Chat: 1e-12, Ask: 1e-12}
	rest := 0.0
	d.Slot.Rest = &rest
	s := newShow(d, d.Channels[:1], "a", d.Channels[0].Shows, 0)
	s.Start = noon
	if got := Turn(d, &rpg.Streams{}, []*Show{s}, noon.Add(time.Hour), rand.New(rand.NewPCG(1, 2))); got[0] != s { //nolint:gosec // reproducible draws
		t.Fatalf("a stream of %v seconds starts again at %v", s.Seconds(), got[0].Start)
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

// TestGuest: a guest has no stream of its own and is never on the air but in a duo with its
// host, which unlocks once the host's own streams have been watched its unlock minutes; the
// lineup does not count the guest among the channels min_share asks for, and when the duo
// ends the host streams on its own again.
func TestGuest(t *testing.T) {
	d := streams()
	d.Channels = append(d.Channels, &data.Channel{ID: "g", Guest: true})
	unlock := 1.0
	d.Duos = []*data.Duo{{Host: "a", With: "g", Unlock: &unlock, Shows: d.Duos[0].Shows}}
	d.Live.Share, d.Live.MinShare = 0, 1
	p := &rpg.Streams{Open: 1}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible draws
	if h := hosts(Lineup(d, p, noon, rng)); len(h) != 3 || slices.Contains(h, "g") || slices.Contains(h, "a+g") {
		t.Fatalf("a guest streams, or its locked duo: %v", h)
	}
	Progress(p, "a").Watched = 60
	h := hosts(Lineup(d, p, noon, rng))
	if !slices.Contains(h, "a+g") || slices.Contains(h, "a") || len(h) != 3 {
		t.Fatalf("the guest's duo once a was watched a minute: %v", h)
	}
	rest := 0.0
	d.Slot.Rest = &rest
	duo := Lineup(d, p, noon, rng)
	duo = duo[slices.IndexFunc(duo, func(s *Show) bool { return s.Key == "a+g" }) : slices.IndexFunc(duo, func(s *Show) bool { return s.Key == "a+g" })+1]
	d.Live.DuoChance = 0
	if h := hosts(Turn(d, p, duo, noon.Add(time.Hour), rng)); len(h) != 1 || h[0] != "a" {
		t.Fatalf("after the guest's duo: %v", h)
	}
}
