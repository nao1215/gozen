package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/nao1215/gozen/internal/cliio"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/play"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

func (c *command) layout(dir string, out io.Writer) error {
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	if err := gamedata.UseWords(fsys); err != nil {
		return err
	}
	db, err := gamedata.LoadDir(fsys, ".")
	if err != nil {
		return err
	}
	if err := layoutMessages(db, fsys, c.stderr); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, lang.T("レイアウト検査済み。文字の収まりは警告として報告します。"))
	return err
}

func layoutMessages(db *gamedata.DB, fsys fs.FS, out io.Writer) error {
	issues, err := checkMessages(db, fsys)
	if err != nil {
		return err
	}
	w, err := builtinFontWarning(db, fsys)
	if err != nil {
		return err
	}
	if w != "" {
		if _, err := fmt.Fprintln(out, "warning: game.yaml: "+w); err != nil {
			return err
		}
	}
	return reportMessages(out, issues)
}

// builtinFontWarning says which characters of the game's text the built-in Go
// Regular font cannot draw, when game.yaml has no fonts.regular; "" otherwise.
func builtinFontWarning(db *gamedata.DB, fsys fs.FS) (string, error) {
	if db.Game.Fonts.Regular != "" {
		return "", nil
	}
	missing, err := missingGlyphs(fsys)
	if err != nil {
		return "", err
	}
	if len(missing) == 0 {
		if db.Game.Language == "ja" {
			return "fonts.regular is not set, and the built-in Go Regular font has Latin letters only: it cannot draw the engine's screens in language ja; set fonts.regular to a font under assets/ with Japanese", nil
		}
		return "", nil
	}
	shown := missing[:min(len(missing), 20)]
	list := make([]string, 0, len(shown))
	for _, r := range shown {
		list = append(list, string(r))
	}
	more := ""
	if len(missing) > len(shown) {
		more = fmt.Sprintf(" and %d more", len(missing)-len(shown))
	}
	return fmt.Sprintf("fonts.regular is not set, and the built-in Go Regular font has no glyph for %d characters of the text: %s%s; set fonts.regular to a font under assets/ that has them",
		len(missing), strings.Join(list, " "), more), nil
}

// missingGlyphs lists, in order, the characters of the strings of game.yaml and
// data/ that Go Regular has no glyph for.
func missingGlyphs(fsys fs.FS) ([]rune, error) {
	font, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	var buf sfnt.Buffer
	seen := map[rune]bool{}
	var missing []rune
	visit := stringVisitor(func(s string) {
		for _, r := range s {
			if seen[r] || r < 0x20 || unicode.IsSpace(r) {
				continue
			}
			seen[r] = true
			if i, err := font.GlyphIndex(&buf, r); err != nil || i == 0 {
				missing = append(missing, r)
			}
		}
	})
	files := []string{"game.yaml"}
	for _, dir := range []string{"data", gamedata.LangDir} { // the translations are drawn too (ADR 0027)
		err = fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && path.Ext(p) == ".yaml" {
				files = append(files, p)
			}
			return err
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	for _, name := range files {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseBytes(b, 0)
		if err != nil {
			return nil, err
		}
		for _, doc := range f.Docs {
			ast.Walk(visit, doc)
		}
	}
	slices.Sort(missing)
	return missing, nil
}

// stringVisitor calls itself with every string scalar of a YAML document that
// is a value. Keys are names, not text (the keys of text: are the engine's own
// Japanese source text).
type stringVisitor func(string)

func (v stringVisitor) Visit(n ast.Node) ast.Visitor {
	switch x := n.(type) {
	case *ast.MappingValueNode:
		ast.Walk(v, x.Value)
		return nil
	case *ast.StringNode:
		v(x.Value)
	case *ast.LiteralNode:
		if x.Value != nil {
			v(x.Value.Value)
		}
	}
	return v
}

// fontProblem is a font of game.yaml that the layout check cannot use, with
// the place of its field in game.yaml (line 0 when the field is not written).
type fontProblem struct {
	line, col int
	key       string
	err       error
}

func (p *fontProblem) Error() string {
	if p.line > 0 {
		return fmt.Sprintf("game.yaml:%d:%d: %s: %v", p.line, p.col, p.key, p.err)
	}
	return fmt.Sprintf("game.yaml: %s: %v", p.key, p.err)
}

func (p *fontProblem) Unwrap() error { return p.err }

// fontProblemOf gives a font error of play.LoadFonts the place of its field
// in game.yaml and the code GZN1201.
func fontProblemOf(fsys fs.FS, err error) error {
	var fe *play.FontError
	if !errors.As(err, &fe) {
		return diag.FontUnreadable.Wrap(err)
	}
	p := &fontProblem{key: fe.Key, err: fe.Err}
	if b, rerr := fs.ReadFile(fsys, "game.yaml"); rerr == nil {
		if f, perr := parser.ParseBytes(b, 0); perr == nil {
			for _, q := range []string{"$." + fe.Key, "$.fonts"} {
				yp, yerr := yaml.PathString(q)
				if yerr != nil {
					continue
				}
				if n, ferr := yp.FilterFile(f); ferr == nil && n != nil {
					tk := n.GetToken()
					p.line, p.col = tk.Position.Line, tk.Position.Column
					break
				}
			}
		}
	}
	return diag.FontUnreadable.Wrap(p)
}

type msgIssue struct {
	file  string
	line  int
	lines int
	kind  string
	text  string
}

var embedRe = regexp.MustCompile(`\{[^{}]*\}`)

func checkMessages(db *gamedata.DB, fsys fs.FS) ([]msgIssue, error) {
	if err := play.LoadFonts(db, fsys); err != nil {
		return nil, fontProblemOf(fsys, err)
	}
	// Use the game-local translator through loaded gamedata. The lines are laid out in the
	// language written and in each other language offered, translated (ADR 0027).
	var out []msgIssue
	var err error
	codes := []string{db.Game.Written()}
	for _, code := range db.Game.Offered() {
		if !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	for _, code := range codes {
		var more []msgIssue
		if more, err = languageMessages(db, fsys, code); err != nil {
			break
		}
		out = append(out, more...)
	}
	if err == nil {
		out = append(out, stoneOfferIssues(db)...)
		out = append(out, spOfferIssues(db)...)
	}
	seen := map[string]bool{}

	uniq := out[:0]
	for _, m := range out {
		k := fmt.Sprintf("%s:%d:%s", m.file, m.line, m.text)
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, m)
		}
	}
	out = uniq
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].line < out[j].line
	})
	return out, err
}

// languageMessages lays out the lines of the data files in a language: the language
// written as it is, another translated by lang/<code>.yaml (its file is tagged with it).
func languageMessages(db *gamedata.DB, fsys fs.FS, code string) ([]msgIssue, error) {
	tr := func(s string) string { return s }
	tag := ""
	if code != db.Game.Written() {
		tr = func(s string) string { return db.TrIn(code, s) }
		tag = " [" + code + "]"
	}
	var out []msgIssue
	err := fs.WalkDir(fsys, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".yaml" {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f, err := parser.ParseBytes(b, 0)
		if err != nil {
			return err
		}
		for _, doc := range f.Docs {
			ast.Walk(msgVisitor(func(n ast.Node) {
				out = append(out, inspectMapping(&db.Game, p+tag, n, tr)...)
			}), doc)
		}
		return nil
	})
	return out, err
}

type msgVisitor func(ast.Node)

func (v msgVisitor) Visit(n ast.Node) ast.Visitor {
	switch n.(type) {
	case *ast.MappingNode, *ast.MappingValueNode:
		v(n)
	}
	return v
}

func pairs(n ast.Node) []*ast.MappingValueNode {
	switch m := n.(type) {
	case *ast.MappingNode:
		return m.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{m}
	}
	return nil
}

func scalar(n ast.Node) (string, int, bool) {
	switch s := n.(type) {
	case *ast.StringNode:
		return s.Value, s.GetToken().Position.Line, true
	case *ast.LiteralNode:
		return s.Value.Value, s.GetToken().Position.Line, true
	}
	return "", 0, false
}

func inspectMapping(gm *gamedata.Game, file string, n ast.Node, tr func(string) string) []msgIssue {
	var out []msgIssue
	var prompt ast.Node
	var options []string
	for _, kv := range pairs(n) {
		key := kv.Key.String()
		switch key {
		case "say":
			text, line, ok := scalar(kv.Value)
			if !ok {
				for _, w := range pairs(kv.Value) {
					if w.Key.String() == "text" {
						text, line, ok = scalar(w.Value)
					}
				}
			}
			if ok {
				out = append(out, sayIssue(gm, file, line, tr(text))...)
			}
		case "prompt":
			prompt = kv.Value
		case "choice":
			if seq, ok := kv.Value.(*ast.SequenceNode); ok {
				for _, it := range seq.Values {
					for _, w := range pairs(it) {
						if w.Key.String() == "label" {
							if s, _, ok := scalar(w.Value); ok {
								options = append(options, embedRe.ReplaceAllString(tr(s), "〇〇〇"))
							}
						}
					}
				}
			}
		}
	}
	if prompt != nil {
		if text, line, ok := scalar(prompt); ok {
			text = embedRe.ReplaceAllString(tr(text), "〇〇〇")
			n := len(play.MessageWrap(gm, text))
			if _, _, cut := play.PromptLines(gm, text, options); cut {
				out = append(out, msgIssue{file, line, n, "cut", text})
			} else if n > play.MessageLines() {
				out = append(out, msgIssue{file, line, n, "shrink", text})
			}
		}
	}
	return out
}

func sayIssue(gm *gamedata.Game, file string, line int, text string) []msgIssue {
	text = embedRe.ReplaceAllString(text, "〇〇〇")
	n := len(play.MessageWrap(gm, text))
	if n <= play.MessageLines() {
		return nil
	}
	kind := "shrink"
	if pages, _ := play.MessagePages(gm, text); len(pages) > 1 {
		kind = "split"
	}
	return []msgIssue{{file, line, n, kind, text}}
}

func reportMessages(writer io.Writer, issues []msgIssue) error {
	out := &cliio.Output{Writer: writer}
	if len(issues) == 0 {
		return nil
	}
	count := map[string]int{}
	byFile := map[string]int{}
	for _, m := range issues {
		count[m.kind]++
		byFile[m.file]++
	}
	summary := fmt.Sprintf(lang.T("warning: 台詞の窓の %d 行を超える文が %d（字を小さくして収めた %d、頁を分けた %d、選択肢の問いの末尾が切れた %d）\n"),
		play.MessageLines(), len(issues), count["shrink"], count["split"], count["cut"])
	out.Println(diag.TextOverflow.Mark(strings.TrimSuffix(summary, "\n")))
	var files []string
	for f := range byFile {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		return byFile[files[i]] > byFile[files[j]] || byFile[files[i]] == byFile[files[j]] && files[i] < files[j]
	})
	var fs []string
	for _, f := range files {
		fs = append(fs, fmt.Sprintf("%s %d", f, byFile[f]))
	}
	out.Printf(lang.T("  ファイルごと: %s\n"), strings.Join(fs, lang.T("、")))

	rank := map[string]int{"cut": 0, "split": 1, "shrink": 2}
	sorted := append([]msgIssue{}, issues...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if rank[sorted[i].kind] != rank[sorted[j].kind] {
			return rank[sorted[i].kind] < rank[sorted[j].kind]
		}
		return sorted[i].lines > sorted[j].lines
	})
	const top = 20
	for i, m := range sorted {
		if i == top {
			out.Printf(lang.T("  ほか %d\n"), len(sorted)-top)
			break
		}
		r := []rune(m.text)
		if len(r) > 30 {
			r = append(r[:30], '…')
		}
		out.Printf(lang.T("  %s:%d %s %d行 %s\n"), m.file, m.line, m.kind, m.lines, strings.ReplaceAll(string(r), "\n", lang.T("／")))
	}
	return out.Err
}

func stoneOfferIssues(db *gamedata.DB) []msgIssue {
	if len(db.StoneSets) == 0 {
		return nil
	}
	size := 28.0
	kinds := map[string]bool{}
	for _, set := range db.StoneSets {
		for _, st := range set.Stones {
			if it := db.Items[st]; it != nil && it.Slot != "" {
				kinds[it.Slot] = true
			}
		}
	}
	slots := 0
	for _, sl := range db.Game.Slots {
		for k := range kinds {
			if (&gamedata.Item{Slot: k}).Fits(sl.ID) {
				slots++
				break
			}
		}
	}
	var names []string
	for _, it := range db.Items {
		if kinds[it.Slot] {
			names = append(names, it.ShortName())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		wi, wj := gfx.TextWidth(names[i], size), gfx.TextWidth(names[j], size)
		return wi > wj || wi == wj && names[i] < names[j]
	})
	worst := strings.Join(names[:min(slots, len(names))], lang.T("・"))
	var out []msgIssue
	var ids []string
	for id := range db.Actors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {

		type cand struct {
			pitch  string
			stones int
		}
		var cands []cand
		for _, set := range db.StoneSets {
			if set.Pitch == "" || len(set.Stones) == 0 || set.Only && !slices.Contains(set.For, id) {
				continue
			}
			i := slices.IndexFunc(cands, func(c cand) bool { return c.pitch == set.Pitch })
			if i < 0 {
				cands = append(cands, cand{set.Pitch, len(set.Stones)})
			} else {
				cands[i].stones = min(cands[i].stones, len(set.Stones))
			}
		}
		longest := ""
		for mask := range 1 << min(len(cands), 16) {
			n, text := 0, ""
			for i, c := range cands {
				if mask&(1<<i) != 0 {
					n += c.stones
					text += c.pitch
				}
			}
			if n <= slots && gfx.TextWidth(text, size) > gfx.TextWidth(longest, size) {
				longest = text
			}
		}
		if longest == "" {
			longest = lang.T("空いた枠には、強い石を回しておく。")
		}
		text := fmt.Sprintf(lang.T("%s　%s → %s"), db.Actors[id].Name, worst, worst) + "\n" + longest
		out = append(out, sayIssue(&db.Game, fmt.Sprintf(lang.T("（石屋の勧め %s）"), id), 0, text)...)
	}
	return out
}

func spOfferIssues(db *gamedata.DB) []msgIssue {
	var out []msgIssue
	var ids []string
	for id := range db.Actors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := rpg.NewActor(db, id)
		c := a.Class()
		if c == nil || len(c.Tree) == 0 {
			continue
		}
		worst, lines := "", 0
		for lv := a.Level; lv <= max(a.Level, c.Max); lv++ {
			b := rpg.NewActor(db, id)
			b.GainExp(b.ExpFor(lv) - b.Exp)
			b.ResetTree()
			plan := b.PlanSP()
			if len(plan) == 0 {
				continue
			}
			text := event.SPPage(db, b, plan)
			if n := len(play.MessageWrap(&db.Game, text)); n > lines {
				worst, lines = text, n
			}
		}
		if worst != "" {
			out = append(out, sayIssue(&db.Game, fmt.Sprintf(lang.T("（修練所の勧め %s）"), id), 0, worst)...)
		}
	}
	return out
}
