package play

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/base/traceformat"
	"github.com/nao1215/gozen/internal/rpg"
)

func inboxMsg(who, text, event string) rpg.InboxMessage {
	return rpg.InboxMessage{Who: who, Text: text, Event: event}
}

// A message waits in the inbox and shows as a notification on a full-screen
// surface; a pick or a click opens it: it leaves the inbox and its event runs
// under the bubble, then the screen is back. The screen's action does not run.
func TestInboxNotificationRunsItsDecision(t *testing.T) {
	var buf bytes.Buffer
	g := openFlowGame(t, &buf)
	g.st.Vars["skip"] = 1
	g.st.Inbox = append(g.st.Inbox, inboxMsg("guide", "Kiln is late.\nSee you at noon.", "brief"), inboxMsg("scout", "Back by six.", "brief"))
	g.openScreen("act_board", func() {})
	wldFrames(t, g, 4)
	g.Draw(covScreen())
	wldPlay(t, g, "look\nwait 2\n", 20)
	looks := traceLines(t, buf.Bytes(), "look")
	l := looks[len(looks)-1]
	if l["inbox"] != 2.0 || !strings.Contains(strings.Join(traceformat.Strings(l["controls"]), ","), "Guide") {
		t.Fatal("the notification is not a control", l)
	}
	briefed, worked := g.st.Vars["briefed"], g.st.Vars["worked"]
	wldPlay(t, g, "pick Guide\nwait 4\n", 30)
	if g.inboxShown == nil || g.inboxShown.Who != "guide" || len(g.st.Inbox) != 1 {
		t.Fatal("opening did not show the message and take it from the inbox", g.inboxShown, g.st.Inbox)
	}
	g.Draw(covScreen())
	wldPlay(t, g, "look\nwait 2\npress z\nwait 10\n", 60)
	if g.inboxShown != nil {
		t.Fatal("the bubble stayed after its event")
	}
	if g.st.Vars["briefed"] == briefed || g.st.Vars["worked"] != worked {
		t.Fatal("the message's event did not run, or the screen's action ran", g.st.Vars)
	}
	r := g.inboxToastRect()
	wldPlay(t, g, "click "+itoa(r.Min.X+20)+" "+itoa(r.Min.Y+20)+"\nwait 4\npress z\nwait 10\n", 60)
	if len(g.st.Inbox) != 0 {
		t.Fatal("a click did not open the last message", g.st.Inbox)
	}
	if len(traceLines(t, buf.Bytes(), "message_open")) != 2 {
		t.Fatal("message_open is not traced for each message")
	}
}

// The message command queues without stopping the event.
func TestMessageCommandQueues(t *testing.T) {
	g := openFlowGame(t, nil)
	wldPlay(t, g, "do message: {who: guide, text: 'Done {var.n}', event: brief}\nwait 4\n", 20)
	if len(g.st.Inbox) != 1 || g.st.Inbox[0].Who != "guide" || g.st.Inbox[0].Text != "Done 0" || g.st.Inbox[0].Event != "brief" {
		t.Fatal(g.st.Inbox)
	}
}
