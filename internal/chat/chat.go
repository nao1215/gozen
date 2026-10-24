// Package chat keeps a flowing log of short messages from any sender: the player, bots,
// or remote players. It does not judge messages (see package react) and does not
// transport them: a Source is the plug for input, bots or a network. Messages from
// people should pass a Filter such as Mask before they are shown.
// See docs/decisions/0015-reaction-and-chat.md.
package chat

import (
	"maps"
	"time"
	"unicode/utf8"
)

// Message is one chat message.
type Message struct {
	// ID is assigned by Log.Post, increasing from 1.
	ID   uint64
	From string
	Text string
	// At is the game time the message was posted.
	At time.Duration
	// Values are numbers attached to the message (a tip). Log copies the map when a
	// message is posted and when it is returned, so a caller never shares it with the log.
	Values map[string]float64
}

// Source supplies the messages that arrived up to the game time now.
type Source interface {
	Poll(now time.Duration) []Message
}

// Filter rewrites a message before it is logged; ok false drops it.
type Filter interface {
	Filter(m Message) (out Message, ok bool)
}

// Config sets up a Log. The zero value keeps 200 messages of up to 200 runes.
type Config struct {
	// Max is how many messages are kept (0 means 200).
	Max int
	// MaxRunes cuts longer texts (0 means 200).
	MaxRunes int
	// Filters run in order on every posted message.
	Filters []Filter
}

// Log is the recent messages, oldest first. It is not safe for concurrent use.
type Log struct {
	cfg  Config
	msgs []Message
	next uint64
}

// New makes an empty log.
func New(cfg Config) *Log {
	if cfg.Max <= 0 {
		cfg.Max = 200
	}
	if cfg.MaxRunes <= 0 {
		cfg.MaxRunes = 200
	}
	return &Log{cfg: cfg, next: 1}
}

// Post cuts the text, runs the filters and logs the message with a new ID. It returns
// a copy of the logged message, or ok false when a filter dropped it. The log keeps its
// own copy of m.Values.
func (l *Log) Post(m Message) (Message, bool) {
	m.Values = maps.Clone(m.Values)
	if utf8.RuneCountInString(m.Text) > l.cfg.MaxRunes {
		m.Text = string([]rune(m.Text)[:l.cfg.MaxRunes])
	}
	for _, f := range l.cfg.Filters {
		var ok bool
		if m, ok = f.Filter(m); !ok {
			return Message{}, false
		}
	}
	m.ID = l.next
	l.next++
	l.msgs = append(l.msgs, cloneMessage(m))
	if over := len(l.msgs) - l.cfg.Max; over > 0 {
		l.msgs = append(l.msgs[:0], l.msgs[over:]...)
	}
	return cloneMessage(m), true
}

// cloneMessage copies the Values map so the log and its callers never share it.
func cloneMessage(m Message) Message {
	m.Values = maps.Clone(m.Values)
	return m
}

// cloneMessages copies ms and each message's Values.
func cloneMessages(ms []Message) []Message {
	if len(ms) == 0 {
		return nil
	}
	out := make([]Message, len(ms))
	for i, m := range ms {
		out[i] = cloneMessage(m)
	}
	return out
}

// Pull polls the sources in order and posts what they return, with At set to now when
// it is zero. It returns the logged messages.
func (l *Log) Pull(now time.Duration, srcs ...Source) []Message {
	var out []Message
	for _, s := range srcs {
		for _, m := range s.Poll(now) {
			if m.At == 0 {
				m.At = now
			}
			if m, ok := l.Post(m); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// Recent is up to the last n messages, oldest first. The slice and each message's
// Values are copies.
func (l *Log) Recent(n int) []Message {
	n = min(max(n, 0), len(l.msgs))
	return cloneMessages(l.msgs[len(l.msgs)-n:])
}

// Since is the kept messages with an ID above id, oldest first. The slice and each
// message's Values are copies.
func (l *Log) Since(id uint64) []Message {
	for i, m := range l.msgs {
		if m.ID > id {
			return cloneMessages(l.msgs[i:])
		}
	}
	return nil
}
