package event

import (
	"fmt"
	"strings"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/react"
)

// respond reconstructs only interpreter memory. Randomness and time belong to
// the game, so saving, resuming and replaying use the normal event contracts.
func (r *Runner) respond(q *data.Respond) {
	book := r.db.ReactionBooks[q.Who]
	if book == nil {
		r.logf(fmt.Errorf("respond: speaker %q has no reaction book", q.Who))
		return
	}
	mind, err := react.New(book, q.Who, react.Config{
		Rand:  r.st.Rand,
		Clock: func() time.Duration { return time.Duration(r.st.Frames) * time.Second / 60 },
	})
	if err != nil {
		r.logf(err)
		return
	}
	if saved := r.st.Reactions[q.Who]; len(saved) > 0 {
		if err := mind.Restore(saved); err != nil {
			r.logf(err)
			return
		}
	}
	from := data.Render(r.db.Tr(q.From), r.env())
	if from == "" {
		from = "player"
	}
	reply, err := mind.React(react.Stimulus{Kind: react.KindSay, From: from, Text: data.Render(r.db.Tr(q.Text), r.env())})
	if err != nil {
		r.logf(err)
		return
	}
	saved, err := mind.Save()
	if err != nil {
		r.logf(err)
		return
	}
	if r.st.Reactions == nil {
		r.st.Reactions = map[string][]byte{}
	}
	r.st.Reactions[q.Who] = saved
	var commands []data.Command
	for _, line := range reply.Lines {
		commands = append(commands, data.Command{Say: &data.Say{Who: q.Who, Text: strings.ReplaceAll(line, "{", "{{"), Face: reply.Face}})
	}
	r.push(commands)
}
