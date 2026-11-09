package hygiene_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// testFileBaselineLimit is the size of testFileBaseline when the rule was added.
const testFileBaselineLimit = 69

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

// repositoryTexts calls visit with every text file of the checkout: files holding a
// NUL byte (pictures, sounds, archives) and built or ignored outputs are skipped.
func repositoryTexts(t *testing.T, visit func(name string, contents []byte)) {
	t.Helper()
	fsy := os.DirFS("../../..")
	err := fs.WalkDir(fsy, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Generated output lives in the top-level bin, build and dist; a source
			// directory of the same name further down is read.
			switch name {
			case "website/public", "website/resources", "bin", "build", "dist":
				return fs.SkipDir
			}
			switch d.Name() {
			case ".git", ".agents", ".codex", ".aws", ".cache", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if name == ".git" {
			return nil
		}
		contents, err := fs.ReadFile(fsy, name)
		if err != nil {
			return err
		}
		if !bytes.ContainsRune(contents, 0) {
			visit(name, contents)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// removedFeatures are names of features that were deliberately removed. They must not
// come back as code, YAML, schema, documentation or fixtures; only the changelog keeps
// their history. Returning one needs a new ADR, which also deletes its line here.
var removedFeatures = []string{
	// The casual-game runner: runners in game.yaml, the run_game command and schema 12.
	"run_game", "RunnerDefinition", "SchemaVersion12", "rules/casual", "0064-casual-game-runtime", "casual-activities",
	// The gozen migrate command; YAML and saves have no migration tools before v1.0.0.
	"gozen migrate",
}

// removedYAMLWords are game-data YAML keys of removed features. Unlike the names above
// they are ordinary words (an atago spec has its own runners), so only game-data tags
// and generated schema properties are checked.
var removedYAMLWords = []string{"runners"}

// TestRemovedFeatures keeps removed features from returning through leftover references.
func TestRemovedFeatures(t *testing.T) {
	t.Parallel()
	repositoryTexts(t, func(name string, contents []byte) {
		// The changelog, decision and review records keep the history; this list names them.
		if name == "CHANGELOG.md" || name == "internal/dev/hygiene/hygiene_test.go" || strings.HasPrefix(name, "docs/decisions/") || strings.HasPrefix(name, "docs/reviews/") {
			return
		}
		for _, word := range removedFeatures {
			if i := bytes.Index(contents, []byte(word)); i >= 0 {
				t.Errorf("%s: byte %d names the removed feature %q; remove the leftover reference", name, i, word)
			}
		}
		if strings.HasPrefix(name, "data/schema/") && strings.HasSuffix(name, ".json") {
			for _, word := range removedYAMLWords {
				if bytes.Contains(contents, []byte(`"`+word+`":`)) {
					t.Errorf("%s: the schema still has the removed YAML key %q", name, word)
				}
			}
		}
		if strings.HasPrefix(name, "internal/model/gamedata/") && strings.HasSuffix(name, ".go") {
			for _, word := range removedYAMLWords {
				if bytes.Contains(contents, []byte(`yaml:"`+word+`"`)) || bytes.Contains(contents, []byte(`yaml:"`+word+`,`)) {
					t.Errorf("%s: game data still reads the removed YAML key %q", name, word)
				}
			}
		}
	})
}

// testFileBaseline lists test files named before the naming rule (AGENTS.md); they are
// debt to consolidate into the file named after their implementation. The list may only
// shrink: delete a line when its file is merged or renamed.
const testFileBaseline = `
internal/base/geom/more_test.go
internal/base/memo/more_test.go
internal/battle/states_more_test.go
internal/event/more_test.go
internal/model/gamedata/check_scenes_more_test.go
internal/model/gamedata/shapes_more_test.go
internal/play/autopilot_cov2_test.go
internal/play/battle_cov2_test.go
internal/play/battle_cov3_test.go
internal/play/battle_cov4_test.go
internal/play/battle_cov5_test.go
internal/play/battle_cov6_test.go
internal/play/battle_cov7_test.go
internal/play/battle_cov8_test.go
internal/play/battle_fx_cov_test.go
internal/play/battle_hooks_cov_test.go
internal/play/battle_replay_cov2_test.go
internal/play/battle_replay_cov_test.go
internal/play/cinema_cov_test.go
internal/play/core_cov_test.go
internal/play/draw_cov_test.go
internal/play/extras_cov_test.go
internal/play/fast_cov_test.go
internal/play/field_cov_test.go
internal/play/floor_cov_test.go
internal/play/fuzz_cov_test.go
internal/play/fuzzread_cov_test.go
internal/play/graphs_cov_test.go
internal/play/helpers_cov_test.go
internal/play/language_cov_test.go
internal/play/misc_cov_test.go
internal/play/mnu_bug_fusion_cov_test.go
internal/play/mnu_bug_tree_cov_test.go
internal/play/mnu_cgreview_cov_test.go
internal/play/mnu_extra_cov_test.go
internal/play/mnu_extrahelp_cov_test.go
internal/play/mnu_gallery2_cov_test.go
internal/play/mnu_gallery3_cov_test.go
internal/play/mnu_gallery_cov_test.go
internal/play/mnu_profiles_cov_test.go
internal/play/mnu_shop_cov_test.go
internal/play/new_cov_test.go
internal/play/novel2_cov_test.go
internal/play/novel_cov_test.go
internal/play/progress_cov_test.go
internal/play/prop_cov_test.go
internal/play/replay_cov_test.go
internal/play/requests_cov_test.go
internal/play/scenecheck_cov_test.go
internal/play/scenes_cov_test.go
internal/play/startat_cov_test.go
internal/play/tactics_cov2_test.go
internal/play/tactics_cov_test.go
internal/play/talk_cov_test.go
internal/play/title_cov_test.go
internal/play/title_flow_cov_test.go
internal/play/titlemusic_cov_test.go
internal/play/trace3_cov_test.go
internal/play/trace_act_cov_test.go
internal/play/trace_fuzz_cov_test.go
internal/play/trace_scene_cov_test.go
internal/play/ui_cov_test.go
internal/play/unicode_cov_test.go
internal/play/walk_cov_test.go
internal/play/world_cov_test.go
internal/play/world_fixture_cov_test.go
internal/rules/atlas/more_test.go
internal/rules/dungeon/host_more_test.go
internal/screen/puppet/more_test.go
`

// vagueTest matches coverage-driven test file names: more, extra, cov and coverage
// suffixes, optionally numbered.
var vagueTest = regexp.MustCompile(`(?:^|_)(?:more|extra|cov|coverage)\d*_test\.go$`)

// numberedTest matches a numbered test file, which is vague unless an implementation
// file has the same number (vocab2.go is tested in vocab2_test.go).
var numberedTest = regexp.MustCompile(`\d_test\.go$`)

// TestTestFileNames applies the AGENTS.md naming rule: a unit test file is named after
// its implementation, never with a vague or numbered coverage suffix.
func TestTestFileNames(t *testing.T) {
	t.Parallel()
	baseline := strings.Fields(testFileBaseline)
	if !slices.IsSorted(baseline) || len(slices.Compact(slices.Clone(baseline))) != len(baseline) {
		t.Fatal("test file baseline must contain sorted, unique paths")
	}
	if len(baseline) > testFileBaselineLimit {
		t.Fatal("test file baseline may only shrink")
	}
	found := map[string]bool{}
	fsy := os.DirFS("../../..")
	err := fs.WalkDir(fsy, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", ".codex", ".aws", ".cache", "bin", "build", "dist", "vendor", "node_modules", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		base := path.Base(name)
		vague := vagueTest.MatchString(base)
		if !vague && numberedTest.MatchString(base) {
			_, err := fs.Stat(fsy, strings.TrimSuffix(name, "_test.go")+".go")
			vague = err != nil
		}
		if !vague {
			return nil
		}
		found[name] = true
		if !slices.Contains(baseline, name) {
			t.Errorf("%s: name the test file after its implementation (foo.go is tested in foo_test.go) or extend that file", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range baseline {
		if !found[name] {
			t.Errorf("%s: renamed or merged; remove it from the test file baseline", name)
		}
	}
}

// TestFixturesAreReferenced rejects stray E2E fixtures and scripts: each entry of
// e2e/atago/fixtures and each file of scripts/ and bench/ must be named by another file.
func TestFixturesAreReferenced(t *testing.T) {
	t.Parallel()
	texts := map[string][]byte{}
	repositoryTexts(t, func(name string, contents []byte) { texts[name] = contents })
	var wanted []string
	for _, dir := range []string{"e2e/atago/fixtures", "scripts", "bench"} {
		entries, err := os.ReadDir("../../../" + dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			wanted = append(wanted, dir+"/"+e.Name())
		}
	}
	for _, entry := range wanted {
		// A fixture is named by its path below the fixtures directory, a script by its
		// path from the repository root or, from its own directory, by a path ending in
		// its name; a common word such as "web" alone is no reference.
		word := entry
		if strings.HasPrefix(entry, "e2e/atago/fixtures/") {
			word = "fixtures/" + path.Base(entry)
		}
		const end = `(?:$|[^A-Za-z0-9._-])`
		full := regexp.MustCompile(`(?:^|[^A-Za-z0-9._-])` + regexp.QuoteMeta(word) + end)
		near := regexp.MustCompile(`/` + regexp.QuoteMeta(path.Base(entry)) + end)
		referenced := false
		for name, contents := range texts {
			named := bytes.Contains(contents, []byte(word)) && full.Match(contents) ||
				path.Dir(name) == path.Dir(entry) && bytes.Contains(contents, []byte(path.Base(entry))) && near.Match(contents)
			if name != entry && !strings.HasPrefix(name, entry+"/") && named {
				referenced = true
				break
			}
		}
		if !referenced {
			t.Errorf("%s: nothing names %q; delete the stray file or reference it where it is used", entry, word)
		}
	}
}
