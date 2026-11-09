package checkdata

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/base/diag"
)

func TestComplexityDiagnostics(t *testing.T) {
	fsys := game(t, map[string]string{"check-complexity.yaml": "file_lines: 1\nexpression_length: 1\n"})
	var out, warnings bytes.Buffer
	db, err := Load(fsys, &out, &warnings)
	if err != nil || db == nil {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(warnings.String(), "[GZN0122]") || !strings.Contains(warnings.String(), "game.yaml:1:1") || !strings.Contains(warnings.String(), "expression characters") {
		t.Fatal(warnings.String())
	}
	_, diagnostics := Diagnose(fsys)
	found := 0
	for _, d := range diagnostics {
		if d.Code == diag.YAMLComplexity.String() {
			found++
			if d.File == "" || d.Line == 0 || d.Severity != "warning" {
				t.Fatalf("%+v", d)
			}
		}
	}
	if found == 0 {
		t.Fatal("missing structured complexity warnings")
	}
	for _, config := range []string{"file_linse: 1", "file_lines: wrong", "file_lines: [1]"} {
		fsys["check-complexity.yaml"] = &fstest.MapFile{Data: []byte(config)}
		out.Reset()
		if _, err := Load(fsys, &out, io.Discard); err == nil || out.Len() != 0 {
			t.Fatalf("bad config %q: %v, %s", config, err, &out)
		}
		if db, ds := Diagnose(fsys); db != nil || len(ds) == 0 || ds[0].Severity != "error" {
			t.Fatalf("bad config accepted: %+v", ds)
		}
	}
}

type deniedComplexityFS struct{ fs.FS }

func (f deniedComplexityFS) Open(name string) (fs.File, error) {
	if name == "check-complexity.yaml" {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestComplexityReadFailure(t *testing.T) {
	fsys := game(t, nil)
	if _, err := Load(deniedComplexityFS{fsys}, io.Discard, io.Discard); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("%v", err)
	}
}
