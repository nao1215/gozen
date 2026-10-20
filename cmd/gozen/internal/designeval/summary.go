package designeval

import (
	"slices"
	"sort"
)

type Gap struct {
	N    int     `json:"n"`
	Avg  float64 `json:"avg"`
	Max  float64 `json:"max"`
	At   string  `json:"at,omitempty"`
	From float64 `json:"from"`
	Day  int     `json:"day,omitempty"`
	Top  []Span  `json:"top,omitempty"`
}

type Span struct {
	From float64 `json:"from"`
	Len  float64 `json:"len"`
	At   string  `json:"at"`
	Day  int     `json:"day"`
}

type Summary struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Cleared     bool               `json:"cleared"`
	ClearMin    float64            `json:"clear_min"`
	ReleaseMin  float64            `json:"release_min"`
	EndMin      float64            `json:"end_min"`
	Days        int                `json:"days"`
	Deepest     int                `json:"deepest"`
	Level       int                `json:"level"`
	Battles     int                `json:"battles"`
	Lost        int                `json:"lost"`
	Retries     int                `json:"retries"`
	Wipes       int                `json:"wipes"`
	Stranded    int                `json:"stranded"`
	Rewards     map[string]int     `json:"rewards"`
	Gap         Gap                `json:"gap"`
	KindGap     map[string]Gap     `json:"kind_gap"`
	Dry         Gap                `json:"dry"`
	Expects     int                `json:"expects"`
	Release     Gap                `json:"release"`
	Unreleased  int                `json:"unreleased"`
	ReleaseWho  map[string]int     `json:"release_who"`
	ReleaseFlow map[string]int     `json:"release_flow"`
	Bosses      []*Boss            `json:"bosses"`
	Duels       []*DuelRec         `json:"duels"`
	Problems    int                `json:"problems"`
	Minutes     map[string]float64 `json:"minutes"`
}

func Summarize(r *Run, release string) Summary {
	ev := append([]Event(nil), r.Events...)
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].T < ev[j].T })
	end := r.End
	if r.Cleared {
		end = r.ClearT
	}
	s := Summary{
		ID: r.Persona.ID, Name: r.Persona.Name, Cleared: r.Cleared, ClearMin: r.ClearT / 60, EndMin: r.End / 60,
		ReleaseMin: r.ReleaseSec / 60, Days: r.Days, Deepest: r.Deepest, Level: r.Level, Battles: r.Battles, Lost: r.Lost, Retries: r.Retries,
		Wipes: r.Wipes, Stranded: r.Stranded, Rewards: map[string]int{}, KindGap: map[string]Gap{},
		ReleaseWho: map[string]int{}, ReleaseFlow: map[string]int{}, Bosses: r.Bosses, Duels: r.Duels, Problems: len(r.Problems), Minutes: map[string]float64{},
	}
	var all, either []float64
	kinds := map[string][]float64{}
	for _, e := range ev {
		if e.T > end {
			break
		}
		if e.Reward {
			s.Rewards[e.Kind]++
			all = append(all, e.T)
			kinds[e.Kind] = append(kinds[e.Kind], e.T)
			if e.Kind == release {
				if e.Who != "" {
					s.ReleaseWho[e.Who]++
				}
				if e.Flow != "" {
					s.ReleaseFlow[e.Flow]++
				}
			}
		}
		if e.Expect {
			s.Expects++
		}
		if e.Reward || e.Expect {
			either = append(either, e.T)
		}
	}
	s.Gap = gaps(all, end, r)
	for k, ts := range kinds {
		s.KindGap[k] = gaps(ts, end, r)
	}
	s.Dry = gaps(either, end, r)

	pending := -1.0
	var waits []float64
	longest, from := 0.0, 0.0
	for _, e := range ev {
		if e.T > end {
			break
		}
		if e.Expect && pending < 0 {
			pending = e.T
		}
		if e.Reward && e.Kind == release {
			if pending >= 0 {
				w := e.T - pending
				waits = append(waits, w)
				if w > longest {
					longest, from = w, pending
				}
			}
			pending = -1
			s.Unreleased = 0
		}
		if e.Expect {
			s.Unreleased++
		}
	}
	if len(waits) > 0 {
		sum := 0.0
		for _, w := range waits {
			sum += w
		}
		s.Release = Gap{N: len(waits), Avg: sum / float64(len(waits)) / 60, Max: longest / 60, From: from / 60}
		s.Release.At, s.Release.Day = placeIn(r, from, from+longest)
	}
	for _, g := range r.Segments {
		if g.From >= end {
			break
		}
		k := "town"
		if g.Dungeon {
			k = "dungeon"
		}
		s.Minutes[k] += (min(g.To, end) - g.From) / 60
	}
	return s
}

func gaps(ts []float64, end float64, r *Run) Gap {
	sort.Float64s(ts)
	var pts []float64
	pts = append(pts, 0)
	for _, t := range ts {
		if t-pts[len(pts)-1] >= 1 {
			pts = append(pts, t)
		}
	}
	if end > pts[len(pts)-1] {
		pts = append(pts, end)
	}
	g := Gap{}
	if len(pts) < 2 {
		return g
	}
	sum, a, bb := 0.0, 0.0, 0.0
	var spans []Span
	for i := 1; i < len(pts); i++ {
		d := pts[i] - pts[i-1]
		sum += d
		spans = append(spans, Span{From: pts[i-1], Len: d})
		if d > g.Max {
			g.Max, a, bb = d, pts[i-1], pts[i]
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Len > spans[j].Len })
	for _, sp := range spans[:min(3, len(spans))] {
		at, day := placeIn(r, sp.From, sp.From+sp.Len)
		g.Top = append(g.Top, Span{From: sp.From / 60, Len: sp.Len / 60, At: at, Day: day})
	}
	g.N = len(pts) - 1
	g.Avg = sum / float64(g.N) / 60
	g.Max /= 60
	g.From = a / 60
	g.At, g.Day = placeIn(r, a, bb)
	return g
}

func placeIn(r *Run, a, b float64) (string, int) {
	spent := map[string]float64{}
	var order []string
	for _, g := range r.Segments {
		if g.To <= a || g.From >= b {
			continue
		}
		d := min(g.To, b) - max(g.From, a)
		if _, ok := spent[g.Place]; !ok {
			order = append(order, g.Place)
		}
		spent[g.Place] += d
	}
	best, bd := "", -1.0
	for _, p := range order {
		if spent[p] > bd {
			best, bd = p, spent[p]
		}
	}
	day := 0
	for _, e := range r.Events {
		if e.T <= a {
			day = e.Day
		}
	}
	return best, day
}

type DuelAvg struct {
	Name                string
	First, Again, Later float64
	NFirst, NLater      int
	Off                 []string
}

func DuelAverages(c *Config, sums []Summary) []DuelAvg {
	out := make([]DuelAvg, len(c.Duels.List))
	for i, d := range c.Duels.List {
		a := &out[i]
		a.Name = d.Troop
		for _, s := range sums {
			if i >= len(s.Duels) || len(c.Duels.Personas) > 0 && !slices.Contains(c.Duels.Personas, s.ID) {
				continue
			}
			r := s.Duels[i]
			a.Name = r.Name
			if r.Opened {
				a.First += r.First
				a.Again += r.Again
				a.NFirst++
			}
			if r.Grown {
				a.Later += r.Later
				a.NLater++
			}
		}
		if a.NFirst > 0 {
			a.First /= float64(a.NFirst)
			a.Again /= float64(a.NFirst)
			if len(c.Duels.First) == 2 && (a.First < c.Duels.First[0] || a.First > c.Duels.First[1]) {
				a.Off = append(a.Off, "first")
			}
			if a.Again < c.Duels.Later {
				a.Off = append(a.Off, "again")
			}
		}
		if a.NLater > 0 {
			a.Later /= float64(a.NLater)
			if a.Later < c.Duels.Later {
				a.Off = append(a.Off, "later")
			}
		}
	}
	return out
}
