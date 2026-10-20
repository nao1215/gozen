package docs

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// japaneseAllowed are the Japanese words a document may use on purpose, by file.
// Everything else in a Markdown document must be English (AGENTS.md).
var japaneseAllowed = map[string][]string{
	"README.md":                          {"御膳"},    // the origin of the name
	"website/content/_index.md":          {"御膳"},    // the origin of the name
	"website/content/getting-started.md": {"はじめから"}, // a key of `text:`, the engine's own Japanese source text
	"website/content/cookbook.md":        {"はじめから"}, // the same key, in the language recipe
	// Engine words quoted as data: the keys of the engine's text catalog.
	"docs/decisions/0015-reaction-and-chat.md":       {"ﾊﾞ"}, // half-width katakana, the case the text explains
	"docs/decisions/0019-engine-languages.md":        {"はじめから", "最初から"},
	"docs/decisions/0021-query-and-commands.md":      {"さいきょうそうび"},
	"docs/decisions/0022-maps-and-painted-scenes.md": {"名前", "話す："},
}

// adultWords may appear only in the mosaic tool (ADR 0004): its code and the mosaic
// sections of the production-tools pages.
var adultWords = regexp.MustCompile(`(?i)\b(penis|pussy|nipples?|nipple_f|vagina|anus|nsfw|hentai|porn\w*|sexual\w*|erotic\w*|nude|naked|genitals?|genitalia|sex)\b|性器|乳首|陰茎|陰部|膣|肛門|エロ|ヌード|セックス|性的|卑猥|裸`)

// adultAllowed reports the part of a file where the words may appear: the whole
// file, a Markdown section that starts at the heading given, or nothing.
func adultAllowed(file string) (whole bool, section string) {
	switch {
	case strings.HasPrefix(file, "internal/mosaic/"), strings.HasPrefix(file, "cmd/gozen/mosaic"),
		file == "internal/dev/docs/language_test.go": // the list of the words itself
		return true, ""
	case file == "website/content/production-tools.md", file == "docs/development/production-tools.md":
		return false, "## gozen art mosaic"
	}
	return false, ""
}

// repositoryText walks the repository's text files (not built output, not binary).
func repositoryText(fsys fs.FS, visit func(name string, body []byte)) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch name {
			case ".git", "build", "bin", "dist", ".cache", "website/public", "website/resources", "third_party_licenses":
				return fs.SkipDir
			}
			return nil
		}
		if name == ".git" || imageExt.MatchString(name) {
			return nil
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return nil // binary
		}
		visit(name, b)
		return nil
	})
}

// markdownSection returns where the section that starts at heading begins and ends
// (the next heading of the same or a higher level), or -1, -1.
func markdownSection(doc, heading string) (start, end int) {
	start = -1
	pos := 0
	level := strings.Count(strings.Fields(heading)[0], "#")
	for _, line := range strings.SplitAfter(doc, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if start < 0 && trimmed == heading {
			start = pos
		} else if start >= 0 {
			if f := strings.Fields(trimmed); len(f) > 0 && strings.Trim(f[0], "#") == "" && strings.Count(f[0], "#") <= level {
				return start, pos
			}
		}
		pos += len(line)
	}
	if start < 0 {
		return -1, -1
	}
	return start, len(doc)
}

func checkLanguage(fsys fs.FS) []string {
	var bad []string
	err := repositoryText(fsys, func(name string, b []byte) {
		text := string(b)
		// Markdown documents and the published JSON (the schema in data/schema) are
		// English only.
		if (strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".json")) && !strings.Contains(name, "/testdata/") {
			clean := text
			for _, w := range japaneseAllowed[name] {
				clean = strings.ReplaceAll(clean, w, "")
			}
			for i, line := range strings.Split(clean, "\n") {
				if japanese.MatchString(line) {
					bad = append(bad, fmt.Sprintf("%s:%d: Japanese in a document; documents are English only (an intended word goes in japaneseAllowed)", name, i+1))
					break
				}
			}
		}
		whole, section := adultAllowed(name)
		if whole {
			return
		}
		from, to := -1, -1
		if section != "" {
			from, to = markdownSection(text, section)
		}
		for _, loc := range adultWords.FindAllStringIndex(text, -1) {
			if loc[0] >= from && loc[1] <= to && from >= 0 {
				continue
			}
			line := strings.Count(text[:loc[0]], "\n") + 1
			bad = append(bad, fmt.Sprintf("%s:%d: %q belongs only to the mosaic tool (internal/mosaic and the mosaic sections of the production-tools pages)", name, line, text[loc[0]:loc[1]]))
		}
	})
	if err != nil {
		bad = append(bad, err.Error())
	}
	sort.Strings(bad)
	return bad
}

// TestDocumentLanguage keeps the documents and the published JSON English and the adult vocabulary of the
// mosaic tool inside the mosaic tool.
func TestDocumentLanguage(t *testing.T) {
	if bad := checkLanguage(os.DirFS(root)); len(bad) > 0 {
		t.Fatalf("%d problems:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

func TestCheckLanguageFindsProblems(t *testing.T) {
	fsys := mapFS{
		"docs/x.md":                            "# Title\n\nこれは日本語。\n",
		"data/schema/x.schema.json":            "{\n  \"description\": \"共通の型\"\n}\n",
		"internal/x/testdata/note.json":        "{\"a\": \"テスト\"}\n",
		"README.md":                            "A gozen (御膳) is a meal.\n",
		"website/content/_index.md":            "Fine.\n",
		"internal/play/x.go":                   "// shows a nude figure\n",
		"internal/mosaic/command/help.go":      "// penis, pussy\n",
		"website/content/production-tools.md":  "## gozen art prompt\n\nnsfw\n\n## gozen art mosaic\n\npenis\n\n### Fetching\n\npussy\n\n## Next\n\nnipple\n",
		"internal/x/testdata/note.md":          "テスト\n",
		"docs/images/a.png":                    "nude",
		"docs/development/production-tools.md": "## gozen art mosaic\n\npenis\n",
	}.fs()
	got := strings.Join(checkLanguage(fsys), "\n")
	for _, want := range []string{
		"docs/x.md:3: Japanese in a document",
		"data/schema/x.schema.json:2: Japanese in a document",
		`internal/play/x.go:1: "nude" belongs only to the mosaic tool`,
		`website/content/production-tools.md:3: "nsfw"`,
		`website/content/production-tools.md:15: "nipple"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, no := range []string{"README.md", "help.go", `"penis"`, `"pussy"`, "testdata", "a.png"} {
		if strings.Contains(got, no) {
			t.Errorf("%s was reported:\n%s", no, got)
		}
	}
	if s, e := markdownSection("a\n## x\nb\n### y\nc\n## z\n", "## x"); s != 2 || e != 17 {
		t.Errorf("markdownSection = %d, %d", s, e)
	}
}
