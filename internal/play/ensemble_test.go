package play

import (
	"image"
	"math"
	"testing"

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
