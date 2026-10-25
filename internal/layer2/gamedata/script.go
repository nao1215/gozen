package gamedata

import (
	"errors"
	"maps"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// ScriptFile is the file name used for diagnostics of command lists and expressions passed from
// Go.
const ScriptFile = "<script>"

// IsScript reports whether src is a command list (a YAML sequence). Otherwise it is treated as a
// single expression.
func IsScript(src string) bool {
	return strings.HasPrefix(strings.TrimSpace(src), "-") || strings.HasPrefix(strings.TrimSpace(src), "[")
}

// ParseScript reads a command list. Errors are Diagnostics with positions.
func ParseScript(src string) ([]Command, Diagnostics) {
	var cmds []Command
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions([]byte(src), &cmds, yaml.Strict()) }); err != nil {
		d := Diagnostic{File: ScriptFile, Message: yaml.FormatError(err, false, false), Severity: "error", err: err}
		if ye, ok := errors.AsType[yaml.Error](err); ok {
			if tk := ye.GetToken(); tk != nil && tk.Position != nil {
				d.Line, d.Col = tk.Position.Line, tk.Position.Column
			}
		}
		return nil, Diagnostics{d}
	}
	return cmds, nil
}

// CheckScript statically checks a command list (YAML starting with "- ") or a single expression
// against db's definitions.
// Spec-version limits are not examined (what is run from Go is not a data contract). If dev is
// false,
// a set on a party member's ability or level is an error (author's decision 5).
func CheckScript(db *DB, src string, dev bool) Diagnostics {
	c := &checker{db: db, script: true, noDev: !dev}
	if !IsScript(src) {
		e, err := parseExpr(src)
		if err != nil {
			return Diagnostics{{File: ScriptFile, Line: 1, Col: 1, Message: err.Error(), Severity: "error", err: err}}
		}
		c.expr(ScriptFile, e, withEventNames(FieldNames))
		return scriptDiagnostics(c.errs, nil)
	}
	_, ds := CompileScript(db, src, dev)
	return ds
}

// scriptDiagnostics turns check errors into Diagnostics, looking up the line from the command's
// index.
func scriptDiagnostics(errs []error, lines []int) Diagnostics {
	out := Diagnostics{}
	for _, d := range Diagnose(joinDiagnostics(errs), "error") {
		d.File = ScriptFile
		d.Path = strings.TrimPrefix(d.Path, ScriptFile)
		d.Line, d.Col = 1, 1
		if rest, ok := strings.CutPrefix(d.Path, "["); ok {
			if n, err := strconv.Atoi(rest[:strings.IndexByte(rest, ']')]); err == nil && n < len(lines) {
				d.Line = lines[n]
			}
		}
		d.err = nil
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// itemLines is the line (1-based) of each item in the sequence.
func itemLines(src string) []int {
	f, err := parser.ParseBytes([]byte(src), 0)
	if err != nil || len(f.Docs) == 0 {
		return nil
	}
	seq, ok := f.Docs[0].Body.(*ast.SequenceNode)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(seq.Values))
	for _, v := range seq.Values {
		line := 1
		if tk := v.GetToken(); tk != nil && tk.Position != nil {
			line = tk.Position.Line
		}
		out = append(out, line)
	}
	return out
}

// Fork is a copy of db. The loaded definitions are shared, and only what changes during play (the
// maps of generated dungeons)
// is held separately. Make one per play when playing from one Game many times.
func Fork(db *DB) *DB {
	out := *db
	out.Actors = make(map[string]*Actor, len(db.Actors))
	maps.Copy(out.Actors, db.Actors)
	out.Locations = make(map[string]*Location, len(db.Locations))
	for id, l := range db.Locations {
		if l != nil && l.Dungeon != nil {
			lc, dc := *l, *l.Dungeon
			dc.Looks = append([]DungeonLook(nil), dc.Looks...)
			dc.Grid = append([][]rune(nil), dc.Grid...)
			lc.Dungeon = &dc
			l = &lc
		}
		out.Locations[id] = l
	}
	return &out
}

// CompileScript parses and validates commands for session, capture and development
// tools. Returned commands resolve reusable verbs to calls; errors return no commands.
func CompileScript(db *DB, src string, dev bool) ([]Command, Diagnostics) {
	cmds, ds := ParseScript(src)
	if ds != nil {
		return nil, ds
	}
	c := &checker{db: db, script: true, noDev: !dev}
	c.commands(ScriptFile, cmds, FieldNames)
	if ds := scriptDiagnostics(c.errs, itemLines(src)); len(ds) > 0 {
		return nil, ds
	}
	return cmds, nil
}
