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
)

// sourcePosition は読み込んだノードの位置。既定値の補完では変更しない。
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
}

func (d *diagnostic) Error() string {
	if d.position.file == "" {
		return d.where + ": " + d.message
	}
	return fmt.Sprintf("%s: %s: %s", d.position, d.where, d.message)
}

func duplicateDiagnostic(at sourcePosition, where string, previous sourcePosition) error {
	return &diagnostic{at, where, fmt.Sprintf("%s is defined twice (previous definition at %s)", where, previous)}
}

// sortedDiagnostics はマップの走査順に依存させず、位置・論理パス・本文の順に並べる。
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
	// 空ファイルや欠けた項目は文書の先頭、または最も近い親へ案内する。
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
			// マージ元を先に登録し、記述順にかかわらず明示した項目の出典を優先する。
			for _, entry := range n.Values {
				if sourceKey(entry.Key) == "<<" {
					doc.tokenPaths[entry.Value.GetToken()] = numberPath(where, "<<")
					if seq, ok := entry.Value.(*ast.SequenceNode); ok {
						// YAML のマージ列は先頭のマップが優先される。
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
	return &diagnostic{doc.position(where), where, message}
}

// yamlError は型・式・構文の元トークンを使い、本文の検索や再解析で位置を推測しない。
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
		return &diagnostic{nodePosition(doc.name, ye.GetToken()), where, ye.GetMessage()}
	}
	return doc.diagnostic("$", err.Error())
}

// locateReported は既存の版・数値診断の論理パスも同じ表示形式へ揃える。
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
	// 読込中の数値検査はファイル名付きのパス、統合後の Check は定義からのパス。
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
	return &diagnostic{findPosition(db.sources, where), where, message}
}

// keepSources は合成の通し番号など、統合で変わるパスだけを付け替える。
func (db *DB) keepSources(doc *sourceDocument, from, to string) {
	for p, at := range doc.positions {
		if p == from || strings.HasPrefix(p, from+".") || strings.HasPrefix(p, from+"[") {
			db.sources[to+strings.TrimPrefix(p, from)] = at
		}
	}
}
