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

// repositoryFiles calls visit with the slash-separated path of every file of the
// checkout; built or ignored outputs are skipped.
func repositoryFiles(t *testing.T, visit func(name string)) {
	t.Helper()
	err := fs.WalkDir(os.DirFS("../../.."), ".", func(name string, d fs.DirEntry, walkErr error) error {
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
		if name != ".git" { // a linked worktree's pointer to its repository
			visit(name)
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
	repositoryFiles(t, func(name string) {
		contents, err := fs.ReadFile(fsy, name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.ContainsRune(contents, 0) {
			visit(name, contents)
		}
	})
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
internal/model/gamedata/check_scenes_more_test.go
internal/model/gamedata/shapes_more_test.go
internal/rules/atlas/more_test.go
internal/rules/battle/states_more_test.go
internal/rules/dungeon/host_more_test.go
internal/rules/event/more_test.go
internal/screen/play/autopilot_cov2_test.go
internal/screen/play/battle_cov2_test.go
internal/screen/play/battle_cov3_test.go
internal/screen/play/battle_cov4_test.go
internal/screen/play/battle_cov5_test.go
internal/screen/play/battle_cov6_test.go
internal/screen/play/battle_cov7_test.go
internal/screen/play/battle_cov8_test.go
internal/screen/play/battle_fx_cov_test.go
internal/screen/play/battle_hooks_cov_test.go
internal/screen/play/battle_replay_cov2_test.go
internal/screen/play/battle_replay_cov_test.go
internal/screen/play/cinema_cov_test.go
internal/screen/play/core_cov_test.go
internal/screen/play/draw_cov_test.go
internal/screen/play/extras_cov_test.go
internal/screen/play/fast_cov_test.go
internal/screen/play/field_cov_test.go
internal/screen/play/floor_cov_test.go
internal/screen/play/fuzz_cov_test.go
internal/screen/play/fuzzread_cov_test.go
internal/screen/play/graphs_cov_test.go
internal/screen/play/helpers_cov_test.go
internal/screen/play/language_cov_test.go
internal/screen/play/misc_cov_test.go
internal/screen/play/mnu_bug_fusion_cov_test.go
internal/screen/play/mnu_bug_tree_cov_test.go
internal/screen/play/mnu_cgreview_cov_test.go
internal/screen/play/mnu_extra_cov_test.go
internal/screen/play/mnu_extrahelp_cov_test.go
internal/screen/play/mnu_gallery2_cov_test.go
internal/screen/play/mnu_gallery3_cov_test.go
internal/screen/play/mnu_gallery_cov_test.go
internal/screen/play/mnu_profiles_cov_test.go
internal/screen/play/mnu_shop_cov_test.go
internal/screen/play/new_cov_test.go
internal/screen/play/novel2_cov_test.go
internal/screen/play/novel_cov_test.go
internal/screen/play/progress_cov_test.go
internal/screen/play/prop_cov_test.go
internal/screen/play/replay_cov_test.go
internal/screen/play/requests_cov_test.go
internal/screen/play/scenecheck_cov_test.go
internal/screen/play/scenes_cov_test.go
internal/screen/play/startat_cov_test.go
internal/screen/play/tactics_cov2_test.go
internal/screen/play/tactics_cov_test.go
internal/screen/play/talk_cov_test.go
internal/screen/play/title_cov_test.go
internal/screen/play/title_flow_cov_test.go
internal/screen/play/titlemusic_cov_test.go
internal/screen/play/trace3_cov_test.go
internal/screen/play/trace_act_cov_test.go
internal/screen/play/trace_fuzz_cov_test.go
internal/screen/play/trace_scene_cov_test.go
internal/screen/play/ui_cov_test.go
internal/screen/play/unicode_cov_test.go
internal/screen/play/walk_cov_test.go
internal/screen/play/world_cov_test.go
internal/screen/play/world_fixture_cov_test.go
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

// conventionalTests are test file names that Go or its tooling gives a meaning of their
// own, so they need no implementation file of the same name. The value is the reason.
// A catch-all name (contracts, helpers, the package's own name) is not a convention:
// it stays in the baseline until its tests move beside what they test.
var conventionalTests = map[string]string{
	"example_test.go": "holds the package's Example functions, which go doc shows and go test runs",
	"export_test.go":  "exposes internals to the package's external tests and holds no test of its own",
	"fuzz_test.go":    "holds the package's Fuzz targets, which the fuzz workflow discovers and runs each night",
}

// buildSuffix matches one trailing platform or build word of a file stem: the
// operating systems and architectures Go reads from a file name, and the words this
// repository uses for a build-tagged variant (cgo, nocgo, other).
var buildSuffix = regexp.MustCompile(`_(?:aix|android|darwin|dragonfly|freebsd|illumos|ios|js|linux|netbsd|openbsd|plan9|solaris|unix|wasip1|windows|386|amd64|arm|arm64|loong64|mips|mips64|mips64le|mipsle|ppc64|ppc64le|riscv64|s390x|wasm|cgo|nocgo|other)$`)

// orphanTests returns, sorted, the test files among the given paths that are named
// after no implementation file of their directory. foo_test.go belongs to foo.go, and
// a platform word may be on either side: foo_linux_test.go belongs to foo_linux.go or
// foo.go, and foo_test.go to foo_linux.go. Integration tests (*_integration_test.go)
// and conventionalTests belong to no single file.
func orphanTests(files []string) []string {
	stems := map[string]bool{} // directory + "/" + stem of an implementation file
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for stem, cut := strings.TrimSuffix(name, ".go"), ""; stem != cut; stem = buildSuffix.ReplaceAllString(stem, "") {
			stems[stem], cut = true, stem
		}
	}
	var orphans []string
	for _, name := range files {
		base := path.Base(name)
		if !strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_integration_test.go") || conventionalTests[base] != "" {
			continue
		}
		owned := false
		for stem, cut := strings.TrimSuffix(name, "_test.go"), ""; stem != cut && !owned; stem = buildSuffix.ReplaceAllString(stem, "") {
			owned, cut = stems[stem], stem
		}
		if !owned {
			orphans = append(orphans, name)
		}
	}
	slices.Sort(orphans)
	return orphans
}

// testStemBaselineLimit is the size of testStemBaseline when the rule was added.
const testStemBaselineLimit = 205

// testStemBaseline lists the test files under cmd/ and internal/ that had no
// implementation file of their name when the rule was added, apart from those already
// in testFileBaseline. Like that list it may only shrink: delete a line when its file is
// renamed after the implementation it tests or merged into that file's test. One path a
// line keeps unrelated changes from touching the same line.
const testStemBaseline = `
cmd/gozen/check_cli_test.go
cmd/gozen/check_offers_test.go
cmd/gozen/design_walk_test.go
cmd/gozen/dispatch_test.go
cmd/gozen/internal/designeval/pick_test.go
cmd/gozen/internal/designeval/return_test.go
cmd/gozen/robust_test.go
cmd/gozen/sim_cli_test.go
cmd/gozen/sim_state_test.go
cmd/gozen/trial_cli_test.go
cmd/internal/distribute/reuse_test.go
cmd/internal/trial/battle_hooks_test.go
cmd/internal/trial/build_test.go
cmd/internal/trial/faces_test.go
cmd/internal/trial/loop_test.go
cmd/internal/trial/tiles_test.go
internal/base/assetfs/helpers_test.go
internal/base/chat/ownership_test.go
internal/base/chat/yamlpath_test.go
internal/base/diag/usage_test.go
internal/base/expr/bare_test.go
internal/base/expr/fuzz_abstract_test.go
internal/base/expr/literal_test.go
internal/base/expr/property_test.go
internal/base/react/react_test.go
internal/base/react/yamlpath_test.go
internal/base/savefile/contracts_test.go
internal/base/savefile/errors_test.go
internal/dev/docs/adr_links_test.go
internal/dev/docs/blocks_test.go
internal/dev/docs/compare_test.go
internal/dev/docs/gifjoin_test.go
internal/dev/docs/gifscene_test.go
internal/dev/docs/gifslim_test.go
internal/dev/docs/image_inputs_test.go
internal/dev/docs/image_selection_test.go
internal/dev/docs/images_test.go
internal/dev/docs/language_test.go
internal/dev/docs/manifest_test.go
internal/dev/docs/mdlinks_test.go
internal/dev/docs/schema_test.go
internal/dev/docs/site_test.go
internal/dev/docs/steps_test.go
internal/dev/hygiene/comments_language_test.go
internal/dev/hygiene/fitlines_test.go
internal/dev/hygiene/hygiene_test.go
internal/drive/headless/balance/battle_hooks_test.go
internal/drive/headless/balance/hard_test.go
internal/drive/headless/balance/run_test.go
internal/drive/headless/battle_hooks_test.go
internal/drive/headless/contracts_test.go
internal/drive/headless/decision_test.go
internal/drive/headless/edge_test.go
internal/drive/headless/monster_explore_test.go
internal/drive/headless/save_test.go
internal/drive/headless/scenes_test.go
internal/drive/headless/standin_test.go
internal/drive/headless/tactics_test.go
internal/drive/session/battle_forms_session_test.go
internal/drive/session/battle_hooks_test.go
internal/drive/session/fuzz_do_test.go
internal/drive/session/lang_test.go
internal/drive/session/monster_session_test.go
internal/drive/session/profiles_test.go
internal/drive/session/request_ownership_test.go
internal/drive/session/resume_errors_test.go
internal/drive/session/resume_test.go
internal/drive/session/scenes_test.go
internal/drive/session/staging_test.go
internal/drive/session/verbs_errors_test.go
internal/drive/session/verbs_test.go
internal/drive/session/voice_test.go
internal/model/gamedata/auto_tactic_test.go
internal/model/gamedata/battle_hooks_contract_test.go
internal/model/gamedata/bestiary_test.go
internal/model/gamedata/call_share_test.go
internal/model/gamedata/carry_test.go
internal/model/gamedata/check_paths_test.go
internal/model/gamedata/command_registry_test.go
internal/model/gamedata/data_test.go
internal/model/gamedata/docs_example_test.go
internal/model/gamedata/drawn_floors_test.go
internal/model/gamedata/entry_ids_test.go
internal/model/gamedata/entrydrag_test.go
internal/model/gamedata/extreme_test.go
internal/model/gamedata/flags_test.go
internal/model/gamedata/forms_check_test.go
internal/model/gamedata/funcvalue_test.go
internal/model/gamedata/fuzz_scenes_test.go
internal/model/gamedata/gallery_test.go
internal/model/gamedata/lineage_cycle_internal_test.go
internal/model/gamedata/merge_test.go
internal/model/gamedata/novel_test.go
internal/model/gamedata/outfit_test.go
internal/model/gamedata/phone_test.go
internal/model/gamedata/planning_version_test.go
internal/model/gamedata/profile_face_test.go
internal/model/gamedata/schema_behavior_test.go
internal/model/gamedata/schema_english_test.go
internal/model/gamedata/schema_test.go
internal/model/gamedata/security_test.go
internal/model/gamedata/zero_test.go
internal/model/rpg/auto_tactics_test.go
internal/model/rpg/expr_helper_test.go
internal/model/rpg/monster_test.go
internal/model/rpg/planning_reading_test.go
internal/model/rpg/presets_test.go
internal/model/rpg/rand_test.go
internal/model/rpg/rpg_test.go
internal/model/rpg/setpath_test.go
internal/model/rpg/state_buy_test.go
internal/model/rpg/state_take_test.go
internal/model/rpg/views_save_test.go
internal/mosaic/command/auto_test.go
internal/mosaic/command/classes_source_test.go
internal/mosaic/command/classes_test.go
internal/mosaic/command/contracts_test.go
internal/mosaic/command/fakes_test.go
internal/mosaic/command/inference_test.go
internal/mosaic/command/inspect_redetect_test.go
internal/mosaic/command/seg_test.go
internal/mosaic/command/tagcache_test.go
internal/mosaic/command/view_test.go
internal/mosaic/contracts_test.go
internal/mosaic/decodeguard_test.go
internal/mosaic/detect/contracts_test.go
internal/mosaic/detect/passes_test.go
internal/mosaic/detect/session_cgo_test.go
internal/mosaic/segment/session_cgo_test.go
internal/mosaic/shapes_test.go
internal/mosaic/tagger/session_cgo_test.go
internal/promptgen/contracts_test.go
internal/promptgen/decodeguard_test.go
internal/promptgen/sdxl_test.go
internal/rules/atlas/limits_test.go
internal/rules/battle/actor_tactics_test.go
internal/rules/battle/auto_tactics_test.go
internal/rules/battle/campaign_test.go
internal/rules/battle/effects_test.go
internal/rules/battle/foes_test.go
internal/rules/battle/order_test.go
internal/rules/battle/passive_test.go
internal/rules/battle/presence_test.go
internal/rules/battle/tactic_test.go
internal/rules/battle/zero_test.go
internal/rules/dungeon/battle_music_test.go
internal/rules/dungeon/fixed_test.go
internal/rules/dungeon/gen_fuzz_test.go
internal/rules/dungeon/host_test.go
internal/rules/dungeon/hunt_test.go
internal/rules/dungeon/limits_test.go
internal/rules/dungeon/looks_test.go
internal/rules/dungeon/openwall_test.go
internal/rules/dungeon/presence_test.go
internal/rules/dungeon/tiles_test.go
internal/rules/dungeon/walked_test.go
internal/rules/event/aside_test.go
internal/rules/event/command_test.go
internal/rules/event/count_test.go
internal/rules/event/image_pick_test.go
internal/rules/event/join_test.go
internal/rules/event/language_test.go
internal/rules/event/leave_test.go
internal/rules/event/levelup_test.go
internal/rules/event/phone_test.go
internal/rules/event/readkey_test.go
internal/rules/event/set_contract_test.go
internal/rules/event/set_family_test.go
internal/rules/event/staging_test.go
internal/screen/doll/doll_test.go
internal/screen/gfx/contracts_test.go
internal/screen/gfx/page_limits_test.go
internal/screen/play/battle_cov_helpers_test.go
internal/screen/play/battle_forms_test.go
internal/screen/play/battle_hooks_test.go
internal/screen/play/battle_limits_test.go
internal/screen/play/battle_sample_layout_test.go
internal/screen/play/battle_say_trace_test.go
internal/screen/play/call_graphics_test.go
internal/screen/play/cg_skip_test.go
internal/screen/play/contracts_test.go
internal/screen/play/entry_bust_layout_test.go
internal/screen/play/entry_dot_test.go
internal/screen/play/entry_parts_graphics_test.go
internal/screen/play/graphics_campaign_test.go
internal/screen/play/graphics_novel_test.go
internal/screen/play/graphics_simulate_test.go
internal/screen/play/graphics_space_test.go
internal/screen/play/graphics_stage_test.go
internal/screen/play/graphics_streams_test.go
internal/screen/play/graphics_test.go
internal/screen/play/graphics_toast_test.go
internal/screen/play/lang_test.go
internal/screen/play/phone_save_test.go
internal/screen/play/preload_test.go
internal/screen/play/resume_test.go
internal/screen/play/screen_home_test.go
internal/screen/play/staging_lifecycle_test.go
internal/screen/play/walk_replay_test.go
internal/screen/play/workspace_perf_test.go
internal/screen/puppet/graphics_test.go
internal/screen/puppet/puppet_test.go
internal/screen/sound/contracts_test.go
internal/screen/sound/fade_test.go
internal/screen/stage/stage_test.go
`

// TestTestFileStems applies the other half of the AGENTS.md naming rule to cmd/ and
// internal/: a unit test file has an implementation file of its name beside it. The
// sample games under examples/ are YAML and assets with tests but no Go implementation,
// so the rule stops at the two source trees.
func TestTestFileStems(t *testing.T) {
	t.Parallel()
	baseline := strings.Fields(testStemBaseline)
	if !slices.IsSorted(baseline) || len(slices.Compact(slices.Clone(baseline))) != len(baseline) {
		t.Fatal("test stem baseline must contain sorted, unique paths")
	}
	if len(baseline) > testStemBaselineLimit {
		t.Fatal("test stem baseline may only shrink")
	}
	var files []string
	repositoryFiles(t, func(name string) {
		if (strings.HasPrefix(name, "cmd/") || strings.HasPrefix(name, "internal/")) && !strings.Contains(name, "/testdata/") {
			files = append(files, name)
		}
	})
	vague := strings.Fields(testFileBaseline)
	found := map[string]bool{}
	for _, name := range orphanTests(files) {
		if slices.Contains(vague, name) { // already debt of TestTestFileNames; listed once
			continue
		}
		found[name] = true
		if !slices.Contains(baseline, name) {
			t.Errorf("%s: no implementation file of that name beside it; name the test file after the file it tests (foo.go is tested in foo_test.go) or, for a test of several files, *_integration_test.go", name)
		}
	}
	for _, name := range baseline {
		if !found[name] {
			t.Errorf("%s: no longer an orphan; remove it from the test stem baseline", name)
		}
	}
}

// TestTestFileStemsGuard proves the scanner on a sample tree.
func TestTestFileStemsGuard(t *testing.T) {
	t.Parallel()
	got := orphanTests([]string{
		"cmd/tool/main.go", "cmd/tool/main_test.go",
		"cmd/tool/play_integration_test.go", "cmd/tool/fuzz_test.go", "cmd/tool/export_test.go", "cmd/tool/example_test.go",
		"cmd/tool/entry_linux.go", "cmd/tool/entry_linux_test.go", "cmd/tool/entry_test.go", "cmd/tool/entry_windows_amd64_test.go",
		"cmd/tool/faces.go", "cmd/tool/faces_nocgo_test.go",
		"cmd/tool/stray_test.go", "cmd/tool/main_cli_test.go", "cmd/tool/contracts_test.go", "cmd/tool/stray_linux_test.go",
		"cmd/other/stray.go", "cmd/other/main_test.go",
		"cmd/tool/notes.md", "cmd/tool/data_test.json",
	})
	want := []string{"cmd/other/main_test.go", "cmd/tool/contracts_test.go", "cmd/tool/main_cli_test.go", "cmd/tool/stray_linux_test.go", "cmd/tool/stray_test.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("orphans = %q, want %q", got, want)
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
