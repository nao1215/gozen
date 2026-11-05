package play

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
)

// TestCallViewShareTakesTheStage: call_view.share puts the shared picture on the
// stage of the call its sharer is in, and leaves everyone in the strip; an empty
// picture or a line outside the call ends it, and the state is untouched.
func TestCallViewShareTakesTheStage(t *testing.T) {
	g := callGame(t, "{address: call.example.test}")
	v := g.db.Game.Screens["studio"].View
	all := g.ensembleMembers(v)
	before := g.st.Planning.Entities["potter"].Observations["camera"]
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	l := g.callLayout(1280, 720, v.Call)
	if g.drawCallShare(dst, v, all, l) {
		t.Fatal("a call with no share drew one")
	}
	g.Handle(event.CallShareEffect{Who: "potter", Picture: "slide.png"})
	if g.callSharer(all) < 0 || !g.drawCallShare(dst, v, all, l) {
		t.Fatal("the sharer's picture did not take the stage")
	}
	if pics := g.callPictures(v, all); len(pics) == 0 || !slices.Contains(pics, "slide.png") {
		t.Fatalf("the shared picture is not warmed: %v", pics)
	}
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "As you see."})
	(&authorScreen{name: "studio"}).drawCall(g, dst)
	if g.callShare == nil {
		t.Fatal("a line of the call ended the share")
	}
	g.Handle(event.CallShareEffect{Who: "nobody", Picture: "slide.png"})
	if g.callSharer(all) >= 0 {
		t.Fatal("a share by someone outside the call showed on it")
	}
	g.Handle(event.CallShareEffect{Who: "potter", Picture: ""})
	if g.callShare != nil {
		t.Fatal("an empty picture did not end the share")
	}
	g.Handle(event.CallShareEffect{Who: "potter", Picture: "slide.png"})
	g.talk.open(g, event.Say{Text: "Back at the desk."})
	if g.callShare != nil {
		t.Fatal("a line outside the call kept the share")
	}
	if g.st.Planning.Entities["potter"].Observations["camera"] != before {
		t.Fatal("sharing changed the state")
	}
}
