//go:build !linux

package play

import "context"

// Other platforms use the in-window composer.
func externalEntry(_ context.Context, _, _ string, _ chan<- extInput) bool { return false }
