package docs

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

var (
	mdLink    = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	mdHeading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)
	codeFence = regexp.MustCompile("(?ms)^\\s*```.*?^\\s*```") //nolint:gocritic // badRegexp: (?m) makes the second ^ match at a line start
)

// githubSlug is the anchor GitHub gives a heading.
func githubSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// anchors are the heading anchors of a Markdown document, numbered like GitHub's for repeats.
func anchors(doc string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	for _, m := range mdHeading.FindAllStringSubmatch(codeFence.ReplaceAllString(doc, ""), -1) {
		text := regexp.MustCompile("[`*]").ReplaceAllString(m[1], "")
		text = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(text, "$1")
		slug := githubSlug(text)
		if n := seen[slug]; n > 0 {
			out[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			out[slug] = true
		}
		seen[slug]++
	}
	return out
}

// checkMarkdownLinks reports the relative links of the repository's Markdown documents
// (outside website/, whose links are site paths) that lead to no file or no heading.
func checkMarkdownLinks(fsys fs.FS) []string {
	docs := map[string]string{}
	var bad []string
	if err := repositoryText(fsys, func(name string, b []byte) {
		if strings.HasSuffix(name, ".md") && !strings.Contains(name, "/testdata/") {
			docs[name] = string(b)
		}
	}); err != nil {
		return []string{err.Error()}
	}
	for name, doc := range docs {
		if strings.HasPrefix(name, "website/") {
			continue
		}
		for _, m := range mdLink.FindAllStringSubmatch(codeFence.ReplaceAllString(doc, ""), -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, frag, _ := strings.Cut(target, "#")
			resolved := name
			if file != "" {
				resolved = path.Clean(path.Join(path.Dir(name), file))
				if _, err := fs.Stat(fsys, resolved); err != nil {
					bad = append(bad, fmt.Sprintf("%s: %s: no such file", name, target))
					continue
				}
			}
			if frag == "" || !strings.HasSuffix(resolved, ".md") || strings.HasPrefix(frag, "L") && strings.Trim(frag[1:], "0123456789-L") == "" {
				continue
			}
			body, ok := docs[resolved]
			if !ok {
				continue
			}
			if !anchors(body)[frag] {
				bad = append(bad, fmt.Sprintf("%s: %s: %s has no heading #%s", name, target, resolved, frag))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// TestMarkdownLinks keeps the links between the repository's documents working,
// headings included (they change when a document is rewritten).
func TestMarkdownLinks(t *testing.T) {
	if bad := checkMarkdownLinks(os.DirFS(root)); len(bad) > 0 {
		t.Fatalf("%d broken links:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

func TestCheckMarkdownLinksFindsBrokenOnes(t *testing.T) {
	fsys := mapFS{
		"docs/a.md":            "# A\n\n## Graphics tests\n\n## Graphics tests\n\n[ok](b.md#the-rules) [self](#graphics-tests-1) [gone](c.md) [bad](b.md#nope) [web](https://x.y/#z)\n\n```\n[in code](nowhere.md)\n```\n",
		"docs/b.md":            "# B\n\n## The `rules`\n",
		"website/content/p.md": "[site](/schema/)",
	}.fs()
	got := strings.Join(checkMarkdownLinks(fsys), "\n")
	want := "docs/a.md: b.md#nope: docs/b.md has no heading #nope\ndocs/a.md: c.md: no such file"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if githubSlug("CI time estimates (Linux)") != "ci-time-estimates-linux" {
		t.Error(githubSlug("CI time estimates (Linux)"))
	}
}
