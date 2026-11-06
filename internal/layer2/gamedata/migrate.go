package gamedata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	"github.com/nao1215/gozen/internal/layer1/optional"
	lang "github.com/nao1215/gozen/internal/text"
)

// Migrate prepares validated schema-11 YAML replacements for the authoring CLI.
// The source must be a valid versioned game (1 through 11). It is never mutated.
// Only changed definition files are returned; identity, data_version, assets,
// saves, text and command replay fingerprints are preserved. The caller copies
// the game before applying replacements and publishing a new directory.
// Legacy schema 0 remains loadable but needs a separate numeric-default upgrade.
func Migrate(fsys fs.FS) (map[string][]byte, error) {
	before, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	if before.Game.SchemaVersion == LegacySchemaVersion {
		return nil, errors.New(lang.T("schema_version: 0 は自動移行できません。数値０の既定値を確認して版１へ移行してください。元のゲームは変更していません"))
	}
	out := map[string][]byte{}
	for _, name := range slices.Sorted(maps.Keys(before.sourceFiles)) {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		typ := reflect.TypeFor[file]()
		if name == "game.yaml" {
			typ = reflect.TypeFor[Game]()
		}
		changed, err := migrateSource(b, typ, before.Game.SchemaVersion)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if !bytes.Equal(b, changed) {
			out[name] = changed
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	after, err := Load(migratedFiles{fsys, out})
	if err != nil {
		return nil, fmt.Errorf(lang.T("移行結果の検証に失敗しました。出力は公開していません: %w"), err)
	}
	a, err := migrationSignature(before)
	if err != nil {
		return nil, err
	}
	b, err := migrationSignature(after)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(a, b) {
		return nil, errors.New(lang.T("移行でゲームの定義が変わるため中止しました。元のゲームは変更していません"))
	}
	return out, nil
}

type migratedFiles struct {
	fs.FS
	replacements map[string][]byte
}

func (m migratedFiles) ReadFile(name string) ([]byte, error) {
	if b, ok := m.replacements[name]; ok {
		return b, nil
	}
	return fs.ReadFile(m.FS, name)
}

// Exported definition fields are the runtime contract. Source positions and
// decoder-presence flags are intentionally excluded by their JSON visibility.
func migrationSignature(db *DB) ([]byte, error) {
	normalized := *db
	normalized.FS = nil
	normalized.Game.SchemaVersion = SchemaVersion11
	return json.Marshal(&normalized)
}

type yamlEdit struct {
	start, end int
	text       string
}
type yamlMigration struct {
	source   string
	lines    []int
	offsets  map[*token.Token]int
	edits    []yamlEdit
	version  int
	keepLong bool
}

// The YAML printer does not round-trip every comment and multiline scalar.
// Edit source spans instead: leave unrelated text, quotes and comments intact.
func migrateSource(source []byte, typ reflect.Type, version int) ([]byte, error) {
	parsed, err := parser.ParseBytes(source, parser.ParseComments)
	keepLong := err != nil
	if keepLong {
		// The comment parser rejects some documents accepted by the loader.
		// Keep optional wrappers and explicit switches in those files; only
		// insert the version/defaults required to preserve their behavior.
		parsed, err = parser.ParseBytes(source, 0)
	}
	if err != nil {
		return nil, err
	}
	if len(parsed.Docs) == 0 || parsed.Docs[0].Body == nil {
		return source, nil
	}
	original, err := parser.ParseBytes(source, 0)
	if err != nil {
		return nil, err
	}
	if len(original.Docs) == 0 || original.Docs[0].Body == nil {
		return source, nil
	}
	resolveMerges(original.Docs[0].Body, map[string]ast.Node{})
	var raw any
	if err := decodeguard.Run(func() error { return yaml.NodeToValue(original.Docs[0].Body, &raw, yaml.UseOrderedMap()) }); err != nil {
		return nil, err
	}
	m := &yamlMigration{source: string(source), lines: []int{0}, offsets: map[*token.Token]int{}, version: version, keepLong: keepLong}
	for i, c := range source {
		if c == '\n' || c == '\r' && (i+1 == len(source) || source[i+1] != '\n') {
			m.lines = append(m.lines, i+1)
		}
	}
	first := parsed.Docs[0].Body.GetToken()
	for first.Prev != nil {
		first = first.Prev
	}
	pos := 0
	for tk := first; tk != nil; tk = tk.Next {
		// The decoder accepts prefixes such as 0<< as merge keys but drops the
		// prefix from their value and position. They cannot be safely edited.
		if tk.Type == token.MergeKeyType && strings.TrimSpace(tk.Origin) != tk.Value {
			return nil, errUnrestorable{}
		}
		if tk.Type == token.ImplicitNullType {
			m.offsets[tk] = pos
			continue // The parser supplies null for an empty authored value.
		}
		leading := true
		for _, r := range tk.Origin {
			got, size := utf8.DecodeRuneInString(m.source[pos:])
			if size == 0 || got != r {
				return nil, errUnrestorable{}
			}
			if leading && !strings.ContainsRune(" \t\r\n", r) {
				m.offsets[tk], leading = pos, false
			}
			pos += size
		}
		if leading {
			m.offsets[tk] = pos
		}
	}
	if err := m.walk(parsed.Docs[0].Body, typ, raw); err != nil {
		return nil, err
	}
	slices.SortFunc(m.edits, func(a, b yamlEdit) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	var result strings.Builder
	pos = 0
	for _, edit := range m.edits {
		if edit.start < pos || edit.end < edit.start || edit.end > len(source) {
			return nil, errors.New(lang.T("YAML の移行範囲が重なっています。元のゲームは変更していません"))
		}
		result.Write(source[pos:edit.start])
		result.WriteString(edit.text)
		pos = edit.end
	}
	result.Write(source[pos:])
	return []byte(result.String()), nil
}

func (m *yamlMigration) offset(t *token.Token) int {
	if offset, ok := m.offsets[t]; ok {
		return offset
	}
	return len(m.source)
}

func (m *yamlMigration) edit(start, end int, text string) {
	m.edits = append(m.edits, yamlEdit{start, end, text})
}

// line uses reconstructed source offsets: parser line numbers can drift after
// a comment followed by CRLF in a file with mixed line endings.
func (m *yamlMigration) line(t *token.Token) int {
	line, exact := slices.BinarySearch(m.lines, m.offset(t))
	if exact {
		line++
	}
	return line
}

func (m *yamlMigration) lineEnd(line int) int {
	if line >= len(m.lines) {
		return len(m.source)
	}
	return m.lines[line]
}

func (m *yamlMigration) walk(node ast.Node, typ reflect.Type, raw any) error {
	if node == nil {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch n := node.(type) {
	case *ast.AnchorNode:
		return m.walk(n.Value, typ, raw)
	case *ast.TagNode:
		return m.walk(n.Value, typ, raw)
	}
	if typ == reflect.TypeFor[Expr]() || typ == reflect.TypeFor[Color]() {
		return nil
	}
	if typ == reflect.TypeFor[AIRef]() {
		typ = reflect.TypeFor[[]Rule]()
	}
	if typ == reflect.TypeFor[Event]() {
		if _, ok := node.(*ast.SequenceNode); ok {
			typ = reflect.TypeFor[[]Command]()
		}
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		fields := map[string]reflect.Type{}
		if typ.Kind() == reflect.Struct {
			fields = definitionFields(typ)
		}
		mapping := optional.Value[yaml.MapSlice](raw)
		for _, entry := range n.Values {
			key := sourceKey(entry.Key)
			child, ok := fields[key]
			if typ.Kind() == reflect.Map {
				child, ok = typ.Elem(), true
			}
			if ok {
				value, _ := numberEntry(mapping, key)
				if err := m.walk(entry.Value, child, value); err != nil {
					return err
				}
			}
			if typ == reflect.TypeFor[Game]() && key == "schema_version" && m.version < SchemaVersion11 {
				value, ok := entry.Value.(*ast.IntegerNode)
				if !ok {
					return errors.New(lang.T("schema_version はアンカーを使わず整数で指定してから移行してください"))
				}
				start := m.offset(value.GetToken())
				m.edit(start, start+len(value.GetToken().Value), "11")
			}
		}
		if typ == reflect.TypeFor[TacticRow]() {
			m.tactic(n, mapping)
		}
		if !m.keepLong && typ == reflect.TypeFor[Event]() && len(n.Values) == 1 && sourceKey(n.Values[0].Key) == "do" && n.GetComment() == nil && n.Values[0].GetComment() == nil && n.Values[0].Key.GetComment() == nil && n.Values[0].FootComment == nil {
			if seq, ok := n.Values[0].Value.(*ast.SequenceNode); ok {
				m.event(n, seq)
			}
		}
		if !m.keepLong && typ == reflect.TypeFor[ItemCount]() && len(n.Values) == 1 && sourceKey(n.Values[0].Key) == "item" && !migrationComment(n, n.Values[0]) {
			// Keep explicit counts, including one, for saved replay fingerprints.
			if item, ok := n.Values[0].Value.(*ast.StringNode); ok {
				if n.IsFlowStyle {
					m.edit(m.offset(n.Start), m.offset(item.GetToken()), "")
					m.edit(m.offset(n.End), m.offset(n.End)+1, "")
				} else {
					m.edit(m.offset(n.Values[0].Key.GetToken()), m.offset(item.GetToken()), "")
				}
			}
		}
	case *ast.SequenceNode:
		if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			list := optional.Value[[]any](raw)
			for i, entry := range n.Values {
				var value any
				if i < len(list) {
					value = list[i]
				}
				if err := m.walk(entry, typ.Elem(), value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func migrationComment(n *ast.MappingNode, entry *ast.MappingValueNode) bool {
	return n.GetComment() != nil || entry.GetComment() != nil || entry.Key.GetComment() != nil || entry.Value.GetComment() != nil || entry.FootComment != nil
}

func (m *yamlMigration) event(n *ast.MappingNode, seq *ast.SequenceNode) {
	// Custom YAML tags can make the parser's closing-token columns ambiguous.
	// Retain the full event form rather than editing around tagged content.
	if len(ast.Filter(ast.TagType, seq)) > 0 {
		return
	}
	entry := n.Values[0]
	if n.IsFlowStyle {
		m.edit(m.offset(n.Start), m.offset(seq.Start), "")
		m.edit(m.offset(seq.End)+1, m.offset(n.End)+1, "")
		return
	}
	key := entry.Key.GetToken()
	keyLine, seqLine := m.line(key), m.line(seq.Start)
	if seq.IsFlowStyle {
		m.edit(m.offset(entry.Key.GetToken()), m.offset(seq.Start), "")
		return
	}
	// The wrapper occupies its own line. Preserve scalar text verbatim while
	// shifting ordinary list entries by the same number of spaces.
	if seqLine <= keyLine {
		return
	}
	if strings.TrimSpace(m.source[m.lines[keyLine-1]:m.offset(entry.Key.GetToken())]) != "" {
		return // The event's own separator shares this line with do.
	}
	// Tabs accepted between indentation and a list marker must keep their columns.
	// Removing leading spaces can turn that valid source into invalid indentation.
	last := m.lastLine(seq)
	for line := seqLine; line <= last && line <= len(m.lines); line++ {
		text := m.source[m.lines[line-1]:m.lineEnd(line)]
		indent := text[:len(text)-len(strings.TrimLeft(text, " \t"))]
		if strings.ContainsRune(indent, '\t') {
			return
		}
	}
	m.edit(m.lines[keyLine-1], m.lineEnd(keyLine), "")
	// Indentation inside multiline scalars is content. Keep its original columns
	// rather than relying on the parser's folded/literal token line conventions.
	if len(ast.Filter(ast.LiteralType, seq)) > 0 {
		return
	}
	for _, scalar := range ast.Filter(ast.StringType, seq) {
		if strings.ContainsAny(strings.TrimSpace(scalar.GetToken().Origin), "\r\n") {
			return
		}
	}
	shift := max(0, (m.offset(seq.Start)-m.lines[seqLine-1])-(m.offset(key)-m.lines[keyLine-1]))
	if shift == 0 {
		return
	}
	for line := seqLine; line <= last && line <= len(m.lines); line++ {
		start, end := m.lines[line-1], m.lineEnd(line)
		count := 0
		for start+count < end && count < shift && m.source[start+count] == ' ' {
			count++
		}
		if count == shift {
			m.edit(start, start+shift, "")
		}
	}
}

func (m *yamlMigration) lastLine(node ast.Node) int {
	if node == nil {
		return 0
	}

	origin := strings.TrimSpace(node.GetToken().Origin)
	last := m.line(node.GetToken()) + strings.Count(origin, "\n") + strings.Count(origin, "\r") - strings.Count(origin, "\r\n")
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, entry := range n.Values {
			last = max(last, m.lastLine(entry.Value))
		}
	case *ast.SequenceNode:
		for _, entry := range n.Values {
			last = max(last, m.lastLine(entry))
		}
	case *ast.AnchorNode:
		last = max(last, m.lastLine(n.Value))
	case *ast.TagNode:
		last = max(last, m.lastLine(n.Value))
	}
	return last
}

func (m *yamlMigration) tactic(n *ast.MappingNode, raw yaml.MapSlice) {
	merged, given := false, -1
	for i, entry := range n.Values {
		switch sourceKey(entry.Key) {
		case "<<":
			merged = true
		case "enabled":
			given = i
		}
	}
	if given < 0 && m.version < SchemaVersion11 {
		value, _ := numberEntry(raw, "enabled")
		enabled := optional.Value[bool](value)
		if !enabled {
			if n.IsFlowStyle {
				text := "enabled: false"
				if len(n.Values) > 0 {
					previous := m.offset(n.End) - 1
					for previous >= 0 && strings.ContainsRune(" \t\r\n", rune(m.source[previous])) {
						previous--
					}
					if previous < 0 || m.source[previous] != ',' {
						text = ", " + text
					}
				}
				m.edit(m.offset(n.End), m.offset(n.End), text)
			} else if len(n.Values) > 0 {
				first := n.Values[0].Key.GetToken()
				firstLine := m.line(first)
				line := m.source[m.lines[firstLine-1]:m.lineEnd(firstLine)]
				ending := "\n"
				if strings.HasSuffix(line, "\r\n") {
					ending = "\r\n"
				} else if strings.HasSuffix(line, "\r") {
					ending = "\r"
				}
				column := utf8.RuneCountInString(m.source[m.lines[firstLine-1]:m.offset(first)])
				m.edit(m.offset(first), m.offset(first), "enabled: false"+ending+strings.Repeat(" ", column))
			}
		}
	}
	if given < 0 || merged || m.keepLong {
		return
	}
	entry := n.Values[given]
	value, ok := entry.Value.(*ast.BoolNode)
	if !ok || !value.Value || migrationComment(n, entry) {
		return
	}
	start, end := m.offset(entry.Key.GetToken()), m.offset(value.GetToken())+len(value.GetToken().Value)
	if n.IsFlowStyle {
		if given+1 < len(n.Values) {
			end = m.offset(n.Values[given+1].Key.GetToken())
		} else if given > 0 {
			// The separator before the last field belongs to the removed field.
			for start > m.offset(n.Start) && m.source[start-1] != ',' {
				start--
			}
			start--
		}
	} else {
		line := m.line(entry.Key.GetToken())
		prefix := m.source[m.lines[line-1]:start]
		if strings.TrimSpace(prefix) == "" {
			start = m.lines[line-1]
			end = m.lineEnd(line)
		}
	}
	m.edit(start, end, "")
}

// errUnrestorable is the refusal to migrate a file whose tokens do not give back
// its source text: nothing is written, so the game stays as it was.
type errUnrestorable struct{}

func (errUnrestorable) Error() string {
	return lang.T("YAML の元の文字列を復元できないため移行を中止しました。元のゲームは変更していません")
}
