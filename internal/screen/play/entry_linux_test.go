//go:build linux

package play

import (
	"context"
	"testing"
)

func TestMissingExternalInputUsesWindow(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if externalEntry(context.Background(), "Name", "Enter a name", make(chan extInput, 1)) {
		t.Fatal("external input started without zenity")
	}
}
