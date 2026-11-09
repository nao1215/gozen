package decodeguard

import (
	"errors"
	"testing"
)

func TestRun(t *testing.T) {
	want := errors.New("bad")
	if err := Run(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("error not returned: %v", err)
	}
	if err := Run(func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Run(func() error { panic("nil map") }); err == nil {
		t.Fatal("a panic was not turned into an error")
	}
}
