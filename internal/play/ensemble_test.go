package play

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

func TestEnsembleGridAspectAndBounds(t *testing.T) {
	for _, count := range []int{0, 1, 2, 8, 9, 16} {
		for _, area := range []image.Rectangle{image.Rect(36, 76, 856, 526), image.Rect(36, 76, 856, 620)} {
			cells := ensembleCells(float64(area.Min.X), float64(area.Min.Y), float64(area.Dx()), float64(area.Dy()), count)
			if len(cells) != count {
				t.Fatal(count, cells)
			}
			for i, r := range cells {
				if r.Empty() || !r.In(area) || math.Abs(float64(r.Dx())/float64(r.Dy())-4.0/3) > .02 {
					t.Fatalf("invalid tile %v", r)
				}
				for _, other := range cells[:i] {
					if r.Overlaps(other) {
						t.Fatal("overlap", r, other)
					}
				}
			}
		}
	}
}

func TestEnsemblePresenceTransitions(t *testing.T) {
	labels := &data.EnsembleChat{Joined: "joined", Left: "left", CameraOff: "camera off", CameraOn: "camera on"}
	log := &ensembleLog{reactions: map[string]string{}}
	members := []ensembleMember{{id: "potter", name: "Potter", camera: true}, {id: "weaver", name: "Weaver"}}
	log.update(members, labels)
	if len(log.messages) != 3 || log.messages[2].text != "camera off" {
		t.Fatal(log.messages)
	}
	log.update(members, labels)
	if len(log.messages) != 3 {
		t.Fatal("refresh duplicated notices")
	}
	members[0].camera = false
	log.update(members[:1], labels)
	if len(log.messages) != 5 || log.messages[3].text != "left" || log.messages[4].text != "camera off" {
		t.Fatal(log.messages)
	}
	members[0].camera = true
	log.update(members[:1], labels)
	if log.messages[5].text != "camera on" {
		t.Fatal(log.messages)
	}
	for range 110 {
		log.post("Potter", "Ready", false)
	}
	if len(log.messages) != 100 {
		t.Fatal("unbounded transcript")
	}
}

func TestEnsembleTranscriptSessionAndRender(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
	g.db.Game.Planning = &data.Planning{Calendar: data.Calendar{Phases: []string{"morning"}}, Entities: map[string]data.Entity{}}
	// Use the existing planning initialization through an empty fixture.
	g.st.Planning = &rpg.PlanningState{Date: "2030-04-01"}
	g.db.Game.Screens = map[string]*data.AuthorScreen{"roundtable": {Kind: "ensemble", View: data.PlanningView{Chat: &data.EnsembleChat{Title: "Workshop messages"}}}}
	first := event.Say{Ensemble: "roundtable", Name: "Potter", Text: "The kiln is ready."}
	g.talk.open(g, first)
	g.talk.open(g, event.Say{Ensemble: "roundtable", Name: "Weaver", Text: "Thank you."})
	if len(g.talk.meeting.messages) != 0 || g.talk.say.Text != "Thank you." {
		t.Fatal("speech must remain in subtitles only", g.talk.meeting)
	}
	for _, size := range [][2]float64{{1920, 1080}, {2560, 1080}, {3840, 2160}} {
		g.fitView(size[0], size[1], 1)
		before := len(g.talk.meeting.messages)
		g.talk.draw(g, btlImage())
		if len(g.talk.meeting.messages) != before {
			t.Fatal("draw changed transcript")
		}
	}
	g.talk.open(g, event.Say{Text: "Outside"})
	g.talk.open(g, first)
	if len(g.talk.meeting.messages) != 0 {
		t.Fatal("previous meeting leaked")
	}
}

func TestEnsembleResponsesUsePublicPresenceAndStableOrder(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
	g.db.Game.Planning = &data.Planning{
		Attributes: map[string]data.Attribute{"secret": {Hidden: true}},
		Entities:   map[string]data.Entity{"potter": {Name: "Potter", Actor: "potter"}, "weaver": {Name: "Weaver", Actor: "weaver"}},
	}
	g.st.Planning = &rpg.PlanningState{Entities: map[string]*rpg.EntityState{
		"potter": {Attributes: map[string]any{"secret": "Do not expose"}, Observations: map[string]any{"present": true, "camera": true, "response": "Ready"}},
		"weaver": {Observations: map[string]any{"present": false, "camera": false, "response": "Thank you"}},
	}}
	v := data.PlanningView{Order: []string{"potter", "weaver"}, Visible: "present", Camera: "camera", Chat: &data.EnsembleChat{Reaction: "response", Joined: "joined"}}
	log := &ensembleLog{reactions: map[string]string{}}
	log.sync(g, v)
	log.react(g, "visitor")
	log.react(g, "visitor")
	if len(log.messages) != 2 || log.messages[1].text != "Ready" {
		t.Fatal(log.messages)
	}
	g.st.Planning.Entities["weaver"].Observations["present"] = true
	log.sync(g, v)
	log.react(g, "visitor")
	if len(log.messages) != 4 || log.messages[3].name != "Weaver" {
		t.Fatal(log.messages)
	}
	g.st.Planning.Entities["potter"].Observations["response"] = "Changed"
	log.sync(g, v)
	log.react(g, "potter")
	if len(log.messages) != 4 {
		t.Fatal("speaker reacted to own line")
	}
	log.react(g, "visitor")
	if log.messages[4].text != "Changed" {
		t.Fatal(log.messages)
	}
	v.Chat.Reaction = "secret"
	for _, member := range g.ensembleMembers(v) {
		if member.reaction != "" {
			t.Fatal("hidden attribute disclosed")
		}
	}
}

func TestCallMotionBoundedAndIndependent(t *testing.T) {
	for tick := range 1200 {
		x, y, a, _, mouth := callMotion("potter", tick, false)
		if math.Abs(x) > 1.4 || math.Abs(y) > 1.1 || math.Abs(a) > .004 || mouth {
			t.Fatal(tick, x, y, a, mouth)
		}
		sx, sy, sa, _, _ := callMotion("potter", tick, true)
		if math.Abs(sx) > 1.4 || math.Abs(sy) > 3.5 || math.Abs(sa) > .010001 {
			t.Fatal(tick, sx, sy, sa)
		}
	}
	x, y, _, _, _ := callMotion("potter", 30, false)
	xx, yy, _, _, _ := callMotion("weaver", 30, false)
	if x == xx && y == yy {
		t.Fatal("participants must have distinct phases")
	}
	xx, yy, _, _, _ = callMotion("potter", 60, false)
	if x == xx && y == yy {
		t.Fatal("camera must move after half a second")
	}
}

func TestEnsembleTranscriptKeepsNewestMessages(t *testing.T) {
	log := &ensembleLog{}
	for i := range 1000 {
		log.post("Weaver", strconv.Itoa(i), i%2 == 0)
	}
	log.post("Weaver", " \t\n", false)
	if len(log.messages) != 100 {
		t.Fatalf("messages = %d", len(log.messages))
	}
	for i, m := range log.messages {
		if m.text != strconv.Itoa(i+900) || m.name != "Weaver" || m.system != (i%2 == 0) {
			t.Fatalf("message %d = %+v", i, m)
		}
	}
}

func BenchmarkEnsembleTranscriptAppend(b *testing.B) {
	log := &ensembleLog{}
	for range 100 {
		log.post("Weaver", "The work is ready.", false)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		log.post("Weaver", "The work is ready.", false)
	}
}

// TestCallGeometryFitsTheScreen lays the call out on screens of several
// heights: every part stays on the screen and inside the browser frame, the
// parts do not overlap, and 1280x720 keeps the original positions.
func TestCallGeometryFitsTheScreen(t *testing.T) {
	l := callGeometry(1280, 720)
	if l.main != (rectF{36, 104, 876, 404}) || l.strip != (rectF{36, 520, 876, 86}) || l.chat != (rectF{928, 104, 316, 502}) || l.controlsY != 634 || l.frameBottom != 664 || l.caption != (rectF{48, 412, 840, 96}) {
		t.Fatalf("1280x720 moved: %+v", l)
	}
	for _, size := range [][2]float64{{640, 360}, {640, 960}, {1280, 720}, {1280, 800}, {1280, 960}, {1280, 1024}, {1280, 600}, {1600, 900}, {1920, 1080}, {2560, 1080}} {
		sw, sh := size[0], size[1]
		l := callGeometry(sw, sh)
		frame := image.Rect(18, 16, int(sw-18), int(l.frameBottom))
		parts := map[string]rectF{"main": l.main, "strip": l.strip, "chat": l.chat, "caption": l.caption, "self": l.self}
		for name, r := range parts {
			if r.w <= 0 || r.h <= 0 || !r.rect().In(frame) {
				t.Errorf("%vx%v %s %+v leaves the frame %v", sw, sh, name, r, frame)
			}
		}
		for _, pair := range [][2]string{{"main", "strip"}, {"main", "chat"}, {"strip", "chat"}} {
			if parts[pair[0]].rect().Overlaps(parts[pair[1]].rect()) {
				t.Errorf("%vx%v %s overlaps %s", sw, sh, pair[0], pair[1])
			}
		}
		if !l.caption.rect().In(l.main.rect()) || !l.self.rect().In(l.main.rect()) {
			t.Errorf("%vx%v caption or self view leaves the stage", sw, sh)
		}
		controls := image.Rect(int(l.controlsX-l.controlR), int(l.controlsY-l.controlR), int(l.controlsX+5*l.controlDX+l.controlR), int(l.controlsY+l.controlR))
		if !controls.In(frame) || controls.Overlaps(l.strip.rect()) {
			t.Errorf("%vx%v controls %v leave the frame or cover the strip", sw, sh, controls)
		}
	}
}

func callGame(t *testing.T, call string, extra ...map[string]string) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "", extra...)
	g.db.Game.Planning = &data.Planning{
		Calendar: data.Calendar{Start: "2030-04-01", End: "2030-04-09", Phases: []string{"morning"}},
		Entities: map[string]data.Entity{
			"potter": {Name: "Potter", Actor: "potter"},
			"weaver": {Name: "Weaver", Actor: "weaver"},
			"glazer": {Name: "Glazer", Actor: "glazer"},
		},
	}
	g.st.Planning = &rpg.PlanningState{Date: "2030-04-01", Entities: map[string]*rpg.EntityState{
		"potter": {Observations: map[string]any{"camera": true}},
		"weaver": {Observations: map[string]any{"camera": false}},
		"glazer": {Observations: map[string]any{"camera": false}},
	}}
	var c data.EnsembleCall
	if err := yaml.Unmarshal([]byte(call), &c); err != nil {
		t.Fatal(err)
	}
	g.db.Game.Screens = map[string]*data.AuthorScreen{"studio": {Title: "Studio", Kind: "ensemble", View: data.PlanningView{
		Order: []string{"potter", "weaver", "glazer"}, Camera: "camera", Chat: &data.EnsembleChat{Title: "Messages"}, Call: &c,
	}}}
	return g
}

// TestCallStageFollowsTheSpeaker enlarges whoever speaks, a camera-off speaker
// included, keeps the last speaker between lines, and falls back to the first
// camera when the call is quiet.
func TestCallStageFollowsTheSpeaker(t *testing.T) {
	g := callGame(t, "{address: call.example.test}")
	v := g.db.Game.Screens["studio"].View
	members := g.ensembleMembers(v)
	if got := g.callStage("studio", members); got != 0 {
		t.Fatalf("quiet call stage = %d, want the first camera", got)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "The loom is strung."})
	if got := g.callStage("studio", members); members[got].id != "weaver" {
		t.Fatalf("camera-off speaker not on the stage: %d", got)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "glazer", Name: "Glazer", Text: "Glaze is mixed."})
	if got := g.callStage("studio", members); members[got].id != "glazer" {
		t.Fatalf("stage did not follow the next speaker: %d", got)
	}
	g.talk.active = false
	if got := g.callStage("studio", members); members[got].id != "glazer" {
		t.Fatalf("stage left the last speaker between lines: %d", got)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Text: "The kiln ticks as it cools."})
	if got := g.callStage("studio", members); members[got].id != "glazer" {
		t.Fatalf("a narrated line of the call left the last speaker: %d", got)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "nobody_in_call", Name: "Me", Text: "Thank you."})
	if got := g.callStage("studio", members); members[got].id != "glazer" {
		t.Fatalf("the caller's own line moved the stage: %d", got)
	}
	g.talk.say = event.Say{}
	if got := g.callStage("studio", members); got != 0 {
		t.Fatalf("stage after the call's lines = %d", got)
	}
}

// TestCallControlsNameTheirIcons reads controls as bare labels (icon by place,
// the last one leaves) and as maps that name the icon and color.
func TestCallControlsNameTheirIcons(t *testing.T) {
	var c data.EnsembleCall
	if err := yaml.Unmarshal([]byte("controls: [Mic, Camera, Leave]"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Controls[0].ControlIcon(0) != "none" || c.Controls[1].ControlIcon(1) != "none" || c.Controls[2].ControlLeave(2, 3) || c.Controls[1].ControlLeave(1, 3) {
		t.Fatalf("bare labels: %+v", c.Controls)
	}
	if err := yaml.Unmarshal([]byte("controls: [{label: Out, icon: leave}, {label: Talk, icon: chat, color: '#112233'}]"), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Controls[0].ControlLeave(0, 2) || c.Controls[1].ControlLeave(1, 2) || c.Controls[1].ControlIcon(1) != "chat" || c.Controls[1].Color.Or(color.NRGBA{}) != (color.NRGBA{0x11, 0x22, 0x33, 0xff}) {
		t.Fatalf("named icons: %+v", c.Controls)
	}
}

// TestPalettesKeepTheBuiltInLook leaves the colors as they were without the
// new ui and view fields, and takes the authored ones when given.
func TestPalettesKeepTheBuiltInLook(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
	if p, _, _, today, _ := burnupColors(nil); p != (color.NRGBA{55, 112, 190, 255}) || today != (color.NRGBA{180, 98, 46, 255}) {
		t.Fatal("burnup default changed")
	}
	if c := g.chrome(); c.buttons != "−   □   ×" || c.bar != (color.NRGBA{224, 230, 239, 255}) || c.nav != "‹   ›" {
		t.Fatalf("chrome default changed: %+v", c)
	}
	if l := g.lightPalette(); l.paper != (color.NRGBA{236, 234, 228, 255}) || l.accent != (color.NRGBA{37, 105, 109, 255}) {
		t.Fatalf("light default changed: %+v", l)
	}
	if tn := g.tonePalette(); tn.note != (color.NRGBA{255, 241, 194, 255}) || tn.primaryCard != (color.NRGBA{26, 113, 122, 255}) {
		t.Fatalf("tones default changed: %+v", tn)
	}
	var ui data.UIStyle
	src := "chrome: {bar: '#101010', buttons: ''}\ntones: {note: '#202020'}\nlight: {paper: '#303030'}\n"
	if err := yaml.Unmarshal([]byte(src), &ui); err != nil {
		t.Fatal(err)
	}
	g.db.Game.UI.Chrome, g.db.Game.UI.Tones, g.db.Game.UI.Light = ui.Chrome, ui.Tones, ui.Light
	if c := g.chrome(); c.buttons != "" || c.bar != (color.NRGBA{16, 16, 16, 255}) || c.nav != "‹   ›" {
		t.Fatalf("authored chrome: %+v", c)
	}
	if g.tonePalette().note != (color.NRGBA{32, 32, 32, 255}) || g.lightPalette().paper != (color.NRGBA{48, 48, 48, 255}) || g.lightPalette().text != (color.NRGBA{40, 49, 58, 255}) {
		t.Fatal("authored tones or light")
	}
	var b data.BurnupView
	if err := yaml.Unmarshal([]byte("colors: {observed: '#00ff00'}"), &b); err != nil {
		t.Fatal(err)
	}
	if p, o, _, _, _ := burnupColors(b.Colors); o != (color.NRGBA{0, 255, 0, 255}) || p != (color.NRGBA{55, 112, 190, 255}) {
		t.Fatal("authored burnup colors")
	}
}

// TestNoMaterialKeepsTheV015Theme keeps the dim text and border rules of games
// without ui.material, and raises dim only for the material look.
func TestNoMaterialKeepsTheV015Theme(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
	g.loadTheme()
	if g.th.dim.A != 0x70 || g.th.disabled.A != 0x70 || g.hasMaterial() {
		t.Fatalf("plain theme changed: %+v", g.th)
	}
	g.db.Game.UI.Material = &data.MaterialStyle{Kind: "opaque"}
	g.loadTheme()
	if g.th.dim.A != 0xc8 || g.th.disabled.A != 0x70 {
		t.Fatalf("material theme: %+v", g.th)
	}
}

// call_view turns a participant's camera on for the call's lines only: the
// state is untouched and a line outside the call ends it.
func TestCallViewCameraIsPresentationOnly(t *testing.T) {
	g := callGame(t, "{address: call.example.test}")
	v := g.db.Game.Screens["studio"].View
	cameraOf := func(id string) bool {
		for _, m := range g.ensembleMembers(v) {
			if m.id == id {
				return m.camera
			}
		}
		t.Fatal("no member", id)
		return false
	}
	if cameraOf("weaver") {
		t.Fatal("weaver's camera is on before call_view")
	}
	g.Handle(event.CallCameraEffect{Who: "weaver", On: true})
	if !cameraOf("weaver") || g.st.Planning.Entities["weaver"].Observations["camera"] != false {
		t.Fatal("call_view did not turn the view on, or changed the state")
	}
	g.talk.open(g, event.Say{Text: "Back at the desk."})
	if cameraOf("weaver") {
		t.Fatal("a line outside the call kept the camera override")
	}
}

// TestCallSelfStageWhileTheCallerSpeaks: with call.self_stage the self view takes the
// stage while the caller (a speaker who is not a participant) or a participant self
// speaks, and leaves it to the participants' rule for their lines; without it the stage
// never shows the self view. Drawing either way leaves the simulation alone.
func TestCallSelfStageWhileTheCallerSpeaks(t *testing.T) {
	g := callGame(t, "{address: call.example.test, self: me.png, self_stage: true}")
	v := g.db.Game.Screens["studio"].View
	all := g.ensembleMembers(v)
	if g.callSelfOnStage("studio", v, all, nil) {
		t.Fatal("a quiet call put the self view on the stage")
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "me", Name: "Me", Text: "Let us begin."})
	if !g.callSelfOnStage("studio", v, all, nil) {
		t.Fatal("the caller's line left the self view off the stage")
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "The loom is strung."})
	if g.callSelfOnStage("studio", v, all, nil) {
		t.Fatal("a participant's line kept the self view on the stage")
	}
	if got := g.callStage("studio", all); all[got].id != "weaver" {
		t.Fatalf("stage after the caller = %d, want the participant who speaks", got)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "potter", Name: "Potter", Text: "Clay is ready."})
	if !g.callSelfOnStage("studio", v, all, &all[0]) {
		t.Fatal("a participant self who speaks stayed off the stage")
	}
	g.db.Game.Screens["studio"].View.Call.SelfStage = false
	v = g.db.Game.Screens["studio"].View
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "me", Name: "Me", Text: "Let us begin."})
	if g.callSelfOnStage("studio", v, all, nil) {
		t.Fatal("without self_stage the caller's line put the self view on the stage")
	}
}

// TestCallFullGeometry lays a full call (call.layout: full) out on several screens: the
// stage runs to the lower edge under the message window, its people are framed above
// the window's top, and the side column, the controls and the self view stay above it.
func TestCallFullGeometry(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {1280, 960}, {1600, 900}, {1920, 1080}, {2560, 1080}} {
		sw, sh := size[0], size[1]
		top := sh - 242*sh/720
		for _, side := range []bool{true, false} {
			l := callGeometryFull(sw, sh, top, side)
			screen := image.Rect(0, 0, int(sw), int(sh))
			if !l.full || l.main.y+l.main.h < sh-16 || l.seen <= 0 || l.main.y+l.seen > top {
				t.Errorf("%vx%v side=%v stage %+v seen %v, window at %v", sw, sh, side, l.main, l.seen, top)
			}
			for name, r := range map[string]rectF{"main": l.main, "chat": l.chat, "self": l.self} {
				if r.w <= 0 || r.h <= 0 || !r.rect().In(screen) {
					t.Errorf("%vx%v side=%v %s %+v leaves the screen", sw, sh, side, name, r)
				}
			}
			if l.main.rect().Overlaps(l.chat.rect()) || l.chat.y+l.chat.h > top || l.self.y+l.self.h > top || l.controlsY+l.controlR > top {
				t.Errorf("%vx%v side=%v the side column, the self view or the controls run under the window: %+v", sw, sh, side, l)
			}
			if side != (l.strip.h == 0) || !side && l.strip.y+l.strip.h > top {
				t.Errorf("%vx%v side=%v strip %+v", sw, sh, side, l.strip)
			}
		}
	}
}

// TestCallOthersGrid lists one person a row, and more people in more columns so that
// a row stays tall enough for a face and a name.
func TestCallOthersGrid(t *testing.T) {
	r := rectF{0, 0, 300, 330}
	for _, tc := range []struct{ n, cols int }{{1, 1}, {6, 1}, {7, 2}, {12, 2}, {13, 3}, {30, 3}} {
		cols, rowH := callOthersGrid(tc.n, r)
		rows := (tc.n + cols - 1) / cols
		if cols != tc.cols || rowH > 76 || float64(rows)*rowH > r.h+0.001 {
			t.Errorf("%d people: %d columns of rows %v", tc.n, cols, rowH)
		}
	}
	if _, rowH := callOthersGrid(0, r); rowH != 0 {
		t.Error("no one listed still took rows")
	}
}

// TestCallChoiceKeepsTheCall: a choice with no line of its own right after a line of a
// call keeps that call behind it; one long after keeps no call.
func TestCallChoiceKeepsTheCall(t *testing.T) {
	g := callGame(t, "{address: call.example.test}")
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "Which one?"})
	g.talk.active, g.talk.closed = false, g.tick
	g.talk.say.Text = ""
	g.talk.ask(g, event.Choice{Options: []string{"Red", "Blue"}})
	if !g.talk.bare || g.talk.say.Ensemble != "studio" {
		t.Fatalf("choice after the call's line: bare %v, call %q", g.talk.bare, g.talk.say.Ensemble)
	}
	g.talk.active, g.talk.closed = false, g.tick-10
	g.talk.ask(g, event.Choice{Options: []string{"Red", "Blue"}})
	if g.talk.say.Ensemble != "" {
		t.Fatalf("a choice long after the call kept it: %q", g.talk.say.Ensemble)
	}
}

// TestCallSideFoldsWhenEmpty: the side list of a call of two (call.others: side, one
// participant besides the self view) folds for every line; the stage widens to the
// column's edge but for a slim column of controls, stacked down from the column's top
// and, in a full call, above the window. The caller's lines put the participant in the
// small view.
func TestCallSideFoldsWhenEmpty(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {1920, 1080}, {1280, 960}} {
		sw, sh := size[0], size[1]
		top := sh - 242*sh/720
		for name, l := range map[string]callLayout{"full": callGeometryFull(sw, sh, top, true), "framed": sideOthers(callGeometry(sw, sh), &data.EnsembleCall{Others: "side"})} {
			right := l.chat.x + l.chat.w
			f := foldSide(l)
			last := f.controlsY + 5*f.controlsDY + f.controlR
			if f.chat.w != 0 || f.main.w <= l.main.w || f.main.x+f.main.w+12+2*f.controlR+16 != right {
				t.Errorf("%vx%v %s did not widen the stage to the column: %+v", sw, sh, name, f)
			}
			if f.controlsX-f.controlR < f.main.x+f.main.w || f.controlsX+f.controlR > right || f.controlDX != 0 || f.controlsDY <= 2*f.controlR {
				t.Errorf("%vx%v %s controls %v,%v do not stand in their column", sw, sh, name, f.controlsX, f.controlsY)
			}
			if l.full && last > top {
				t.Errorf("%vx%v the folded controls run under the window: %v > %v", sw, sh, last, top)
			}
			if f.self.x+f.self.w > f.main.x+f.main.w || f.self.x < f.main.x {
				t.Errorf("%vx%v %s self view %+v leaves the stage %+v", sw, sh, name, f.self, f.main)
			}
		}
	}
	g := callGame(t, "{address: call.example.test, layout: full, captions: overlay, others: side, self: me.png, self_stage: true, controls: [Mic, Camera, Leave]}")
	g.st.Planning.Entities["weaver"].Observations["visible"] = false
	g.db.Game.Screens["studio"].View.Order = []string{"potter"}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "potter", Name: "Potter", Text: "Just us."})
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	(&authorScreen{name: "studio"}).drawCall(g, dst)
	g.talk.drawNovel(g, dst)
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "me", Name: "Me", Text: "And me."})
	(&authorScreen{name: "studio"}).drawCall(g, dst)
	g.talk.drawNovel(g, dst)
}

func TestCallDrawingPreservesSimulation(t *testing.T) {
	for _, config := range []string{
		"{address: call.example.test, controls: [{label: Microphone, icon: mic}, {label: Camera, icon: camera}, {label: Share, icon: share}, {label: Hand, icon: hand}, {label: Chat, icon: chat}, {label: Leave, icon: leave}]}",
		"{layout: full, captions: overlay, others: side, others_title: Makers}",
		"{self: potter, self_stage: true, others: side}",
		"{self: x.png, self_label: Caller, self_stage: true, still: true}",
	} {
		t.Run(config, func(t *testing.T) {
			g := callGame(t, config)
			view := &g.db.Game.Screens["studio"].View
			view.Fullscreen = true
			view.Portrait, view.Icon, view.Backdrop, view.Badge = "portrait", "icon", "backdrop", "badge"
			for id, e := range g.st.Planning.Entities {
				e.Observations["portrait"] = "x.png"
				e.Observations["icon"] = "x.png"
				e.Observations["backdrop"] = "x.png"
				e.Observations["badge"] = id
			}
			if view.Call.Self == "potter" {
				view.Order = view.Order[:2]
			}
			s := newAuthorScreen(g, "studio", nil, nil)
			dst := covScreen()
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			random, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			for _, speaker := range []string{"potter", "weaver", "caller", ""} {
				g.talk.open(g, event.Say{Ensemble: "studio", Actor: speaker, Name: speaker, Text: "The workshop is ready."})
				g.talk.meeting.post("Weaver", strings.Repeat("The materials are ready. ", 20), false)
				g.talk.meeting.post("Potter", "joined", true)
				g.talk.meeting.reactions["potter"] = "Ready to begin"
				messages := slices.Clone(g.talk.meeting.messages)
				for _, tick := range []int{0, 30, 120} {
					g.tick = tick
					s.draw(g, dst)
					if !slices.Equal(messages, g.talk.meeting.messages) || g.callerSpeaks {
						t.Fatal("drawing changed transcript or leaked speaker override")
					}
				}
			}
			after, err := json.Marshal(g.st)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("call presentation changed saved state", err)
			}
			randomAfter, err := g.pcg.MarshalBinary()
			if err != nil || !bytes.Equal(random, randomAfter) {
				t.Fatal("call presentation consumed randomness", err)
			}
		})
	}
}

// TestCallMutedChoice: a muted choice on a call shows only the unmute button
// and strikes the call's mic. OK on the button, or a click on the mic, brings
// the options; left alone for its seconds, the choice is answered past its
// options (the player kept quiet).
func TestCallMutedChoice(t *testing.T) {
	g := callGame(t, "{address: call.example.test, controls: [{label: Mic, icon: mic}, {label: Leave, icon: leave}]}")
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	ask := func() {
		g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "Anyone?"})
		g.talk.active, g.talk.closed = false, g.tick
		g.talk.say.Text = ""
		g.talk.ask(g, event.Choice{Options: []string{"Red", "Blue"}, Muted: 1})
	}
	step := func(in input) (bool, int) {
		g.in = in
		return g.talk.update(g)
	}

	ask()
	if g.talk.held == nil || len(g.talk.choice.items) != 1 || g.talk.answers() != 3 {
		t.Fatalf("the muted choice shows %d items (held %v), takes %d answers", len(g.talk.choice.items), g.talk.held != nil, g.talk.answers())
	}
	var in input
	in.pressed[btnOK] = true
	if done, _ := step(in); done || g.talk.held != nil || len(g.talk.choice.items) != 2 {
		t.Fatal("OK on the unmute button did not bring the options")
	}

	ask()
	g.talk.draw(g, dst)
	if g.talk.micR == 0 {
		t.Fatal("the call did not draw its mic while muted")
	}
	if done, _ := step(input{click: true, mouse: image.Pt(int(g.talk.mic.x), int(g.talk.mic.y))}); done || g.talk.held != nil {
		t.Fatal("a click on the call's mic did not unmute")
	}
	for range 120 {
		if done, _ := step(input{}); done {
			t.Fatal("an unmuted choice ran out of time")
		}
	}

	ask()
	step(input{})
	left := g.talk.muted
	g.talk.reword(g)
	if g.talk.held == nil || len(g.talk.choice.items) != 1 || g.talk.muted != left {
		t.Fatalf("a language change unmuted the choice or reset its time: held %v, %d items, %d ticks left of %d", g.talk.held != nil, len(g.talk.choice.items), g.talk.muted, left)
	}
	for i := 1; i < 60; i++ {
		if done, answer := step(input{}); done {
			if i != 59 || answer != 2 {
				t.Fatalf("kept quiet after %d ticks with answer %d, want 60 and 2", i+1, answer)
			}
			g.talk.draw(g, dst)
			if g.talk.held != nil || g.talk.micR != 0 {
				t.Fatal("the mic stayed muted after the player kept quiet")
			}
			return
		}
	}
	t.Fatal("the muted choice did not run out")
}

// TestCallHeadFraming keeps the hair inside a call view: a face whose hair fits keeps
// its framing, a little hair too high moves the face down, much hair moves it down a
// tenth of the view and shrinks it, and a picture drawn cut at its own top is framed
// as before.
func TestCallHeadFraming(t *testing.T) {
	t.Parallel()
	bounds := image.Rect(0, 0, 400, 1200)
	face := image.Rect(150, 200, 250, 300) // 100 pixels high
	const y, fh = 50.0, 500.0
	headTop := func(content image.Rectangle, top, height float64) float64 {
		return top - height*float64(face.Min.Y-content.Min.Y)/float64(face.Dy())
	}
	for _, c := range []struct {
		name    string
		content image.Rectangle
		moved   bool
		shrunk  bool
	}{
		{"fits", image.Rect(100, 180, 300, 1200), false, false},
		{"a little hair above", image.Rect(100, 150, 300, 1200), true, false},
		{"much hair above", image.Rect(100, 20, 300, 1200), true, true},
		{"drawn cut at its top", image.Rect(100, 0, 300, 1200), false, false},
	} {
		top, height := callHeadFraming(bounds, face, c.content, y, fh)
		if moved := top != y+fh*.22; moved != c.moved {
			t.Errorf("%s: face top %v, moved %v, want moved %v", c.name, top, moved, c.moved)
		}
		if shrunk := height < fh*.48; shrunk != c.shrunk {
			t.Errorf("%s: face height %v, shrunk %v, want %v", c.name, height, shrunk, c.shrunk)
		}
		if top > y+fh*.32+1e-9 {
			t.Errorf("%s: face top %v below a tenth more than its place", c.name, top)
		}
		if c.content.Min.Y > 0 {
			if h := headTop(c.content, top, height); h < y+fh*.02-1e-9 {
				t.Errorf("%s: hair top %v above the view's top %v", c.name, h, y)
			}
		}
	}
	// No face found: the upper middle quarter stands for it, and the hair still fits.
	top, height := callHeadFraming(bounds, image.Rectangle{}, image.Rect(100, 0, 300, 1200), y, fh)
	if top != y+fh*.22 || height != fh*.48 {
		t.Fatalf("a picture opaque to its top and no face: %v %v, want the plain framing", top, height)
	}
}
