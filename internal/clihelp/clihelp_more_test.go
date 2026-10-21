package clihelp

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

func TestParseErrorsCarryTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code diag.Code
	}{
		{"unknown long option", []string{"--zzz"}, diag.UnknownOption},
		{"unknown short option", []string{"-z"}, diag.UnknownOption},
		{"a value that is no number", []string{"--count", "many"}, diag.BadValue},
		{"a missing value", []string{"--count"}, diag.BadValue},
		{"a single-dash long option", []string{"-count", "2"}, diag.SingleDashOption},
	} {
		fs := NewFlagSet("t")
		fs.Int("count", 1, "how many")
		_, err := Parse(fs, tc.args)
		if err == nil || diag.Of(err) != tc.code {
			t.Errorf("%s: %v (want %s)", tc.name, err, tc.code)
		}
	}
}

func TestHelpRequestedStopsAtTheTerminator(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"a", "b"}, false},
		{[]string{"-h"}, true},
		{[]string{"x", "--help"}, true},
		{[]string{"--", "--help"}, false},
		{[]string{"-n", "--", "-h"}, false},
		{[]string{"--help", "--"}, true},
	} {
		if got := HelpRequested(tc.args); got != tc.want {
			t.Errorf("HelpRequested(%q) = %v", tc.args, got)
		}
	}
}

func TestVersionComesFromTheReleaseBuild(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "v9.8.7"
	if got := Version(); got != "v9.8.7" {
		t.Errorf("the release build's version wins: %q", got)
	}
	version = ""
	if got := Version(); got == "" {
		t.Error("a build without a version still names itself")
	}
}

// A flag no group lists is shown under Other, a hidden one is not, and the label shows the
// kind of value; a group that names a flag the set lacks skips it.
func TestOptionsLabelsHiddenFlagsAndMissingNames(t *testing.T) {
	fs := NewFlagSet("demo")
	fs.Float64("ratio", 0.5, "the ratio")
	fs.Duration("wait", 0, "how long")
	fs.Uint64("seed", 7, "the seed")
	fs.StringArray("item", nil, "an item")
	fs.String("kind", "", "the `KIND` of it")
	fs.Bool("secret", false, "not for the page")
	if err := fs.MarkHidden("secret"); err != nil {
		t.Fatal(err)
	}
	p := Page{Name: "demo", Summary: "d", Usage: []string{"demo"}, Groups: []Group{
		{Title: "Real", Flags: []string{"ghost", "ratio"}, Extra: [][2]string{{"--dynamic X", "an option made at run time"}}},
	}}
	got := p.String(fs)
	for _, want := range []string{"--ratio NUM", "--wait DURATION", "--seed N", "--item VALUE", "--kind KIND", "--dynamic X", "an option made at run time", "  Other:\n", "  Real:\n", "(default: 7)", "(default: 0.5)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "ghost") {
		t.Errorf("a hidden flag and an absent flag are not listed:\n%s", got)
	}
	// With no General group Other comes last; a page without a flag set has no [Options].
	if strings.Index(got, "Real:") > strings.Index(got, "Other:") {
		t.Errorf("Other follows the groups:\n%s", got)
	}
	if bare := (Page{Name: "x", Summary: "y", Usage: []string{"x"}}).String(nil); strings.Contains(bare, "[Options]") {
		t.Errorf("no flag set, no options:\n%s", bare)
	}
}

func TestPageWrapsAndOrdersItsSections(t *testing.T) {
	fs := NewFlagSet("demo")
	p := Page{
		Name: "demo", Summary: "a demo", Usage: []string{"demo [OPTIONS]"},
		About:    "First paragraph " + strings.Repeat("word ", 40) + "\n\nSecond paragraph.",
		Commands: [][2]string{{"alpha", "does the first thing"}, {"b", "does the second thing, which is described with enough words to wrap onto another line of the page"}},
		Hint:     "Run demo <command> --help.",
		Notes:    []string{"A note."},
	}
	got := p.String(fs)
	order := []string{"demo - a demo (", "[Usage]", "First paragraph", "Second paragraph.", "[Commands]", "[Options]", "A note.", "Run demo <command> --help.", "Documentation:"}
	last := -1
	for _, s := range order {
		at := strings.Index(got, s)
		if at < 0 || at < last {
			t.Fatalf("%q is out of order or missing in\n%s", s, got)
		}
		last = at
	}
	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > Width {
			t.Errorf("line over %d columns: %q", Width, line)
		}
	}
	if !strings.Contains(got, "\n  alpha  does the first thing\n") || !strings.Contains(got, "\n  b      does the second thing") {
		t.Errorf("command names are padded to one column:\n%s", got)
	}
}

func TestWrapKeepsLongWordsWhole(t *testing.T) {
	long := strings.Repeat("x", 120)
	got := Wrap("a "+long+" b", 4)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 || lines[1] != "    "+long {
		t.Errorf("a word longer than the line sits on its own line: %q", lines)
	}
	if Wrap("", 2) != "  \n" {
		t.Errorf("an empty paragraph is an indent: %q", Wrap("", 2))
	}
}

func TestErrHelpIsAnError(t *testing.T) {
	if !errors.Is(ErrHelp, ErrHelp) || ErrHelp.Error() == "" {
		t.Fatal("ErrHelp")
	}
}
