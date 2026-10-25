package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

// TestReferencePageIsCurrent keeps website/content/errors.md equal to the registry.
func TestReferencePageIsCurrent(t *testing.T) {
	t.Parallel()
	p := "../../../website/content/errors.md"
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(got), "\r\n", "\n") != string(markdown()) {
		t.Fatal("website/content/errors.md is out of date; run go run ./internal/dev/errgen from the module root")
	}
	for _, e := range diag.All() {
		if !strings.Contains(string(got), "### "+e.Code.String()+"\n") {
			t.Errorf("%s is missing from the page", e.Code)
		}
	}
}
