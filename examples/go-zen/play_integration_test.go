//go:build !js

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// missingArt matches the check's report of a picture that is not drawn yet.
var missingArt = regexp.MustCompile(`no (?:file|picture|face) assets/(\S+\.(?:png|jpg))`)

// standIns is the game with a plain picture for each one not drawn yet, so the rules can
// be played before the art is done (the published game itself has no stand-ins), and the
// pictures it stood in for.
func standIns(t *testing.T) (fs.FS, []string) {
	t.Helper()
	sub := os.DirFS("game")
	m := fstest.MapFS{}
	if err := fs.WalkDir(sub, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, name)
		m[name] = &fstest.MapFile{Data: b}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err := gozen.Load(m, ".")
	if err == nil {
		return m, nil
	}
	var missing []string
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if sm := missingArt.FindStringSubmatch(line); sm != nil {
			if _, ok := m["assets/"+sm[1]]; !ok {
				missing = append(missing, sm[1])
				m["assets/"+sm[1]] = &fstest.MapFile{Data: plainPicture(t, sm[1])}
			}
			continue
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "check:") {
			t.Fatalf("the game has a mistake other than a picture not drawn yet: %s", line)
		}
	}
	if _, err := gozen.Load(m, "."); err != nil {
		t.Fatal(err)
	}
	return m, missing
}

func plainPicture(t *testing.T, name string) []byte {
	t.Helper()
	w, h := 256, 256
	if strings.HasPrefix(name, "bg/") || strings.HasPrefix(name, "cg/") {
		w, h = 1920, 1080
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 90, 90, 110, 255
	}
	var b bytes.Buffer
	var err error
	if strings.HasSuffix(name, ".jpg") {
		err = jpeg.Encode(&b, img, nil)
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestTheGameLoads loads Go Zen as the window does; until all its art is drawn, it lists
// the pictures still missing and checks that nothing else is wrong.
func TestTheGameLoads(t *testing.T) {
	_, missing := standIns(t)
	if len(missing) > 0 {
		t.Skipf("%d pictures are not drawn yet: %s", len(missing), strings.Join(missing, ", "))
	}
}

func TestHighRankDragonAppearsOnlyOnDeepFloor(t *testing.T) {
	fsys, _ := standIns(t)
	db, err := data.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, pick := range db.Locations["b3"].Encounters.Troops {
		if pick.Troop == "crystal_dragon_group" {
			if got := pick.When.String(); got != "var.floor >= 5" {
				t.Fatalf("dragon floor gate = %q", got)
			}
			return
		}
	}
	t.Fatal("dragon encounter missing")
}

func TestBreedSimulationCaptureRootsAppearOnPlayableFloors(t *testing.T) {
	fsys, _ := standIns(t)
	db, err := data.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	floorGate := regexp.MustCompile(`^var\.floor >= (\d+)$`)
	for species, path := range db.BreedReach() {
		if path.Steps != 0 {
			continue
		}
		found := false
		for _, location := range []string{"b1", "b2", "b3"} {
			for _, pick := range db.Locations[location].Encounters.Troops {
				if troop := db.Troops[pick.Troop]; troop == nil || !slices.Contains(troop.Members, species) {
					continue
				}
				if location != "b3" || !pick.When.Set() {
					found = true
					continue
				}
				parts := floorGate.FindStringSubmatch(pick.When.String())
				if len(parts) != 2 {
					t.Fatalf("%s has an unreviewed floor gate %q", species, pick.When.String())
				}
				floor, err := strconv.Atoi(parts[1])
				if err != nil {
					t.Fatal(err)
				}
				if floor <= 5 { // The B3 tunnel permits floors one through five.
					found = true
				}
			}
		}
		if !found {
			t.Errorf("sim breed calls %s capturable, but it has no encounter on a playable floor", species)
		}
	}
}

func TestBreedingStagingUsesGoZenRanks(t *testing.T) {
	fsys, _ := standIns(t)
	db, err := data.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		species string
		tier    int
	}{
		{"steam_slime", 0}, {"succubus", 1}, {"crystal_dragon", 2},
	} {
		if got := db.Breeding.Staging.Tier(db.Enemies[tc.species].Rank); got != tc.tier {
			t.Errorf("%s birth tier = %d, want %d", tc.species, got, tc.tier)
		}
	}
}

func TestNamedBatSpeciesShareFamilyAndGrowth(t *testing.T) {
	fsys, _ := standIns(t)
	db, err := data.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"frost_bat", "gold_bat", "night_bat"} {
		e := db.Enemies[id]
		if e.Family != "bird" || db.MonsterClass(id) != "monster_bird" {
			t.Errorf("%s is a bat but has family %q and growth class %q", id, e.Family, db.MonsterClass(id))
		}
	}
}

// route is a play from the start to the clear: the town on its map, the three floors of
// the Undercistern, the finale.
var route = []string{
	"pick: Pellmere", "pick: Fenn & Daughters", "sell: {shop: fenn, item: old_gear, count: 3}",
	"buy: {shop: fenn, item: tonic, count: 2}", "pick: Pellmere", "pick: The Last Well", "pick: Tilly",
	"pick: Pellmere", "pick: Pellmere Plaza", "pick: Ox", "pick: Pellmere", "pick: The Dry Cup",
	"pick: fight smarter", "pick: Rest", "pick: Pellmere", "pick: Undercistern Gate", "pick: Hobb's key",
	"walk: L", "walk: Q", "walk: T", "walk: U", "walk: '>'", "pick: Rest", "pick: Down to the Cistern Hall",
	"walk: k", "walk: G", "walk: k", "walk: M", "walk: k", "walk: N", "walk: W", "walk: V", "walk: '>'",
	"walk: Y", "walk: P", "walk: H", "walk: t", "walk: o", "walk: k", "walk: E",
}

// player plays as a player would: AUTO battles with the party's tactics, back to rest
// (the inn from B1, Hobb's Landing from B2, tonics on B3) when someone is hurt, and the
// Brass Warden tried again after resting when it wins.
type player struct {
	t *testing.T
	s *gozen.Session
}

func (p *player) q(src string) string {
	var v any
	if err := p.s.Query(src, &v); err != nil {
		return ""
	}
	return fmt.Sprint(v)
}

func (p *player) f(src string) float64 {
	var v float64
	if err := p.s.Query(src, &v); err != nil {
		p.t.Fatalf("%s: %v", src, err)
	}
	return v
}

func (p *player) do(script string) error {
	err := p.s.Do(context.Background(), "- "+script+"\n")
	if p.q("over") == "true" {
		p.t.Fatalf("the party fell at %s after %q", p.q("location"), script)
	}
	return err
}

// tryTo does what a player tries: a walk a battle cut short is taken up by the next step.
func (p *player) tryTo(script string) {
	if err := p.do(script); err != nil {
		p.t.Logf("%s: %v", script, err)
	}
}

func (p *player) rest() {
	switch p.q("location") {
	case "b1":
		p.tryTo("walk: '<'")
		for _, s := range []string{"pick: Pellmere", "pick: The Dry Cup", "pick: Rest", "pick: Pellmere", "pick: Undercistern Gate", "pick: Down into the dark"} {
			if err := p.do(s); err != nil {
				p.t.Fatal(err)
			}
		}
	case "b2":
		p.tryTo("walk: '<'")
		for _, s := range []string{"pick: Rest", "pick: Down to the Cistern Hall"} {
			if err := p.do(s); err != nil {
				p.t.Fatal(err)
			}
		}
	case "b3":
		for _, id := range []string{"maren", "ox", "tilly", "kes"} {
			if p.q("actor."+id+".in_party") == "true" && p.f("item.tonic") > 0 && p.f("actor."+id+".hp_rate") < 0.6 {
				p.tryTo("use: tonic\n  target: " + id)
			}
		}
	}
}

func (p *player) step(script string) {
	p.t.Helper()
	dungeon := map[string]bool{"b1": true, "b2": true, "b3": true}
	if dungeon[p.q("location")] && p.f("party.min_hp_rate") < 0.7 {
		p.rest()
	}
	for range 6 {
		before := p.q("location")
		err := p.do(script)
		if err != nil && !strings.Contains(err.Error(), "no way to") {
			p.t.Fatalf("%q at %s: %v", script, p.q("location"), err)
		}
		if !dungeon[p.q("location")] || p.q("location") != before || p.f("party.min_hp_rate") >= 0.45 {
			return
		}
		p.rest()
		if err == nil {
			return
		}
	}
}

// TestPlayToTheEnd plays Go Zen to its end with several seeds, as a player who rests
// when hurt: no seed loses the party (the balance of check/balance.md in the sample's
// workspace: the Warden may win and is tried again; the rest never wipe the party).
func TestPlayToTheEnd(t *testing.T) {
	fsys, _ := standIns(t)
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	seeds := uint64(12)
	if testing.Short() {
		seeds = 2
	}
	for seed := uint64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			s, err := gozen.New(g, gozen.Config{Seed: seed})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Begin(context.Background()); err != nil {
				t.Fatal(err)
			}
			p := &player{t: t, s: s}
			for _, sc := range route {
				p.step(sc)
				for try := 0; sc == "walk: W" && p.q("flag.warden_down") != "true"; try++ {
					if try == 8 {
						t.Fatal("the Warden was never beaten")
					}
					p.rest()
					p.step(sc)
				}
			}
			if p.q("cleared") != "true" {
				t.Fatalf("not cleared: at %s", p.q("location"))
			}
		})
	}
}
