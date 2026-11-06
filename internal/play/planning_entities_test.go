package play

import (
	"encoding/json"
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestEntityTableInlineDetailAndNavigation(t *testing.T) {
	db := testgame.Load(t, nil)
	p := &data.Planning{Entities: map[string]data.Entity{}}
	order := []string{}
	for i := range 12 {
		id := strconv.Itoa(i)
		order = append(order, id)
		p.Entities[id] = data.Entity{Name: id}
	}
	db.Game.Planning = p
	db.Game.Screens = map[string]*data.AuthorScreen{"roster": {Kind: "entities", View: data.PlanningView{Fullscreen: true, Order: order, Detail: "record"}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "roster", nil, nil)
	ids, cells, detail := s.entityTableLayout(g)
	if len(ids) < 8 || !detail.Empty() {
		t.Fatalf("initial layout: %v %v", ids, detail)
	}
	_, sh := g.screen()
	for _, r := range cells {
		if r.Min.X != 16 || r.Dx() < 1000 || r.Max.Y > int(sh)-112 {
			t.Fatalf("table does not fit: %v", r)
		}
	}
	// Open the last visible row: its detail must be fully revealed, without
	// consuming a phase or obscuring any other row.
	g.in.click = true
	g.in.mouse = cells[len(cells)-1].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	if s.target != ids[len(ids)-1] {
		t.Fatal("wrong row opened")
	}
	_, expanded, detail := s.entityTableLayout(g)
	if detail.Dy() != 180 {
		t.Fatalf("detail clipped: %v", detail)
	}
	for _, r := range expanded {
		if r.Overlaps(detail) {
			t.Fatal("detail covers a row")
		}
	}
	g.in.mouse = expanded[0].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	if s.target != "" {
		t.Fatal("second click did not close detail")
	}
	g.in.click = false
	g.in.wheel = -1
	for range 20 {
		s.updateEntityTable(g)
	}
	ids, _, _ = s.entityTableLayout(g)
	if !slices.Contains(ids, "11") {
		t.Fatal("cannot reach final resident")
	}
	s.refresh(g)
	if s.residentTop == 0 {
		t.Fatal("refresh lost scroll position")
	}
	g.in.wheel = 1
	for range 20 {
		s.updateEntityTable(g)
	}
	if s.residentTop != 0 {
		t.Fatal("cannot return to first resident")
	}
	g.in.wheel = 0
	g.in.pressed[btnDown] = true
	for range order {
		s.updateEntityTable(g)
		visible, _, _ := s.entityTableLayout(g)
		if !slices.Contains(visible, order[s.focus]) {
			t.Fatal("keyboard focus moved offscreen")
		}
	}
	g.in.pressed[btnDown] = false
	g.in.pressed[btnOK] = true
	s.updateEntityTable(g)
	if s.target != order[s.focus] {
		t.Fatal("keyboard opened wrong resident")
	}
}

func TestEntityTableSortingAndCompleteRecords(t *testing.T) {
	db := testgame.Load(t, nil)
	if err := LoadFonts(db, gameFiles(t)); err != nil {
		t.Fatal(err)
	}
	order := []string{"potter", "weaver", "smith", "helper", "visitor"}
	p := &data.Planning{Entities: map[string]data.Entity{}, Activities: map[string]data.Activity{}, Attributes: map[string]data.Attribute{"secret": {Hidden: true}}}
	for _, id := range order {
		p.Entities[id] = data.Entity{Name: id}
	}
	for i := range 13 {
		id := fmt.Sprintf("task%02d", i)
		p.Activities[id] = data.Activity{Name: id, Assignee: "potter", Effort: 10}
	}
	db.Game.Planning = p
	db.Game.Screens = map[string]*data.AuthorScreen{"roster": {Kind: "entities", View: data.PlanningView{Fullscreen: true, Order: order, Visible: "visible", Detail: "record", Fields: []data.PlanningField{{Key: "hours", Label: "Hours"}, {Key: "secret", Label: "Unavailable"}}}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	for i, id := range order {
		e := g.st.Planning.Entities[id]
		e.Observations["visible"] = id != "visitor"
		e.Attributes["secret"] = 100 - i
	}
	for id, n := range map[string]float64{"potter": 2, "weaver": 12, "smith": 12, "visitor": 100} {
		g.st.Planning.Entities[id].Observations["hours"] = n
	}
	record := []string{}
	for i := range 17 {
		record = append(record, fmt.Sprintf("Recorded line %02d", i))
	}
	g.st.Planning.Entities["potter"].Observations["record"] = strings.Join(record, "\n")
	before, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	s := newAuthorScreen(g, "roster", nil, nil)
	header := s.entityTableHeaders(g)[1]
	g.in = input{click: true, mouse: header.Min.Add(image.Pt(8, 8))}
	for _, want := range [][]string{{"weaver", "smith", "potter", "helper"}, {"potter", "weaver", "smith", "helper"}, order[:4]} {
		s.updateEntityTable(g)
		if got := s.tableResidents(g); !slices.Equal(got, want) {
			t.Fatalf("sort got %v want %v", got, want)
		}
		s.refresh(g)
		ids, _, _ := s.entityTableLayout(g)
		if !slices.Equal(ids, want) {
			t.Fatal("layout ignores ordering", ids)
		}
	}
	// A column cannot sort by a hidden attribute even if it has the same key.
	g.in.mouse = s.entityTableHeaders(g)[2].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	if got := s.tableResidents(g); !slices.Equal(got, order[:4]) {
		t.Fatal("hidden data influenced order", got)
	}
	s.residentSort = 0
	_, cells, _ := s.entityTableLayout(g)
	g.in.mouse = cells[0].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	_, _, detail := s.entityTableLayout(g)
	var notes, tasks []string
	_, _, pages := s.entityDetailPage(g, float64(detail.Dx()), float64(detail.Dy()))
	g.in.mouse = entityDetailButtons(detail)[1].Min.Add(image.Pt(8, 8))
	for range pages {
		a, b, _ := s.entityDetailPage(g, float64(detail.Dx()), float64(detail.Dy()))
		tasks = append(tasks, a...)
		notes = append(notes, b...)
		s.updateEntityTable(g)
	}
	if !slices.Equal(notes, record) || len(tasks) != 13 || !strings.Contains(tasks[12], "task12") {
		t.Fatalf("content truncated: %v %v", tasks, notes)
	}
	if s.residentPage != pages-1 || s.residentTop != 0 || s.target != "potter" {
		t.Fatal("paging moved selection or overflowed")
	}
	s.refresh(g)
	if s.residentPage != pages-1 {
		t.Fatal("refresh lost record page")
	}
	g.in = input{wheel: 1, mouse: detail.Min.Add(image.Pt(8, 8))}
	for range 30 {
		s.updateEntityTable(g)
	}
	if s.residentPage != 0 || s.residentTop != 0 {
		t.Fatal("detail wheel scrolled table or underflowed")
	}
	_, rows, _ := s.entityTableLayout(g)
	g.in = input{click: true, mouse: rows[1].Min.Add(image.Pt(8, 8))}
	s.updateEntityTable(g)
	if s.target != "weaver" || s.residentPage != 0 {
		t.Fatal("new resident kept old page")
	}
	after, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("browsing changed saved state")
	}
}
