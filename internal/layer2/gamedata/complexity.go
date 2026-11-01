package gamedata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"

	"github.com/nao1215/gozen/internal/diag"
)

// ComplexityConfig controls authoring warnings consumed by checkdata. Zero uses
// the defaults; a negative limit disables that metric. Inputs are not mutated.
// Counts warn above the limit, except repetitions and unused definitions, which
// warn at the limit. These are review hints, never runtime validity rules.
type ComplexityConfig struct {
	FileLines         int `yaml:"file_lines"`
	BlockLines        int `yaml:"block_lines"`
	Repetitions       int `yaml:"repetitions"`
	MinBlockNodes     int `yaml:"min_block_nodes"`
	ExpressionLength  int `yaml:"expression_length"`
	ExpressionDepth   int `yaml:"expression_depth"`
	ScreenOperations  int `yaml:"screen_operations"`
	IndexedNames      int `yaml:"indexed_names"`
	UnusedDefinitions int `yaml:"unused_definitions"`
}

func (c ComplexityConfig) defaults() ComplexityConfig {
	for _, v := range []struct {
		target *int
		value  int
	}{
		{&c.FileLines, 5000},
		{&c.BlockLines, 500},
		{&c.Repetitions, 8},
		{&c.MinBlockNodes, 40},
		{&c.ExpressionLength, 240},
		{&c.ExpressionDepth, 8},
		{&c.ScreenOperations, 40},
		{&c.IndexedNames, 8},
		{&c.UnusedDefinitions, 8},
	} {
		if *v.target == 0 {
			*v.target = v.value
		}
	}
	return c
}

type complexityBlock struct {
	doc                 *sourceDocument
	path, name, section string
	shape               [32]byte
	nodes               int
}
type complexityScan struct {
	cfg      ComplexityConfig
	warnings []error
	blocks   []complexityBlock
	values   map[string]bool
}

// ComplexityWarnings rereads the sources identified by Load, without
// expansion or state changes. It returns deterministic positioned warnings for
// the CLI, or an I/O/parse error. The caller must keep source files unchanged
// between Load and this call. Results are not proof of dead code or a runtime
// cost estimate. Aliases are counted
// where written and are not expanded. Only events, screens and planning rules
// participate in duplicate and unused-definition checks.
func (db *DB) ComplexityWarnings(cfg ComplexityConfig) ([]error, error) {
	s := complexityScan{cfg: cfg.defaults(), values: map[string]bool{}}
	for _, name := range slices.Sorted(maps.Keys(db.sourceFiles)) {
		b, err := fs.ReadFile(db.FS, name)
		if err != nil {
			return nil, err
		}
		source := &DB{sourceFiles: map[string]*sourceDocument{}}
		doc, err := source.readSource(b, name)
		if err != nil {
			return nil, err
		}
		lines := strings.Count(string(b), "\n")
		if len(b) > 0 && b[len(b)-1] != '\n' {
			lines++
		}
		typ := reflect.TypeFor[file]()
		if name == "game.yaml" {
			typ = reflect.TypeFor[Game]()
		}
		s.walk(doc, doc.root, "", typ)
		s.limit(doc, "", "file lines", lines, s.cfg.FileLines, "separate content from repeated rules; splitting files alone does not remove duplication")
		doc.root, doc.tokenPaths, doc.bareKeys = nil, nil, nil
	}
	s.definitions(db)
	return sortedDiagnostics(s.warnings), nil
}

func (s *complexityScan) warn(doc *sourceDocument, p, message string) {
	s.warnings = append(s.warnings, coded(doc.diagnostic(p, message), diag.YAMLComplexity))
}

func (s *complexityScan) limit(doc *sourceDocument, p, metric string, got, limit int, fix string) {
	if limit > 0 && got > limit {
		s.warn(doc, p, fmt.Sprintf("%s = %d (limit %d); %s", metric, got, limit, fix))
	}
}

var indexedName = regexp.MustCompile(`\d+`)

// walk follows schema types, so long prose and labels are not expressions.
func (s *complexityScan) walk(doc *sourceDocument, node ast.Node, p string, typ reflect.Type) {
	if node == nil {
		return
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeFor[Expr]() {
		value := node.GetToken().Value
		if literal, ok := node.(*ast.LiteralNode); ok && literal.Value != nil {
			value = literal.Value.Value
		}
		s.limit(doc, p, "expression characters", utf8.RuneCountInString(value), s.cfg.ExpressionLength, "name intermediate values or move repeated calculations into a rule")
		s.limit(doc, p, "expression nesting", expressionNesting(value), s.cfg.ExpressionDepth, "split nested calculations into named steps")
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		families := map[string][]string{}
		for _, e := range n.Values {
			key := sourceKey(e.Key)
			child := numberPath(p, key)
			if indexedName.MatchString(key) {
				family := indexedName.ReplaceAllString(key, "#")
				families[family] = append(families[family], child)
			}
			s.walk(doc, e.Value, child, complexityField(typ, key))
			if p == "events" || p == "screens" || p == "planning.rules" {
				shape, count, last, operations := complexityShape(e.Value)
				s.blocks = append(s.blocks, complexityBlock{doc, child, key, p, shape, count})
				s.limit(doc, child, "block lines", last-e.Key.GetToken().Position.Line+1, s.cfg.BlockLines, "extract shared behavior and keep each event or screen focused")
				if p == "screens" {
					s.limit(doc, child, "screen set/plan commands", operations, s.cfg.ScreenOperations, "call a shared event or planning rule from the screen")
				}
			}
		}
		for _, family := range slices.Sorted(maps.Keys(families)) {
			paths := families[family]
			if s.cfg.IndexedNames > 0 && len(paths) >= s.cfg.IndexedNames {
				s.warn(doc, paths[0], fmt.Sprintf("indexed names %q = %d (limit %d); review date/ID copies and use collection data or shared behavior", family, len(paths), s.cfg.IndexedNames))
			}
		}
	case *ast.SequenceNode:
		var elem reflect.Type
		if typ != nil && (typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array) {
			elem = typ.Elem()
		}
		for i, v := range n.Values {
			s.walk(doc, v, fmt.Sprintf("%s[%d]", p, i), elem)
		}
	case *ast.AnchorNode:
		s.walk(doc, n.Value, p, typ)
	case *ast.TagNode:
		s.walk(doc, n.Value, p, typ)
	case *ast.AliasNode:
		// Authored aliases do not multiply cognitive load or recurse through cycles.
	default:
		s.values[node.GetToken().Value] = true
	}
}

func complexityField(typ reflect.Type, key string) reflect.Type {
	if typ == nil {
		return nil
	}
	if typ == reflect.TypeFor[Assignments]() {
		if !LiteralSet(key) {
			return reflect.TypeFor[Expr]()
		}
		return nil
	}
	switch typ.Kind() { //nolint:exhaustive // Only schema maps and structs have fields.
	case reflect.Struct:
		return definitionFields(typ)[key]
	case reflect.Map:
		return typ.Elem()
	}
	return nil
}

func expressionNesting(value string) int {
	depth, peak := 0, 0
	var quote rune
	escaped := false
	for _, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			switch r {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(', '[':
			depth++
			peak = max(peak, depth)
		case ')', ']':
			depth--
		}
	}
	return peak
}

// Hash structural keys and scalar kinds, retaining sequence order and sorting
// map entries. Literal values vary between copies. Size excludes map keys.
func complexityShape(node ast.Node) (shape [32]byte, nodes, last, operations int) {
	if node == nil {
		return shape, nodes, last, operations
	}
	if literal, ok := node.(*ast.LiteralNode); ok && literal.Value != nil {
		shape, nodes, _, operations = complexityShape(literal.Value)
		// The parser positions a block scalar's value at its final line.
		return shape, nodes, literal.Value.GetToken().Position.Line, operations
	}
	nodes = 1
	last = node.GetToken().Position.Line + strings.Count(strings.TrimRight(node.GetToken().Origin, "\r\n"), "\n")
	var parts []string
	add := func(v ast.Node) {
		h, count, line, ops := complexityShape(v)
		nodes += count
		last = max(last, line)
		operations += ops
		parts = append(parts, hex.EncodeToString(h[:]))
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			key := sourceKey(e.Key)
			if key == "set" || key == "plan" {
				operations++
			}
			add(e.Value)
			parts[len(parts)-1] = strconv.Quote(key) + ":" + parts[len(parts)-1]
		}
		slices.Sort(parts)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			add(v)
		}
	case *ast.AnchorNode:
		add(n.Value)
	case *ast.TagNode:
		add(n.Value)
	}
	shape = sha256.Sum256([]byte(fmt.Sprintf("%T:%s", node, strings.Join(parts, ";"))))
	return shape, nodes, last, operations
}

func (s *complexityScan) definitions(db *DB) {
	groups := map[string][]complexityBlock{}
	unused := map[string][]complexityBlock{}
	for _, b := range s.blocks {
		if s.cfg.MinBlockNodes > 0 && b.nodes >= s.cfg.MinBlockNodes {
			key := fmt.Sprintf("%s/%x", b.section, b.shape)
			groups[key] = append(groups[key], b)
		}
		if s.values[b.name] {
			continue
		}
		if b.section == "events" && db.Events[b.name] != nil && db.Events[b.name].Trigger != "" {
			continue
		}
		unused[b.section] = append(unused[b.section], b)
	}
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		blocks := groups[key]
		if s.cfg.Repetitions > 0 && len(blocks) >= s.cfg.Repetitions {
			b := blocks[0]
			s.warn(b.doc, b.path, fmt.Sprintf("same-shaped %s blocks = %d (limit %d, %d nodes each); consider shared events, rules, templates, draw tables or iteration; review differing values before merging", b.section, len(blocks), s.cfg.Repetitions, b.nodes))
		}
	}
	for _, section := range slices.Sorted(maps.Keys(unused)) {
		blocks := unused[section]
		if s.cfg.UnusedDefinitions > 0 && len(blocks) >= s.cfg.UnusedDefinitions {
			names := make([]string, 0, len(blocks))
			for _, b := range blocks {
				names = append(names, b.name)
			}
			b := blocks[0]
			s.warn(b.doc, b.path, fmt.Sprintf("possibly unreferenced %s definitions = %d (limit %d): %s; connect consumers or remove obsolete definitions; external script entry points may be intentional", section, len(blocks), s.cfg.UnusedDefinitions, strings.Join(names, ", ")))
		}
	}
}
