package main

import (
	"bytes"
	"testing"
)

func TestExpeditionRoundTrip(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "location=trail battles=1 wins=1 gold=3\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
