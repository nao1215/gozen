package cliio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

type failingWriter struct{ n int }

func (w *failingWriter) Write(b []byte) (int, error) {
	w.n--
	if w.n < 0 {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}

func TestOutputKeepsTheFirstFailureAndStopsWriting(t *testing.T) {
	w := &failingWriter{n: 1}
	out := &Output{Writer: w}
	out.Print("a")
	if out.Err != nil {
		t.Fatalf("the first write succeeds: %v", out.Err)
	}
	out.Printf("%d", 2)
	if !errors.Is(out.Err, io.ErrClosedPipe) {
		t.Fatalf("the second write fails: %v", out.Err)
	}
	first := out.Err
	out.Println("x")
	out.Print("y")
	out.Printf("z")
	if !errors.Is(out.Err, first) || w.n != -1 {
		t.Errorf("after a failure nothing more is written: err %v, calls left %d", out.Err, w.n)
	}
	if n, err := out.Write([]byte("w")); n != 0 || !errors.Is(err, first) {
		t.Errorf("Write after a failure: %d %v", n, err)
	}
}

func TestUsageErrorsCarryTheirCodes(t *testing.T) {
	err := Invalid(diag.Operands, "expected %d args", 2)
	var u *UsageError
	if !errors.As(err, &u) || u.Code != diag.Operands || u.Message != "expected 2 args" {
		t.Fatalf("%#v", err)
	}
	if got := err.Error(); got != "expected 2 args ["+diag.Operands.String()+"]" {
		t.Errorf("the message ends with the code: %q", got)
	}
	coded := Parse(diag.UnknownOption.Errorf("unknown option --zzz"))
	if !errors.As(coded, &u) || u.Code != diag.UnknownOption || !strings.Contains(u.Message, "--zzz") || strings.Contains(u.Message, "GZN") {
		t.Errorf("a coded parse error keeps its code and its bare text: %#v", coded)
	}
	plain := Parse(errors.New("something else"))
	if !errors.As(plain, &u) || u.Code != diag.BadValue || u.Message != "something else" {
		t.Errorf("an error with no code is a bad value: %#v", plain)
	}
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) / 2, nil }
func TestOutputShortWrite(t *testing.T) {
	out := &Output{Writer: shortWriter{}}
	out.Println("hello")
	if !errors.Is(out.Err, io.ErrShortWrite) {
		t.Fatal(out.Err)
	}
}

func TestReportRoundTripAndWriteFailures(t *testing.T) {
	for _, format := range []string{"json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			r := Report{Command: "new", Format: format}
			var out bytes.Buffer
			value := "line\nwith \"quotes\" and 日本語"
			if err := r.Result(&out, "ok", map[string]string{"title": value}); err != nil {
				t.Fatal(err)
			}
			var got result
			dec := json.NewDecoder(&out)
			if err := dec.Decode(&got); err != nil {
				t.Fatal(err)
			}
			data, ok := got.Data.(map[string]any)
			if !ok || got.Version != 1 || got.Command != "new" || data["title"] != value {
				t.Fatalf("%+v", got)
			}
			if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Fatalf("extra output: %v", err)
			}
			if (got.Type == "summary") != (format == "jsonl") {
				t.Fatal(got.Type)
			}
			w := &Output{Writer: shortWriter{}}
			if err := r.Result(w, "ok", nil); !errors.Is(err, io.ErrShortWrite) {
				t.Fatal(err)
			}
		})
	}
}

func TestDiagnosticCodesSurviveStructuredReporting(t *testing.T) {
	for _, entry := range diag.All() {
		if entry.Code.ExitCode() == 0 {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			err := entry.Code.Errorf("operation failed")
			if entry.Code.ExitCode() == 2 {
				err = Invalid(entry.Code, "operation failed")
			}
			err = fmt.Errorf("outer: %w", err)
			var out bytes.Buffer
			if e := (Report{Command: "test", Format: "json"}).Diagnostic(&out, err); e != nil {
				t.Fatal(e)
			}
			var got struct {
				Code     string
				Message  string
				Severity string
			}
			if e := json.Unmarshal(out.Bytes(), &got); e != nil {
				t.Fatal(e)
			}
			if got.Code != entry.Code.String() || got.Severity != "error" || !strings.Contains(got.Message, "operation failed") {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestFinishPreservesExitAndWriteFailures(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			report := Report{Command: "sim routes", Format: format}
			for _, tc := range []struct {
				name   string
				err    error
				exit   int
				silent bool
			}{
				{"success", nil, 0, true},
				{"usage", Invalid(diag.Operands, "expected a game"), 2, false},
				{"runtime", io.ErrUnexpectedEOF, 1, false},
				{"reported", ErrReported, 1, true},
				{"joined", errors.Join(ErrReported, io.ErrClosedPipe), 1, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					out, diagnostics := &Output{Writer: &stdout}, &Output{Writer: &stderr}
					if exit := report.Finish(out, diagnostics, tc.err); exit != tc.exit {
						t.Fatalf("exit %d, want %d", exit, tc.exit)
					}
					if stdout.Len() != 0 || (stderr.Len() == 0) != tc.silent {
						t.Fatalf("stdout %s; stderr %s", stdout.String(), stderr.String())
					}
					if tc.name == "usage" && !strings.Contains(stderr.String(), diag.Operands.String()) {
						t.Fatal("lost diagnostic code")
					}
					if report.Machine() && stderr.Len() > 0 && !json.Valid(stderr.Bytes()) {
						t.Fatalf("non-JSON diagnostic: %s", stderr.String())
					}
				})
			}
			for _, operationErr := range []error{nil, ErrReported, Invalid(diag.Operands, "bad input")} {
				out := &Output{Writer: &bytes.Buffer{}, Err: io.ErrShortWrite}
				if exit := report.Finish(out, &Output{Writer: &bytes.Buffer{}}, operationErr); exit != 1 {
					t.Fatalf("stdout failure: exit %d", exit)
				}
			}
			if exit := report.Finish(&Output{Writer: &bytes.Buffer{}}, &Output{Writer: shortWriter{}}, Invalid(diag.Operands, "bad input")); exit != 1 {
				t.Fatalf("diagnostic short write: exit %d", exit)
			}
		})
	}
}
