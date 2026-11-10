package play

import (
	"bytes"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func atlasPanelFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	return btlFiles(t, "schema_version: 3\n", testgame.AtlasMenuData, map[string]string{"assets/maps/menu.png": string(testgame.Picture(64, 36)), "assets/x.png": string(btlPNG(100, 100))})
}

func TestAtlasSelectionPanel(t *testing.T) {
	g, err := New(Options{FS: atlasPanelFiles(t), Location: "selection", NoSound: true, SaveDir: t.TempDir(), Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	s, ok := g.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	wldFrames(t, g, 10)
	g.st.Spot = "town"
	before, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	rngBefore, err := g.pcg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	box := s.selectionPanelBox(g)
	for _, point := range []string{"town", "quarry", "tower"} {
		s.focusPoint(point)
		heading, body, portrait := s.selectionAdvice(g)
		if !strings.Contains(strings.ToLower(heading), point) || body == "" || portrait != "x.png" {
			t.Fatalf("advice %s: %q %q %q", point, heading, body, portrait)
		}
		s.zoomCamera(g, 2)
		s.panCamera(g, 20, 10)
		if s.selectionPanelBox(g) != box {
			t.Fatal("panel follows camera")
		}
	}
	s.resetCamera(g, true)
	// A tap on a marker selects it and shows its advice; nothing runs yet.
	x, y := s.screen(g, g.db.Locations[s.loc].Map.Points["quarry"].At.Pt())
	tap := input{mouse: image.Pt(int(x), int(y-10)), click: true, pdown: true, pup: true, moved: true}
	s.focusPoint("tower")
	g.in = tap
	s.update(g)
	if s.choices[s.sel].Point != "quarry" || len(s.queue) > 0 || g.st.Vars["quarry"] != nil {
		t.Fatal("the first tap on a point activated it or selected the wrong one")
	}
	// A finger that went down on another point's advice and lifts here selects too.
	s.focusPoint("tower")
	g.in = input{mouse: tap.mouse, pdown: true, pheld: true, moved: true}
	s.update(g)
	g.in = input{mouse: tap.mouse, click: true, pup: true, moved: true}
	s.update(g)
	if s.choices[s.sel].Point != "quarry" || len(s.queue) > 0 {
		t.Fatal("a touch on another point than the focused one activated it")
	}
	g.in = input{}
	g.in.pressed[btnCancel] = true
	s.update(g)
	if !s.panel.hidden {
		t.Fatal("cancel did not hide panel")
	}
	g.in = input{}
	g.in.pressed[btnOK] = true
	s.update(g)
	if s.panel.hidden || s.walking() {
		t.Fatal("confirm after cancel should first reopen advice")
	}
	// Random templates and conditions are evaluated against an isolated RNG.
	p := g.db.Locations[s.loc].Map.Points["quarry"]
	p.Advice = append([]data.MapAdvice{{When: data.MustExpr("rand(1, 10) > 0"), Text: "Preview {rand(1, 10)}"}}, p.Advice...)
	for range 5 {
		s.refresh(g)
		s.selectionAdvice(g)
	}
	p.Advice = p.Advice[1:]
	after, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	rngAfter, err := g.pcg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !bytes.Equal(rngBefore, rngAfter) {
		t.Fatal("browsing mutated saved state or RNG")
	}
	s.focusPoint("quarry")
	g.st.Items["potion"] = 0
	_, body, _ := s.selectionAdvice(g)
	if !strings.Contains(body, "You have 0") {
		t.Fatal(body)
	}
	g.st.Items["potion"] = 3
	_, body, _ = s.selectionAdvice(g)
	if !strings.Contains(body, "supplies are ready") || s.selectionPanelBox(g) != box {
		t.Fatal("state-dependent advice or fixed geometry changed")
	}
	// Translation precedes interpolation and leaves the authored source intact.
	g.db.Words["ja"] = map[string]string{"Quarry guide": "採石場の案内", "Bring three potions. You have {item.potion}.": "薬は {item.potion} 個です。"}
	g.st.Items["potion"] = 2
	g.db.Show("ja")
	heading, translated, _ := s.selectionAdvice(g)
	if heading != "採石場の案内" || translated != "薬は 2 個です。" {
		t.Fatalf("untranslated advice: %q %q", heading, translated)
	}
	g.db.Show("en")

	// The panel's confirmation runs the action.
	b := s.panelButtons(g)[0]
	g.in = input{click: true, mouse: image.Pt(int(b.x+10), int(b.y+10))}
	s.update(g)
	g.in = input{}
	wldSettle(t, g, s, 180)
	if g.st.Vars["quarry"] != 1.0 || g.st.Spot != "town" || g.st.Steps != 0 {
		t.Fatalf("confirm: vars=%v spot=%s steps=%d", g.st.Vars, g.st.Spot, g.st.Steps)
	}
	// So does a second tap on the point the panel shows: no button is needed.
	s.focusPoint("quarry")
	g.in = tap
	s.update(g)
	g.in = input{}
	wldSettle(t, g, s, 180)
	if g.st.Vars["quarry"] != 2.0 || g.st.Spot != "town" {
		t.Fatalf("a second tap on the focused point did not run it: vars=%v spot=%s", g.st.Vars, g.st.Spot)
	}
	s.focusPoint("town")
	for range 2 {
		g.in = input{}
		g.in.pressed[btnOK] = true
		s.update(g)
		g.in = input{}
		wldSettle(t, g, s, 180)
	}
	if g.st.Vars["town"] != 2.0 {
		t.Fatal("current-point action cannot be repeated")
	}
	// Saving the menu retains the party's position, not the last previewed point.
	s.focusPoint("tower")
	saved, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(saved, []byte(`"spot":"town"`)) && !bytes.Contains(saved, []byte(`"spot": "town"`)) {
		t.Fatal("save lost actual position")
	}
}
