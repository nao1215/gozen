package docs

import (
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// adrMention is a link to the decision records or an ADR named by its number.
var adrMention = regexp.MustCompile(`docs/decisions/|\bADR[ -]?\d+`)

// readerDocs are the documents for the readers of the engine, not its
// developers: README.md and the website's pages.
func readerDocs(fsys fs.FS) map[string]string {
	out := map[string]string{}
	if b, err := fs.ReadFile(fsys, "README.md"); err == nil {
		out["README.md"] = string(b)
	}
	_ = fs.WalkDir(fsys, "website/content", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil //nolint:nilerr // a page that cannot be read is left to the site's own test
		}
		if b, err := fs.ReadFile(fsys, p); err == nil {
			out[p] = string(b)
		}
		return nil
	})
	return out
}

// adrMentions are the lines of the reader documents that link to or name an ADR.
func adrMentions(fsys fs.FS) []string {
	var bad []string
	for name, body := range readerDocs(fsys) {
		for i, line := range strings.Split(body, "\n") {
			if m := adrMention.FindString(line); m != "" {
				bad = append(bad, name+":"+strconv.Itoa(i+1)+": "+m)
			}
		}
	}
	return bad
}

// TestNoADRsForReaders: the decision records are for the engine's developers;
// README.md and the website link to the website's pages instead (docs/ and
// skills/ may name them).
func TestNoADRsForReaders(t *testing.T) {
	if bad := adrMentions(os.DirFS(root)); len(bad) > 0 {
		t.Fatalf("README.md and the website must not link to or name ADRs (link the website's page instead):\n%s", strings.Join(bad, "\n"))
	}
}

func TestADRMentionsFound(t *testing.T) {
	fsys := mapFS{
		"README.md":                "See [ADR 0057](docs/decisions/0057-phone-apps.md).\nfine\n",
		"website/content/a.md":     "Options (ADR 0018).\n",
		"website/content/b/c.md":   "[x](https://github.com/nao1215/gozen/blob/main/docs/decisions/0036-x.md)\n",
		"website/content/ok.md":    "No records here; ADRIFT is a word.\n",
		"docs/decisions/0057-x.md": "ADR 0057\n",
		"skills/gozen/SKILL.md":    "ADR 0057\n",
	}
	got := adrMentions(fsys.fs())
	if len(got) != 3 {
		t.Fatalf("want 3 mentions (README, a.md, c.md), got %v", got)
	}
}
