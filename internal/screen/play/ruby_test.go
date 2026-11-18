package play

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
)

func TestParseRuby(t *testing.T) {
	for _, c := range []struct {
		in, plain string
		spans     []rubySpan
	}{
		{"no marks", "no marks", nil},
		{"|魔法使い《ウィザード》が来た", "魔法使いが来た", []rubySpan{{0, 4, "ウィザード"}}},
		{"彼は｜Gozen《ごぜん》を使う", "彼はGozenを使う", []rubySpan{{2, 7, "ごぜん"}}},
		// Without a bar the base is the run of kanji before the mark.
		{"雪の峠《とうげ》を越えた", "雪の峠を越えた", []rubySpan{{2, 3, "とうげ"}}},
		{"佐々木《ささき》と東京《とうきょう》", "佐々木と東京", []rubySpan{{0, 3, "ささき"}, {4, 6, "とうきょう"}}},
		// A mark with no base, an empty reading or no closing mark is kept as written.
		{"《注》を見よ", "《注》を見よ", nil},
		{"空《》", "空《》", nil},
		{"開き《だけ", "開き《だけ", nil},
		// A bar with no reading after it, and the first of two bars, are text.
		{"a|b《》", "a|b《》", nil},
		{"x|y|語《ご》", "x|y語", []rubySpan{{3, 4, "ご"}}},
		// A bar before a mark with no base stays text and does not reach the next reading.
		{"|《注》漢字《かんじ》", "|《注》漢字", []rubySpan{{4, 6, "かんじ"}}},
	} {
		plain, spans := parseRuby(c.in)
		if plain != c.plain || !reflect.DeepEqual(spans, c.spans) {
			t.Errorf("%q: %q %v, want %q %v", c.in, plain, spans, c.plain, c.spans)
		}
	}
}

// TestRubyInTheWindow: a line with a reading is typed out and logged without its marks.
func TestRubyInTheWindow(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	g.talk.say.Text = "|魔法使い《ウィザード》が来た。"
	g.talk.layout(g)
	if got := g.talk.pages[0][0]; got != "魔法使いが来た。" || g.talk.total != 8 {
		t.Fatalf("page %q of %d runes", got, g.talk.total)
	}
	if len(g.talk.ruby) != 1 || len(g.talk.pageAt) != 1 || g.talk.pageAt[0] != 0 {
		t.Fatalf("ruby %v at %v", g.talk.ruby, g.talk.pageAt)
	}
	g.talk.drawNovel(g, btlImage())
	s := event.Say{Text: "|魔法使い《ウィザード》が来た。"}
	if _, text := (logLine{line: &s}).shown(g.db); text != "魔法使いが来た。" {
		t.Fatalf("the backlog shows %q", text)
	}
}

// TestRubyPlacesAfterAWrap: a reading at the end of a page shows once the page is typed
// out, though the wrap dropped the space before it; it waits for its whole base.
func TestRubyPlacesAfterAWrap(t *testing.T) {
	plain, spans := parseRuby("あい うえ 漢字《かんじ》")
	lines := []string{"あい うえ", "漢字"}
	if got := rubyPlaces(spans, plain, lines, 0, 7); len(got) != 1 || got[0] != (rubyPlace{1, "", "漢字", "かんじ"}) {
		t.Fatalf("typed out: %+v", got)
	}
	if got := rubyPlaces(spans, plain, lines, 0, 6); len(got) != 0 {
		t.Fatalf("half the base typed: %+v", got)
	}
}
