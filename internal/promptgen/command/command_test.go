package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunContracts(t *testing.T) {
	catalog := filepath.Join("..", "..", "..", "docs", "examples", "production", "catalog.yaml")
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

func TestBatchScenes(t *testing.T) {
	catalog := filepath.Join("..", "..", "..", "docs", "examples", "production", "catalog.yaml")
	ids := filepath.Join(t.TempDir(), "scenes.txt")
	if err := os.WriteFile(ids, []byte("landscape\nmissing\n\nlandscape\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config", catalog, "--scenes", ids, "--format", "json"}
	var out bytes.Buffer
	if err := run(args, &out, io.Discard); err == nil {
		t.Fatal("missing scene must fail the batch")
	}
	dec := json.NewDecoder(&out)
	for i, id := range []string{"landscape", "missing", "landscape"} {
		var row struct {
			ID     string                    `json:"id"`
			Result struct{ Positive string } `json:"result"`
			Error  string                    `json:"error"`
		}
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.ID != id || (row.Error != "") != (i == 1) || (i != 1 && !strings.Contains(row.Result.Positive, "quiet garden")) {
			t.Fatalf("batch row %d: %+v", i, row)
		}
	}
	if err := run(args, badWriter{}, io.Discard); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("lost batch output error: %v", err)
	}
	if err := os.WriteFile(ids, []byte("landscape\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(args, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--scene", "landscape"}, {"--subject", "guide"}, {"--plan", "plan.yaml"}, {"--format", "text"}} {
		if err := run(append(append([]string{}, args...), extra...), io.Discard, io.Discard); err == nil {
			t.Fatalf("accepted incompatible batch arguments %v", extra)
		}
	}
}
