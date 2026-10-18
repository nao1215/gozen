// Package cli implements the common command-line I/O and argument contract.
package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Output remembers the first write failure; callers must check Err before success.
type Output struct {
	Writer io.Writer
	Err    error
}

func (o *Output) Write(b []byte) (int, error) {
	if o.Err != nil {
		return 0, o.Err
	}
	n, err := o.Writer.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	o.Err = err
	return n, err
}
func (o *Output) Printf(f string, a ...any) {
	if o.Err == nil {
		_, o.Err = fmt.Fprintf(o, f, a...)
	}
}
func (o *Output) Println(a ...any) {
	if o.Err == nil {
		_, o.Err = fmt.Fprintln(o, a...)
	}
}
func (o *Output) Print(a ...any) {
	if o.Err == nil {
		_, o.Err = fmt.Fprint(o, a...)
	}
}

// UsageError indicates invalid command-line syntax (exit 2).
type UsageError struct{ Message string }

func (e *UsageError) Error() string    { return e.Message }
func Invalid(f string, a ...any) error { return &UsageError{fmt.Sprintf(f, a...)} }

// Parse accepts flags on either side of operands, and honors -- for literal paths.
func Parse(f *flag.FlagSet, args []string, usage, requirements string, out *Output) error {
	var b bytes.Buffer
	f.SetOutput(&b)
	f.Usage = func() {
		fmt.Fprintf(&b, "Usage: %s\n", usage) //nolint:errcheck // bytes.Buffer writes cannot fail.
		f.PrintDefaults()
		fmt.Fprintf(&b, "\nRequirements:\n  %s\n  See https://github.com/nao1215/gozen#%s\n", requirements, strings.Split(f.Name(), " ")[0]) //nolint:errcheck // bytes.Buffer writes cannot fail.
	}
	var flags, operands []string
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if len(s) < 2 || s[0] != '-' {
			operands = append(operands, s)
			continue
		}
		key := strings.TrimLeft(s, "-")
		key, _, hasValue := strings.Cut(key, "=")
		flags = append(flags, s)
		if key == "h" || key == "help" {
			continue
		}
		entry := f.Lookup(key)
		if entry == nil {
			return Invalid("unknown flag %s", s)
		}
		isBool := false
		if v, ok := entry.Value.(interface{ IsBoolFlag() bool }); ok {
			isBool = v.IsBoolFlag()
		}
		if !hasValue && !isBool {
			if i+1 >= len(args) {
				return Invalid("flag %s needs a value", s)
			}
			flags = append(flags, args[i+1]) //nolint:gosec // i+1 is checked against len(args) immediately above.
			i++
		}
	}
	err := f.Parse(append(append(flags, "--"), operands...))
	if errors.Is(err, flag.ErrHelp) {
		out.Print(b.String())
		return flag.ErrHelp
	}
	if err != nil {
		return Invalid("%v", err)
	}
	return nil
}
