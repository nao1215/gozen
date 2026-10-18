package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunContracts(t *testing.T) {
	catalog := filepath.Join("..", "..", "docs", "examples", "production", "catalog.yaml")
	for _, tc := range []struct {
		args     []string
		fail     bool
		contains string
	}{
		{[]string{"--help"}, false, "Requirements"},
		{[]string{"lint", "--config", "missing.yaml", "--help"}, false, "Requirements"},
		{[]string{"--config", catalog, "--scene", "landscape", "--format", "json"}, false, "quiet garden"},
		{[]string{"lint", "--config", catalog, "--format", "json"}, false, "[]"},
		{[]string{"--config", catalog, "--format", "bad"}, true, ""},
		{[]string{"--config", catalog, "--plan", "missing.yaml", "--scene", "landscape"}, true, ""},
		{[]string{"--config", "missing.yaml"}, true, ""},
		{[]string{"--config", catalog, "--subject", "unknown"}, true, ""},
		{[]string{"--config", catalog, "free text"}, true, ""},
	} {
		var out, diag bytes.Buffer
		err := run(tc.args, &out, &diag)
		if (err != nil) != tc.fail {
			t.Errorf("%v: %v %s", tc.args, err, &diag)
		}
		if !strings.Contains(out.String(), tc.contains) {
			t.Errorf("%v: output %q", tc.args, &out)
		}
	}
	if err := run([]string{"--config", catalog}, badWriter{}, io.Discard); err == nil {
		t.Fatal("lost output failure")
	}
}

type badWriter struct{}

func (badWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
