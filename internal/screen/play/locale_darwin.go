//go:build darwin

package play

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// DeviceLocales are the languages of the locale variables (LC_ALL, LC_MESSAGES, LANG),
// then the system's languages (AppleLanguages): an app opened from the Finder has no
// locale variables.
func DeviceLocales(ctx context.Context) []string {
	out := envLocales(os.Getenv)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if b, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		out = append(out, appleLanguages(string(b))...)
	}
	return out
}
