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

// TestGraphicsScreensStreams draws the stream lobby, the watch view and a duo: the cards
// and the stage show the puppets over their rooms, the bar and the chat column are the
// light page, and a line stands on a black caption band.
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
		{"single", "0", "gap 1\nwait 60\nshot lobby\npress ok\nwait 20\ntype Pat\npress ok\nwait 100\nshot watch\nquit\n", func(t *testing.T, root string) {
			lobby := shotImage(t, root, "lobby")
			// the bar, the side list, the first card's picture (its puppet over the room) and its chat
			x, y, w, h := 284.0, streamBarH+20, 1280-260-48-lobbyChatW, (1280-260-48-lobbyChatW)*9/16
			if !near(lobby, 640, 6, white) || !near(lobby, 100, 690, white) || !differs(lobby, int(x+w/2), int(y+h/2)) || !near(lobby, int(x+w+lobbyChatW/2), int(y+4), white) {
				t.Error("the lobby is not drawn as the bar, the list and a card with its puppet and chat")
			}
			watch := shotImage(t, root, "watch")
			sw := 1280.0 - streamChatW
			if !differs(watch, int(sw/2), 300) || !near(watch, int(sw+streamChatW/2), 300, white) {
				t.Error("the watch view has no puppet on its stage or no chat column")
			}
			black := 0
			for yy := 420; yy < int(streamBarH+sw*9/16); yy++ {
				if r, g, b, _ := watch.At(int(sw/2), yy).RGBA(); r>>8 < 90 && g>>8 < 90 && b>>8 < 90 {
					black++
				}
			}
			if black < 10 {
				t.Error("no caption band under the line")
			}
		}},
		{"duo", "1", "gap 1\nwait 60\npress ok\nwait 20\npress ok\nwait 100\nshot duo\nquit\n", func(t *testing.T, root string) {
			duo := shotImage(t, root, "duo")
			sw := 1280.0 - streamChatW
			if !differs(duo, int(sw*0.28), 330) || !differs(duo, int(sw*0.72), 330) {
				t.Error("the duo does not stand both hosts on the stage")
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
