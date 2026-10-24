package hygiene_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// These are generic location formats, never a project-specific word list.
func locationPatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`/(?:home|Users)/[A-Za-z0-9][A-Za-z0-9._-]*/`),
		regexp.MustCompile(`[A-Za-z]:[\\/]Users[\\/][A-Za-z0-9][A-Za-z0-9._-]*[\\/]`),
		regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9-]*\.(?:local|lan|internal|corp)\b`),
	}
}

// locationHints are the literal parts every match of locationPatterns contains (the home
// directory, the Users directory, the private domain). A file without any of them cannot
// match, so the walk below skips the regular expressions for most of the repository (the
// pictures and sounds among it) without changing what it finds.
var locationHints = [][]byte{[]byte("/home/"), []byte("Users"), []byte(".local"), []byte(".lan"), []byte(".internal"), []byte(".corp")}

func mayHaveLocation(contents []byte) bool {
	for _, h := range locationHints {
		if bytes.Contains(contents, h) {
			return true
		}
	}
	return false
}

func TestLocationHints(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{
		"/" + "home" + "/demo/project",
		"/" + "Users" + "/demo/project",
		"C:" + `\` + "Users" + `\demo\project`,
		"build-host" + "." + "internal",
		"printer" + "." + "local",
		"nas" + "." + "lan",
		"git" + "." + "corp",
	} {
		matched := false
		for _, p := range locationPatterns() {
			matched = matched || p.MatchString(sample)
		}
		if !matched || !mayHaveLocation([]byte(sample)) {
			t.Errorf("%q: pattern match %v, hint %v; want both", sample, matched, mayHaveLocation([]byte(sample)))
		}
	}
}

func TestGenericLocationPatterns(t *testing.T) {
	t.Parallel()
	for i, sample := range []string{
		"/" + "home" + "/demo/project",
		"C:" + `\` + "Users" + `\demo\project`,
		"build-host" + "." + "internal",
	} {
		if !locationPatterns()[i].MatchString(sample) {
			t.Errorf("pattern %d failed to detect a generic private location", i)
		}
	}
}

func TestRepositoryLocations(t *testing.T) {
	t.Parallel()
	patterns := locationPatterns()
	fsy := os.DirFS("../../..")
	err := fs.WalkDir(fsy, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if name == "website/public" { // the built site (git ignores it), the browser build among it
				return fs.SkipDir
			}
			switch d.Name() {
			case ".git", ".agents", ".codex", ".aws", ".cache", "bin", "build", "dist", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if name == ".git" { // a linked worktree's pointer to its repository
			return nil
		}
		contents, err := fs.ReadFile(fsy, name)
		if err != nil {
			return err
		}
		if !mayHaveLocation(contents) {
			return nil
		}
		for _, pattern := range patterns {
			if loc := pattern.FindIndex(contents); loc != nil {
				t.Errorf("%s: generic private location at byte %d; remove machine-specific data", name, loc[0])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestApplicationPackagesAreInternal scans source independently of build tags and OS.
// Only executables may live outside an internal directory; tests may assemble layers.
func TestApplicationPackagesAreInternal(t *testing.T) {
	t.Parallel()
	source := os.DirFS("../../..")
	err := fs.WalkDir(source, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", ".codex", ".aws", ".cache", "bin", "build", "dist", "vendor", "node_modules":
				return fs.SkipDir
			}
			if name == "website/public" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.Contains("/"+name, "/internal/") {
			return nil
		}
		b, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, b, parser.PackageClauseOnly)
		if err != nil {
			return err
		}
		if f.Name.Name != "main" {
			t.Errorf("%s: package %s exposes application code outside internal", name, f.Name.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
