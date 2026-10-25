package play

import (
	"bytes"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
	// Real pointer input selects a marker, and clicking it again still only previews.
	x, y := s.screen(g, g.db.Locations[s.loc].Map.Points["quarry"].At.Pt())
	for range 2 {
		g.in = input{mouse: image.Pt(int(x), int(y-10)), click: true, moved: true}
		s.update(g)
		if s.choices[s.sel].Point != "quarry" || s.walking() {
			t.Fatal("point click activated or selected the wrong action")
		}
	}
	g.in = input{}
	g.in.pressed[btnCancel] = true
	s.update(g)
	if !s.panelHidden {
		t.Fatal("cancel did not hide panel")
	}
	g.in = input{}
	g.in.pressed[btnOK] = true
	s.update(g)
	if s.panelHidden || s.walking() {
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

	// Only the explicit panel confirmation runs the action.
	b := s.panelButtons(g)[0]
	g.in = input{click: true, mouse: image.Pt(int(b.x+10), int(b.y+10))}
	s.update(g)
	g.in = input{}
	wldSettle(t, g, s, 180)
	if g.st.Vars["quarry"] != 1.0 || g.st.Spot != "town" || g.st.Steps != 0 {
		t.Fatalf("confirm: vars=%v spot=%s steps=%d", g.st.Vars, g.st.Spot, g.st.Steps)
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
