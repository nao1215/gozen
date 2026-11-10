package decodeguard

import (
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

func TestRun(t *testing.T) {
	want := errors.New("bad")
	if err := Run(nil, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("error not returned: %v", err)
	}
	if err := Run(nil, func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Run(nil, func() error { panic("nil map") }); err == nil {
		t.Fatal("a panic was not turned into an error")
	}
}

// TestRunRefusesTaggedScalarsBeforeDecoding: the decoder dereferences nil on a
// tagged scalar under a list. Recovering that fault corrupted the heap on Windows
// (golang/go#81238), so decode must not be called at all, and the error names the
// place. Every input the guard lets through decodes into lists without a panic.
func TestRunRefusesTaggedScalarsBeforeDecoding(t *testing.T) {
	type target struct {
		A []int          `yaml:"a"`
		B [2]int         `yaml:"b"`
		M map[string]int `yaml:"m"`
		X any            `yaml:"x"`
	}
	decodeInto := func(src string) (called bool, err error) {
		err = Run([]byte(src), func() error {
			called = true
			var v target
			return yaml.Unmarshal([]byte(src), &v)
		})
		return called, err
	}
	for _, src := range []string{
		"a: !x 0", "b: !x 0", "a: !!str 0", "a: !x &k 0", "x: 1\na: !x \"s\"",
		"a: !x |\n  text", "m: {k: !x 0}", "a: [!x 0]", "x: !!binary aGk=", "--- 1\n---\na: !x 0",
	} {
		called, err := decodeInto(src)
		if called || err == nil {
			t.Errorf("%q: decode called %v, error %v; want refused before decoding", src, called, err)
			continue
		}
		var syntax *yaml.SyntaxError
		if !errors.As(err, &syntax) || syntax.Token == nil || !strings.Contains(err.Error(), "tag") {
			t.Errorf("%q: the refusal must be a positioned syntax error: %v", src, err)
		}
	}
	for _, src := range []string{
		"a: [1, 2]", "a: !x [1]", "a: !x\n- 1", "m: !x {k: 1}", "a: !x", "x: \"wow!\"", "x: !unfinished [",
	} {
		called, err := decodeInto(src)
		if !called {
			t.Errorf("%q: refused (%v); only a tagged scalar is refused", src, err)
		}
		if err != nil && strings.Contains(err.Error(), "nil pointer") {
			t.Errorf("%q: the decoder still dereferenced nil: %v", src, err)
		}
	}
}

func TestRunNode(t *testing.T) {
	file, err := parser.ParseBytes([]byte("a: !x 0"), 0)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := RunNode(file.Docs[0].Body, func() error { called = true; return nil }); err == nil || called {
		t.Fatalf("a tagged scalar node was decoded: called %v, error %v", called, err)
	}
	if err := RunNode(nil, func() error { panic("x") }); err == nil {
		t.Fatal("a panic was not turned into an error")
	}
}
