package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
)

func TestStandCanLeaveAtTickZero(t *testing.T) {
	for _, leave := range []event.StandEffect{{None: true}, {Who: "Courier", Leave: "fade"}, {Who: "Courier", Leave: "slide"}} {
		t.Run(leave.Leave, func(t *testing.T) {
			g := stagedGame(t, Options{})
			g.tick = 0
			g.stand(event.StandEffect{Who: "Courier", Face: "x.png", At: 0.25})
			g.stand(leave)
			g.tick = castOut
			if figs := g.castFigures(""); len(figs) != 0 {
				t.Errorf("leave %+v retained %+v", leave, figs)
			}
		})
	}
}

func TestLoadClearsAllPreviousPortraits(t *testing.T) {
	g := stagedGame(t, Options{Location: "camp"})
	g.onStage(event.Say{Name: "Courier", Face: "x.png"})
	g.sideOf("name:Courier", "")
	g.leaveStage()
	g.onStage(event.Say{Name: "Guard", Face: "x.png"})
	g.loadGame(g.st)
	if figs := g.stageFigures(false, func(face string) bool { return face != "" }); len(figs) != 0 {
		t.Fatalf("loaded game retained portraits: %+v", figs)
	}
}
