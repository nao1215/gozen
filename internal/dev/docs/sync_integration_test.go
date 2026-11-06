package docs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// These integration checks compare the author-facing contracts with their
// owners. Inventories are ordinary Markdown tables, so readers see the same
// names that the tests check; HTML comments alone never satisfy coverage.
func markdownUnder(t testing.TB, dir string) []string {
	return textFilesUnder(t, dir, []string{".md"})
}

func textFilesUnder(t testing.TB, dir string, extensions []string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "public" || d.Name() == "resources") {
			return fs.SkipDir
		}
		if !d.IsDir() && slices.Contains(extensions, filepath.Ext(p)) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(paths)
	return paths
}

func documentText(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // Source files discovered inside this checkout.
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func reportDrift(t testing.TB, bad []string) {
	t.Helper()
	slices.Sort(bad)
	if len(bad) != 0 {
		t.Errorf("documentation drift (%d):\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// inventoryRows reads only tables with the specified first-column heading.
// Each row needs a backtick-delimited identifier and useful visible prose.
func inventoryRows(doc, text, heading string) (map[string]string, []string) {
	rows := map[string]string{}
	var bad []string
	active := false
	for i, line := range strings.Split(text, "\n") {
		cells := strings.Split(strings.TrimSpace(line), "|")
		if len(cells) < 4 {
			active = false
			continue
		}
		first := strings.TrimSpace(cells[1])
		if first == heading {
			active = true
			continue
		}
		if !active || strings.HasPrefix(first, "---") || strings.HasPrefix(first, ":--") {
			continue
		}
		where := fmt.Sprintf("%s:%d", doc, i+1)
		if !strings.HasPrefix(first, "`") || !strings.HasSuffix(first, "`") {
			bad = append(bad, where+": inventory identifier must be in backticks")
			continue
		}
		name := strings.Trim(first, "`")
		if _, found := rows[name]; found {
			bad = append(bad, where+": duplicate "+heading+" "+name)
		}
		value := strings.TrimSpace(strings.Join(cells[2:len(cells)-1], "|"))
		if len(strings.Fields(value)) < 3 {
			bad = append(bad, where+": explain the purpose of "+name)
		}
		rows[name] = value
	}
	return rows, bad
}

func inventoryDiff(where string, want, got map[string]string) []string {
	var bad []string
	for name := range want {
		if _, ok := got[name]; !ok {
			bad = append(bad, where+": missing "+name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			bad = append(bad, where+": stale or unknown "+name)
		}
	}
	return bad
}

func TestSyncEventVerbs(t *testing.T) {
	verbs := map[string]string{}
	publicFields := map[string]string{}
	for field := range reflect.TypeFor[gamedata.Command]().Fields() {
		role := field.Tag.Get("command")
		if role != "" && role != "internal" {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			publicFields[name] = field.Name
			if role == "verb" {
				verbs[name] = field.Name
			}
		}
	}
	// The Schema page renders these descriptions from the committed schema.
	fields := schemaFields(t)
	descriptions, bad := schemaDescriptions(t)
	schemaCommand := map[string]string{}
	for _, field := range fields["Command"] {
		schemaCommand[field] = field
	}
	bad = append(bad, inventoryDiff("website schema Command fields", publicFields, schemaCommand)...)
	for name := range verbs {
		if !slices.Contains(fields["Command"], name) || descriptions["Command"][name] == "" {
			bad = append(bad, "website schema Command: missing event verb "+name)
		}
	}
	doc := "skills/gozen/references/commands.md"
	rows, problems := inventoryRows(doc, documentText(t, doc), "Event verb")
	bad = append(bad, problems...)
	bad = append(bad, inventoryDiff(doc, verbs, rows)...)
	for _, dir := range []string{"website/content", "skills/gozen"} {
		for _, name := range markdownUnder(t, dir) {
			bad = append(bad, checkEventMentions(name, documentText(t, name), verbs)...)
		}
	}
	reportDrift(t, bad)
}

// Command contexts avoid treating unrelated YAML keys (a puppet's "head", a
// workflow's "run") as verbs. Parse-only fragments still get this name check.
func checkEventMentions(doc, text string, verbs map[string]string) []string {
	blocks, _ := parseBlocks(doc, []byte(text))
	aliases := map[string]string{}
	var values []any
	for _, b := range blocks {
		var value any
		if isYAML(b.lang) {
			if err := yaml.Unmarshal([]byte(b.body), &value); err != nil {
				value = nil // Syntax failures and deliberately non-YAML examples belong to TestDocBlocks.
			}
		}
		values = append(values, value)
		if m, ok := value.(map[string]any); ok {
			if names, ok := m["verbs"].(map[string]any); ok {
				for name := range names {
					aliases[name] = "local event alias"
				}
			}
		}
	}
	// Inline alias declarations are also common in short skill recipes.
	for _, m := range regexp.MustCompile("`verbs: (\\{[^`]+\\})`").FindAllStringSubmatch(text, -1) {
		var names map[string]string
		if yaml.Unmarshal([]byte(m[1]), &names) == nil {
			for name := range names {
				aliases[name] = "local event alias"
			}
		}
	}
	fields := map[string]bool{}
	for field := range reflect.TypeFor[gamedata.Command]().Fields() {
		if role := field.Tag.Get("command"); role != "" && role != "internal" {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			fields[name] = true
		}
	}
	var bad []string
	var walk func(any, string, bool)
	walk = func(value any, where string, command bool) {
		switch v := value.(type) {
		case []any:
			for i, item := range v {
				walk(item, fmt.Sprintf("%s[%d]", where, i), command)
			}
		case map[string]any:
			if command {
				for key := range v {
					if !fields[key] && aliases[key] == "" {
						bad = append(bad, where+": unknown event command field "+key)
					}
				}
			}
			for key, item := range v {
				if key == "rules" {
					// Planning rules contain PlanCommand operations, not event
					// verbs. Reaction rules have their own vocabulary too.
					continue
				}
				if key == "events" {
					if events, ok := item.(map[string]any); ok {
						for id, event := range events {
							_, compact := event.([]any)
							walk(event, where+".events."+id, compact)
						}
					}
					continue
				}
				commands := slices.Contains([]string{"do", "talk", "on_enter", "then", "else", "on_win", "on_lose", "on_escape", "setup"}, key)
				walk(item, where+"."+key, commands)
			}
		}
	}
	for i, b := range blocks {
		command := false
		if list, ok := values[i].([]any); ok {
			// An isolated list containing a verb is an event fragment. Explicit
			// event-fragment marks also catch lists made entirely of stale verbs.
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					for key := range m {
						command = command || verbs[key] != "" || aliases[key] != ""
					}
				}
			}
			if b.mark != nil && strings.Contains(b.mark.reason, "event") {
				command = true
			}
		}
		walk(values[i], b.where, command)
	}
	// Verb tables can group several names in one cell, as writing-a-game does.
	active := false
	inlineName := regexp.MustCompile("`([a-z][a-z0-9_]*)`")
	for i, line := range strings.Split(text, "\n") {
		cells := strings.Split(strings.TrimSpace(line), "|")
		if len(cells) < 4 {
			active = false
			continue
		}
		first := strings.TrimSpace(cells[1])
		if first == "Verb" || first == "Event verb" {
			active = true
			continue
		}
		if active {
			for _, m := range inlineName.FindAllStringSubmatch(first, -1) {
				if verbs[m[1]] == "" && aliases[m[1]] == "" {
					bad = append(bad, fmt.Sprintf("%s:%d: unknown event verb %s", doc, i+1, m[1]))
				}
			}
		}
	}
	// Explicit prose references have an unambiguous event namespace. Bare
	// "command" can mean CLI or input-script commands, so do not guess it.
	mention := regexp.MustCompile("(?:event command|verb) `([a-z][a-z0-9_]*)`|`([a-z][a-z0-9_]*)` (?:event command|verb)")
	for i, line := range strings.Split(text, "\n") {
		for _, match := range mention.FindAllStringSubmatch(line, -1) {
			name := match[1] + match[2]
			if verbs[name] == "" && aliases[name] == "" {
				bad = append(bad, fmt.Sprintf("%s:%d: unknown event verb %s", doc, i+1, name))
			}
		}
	}
	return bad
}

func TestSyncDiagnosticCodes(t *testing.T) {
	errorsPage := documentText(t, "website/content/errors.md")
	bad := checkDiagnosticPage(errorsPage)
	for _, dir := range []string{"docs", "skills", "website"} {
		for _, doc := range textFilesUnder(t, dir, []string{".md", ".yaml", ".yml", ".json", ".jsonl", ".txt", ".toml", ".html", ".gotmpl", ".sh", ".py", ".js", ".css"}) {
			bad = append(bad, checkDiagnosticMentions(doc, documentText(t, doc))...)
		}
	}
	reportDrift(t, bad)
}

func checkDiagnosticPage(text string) []string {
	var bad []string
	for _, entry := range diag.All() {
		if !strings.Contains(text, "### "+entry.Code.String()+"\n") {
			bad = append(bad, "website/content/errors.md: missing heading for "+entry.Code.String())
		}
	}
	return bad
}

func checkDiagnosticMentions(doc, text string) []string {
	var bad []string
	codeRE := regexp.MustCompile(`(?i)\bGZN\d{4}\b`)
	for i, line := range strings.Split(text, "\n") {
		for _, code := range codeRE.FindAllString(line, -1) {
			if _, ok := diag.Parse(code); !ok {
				bad = append(bad, fmt.Sprintf("%s:%d: unknown diagnostic %s", doc, i+1, code))
			}
		}
	}
	return bad
}

func TestSyncSchemaSections(t *testing.T) {
	want := map[string]string{}
	fields := schemaFields(t)
	for _, file := range []string{"game", "data"} {
		var schema struct {
			Ref string `json:"$ref"`
		}
		name := "data/schema/" + file + ".schema.json"
		if err := json.Unmarshal([]byte(documentText(t, name)), &schema); err != nil {
			t.Fatal(err)
		}
		typ := strings.TrimPrefix(schema.Ref, "types.schema.json#/$defs/")
		if len(fields[typ]) == 0 {
			t.Fatalf("%s: no root properties; update the inventory reader", name)
		}
		for _, key := range fields[typ] {
			want[file+"."+key] = key
		}
	}
	doc := "skills/gozen/references/yaml.md"
	rows, bad := inventoryRows(doc, documentText(t, doc), "Section")
	bad = append(bad, inventoryDiff(doc, want, rows)...)
	reportDrift(t, bad)
}

var (
	adrLink  = regexp.MustCompile(`\[([^\]\n]*)\]\((\d{4})-([^()#]+\.md)(?:#[^()]*)?\)`)
	adrFile  = regexp.MustCompile(`^(\d{4})-.+\.md$`)
	adrLabel = regexp.MustCompile(`^(?:ADR )?\d{4}$`)
)

func checkADRIndex(files []string, index string) []string {
	var bad []string
	byNumber, byFile, listed := map[string]string{}, map[string]bool{}, map[string]bool{}
	for _, name := range files {
		m := adrFile.FindStringSubmatch(name)
		if m == nil {
			bad = append(bad, "docs/decisions/"+name+": expected NNNN-title.md")
			continue
		}
		if previous, ok := byNumber[m[1]]; ok {
			bad = append(bad, "docs/decisions: duplicate ADR "+m[1]+": "+previous+", "+name)
		}
		byNumber[m[1]], byFile[name] = name, true
	}
	for i, line := range strings.Split(index, "\n") {
		for _, m := range adrLink.FindAllStringSubmatch(line, -1) {
			name := m[2] + "-" + m[3]
			where := fmt.Sprintf("docs/decisions/index.md:%d", i+1)
			row := strings.HasPrefix(strings.TrimSpace(line), "| "+m[0]+" |")
			if (row || adrLabel.MatchString(m[1])) && strings.TrimPrefix(m[1], "ADR ") != m[2] {
				bad = append(bad, where+": ADR label "+m[1]+" disagrees with "+name)
			}
			if !byFile[name] {
				bad = append(bad, where+": nonexistent ADR "+name)
			}
			// Cross-references in the status column do not count as index rows.
			if row {
				if listed[name] {
					bad = append(bad, where+": repeated ADR row "+name)
				}
				listed[name] = true
			}
		}
	}
	for name := range byFile {
		if !listed[name] {
			bad = append(bad, "docs/decisions/index.md: missing ADR row "+name)
		}
	}
	return bad
}

func TestSyncADRIndex(t *testing.T) {
	var files []string
	var bad []string
	for _, name := range markdownUnder(t, "docs/decisions") {
		if filepath.Base(name) != "index.md" {
			files = append(files, filepath.Base(name))
			if number := adrFile.FindStringSubmatch(filepath.Base(name)); number != nil {
				first, _, _ := strings.Cut(documentText(t, name), "\n")
				if !strings.HasPrefix(first, "# ADR "+number[1]+": ") {
					bad = append(bad, name+":1: title must identify ADR "+number[1])
				}
			}
		}
	}
	bad = append(bad, checkADRIndex(files, documentText(t, "docs/decisions/index.md"))...)
	reportDrift(t, bad)
}

type cliContract struct {
	Command string `json:"command"`
	Options []struct {
		Name  string `json:"name"`
		Short string `json:"short"`
	} `json:"options"`
}

func TestSyncCLIHelp(t *testing.T) {
	// Help does not initialize graphics, load a game or download models. Use the
	// executable's real registry, including the production command adapters.
	bin := filepath.Join(t.TempDir(), exe("gozen"))
	command(t, root, 5*time.Minute, "go", "build", "-buildvcs=false", "-o", bin, "./cmd/gozen")
	cmd := exec.CommandContext(t.Context(), bin, "help", "--format", "json") //nolint:gosec // The executable built above in the test's temporary directory.
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Commands []cliContract `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Commands) == 0 {
		t.Fatal("help returned no commands")
	}
	want := map[string]string{}
	for _, c := range envelope.Data.Commands {
		name := strings.TrimSpace("gozen " + c.Command)
		var flags []string
		for _, option := range c.Options {
			flags = append(flags, "--"+option.Name)
			if option.Short != "" {
				flags = append(flags, "-"+option.Short)
			}
		}
		want[name] = strings.Join(flags, " ")
	}
	flagRE := regexp.MustCompile("`(-{1,2}[a-zA-Z][a-zA-Z0-9-]*)`")
	for _, doc := range []string{"website/content/commands.md", "skills/gozen/references/cli.md"} {
		rows, bad := inventoryRows(doc, documentText(t, doc), "CLI command")
		bad = append(bad, inventoryDiff(doc, want, rows)...)
		for name, flags := range want {
			if _, exists := rows[name]; !exists {
				continue
			}
			gotFlags, wantFlags := map[string]string{}, map[string]string{}
			for f := range strings.FieldsSeq(flags) {
				wantFlags[f] = f
			}
			for _, m := range flagRE.FindAllStringSubmatch(rows[name], -1) {
				gotFlags[m[1]] = m[1]
			}
			bad = append(bad, inventoryDiff(doc+": "+name, wantFlags, gotFlags)...)
		}
		reportDrift(t, bad)
	}
	for _, doc := range append([]string{"website/content/commands.md"}, markdownUnder(t, "skills/gozen")...) {
		reportDrift(t, checkCLIMentions(doc, documentText(t, doc), want))
	}
}

var (
	cliFlags   = regexp.MustCompile(`(?:^|[\s,` + "`" + `])(-{1,2}[a-zA-Z][a-zA-Z0-9-]*)\b`)
	inlineCode = regexp.MustCompile("`([^`\n]+)`")
)

func checkCLIMentions(doc, text string, contracts map[string]string) []string {
	var bad []string
	allFlags := map[string]bool{}
	for _, flags := range contracts {
		for flag := range strings.FieldsSeq(flags) {
			allFlags[flag] = true
		}
	}
	checkFlags := func(where, name, text string) {
		words := strings.Fields(text)
		if end := slices.Index(words, "--"); end >= 0 {
			text = strings.Join(words[:end], " ")
		}
		for _, m := range cliFlags.FindAllStringSubmatch(text, -1) {
			if !slices.Contains(strings.Fields(contracts[name]), m[1]) {
				bad = append(bad, where+": "+name+" has no option "+m[1])
			}
		}
	}
	checkInvocation := func(where, code string) {
		words := strings.Fields(code)
		at := slices.Index(words, "gozen")
		if at < 0 || at+1 == len(words) {
			return
		}
		words = words[at+1:]
		name, used := "gozen", 0
		for i, word := range words {
			if _, ok := contracts[name+" "+word]; !ok {
				break
			}
			name += " " + word
			used = i + 1
		}
		if used == 0 && !strings.HasPrefix(words[0], "-") {
			if !strings.ContainsAny(words[0], "<[") {
				bad = append(bad, where+": unknown CLI command gozen "+words[0])
			}
			return // A placeholder command cannot have its options checked.
		}
		if used < len(words) && slices.Contains([]string{"gozen sim", "gozen models", "gozen theme"}, name) {
			next := words[used]
			if !strings.HasPrefix(next, "-") && !strings.ContainsAny(next, "<[") {
				bad = append(bad, where+": unknown CLI subcommand "+name+" "+next)
			}
		}
		if name == "gozen help" && used < len(words) && !strings.HasPrefix(words[used], "-") {
			query := words[used:]
			for len(query) > 0 && query[0] == "help" {
				query = query[1:]
			}
			end := slices.IndexFunc(query, func(word string) bool { return strings.HasPrefix(word, "-") })
			if end >= 0 {
				query = query[:end]
			}
			target := strings.TrimSpace("gozen " + strings.Join(query, " "))
			if _, ok := contracts[target]; !ok && len(query) > 0 && !diag.Syntax(query[0]) && !strings.ContainsAny(query[0], "<[") {
				bad = append(bad, where+": unknown CLI help target "+target)
			}
		}
		checkFlags(where, name, strings.Join(words[used:], " "))
	}
	// Check shell commands and inline invocations, including skipped shell
	// examples: a missing display is no excuse for a nonexistent option.
	blocks, _ := parseBlocks(doc, []byte(text))
	for _, b := range blocks {
		if shellLangs[b.lang] {
			for _, line := range commands(b.body) {
				line, _, _ = strings.Cut(line, " #")
				checkInvocation(b.where, line)
			}
		}
	}
	context := map[int]string{}
	fenced := false
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		where := fmt.Sprintf("%s:%d", doc, i+1)
		for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
			checkInvocation(where, m[1])
			if strings.HasPrefix(m[1], "-") {
				for _, flag := range cliFlags.FindAllStringSubmatch(m[1], -1) {
					name := flag[1]
					// The conventions explicitly demonstrate rejected single-
					// dash spellings of real long options (for example -seed).
					invalidExample := !strings.HasPrefix(name, "--") && allFlags["-"+name] && (strings.Contains(line, "rejected") || strings.Contains(line, "invalid"))
					if !allFlags[name] && !invalidExample {
						bad = append(bad, where+": unknown CLI option "+name)
					}
				}
			}
		}
		if strings.HasPrefix(line, "#") {
			level := len(line) - len(strings.TrimLeft(line, "#"))
			for previous := range context {
				if previous >= level {
					delete(context, previous)
				}
			}
			name := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if !strings.HasPrefix(name, "gozen") {
				name = "gozen " + name
			}
			if _, ok := contracts[name]; ok {
				context[level] = name
			}
		}
		// Option tables inherit their nearest command heading. Index tables
		// are checked separately, one exact option set per command.
		if strings.HasPrefix(trimmed, "| `-") {
			level, name := 0, ""
			for depth, command := range context {
				if depth > level {
					level, name = depth, command
				}
			}
			if name != "" {
				cell := strings.Split(trimmed, "|")[1]
				checkFlags(where, name, cell)
			}
		}
	}
	return bad
}

func TestSyncFindsDrift(t *testing.T) {
	t.Run("inventory", func(t *testing.T) {
		text := "| Event verb | Purpose |\n| --- | --- |\n| `say` | Speak a line |\n| `removed` | An obsolete command |\n| `say` | Speak a second line |\n"
		rows, bad := inventoryRows("skill.md", text, "Event verb")
		bad = append(bad, inventoryDiff("skill.md", map[string]string{"say": "", "phone": ""}, rows)...)
		assertDrift(t, bad, "duplicate Event verb say", "missing phone", "stale or unknown removed")
	})
	t.Run("verbs in fragments and aliases", func(t *testing.T) {
		text := "<!-- verify: parse event fragment for name validation -->\n```yaml\n- retired: true\n- choice:\n  - label: Go\n    do: [{typo: true}]\n```\n```yaml\nverbs: {greet: greeting}\nevents:\n  greeting: [{say: Hello}, {greet: true}]\n```\n| Verb | Does |\n| --- | --- |\n| `obsolete`, `say` | Speak a line |\n"
		bad := checkEventMentions("page.md", text, map[string]string{"say": "Say", "choice": "Choice"})
		assertDrift(t, bad, "unknown event command field retired", "unknown event command field typo", "unknown event verb obsolete")
		if len(bad) != 3 {
			t.Fatalf("local alias or valid command rejected: %v", bad)
		}
		planning := "```yaml\nplanning:\n  rules:\n    rest:\n      do: [{op: set, attribute: vigor, value: 1}]\n```\nThe event command `removed` is obsolete.\n"
		bad = checkEventMentions("planning.md", planning, map[string]string{"plan": "Plan"})
		if len(bad) != 1 || !strings.Contains(bad[0], "unknown event verb removed") {
			t.Fatalf("planning operations and event prose must remain distinct: %v", bad)
		}
	})
	t.Run("CLI flags belong to their command", func(t *testing.T) {
		contracts := map[string]string{"gozen": "--help -h", "gozen help": "--help -h --format", "gozen run": "--help -h --dev", "gozen check": "--help -h --format", "gozen sim": "--help -h", "gozen sim routes": "--help --depth"}
		text := "`gozen gone GAME`\n`gozen check GAME --help --dev`\n`gozen sim absent`\n## run\n| Option | Purpose |\n| --- | --- |\n| `--format` | Select the format |\n```sh\ngozen run --help -h --removed\n```\n`gozen check -- -draft`\n`gozen <command> --OPTION`\n"
		bad := checkCLIMentions("cli.md", text, contracts)
		assertDrift(t, bad, "unknown CLI command gozen gone", "gozen check has no option --dev", "unknown CLI subcommand gozen sim absent", "gozen run has no option --format", "gozen run has no option --removed")
		if len(bad) != 5 {
			t.Fatalf("terminator, aliases or placeholder misread: %v", bad)
		}
		bad = checkCLIMentions("options.md", "Use `--removed` for output.\nThe single-dash `-format` is rejected.\n", contracts)
		if len(bad) != 1 || !strings.Contains(bad[0], "unknown CLI option --removed") {
			t.Fatalf("standalone option checking: %v", bad)
		}
		bad = checkCLIMentions("help.md", "`gozen help help sim routes --format json`\n`gozen help GZN1101`\n`gozen help removed`", contracts)
		if len(bad) != 1 || !strings.Contains(bad[0], "unknown CLI help target gozen removed") {
			t.Fatalf("help target checking: %v", bad)
		}
	})
	t.Run("ADR rows and references", func(t *testing.T) {
		index := "| [0002](0001-first.md) | First | See [0003](0003-third.md) |\n| [0002](0002-gone.md) | Gone | |\n"
		bad := checkADRIndex([]string{"0001-first.md", "0001-duplicate.md", "0003-third.md"}, index)
		assertDrift(t, bad, "duplicate ADR 0001", "ADR label 0002 disagrees", "nonexistent ADR 0002-gone.md", "missing ADR row 0003-third.md", "missing ADR row 0001-duplicate.md")
		bad = checkADRIndex([]string{"0001-first.md"}, "| [0001](0001-first.md) | First | |\nSee [future decision](9999-gone.md#details) and [ADR 0002](0001-first.md).")
		assertDrift(t, bad, "nonexistent ADR 9999-gone.md", "ADR label ADR 0002 disagrees")
		if len(bad) != 2 {
			t.Fatalf("descriptive links and numbered references: %v", bad)
		}
	})
	t.Run("diagnostic coverage and references", func(t *testing.T) {
		page := documentText(t, "website/content/errors.md")
		code := diag.All()[0].Code.String()
		page = strings.ReplaceAll(page, "### "+code+"\n", "")
		bad := checkDiagnosticPage(page)
		if len(bad) != 1 || !strings.Contains(bad[0], "missing heading for "+code) {
			t.Fatalf("a mention outside a heading cannot cover a diagnostic: %v", bad)
		}
		bad = checkDiagnosticMentions("page.md", "GZN01xx is a family.\n"+code+" exists; GZN9999 does not.\ngzn9998 is unknown too.")
		assertDrift(t, bad, "page.md:2: unknown diagnostic GZN9999", "page.md:3: unknown diagnostic gzn9998")
		if len(bad) != 2 {
			t.Fatalf("families and registered codes should pass: %v", bad)
		}
	})
}

func assertDrift(t testing.TB, bad []string, want ...string) {
	t.Helper()
	text := strings.Join(bad, "\n")
	for _, s := range want {
		if !strings.Contains(text, s) {
			t.Errorf("missing failure %q in:\n%s", s, text)
		}
	}
}
