// Package cli implements the common command-line I/O and argument contract.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/nao1215/gozen/internal/diag"
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

// UsageError indicates invalid command-line syntax (exit 2). Code is its
// diagnostic code (GZN2xxx, ADR 0024).
type UsageError struct {
	Message string
	Code    diag.Code
}

func (e *UsageError) Error() string { return e.Code.Mark(e.Message) }

// Invalid is an argument error with its code.
func Invalid(code diag.Code, f string, a ...any) error {
	return &UsageError{fmt.Sprintf(f, a...), code}
}

// Parse turns an error of clihelp.Parse into an argument error, keeping its code.
func Parse(err error) error {
	if d, ok := errors.AsType[*diag.Error](err); ok {
		return &UsageError{d.Err.Error(), d.Code}
	}
	return &UsageError{err.Error(), diag.BadValue}
}
