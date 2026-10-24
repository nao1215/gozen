package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/memo"
	lang "github.com/nao1215/gozen/internal/text"
)

func shotImage() image.Image {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 36))
	im.Set(3, 3, color.NRGBA{200, 10, 10, 255})
	return im
}

func writeMemo(t *testing.T, root string, m memo.Memo, withShot bool) {
	t.Helper()
	var shot image.Image
	if withShot {
		shot = shotImage()
	}
	if _, err := memo.Write(root, &m, shot, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPlayNotesRendersEveryKindOfNote(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	root := filepath.Join(t.TempDir(), "notes")
	writeMemo(t, root, memo.Memo{
		Source: memo.Human, Kind: "good", Label: "Good", Text: "Nice <view>", Time: "2026-01-02T03:04:05Z", Seconds: 125, Language: "en",
		Location: "camp", Place: "Camp", Scenes: []string{"field", "dialog"}, Event: "intro", Who: "Mira", Line: "Hello & welcome", Version: "v1", Image: "shot.png",
	}, true)
	writeMemo(t, root, memo.Memo{
		Source: memo.Bot, Bot: "Walker", Kind: "dull", Label: "Dull", Time: "2026-01-02T04:00:00Z",
		Location: "cave", Place: "Cave", Floor: 2, Pos: []int{1, 1}, Facing: "north",
		Map: []string{"#####", "#...#", "#.D.#", "#####"}, Seen: []string{"00000", "01110", "01100", "00000"},
		Vars: map[string]any{"depth": 2.0, "key": "x"}, Data: map[string]any{"persona": "walker", "run": "r1"},
		Resume: &memo.Resume{From: "here", Event: "intro"},
	}, false)
	writeMemo(t, root, memo.Memo{Source: memo.LLM, Kind: "cold", Label: "Cold", Text: "", Time: "2026-01-02T04:30:00Z", Location: "cave", Place: "Cave", Floor: 2, Pos: []int{1, 1}}, false)
	writeMemo(t, root, memo.Memo{Source: memo.Human, Kind: "good", Label: "Good", Text: "g", Time: "2026-01-02T05:00:00Z", Location: "cave", Place: "Cave", Floor: 2, Pos: []int{2, 2}}, false)
	writeMemo(t, root, memo.Memo{Source: memo.Autoplay, Kind: "hot", Label: "Hot", Text: "h", Time: "2026-01-03T00:00:00Z", Location: "cave", Place: "Cave", Floor: 3, Pos: []int{0, 0}, Map: []string{"..."}}, false)
	for i, kind := range []string{"lost", "bug", "odd"} {
		writeMemo(t, root, memo.Memo{Source: "custom", Kind: kind, Label: kind, Text: kind, Time: "2026-01-04T0" + string(rune('0'+i)) + ":00:00Z", Location: "cave", Place: "Cave", Floor: 4, Pos: []int{i, 0}, Map: []string{"..."}}, false)
	}
	writeMemo(t, root, memo.Memo{Source: "custom", Kind: "odd", Label: "odd", Text: "elsewhere", Time: "2026-01-05T00:00:00Z"}, false)
	out := filepath.Join(t.TempDir(), "html")
	stdout, _ := invoke(t, 0, "notes", root, "--out", out)
	if !strings.Contains(stdout, "9 memos exported") {
		t.Fatalf("stdout: %q", stdout)
	}
	html := string(read(t, filepath.Join(out, "index.html")))
	for _, want := range []string{
		`<html lang="en">`,
		"Nice &lt;view&gt;",
		"Hello &amp; welcome",
		"<h2>Cave B2 (3)</h2>",
		"<h2>Cave B3 (1)</h2>",
		"<h2>Cave B4 (3)</h2>",
		"<h2>Camp (1)</h2>",
		"<h2>(no location) (1)</h2>",
		"B2 (1,1) north",
		"depth 2, key x",
		"persona walker, run r1",
		"v1",
		"field &gt; dialog",
		`class="pin">#1</span>`,
		`class="pin">#2</span>`,
		"<svg ",
		`fill="#7a8aa6"`,
		`fill="#3a4252"`,
		`fill="#6b6b6b"`,
		`fill="#3b6ea8"`,
		`fill="#b0457a"`,
		`fill="#2e7d4f"`,
		`fill="#a07a20"`,
		`fill="#b03a3a"`,
		`fill="#e0a030"`,
		"k-lost", "k-bug", "k-odd",
		`<option value="custom">custom</option>`,
		`<option value="bot">bot</option>`,
		"bot (Walker)",
		"LLM, Cave B2 (1,1)",
		"thumbs/2026-01-02_001.jpg",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	// The note without a text says so, and groups come in key order.
	if !regexp.MustCompile(`Cave B2.*Cave B3`).MatchString(strings.ReplaceAll(html, "\n", " ")) {
		t.Error("groups are in order")
	}
	read(t, filepath.Join(out, "images", "2026-01-02_001.png"))
	read(t, filepath.Join(out, "thumbs", "2026-01-02_001.jpg"))
}

func TestPlayNotesSpeaksTheLanguageOfTheNotes(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	root := filepath.Join(t.TempDir(), "notes")
	// A note from before the language was recorded was written with the Japanese words.
	writeMemo(t, root, memo.Memo{Source: memo.Human, Kind: "good", Label: "良い", Text: "いい", Time: "2026-01-02T03:04:05Z", Location: "camp"}, false)
	out := filepath.Join(t.TempDir(), "ja")
	invoke(t, 0, "notes", root, "--out", out)
	html := string(read(t, filepath.Join(out, "index.html")))
	if !strings.Contains(html, `<html lang="ja">`) || !strings.Contains(html, "テストプレイのメモ") {
		t.Errorf("an old note is shown in Japanese: %s", html[:min(300, len(html))])
	}
	// A language the engine has no words for falls back to English.
	root = filepath.Join(t.TempDir(), "notes")
	writeMemo(t, root, memo.Memo{Source: memo.Human, Kind: "good", Label: "Good", Text: "ok", Time: "2026-01-02T03:04:05Z", Language: "zz", Location: "camp"}, false)
	out = filepath.Join(t.TempDir(), "zz")
	invoke(t, 0, "notes", root, "--out", out)
	if html = string(read(t, filepath.Join(out, "index.html"))); !strings.Contains(html, `<html lang="en">`) || !strings.Contains(html, "<h2>camp (1)</h2>") {
		t.Errorf("unknown language: %s", html[:min(300, len(html))])
	}
}

func TestPlayNotesDefaultsAndFailures(t *testing.T) {
	root := filepath.Join(t.TempDir(), "notes")
	writeMemo(t, root, memo.Memo{Source: memo.Human, Kind: "good", Label: "Good", Text: "ok", Time: "2026-01-02T03:04:05Z", Language: "en", Location: "camp"}, false)
	// Without --out the page goes next to the notes.
	invoke(t, 0, "notes", root, "--out", root+"-html")
	read(t, root+"-html/index.html")
	invoke(t, 1, "notes", root, "--out", root+"-html")
	invoke(t, 2, "notes")
	invoke(t, 2, "notes", root, root)
	invoke(t, 1, "notes", filepath.Join(t.TempDir(), "absent"), "--out", filepath.Join(t.TempDir(), "x"))
	// The page never goes inside the notes.
	invoke(t, 1, "notes", root, "--out", filepath.Join(root, "page"))

	bad := filepath.Join(t.TempDir(), "bad")
	writeMemo(t, bad, memo.Memo{Source: memo.Human, Kind: "good", Label: "G", Time: "2026-01-02T03:04:05Z", Image: "gone.png"}, false)
	_, stderr := invoke(t, 1, "notes", bad, "--out", filepath.Join(t.TempDir(), "o1"))
	if stderr == "" {
		t.Error("an image the note names but the folder lacks is reported")
	}
	// A picture that is not a PNG cannot be shrunk.
	day := filepath.Join(bad, "2026-01-02")
	if err := os.WriteFile(filepath.Join(day, "gone.png"), []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr = invoke(t, 1, "notes", bad, "--out", filepath.Join(t.TempDir(), "o2"))
	if !strings.Contains(stderr, "gone.png") {
		t.Errorf("stderr: %q", stderr)
	}
}
