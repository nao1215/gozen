package chat

import (
	"strings"
	"testing"
	"time"
)

func FuzzMask(f *testing.F) {
	for _, s := range []string{"ばか", "ﾊﾞｶ", "\xff", "", strings.Repeat("ば か", 100)} {
		f.Add(s)
	}
	m := NewMask([]string{"ばか", "bad"})
	f.Fuzz(func(t *testing.T, s string) {
		out, ok := m.Filter(Message{Text: s})
		if !ok || len([]rune(out.Text)) != len([]rune(s)) {
			t.Fatalf("%q -> %q", s, out.Text)
		}
	})
}

func TestMaskCatchesTheDisguises(t *testing.T) {
	m := NewMask([]string{"bad", "ばか", "", "---"})
	cases := map[string]string{
		"bad":          "***",
		"BAD":          "***",
		"ｂａｄ":          "***",
		"b a d":        "* * *",
		"b-a_d":        "*****",
		"you are bad":  "you are ***",
		"ばか":           "**",
		"バカ":           "**",
		"ば-か":          "***",
		"ばーか":          "***",
		"ＢＡＤ！！":        "***！！",
		"nothing here": "nothing here",
		"":             "",
		"🙂bad🙂":        "🙂***🙂",
		"bbad":         "b***",
	}
	for in, want := range cases {
		got, ok := m.Filter(Message{Text: in})
		if !ok || got.Text != want {
			t.Errorf("%q -> %q (ok %v), want %q", in, got.Text, ok, want)
		}
	}
}

func TestMaskNormalizesBeforeMatching(t *testing.T) {
	m := NewMask([]string{"ばか", "ガキ", "bad"})
	cases := map[string]string{
		"ﾊﾞｶ":             "***",
		"は\u3099か":        "***",
		"ﾊﾞｶ!":            "***!",
		"x\u30ab\u3099ｷy": "x***y",
		"ｶﾞｷ":             "***",
		"ﾊﾟｶ":             "ﾊﾟｶ",
		"ｂａｄ":             "***",
		"か\u3099き":        "***",
	}
	for in, want := range cases {
		got, _ := m.Filter(Message{Text: in})
		if got.Text != want {
			t.Errorf("%q -> %q, want %q", in, got.Text, want)
		}
	}
}

func TestMaskKeepsTheRestOfTheMessage(t *testing.T) {
	m := NewMask([]string{"bad"})
	in := Message{ID: 7, From: "x", Text: "so bad", At: time.Second, Values: map[string]float64{"tip": 3}}
	out, ok := m.Filter(in)
	if !ok || out.ID != 7 || out.From != "x" || out.At != time.Second || out.Values["tip"] != 3 || out.Text != "so ***" {
		t.Errorf("%+v", out)
	}
	if in.Text != "so bad" {
		t.Error("the input message was changed")
	}
}

func TestMaskNormalizesHangulSequences(t *testing.T) {
	for _, text := range []string{"가", "\u1100\u1161", "\u3131\u314f"} {
		got, _ := NewMask([]string{"가"}).Filter(Message{Text: "x" + text + "y"})
		want := "x" + strings.Repeat("*", len([]rune(text))) + "y"
		if got.Text != want {
			t.Errorf("%q: got %q, want %q", text, got.Text, want)
		}
	}
}
