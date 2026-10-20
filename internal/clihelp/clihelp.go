// Package clihelp parses GNU-style options and renders the --help pages of the
// gozen command and the runner (ADR 0018). It is shared by cmd/gozen, the
// production command packages, and runner, so every help page has the same
// shape: a title line, [Usage], [Commands], [Examples], [Options] grouped by
// kind, notes, and the project links.
package clihelp

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/diag"
)

// README is the public README section every help page and every
// missing-prerequisite diagnostic points to.
const README = "https://github.com/nao1215/gozen#gozen"

// Links ends every help page.
const Links = "Documentation:   https://nao1215.github.io/gozen/\n" +
	"Report an issue: https://github.com/nao1215/gozen/issues\n" +
	"GitHub Sponsors: https://github.com/sponsors/nao1215\n"

// Width is the column help text wraps at.
const Width = 80

// Example is one entry of [Examples]: what it does, then the command line.
type Example [2]string

// Group is one titled block of [Options]. Flags are long names in display
// order; Extra adds entries that are not flags of the set (label, description),
// such as options defined at run time.
type Group struct {
	Title string
	Flags []string
	Extra [][2]string
}

// Page describes one help page.
type Page struct {
	Name     string      // "gozen", "gozen sim" or "gozen sim battle"
	Summary  string      // one phrase after the name
	Usage    []string    // command lines, without the leading indent
	About    string      // paragraphs under [Usage], separated by blank lines
	Commands [][2]string // [Commands] (name, one-line description); top level only
	Examples []Example
	Groups   []Group
	Notes    []string // paragraphs after [Options]; requirements belong here
	Hint     string   // one line before the links, e.g. how to get a command's help
}

// version is set by the release build (-ldflags "-X ...clihelp.version=vX.Y.Z").
var version string

// Version reports the version set by the release build, the module version
// recorded by go install, or devel for source builds.
func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

// NewFlagSet returns a GNU-style flag set: options and operands may be mixed,
// -- ends the options, and -h/--help is defined. Parse errors are returned,
// never printed.
func NewFlagSet(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SetInterspersed(true)
	fs.SortFlags = false
	fs.BoolP("help", "h", false, "print this help and exit")
	return fs
}

// Parse parses args and reports whether help was asked for. A single-dash long
// option such as -seed is rejected with a hint instead of being read as a
// cluster of short options (where -out DIR would silently mean -o ut DIR).
//
// The errors carry a diagnostic code (ADR 0024): an unknown option, a
// single-dash long option, or a value that cannot be read.
func Parse(fs *pflag.FlagSet, args []string) (help bool, err error) {
	if err := singleDash(fs, args); err != nil {
		return false, diag.SingleDashOption.Wrap(err)
	}
	if err := fs.Parse(args); err != nil {
		return false, parseCode(err).Wrap(err)
	}
	return fs.GetBool("help")
}

// parseCode names a pflag error. pflag has no error types for these, so the
// message is read; an unrecognized one is a bad value.
func parseCode(err error) diag.Code {
	switch msg := err.Error(); {
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		return diag.UnknownOption
	default:
		return diag.BadValue
	}
}

// HelpRequested reports whether args ask for help before any -- terminator.
// It is for commands that must answer --help before they can define all flags.
func HelpRequested(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func singleDash(fs *pflag.FlagSet, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return nil
		}
		if strings.HasPrefix(a, "--") {
			name, _, hasValue := strings.Cut(a[2:], "=")
			if f := fs.Lookup(name); f != nil && !hasValue && f.NoOptDefVal == "" {
				i++ // the next argument is this option's value
			}
			continue
		}
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		if len(a) == 2 {
			if f := fs.ShorthandLookup(a[1:]); f != nil && f.NoOptDefVal == "" {
				i++ // the next argument is this option's value
			}
			continue
		}
		if name, _, _ := strings.Cut(a[1:], "="); fs.Lookup(name) != nil {
			return fmt.Errorf("single-dash long option %s is not accepted; use --%s", a, a[1:])
		}
	}
	return nil
}

// String renders the page. fs may be nil for a page without options.
func (p Page) String(fs *pflag.FlagSet) string {
	var b helpText
	b.WriteString(Wrap(fmt.Sprintf("%s - %s (%s)", p.Name, p.Summary, Version()), 0))
	b.WriteString("\n[Usage]\n")
	for _, u := range p.Usage {
		b.WriteString("  " + u + "\n")
	}
	if p.About != "" {
		for _, para := range strings.Split(p.About, "\n\n") {
			b.WriteString("\n" + Wrap(para, 2))
		}
	}
	if len(p.Commands) > 0 {
		b.WriteString("\n[Commands]\n")
		w := 0
		for _, c := range p.Commands {
			w = max(w, len(c[0]))
		}
		for _, c := range p.Commands {
			b.WriteString(entry("  ", c[0], c[1], w))
		}
	}
	if len(p.Examples) > 0 {
		b.WriteString("\n[Examples]\n")
		for _, e := range p.Examples {
			b.WriteString(Wrap("- "+e[0], 2))
			b.WriteString("    " + e[1] + "\n")
		}
	}
	if fs != nil {
		b.WriteString("\n" + options(fs, p.Groups))
	}
	for _, n := range p.Notes {
		b.WriteString("\n" + Wrap(n, 0))
	}
	if p.Hint != "" {
		b.WriteString("\n" + Wrap(p.Hint, 0))
	}
	b.WriteString("\n" + Links)
	return b.String()
}

// options renders [Options]. A flag missing from every group is listed under
// Other, so a newly added flag never disappears from --help.
func options(fs *pflag.FlagSet, groups []Group) string {
	seen := map[string]bool{}
	labels := map[string]string{}
	w := 0
	for _, g := range groups {
		for _, name := range g.Flags {
			f := fs.Lookup(name)
			if f == nil {
				continue
			}
			seen[name] = true
			labels[name] = label(f)
			w = max(w, len(labels[name]))
		}
		for _, e := range g.Extra {
			w = max(w, len(e[0]))
		}
	}
	var other []string
	fs.VisitAll(func(f *pflag.Flag) {
		if !seen[f.Name] && !f.Hidden {
			other = append(other, f.Name)
			labels[f.Name] = label(f)
			w = max(w, len(labels[f.Name]))
		}
	})
	if len(other) > 0 {
		// Keep General (help, version) last.
		n := len(groups)
		if n > 0 && groups[n-1].Title == "General" {
			groups = append(append(append([]Group{}, groups[:n-1]...), Group{Title: "Other", Flags: other}), groups[n-1])
		} else {
			groups = append(append([]Group{}, groups...), Group{Title: "Other", Flags: other})
		}
	}
	var b helpText
	b.WriteString("[Options]\n")
	first := true
	for _, g := range groups {
		if !first {
			b.WriteString("\n")
		}
		first = false
		b.WriteString("  " + g.Title + ":\n")
		for _, name := range g.Flags {
			f := fs.Lookup(name)
			if f == nil {
				continue
			}
			_, usage := pflag.UnquoteUsage(f)
			b.WriteString(entry("    ", labels[name], usage+defaultNote(f), w))
		}
		for _, e := range g.Extra {
			b.WriteString(entry("    ", e[0], e[1], w))
		}
	}
	return b.String()
}

// label renders "-n, --battles N" or "    --seed N".
func label(f *pflag.Flag) string {
	s := "    "
	if f.Shorthand != "" {
		s = "-" + f.Shorthand + ", "
	}
	s += "--" + f.Name
	name, _ := pflag.UnquoteUsage(f)
	switch name {
	case "":
	case "int", "uint", "uint64", "int64":
		s += " N"
	case "float":
		s += " NUM"
	case "string", "value", "stringArray", "strings":
		s += " VALUE"
	case "duration":
		s += " DURATION"
	default:
		s += " " + name
	}
	return s
}

// defaultNote adds "(default: x)" unless the zero value (empty, false, 0)
// already means "not given" or the description states the default itself.
func defaultNote(f *pflag.Flag) string {
	if f.DefValue == "" || f.DefValue == "false" || f.DefValue == "0" || f.DefValue == "[]" || strings.Contains(f.Usage, "default") {
		return ""
	}
	return " (default: " + f.DefValue + ")"
}

// entry renders a label and its description wrapped into the column after it.
func entry(indent, label, desc string, w int) string {
	pad := indent + strings.Repeat(" ", w+2)
	first := indent + label + strings.Repeat(" ", w-len(label)+2)
	return wrapWith(desc, first, pad)
}

// Wrap fills one paragraph to Width with the given indent.
func Wrap(s string, indent int) string {
	pad := strings.Repeat(" ", indent)
	return wrapWith(s, pad, pad)
}

func wrapWith(s, first, rest string) string {
	var b helpText
	line := first
	start := len(first)
	limit := max(Width-len(rest), 20)
	col := 0
	for _, word := range strings.Fields(s) {
		switch {
		case col == 0:
			line += word
			col = len(word)
		case col+1+len(word) <= limit-(start-len(rest)):
			line += " " + word
			col += 1 + len(word)
		default:
			b.WriteString(line + "\n")
			line, col, start = rest+word, len(word), len(rest)
		}
	}
	b.WriteString(line + "\n")
	return b.String()
}

// ErrHelp is returned by helpers that printed help; callers treat it as success.
var ErrHelp = errors.New("help requested")

// helpText collects help output; unlike strings.Builder it has no error to ignore.
type helpText []string

func (t *helpText) WriteString(s string) { *t = append(*t, s) }
func (t helpText) String() string        { return strings.Join(t, "") }
