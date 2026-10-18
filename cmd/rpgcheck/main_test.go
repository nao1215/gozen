package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("failed output") }
func TestContract(t *testing.T) {
	for _, tt := range []struct {
		args   []string
		code   int
		result string
	}{
		{[]string{"--help"}, 0, "Requirements:"}, {nil, 2, ""}, {[]string{"--unknown"}, 2, ""}, {[]string{"missing"}, 1, ""},
		{[]string{"../../e2e/atago/fixtures/game"}, 0, "Structure checked"},
	} {
		var out, stderr bytes.Buffer
		if got := run(tt.args, &out, &stderr); got != tt.code {
			t.Fatalf("%v: exit %d; %s", tt.args, got, stderr.String())
		}
		if !strings.Contains(out.String(), tt.result) {
			t.Fatal(out.String())
		}
		if tt.code == 0 && stderr.Len() != 0 {
			t.Fatal(stderr.String())
		}
		if tt.code != 0 && out.Len() != 0 {
			t.Fatal("error wrote stdout")
		}
	}
	if got := run([]string{"--help"}, failWriter{}, io.Discard); got != 1 {
		t.Fatal(got)
	}
	if got := run([]string{"../../e2e/atago/fixtures/game"}, failWriter{}, io.Discard); got != 1 {
		t.Fatal(got)
	}
}
