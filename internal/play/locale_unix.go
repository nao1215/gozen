//go:build !js && !windows && !darwin

package play

import (
	"context"
	"os"
)

// DeviceLocales are the languages of the locale variables (LC_ALL, LC_MESSAGES, LANG).
func DeviceLocales(context.Context) []string { return envLocales(os.Getenv) }
