package gozen_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// japaneseCommentAllowlist lists files whose Go comments may contain unquoted
// Japanese text. The key is a slash-separated path relative to the repository
// root; the value is the reason. Keep it empty if possible: a comment may quote
// Japanese examples inside "..." or the corner brackets, which are not checked.
var japaneseCommentAllowlist = map[string]string{}

// TestGoCommentsAreEnglish fails when a Go comment contains Japanese
// characters. String literals are not checked: game-facing text, catalogs, and
// test expectations may be Japanese. Only comments must be English. A comment
// may still quote a Japanese example inside double quotes, backquotes, or
// corner brackets, because some comments explain behavior on such examples.
func TestGoCommentsAreEnglish(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "build", "node_modules", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		slash := filepath.ToSlash(path)
		if _, ok := japaneseCommentAllowlist[slash]; ok {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		for _, group := range file.Comments {
			for _, c := range group.List {
				if hasJapanese(quoted.ReplaceAllString(c.Text, "")) {
					t.Errorf("%s: Go comment contains Japanese; write it in English", fset.Position(c.Pos()))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// quoted matches a quoted span: "...", `...`, or a corner-bracket pair.
var quoted = regexp.MustCompile("\"[^\"\n]*\"|`[^`\n]*`|「[^」\n]*」")

func hasJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
			(r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF) {
			return true
		}
	}
	return false
}
