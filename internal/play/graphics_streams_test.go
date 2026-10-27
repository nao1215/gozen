//go:build graphics

package play

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	lang "github.com/nao1215/gozen/internal/text"
)

// TestGraphicsScreensStreams draws the stream lobby, the watch view, a duo and the full
// screen: the cards and the stage show the puppets over their rooms, the cards begin at the
// left edge, the bar and the chat column are the light page, a line stands on a black
// caption band, and in full screen the stage fills the screen under a darkened chat that
// the settings keep.
func TestGraphicsScreensStreams(t *testing.T) {
	room := [3]uint32{80, 120, 170} // assets/x.png, the rooms
	differs := func(img image.Image, x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		d := func(a, b uint32) bool { return a>>8 > b+24 || a>>8+24 < b }
		return d(r, room[0]) || d(g, room[1]) || d(b, room[2])
	}
	near := func(img image.Image, x, y int, want [3]uint32) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		d := func(a, b uint32) bool { return a>>8 <= b+16 && a>>8+16 >= b }
		return d(r, want[0]) && d(g, want[1]) && d(b, want[2])
	}
	white := [3]uint32{255, 255, 255}
	bar, barJa, barH := tallBar(t)
	button := func(bs []barButton, pick func(barButton) bool) barButton { return bs[slices.IndexFunc(bs, pick)] }
	isEn := func(b barButton) bool { return b.lang == "en" }
	isJa := func(b barButton) bool { return b.lang == "ja" }
	ja, set := button(bar, isJa), button(barJa, func(b barButton) bool { return b.gear })
	tall := fmt.Sprintf("window 390 844\ngap 1\nwait 30\ntype Pat\npress ok\nwait 30\nshot lobby\nclick %d %d\nwait 20\nshot lobby_ja\nclick %d %d\nwait 30\nshot settings\nclick 30 20\nwait 30\npress ok\nwait 100\nshot watch\npress hide\nwait 20\nshot full\nquit\n",
		int(ja.x+ja.w/2), int(ja.y+15), int(set.x+set.w/2), int(set.y+15))
	brand := [3]uint32{0xe0, 0x45, 0x7b}
	// plate: the name plate's band, in the host's color: ann's, or bo's, who has none (the brand's)
	// fullButton: the full screen's button centred at x, y, its top left corner's arm drawn
	// white, out from the middle to go to the full screen, in to come back (full)
	fullButton := func(img image.Image, x, y int, full bool) bool {
		d := fullSize * 0.2
		if full {
			d = fullSize * 0.08
		}
		r, g, b, _ := img.At(x-6, y-int(d)).RGBA()
		return r>>8 > 200 && g>>8 > 200 && b>>8 > 200
	}
	plate := func(img image.Image, x, y int) bool {
		return near(img, x, y, [3]uint32{0x2a, 0x8c, 0x4a}) || near(img, x, y, brand)
	}
	speakerOn := func(img image.Image) bool { return near(img, 1205, 668, brand) } // the read-aloud button, filled when on
	voiced := func(fsys fstest.MapFS) {
		fsys["data/streams.yaml"].Data = append([]byte("voices: voice\n"), fsys["data/streams.yaml"].Data...)
		fsys["assets/voice/ann/.keep"] = &fstest.MapFile{}
	}
	for _, tc := range []struct {
		name, duo, script string
		edit              func(fstest.MapFS)
		check             func(t *testing.T, root string)
	}{
		{"single", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\nshot lobby\npress ok\nwait 100\nshot watch\nquit\n", nil, func(t *testing.T, root string) {
			lobby := shotImage(t, root, "lobby")
			// the bar, the first card's picture (its room from the left edge, its puppet) and its chat
			x, y, w, h := lobbyEdge, streamBarH+20, 1280-2*lobbyEdge-lobbyChatW, (1280-2*lobbyEdge-lobbyChatW)*9/16
			if !near(lobby, 27, 26, room) {
				t.Error("the bar does not draw the logo before the title")
			}
			if !near(lobby, 640, 6, white) || !near(lobby, int(x+60), int(y+60), room) || !differs(lobby, int(x+w/2), int(y+h/2)) || !near(lobby, int(x+w+lobbyChatW/2), int(y+4), white) {
				t.Error("the lobby is not drawn as the bar and a card from the left edge with its puppet and chat")
			}
			watch := shotImage(t, root, "watch")
			sw := 1280.0 - streamChatW
			if !differs(watch, int(sw/2), 300) || !near(watch, int(sw+streamChatW/2), 300, white) {
				t.Error("the watch view has no puppet on its stage or no chat column")
			}
			if darkRun(watch, int(sw/2), 420, int(streamBarH+sw*9/16)) < 10 {
				t.Error("no caption band under the line")
			}
			if !fullButton(watch, int(sw-fullGap-fullSize/2), int(streamBarH+sw*9/16-fullGap-fullSize/2), false) {
				t.Error("no full screen's button at the bottom right of the stage")
			}
			// the name plate at the bottom left of the stage, the caption above it
			if bottom := int(streamBarH + sw*9/16); !plate(watch, 20, bottom-24) || darkRun(watch, int(sw/2), bottom-42, bottom-10) > 6 {
				t.Error("no name plate at the bottom left of the stage, or the caption over it")
			}
			if speakerOn(watch) {
				t.Error("the desktop shows a read-aloud button without voices")
			}
		}},
		{"voices", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 100\nshot watch\nquit\n", voiced, func(t *testing.T, root string) {
			if !speakerOn(shotImage(t, root, "watch")) {
				t.Error("the desktop does not show the read-aloud button, on, with recorded voices")
			}
		}},
		// the tip button and the first amount are clicked where they are, not reached with
		// the keys: the show starts at a random beat, and a question's answers come first
		// among the controls the keys go through
		{"tip", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 60\nclick 1162 683\nwait 10\nclick 1090 538\nwait 20\nshot tip\nquit\n", nil, func(t *testing.T, root string) {
			// a tip of the first tier: a band of its color at the top of the stage, centred
			tip := shotImage(t, root, "tip")
			sw := 1280.0 - streamChatW
			x := int((sw - min(sw*0.8, 460)) / 2)
			if !near(tip, x+4, int(streamBarH+44+27), [3]uint32{0xe3, 0xf1, 0xff}) {
				t.Error("no band of the tip over the stage")
			}
		}},
		// the tips: the field of their message over the amounts, empty with its hint
		{"tipbox", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 60\nclick 1162 683\nwait 10\nshot tipbox\npress hide\nwait 20\nshot tipbox_full\nquit\n", nil, func(t *testing.T, root string) {
			page := [3]uint32{0xf4, 0xf4, 0xf6}
			for _, name := range []string{"tipbox", "tipbox_full"} {
				if img := shotImage(t, root, name); !near(img, 918, 488, page) || !near(img, 910, 488, white) {
					t.Errorf("%s: no field of the tip's message over the amounts", name)
				}
			}
		}},
		{"tipbox_tall", "0", "window 390 844\ngap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 100\nclick 272 807\nwait 10\nshot tipbox\nquit\n", nil, func(t *testing.T, root string) {
			if img := shotImage(t, root, "tipbox"); !near(img, 18, 612, [3]uint32{0xf4, 0xf4, 0xf6}) || !near(img, 10, 612, white) {
				t.Error("no field of the tip's message over the amounts on a phone held upright")
			}
		}},
		{"full", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 100\npress hide\nwait 20\nshot full\nquit\n", nil, func(t *testing.T, root string) {
			full := shotImage(t, root, "full")
			// the room under where the page's bar and the information were, and the chat darkened over the stage
			if !near(full, 60, 100, room) || !near(full, 60, 650, room) || !plate(full, 20, 695) || near(full, int(1280-streamChatW/2), 300, white) || !fullButton(full, int(1280-streamChatW-fullGap-fullSize/2), int(720-fullGap-fullSize/2), true) {
				t.Error("the full screen does not spread the stage under a darkened chat")
			}
			b, err := os.ReadFile(filepath.Join(root, "saves", "config.json")) //nolint:gosec // Test-owned save.
			if err != nil || !bytes.Contains(b, []byte(`"stream_full": true`)) {
				t.Errorf("the settings do not keep the full screen: %s, %v", b, err)
			}
		}},
		{"cards", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress right\nwait 20\nshot cards\nquit\n", nil, func(t *testing.T, root string) {
			// the second, small card scrolled into view has its show's line on a black band too
			cards := shotImage(t, root, "cards")
			w0 := 1280.0 - 2*lobbyEdge - lobbyChatW
			w := (1280 - 2*lobbyEdge - 3*lobbyGap) / 4
			h := w * 9 / 16
			y := streamBarH + 20 + w0*9/16 + 66
			y -= y + h + 60 - 720 // scrolled to the chosen card
			if dark := darkRun(cards, int(lobbyEdge+w/2), int(y+h-40), int(y+h-6)); dark < 8 {
				t.Errorf("the small card has no caption band (%d dark pixels)", dark)
			}
		}},
		{"duo", "1", "gap 1\nwait 60\npress ok\nwait 20\npress ok\nwait 100\nshot duo\nquit\n", nil, func(t *testing.T, root string) {
			duo := shotImage(t, root, "duo")
			sw := 1280.0 - streamChatW
			if !differs(duo, int(sw*0.28), 330) || !differs(duo, int(sw*0.72), 330) {
				t.Error("the duo does not stand both hosts on the stage")
			}
		}},
		{"tall", "0", tall, nil, func(t *testing.T, root string) {
			// a phone held upright: one card across the screen under the bar, then the next card under it
			lobby := shotImage(t, root, "lobby")
			if b := lobby.Bounds(); b.Dx() != 390 || b.Dy() != 844 {
				t.Fatalf("the lobby is %v, not the window's 390x844", b.Size())
			}
			w := 390 - 2*tallEdge
			h := w * 9 / 16
			top := barH + 16
			if !near(lobby, 195, 6, white) || !near(lobby, int(tallEdge+20), int(top+60), room) || !differs(lobby, 195, int(top+h/2)) || !near(lobby, int(tallEdge+20), int(top+h+tallNames+60), room) {
				t.Error("the portrait lobby does not stand the cards one under another across the screen")
			}
			if y1 := top + h + tallNames; darkRun(lobby, 195, int(y1+h-40), int(y1+h-6)) < 8 {
				t.Error("the second portrait card has no caption band")
			}
			// the languages in the bar, the one shown filled: English, then Japanese once its
			// button is tapped
			ink := [3]uint32{0x11, 0x11, 0x14}
			filled := func(img image.Image, b barButton) bool { return near(img, int(b.x+5), int(b.y+15), ink) }
			lobbyJa := shotImage(t, root, "lobby_ja")
			if !filled(lobby, button(bar, isEn)) || filled(lobby, ja) || filled(lobbyJa, button(barJa, isEn)) || !filled(lobbyJa, button(barJa, isJa)) {
				t.Error("the language buttons do not show the language shown, or a tap does not switch it")
			}
			// the settings: a page of the stream site under its bar, not a window over the lobby
			settings := shotImage(t, root, "settings")
			if !near(settings, 195, 6, white) || !near(settings, 4, 400, [3]uint32{0xf4, 0xf4, 0xf6}) {
				t.Error("the settings opened from the bar are not a page under the stream bar")
			}
			// the stage across the top, the chat column under it, and a caption band on the stage
			watch := shotImage(t, root, "watch")
			sth := 390.0 * 9 / 16
			if !differs(watch, 195, int(streamBarH+sth/2)) || !near(watch, 30, int(streamBarH+sth+tallInfoH+120), white) {
				t.Error("the portrait watch view has no puppet on its stage or no chat under it")
			}
			if darkRun(watch, 195, int(streamBarH+sth*0.6), int(streamBarH+sth)) < 10 {
				t.Error("no caption band on the portrait stage")
			}
			if bottom := int(streamBarH + sth); !fullButton(watch, int(390-fullGap-fullSize/2), int(float64(bottom)-fullGap-fullSize/2), false) || !plate(watch, 16, bottom-22) || darkRun(watch, 195, bottom-36, bottom-10) > 6 {
				t.Error("no name plate on the portrait stage, or the caption over it")
			}
			// full screen: the stage at the top, 16:9 as on the page (the puppets show the same
			// part of themselves), the darkened chat under it from the stage's bottom down
			full := shotImage(t, root, "full")
			band := func(y int) bool { return !near(full, 20, y, white) && !near(full, 20, y, room) }
			if under := full.At(20, int(sth+12)); full.At(20, int(sth-12)) == under || under != full.At(20, 400) || !band(int(sth+12)) {
				t.Error("the portrait full screen does not keep a 16:9 stage at the top over a darkened chat")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "game")
			fsys := streamsFS(t, tc.duo)
			if tc.edit != nil {
				tc.edit(fsys)
			}
			if err := os.CopyFS(game, fsys); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(root, "script.txt")
			if err := os.WriteFile(script, []byte(tc.script), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--start", "booth", "--mute", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")) //nolint:gosec // Fixed host and private fixture paths.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			tc.check(t, root)
		})
	}
}

// darkRun counts the dark pixels of the column x from y0 to y1: a caption's black band.
func darkRun(img image.Image, x, y0, y1 int) int {
	n := 0
	for y := y0; y < y1; y++ {
		if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 90 && g>>8 < 90 && b>>8 < 90 {
			n++
		}
	}
	return n
}

// tallBar is where the portrait lobby's bar puts its buttons (the English and the Japanese
// ones first, then the settings'), before and after Japanese is shown, and the height of the
// bar.
func tallBar(t *testing.T) (bar, barJa []barButton, barH float64) {
	t.Helper()
	g := streamsGame(t, "0")
	defer func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	}()
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.fitView(390, 844, 1)
	bar = g.barButtons(l.entries(g))
	g.setLanguage("ja")
	return bar, g.barButtons(l.entries(g)), barHeight(bar)
}
