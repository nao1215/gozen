package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/model/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

type checkPolicy struct {
	strict                  bool
	baseline, writeBaseline string
}

// Counts use source-relative paths; moving a warning to another file spends a
// new budget. An empty code or file is an explicit bucket for unpositioned warnings.
type checkBaseline struct {
	Version int                       `json:"version"`
	Only    string                    `json:"only"`
	Counts  map[string]map[string]int `json:"counts"`
}

func warningCounts(only string, ds gamedata.Diagnostics) checkBaseline {
	b := checkBaseline{Version: 1, Only: only, Counts: map[string]map[string]int{}}
	for _, d := range ds {
		if d.Severity != "warning" {
			continue
		}
		if b.Counts[d.Code] == nil {
			b.Counts[d.Code] = map[string]int{}
		}
		b.Counts[d.Code][d.File]++
	}
	return b
}

func readCheckBaseline(name, only string) (b checkBaseline, err error) {
	f, err := os.Open(name) //nolint:gosec // Explicit CLI input.
	if err != nil {
		return b, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return b, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return b, errors.New(lang.T("基準ファイルには JSON オブジェクトを一つだけ指定してください"))
	}
	if b.Version != 1 || b.Only != only || b.Counts == nil {
		return b, errors.New(lang.T("基準ファイルの版または --only の範囲が一致しません"))
	}
	for code, files := range b.Counts {
		if code != "" {
			c, ok := diag.Parse(code)
			if !ok || c.String() != code || c.ExitCode() > 1 {
				return b, fmt.Errorf(lang.T("基準ファイルの警告コードが不正です: %s"), code)
			}
		}
		for file, count := range files {
			if count < 0 || file != "" && !fs.ValidPath(file) {
				return b, fmt.Errorf(lang.T("基準ファイルの件数または相対パスが不正です: %s"), file)
			}
		}
	}
	return b, nil
}

func (b checkBaseline) growsFrom(old checkBaseline) bool {
	for code, files := range b.Counts {
		for file, count := range files {
			if count > old.Counts[code][file] {
				return true
			}
		}
	}
	return false
}

func writeCheckBaseline(name string, b checkBaseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // Explicit CLI output; never overwrite inputs.
	if errors.Is(err, fs.ErrExist) {
		return diag.OutputExists.Wrap(err)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	if err = errors.Join(err, f.Close()); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	return nil
}

func (c *command) checkEnforced(dir, only string, policy checkPolicy) error {
	var previous checkBaseline
	if policy.baseline != "" {
		var err error
		previous, err = readCheckBaseline(policy.baseline, only)
		if err != nil {
			return fmt.Errorf("%s: %w", policy.baseline, err)
		}
	}
	ds, err := diagnoseCheck(dir, only)
	if err != nil {
		return err
	}
	current := warningCounts(only, ds)
	failed := false
	for _, d := range ds {
		failed = failed || d.Severity == "error"
	}
	rejected := (policy.strict || policy.baseline != "") && current.growsFrom(previous)
	if policy.writeBaseline != "" && !failed {
		if err := writeCheckBaseline(policy.writeBaseline, current); err != nil {
			return err
		}
	}
	if c.report.Machine() {
		return c.reportCheck(ds, rejected)
	}
	for _, d := range ds {
		position := d.File
		if d.Line > 0 {
			position += fmt.Sprintf(":%d:%d", d.Line, d.Col)
		}
		message := fmt.Sprintf("%s: %s: %s: %s", d.Severity, position, d.Path, d.Message)
		if code, ok := diag.Parse(d.Code); ok {
			message = code.Mark(message)
		}
		if _, err := fmt.Fprintln(c.stderr, message); err != nil {
			return err
		}
	}
	if failed || rejected {
		if rejected {
			if _, err := fmt.Fprintln(c.stderr, lang.T("check: 警告件数が基準を超えています")); err != nil {
				return err
			}
		}
		return cliio.ErrReported
	}
	if only == "structure" {
		_, err := fmt.Fprintln(c.out, lang.T("check: 構造検査に合格しました。レイアウトは検査していません"))
		return err
	}
	_, err = fmt.Fprintln(c.out, lang.T("check: 検査に合格しました"))
	return err
}
