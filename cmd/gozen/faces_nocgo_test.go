//go:build !cgo

package main

import (
	"strings"
	"testing"
)

// TestFacesWithoutCgo: a gozen built without cgo has no ONNX inference; faces says so.
func TestFacesWithoutCgo(t *testing.T) {
	_, stderr := invoke(t, 1, "faces", scenesGame(t), "--model", t.TempDir()+"/model.onnx")
	if !strings.Contains(stderr, "CGO_ENABLED=0") {
		t.Fatalf("the reason: %s", stderr)
	}
}
