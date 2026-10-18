package api_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/dev/apilist"
)

func TestPublicIdentifiers(t *testing.T) {
	t.Parallel()
	got, err := apilist.Snapshot(os.DirFS(".."), "github.com/nao1215/gozen")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("public.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("public identifiers changed; explain the API change, run go run ./internal/dev/apigen from the module root, and review api/public.txt\ncurrent inventory:\n%s", got)
	}
}
