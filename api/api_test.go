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
	public, schema := apilist.Split(got, "github.com/nao1215/gozen")
	for _, f := range []struct{ name, got string }{{"public.txt", public}, {"schema.txt", schema}} {
		want, err := os.ReadFile(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if f.got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Fatalf("public identifiers changed; explain the API change, run go run ./internal/dev/apigen from the module root, and review api/%s\ncurrent inventory:\n%s", f.name, f.got)
		}
	}
}
