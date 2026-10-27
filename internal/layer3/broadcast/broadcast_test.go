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
		Timing: data.StreamTiming{Say: 1, PerChar: 0.5, Chat: 2, Ask: 10},
		Live:   data.StreamLive{Share: 0.5, MinShare: 0.6, DuoChance: 1},
		Away:   data.StreamAway{PerHour: 100, CapHours: 24, StarBonus: 0.1},
	}
	for _, id := range []string{"a", "b", "c"} {
		d.Channels = append(d.Channels, &data.Channel{ID: id, Shows: []*data.Episode{ep(id + "1"), ep(id + "2")}})
	}
	d.Duos = []*data.Duo{{Host: "a", With: "b", Shows: []*data.Episode{ep("ab1")}}}
	return d
}

var noon = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func TestTimesAndBeatAt(t *testing.T) {
	s := newShow(streams(), nil, "a", streams().Channels[0].Shows, 0, noon)
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
			shows := Lineup(d, &rpg.Streams{Open: tc.open}, noon, rand.New(rand.NewPCG(1, 2)))
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
				if late := noon.Sub(s.Start); late < 0 || late > maxLate {
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
	rng := rand.New(rand.NewPCG(1, 2))
	shows := Lineup(d, p, noon, rng)
	if shows[0].Episode.Title != "a1" {
		t.Fatalf("first episode %q", shows[0].Episode.Title)
	}
	MarkSeen(p, shows[0])
	MarkSeen(p, shows[0])
	MarkSeen(p, shows[0])
	if got := Lineup(d, p, noon, rng)[0].Episode.Title; got != "a2" {
		t.Errorf("after three, the episode %q (two wrap round)", got)
	}
	duo := &Show{Hosts: []*data.Channel{d.Channels[0], d.Channels[1]}, Key: DuoKey("a", "b")}
	MarkSeen(p, duo)
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
	for _, tc := range []struct {
		points float64
		step   int
	}{{0, 0}, {29.9, 0}, {30, 1}, {149, 1}, {150, 2}, {600, 3}, {2999, 3}, {3000, 4}, {1e9, MaxStep}} {
		if got := Step(tc.points); got != tc.step {
			t.Errorf("Step(%g) = %d, want %d", tc.points, got, tc.step)
		}
	}
	if PicksAnswer(0, 0.6) || !PicksAnswer(0, 0.59) || !PicksAnswer(MaxStep, 0.999) {
		t.Error("answer chances")
	}
}

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
