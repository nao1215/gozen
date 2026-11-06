package diag

import (
	"errors"
	"fmt"
	"io/fs"
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
		if (len(e.Example) == 0) == (strings.TrimSpace(e.NotYAML) == "") {
			t.Errorf("%s needs exactly one reproduction or non-YAML reason", e.Code)
		}
		names := map[string]bool{}
		for _, file := range e.Example {
			if !fs.ValidPath(file.Name) || names[file.Name] {
				t.Errorf("%s: invalid or duplicate file %q", e.Code, file.Name)
			}
			names[file.Name] = true
			if file.Encoding != "" && file.Encoding != "base64" {
				t.Errorf("%s: unknown encoding %q", e.Code, file.Encoding)
			}
		}
		if len(e.Example) > 0 && !names["game.yaml"] {
			t.Errorf("%s: missing game.yaml", e.Code)
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

func TestExamplesBelongToRegisteredCodes(t *testing.T) {
	t.Parallel()
	for number := range examples {
		if _, ok := Lookup(Code(number)); !ok {
			t.Errorf("example for unregistered code %d", number)
		}
	}
}

func TestRegisterRefusesIncompleteEntries(t *testing.T) {
	full := Entry{Summary: "s", Detail: "d.", Fix: "f."}
	for name, call := range map[string]func(){
		"range":           func() { register(3001, "X", full) },
		"family":          func() { register(1100, "X", full) },
		"no name":         func() { register(1198, "", full) },
		"no group":        func() { register(1001, "X", full) },
		"no reproduction": func() { register(1198, "X", full) },
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

func TestRegisterRefusesMissingTextAndDuplicates(t *testing.T) {
	full := Entry{Summary: "s", Detail: "d.", Fix: "f."}
	existing := All()[0]
	for name, tc := range map[string]struct {
		call func()
		want string
	}{
		"no summary": {func() { register(1198, "X", Entry{Detail: "d.", Fix: "f."}) }, "has no summary"},
		"no detail":  {func() { register(1198, "X", Entry{Summary: "s", Fix: "f."}) }, "has no detail"},
		"no fix": {func() {
			register(1198, "X", Entry{Summary: "s", Detail: "d.", NotYAML: "Requires a command-line argument."})
		}, "has no fix"},
		"duplicate": {func() { register(int(existing.Code), "Copy", full) }, "already registered as " + existing.Name},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Errorf("%s: panic %v, want %q", name, r, tc.want)
				}
			}()
			tc.call()
		}()
	}
}

func TestParseAndNumberTolerateSpacesAndRejectJunk(t *testing.T) {
	e := All()[0]
	if c, ok := Parse("  " + strings.ToLower(e.Code.String()) + "\n"); !ok || c != e.Code {
		t.Errorf("Parse with spaces: %v %v", c, ok)
	}
	if _, ok := Parse("GZN12"); ok {
		t.Error("a short code parsed")
	}
	if _, ok := Parse("1101"); ok {
		t.Error("digits without GZN parsed")
	}
	if n, ok := Number(" GZN0042 "); !ok || n != 42 {
		t.Errorf("Number with spaces: %v %v", n, ok)
	}
	if _, ok := Number("GZN00x2"); ok {
		t.Error("a code with a letter in it")
	}
}

func TestNearListsTheFamilyOfACodeInOrder(t *testing.T) {
	e := All()[0]
	near := Near(e.Code)
	if len(near) == 0 {
		t.Fatalf("%s has no family", e.Code)
	}
	for i, n := range near {
		if int(n.Code)/100 != int(e.Code)/100 || i > 0 && near[i-1].Code >= n.Code {
			t.Errorf("%s is not in the family of %s in order", n.Code, e.Code)
		}
	}
	if got := Near(Code(9999)); len(got) != 0 {
		t.Errorf("an unknown family: %v", got)
	}
}

func TestErrorWrappingKeepsTheCause(t *testing.T) {
	cause := errors.New("disk on fire")
	wrapped := Code(1501).Wrap(cause)
	if !errors.Is(wrapped, cause) || Of(wrapped) != 1501 || !strings.HasSuffix(wrapped.Error(), "[GZN1501]") {
		t.Errorf("%v", wrapped)
	}
	// The outermost code names the error.
	if got := Of(Code(1502).Wrap(wrapped)); got != 1502 {
		t.Errorf("the outermost code names the error: %v", got)
	}
}

func TestFamiliesNameTheirRangesAndExitStatus(t *testing.T) {
	fams := Families()
	if len(fams) == 0 {
		t.Fatal("no families")
	}
	seen := map[string]bool{}
	for _, f := range fams {
		if !strings.HasPrefix(f.Prefix, "GZN") || !strings.HasSuffix(f.Prefix, "xx") || len(f.Prefix) != 7 || f.Meaning == "" || f.Exit < 0 || f.Exit > 2 || seen[f.Prefix] {
			t.Errorf("%+v", f)
		}
		seen[f.Prefix] = true
		// Warnings (GZN01xx, GZN02xx) never fail a command; the 2xxx range is a usage error.
		if (f.Prefix[3] == '0') != (f.Exit == 0) || (f.Prefix[3] == '2') != (f.Exit == 2) {
			t.Errorf("%s exits with %d", f.Prefix, f.Exit)
		}
	}
}
