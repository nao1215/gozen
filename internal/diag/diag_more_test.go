package diag

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRegisterRefusesMissingTextAndDuplicates(t *testing.T) {
	full := Entry{Summary: "s", Detail: "d.", Fix: "f."}
	existing := All()[0]
	for name, tc := range map[string]struct {
		call func()
		want string
	}{
		"no summary": {func() { register(1198, "X", Entry{Detail: "d.", Fix: "f."}) }, "has no summary"},
		"no detail":  {func() { register(1198, "X", Entry{Summary: "s", Fix: "f."}) }, "has no detail"},
		"duplicate":  {func() { register(int(existing.Code), "Copy", full) }, "already registered as " + existing.Name},
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

func TestCodesFindsRegisteredCodesOnceInOrder(t *testing.T) {
	all := All()
	a, b := all[0].Code, all[len(all)-1].Code
	text := "first " + b.String() + " then " + a.String() + " and again " + b.String() + " but GZN9999 is unknown and xGZN" + "0001 is glued"
	got := Codes(text)
	if len(got) != 2 || got[0] != b || got[1] != a {
		t.Errorf("%v (want [%s %s])", got, b, a)
	}
	if len(Codes("nothing here")) != 0 {
		t.Error("codes in plain text")
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
