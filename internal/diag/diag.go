// Package diag defines the diagnostic codes of gozen (ADR 0024): stable,
// searchable names for the problems the command and the definitions loader
// report.
//
// A code is GZN and four digits. The thousands digit is the exit value of the
// process that reports it, so a code refines the exit contract of ADR 0012
// rather than adding a second one:
//
//	GZN0xxx  exit 0  a warning; the command still succeeds
//	GZN1xxx  exit 1  processing failed: the game, a file, a simulation, a capture
//	GZN2xxx  exit 2  the command line is invalid
//
// The hundreds digit is the family: what the reader has to fix. One code per
// fix, never one per call site, so the published list reads as problems a
// person can act on. Messages may be reworded; codes do not change.
//
// register is the only way to make a Code, and it refuses an entry without
// the text the published reference needs, so an undocumented code cannot
// exist.
package diag

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Code is one diagnostic's stable identity. The zero value means "no code".
type Code int

// String renders the code in its published form, e.g. "GZN1101". The zero
// value renders as the empty string.
func (c Code) String() string {
	if c == 0 {
		return ""
	}
	return fmt.Sprintf("GZN%04d", int(c))
}

// ExitCode is the exit value a diagnostic with this code produces: the
// thousands digit.
func (c Code) ExitCode() int { return int(c) / 1000 }

// Mark adds the code to the end of a message: "msg [GZN1101]". The position
// at the start of a line stays where editors look for it. The zero code
// returns msg unchanged.
func (c Code) Mark(msg string) string {
	if c == 0 {
		return msg
	}
	return msg + " [" + c.String() + "]"
}

// Errorf builds an error whose message ends with the code. It forwards to
// fmt.Errorf, so %w still wraps. Only the site that names the problem adds a
// code; a wrapper that adds context leaves it to the error it wraps.
func (c Code) Errorf(format string, args ...any) error {
	return &Error{Code: c, Err: fmt.Errorf(format, args...)}
}

// Wrap attaches the code to err; a nil err stays nil.
func (c Code) Wrap(err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: c, Err: err}
}

// Error is an error with a code. Its message is the wrapped message with the
// code at the end.
type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string { return e.Code.Mark(e.Err.Error()) }
func (e *Error) Unwrap() error { return e.Err }

// Of returns the code of the first coded error in err's chain, or 0.
func Of(err error) Code {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return 0
}

// Entry is a code's published documentation.
type Entry struct {
	// Code is the diagnostic's identity.
	Code Code
	// Name is the Go identifier this package binds the code to; a test uses it
	// to prove every code is raised from real code.
	Name string
	// Summary is the one-line meaning, written as the problem.
	Summary string
	// Detail explains what gozen was doing and why this is reported.
	Detail string
	// Fix tells the reader what to change.
	Fix string
	// Since is the gozen version the code first shipped in.
	Since string
}

// since is the version every code registered before the first release ships in.
const since = "v0.1.0"

// codeText matches a code in its published form anywhere in a string.
var codeText = regexp.MustCompile(`\bGZN(\d{4})\b`)

var (
	registry []Entry
	byCode   = map[Code]Entry{}
)

// register records one diagnostic and returns its code. It panics on a
// malformed registration: every call is a package-level variable, so a
// mistake fails the first test that imports diag.
func register(number int, name string, e Entry) Code {
	c := Code(number)
	switch {
	case number < 1 || number > 2999:
		panic(fmt.Sprintf("diag: code %d is outside GZN0001-GZN2999", number))
	case number%100 == 0:
		panic(fmt.Sprintf("diag: %s ends in 00, which names a family, not a code", c))
	case family(c) == nil:
		panic(fmt.Sprintf("diag: %s is in no family", c))
	case name == "":
		panic(fmt.Sprintf("diag: %s has no identifier name", c))
	case e.Summary == "":
		panic(fmt.Sprintf("diag: %s has no summary", c))
	case e.Detail == "":
		panic(fmt.Sprintf("diag: %s has no detail", c))
	case e.Fix == "":
		panic(fmt.Sprintf("diag: %s has no fix", c))
	}
	if prev, dup := byCode[c]; dup {
		panic(fmt.Sprintf("diag: %s is already registered as %s", c, prev.Name))
	}
	if e.Since == "" {
		e.Since = since
	}
	e.Code, e.Name = c, name
	registry = append(registry, e)
	byCode[c] = e
	return c
}

// All returns every registered diagnostic in code order.
func All() []Entry {
	out := slices.Clone(registry)
	slices.SortFunc(out, func(a, b Entry) int { return int(a.Code) - int(b.Code) })
	return out
}

// Lookup returns the entry for a code.
func Lookup(c Code) (Entry, bool) {
	e, ok := byCode[c]
	return e, ok
}

// Syntax reports whether s has the published form of a code (GZN and four
// digits, any case), registered or not.
func Syntax(s string) bool {
	return len(s) == 7 && strings.EqualFold(s[:3], "GZN") && strings.Trim(s[3:], "0123456789") == ""
}

// Parse reads a registered code in its published form, ignoring case.
func Parse(s string) (Code, bool) {
	s = strings.TrimSpace(s)
	if !Syntax(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s[3:])
	if err != nil {
		return 0, false
	}
	c := Code(n)
	if _, ok := byCode[c]; !ok {
		return 0, false
	}
	return c, true
}

// Number reads the number of a code in its published form whether or not it
// is registered; ok is false when s does not have the form.
func Number(s string) (Code, bool) {
	if !Syntax(strings.TrimSpace(s)) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(s)[3:])
	return Code(n), err == nil
}

// Near returns the registered codes of the family c belongs to, in order. It
// is what an unknown code is answered with.
func Near(c Code) []Entry {
	var out []Entry
	for _, e := range All() {
		if int(e.Code)/100 == int(c)/100 {
			out = append(out, e)
		}
	}
	return out
}

// Codes returns every registered code mentioned in text, in order of
// appearance and without repeats.
func Codes(text string) []Code {
	var out []Code
	for _, m := range codeText.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		c := Code(n)
		if _, ok := byCode[c]; ok && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
