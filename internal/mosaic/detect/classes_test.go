package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseNamesMetadata(t *testing.T) {
	got, err := ParseNamesMetadata("{1: 'b', 0: 'a', 2: \"c d\"}")
	if err != nil || !slices.Equal(got, []string{"a", "b", "c d"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "{}", "{0: 'a', 2: 'b'}", "{0: 'a', 0: 'b'}", "{0: 'a', 1: 'a'}"} {
		if _, err := ParseNamesMetadata(bad); err == nil {
			t.Fatalf("%q must fail", bad)
		}
	}
}

func TestParseLabelsJSON(t *testing.T) {
	got, err := ParseLabelsJSON([]byte(`["nipple_f","penis","pussy"]`))
	if err != nil || !slices.Equal(got, Labels) {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{`{}`, `[]`, `["a",""]`, `["a","a"]`, `[1]`} {
		if _, err := ParseLabelsJSON([]byte(bad)); err == nil {
			t.Fatalf("%s must fail", bad)
		}
	}
}

func TestResolveClasses(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, src, err := ResolveClasses(model, "", Labels)
	if err != nil || src != SourceDefault || !slices.Equal(names, Labels) {
		t.Fatalf("default: %v %s %v", names, src, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.json"), []byte(`["face"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	names, src, err = ResolveClasses(model, "", Labels)
	if err != nil || src != SourceJSON || !slices.Equal(names, []string{"face"}) {
		t.Fatalf("json: %v %s %v", names, src, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.json"), []byte(`["a","a"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ResolveClasses(model, "", Labels); err == nil {
		t.Fatal("an invalid labels.json must be an error")
	}
}
