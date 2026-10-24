package chat

import "testing"

type retainedFilter struct{ values map[string]float64 }

func (f retainedFilter) Filter(m Message) (Message, bool) {
	m.Values = f.values
	return m, true
}

func TestPostOwnsFilterOutput(t *testing.T) {
	values := map[string]float64{"score": 1}
	l := New(Config{Filters: []Filter{retainedFilter{values}}})
	if _, ok := l.Post(Message{Text: "Hello"}); !ok {
		t.Fatal("message rejected")
	}
	values["score"] = 2
	if got := l.Recent(1)[0].Values["score"]; got != 1 {
		t.Fatalf("filter changed logged score to %v", got)
	}
}
