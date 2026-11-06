package play

import (
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/rpg"
)

func TestInstantPages(t *testing.T) {
	for _, instant := range []bool{false, true} {
		line := talk{say: event.Say{Instant: instant}, pages: [][]string{{"First page"}, {"Second page"}}}
		for page := range line.pages {
			line.setPage(page)
			if instant && line.shown != line.total || !instant && line.shown != 0 {
				t.Fatalf("instant %v, page %d: %d/%d", instant, page, line.shown, line.total)
			}
		}
	}
}

const mutedData = `events:
  mt:
    do:
      - say: "Start."
      - choice:
          - {label: Speak, do: [{set: {var.spoke: 1}}]}
          - {label: Agree, do: [{set: {var.spoke: 2}}]}
        muted: {seconds: 0.5, do: [{set: {flag.quiet: true}}]}
      - say: "After."
      - say: "End."
`

func mutedGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/muted.yaml"] = &fstest.MapFile{Data: []byte(mutedData)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "camp", Event: "mt"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestMutedChoiceSaveReplays: a muted choice left alone keeps quiet, and a
// save after it loads with the same state; one unmuted and answered loads with
// the option taken. Both replay the saved answer through the muted choice.
func TestMutedChoiceSaveReplays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		unmute bool
		check  func(st map[string]any, quiet bool) bool
	}{
		{"kept quiet", false, func(v map[string]any, quiet bool) bool { return quiet && v["spoke"] == nil }},
		{"unmuted", true, func(v map[string]any, quiet bool) bool { return !quiet && v["spoke"] == float64(2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := mutedGame(t)
			for range 60 {
				if g.talk.held != nil {
					break
				}
				if g.fast == nil && at(g) == "Start." {
					btlKey(g, btnOK)
				}
				sysFrames(t, g, 1)
			}
			if g.talk.held == nil {
				t.Fatalf("the muted choice did not come up: at %q", at(g))
			}
			if tc.unmute {
				btlKey(g, btnOK)
				btlKey(g, btnDown)
				btlKey(g, btnOK)
			}
			for range 60 {
				if at(g) == "After." {
					break
				}
				sysFrames(t, g, 1)
			}
			if at(g) != "After." || !tc.check(g.st.Vars, g.st.Flags["quiet"]) {
				t.Fatalf("at %q, vars %v, flags %v", at(g), g.st.Vars, g.st.Flags)
			}
			saved, err := g.saveBytes()
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			h := mutedGame(t)
			t.Cleanup(func() {
				if err := h.Close(); err != nil {
					t.Error(err)
				}
			})
			sysFrames(t, h, 2)
			st, err := rpg.Parse(h.db, saved, nil)
			if err != nil {
				t.Fatal(err)
			}
			h.loadSavedGame(st)
			if h.failure != nil {
				t.Fatal(h.failure)
			}
			if at(h) != "After." || !tc.check(h.st.Vars, h.st.Flags["quiet"]) {
				t.Fatalf("loaded at %q, vars %v, flags %v", at(h), h.st.Vars, h.st.Flags)
			}
		})
	}
}
