package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

// ADR 0024: a diagnostic ends with its code, the first digit of the code is the exit status,
// and a usage error also says where the help is.
func TestUsageErrorsEndWithCodeAndAPointerToHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code diag.Code
		help string
	}{
		{nil, diag.CommandMissing, "gozen --help"},
		{[]string{"--zzz"}, diag.UnknownOption, "gozen --help"},
		{[]string{"-v", "extra"}, diag.OptionBeforeCommand, "gozen --help"},
		{[]string{"--seed", "3", "check", "x"}, diag.UnknownOption, "gozen --help"},
		{[]string{"-version"}, diag.SingleDashOption, "gozen --help"},
		{[]string{"bogus"}, diag.UnknownCommand, "gozen --help"},
		{[]string{"sim"}, diag.CommandMissing, "gozen sim --help"},
		{[]string{"sim", "bogus"}, diag.UnknownCommand, "gozen sim --help"},
		{[]string{"sim", "battle", "--seed", "x"}, diag.BadValue, "gozen sim battle --help"},
		{[]string{"sim", "battle", "--zzz"}, diag.UnknownOption, "gozen sim battle --help"},
		{[]string{"check"}, diag.Operands, "gozen check --help"},
		{[]string{"check", "--only", "everything", "x"}, diag.NotAChoice, "gozen check --help"},
		{[]string{"check", "--format", "xml", "x"}, diag.NotAChoice, "gozen check --help"},
		{[]string{"book"}, diag.Operands, "gozen book --help"},
		{[]string{"version", "extra"}, diag.Operands, "gozen version --help"},
		{[]string{"play", "notes"}, diag.Operands, "gozen play notes --help"},
		{[]string{"help", "GZN9999"}, diag.UnknownCode, "gozen --help"},
	} {
		out, stderr := invoke(t, 2, tc.args...)
		if out != "" {
			t.Errorf("%v: stdout %q", tc.args, out)
		}
		lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
		if len(lines) < 2 || !strings.Contains(lines[0], "["+tc.code.String()+"]") && !strings.Contains(stderr, "["+tc.code.String()+"]") {
			t.Errorf("%v: want %s in %q", tc.args, tc.code, stderr)
			continue
		}
		if last := lines[len(lines)-1]; last != `Run "`+tc.help+`" for usage.` {
			t.Errorf("%v: last line %q", tc.args, last)
		}
	}
}

func TestVersionAndHelpAnswerOnStdoutWithExitZero(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"-v"}, {"--version"}} {
		out, stderr := invoke(t, 0, args...)
		if !strings.HasPrefix(out, "gozen ") || strings.Count(out, "\n") != 1 || stderr != "" {
			t.Errorf("%v: %q %q", args, out, stderr)
		}
	}
	for _, args := range [][]string{{"version", "--help"}, {"help", "version"}, {"help", "help", "version"}} {
		out, _ := invoke(t, 0, args...)
		if !strings.HasPrefix(out, "gozen version - ") {
			t.Errorf("%v: %q", args, out)
		}
	}
	// Help works before the prerequisites do: no game directory is needed.
	for _, args := range [][]string{{"check", "-h"}, {"sim", "battle", "-h"}} {
		out, _ := invoke(t, 0, args...)
		if !strings.Contains(out, "[Options]") {
			t.Errorf("%v: %q", args, out)
		}
	}
}

func TestHelpForACodeNamesItsFamilyWhenTheCodeIsNotRegistered(t *testing.T) {
	var unregistered string
	for n := 1101; n < 1199; n++ {
		if _, ok := diag.Parse(diag.Code(n).String()); !ok {
			unregistered = diag.Code(n).String()
			break
		}
	}
	if unregistered == "" {
		t.Skip("every code of the family is registered")
	}
	_, stderr := invoke(t, 2, "help", strings.ToLower(unregistered))
	if !strings.Contains(stderr, "unknown diagnostic code "+unregistered) || !strings.Contains(stderr, "codes of the same family: GZN11") || !strings.Contains(stderr, diag.URL) {
		t.Errorf("stderr: %q", stderr)
	}
	// A code outside every family has no neighbours to list.
	_, stderr = invoke(t, 2, "help", "GZN2999")
	if strings.Contains(stderr, "same family") || !strings.Contains(stderr, "see "+diag.URL) {
		t.Errorf("stderr: %q", stderr)
	}
}

type failAfter struct{ ok int }

func (w *failAfter) Write(b []byte) (int, error) {
	w.ok--
	if w.ok < 0 {
		return 0, errWrite
	}
	return len(b), nil
}

// A diagnostic that cannot be written still ends in a nonzero status.
func TestRunSurvivesAStderrThatFails(t *testing.T) {
	var out bytes.Buffer
	if got := run([]string{"bogus"}, &out, &failAfter{}); got != 2 {
		t.Errorf("exit %d", got)
	}
	if got := run([]string{"check", "/does/not/exist"}, &out, &failAfter{}); got != 1 {
		t.Errorf("exit %d", got)
	}
}
