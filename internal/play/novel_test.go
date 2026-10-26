package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// ui.hide_bar_on_choice hides the buttons under the novel window while a choice is shown,
// and a click there no longer presses them; without it they stay.
func TestNovelBarHiddenOnChoice(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel, hide_bar_on_choice: true}\n", "")
	g.events.Start([]data.Command{{Prompt: "Which?", Choice: []data.ChoiceOption{{Label: "A"}, {Label: "B"}}}})
	btlUntil(t, g, "the choice", 300, func() bool { return g.talk.choice != nil })
	if g.novelBarShown() {
		t.Fatal("the bar shows under a choice")
	}
	r := g.novelButtonRects()[1] // the log
	g.in.mouse.X, g.in.mouse.Y, g.in.click = r.Min.X+2, r.Min.Y+2, true
	g.novelInput()
	if g.logView != nil || !g.in.click {
		t.Fatal("a click on the hidden log button was taken by it")
	}
	g.talk.draw(g, btlImage())
	g.db.Game.UI.HideBarOnChoice = false
	if !g.novelBarShown() {
		t.Fatal("the bar is hidden without ui.hide_bar_on_choice")
	}
}
