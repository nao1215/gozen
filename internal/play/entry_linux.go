//go:build linux

package play

import (
	"context"
	"os/exec"
	"strings"
)

// externalEntry uses a separate process only when requested by the host.
func externalEntry(ctx context.Context, title, label string, ch chan<- extInput) bool {
	path, err := exec.LookPath("zenity")
	if err != nil {
		return false
	}
	go func() {
		//nolint:gosec // The executable comes from LookPath; arguments are separate and no shell is used.
		out, err := exec.CommandContext(ctx, path, "--entry", "--title", title, "--text", label, "--width", "640").Output()
		ch <- extInput{text: strings.TrimRight(string(out), "\n"), ok: err == nil}
	}()
	return true
}
