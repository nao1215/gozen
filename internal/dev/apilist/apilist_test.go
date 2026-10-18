package apilist

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestSnapshot(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{}
	for name, source := range map[string]string{
		"go.mod": "module example.org/game\n",
		"model/types.go": `package model
const First, Second = 1, 2
var Shared, hidden int
type State[T any] struct { Name string; private int; *Embedded }
type Embedded struct{}
type Port interface { Run(); hidden(); EmbeddedPort }
type EmbeddedPort interface{}
type Alias = State[int]
func Load() {}
func (*State[T]) Save() {}
type local struct{ Visible int }
func (local) Invisible() {}
func hiddenFunc() {}
`,
		"model/system_windows.go":  "//go:build windows\n\npackage model\nfunc WindowsOnly() {}\n",
		"model/system_linux.go":    "package model\nfunc Load() {}\n",
		"model/types_test.go":      "not Go source",
		"model/internal/secret.go": "not Go source",
		"internal/private.go":      "not Go source",
		"cmd/sample/main.go":       "not Go source",
		"examples/sample/main.go":  "not Go source",
		"testdata/broken.go":       "not Go source",
		"vendor/dependency/lib.go": "not Go source",
		".cache/temp.go":           "not Go source",
		"_scratch/temp.go":         "not Go source",
		"tools/go.mod":             "module example.org/tools\n",
		"tools/public.go":          "not Go source",
		"program/main.go":          "package main\nfunc Main() {}",
		"x/demo/demo.go":           "package demo\nfunc Try() {}",
	} {
		files[name] = &fstest.MapFile{Data: []byte(source)}
	}
	got, err := Snapshot(files, "example.org/game")
	if err != nil {
		t.Fatal(err)
	}
	want := `# Declared public identifiers; all OS/build-tag variants; see docs/development/api.md.
example.org/game/model const First
example.org/game/model const Second
example.org/game/model field Alias.Name
example.org/game/model field State.Name
example.org/game/model field local.Visible
example.org/game/model field-embed Alias.Embedded
example.org/game/model field-embed State.Embedded
example.org/game/model func Load
example.org/game/model func WindowsOnly
example.org/game/model interface Port.Run
example.org/game/model interface-embed Port.EmbeddedPort
example.org/game/model method Alias.Save
example.org/game/model method State.Save
example.org/game/model method local.Invisible
example.org/game/model package model
example.org/game/model type Alias
example.org/game/model type Embedded
example.org/game/model type EmbeddedPort
example.org/game/model type Port
example.org/game/model type State
example.org/game/model var Shared
example.org/game/x/demo func Try
example.org/game/x/demo package demo
`
	if got != want {
		t.Fatalf("inventory mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSnapshotRejectsInvalidSource(t *testing.T) {
	t.Parallel()
	_, err := Snapshot(fstest.MapFS{"model/broken.go": {Data: []byte("package model\nfunc")}}, "example.org/game")
	if err == nil || !strings.Contains(err.Error(), "model/broken.go") {
		t.Fatalf("expected a parse error identifying the source file, got %v", err)
	}
}

func TestInternalAliasContract(t *testing.T) {
	files := fstest.MapFS{
		"public/types.go": {Data: []byte(`package public
import shared "example.org/game/internal/model"
type State = shared.State
`)},
		"internal/model/state.go": {Data: []byte(`package model
type State struct{ Gold int; hidden int }
func (*State) Save() {}
`)},
	}
	got, err := Snapshot(files, "example.org/game")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"public field State.Gold", "public method State.Save"} {
		if !strings.Contains(got, want) {
			t.Fatalf("alias hides %s: %s", want, got)
		}
	}
	if strings.Contains(got, "internal/model") || strings.Contains(got, "hidden") {
		t.Fatal(got)
	}
	files["internal/model/state.go"].Data = []byte(`package model
type State struct{ Gold int; NewContract string }
func (*State) Save() {}
`)
	changed, err := Snapshot(files, "example.org/game")
	if err != nil {
		t.Fatal(err)
	}
	if got == changed || !strings.Contains(changed, "State.NewContract") {
		t.Fatal("internal alias API growth went undetected")
	}
}
