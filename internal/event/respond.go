package event

import (
	"fmt"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/react"
)

// respond reconstructs only interpreter memory. Randomness and time belong to
// the game, so saving, resuming and replaying use the normal event contracts.
func (r *Runner) respond(q *data.Respond) {
	mind, err := r.st.Mind(q.Who, r.st.Rand)
	if err != nil {
		r.logf(fmt.Errorf("respond: %w", err))
		return
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
	if err := r.st.KeepMind(q.Who, mind); err != nil {
		r.logf(err)
		return
	}
	// each line as the book writes it, filled as the book filled it: translated and read
	// as any line of the source (ADR 0027)
	var commands []data.Command
	for i, line := range reply.Written {
		commands = append(commands, data.Command{Say: &data.Say{Who: q.Who, Text: line, Face: reply.Face, Values: reply.Fields[i]}})
	}
	r.push(commands)
}
