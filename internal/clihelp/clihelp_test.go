package clihelp

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

func TestParseGNUForms(t *testing.T) {
	for _, args := range [][]string{
		{"sample dir", "-n", "2", "troop"},
		{"-n2", "sample dir", "troop"},
		{"--count=2", "sample dir", "troop"},
		{"sample dir", "--count", "2", "troop"},
	} {
		fs := NewFlagSet("test")
		n := fs.IntP("count", "n", 1, "runs")
		help, err := Parse(fs, args)
		if err != nil || help {
			t.Fatalf("%v: %v %v", args, help, err)
		}
		if *n != 2 || fs.NArg() != 2 || fs.Arg(0) != "sample dir" {
			t.Fatalf("%v: %d %v", args, *n, fs.Args())
		}
	}
	fs := NewFlagSet("test")
	if help, err := Parse(fs, []string{"--", "--help"}); err != nil || help || fs.Arg(0) != "--help" {
		t.Fatalf("%v %v %v", help, err, fs.Args())
	}
	fs = NewFlagSet("test")
	if help, err := Parse(fs, []string{"x", "-h"}); err != nil || !help {
		t.Fatalf("%v %v", help, err)
	}
}

func TestParseRejectsSingleDashLongOptions(t *testing.T) {
	for _, args := range [][]string{{"-seed", "7"}, {"-out", "dir"}, {"-seed=7"}} {
		fs := NewFlagSet("test")
		fs.Uint64("seed", 1, "seed")
		fs.StringP("out", "o", "", "out")
		_, err := Parse(fs, args)
		if err == nil || !strings.Contains(err.Error(), "use --") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	// A value that looks like a single-dash option belongs to its option.
	fs := NewFlagSet("test")
	note := fs.String("note", "", "note")
	fs.StringP("out", "o", "", "out")
	if _, err := Parse(fs, []string{"--note", "-out", "-o", "-note"}); err != nil || *note != "-out" {
		t.Fatalf("%v %q", err, *note)
	}
}

func TestPageShape(t *testing.T) {
	fs := NewFlagSet("demo")
	fs.StringP("out", "o", "", "write to `DIR`")
	fs.Int("tries", 3, "tries")
	fs.Bool("extra", false, "a flag left out of every group")
	p := Page{
		Name: "demo", Summary: "a demo", Usage: []string{"demo [OPTIONS]"},
		Examples: []Example{{"run it", "demo -o x"}},
		Groups:   []Group{{Title: "Output", Flags: []string{"out", "tries"}}, {Title: "General", Flags: []string{"help"}}},
		Notes:    []string{"Requirements: none."},
	}
	got := p.String(fs)
	for _, want := range []string{
		"demo - a demo (", "[Usage]\n  demo [OPTIONS]\n", "[Examples]\n  - run it\n    demo -o x\n",
		"[Options]\n  Output:\n    -o, --out DIR", "--tries N", "(default: 3)", "  Other:\n", "--extra", "  General:\n    -h, --help",
		"Requirements: none.", Links,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "Other:") > strings.Index(got, "General:") {
		t.Error("General must stay last")
	}
	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > Width {
			t.Errorf("line longer than %d: %q", Width, line)
		}
	}
}

// A release build stamps the version with -ldflags; it wins over the build info.
func TestVersionFromReleaseBuild(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "v1.2.3"
	if got := Version(); got != "v1.2.3" {
		t.Fatalf("Version() = %q", got)
	}
	version = ""
	if got := Version(); got == "" {
		t.Fatal("Version() is empty without a stamped version")
	}
}

func TestOutputFormatIgnoresValuesAndSurvivesParseErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--unknown", "--format", "json"}, "json"},
		{[]string{"-title", "bad", "--format=jsonl"}, "jsonl"},
		{[]string{"--title", "--format", "json"}, "text"},
		{[]string{"--", "--format", "json"}, "text"},
		{[]string{"-t", "--format", "json"}, "text"},
		{[]string{"-vt", "--format", "json"}, "text"},
		{[]string{"--format", "json", "--format", "jsonl"}, "jsonl"},
	} {
		f := NewFlagSet("test")
		f.String("format", "text", "format")
		f.StringP("title", "t", "", "title")
		f.BoolP("verbose", "v", false, "verbose")
		if got := OutputFormat(f, tc.args); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.args, got, tc.want)
		}
	}
}

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
