package play

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
func TestHelpAndErrorsDoNotExit(t *testing.T) {
	var out bytes.Buffer
	if err := Main(context.Background(), nil, "invalid", []string{"--help"}, Config{Output: &out}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Requirements:", "[Usage]", "[Examples]", "[Options]", "  Test play (--dev):", "-h, --help",
		"Report an issue: https://github.com/nao1215/gozen/issues",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help lacks %q", want)
		}
	}
	if strings.Contains(out.String(), "  Other:") {
		t.Fatal("an option is missing from the help groups")
	}
	var short bytes.Buffer
	if err := Main(context.Background(), nil, "invalid", []string{"-h"}, Config{Output: &short}); err != nil || short.String() != out.String() {
		t.Fatalf("-h differs from --help: %v", err)
	}
	want := errors.New("output unavailable")
	if err := Main(context.Background(), nil, ".", []string{"--help"}, Config{Output: failingWriter{want}}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-dev"}, {"--dev", "-seed", "7"}, {"-mute"}} {
		err := Main(context.Background(), nil, ".", args, Config{})
		if err == nil || !strings.Contains(err.Error(), "single-dash long option") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	// Both GNU value forms reach the same check after parsing.
	for _, args := range [][]string{{"--seed=7", "--start=camp"}, {"--seed", "7", "--start", "camp"}} {
		err := Main(context.Background(), nil, ".", args, Config{})
		if err == nil || err.Error() != "--start requires --dev" {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"--unknown"}, {"--start", "camp"}, {"--dev", "--script", "missing"}, {"extra"}} {
		if err := Main(context.Background(), nil, ".", args, Config{}); err == nil {
			t.Fatal(args)
		}
	}
}

func TestEmbeddedRootAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Main(ctx, nil, ".", nil, Config{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
