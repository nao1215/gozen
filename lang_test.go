package gozen_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestTranslatedSession: the same game loaded with an overlay that shows it in Japanese
// gives Japanese requests, and a policy prefers an option by its translated label
// (ADR 0027).
func TestTranslatedSession(t *testing.T) {
	head := "schema_version: 4\nid: twotongues\nwritten_in: en\nlanguages: [en, ja]\n"
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte(head + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: []byte("a stub")},
		"data/talk.yaml": {Data: []byte(`events:
  talk:
    do:
      - say: {who: Courier, text: "Gold {gold}."}
      - choice:
          - {label: Stay, do: [{set: {var.way: 1}}]}
          - {label: Go, do: [{set: {var.way: 2}}]}
        prompt: "Where now?"
`)},
		"lang/ja.yaml": {Data: []byte(`text:
  "Gold {gold}.": "所持金 {gold}。"
  "Courier": "飛脚"
  "Where now?": "どうする？"
  "Stay": "残る"
  "Go": "行く"
`)},
	}
	run := func(g *gozen.Game, prefer string) []gozen.Request {
		var got []gozen.Request
		s, err := gozen.New(g, gozen.Config{Seed: 1, Policy: gozen.DefaultPolicy{Prefer: []string{prefer}}, OnRequest: func(r gozen.Request) { got = append(got, r) }})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Do(context.Background(), "- call: talk"); err != nil {
			t.Fatal(err)
		}
		var way int
		if err := s.Query("var.way", &way); err != nil || way != 2 {
			t.Fatalf("took way %d (%v)", way, err)
		}
		return got
	}
	en, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if got := run(en, "Go"); got[0].Text != "Gold 100." || got[0].Who != "Courier" || got[1].Text != "Where now?" {
		t.Fatalf("English %+v", got)
	}
	ja, err := gozen.Load(fsys, ".", fstest.MapFS{"game.yaml": {Data: []byte(head + "language: ja\n" + testgame.Game)}})
	if err != nil {
		t.Fatal(err)
	}
	got := run(ja, "行く")
	if got[0].Text != "所持金 100。" || got[0].Who != "飛脚" || got[1].Text != "どうする？" || got[1].Options[1] != "行く" {
		t.Fatalf("Japanese %+v", got)
	}
}
