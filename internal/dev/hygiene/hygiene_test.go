package hygiene_test

import (
	"io/fs"
	"os"
	"regexp"
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
