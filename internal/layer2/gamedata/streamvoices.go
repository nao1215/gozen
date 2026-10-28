package gamedata

import (
	"cmp"
	"slices"

	"github.com/nao1215/gozen/internal/react"
)

// StreamVoiceLine is one authoring record consumed by gozen book --section voices.
// Text is translated and filled when Fields is empty; File is then its recording
// path (empty without streams.voices). Otherwise Text is a template and Fields
// lists the runtime values still needed; such a record must not be synthesized.
type StreamVoiceLine struct {
	Channel  string   `json:"channel"`
	Language string   `json:"language"`
	Text     string   `json:"text"`
	File     string   `json:"file,omitempty"`
	Fields   []string `json:"fields,omitempty"`
}

// StreamVoiceLines returns a deterministic, deduplicated recording inventory for
// authoring tools, without reading audio or mutating language or reaction state.
// Reaction expansions are candidates, not a simulation of rule reachability.
func (db *DB) StreamVoiceLines() []StreamVoiceLine {
	lines := []StreamVoiceLine{}
	if db.Streams != nil {
		for _, ch := range db.Streams.Channels {
			if ch != nil {
				lines = append(lines, db.streamVoiceLines(ch)...)
			}
		}
	}
	return lines
}

func (db *DB) streamVoiceLines(ch *Channel) []StreamVoiceLine {
	s := db.Streams
	out := []StreamVoiceLine{}
	for _, code := range slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(db.Game.Offered()), db.Game.Written())))) {
		// A from-only reaction repeats across tip texts; keep only one candidate.
		seen := map[struct {
			text     string
			resolved bool
		}]bool{}
		add := func(source string, fields map[string]string) {
			if source == "" {
				return
			}
			text := db.TrIn(code, source)
			pending := sortedInterpolations(text)
			if len(pending) > 0 && fields != nil {
				for _, name := range pending {
					if _, ok := fields[name]; !ok {
						return // runtime state cannot be guessed by a recording tool
					}
				}
				text = react.Fill(text, fields)
				pending = nil
			}
			key := struct {
				text     string
				resolved bool
			}{text, len(pending) == 0}
			if seen[key] {
				return
			}
			seen[key] = true
			line := StreamVoiceLine{Channel: ch.ID, Language: code, Text: text, Fields: pending}
			if len(pending) == 0 {
				line.File = s.Voice(ch.ID, text)
			}
			out = append(out, line)
		}
		for _, l := range slices.Concat(ch.First, ch.Streak, ch.Stayed, ch.Bridges, s.Crowd.TipLines, []string{ch.VoiceFallback}) {
			add(l, nil)
		}
		for _, lines := range ch.Greet {
			for _, l := range lines {
				add(l, nil)
			}
		}
		episodes := func(eps []*Episode, host string) {
			for _, ep := range eps {
				for _, b := range ep.Beats() {
					if b.Line != nil && cmp.Or(b.Speaker, host) == ch.ID {
						add(b.Line.Text, nil)
					}
					if b.Ask == nil {
						continue
					}
					asker := cmp.Or(b.Ask.Who, host)
					if asker == ch.ID {
						add(b.Ask.Q, nil)
					}
					for i, o := range b.Ask.Options {
						for _, r := range o.R {
							if cmp.Or(r.Speaker, asker) != ch.ID {
								continue
							}
							add(r.Line.Text, nil) // the player may choose this option
							for _, npc := range b.Ask.NPC {
								if npc.Option == i {
									add(r.Line.Text, map[string]string{"from": db.TrIn(code, npc.Name), "days": "0"})
								}
							}
						}
					}
				}
			}
		}
		episodes(ch.Shows, ch.ID)
		for _, duo := range s.Duos {
			if duo != nil && (duo.Host == ch.ID || duo.With == ch.ID) {
				episodes(duo.Shows, duo.Host)
			}
		}
		if book := db.ReactionBooks[ch.Actor]; book != nil {
			for _, l := range book.Lines(ch.Actor) {
				add(l, nil)
				if len(sortedInterpolations(db.TrIn(code, l))) == 0 {
					continue
				}
				// Only bot tips reach the reaction book. Ordinary scripted chat is
				// displayed without asking the book for a response.
				for _, name := range s.Crowd.Names {
					for _, text := range s.Crowd.TipLines {
						add(l, map[string]string{"from": db.TrIn(code, name), "text": db.TrIn(code, text)})
					}
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b StreamVoiceLine) int {
		return cmp.Or(cmp.Compare(a.Language, b.Language), cmp.Compare(a.Text, b.Text), cmp.Compare(a.File, b.File))
	})
	return out
}
