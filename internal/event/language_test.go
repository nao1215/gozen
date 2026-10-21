package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

const twoTongues = `
events:
  talk:
    do:
      - say: {who: Courier, text: "Gold {gold}."}
      - choice:
          - {label: Stay, do: []}
          - {label: Go, do: []}
        prompt: "Where now? ({gold})"
`

// TestLanguageChangedMidwayRewordsWhatIsOnScreen (ADR 0027): a line already shown is shown again in the new language, with the values
// its {expressions} had when it was shown; so are a question and its options. Changing back restores the source words.
func TestLanguageChangedMidwayRewordsWhatIsOnScreen(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":      "schema_version: 4\nid: tongues\nwritten_in: en\nlanguages: [en, ja]\n" + testgame.Game,
		"data/talk.yaml": twoTongues,
		"lang/ja.yaml": `text:
  "Gold {gold}.": "所持金 {gold}。"
  "Courier": "飛脚"
  "Where now? ({gold})": "どうする？（{gold}）"
  "Stay": "残る"
  "Go": "行く"
`,
	})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["talk"].Do)

	say := testgame.As[event.Say](t, r.Next())
	if say.Text != "Gold 100." || say.Name != "Courier" {
		t.Fatalf("shown in the source language: %+v", say)
	}
	st.Gold = 5 // the purse changes while the line is on the screen
	db.Show("ja")
	if got := say.Reword(db); got.Text != "所持金 100。" || got.Name != "飛脚" {
		t.Errorf("reworded: %+v (the value is the one shown, 100)", got)
	}
	db.Show("en")
	if got := say.Reword(db); got.Text != "Gold 100." || got.Name != "Courier" {
		t.Errorf("back in the source language: %+v", got)
	}
	db.Show("ja")
	db.Show("ja") // choosing the same language again changes nothing
	if got := say.Reword(db); got.Text != "所持金 100。" {
		t.Errorf("%+v", got)
	}

	db.Show("en")
	q := testgame.As[event.Choice](t, r.Next())
	db.Show("ja")
	if q.Prompt != "Where now? (5)" {
		t.Fatalf("the question was made in the source language: %q", q.Prompt)
	}
	st.Gold = 77
	got := q.Reword(db)
	if got.Prompt != "どうする？（5）" || len(got.Options) != 2 || got.Options[0] != "残る" || got.Options[1] != "行く" {
		t.Errorf("reworded question: %+v", got)
	}
	if q.Question().Text != "Where now? (5)" || q.Question().Reword(db).Text != "どうする？（5）" {
		t.Errorf("the question as a line: %+v", q.Question().Reword(db))
	}
	// the choice still answers by number, whatever language shows it
	r.Answer(1)
	// an untranslated or unknown language falls back to the source
	db.Show("fr")
	if got := say.Reword(db); got.Text != "Gold 100." {
		t.Errorf("an unknown language shows the source: %q", got.Text)
	}
}

// TestRewordOfALineWithAnotherSpeakerKeepsTheName: a speaker who is a party member is shown by the actor's own name.
func TestRewordOfAnActorSpeaker(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":      "schema_version: 4\nid: tongues\nwritten_in: en\nlanguages: [en, ja]\n" + testgame.Game,
		"data/talk.yaml": "events: {hi: {do: [{say: {who: scout, text: Hello}}]}}\n",
		"lang/ja.yaml":   "text:\n  \"Hello\": \"こんにちは\"\n  \"Scout\": \"斥候\"\n",
	})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["hi"].Do)
	say := testgame.As[event.Say](t, r.Next())
	db.Show("ja")
	got := say.Reword(db)
	if got.Text != "こんにちは" || got.Name == "" {
		t.Errorf("%+v", got)
	}
}
