package docs

import (
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

var (
	attrURL   = regexp.MustCompile(`(?i)\s(?:href|src)=("[^"]*"|'[^']*'|[^\s>]+)`)
	attrID    = regexp.MustCompile(`(?i)\sid=("[^"]*"|'[^']*'|[^\s>]+)`)
	repoFile  = regexp.MustCompile(`^https://github\.com/nao1215/gozen/(?:tree|blob)/main/([^?#]*)`)
	baseURLRe = regexp.MustCompile(`(?m)^baseURL\s*=\s*"([^"]+)"`)
)

func unquote(v string) string {
	return html.UnescapeString(strings.Trim(v, `"'`))
}

// TestSiteLinks checks the built website (GOZEN_SITE_DIR, after make website):
// every internal link and picture leads to a file of the site, every #fragment
// to an id of its page, and every link to a file of this repository on GitHub
// to a file of the checkout. Run it with make website-check.
func TestSiteLinks(t *testing.T) {
	dir := os.Getenv("GOZEN_SITE_DIR")
	if dir == "" {
		t.Skip("set GOZEN_SITE_DIR to the built site (make website-check)")
	}
	conf, err := os.ReadFile(filepath.Join(root, "website", "hugo.toml"))
	if err != nil {
		t.Fatal(err)
	}
	m := baseURLRe.FindSubmatch(conf)
	if m == nil {
		t.Fatal("website/hugo.toml has no baseURL")
	}
	bad := checkSite(os.DirFS(dir), string(m[1]), func(p string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		return err == nil
	})
	if len(bad) > 0 {
		t.Fatalf("%d broken links in the built site:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// checkSite reports the broken links of a built site served at base.
// inRepo reports whether a repository path exists.
func checkSite(site fs.FS, base string, inRepo func(string) bool) []string {
	u, err := url.Parse(base)
	if err != nil {
		return []string{err.Error()}
	}
	basePath := strings.TrimSuffix(u.Path, "/") + "/"
	ids := map[string]map[string]bool{} // page file -> ids
	pages := map[string]string{}
	var bad []string
	if err := fs.WalkDir(site, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".html") {
			b, err := fs.ReadFile(site, p)
			if err != nil {
				return err
			}
			pages[p] = string(b)
			set := map[string]bool{}
			for _, m := range attrID.FindAllStringSubmatch(string(b), -1) {
				set[unquote(m[1])] = true
			}
			ids[p] = set
		}
		return nil
	}); err != nil {
		return []string{err.Error()}
	}
	// file returns the site file a site path is served from, or "".
	file := func(p string) string {
		p = strings.TrimPrefix(p, "/")
		for _, c := range []string{p, path.Join(p, "index.html")} {
			if c == "" {
				c = "index.html"
			}
			if st, err := fs.Stat(site, c); err == nil && !st.IsDir() {
				return c
			}
		}
		if p == "" {
			return "index.html"
		}
		return ""
	}
	names := make([]string, 0, len(pages))
	for p := range pages {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, page := range names {
		for _, m := range attrURL.FindAllStringSubmatch(pages[page], -1) {
			raw := unquote(m[1])
			report := func(why string) { bad = append(bad, fmt.Sprintf("%s: %s: %s", page, raw, why)) }
			if rm := repoFile.FindStringSubmatch(raw); rm != nil {
				if p := strings.TrimSuffix(rm[1], "/"); !inRepo(p) {
					report("no such file in the repository")
				}
				continue
			}
			target := raw
			if strings.HasPrefix(target, base) {
				target = basePath + strings.TrimPrefix(target, base)
			}
			lower := strings.ToLower(target)
			if strings.Contains(lower, "://") || strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "javascript:") {
				continue
			}
			ref, err := url.Parse(target)
			if err != nil {
				report(err.Error())
				continue
			}
			var sitePath string
			switch {
			case ref.Path == "":
				sitePath = "/" + page
			case strings.HasPrefix(ref.Path, "/"):
				if !strings.HasPrefix(ref.Path, basePath) {
					report("an absolute path outside " + basePath + " breaks on GitHub Pages")
					continue
				}
				sitePath = "/" + strings.TrimPrefix(ref.Path, basePath)
			default:
				sitePath = "/" + path.Join(path.Dir(page), ref.Path)
				if strings.HasSuffix(ref.Path, "/") {
					sitePath += "/"
				}
			}
			// Pagefind writes its files after Hugo (website.yml).
			if strings.HasPrefix(sitePath, "/pagefind/") {
				continue
			}
			f := file(sitePath)
			if f == "" {
				report("no such page or file")
				continue
			}
			if ref.Fragment != "" && strings.HasSuffix(f, ".html") && !ids[f][ref.Fragment] {
				report(fmt.Sprintf("%s has no id %q", f, ref.Fragment))
			}
		}
	}
	return bad
}

func TestCheckSiteFindsBrokenLinks(t *testing.T) {
	site := fstest.MapFS{
		"index.html":         {Data: []byte(`<a href=/gozen/install/>i</a><a href="/gozen/install/#ok">o</a><a href=#top id=top>t</a><img src=/gozen/images/a.jpg>`)},
		"install/index.html": {Data: []byte(`<h2 id=ok>x</h2><a href=../gone/>g</a><a href="#nope">n</a><a href=/install/>abs</a>`)},
		"images/a.jpg":       {Data: []byte("x")},
		"404.html":           {Data: []byte(`<a href="https://nao1215.github.io/gozen/missing/">m</a><a href="https://github.com/nao1215/gozen/blob/main/gone.md">r</a><a href="https://github.com/nao1215/gozen/tree/main/examples/">e</a><script src=/gozen/pagefind/pagefind-ui.js></script>`)},
	}
	got := strings.Join(checkSite(site, "https://nao1215.github.io/gozen/", func(p string) bool { return p == "examples" }), "\n")
	for _, want := range []string{
		"install/index.html: ../gone/: no such page or file",
		`install/index.html: #nope: install/index.html has no id "nope"`,
		"install/index.html: /install/: an absolute path outside /gozen/",
		"404.html: https://nao1215.github.io/gozen/missing/: no such page or file",
		"404.html: https://github.com/nao1215/gozen/blob/main/gone.md: no such file in the repository",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "index.html: /gozen/install/") || strings.Contains(got, "a.jpg") || strings.Contains(got, "examples") || strings.Contains(got, "pagefind") || strings.Contains(got, "#top") {
		t.Errorf("a good link was reported:\n%s", got)
	}
}

// TestReadmeLinks checks that the relative links of README.md lead to files of
// the repository, and its links to the site to pages of the site's content.
func TestReadmeLinks(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`\]\(([^)\s]+)\)|href="([^"]+)"`)
	site := "https://nao1215.github.io/gozen/"
	for _, m := range link.FindAllStringSubmatch(string(b), -1) {
		target := m[1] + m[2]
		switch {
		case strings.HasPrefix(target, "#"):
		case strings.HasPrefix(target, site):
			page := strings.Trim(strings.SplitN(strings.TrimPrefix(target, site), "#", 2)[0], "/")
			if page == "" || page == "schema" {
				continue // the home page and the page built from data/schema/types.schema.json
			}
			if _, err := os.Stat(filepath.Join(root, "website", "content", page+".md")); err != nil { //nolint:gosec // A page named by README.md, looked up in this repository.
				t.Errorf("README.md: %s: no page website/content/%s.md", target, page)
			}
		case repoFile.MatchString(target):
			p := strings.TrimSuffix(repoFile.FindStringSubmatch(target)[1], "/")
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil { //nolint:gosec // A path named by README.md, looked up in this repository.
				t.Errorf("README.md: %s: no such file", target)
			}
		case strings.Contains(target, "://"):
		default:
			p := strings.SplitN(target, "#", 2)[0]
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil { //nolint:gosec // A path named by README.md, looked up in this repository.
				t.Errorf("README.md: %s: no such file", target)
			}
		}
	}
}
