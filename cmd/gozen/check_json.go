package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"

	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// diagnoseCheck collects structured errors and warnings for the selected scope.
// Layout checks load the game but do not run structural authoring checks or read
// their complexity configuration, just like the text-only layout command.
func diagnoseCheck(dir, only string) gamedata.Diagnostics {
	var db *gamedata.DB
	var diags gamedata.Diagnostics
	fsys, err := gameFS(dir)
	if err != nil {
		diags = gamedata.Diagnose(err, "error")
	} else if only == "layout" {
		err = gamedata.UseWords(fsys)
		if err == nil {
			db, err = gamedata.LoadDir(fsys, ".")
		}
		if diag.Of(err) == 0 && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission)) {
			err = diag.GameUnreadable.Wrap(err)
		}
		diags = gamedata.Diagnose(err, "error")
	} else {
		db, diags = checkdata.Diagnose(fsys)
	}
	if db != nil && only != "structure" {
		issues, err := checkMessages(db, fsys)
		if err == nil {
			var w string
			if w, err = builtinFontWarning(db, fsys); w != "" {
				diags = append(diags, gamedata.Diagnostic{File: "game.yaml", Path: "fonts.regular", Message: w, Severity: "warning"})
			}
		}
		var fp *fontProblem
		switch {
		case errors.As(err, &fp):
			diags = append(diags, gamedata.Diagnostic{File: "game.yaml", Line: fp.line, Col: fp.col, Path: fp.key, Message: fp.err.Error(), Severity: "error", Code: diag.Of(err).String()})
		case err != nil:
			msg := err
			if u := errors.Unwrap(err); u != nil {
				msg = u
			}
			diags = append(diags, gamedata.Diagnostic{Path: "fonts", Message: msg.Error(), Severity: "error", Code: diag.Of(err).String()})
		}
		for _, m := range issues {
			diags = append(diags, gamedata.Diagnostic{
				File: m.file, Line: m.line,
				Message: fmt.Sprintf("a text of %d lines does not fit the message window (%s): %s", m.lines, m.kind, m.text), Severity: "warning",
				Code: diag.TextOverflow.String(),
			})
		}
	}
	if diags == nil {
		diags = gamedata.Diagnostics{}
	}
	return diags
}

func (c *command) checkJSON(dir, only string) error {
	diags := diagnoseCheck(dir, only)
	return c.reportCheck(diags, false)
}

func (c *command) reportCheck(diags gamedata.Diagnostics, rejected bool) error {
	failed := 0
	for _, d := range diags {
		if d.Severity == "error" {
			failed++
		}
	}
	status := "ok"
	if failed > 0 || rejected {
		status = "failed"
	}
	for i, d := range diags {
		if c.report.Format != "jsonl" {
			break
		}
		if err := c.report.Item(c.out, strconv.Itoa(i), d.Severity, d); err != nil {
			return err
		}
	}
	if err := c.report.Result(c.out, status, diags); err != nil {
		return err
	}
	if failed > 0 || rejected {
		return cliio.ErrReported
	}
	return nil
}
