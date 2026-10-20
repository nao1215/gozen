package gamedata

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"

	"github.com/nao1215/gozen/internal/diag"
)

// sourcePosition is the position of a loaded node. Defaults filled in do not change it.
type sourcePosition struct {
	file         string
	line, column int
}

func (p sourcePosition) String() string {
	return fmt.Sprintf("%s:%d:%d", p.file, p.line, p.column)
}

type diagnostic struct {
	position sourcePosition
	where    string
	message  string
	code     diag.Code // 0 until the family has codes (ADR 0024)
}

func (d *diagnostic) Error() string {
	if d.position.file == "" {
		return d.code.Mark(d.where + ": " + d.message)
	}
	return d.code.Mark(fmt.Sprintf("%s: %s: %s", d.position, d.where, d.message))
}

func duplicateDiagnostic(at sourcePosition, where string, previous sourcePosition) error {
	return &diagnostic{at, where, fmt.Sprintf("%s is defined twice (previous definition at %s)", where, previous), diag.Duplicate}
}

// coded gives a diagnostic its code; other errors are returned as they are.
func coded(err error, code diag.Code) error {
	var d *diagnostic
	if errors.As(err, &d) {
		d.code = code
	}
	return err
}

// sortedDiagnostics orders by position, logical path and message, independent of map iteration order.
func sortedDiagnostics(errs []error) []error {
	slices.SortFunc(errs, func(a, b error) int {
		var da, db *diagnostic
		oka, okb := errors.As(a, &da), errors.As(b, &db)
		if oka && okb {
			if n := strings.Compare(da.position.file, db.position.file); n != 0 {
				return n
			}
			if n := da.position.line - db.position.line; n != 0 {
				return n
			}
			if n := da.position.column - db.position.column; n != 0 {
				return n
			}
		}
		return strings.Compare(a.Error(), b.Error())
	})
	return errs
}

func joinDiagnostics(errs []error) error {
	var flat []error
	var add func(error)
	add = func(err error) {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				add(child)
			}
		} else if err != nil {
			flat = append(flat, err)
		}
	}
	for _, err := range errs {
		add(err)
	}
	return errors.Join(sortedDiagnostics(flat)...)
}

type sourceDocument struct {
	name       string
	root       ast.Node
	positions  map[string]sourcePosition
	tokenPaths map[*token.Token]string
}

func (db *DB) readSource(b []byte, name string) (*sourceDocument, error) {
	doc := &sourceDocument{name: name, positions: map[string]sourcePosition{}, tokenPaths: map[*token.Token]string{}}
	parsed, err := parser.ParseBytes(b, 0, parser.AllowDuplicateMapKey())
	if err != nil {
		return nil, doc.yamlError(err)
	}
	if len(parsed.Docs) > 0 {
		doc.root = parsed.Docs[0].Body
	}
	// Empty files and missing items point to the start of the document or the nearest parent.
	doc.positions[""] = sourcePosition{name, 1, 1}
	var errs []error
	anchors := map[string]ast.Node{}
	active := map[ast.Node]bool{}
	var walk func(ast.Node, string)
	walk = func(node ast.Node, where string) {
		if node == nil || active[node] {
			return
		}
		active[node] = true
		defer delete(active, node)
		doc.tokenPaths[node.GetToken()] = where
		if _, exists := doc.positions[where]; !exists {
			tk := node.GetToken()
			if m, ok := node.(*ast.MappingNode); ok && len(m.Values) > 0 {
				tk = m.Values[0].Key.GetToken()
			}
			doc.positions[where] = nodePosition(name, tk)
		}
		switch n := node.(type) {
		case *ast.MappingNode:
			seen := map[string]sourcePosition{}
			for _, entry := range n.Values {
				key := sourceKey(entry.Key)
				at := nodePosition(name, entry.Key.GetToken())
				if previous, exists := seen[key]; exists {
					errs = append(errs, duplicateDiagnostic(at, numberPath(where, key), previous))
				}
				seen[key] = at
			}
			// Register the merge source first, so the explicitly written item's origin wins regardless of writing order.
			for _, entry := range n.Values {
				if sourceKey(entry.Key) == "<<" {
					doc.tokenPaths[entry.Value.GetToken()] = numberPath(where, "<<")
					if seq, ok := entry.Value.(*ast.SequenceNode); ok {
						// In a YAML merge sequence, the first map takes priority.
						for i := len(seq.Values) - 1; i >= 0; i-- {
							walk(seq.Values[i], where)
						}
					} else {
						walk(entry.Value, where)
					}
				}
			}
			for _, entry := range n.Values {
				key := sourceKey(entry.Key)
				if key == "<<" {
					continue
				}
				p := numberPath(where, key)
				doc.positions[p] = nodePosition(name, entry.Key.GetToken())
				doc.tokenPaths[entry.Key.GetToken()] = p
				walk(entry.Value, p)
			}
		case *ast.SequenceNode:
			for i, value := range n.Values {
				walk(value, fmt.Sprintf("%s[%d]", where, i))
			}
		case *ast.AnchorNode:
			anchors[n.Name.GetToken().Value] = n.Value
			walk(n.Value, where)
		case *ast.AliasNode:
			walk(anchors[n.Value.GetToken().Value], where)
		case *ast.TagNode:
			walk(n.Value, where)
		}
	}
	walk(doc.root, "")
	if len(errs) > 0 {
		return nil, joinDiagnostics(errs)
	}
	db.sourceFiles[name] = doc
	return doc, nil
}

func sourceKey(node ast.MapKeyNode) string {
	if s, ok := node.(*ast.StringNode); ok {
		return s.Value
	}
	return node.GetToken().Value
}

func nodePosition(file string, tk *token.Token) sourcePosition {
	if tk == nil || tk.Position == nil {
		return sourcePosition{file, 1, 1}
	}
	return sourcePosition{file, tk.Position.Line, tk.Position.Column}
}

func parentSourcePath(where string) string {
	if i := strings.LastIndexAny(where, ".["); i >= 0 {
		return where[:i]
	}
	return ""
}

func findPosition(positions map[string]sourcePosition, where string) sourcePosition {
	for {
		if at, ok := positions[where]; ok {
			return at
		}
		if where == "" {
			return sourcePosition{}
		}
		where = parentSourcePath(where)
	}
}

func (doc *sourceDocument) logicalPath(where string) string {
	if where == doc.name {
		return ""
	}
	if p, ok := strings.CutPrefix(where, doc.name+" "); ok {
		return p
	}
	return strings.TrimPrefix(where, doc.name+".")
}

func (doc *sourceDocument) position(where string) sourcePosition {
	return findPosition(doc.positions, doc.logicalPath(where))
}

func (doc *sourceDocument) diagnostic(where, message string) error {
	if where == "" {
		where = "$"
	}
	return &diagnostic{doc.position(where), where, message, 0}
}

// yamlCode names a YAML error: a duplicate key, a value of the wrong type or an
// unknown field is the shape of a definition; anything else is the syntax.
func yamlCode(err error) diag.Code {
	var (
		dup     *yaml.DuplicateKeyError
		typ     *yaml.TypeError
		unknown *yaml.UnknownFieldError
		node    *yaml.UnexpectedNodeTypeError
		syntax  *yaml.SyntaxError
	)
	switch {
	case errors.As(err, &dup):
		return diag.Duplicate
	case errors.As(err, &typ), errors.As(err, &unknown), errors.As(err, &node):
		return diag.Shape
	case errors.As(err, &syntax):
		return diag.YAMLSyntax
	}
	return diag.Shape
}

// yamlError uses the original token of the type, expression or syntax, and does not guess a position by searching the text or re-parsing.
func (doc *sourceDocument) yamlError(err error) error {
	var ye yaml.Error
	if errors.As(err, &ye) {
		where := doc.tokenPaths[ye.GetToken()]
		if where == "" {
			where = "$"
		}
		if doc.name == "game.yaml" && where != "$" {
			where = "game.yaml " + where
		}
		return &diagnostic{nodePosition(doc.name, ye.GetToken()), where, ye.GetMessage(), yamlCode(err)}
	}
	return doc.diagnostic("$", err.Error())
}

// locateReported aligns the logical paths of existing version and numeric diagnostics to the same display form.
func (doc *sourceDocument) locateReported(err error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var errs []error
		for _, child := range joined.Unwrap() {
			errs = append(errs, doc.locateReported(child))
		}
		return joinDiagnostics(errs)
	}
	var existing *diagnostic
	if errors.As(err, &existing) {
		return err
	}
	where, message, ok := strings.Cut(err.Error(), ": ")
	if !ok {
		return doc.diagnostic("$", err.Error())
	}
	return doc.diagnostic(where, message)
}

func (db *DB) locateReported(err error) error {
	if doc := db.sourceFiles["game.yaml"]; doc != nil {
		return doc.locateReported(err)
	}
	return err
}

func (db *DB) diagnostic(where, message string) error {
	// Numeric checks during loading use paths with a file name; Check after merging uses paths from the definitions.
	var source *sourceDocument
	for name, doc := range db.sourceFiles {
		if where == name || strings.HasPrefix(where, name+".") || strings.HasPrefix(where, name+" ") {
			if source == nil || len(name) > len(source.name) {
				source = doc
			}
		}
	}
	if source != nil {
		return source.diagnostic(where, message)
	}
	return &diagnostic{findPosition(db.sources, where), where, message, 0}
}

// keepSources re-attaches only paths that change on merging, such as the serial numbers of composition.
func (db *DB) keepSources(doc *sourceDocument, from, to string) {
	for p, at := range doc.positions {
		if p == from || strings.HasPrefix(p, from+".") || strings.HasPrefix(p, from+"[") {
			db.sources[to+strings.TrimPrefix(p, from)] = at
		}
	}
}

// Diagnostic is one problem found in a game's YAML: where it is and what is wrong. The
// JSON form is what gozen check --format json prints (ADR 0021).
type Diagnostic struct {
	// File is the file under the game directory ("data/events.yaml"); empty when the
	// problem has no single place.
	File string `json:"file"`
	// Line and Col start at 1; 0 when unknown.
	Line int `json:"line"`
	Col  int `json:"col"`
	// Path is the logical path of the definition ("events.inn.do[2].battle").
	Path string `json:"path"`
	// Message says what is wrong, in English or in the game's language.
	Message string `json:"message"`
	// Severity is "error" (the game does not load) or "warning".
	Severity string `json:"severity"`
	// Code is the diagnostic code ("GZN1101", ADR 0024) for a tool or an agent to
	// branch on instead of the wording; empty when the problem has none yet.
	// gozen help <code> explains it.
	Code string `json:"code,omitempty"`
	err  error
}

// line is the diagnostic as one line: file:line:col: path: message.
func (d Diagnostic) line() string {
	if d.err != nil {
		return d.err.Error()
	}
	s := d.Message
	if d.Path != "" {
		s = d.Path + ": " + s
	}
	if d.File != "" {
		s = fmt.Sprintf("%s:%d:%d: %s", d.File, d.Line, d.Col, s)
	}
	if d.Code != "" {
		s += " [" + d.Code + "]"
	}
	return s
}

// Diagnostics are the problems of a game, in file and line order. Load returns them as
// its error; use errors.As to get them.
type Diagnostics []Diagnostic

// Error is one diagnostic a line.
func (ds Diagnostics) Error() string {
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = d.line()
	}
	return strings.Join(lines, "\n")
}

// Unwrap gives the errors the diagnostics came from (errors.Is still finds fs.ErrNotExist).
func (ds Diagnostics) Unwrap() []error {
	var out []error
	for _, d := range ds {
		if d.err != nil {
			out = append(out, d.err)
		}
	}
	return out
}

// Diagnose turns the errors of Load or the warnings of the checks into Diagnostics of
// the severity ("error" or "warning"). Errors without a place keep only the message.
func Diagnose(err error, severity string) Diagnostics {
	var flat []error
	var add func(error)
	add = func(err error) {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				add(child)
			}
		} else if err != nil {
			flat = append(flat, err)
		}
	}
	add(err)
	out := make(Diagnostics, 0, len(flat))
	for _, e := range flat {
		var d *diagnostic
		if errors.As(e, &d) && d.Error() == e.Error() {
			path := d.where
			for _, sep := range []string{" ", "."} { // "game.yaml fonts.regular" means fonts.regular of game.yaml
				path = strings.TrimPrefix(path, "game.yaml"+sep)
			}
			out = append(out, Diagnostic{File: d.position.file, Line: d.position.line, Col: d.position.column,
				Path: path, Message: d.message, Severity: severity, Code: d.code.String(), err: e})
			continue
		}
		msg := e.Error()
		var marked *diag.Error
		if errors.As(e, &marked) {
			msg = marked.Err.Error()
		}
		out = append(out, Diagnostic{Message: msg, Severity: severity, Code: diag.Of(e).String(), err: e})
	}
	return out
}
