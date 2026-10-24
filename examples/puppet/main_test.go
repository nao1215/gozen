package main

import (
	"bytes"
	"errors"
	"io/fs"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer5/puppet"
)

func TestHelpAndOptions(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"puppet - ", "[Usage]\n", "[Examples]\n", "[Options]\n", "-h, --help", "Requirements:", "GitHub Sponsors:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help lacks %q", want)
		}
	}
	o, err := parse([]string{"--window=0.5", "--seed", "9", "--tour", "--frames", "f"})
	if err != nil || o.window != 0.5 || o.seed != 9 || !o.tour || o.frames != "f" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"-seed", "1"}, {"--window", "0"}, {"--seed"}, {"--frames", "f"}, {"extra"}, {"--colour"}} {
		var u usageError
		if err := run(args, &out); !errors.As(err, &u) {
			t.Errorf("%v: got %v, want a usage error", args, err)
		}
	}
}

// The embedded streamer loads with every face and reaction the tour and keys use.
func TestStreamerLoads(t *testing.T) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := puppet.Load(sub, "streamer/puppet.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if w, h := rig.Size(); h != 900 || w < 600 {
		t.Fatalf("size %dx%d", w, h)
	}
	p := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Deterministic animation timing, not security.
	for _, c := range tour() {
		if err := c.do(p); err != nil {
			t.Fatalf("%s: %v", c.label, err)
		}
	}
}
