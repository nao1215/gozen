// Package cli implements the common command-line I/O and argument contract.
package cli

import (
	"fmt"
	"io"
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
