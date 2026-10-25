package diag

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCodeForms(t *testing.T) {
	t.Parallel()
	c := Code(1101)
	if c.String() != "GZN1101" || c.ExitCode() != 1 || Code(101).String() != "GZN0101" || Code(101).ExitCode() != 0 {
		t.Fatal(c.String(), c.ExitCode())
	}
	if got := c.Mark("data/a.yaml:1:2: x: bad"); got != "data/a.yaml:1:2: x: bad [GZN1101]" {
		t.Fatal(got)
	}
	if Code(0).Mark("m") != "m" || Code(0).String() != "" {
		t.Fatal("the zero code must not mark")
	}
	base := errors.New("base")
	err := Code(2102).Errorf("wrap: %w", base)
	if err.Error() != "wrap: base [GZN2102]" || !errors.Is(err, base) || Of(fmt.Errorf("outer: %w", err)) != 2102 {
		t.Fatal(err)
	}
	if Of(base) != 0 || Code(1).Wrap(nil) != nil {
		t.Fatal("no code")
	}
	for _, s := range []string{"GZN1101", "gzn1101", "GZN0001"} {
		if !Syntax(s) {
			t.Errorf("%s has the form of a code", s)
		}
	}
	for _, s := range []string{"GZN11011", "GZN11a1", "ATG1101", "", "GZN"} {
		if Syntax(s) {
			t.Errorf("%s does not have the form of a code", s)
		}
	}
	if _, ok := Parse("GZN9999"); ok {
		t.Fatal("an unregistered code parsed")
	}
	if n, ok := Number("GZN9999"); !ok || n != 9999 {
		t.Fatal(n, ok)
	}
}

func TestRegisteredEntries(t *testing.T) {
	t.Parallel()
	for _, e := range All() {
		if got, ok := Parse(e.Code.String()); !ok || got != e.Code {
			t.Errorf("%s does not parse back", e.Code)
		}
		if got, ok := Parse(strings.ToLower(e.Code.String())); !ok || got != e.Code {
			t.Errorf("%s does not parse in lower case", e.Code)
		}
		if family(e.Code) == nil || e.Code.ExitCode() != family(e.Code).Exit {
			t.Errorf("%s is outside its family", e.Code)
		}
		if strings.HasSuffix(e.Summary, ".") || e.Summary != strings.ToLower(e.Summary[:1])+e.Summary[1:] {
			t.Errorf("%s: the summary is a lower-case phrase without a period: %q", e.Code, e.Summary)
		}
		for _, s := range []string{e.Detail, e.Fix} {
			if !strings.HasSuffix(s, ".") {
				t.Errorf("%s: detail and fix are sentences: %q", e.Code, s)
			}
		}
		text := Text(e)
		if !strings.HasPrefix(text, e.Code.String()+" - ") || !strings.Contains(text, URL) {
			t.Errorf("%s: %q", e.Code, text)
		}
		for line := range strings.SplitSeq(text, "\n") {
			if len(line) > 80 && !strings.HasPrefix(line, "http") {
				t.Errorf("%s: line over 80 columns: %q", e.Code, line)
			}
		}
		if len(Near(e.Code)) == 0 {
			t.Errorf("%s has no neighbours", e.Code)
		}
	}
}

func TestRegisterRefusesIncompleteEntries(t *testing.T) {
	full := Entry{Summary: "s", Detail: "d.", Fix: "f."}
	for name, call := range map[string]func(){
		"range":    func() { register(3001, "X", full) },
		"family":   func() { register(1100, "X", full) },
		"no name":  func() { register(1198, "", full) },
		"no fix":   func() { register(1198, "X", Entry{Summary: "s", Detail: "d."}) },
		"no group": func() { register(1001, "X", full) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: register accepted it", name)
				}
			}()
			call()
		}()
	}
}
