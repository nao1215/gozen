package docs

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen"
)

// The documents whose steps are checked: README.md, the website's pages and the skill.
func stepDocuments(t testing.TB) []string {
	t.Helper()
	pages, err := filepath.Glob(filepath.Join(root, "website", "content", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs := []string{"README.md"}
	for _, p := range pages {
		docs = append(docs, path.Join("website/content", filepath.Base(p)))
	}
	// the skill for coding agents (skills/gozen): its commands are run the same way
	skill, err := filepath.Glob(filepath.Join(root, "skills", "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	refs, err := filepath.Glob(filepath.Join(root, "skills", "*", "references", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(skill, refs...) {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, filepath.ToSlash(rel))
	}
	return docs
}

// block is one fenced code block of a document.
type block struct {
	doc   string
	line  int // the line of the opening fence, 1-based
	lang  string
	body  string
	mark  *mark
	where string
}

// mark is a <!-- verify: KIND [key=value ...] [reason] --> comment just above a block.
type mark struct {
	kind   string
	opts   map[string]string
	reason string
	line   int
}

var (
	fenceOpen = regexp.MustCompile("^(\\s*)(```+|~~~+)\\s*([A-Za-z0-9_+-]*)")
	markLine  = regexp.MustCompile(`^\s*<!--\s*verify:\s*(.*?)\s*-->\s*$`)
)

// Languages that hold steps: every block in them is run unless marked.
var shellLangs = map[string]bool{"sh": true, "shell": true, "bash": true, "console": true, "zsh": true}

func isYAML(lang string) bool { return lang == "yaml" || lang == "yml" }

// The kinds of mark, and the languages each applies to ("" is any).
var markKinds = map[string]string{
	"skip":   "shell yaml go",
	"run":    "shell",
	"each":   "shell",
	"build":  "shell",
	"file":   "",
	"load":   "yaml",
	"same":   "yaml",
	"parse":  "yaml",
	"output": "text",
}

// parseBlocks reads the fenced blocks and their marks. It reports marks that
// are malformed or that no block follows.
func parseBlocks(doc string, src []byte) ([]block, []string) {
	lines := strings.Split(string(src), "\n")
	var blocks []block
	var bad []string
	var pending *mark
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if m := markLine.FindStringSubmatch(line); m != nil {
			if pending != nil {
				bad = append(bad, fmt.Sprintf("%s:%d: a verify mark with no code block after it", doc, pending.line))
			}
			mk, err := parseMark(m[1])
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s:%d: %v", doc, i+1, err))
				pending = nil
				continue
			}
			mk.line = i + 1
			pending = &mk
			continue
		}
		m := fenceOpen.FindStringSubmatch(line)
		if m == nil {
			if strings.TrimSpace(line) != "" && pending != nil {
				bad = append(bad, fmt.Sprintf("%s:%d: a verify mark with no code block after it", doc, pending.line))
				pending = nil
			}
			continue
		}
		indent, fence, lang := m[1], m[2], strings.ToLower(m[3])
		var body []string
		j := i + 1
		for ; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), fence) && strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[j]), fence[:1])) == "" {
				break
			}
			body = append(body, strings.TrimPrefix(lines[j], indent))
		}
		b := block{doc: doc, line: i + 1, lang: lang, body: strings.Join(body, "\n") + "\n", mark: pending}
		b.where = fmt.Sprintf("%s:%d", doc, b.line)
		blocks = append(blocks, b)
		pending = nil
		i = j
	}
	if pending != nil {
		bad = append(bad, fmt.Sprintf("%s:%d: a verify mark with no code block after it", doc, pending.line))
	}
	return blocks, bad
}

func parseMark(s string) (mark, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return mark{}, errors.New("an empty verify mark")
	}
	mk := mark{kind: fields[0], opts: map[string]string{}}
	if _, ok := markKinds[mk.kind]; !ok {
		return mk, fmt.Errorf("unknown verify mark %q (want one of skip, run, each, build, file, load, same, parse, output)", mk.kind)
	}
	rest := fields[1:]
	for len(rest) > 0 {
		k, v, ok := strings.Cut(rest[0], "=")
		if !ok || !slices.Contains([]string{"fixture", "except", "under", "group", "as"}, k) {
			break
		}
		mk.opts[k] = v
		rest = rest[1:]
	}
	switch mk.kind {
	case "file", "same":
		if len(rest) != 1 {
			return mk, fmt.Errorf("verify: %s takes one path", mk.kind)
		}
		mk.opts["path"] = rest[0]
		rest = nil
	}
	mk.reason = strings.Join(rest, " ")
	needsReason := mk.kind == "skip" || mk.kind == "build" || mk.kind == "parse" || mk.opts["except"] != ""
	if needsReason && len([]rune(mk.reason)) < 8 {
		return mk, fmt.Errorf("verify: %s needs a reason: why the step cannot be run as written", mk.kind)
	}
	if !needsReason && mk.reason != "" {
		return mk, fmt.Errorf("verify: %s takes no reason, found %q", mk.kind, mk.reason)
	}
	if e := mk.opts["except"]; e != "" {
		for n := range strings.SplitSeq(e, ",") {
			if v, err := strconv.Atoi(n); err != nil || v < 1 {
				return mk, fmt.Errorf("verify: except=%s: want command numbers such as 2,3", e)
			}
		}
		if mk.kind != "each" && mk.kind != "run" {
			return mk, errors.New("verify: except works with run and each")
		}
	}
	return mk, nil
}

// checkMarks reports blocks whose status is missing or does not fit.
func checkMarks(blocks []block) []string {
	var bad []string
	var lastShell *block
	for i := range blocks {
		b := &blocks[i]
		lang := b.lang
		kindLang := "text"
		switch {
		case shellLangs[lang]:
			kindLang = "shell"
		case isYAML(lang):
			kindLang = "yaml"
		case lang == "go":
			kindLang = "go"
		}
		if b.mark == nil {
			if kindLang == "yaml" {
				bad = append(bad, b.where+": a YAML example needs a verify mark: load, file PATH, same PATH, parse REASON or skip REASON")
			}
		} else {
			allowed := markKinds[b.mark.kind]
			if allowed != "" && !slices.Contains(strings.Fields(allowed), kindLang) {
				bad = append(bad, fmt.Sprintf("%s: verify: %s does not apply to a %q block", b.where, b.mark.kind, lang))
			}
			if b.mark.kind == "output" && (lastShell == nil || lastShell.mark != nil && lastShell.mark.kind == "skip") {
				bad = append(bad, b.where+": verify: output follows no step that is run")
			}
			if f := b.mark.opts["fixture"]; f != "" {
				if _, err := os.Stat(filepath.Join("testdata", "fixtures", f)); err != nil {
					bad = append(bad, fmt.Sprintf("%s: no fixture testdata/fixtures/%s", b.where, f))
				}
			}
		}
		if kindLang == "shell" {
			lastShell = b
		}
	}
	return bad
}

// TestDocBlocks checks the marks of every code block of README.md and the
// website, and the YAML examples that need no shell: load (all the load
// examples of a page together, as data files over testdata/docgame), same (equal to a file of the
// repository) and parse (YAML syntax). The shell steps run in TestDocSteps.
func TestDocBlocks(t *testing.T) {
	for _, doc := range stepDocuments(t) {
		src, err := os.ReadFile(filepath.Join(root, doc)) //nolint:gosec // A document of this repository.
		if err != nil {
			t.Fatal(err)
		}
		blocks, bad := parseBlocks(doc, src)
		bad = append(bad, checkMarks(blocks)...)
		bad = append(bad, checkYAMLBlocks(blocks)...)
		for _, b := range bad {
			t.Error(b)
		}
	}
}

// checkYAMLBlocks loads, compares and parses the YAML examples of one document.
func checkYAMLBlocks(blocks []block) []string {
	var bad []string
	var loads []block
	for _, b := range blocks {
		if !isYAML(b.lang) || b.mark == nil {
			continue
		}
		switch b.mark.kind {
		case "skip":
			continue
		case "same":
			want, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(b.mark.opts["path"])))
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s: %v", b.where, err))
			} else if !sameYAML(b.body, string(want)) {
				bad = append(bad, fmt.Sprintf("%s: the example differs from %s; copy the file again", b.where, b.mark.opts["path"]))
			}
		case "load":
			loads = append(loads, b)
		}
		var v any
		if err := yaml.Unmarshal([]byte(b.body), &v); err != nil {
			bad = append(bad, fmt.Sprintf("%s: not YAML: %v", b.where, err))
		}
	}
	// The load examples of a page load together, or by group=NAME (a recipe
	// of the cookbook is its own game).
	groups := map[string][]block{}
	var names []string
	for _, b := range loads {
		g := b.mark.opts["group"]
		if _, ok := groups[g]; !ok {
			names = append(names, g)
		}
		groups[g] = append(groups[g], b)
	}
	for _, g := range names {
		loads := groups[g]
		if err := loadExamples(loads); err != nil {
			msg := zzDoc.ReplaceAllStringFunc(err.Error(), func(s string) string {
				m := zzDoc.FindStringSubmatch(s)
				i, err := strconv.Atoi(m[1])
				if err != nil || i >= len(loads) {
					return s
				}
				return fmt.Sprintf("the block at %s, line %s:%s", loads[i].where, m[2], m[3])
			})
			msg = strings.ReplaceAll(msg, "game.yaml:", "game.yaml (the docgame's with the examples' as=game.yaml merged in):")
			what := "the load examples of the page"
			if g != "" {
				what = "the load examples of group=" + g
			}
			bad = append(bad, fmt.Sprintf("%s: %s do not load together over internal/dev/docs/testdata/docgame:\n%s", loads[0].where, what, msg))
		}
	}
	return bad
}

// sameYAML reports whether two YAML documents hold the same values; comments and layout may differ.
func sameYAML(a, b string) bool {
	var va, vb any
	if yaml.Unmarshal([]byte(a), &va) != nil || yaml.Unmarshal([]byte(b), &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

var zzDoc = regexp.MustCompile(`data/zz-doc-(\d+)\.yaml:(\d+):(\d+)`)

var assetName = regexp.MustCompile(`[\w./-]+\.(?:jpg|jpeg|png|webp|ogg|wav|mp3|ttf|otf)\b`)

// loadExamples loads the docgame with the examples added as data files. Pictures
// and sounds the examples name are present as stubs: gozen.Load checks that they
// exist, and gozen check that they are pictures.
func loadExamples(examples []block) error {
	overlay := fstest.MapFS{}
	game := map[string]any{}
	for i, b := range examples {
		as := b.mark.opts["as"]
		// The pictures a file under assets/ names are relative to its directory.
		dir := "assets"
		if strings.HasPrefix(as, "assets/") {
			dir = path.Dir(as)
		}
		for _, a := range assetName.FindAllString(b.body, -1) {
			overlay[dir+"/"+strings.TrimPrefix(a, "assets/")] = &fstest.MapFile{Data: []byte("a stub")}
		}
		if strings.HasPrefix(as, "assets/") && path.Clean(as) == as && !strings.Contains(as, "..") {
			// A file the game reads from assets/, such as a moving portrait (as=assets/PATH).
			overlay[as] = &fstest.MapFile{Data: []byte(b.body)}
			continue
		}
		switch as {
		case "":
		case "game.yaml":
			// The keys of the example replace the docgame's (as=game.yaml).
			var keys map[string]any
			if err := yaml.Unmarshal([]byte(b.body), &keys); err != nil {
				return fmt.Errorf("%s: %w", b.where, err)
			}
			maps.Copy(game, keys)
			continue
		default:
			return fmt.Errorf("%s: as=%s: want as=game.yaml or as=assets/PATH", b.where, as)
		}
		body := b.body
		if under := b.mark.opts["under"]; under != "" {
			body = under + ":\n" + body
		}
		overlay[fmt.Sprintf("data/zz-doc-%02d.yaml", i)] = &fstest.MapFile{Data: []byte(body)}
	}
	if len(game) > 0 {
		base, err := os.ReadFile("testdata/docgame/game.yaml")
		if err != nil {
			return err
		}
		merged := map[string]any{}
		if err := yaml.Unmarshal(base, &merged); err != nil {
			return err
		}
		maps.Copy(merged, game)
		b, err := yaml.Marshal(merged)
		if err != nil {
			return err
		}
		overlay["game.yaml"] = &fstest.MapFile{Data: b}
	}
	_, err := gozen.Load(os.DirFS("testdata/docgame"), ".", overlay)
	return err
}

// The marks are strict: unknown kinds, missing reasons and lone marks fail.
func TestParseBlocksMarks(t *testing.T) {
	src := strings.Join([]string{
		"<!-- verify: skip -->",
		"```shell",
		"go run .",
		"```",
		"<!-- verify: maybe -->",
		"```shell",
		"x",
		"```",
		"<!-- verify: run -->",
		"",
		"text in between",
		"```yaml",
		"a: 1",
		"```",
		"  <!-- verify: each except=2 needs a detection model -->",
		"  ```shell",
		"  gozen a",
		"  gozen b",
		"  ```",
		"<!-- verify: output -->",
		"```text",
		"ok",
		"```",
		"<!-- verify: file -->",
		"```yaml",
		"a: 1",
		"```",
		"<!-- verify: load -->",
		"```shell",
		"true",
		"```",
		"<!-- verify: parse ok -->",
		"```yaml",
		"a: [",
		"```",
	}, "\n")
	blocks, bad := parseBlocks("p.md", []byte(src))
	bad = append(bad, checkMarks(blocks)...)
	bad = append(bad, checkYAMLBlocks(blocks)...)
	got := strings.Join(bad, "\n")
	for _, want := range []string{
		"p.md:1: verify: skip needs a reason",
		`p.md:5: unknown verify mark "maybe"`,
		"p.md:9: a verify mark with no code block after it",
		"p.md:12: a YAML example needs a verify mark",
		"p.md:24: verify: file takes one path",
		`p.md:29: verify: load does not apply to a "shell" block`,
		"p.md:32: verify: parse needs a reason",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	each := blocks[3]
	if each.mark == nil || each.mark.kind != "each" || each.mark.opts["except"] != "2" || each.body != "gozen a\ngozen b\n" {
		t.Errorf("the indented each block: %+v", each)
	}
	if strings.Contains(got, "p.md:21") {
		t.Errorf("output after a run step was reported:\n%s", got)
	}
}

// A load example that breaks a reference fails with its place.
func TestLoadExamplesReportsErrors(t *testing.T) {
	good := block{body: "locations:\n  camp2: {name: Camp, background: camp.jpg, exits: [{to: docs_start}]}\n", mark: &mark{kind: "load", opts: map[string]string{}}}
	if err := loadExamples([]block{good}); err != nil {
		t.Fatalf("a good example: %v", err)
	}
	broken := block{body: "locations:\n  camp3: {name: Camp, exits: [{to: nowhere}]}\n", mark: &mark{kind: "load", opts: map[string]string{}}}
	err := loadExamples([]block{good, broken})
	if err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("a broken reference: %v", err)
	}
	game := block{body: "language: ja\ntext: {はじめから: Begin}\n", mark: &mark{kind: "load", opts: map[string]string{"as": "game.yaml"}}}
	if err := loadExamples([]block{game, good}); err != nil {
		t.Fatalf("a game.yaml example: %v", err)
	}
	badGame := block{body: "start: {location: nowhere}\n", mark: &mark{kind: "load", opts: map[string]string{"as": "game.yaml"}}}
	if err := loadExamples([]block{badGame}); err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("a broken game.yaml example: %v", err)
	}
	// Two recipes that define the same place load as two games with group=.
	one := block{doc: "c.md", where: "c.md:1", lang: "yaml", body: "locations:\n  inn: {name: Inn}\n", mark: &mark{kind: "load", opts: map[string]string{"group": "a"}}}
	two := block{doc: "c.md", where: "c.md:9", lang: "yaml", body: "locations:\n  inn: {name: Inn}\n", mark: &mark{kind: "load", opts: map[string]string{"group": "b"}}}
	if bad := checkYAMLBlocks([]block{one, two}); len(bad) > 0 {
		t.Fatalf("two groups: %v", bad)
	}
	two.mark.opts["group"] = "a"
	if bad := checkYAMLBlocks([]block{one, two}); len(bad) != 1 || !strings.Contains(bad[0], "group=a") {
		t.Fatalf("one group with the place twice: %v", bad)
	}
	// A file under assets/ (as=assets/PATH) with its pictures beside it.
	rig := block{body: "base: base.png\nfaces: {normal: {}}\n", mark: &mark{kind: "load", opts: map[string]string{"as": "assets/p/rig.yaml"}}}
	actor := block{body: "actors:\n  rigged: {name: Rigged, class: walker, puppet: p/rig.yaml}\n", mark: &mark{kind: "load", opts: map[string]string{}}}
	if err := loadExamples([]block{rig, actor}); err != nil {
		t.Fatalf("a puppet under assets/: %v", err)
	}
	if err := loadExamples([]block{actor}); err == nil || !strings.Contains(err.Error(), "p/rig.yaml") {
		t.Fatalf("an actor without its puppet: %v", err)
	}
	under := block{body: "  camp4:\n    name: Camp\n", mark: &mark{kind: "load", opts: map[string]string{"under": "locations"}}}
	if err := loadExamples([]block{under}); err != nil {
		t.Fatalf("an example under a key: %v", err)
	}
	if !sameYAML("a: 1 # one\nb: [2]\n", "# x\na: 1\nb:\n  - 2\n") || sameYAML("a: 1", "a: 2") {
		t.Error("sameYAML")
	}
}
