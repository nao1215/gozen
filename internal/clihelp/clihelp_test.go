package clihelp

import (
	"strings"
	"testing"
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
