//go:build graphics

package play

import (
	"bytes"
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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
	for _, tc := range []struct {
		name, duo, script string
		check             func(t *testing.T, root string)
	}{
		{"single", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\nshot lobby\npress ok\nwait 100\nshot watch\nquit\n", func(t *testing.T, root string) {
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
		}},
		{"full", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress ok\nwait 100\npress hide\nwait 20\nshot full\nquit\n", func(t *testing.T, root string) {
			full := shotImage(t, root, "full")
			// the room under where the page's bar and the information were, and the chat darkened over the stage
			if !near(full, 60, 100, room) || !near(full, 60, 700, room) || near(full, int(1280-streamChatW/2), 300, white) {
				t.Error("the full screen does not spread the stage under a darkened chat")
			}
			b, err := os.ReadFile(filepath.Join(root, "saves", "config.json")) //nolint:gosec // Test-owned save.
			if err != nil || !bytes.Contains(b, []byte(`"stream_full": true`)) {
				t.Errorf("the settings do not keep the full screen: %s, %v", b, err)
			}
		}},
		{"cards", "0", "gap 1\nwait 30\ntype Pat\npress ok\nwait 30\npress right\nwait 20\nshot cards\nquit\n", func(t *testing.T, root string) {
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
		{"duo", "1", "gap 1\nwait 60\npress ok\nwait 20\npress ok\nwait 100\nshot duo\nquit\n", func(t *testing.T, root string) {
			duo := shotImage(t, root, "duo")
			sw := 1280.0 - streamChatW
			if !differs(duo, int(sw*0.28), 330) || !differs(duo, int(sw*0.72), 330) {
				t.Error("the duo does not stand both hosts on the stage")
			}
		}},
		{"tall", "0", "window 390 844\ngap 1\nwait 30\ntype Pat\npress ok\nwait 30\nshot lobby\npress ok\nwait 100\nshot watch\npress hide\nwait 20\nshot full\nquit\n", func(t *testing.T, root string) {
			// a phone held upright: one card across the screen under the bar, then the next card under it
			lobby := shotImage(t, root, "lobby")
			if b := lobby.Bounds(); b.Dx() != 390 || b.Dy() != 844 {
				t.Fatalf("the lobby is %v, not the window's 390x844", b.Size())
			}
			w := 390 - 2*tallEdge
			h := w * 9 / 16
			top := streamBarH + 16
			if !near(lobby, 195, 6, white) || !near(lobby, int(tallEdge+20), int(top+60), room) || !differs(lobby, 195, int(top+h/2)) || !near(lobby, int(tallEdge+20), int(top+h+tallNames+60), room) {
				t.Error("the portrait lobby does not stand the cards one under another across the screen")
			}
			if y1 := top + h + tallNames; darkRun(lobby, 195, int(y1+h-40), int(y1+h-6)) < 8 {
				t.Error("the second portrait card has no caption band")
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
			// full screen: the room from the top to the bottom, the chat darkened over its lower part
			full := shotImage(t, root, "full")
			if !near(full, 20, 120, room) || near(full, 20, 700, room) || near(full, 20, 700, white) {
				t.Error("the portrait full screen does not fill the screen with the stage under a darkened chat")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "game")
			if err := os.CopyFS(game, streamsFS(t, tc.duo)); err != nil {
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
