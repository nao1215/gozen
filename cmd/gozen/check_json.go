package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	data "github.com/nao1215/gozen/schema"
)

// checkJSON prints every problem of the game as a JSON array of diagnostics (file, line,
// col, path, message, severity, code) on stdout, warnings included. It fails when one is
// an error. only is "", "structure" or "layout" (check --only); with layout, the
// warnings of the structure are left out (its errors stay: the layout needs a game).
func (c *command) checkJSON(dir, only string) error {
	var db *data.DB
	var diags gamedata.Diagnostics
	fsys, err := gameFS(dir)
	if err != nil {
		diags = gamedata.Diagnose(err, "error")
	} else {
		db, diags = checkdata.Diagnose(fsys)
	}
	if db != nil && only == "layout" {
		diags = nil
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
			diags = append(diags, gamedata.Diagnostic{File: m.file, Line: m.line,
				Message: fmt.Sprintf("a text of %d lines does not fit the message window (%s): %s", m.lines, m.kind, m.text), Severity: "warning",
				Code: diag.TextOverflow.String()})
		}
	}
	if diags == nil {
		diags = gamedata.Diagnostics{}
	}
	b, err := json.MarshalIndent(diags, "", "  ")
	if err != nil {
		return err
	}
	c.out.Println(string(b))
	failed := 0
	for _, d := range diags {
		if d.Severity == "error" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("check: %d errors", failed)
	}
	return nil
}
