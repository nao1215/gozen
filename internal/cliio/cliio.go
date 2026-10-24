// Package cliio implements the common command-line I/O and argument contract.
package cliio

import (
	"encoding/json"
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

// Report selects the versioned command result contract. Callers own the writers
// and provide command-specific data; it never runs commands or creates artifacts.
type Report struct {
	Command string
	Format  string
}

// Validate rejects unsupported encodings before a command changes files.
func (r Report) Validate() error {
	switch r.Format {
	case "text", "json", "jsonl":
		return nil
	default:
		return Invalid(diag.NotAChoice, "--format is text, json, or jsonl, not %q", r.Format)
	}
}

// Machine reports whether stdout and diagnostics must contain only JSON records.
func (r Report) Machine() bool { return r.Format == "json" || r.Format == "jsonl" }

type result struct {
	Version int    `json:"version"`
	Command string `json:"command"`
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	Status  string `json:"status"`
	Data    any    `json:"data"`
}

// Result writes one JSON document or the terminal JSONL summary. Text rendering
// stays with the command, which knows the meaning of its data.
func (r Report) Result(w io.Writer, status string, data any) error {
	if !r.Machine() {
		return errors.New("structured result requires json or jsonl")
	}
	v := result{Version: 1, Command: r.Command, Status: status, Data: data}
	if r.Format == "jsonl" {
		v.Type = "summary"
	}
	return json.NewEncoder(w).Encode(v)
}

// Item writes one JSONL item. Callers preserve input order and finish with Result.
func (r Report) Item(w io.Writer, id, status string, data any) error {
	if r.Format != "jsonl" {
		return errors.New("item requires jsonl")
	}
	return json.NewEncoder(w).Encode(result{Version: 1, Command: r.Command, Type: "item", ID: id, Status: status, Data: data})
}

// Diagnostic writes a fatal error as one JSONL record to the diagnostic writer.
// Its return value reports a failed diagnostic write, not the command's exit code.
func (r Report) Diagnostic(w io.Writer, err error) error {
	code := diag.Of(err).String()
	message := err.Error()
	if u, ok := errors.AsType[*UsageError](err); ok {
		code, message = u.Code.String(), u.Message
	}
	return json.NewEncoder(w).Encode(struct {
		Version  int    `json:"version"`
		Command  string `json:"command"`
		Severity string `json:"severity"`
		Code     string `json:"code,omitempty"`
		Message  string `json:"message"`
	}{1, r.Command, "error", code, message})
}
