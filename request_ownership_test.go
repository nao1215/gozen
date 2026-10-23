package gozen_test

import (
	"testing"

	"github.com/nao1215/gozen"
)

func TestChoiceRequestsOwnTheirOptions(t *testing.T) {
	s := session(t, load(t), gozen.Config{OnRequest: func(q gozen.Request) {
		if q.Kind == "choice" {
			q.Options[0] = "changed by observer"
		}
	}})
	if err := s.Start("- choice:\n    - {label: Left}\n    - {label: Right}\n"); err != nil {
		t.Fatal(err)
	}
	q := s.Next()
	if q.Kind != "choice" || q.Options[0] != "Left" {
		t.Fatalf("observer changed request: %+v", q)
	}
	q.Options[0] = "changed by caller"
	if got := s.Next(); got.Options[0] != "Left" {
		t.Fatalf("caller changed pending request: %+v", got)
	}
}
