package hygiene_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/dev/hygiene"
)

const module = "github.com/nao1215/gozen/"

func sourceFiles(t *testing.T, visit func(string, []byte)) {
	t.Helper()
	err := fs.WalkDir(os.DirFS("../../.."), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".cache", ".agents", ".codex", ".aws", "bin", "build", "dist", "vendor", "node_modules", "public", "resources":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") {
			b, err := fs.ReadFile(os.DirFS("../../.."), name)
			if err != nil {
				return err
			}
			visit(name, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func parsed(t *testing.T, name string, b []byte) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, b, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestDecisionCoverage checks the table both ways: every test a decision lists exists
// without a build tag, and every test of this package is listed by a decision, so dango
// runs and explains it. A scanner's guard test is listed beside its scanner; the
// package has no test that only go test runs.
func TestDecisionCoverage(t *testing.T) {
	found := map[string]bool{}
	var declared []string
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasSuffix(name, "_test.go") {
			return
		}
		tagged := regexp.MustCompile(`(?m)^//go:build `).Match(b)
		if tagged && path.Dir(name) != hygienePackage {
			return
		}
		f := parsed(t, name, b)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Params.NumFields() != 1 || fn.Type.Results != nil {
				continue
			}
			if !tagged {
				found[path.Dir(name)+"/"+fn.Name.Name] = true
			}
			if path.Dir(name) == hygienePackage && strings.HasPrefix(fn.Name.Name, "Test") {
				declared = append(declared, fn.Name.Name)
			}
		}
	})
	for _, name := range unregisteredTests(hygienePackage, declared, hygiene.Decisions()) {
		t.Errorf("%s/%s: no decision lists the test, so dango neither runs nor explains it; add it to a decision in decisions.go (a guard test beside its scanner)", hygienePackage, name)
	}
	sample := []hygiene.Decision{{ID: "sample", Tests: []hygiene.Test{{Package: "a", Name: "TestListed"}, {Package: "b", Name: "TestElsewhere"}}}}
	if got := unregisteredTests("a", []string{"TestStray", "TestListed", "TestElsewhere"}, sample); !slices.Equal(got, []string{"TestElsewhere", "TestStray"}) {
		t.Errorf("unregistered tests of the sample = %q; want the two that no decision lists for package a", got)
	}
	ids := map[string]bool{}
	for _, d := range hygiene.Decisions() {
		if ids[d.ID] || d.ID == "" || d.Name == "" || d.Reason == "" || d.Fix == "" || len(d.Tests) == 0 {
			t.Errorf("invalid decision: %+v", d)
		}
		ids[d.ID] = true
		for _, test := range d.Tests {
			if !strings.HasPrefix(test.Name, "Test") || !found[test.Package+"/"+test.Name] {
				t.Errorf("%s: missing unconditional enforcing test %s/%s", d.ID, test.Package, test.Name)
			}
		}
	}
	// Dango runs exactly the table's tests; its protocol tests reject missing or skipped tests.
	for file, required := range map[string]string{"Makefile": "\n\t$(GO) tool dango lint\n", ".github/workflows/ci.yml": "run: make lint"} {
		b, err := fs.ReadFile(os.DirFS("../../.."), file)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)
		if file == "Makefile" {
			body = regexp.MustCompile(`(?m)^lint:[^\n]*\n(?:\t[^\n]*\n)*`).FindString(body)
		}
		if !strings.Contains(body, required) {
			t.Errorf("%s must enforce the decision table through make lint", file)
		}
	}
}

type archComponent struct {
	In any `yaml:"in"`
}
type archDeps struct {
	May     []string `yaml:"mayDependOn"`
	Vendors []string `yaml:"canUse"`
}
type archPolicy struct {
	Components map[string]archComponent `yaml:"components"`
	Vendors    map[string]archComponent `yaml:"vendors"`
	Deps       map[string]archDeps      `yaml:"deps"`
}

func inPatterns(c archComponent) []string {
	switch v := c.In.(type) {
	case string:
		return []string{v}
	case []any:
		patterns := make([]string, 0, len(v))
		for _, p := range v {
			patterns = append(patterns, fmt.Sprint(p))
		}
		return patterns
	}
	return nil
}

// componentOf returns the component that owns a package. Like go-arch-lint, the
// most specific pattern wins: a level's directory (internal/base/**) holds the
// named components under it (internal/base/expr/**).
func componentOf(name string, components map[string]archComponent) string {
	owner, longest := "", -1
	for _, id := range slices.Sorted(maps.Keys(components)) {
		for _, p := range inPatterns(components[id]) {
			base := strings.TrimSuffix(p, "/**")
			if (name == base || (strings.HasSuffix(p, "/**") && strings.HasPrefix(name, base+"/"))) && len(base) > longest {
				owner, longest = id, len(base)
			}
		}
	}
	return owner
}

func loadPolicy(t *testing.T) archPolicy {
	t.Helper()
	b, err := os.ReadFile("../../../.go-arch-lint.yml")
	if err != nil {
		t.Fatal(err)
	}
	var policy archPolicy
	if err := yaml.Unmarshal(b, &policy); err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestDependencyDirection(t *testing.T) {
	policy := loadPolicy(t)
	sourceFiles(t, func(name string, b []byte) {
		if strings.HasSuffix(name, "_test.go") || strings.Contains(name, "/testdata/") {
			return
		}
		owner := componentOf(path.Dir(name), policy.Components)
		if owner == "" {
			t.Errorf("%s: package has no architecture component", name)
			return
		}
		for _, imp := range parsed(t, name, b).Imports {
			target, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Split(target, "/")[0], ".") {
				continue
			}
			if local, ok := strings.CutPrefix(target, module); ok {
				dep := componentOf(local, policy.Components)
				if dep != owner && !slices.Contains(policy.Deps[owner].May, dep) {
					t.Errorf("%s: %s must not import %s (%s); move the dependency downward", name, owner, target, dep)
				}
			} else {
				vendor := componentOf(target, policy.Vendors)
				if vendor == "" || !slices.Contains(policy.Deps[owner].Vendors, vendor) {
					t.Errorf("%s: vendor %s is not permitted for %s", name, target, owner)
				}
			}
		}
	})
}

// TestLevelOrder keeps the architecture policy in the order of hygiene.Levels
// (ADR 0064): every component has exactly one level, and none may depend on a
// component of a higher level.
func TestLevelOrder(t *testing.T) {
	for _, v := range levelViolations(loadPolicy(t), hygiene.Levels()) {
		t.Error(v)
	}
}

func levelViolations(policy archPolicy, levels []hygiene.Level) []string {
	var out []string
	rank := map[string]int{}
	for i, l := range levels {
		for _, c := range l.Components {
			if _, dup := rank[c]; dup {
				out = append(out, fmt.Sprintf("component %s is in two levels; give it one", c))
			}
			rank[c] = i
			if _, ok := policy.Components[c]; !ok {
				out = append(out, fmt.Sprintf("level %s names %s, which .go-arch-lint.yml does not define", l.Name, c))
			}
		}
	}
	for _, c := range slices.Sorted(maps.Keys(policy.Components)) {
		if _, ok := rank[c]; !ok {
			out = append(out, fmt.Sprintf("component %s has no level; add it to internal/dev/hygiene/levels.go", c))
		}
	}
	for _, owner := range slices.Sorted(maps.Keys(policy.Deps)) {
		for _, dep := range policy.Deps[owner].May {
			if rank[dep] > rank[owner] {
				out = append(out, fmt.Sprintf("%s (%s) may depend on %s (%s), a higher level; move the shared code down", owner, levels[rank[owner]].Name, dep, levels[rank[dep]].Name))
			}
		}
	}
	return out
}

// TestLevelDirectories keeps the numbered layer directories from returning and
// puts every package of an engine level in the level's directory, so a path
// tells its level (ADR 0064). Application components keep their own places.
func TestLevelDirectories(t *testing.T) {
	policy := loadPolicy(t)
	dirs := map[string]bool{}
	for _, l := range hygiene.Levels() {
		if l.Dir == "" {
			continue
		}
		dirs[l.Dir] = true
		for _, c := range l.Components {
			for _, p := range inPatterns(policy.Components[c]) {
				if p != l.Dir+"/**" && !strings.HasPrefix(p, l.Dir+"/") {
					t.Errorf("component %s (%s) maps to %s, outside its level directory %s", c, l.Name, p, l.Dir)
				}
			}
		}
	}
	for _, l := range hygiene.Levels() {
		if l.Dir != "" {
			continue
		}
		for _, c := range l.Components {
			for _, p := range inPatterns(policy.Components[c]) {
				for d := range dirs {
					if strings.HasPrefix(p, d+"/") {
						t.Errorf("application component %s maps to %s, inside the level directory %s", c, p, d)
					}
				}
			}
		}
	}
	entries, err := os.ReadDir("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && regexp.MustCompile(`^layer\d+$`).MatchString(e.Name()) {
			t.Errorf("internal/%s: name the directory after its level in internal/dev/hygiene/levels.go", e.Name())
		}
	}
}

func TestLevelOrderRejectsUpwardAndMissing(t *testing.T) {
	policy := archPolicy{
		Components: map[string]archComponent{"low": {}, "high": {}, "stray": {}},
		Deps:       map[string]archDeps{"low": {May: []string{"high"}}},
	}
	levels := []hygiene.Level{{Name: "one", Components: []string{"low", "ghost"}}, {Name: "two", Components: []string{"high", "low"}}}
	got := levelViolations(policy, levels)
	for _, want := range []string{"component low is in two levels", "names ghost", "component stray has no level"} {
		if !slices.ContainsFunc(got, func(e string) bool { return strings.Contains(e, want) }) {
			t.Errorf("missing report %q in %q", want, got)
		}
	}
	got = levelViolations(archPolicy{
		Components: map[string]archComponent{"low": {}, "high": {}},
		Deps:       map[string]archDeps{"low": {May: []string{"high"}}},
	}, []hygiene.Level{{Name: "one", Components: []string{"low"}}, {Name: "two", Components: []string{"high"}}})
	if len(got) != 1 || !strings.Contains(got[0], "low (one) may depend on high (two), a higher level") {
		t.Errorf("upward dependency reports = %q", got)
	}
}

func exprText(t *testing.T, n ast.Node) string {
	var b bytes.Buffer
	if err := format.Node(&b, token.NewFileSet(), n); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Existing specialized loaders are frozen by signature. New exceptions are forbidden.
var loaderBaseline = map[string]string{
	"cmd/internal/checkdata/check.go:Load":        "func(fsys fs.FS, out, diagnostics io.Writer) (*gamedata.DB, error)",
	"internal/drive/session/game.go:Load":         "func(fsys fs.FS, dir string, extra ...fs.FS) (*Game, error)",
	"internal/model/gamedata/load.go:Load":        "func(fsys fs.FS) (*DB, error)",
	"internal/model/gamedata/load_dir.go:LoadDir": "func(fsys fs.FS, path string) (*DB, error)",
	"internal/screen/sound/files.go:LoadEffects":  "func(fsys fs.FS) int",
	"internal/screen/sound/bgm.go:LoadBGM":        "func(fsys fs.FS, songs []Song, first string, oneShot []string, vol float64)",
	"internal/screen/play/msgfit.go:LoadFonts":    "func(db *data.DB, fsys fs.FS) error",
}

func TestAPIShape(t *testing.T) {
	sourceFiles(t, func(name string, b []byte) {
		if strings.HasSuffix(name, "_test.go") || strings.Contains(name, "/testdata/") {
			return
		}
		f := parsed(t, name, b)
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if ok && ast.IsExported(spec.Name.Name) && (strings.HasSuffix(spec.Name.Name, "Option") || strings.HasSuffix(spec.Name.Name, "Options")) {
				if _, ok := spec.Type.(*ast.FuncType); ok {
					t.Errorf("%s: %s: use a Config struct, not functional options", name, spec.Name.Name)
				}
			}
			return true
		})
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !ast.IsExported(fn.Name.Name) {
				continue
			}
			sig := exprText(t, fn.Type)
			if !strings.HasPrefix(fn.Name.Name, "Load") || !slices.ContainsFunc(fn.Type.Params.List, func(field *ast.Field) bool { return strings.Contains(exprText(t, field.Type), "fs.FS") }) {
				continue
			}
			if loaderBaseline[name+":"+fn.Name.Name] == sig {
				continue
			}
			ps := fn.Type.Params.List
			valid := fn.Name.Name == "Load" && len(ps) == 2 && exprText(t, ps[0].Type) == "fs.FS" && exprText(t, ps[1].Type) == "string" && fn.Type.Results != nil && fn.Type.Results.NumFields() == 2 && exprText(t, fn.Type.Results.List[len(fn.Type.Results.List)-1].Type) == "error"
			if !valid {
				t.Errorf("%s: %s: use Load(fsys fs.FS, path string) (T, error)", name, fn.Name.Name)
			}
		}
	})
}

func TestCommandExecution(t *testing.T) {
	factories := map[string]bool{"operation": true, "top": true, "group": true}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasPrefix(name, "cmd/gozen/") || strings.HasSuffix(name, "_test.go") {
			return
		}
		for _, decl := range parsed(t, name, b).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv != nil && fn.Type.Results != nil && len(fn.Type.Results.List) == 1 && exprText(t, fn.Type.Results.List[0].Type) == "clihelp.Operation" {
				factories[fn.Name.Name] = true
			}
		}
	})
	sourceFiles(t, func(name string, b []byte) {
		if strings.HasSuffix(name, "_test.go") || (!strings.HasPrefix(name, "cmd/gozen/") && !strings.HasPrefix(name, "cmd/internal/repro/") && !strings.Contains(name, "/command/")) {
			return
		}
		f := parsed(t, name, b)
		for _, violation := range commandExecutionViolations(f, name) {
			t.Errorf("%s: %s", name, violation)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// These are the existing parser/dispatcher infrastructure, not operations.
			if name == "cmd/gozen/main.go" && slices.Contains([]string{"main", "run", "parse", "top", "group"}, fn.Name.Name) {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := exprText(t, call.Fun)
				if strings.HasSuffix(callee, ".parse") {
					// A batch case is an external input boundary using the shared grammar.
					// Its original input may be parsed; reconstructed command argv may not.
					externalCase := name == "cmd/gozen/simbatch.go" && fn.Name.Name == "runCase" && len(call.Args) == 3 && exprText(t, call.Args[1]) == "bc.Args"
					if !externalCase {
						t.Errorf("%s: %s parses constructed command arguments through %s; use typed settings", name, fn.Name.Name, callee)
					}
				}
				if name == "cmd/gozen/help.go" && fn.Name.Name == "describeCommand" && strings.HasSuffix(callee, ".Run") {
					t.Error("machine help must not execute operations")
				}
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && strings.HasPrefix(name, "cmd/gozen/") && factories[selector.Sel.Name] {
					// Machine help consumes registration metadata but must never execute it.
					if name == "cmd/gozen/help.go" && fn.Name.Name == "describeCommand" {
						return true
					}
					t.Errorf("%s: %s re-enters command registration through %s; call typed operations", name, fn.Name.Name, callee)
				}
				if (callee == "run" && strings.HasPrefix(name, "cmd/gozen/")) || callee == "clihelp.Parse" || callee == "exec.Command" || callee == "exec.CommandContext" {
					t.Errorf("%s: %s calls %s; call typed operations instead of CLI dispatch", name, fn.Name.Name, callee)
				}
				return true
			})
		}
	})
}

// YAML words are checked lexically, not as a proof of gameplay generality.
// The frozen baseline below may only shrink; new words cannot be added to it.
const (
	yamlBaselineLimit = 728
	yamlBaseline      = `
acceleration accent accepts act action activation activity activity_attributes
add_state addresses admission advice affect affection affiliation
after_win again all anim answer answer_chance answers any
appear args armies army asides ask assignee at_point
attack_skill attribute attributes auto auto_start auto_tactics auto_tempo auto_wait
avoid away back back_weight backdrop backgrounds backlog bad
badge balloon band bar bare bars battle_ai_history battle_background
battle_foe_names battle_fx battle_message battle_music beats_per_bar before behind below
bestiary blink block_lines blocked blocks blur body bold
bonus bonus_item bonus_skill border boss boss_win_music bottom bounce
bounds bpm breed breeding bridges brightness browse burnup
bust buttons buy c call_view camera camera_off
camera_on can_escape cap_hours caption capture card card_bubbles carousel
carry ceiling ceiling_texture cg_review cgs chance channels char
charge checkpoint child children choice_position chrome clash clear
col colors columns command comment compact complete condition
consume control controls cost counter cover crit
critical critical_rate critical_ratio crowd currency cursor cut cutin
cutin_box cutin_lines cx damage damage_rate darkness data data_version
date date_format date_link deadline deal defeat defend
defense dependency_ratio depth depth_bias desc description destination detail
dim dir done door dot drag drain
draw_roads drop drops due duo_chance duos each east
echo edge effect effects element elements else enabled
end_turn end_turn_button endings endure enemy enter entity episodes
equip_slots equip_stones escape evening event_label event_when every
exclusive exp expr expression_depth expression_length extra face_at face_box
face_expr face_rules face_shade face_shade_top face_shape families family far
fatal field_notices fight fighters file_lines files filled filter_label
finish first first_person fit flag flash flat floor
floor_texture floors flows focus foe_scale foe_scale_when foes fonts
footer for forecast_label fps frame free_move front_weight fronts
full full_color fullscreen fusions fx gallery game_over garrison
gauge generate give goods greet greeting grid group
groups guard_rate guard_skill guest guest_backdrops guests hair hard
haze head header header_face header_tint heading heal held
hidden hide hide_bar_on_choice hide_closed hide_disabled hide_gold highlight hint
hit hits hitstop  host hp hp_triggers hurt
icon image_height images in indexed_names initial instant instead
into intro items join join_exp joined keeper
kinds label_expr label_size labels landmark landmark_ground landmark_scale learn
leave ledger ledgers left legacy_keys legend_rank level_scale light
likes lines live lobby locked logo look looks
loop_start loops lore lose lost manual mark mark_labels
marker markers marks mask match material materials max
max_level max_party max_runes max_soldiers max_value max_zoom maximum maximum_key
meet memo menus merge met meter meters might
milestone min min_block_nodes min_level min_plus min_share minutes missed
missing_voice monster_class monster_families mood mood_names moods morning motion
mp music_pictures muted name_tag nav near new no_map
no_menu no_standing node north note notice novel_window now
npc observation observations observed ok on on_appear on_draw
on_enter on_escape on_leave on_turn on_turn_end on_turn_end_allies on_user
on_win one_shot one_way only opacities opacity open_wall
optional options order_view origin_u origin_v others_title out outfit
over over_size page pages pairs pan paper param
params parent parents passives  patterns peek per_char
per_hour per_soldier per_turn percent period persist phase phone_accent
phone_background phone_muted phone_text picture_fit pictures pin_end_turn pitch place
planned planned_label play policies policy pop portrait posts
power preferred preset price primary primary_card primary_text priority
profile profile_book progress prompt q qr question quick_menu
quiet r rank rare_rank reaction reactions read reason
recoil recover recruit rect refraction region_borders region_numbers region_roads
remove remove_on_damage remove_state repetitions reply reposts
requires reserve reset_actor reset_tree resist respond rest restart
restore_checkpoint restrict restrict_chance resume_to retain retry revive ring
rise roamers rollback roofs rooms rope rounds row
rule rules sacrifice safe_area sample sample_freshness sample_label
save scene_review scenes schedule screen_operations scroll search seed
seed_xor seen selection_panel selector self self_backdrop self_label self_stage
sell send sex sex_ratio sfx sfx_vary shadow shake
shape share shops short side sight sign
simple_tactics size skill skill_slots skip sky slide
soldiers solid song songs sortie south sp sp_apply
sp_decline sp_pitch sp_suggest speaking speech spent spin stage_drop
stage_framing stage_puppets stage_scale stages staging stairs stand_ins star_bonus
start_sp stat state_resist states stats_by stay stayed step
step_ticks still stone_apply stone_choose stone_decline stone_sets stone_suggest stone_wearer
stones stop streak stream streams strict style subject
subtitle subtitle_size subtitle_tracking summon summon_max surfaces symbol tabs
tactic tactic_actions tactic_conditions tactic_presets tactic_targets tactics tag tagline
take talk_on_enter talk_portrait tempo terms terrain text_expr text_key
theme thickness things ticket tile timeout timeout_result timing
tint tip_lines tip_per tips title_art title_background title_figures title_music
title_text today token tone tones top total tracking
transform transform_clear transform_hp travel_speed tree turn turn_speed turn_ticks
unavailable unbounded underside unit units unknown unlock until
until_taken unused_definitions url values variance vars verbs via
video visible voice voice_fallback voices volume wait walk_speed
walked wall wall_regions wallpaper wallpaper_strength warning_key wash web
week_start weekdays weekdays_off weight west wheel_up win win_music
window window_opacity with won written_in x y zoom
`
)

func TestYAMLWords(t *testing.T) {
	keys := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasPrefix(name, "internal/model/gamedata/") || strings.HasSuffix(name, "_test.go") {
			return
		}
		ast.Inspect(parsed(t, name, b), func(n ast.Node) bool {
			f, ok := n.(*ast.Field)
			if !ok || f.Tag == nil {
				return true
			}
			tag, err := strconv.Unquote(f.Tag.Value)
			if err != nil {
				t.Fatal(err)
			}
			key, _, _ := strings.Cut(reflect.StructTag(tag).Get("yaml"), ",")
			if key != "" && key != "-" {
				keys[key] = true
			}
			return true
		})
	})
	schema, docs, specs := "", "", ""
	fixtures := map[string]string{}
	err := fs.WalkDir(os.DirFS("../../.."), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(name, ".") && name != "." {
				return fs.SkipDir
			}
			switch d.Name() {
			case "build", "bin", "dist", "public", "resources", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		wanted := (strings.HasPrefix(name, "data/schema/") && strings.HasSuffix(name, ".json")) || ((strings.HasPrefix(name, "docs/") || strings.HasPrefix(name, "website/content/")) && strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, "docs/decisions/")) || strings.HasSuffix(name, ".yaml")
		if !wanted {
			return nil
		}
		b, err := fs.ReadFile(os.DirFS("../../.."), name)
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(name, "data/schema/"):
			schema += string(b)
			var v any
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			schemaKeys(v, keys)
		case strings.HasSuffix(name, ".md"):
			docs += string(b)
		case strings.HasSuffix(name, ".atago.yaml"):
			specs += string(b)
		case strings.HasPrefix(name, "examples/") || strings.Contains(name, "/testdata/") || strings.Contains(name, "/fixtures/"):
			fixtures[name] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline := strings.Fields(yamlBaseline)
	if !slices.IsSorted(baseline) || len(slices.Compact(slices.Clone(baseline))) != len(baseline) {
		t.Fatal("YAML baseline must contain sorted, unique words")
	}
	if len(baseline) > yamlBaselineLimit {
		t.Fatal("YAML exception allowlist may only shrink")
	}
	docWords := map[string]bool{}
	for _, word := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`).FindAllString(docs, -1) {
		docWords[word] = true
	}
	field := regexp.MustCompile(`(?:^|[\s{,])([A-Za-z_][A-Za-z0-9_]*)\s*:`)
	specWords := map[string]bool{}
	for _, match := range field.FindAllStringSubmatch(specs, -1) {
		specWords[match[1]] = true
	}
	counts := map[string]int{}
	groups := map[string]map[string]bool{}
	for name, body := range fixtures {
		seen := map[string]bool{}
		for _, match := range field.FindAllStringSubmatch(body, -1) {
			seen[match[1]] = true
		}
		for key := range seen {
			group := fixtureRoot(name)
			if groups[key] == nil {
				groups[key] = map[string]bool{}
			}
			groups[key][group] = true
		}
	}
	for key, set := range groups {
		counts[key] = len(set)
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		covered := strings.Contains(schema, `"`+key+`"`) && docWords[key] && specWords[key] && counts[key] >= 2
		if slices.Contains(baseline, key) {
			if covered {
				t.Errorf("%s: now covered; remove its YAML baseline exception", key)
			}
			continue
		}
		if !covered {
			t.Errorf("%s: YAML word needs schema, docs, E2E and two fixtures (fixtures=%d)", key, counts[key])
		}
	}
	for _, key := range baseline {
		if !keys[key] {
			t.Errorf("%s: obsolete YAML baseline exception", key)
		}
	}
}

func schemaKeys(v any, keys map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "properties" {
				if p, ok := v.(map[string]any); ok {
					for key := range p {
						keys[key] = true
					}
				}
			}
			schemaKeys(v, keys)
		}
	case []any:
		for _, v := range x {
			schemaKeys(v, keys)
		}
	}
}

// Several YAML files in one game are one example, not independent evidence.
func fixtureRoot(name string) string {
	if rest, ok := strings.CutPrefix(name, "examples/"); ok {
		sample, _, _ := strings.Cut(rest, "/")
		return "examples/" + sample
	}
	for _, marker := range []string{"/fixtures/", "/testdata/"} {
		if before, after, ok := strings.Cut(name, marker); ok {
			sample, _, _ := strings.Cut(after, "/")
			return before + marker + sample
		}
	}
	return name
}

// No production file under cmd/gozen currently needs os/exec. This frozen
// allowlist has a zero-entry ceiling; it must never grow to admit a handler.
var commandExecAllowlist = []string{}

func commandExecutionViolations(f *ast.File, name string) []string {
	var violations []string
	if !strings.HasPrefix(name, "cmd/gozen/") && !strings.HasPrefix(name, "cmd/internal/repro/") {
		return violations
	}
	parsers := map[string]bool{}
	dotParser := false
	for _, imp := range f.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			violations = append(violations, "invalid import path")
			continue
		}
		if imported == "os/exec" && !slices.Contains(commandExecAllowlist, name) {
			violations = append(violations, "os/exec import is forbidden; write artifacts or call typed operations")
		}
		if imported == module+"internal/base/clihelp" {
			alias := "clihelp"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			parsers[alias] = true
			dotParser = alias == "."
		}
	}
	// Inspect references as well as calls: assigning dispatch to a local alias
	// must not bypass the rule. String literals containing argv are just output.
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && name == "cmd/gozen/main.go" &&
			slices.Contains([]string{"main", "run", "parse", "top", "group"}, fn.Name.Name) {
			continue
		}
		fn, isFunc := decl.(*ast.FuncDecl)
		externalCase := name == "cmd/gozen/simbatch.go" && isFunc && fn.Name.Name == "runCase"
		ast.Inspect(decl, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if n.Name == "runWith" || (n.Name == "run" && (n.Obj == nil || n.Obj.Kind == ast.Fun) && path.Dir(name) == "cmd/gozen") || (dotParser && n.Name == "Parse") {
					violations = append(violations, "CLI dispatch reference "+n.Name+" is forbidden in handlers")
				}
			case *ast.SelectorExpr:
				if n.Sel.Name == "parse" && !externalCase {
					violations = append(violations, "command parser reference is forbidden in handlers")
				}
				if id, ok := n.X.(*ast.Ident); ok && parsers[id.Name] && n.Sel.Name == "Parse" {
					violations = append(violations, "command parser reference is forbidden in handlers")
				}
			}
			return true
		})
	}
	return violations
}

func TestCommandExecutionGuard(t *testing.T) {
	if len(commandExecAllowlist) != 0 {
		t.Fatal("os/exec allowlist may not grow")
	}
	for _, tc := range []struct {
		name, source string
		bad          bool
	}{
		{"exec", `package main; import "os/exec"; func handler() { exec.Command("gozen") }`, true},
		{"alias", `package main; import child "os/exec"; func handler() { child.Command("gozen") }`, true},
		{"blank", `package main; import _ "os/exec"`, true},
		{"dot", `package main; import . "os/exec"; func handler() { Command("gozen") }`, true},
		{"dispatch", "package main; func handler() { runWith(nil) }", true},
		{"method parser", "package main; func handler(c *command) { c.parse(nil,nil,nil) }", true},
		{"method parser alias", "package main; func handler(c *command) { f := c.parse; f(nil,nil,nil) }", true},
		{"dispatch alias", "package main; func handler() { f := runWith; f(nil) }", true},
		{"parser", `package main; import cli "github.com/nao1215/gozen/internal/base/clihelp"; func handler() { cli.Parse(nil,nil) }`, true},
		{"output", `package main; func handler() { argv := []string{"gozen", "run"}; script := "exec gozen run"; _, _ = argv, script }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := commandExecutionViolations(parsed(t, tc.name, []byte(tc.source)), "cmd/gozen/trace_repro.go")
			if (len(got) > 0) != tc.bad {
				t.Fatalf("violations = %v, want rejection %v", got, tc.bad)
			}
		})
	}
}

// pictureDecodeAllowlist names the functions that decode a picture without a
// picturelimit check of their own, by file and function, each with the bound that
// holds there instead. An entry that no longer excuses a decode fails the test.
var pictureDecodeAllowlist = map[string]string{
	"cmd/internal/distribute/webfiles.go:toWebP":              "its only caller, shrinker.webP, checks DecodeConfig against the memory budget and takes that memory first",
	"internal/mosaic/command/main.go:decode":                  "checks image.DecodeConfig against the same limits itself: the architecture policy keeps the mosaic tool from importing internal/base",
	"internal/model/gamepackage/archive.go:validatePNG":       "checks png.DecodeConfig for a square of at most 1024 pixels before the decode",
	"internal/screen/play/phone_packages.go:phonePackageIcon": "decodes Release.IconBytes: at most 64 KiB that validatePNG accepted, a square of at most 1024 pixels",
}

// pictureDecoders are the standard decoders of whole pictures, by import path.
// Any package named webp is treated as one too.
var pictureDecoders = map[string][]string{
	"image":      {"Decode"},
	"image/png":  {"Decode"},
	"image/jpeg": {"Decode"},
	"image/gif":  {"Decode", "DecodeAll"},
}

// unboundedPictureDecodes lists the references to a picture decoder in f that no
// picturelimit.Header or picturelimit.Dimensions precedes in the same top-level
// function. A reference counts, not only a call, so a decoder kept in a variable
// is found. picturelimit.Read bounds the encoded bytes only, so it does not make
// a decode bounded. internal/screen/picdecode is the checked decoder itself.
func unboundedPictureDecodes(f *ast.File, name string) []string {
	if strings.HasPrefix(name, "internal/screen/picdecode/") {
		return nil
	}
	var violations []string
	decoders := map[string][]string{}
	limit := ""
	for _, imp := range f.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			violations = append(violations, "invalid import path")
			continue
		}
		funcs := pictureDecoders[imported]
		if path.Base(imported) == "webp" {
			funcs = []string{"Decode"}
		}
		alias := path.Base(imported)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		switch {
		case imported == module+"internal/base/picturelimit":
			limit = alias
		case funcs == nil || alias == "_":
		case alias == ".":
			violations = append(violations, "a dot import of "+imported+" hides its decoder from this check")
		default:
			decoders[alias] = funcs
		}
	}
	scan := func(owner string, body ast.Node) {
		guard := token.NoPos
		found := map[token.Pos]string{}
		ast.Inspect(body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if limit != "" && pkg.Name == limit && (sel.Sel.Name == "Header" || sel.Sel.Name == "Dimensions") && (guard == token.NoPos || sel.Pos() < guard) {
				guard = sel.Pos()
			}
			if slices.Contains(decoders[pkg.Name], sel.Sel.Name) {
				found[sel.Pos()] = pkg.Name + "." + sel.Sel.Name
			}
			return true
		})
		for _, pos := range slices.Sorted(maps.Keys(found)) {
			if guard == token.NoPos || pos < guard {
				violations = append(violations, owner+" uses "+found[pos]+" before picturelimit.Header or picturelimit.Dimensions")
			}
		}
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Body != nil {
				scan(fn.Name.Name, fn.Body)
			}
			continue
		}
		scan("a package-level declaration", decl)
	}
	return violations
}

// TestBoundedPictureDecode requires every decode of a whole picture outside
// tests to follow a picturelimit check in the same function: a picture's header
// decides how much memory its decode takes, and a small file may claim a huge one.
func TestBoundedPictureDecode(t *testing.T) {
	used := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if strings.HasSuffix(name, "_test.go") || strings.Contains(name, "/testdata/") {
			return
		}
		for _, violation := range unboundedPictureDecodes(parsed(t, name, b), name) {
			owner, _, _ := strings.Cut(violation, " uses ")
			if _, ok := pictureDecodeAllowlist[name+":"+owner]; ok {
				used[name+":"+owner] = true
				continue
			}
			t.Errorf("%s: %s; check the header first (picturelimit.Header) or decode through picdecode", name, violation)
		}
	})
	for entry, reason := range pictureDecodeAllowlist {
		if !used[entry] {
			t.Errorf("%s no longer decodes a picture unchecked; remove it from pictureDecodeAllowlist", entry)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s needs the reason its decode is bounded", entry)
		}
	}
}

func TestBoundedPictureDecodeGuard(t *testing.T) {
	const limit = `import "github.com/nao1215/gozen/internal/base/picturelimit"; `
	for _, tc := range []struct {
		name, source string
		bad          bool
	}{
		{"unchecked png", `package p; import "image/png"; func f() { png.Decode(nil) }`, true},
		{"unchecked image", `package p; import "image"; func f() { image.Decode(nil) }`, true},
		{"unchecked jpeg alias", `package p; import j "image/jpeg"; func f() { j.Decode(nil) }`, true},
		{"unchecked gif frames", `package p; import "image/gif"; func f() { gif.DecodeAll(nil) }`, true},
		{"unchecked webp", `package p; import "golang.org/x/image/webp"; func f() { webp.Decode(nil) }`, true},
		{"decoder kept in a variable", `package p; import "image/png"; func f() { d := png.Decode; d(nil) }`, true},
		{"decoder in a closure", `package p; import "image/png"; func f() { func() { png.Decode(nil) }() }`, true},
		{"decoder of a package variable", `package p; import "image/png"; var d = png.Decode`, true},
		{"dot import", `package p; import . "image/png"; func f() { Decode(nil) }`, true},
		{"encoded size alone", `package p; ` + limit + `import "image/png"; func f() { picturelimit.Read(nil, ""); png.Decode(nil) }`, true},
		{"checked after the decode", `package p; ` + limit + `import "image/png"; func f() { png.Decode(nil); picturelimit.Header(nil) }`, true},
		{"checked in another function", `package p; ` + limit + `import "image/png"; func g() { picturelimit.Header(nil) }; func f() { png.Decode(nil) }`, true},
		{"header checked", `package p; ` + limit + `import "image/png"; func f() { if picturelimit.Header(nil) != nil { return }; png.Decode(nil) }`, false},
		{"dimensions checked", `package p; ` + limit + `import "image"; func f() { picturelimit.Dimensions(1, 1); image.Decode(nil) }`, false},
		{"limit alias", `package p; import lim "github.com/nao1215/gozen/internal/base/picturelimit"; import "image/gif"; func f() { lim.Header(nil); gif.DecodeAll(nil) }`, false},
		{"header only", `package p; import "image/png"; func f() { png.DecodeConfig(nil); png.Encode(nil, nil) }`, false},
		{"registered decoder", `package p; import _ "image/png"; func f() {}`, false},
		{"another Decode", `package p; import "encoding/json"; func f() { json.NewDecoder(nil).Decode(nil) }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := unboundedPictureDecodes(parsed(t, tc.name, []byte(tc.source)), "cmd/gozen/sample.go")
			if (len(got) > 0) != tc.bad {
				t.Fatalf("violations = %v, want rejection %v", got, tc.bad)
			}
		})
	}
	sample := `package p; import "image"; func f() { image.Decode(nil) }`
	if got := unboundedPictureDecodes(parsed(t, "decode.go", []byte(sample)), "internal/screen/picdecode/decode.go"); len(got) != 0 {
		t.Fatalf("picdecode is the checked decoder: %v", got)
	}
}

// usageErrorAllowlist names the option messages that are not argument errors,
// by file and message. Each entry says why the command exits 1 instead of 2.
var usageErrorAllowlist = map[string]string{
	// The option is valid; the model it needs is a missing prerequisite (exit 1, with the requirements).
	"internal/mosaic/command/main.go: --shape mask requires a user-supplied --seg model": "missing prerequisite",
	// The option is valid; the file it names cannot be read (exit 1, an I/O failure).
	"internal/mosaic/command/main.go: --fix: %w":    "unreadable file",
	"internal/mosaic/command/main.go: --manual: %w": "unreadable file",
}

// usageErrorViolations reports errors about an option that a command handler
// builds with errors.New or fmt.Errorf. Such an error exits 1 without a code or
// the --help hint; an argument error is cliio.Invalid (exit 2, ADR 0042).
func usageErrorViolations(f *ast.File, name string) []string {
	if strings.HasSuffix(name, "_test.go") || (!strings.HasPrefix(name, "cmd/gozen/") && !strings.Contains(name, "/command/")) {
		return nil
	}
	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		id := pkg.Name + "." + sel.Sel.Name
		if id != "errors.New" && id != "fmt.Errorf" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		message, err := strconv.Unquote(lit.Value)
		if err != nil || !strings.HasPrefix(message, "--") {
			return true
		}
		if _, allowed := usageErrorAllowlist[name+": "+message]; !allowed {
			violations = append(violations, fmt.Sprintf("%s(%q): an error about an option is cliio.Invalid", id, message))
		}
		return true
	})
	return violations
}

func TestUsageErrors(t *testing.T) {
	used := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		f := parsed(t, name, b)
		for _, v := range usageErrorViolations(f, name) {
			t.Errorf("%s: %s", name, v)
		}
		for key := range usageErrorAllowlist {
			file, message, _ := strings.Cut(key, ": ")
			if file == name && bytes.Contains(b, []byte(strconv.Quote(message))) {
				used[key] = true
			}
		}
	})
	for key, reason := range usageErrorAllowlist {
		if !used[key] || reason == "" {
			t.Errorf("usage error allowlist entry %q is stale or has no reason; remove it", key)
		}
	}
}

func TestUsageErrorsGuard(t *testing.T) {
	for _, tc := range []struct {
		name, file, source string
		bad                bool
	}{
		{"errorf", "cmd/gozen/x.go", `package main; import "fmt"; func h() error { return fmt.Errorf("--boss: unknown troop %q", "x") }`, true},
		{"new", "internal/mosaic/command/x.go", `package command; import "errors"; func h() error { return errors.New("--labels cannot be empty") }`, true},
		{"invalid", "cmd/gozen/x.go", `package main; func h() error { return cliio.Invalid(diag.BadValue, "--boss: unknown troop") }`, false},
		{"not an option", "cmd/gozen/x.go", `package main; import "errors"; func h() error { return errors.New("the save is not valid JSON") }`, false},
		{"library", "internal/model/rpg/x.go", `package rpg; import "errors"; func h() error { return errors.New("--flags is for tests") }`, false},
		{"test file", "cmd/gozen/x_test.go", `package main; import "errors"; func h() error { return errors.New("--x") }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := usageErrorViolations(parsed(t, tc.file, []byte(tc.source)), tc.file)
			if (len(got) > 0) != tc.bad {
				t.Fatalf("violations = %v, want rejection %v", got, tc.bad)
			}
		})
	}
}

// choiceAllowlist names the handlers that reject a choice by hand, by file,
// with the reason the set cannot be declared with clihelp.Choices.
var choiceAllowlist = map[string]string{
	"cmd/gozen/campaign.go": "--tactic is one of the tactics the game declares, known only after the game is read",
}

// choiceViolations reports a fixed choice set that is checked by hand instead
// of being declared: clihelp.Choices validates the value, words the rejection
// and lists the values in the machine-readable help from one list. An empty
// string is never a choice; an option left at its default is not checked.
func choiceViolations(f *ast.File, name string) []string {
	if strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "internal/base/") {
		return nil
	}
	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && id.Name == "diag" && n.Sel.Name == "NotAChoice" && choiceAllowlist[name] == "" {
				violations = append(violations, "diag.NotAChoice by hand: declare the values with clihelp.Choices")
			}
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Choices" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "clihelp" {
				return true
			}
			for _, arg := range n.Args[min(1, len(n.Args)):] {
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``") {
					violations = append(violations, "clihelp.Choices lists an empty value: leave the default out of the choices")
				}
			}
		}
		return true
	})
	return violations
}

func TestDeclaredChoices(t *testing.T) {
	used := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		for _, v := range choiceViolations(parsed(t, name, b), name) {
			t.Errorf("%s: %s", name, v)
		}
		if choiceAllowlist[name] != "" && bytes.Contains(b, []byte("diag.NotAChoice")) {
			used[name] = true
		}
	})
	for name := range choiceAllowlist {
		if !used[name] {
			t.Errorf("choice allowlist entry %q is stale; remove it", name)
		}
	}
}

func TestDeclaredChoicesGuard(t *testing.T) {
	for _, tc := range []struct {
		name, file, source string
		bad                bool
	}{
		{"by hand", "cmd/gozen/x.go", `package main; func h() error { return cliio.Invalid(diag.NotAChoice, "--as is a or b") }`, true},
		{"empty choice", "cmd/gozen/x.go", `package main; func h() { clihelp.Choices(f.Lookup("only"), "", "structure") }`, true},
		{"declared", "cmd/gozen/x.go", `package main; func h() { clihelp.Choices(f.Lookup("only"), "structure", "layout") }`, false},
		{"allowed", "cmd/gozen/campaign.go", `package main; func h() error { return cliio.Invalid(diag.NotAChoice, "--tactic") }`, false},
		{"parser", "internal/base/clihelp/clihelp.go", `package clihelp; func h() error { return diag.NotAChoice.Errorf("x") }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := choiceViolations(parsed(t, tc.file, []byte(tc.source)), tc.file)
			if (len(got) > 0) != tc.bad {
				t.Fatalf("violations = %v, want rejection %v", got, tc.bad)
			}
		})
	}
}

// TestDeadcodeGate rejects tool/filter failures while allowing only explicitly
// exempted findings. Diagnostics must remain on stderr, outside the filter.
func TestDeadcodeGate(t *testing.T) {
	for _, tool := range []string{"make", "sh", "grep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("deadcode gate requires make, a POSIX shell and grep: %s unavailable: %v", tool, err)
		}
	}
	makefile, err := filepath.Abs("../../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, output, allow string
		status              int
		missing, fail       bool
	}{
		{name: "tool failure", status: 7, fail: true},
		{name: "tool failure with exempt output", output: "exempt", allow: "^exempt$", status: 7, fail: true},
		{name: "finding", output: "unreachable", allow: "^exempt$", fail: true},
		{name: "blank allowlist", output: "unreachable", fail: true},
		{name: "blank separator preserves finding", output: "unreachable", allow: "^exempt$\n\n", fail: true},
		{name: "blank separator preserves exemption", output: "exempt", allow: "\n^exempt$\n\n"},
		{name: "whitespace separator", output: "unreachable", allow: " \t\n^exempt$", fail: true},
		{name: "comment is not a pattern", output: "exempt", allow: "# [ explanatory text\n^exempt$"},
		{name: "exempt finding", output: "exempt", allow: "^exempt$"},
		{name: "no findings", allow: "^exempt$"},
		{name: "invalid filter", output: "exempt", allow: "[", fail: true},
		{name: "missing filter", output: "exempt", missing: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := fmt.Sprintf("printf 'diagnostic on stderr\\n' >&2\nprintf '%%s\\n' '%s'\nexit %d\n", tc.output, tc.status)
			if err := os.WriteFile(filepath.Join(dir, "tool.sh"), []byte(script), 0o600); err != nil {
				t.Fatal(err)
			}
			if !tc.missing {
				if err := os.WriteFile(filepath.Join(dir, ".deadcode-allow"), []byte(tc.allow+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-s", "-f", makefile, "deadcode", "DEADCODE=sh ./tool.sh", "SHELL=sh") //nolint:gosec // Fixed repository recipe and isolated fixture; no external command input.
			cmd.Dir = dir
			var out, diagnostics bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &diagnostics
			err := cmd.Run()
			if (err != nil) != tc.fail {
				t.Fatalf("make error %v, want failure %v; stdout=%q stderr=%q", err, tc.fail, out.String(), diagnostics.String())
			}
			if !strings.Contains(diagnostics.String(), "diagnostic on stderr") || strings.Contains(out.String(), "diagnostic on stderr") {
				t.Fatalf("diagnostic channel: stdout=%q stderr=%q", out.String(), diagnostics.String())
			}
			if tc.name == "finding" && !strings.Contains(out.String(), "unreachable") {
				t.Fatalf("finding lost: %q", out.String())
			}
		})
	}
}

// TestCIChangeScope exercises the real merge-diff classifier, including missing
// history. Required checks may omit expensive steps only on an explicit false.
// A change inside any engine level directory must start the performance job, so
// moving or adding a level directory cannot silently drop it from the filter.
func TestCIChangeScope(t *testing.T) {
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("CI scope requires %s: %v", tool, err)
		}
	}
	script, err := filepath.Abs("../../../scripts/ci_changes.sh")
	if err != nil {
		t.Fatal(err)
	}
	type scopeCase struct {
		name, event      string
		files            []string
		history          bool
		code, docs, perf string
	}
	cases := []scopeCase{
		{"release notes PR", "pull_request", []string{"CHANGELOG.md"}, true, "false", "false", "false"},
		{"mixed docs PR", "pull_request", []string{"CHANGELOG.md", "README.md"}, true, "false", "true", "false"},
		{"mixed code PR", "pull_request", []string{"CHANGELOG.md", "main.go"}, true, "true", "true", "false"},
		{"executable examples", "pull_request", []string{"docs/examples/game.yaml"}, true, "true", "true", "false"},
		{"unknown path", "pull_request", []string{"unknown"}, true, "true", "true", "false"},
		{"main push", "push", []string{"CHANGELOG.md"}, true, "true", "true", "true"},
		{"manual run", "workflow_dispatch", []string{"CHANGELOG.md"}, true, "true", "true", "true"},
		{"empty diff", "pull_request", nil, true, "true", "true", "true"},
		{"missing history", "pull_request", []string{"CHANGELOG.md"}, false, "true", "true", "true"},
	}
	for _, l := range hygiene.Levels() {
		if l.Dir != "" {
			cases = append(cases, scopeCase{"level " + l.Name, "pull_request", []string{l.Dir + "/pkg/file.go"}, true, "true", "true", "true"})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // Fixed test commands in an isolated repository.
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("git %v: %v; stdout=%s stderr=%s", args, err, &stdout, &stderr)
				}
			}
			git("init", "-q")
			if tc.history {
				git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "base")
			}
			for _, name := range tc.files {
				file := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("fixture\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			git("add", ".")
			git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "change")
			output := filepath.Join(t.TempDir(), "outputs")
			cmd := exec.CommandContext(t.Context(), "bash", script, tc.event) //nolint:gosec // Repository script and fixed event fixtures.
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+output)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("scope: %v; stdout=%s stderr=%s", err, &stdout, &stderr)
			}
			got, err := os.ReadFile(output) //nolint:gosec // Output inside the test temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"code=" + tc.code + "\n", "docs_runtime=" + tc.docs + "\n", "perf=" + tc.perf + "\n"} {
				if !strings.Contains("\n"+string(got), "\n"+want) {
					t.Fatalf("outputs %q missing %q", got, want)
				}
			}
		})
	}
}

// graphicsOnly reports whether a file's build constraint needs the graphics tag.
func graphicsOnly(b []byte) bool {
	line := regexp.MustCompile(`(?m)^//go:build .*$`).Find(b)
	if line == nil {
		return false
	}
	expr, err := constraint.Parse(string(line))
	if err != nil {
		return false
	}
	return !expr.Eval(func(tag string) bool { return tag != "graphics" }) && expr.Eval(func(string) bool { return true })
}

// TestGraphicsTestSelection keeps every test of a graphics-tagged file inside what
// make graphics and the graphics jobs select. They select by the TestGraphics
// prefix, so a test with another name, or a package the CI workflow runs with a
// narrower pattern only, would compile and never run.
func TestGraphicsTestSelection(t *testing.T) {
	packages := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasSuffix(name, "_test.go") || !graphicsOnly(b) {
			return
		}
		// The samples are modules of their own, run whole by make <sample>-graphics.
		if !strings.HasPrefix(name, "examples/") {
			packages[path.Dir(name)] = true
		}
		for _, decl := range parsed(t, name, b).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "TestGraphics") {
				t.Errorf("%s: %s is in a graphics-tagged file; name it TestGraphics* so make graphics and CI select it", name, fn.Name.Name)
			}
		}
	})
	if len(packages) == 0 {
		t.Fatal("no graphics-tagged test file was found")
	}
	root := os.DirFS("../../..")
	makefile, err := fs.ReadFile(root, "Makefile")
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := fs.ReadFile(root, ".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^GRAPHICS_FILTER := -run TestGraphics$`).Match(makefile) {
		t.Error("Makefile: make graphics must select by the TestGraphics prefix")
	}
	for _, pkg := range slices.Sorted(maps.Keys(packages)) {
		selected := regexp.MustCompile(`(?m)^.*-tags=graphics.* \./` + regexp.QuoteMeta(pkg) + ` -run '?\^?TestGraphics.*$`)
		if !selected.Match(workflow) {
			t.Errorf(".github/workflows/ci.yml: no go test line selects the TestGraphics tests of ./%s", pkg)
		}
	}
}

// TestCIGoTestRetry runs the unit job's script against a stand-in go command.
// Only the known Windows runtime crash of a listed package is retried; ordinary
// failures, other packages and other platforms fail on the first run.
func TestCIGoTestRetry(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("the unit job script requires bash: %v", err)
	}
	script, err := filepath.Abs("../../../scripts/ci_go_test.sh")
	if err != nil {
		t.Fatal(err)
	}
	const (
		mosaic    = "github.com/nao1215/gozen/internal/mosaic"
		promptgen = "github.com/nao1215/gozen/internal/promptgen"
	)
	ok := "ok  \tgithub.com/nao1215/gozen/internal/model/rpg\t0.1s\n"
	gcCrash := func(pkg string) string {
		return "runtime: marked free object in span\nfatal error: found pointer to free object\n\ngoroutine 3 [running]:\nruntime.throw()\n\tC:/go/src/runtime/panic.go:1243\nFAIL\t" + pkg + "\t0.1s\n"
	}
	exception := "Exception 0xc0000005 0x8 0x7ff704622728 0x7ff704622728\nPC=0x7ff704622728\nfatal error: runtime.goparkunlock(...)\nFAIL\t" + promptgen + "\t0.1s\n"
	pass := func(pkg string) string { return "ok  \t" + pkg + "\t0.1s\n" }
	for _, tc := range []struct {
		name, goos string
		runs       []string // output of each go test run; a run fails when it has FAIL
		status     int
		calls      []string
	}{
		{"success", "windows", []string{ok}, 0, []string{"test ./..."}},
		{"GC crash retried", "windows", []string{ok + gcCrash(mosaic) + "FAIL\n", pass(mosaic)}, 0, []string{"test ./...", "test -count=1 " + mosaic}},
		{"index out of range in the collector", "windows", []string{strings.Replace(gcCrash(mosaic), "found pointer to free object", "index out of range", 1), pass(mosaic)}, 0, []string{"test ./...", "test -count=1 " + mosaic}},
		{"access violation retried", "windows", []string{exception, pass(promptgen)}, 0, []string{"test ./...", "test -count=1 " + promptgen}},
		{"both packages retried together", "windows", []string{gcCrash(mosaic) + exception, pass(mosaic) + pass(promptgen)}, 0, []string{"test ./...", "test -count=1 " + mosaic + " " + promptgen}},
		{"bounded retries", "windows", []string{gcCrash(mosaic), gcCrash(mosaic), gcCrash(mosaic), gcCrash(mosaic)}, 1, []string{"test ./...", "test -count=1 " + mosaic, "test -count=1 " + mosaic, "test -count=1 " + mosaic}},
		{"ordinary failure after a retry", "windows", []string{gcCrash(mosaic), "--- FAIL: TestX (0.00s)\nFAIL\n" + "FAIL\t" + mosaic + "\t0.1s\n"}, 1, []string{"test ./...", "test -count=1 " + mosaic}},
		{"other platform", "linux", []string{gcCrash(mosaic)}, 1, []string{"test ./..."}},
		{"unlisted package", "windows", []string{gcCrash("github.com/nao1215/gozen/internal/screen/play")}, 1, []string{"test ./..."}},
		{"crash beside an ordinary failure", "windows", []string{gcCrash(mosaic) + "--- FAIL: TestY (0.00s)\nFAIL\nFAIL\tgithub.com/nao1215/gozen/internal/model/rpg\t0.1s\n"}, 1, []string{"test ./..."}},
		{"failed test in the crashed package", "windows", []string{"--- FAIL: TestZ (0.00s)\n" + gcCrash(mosaic)}, 1, []string{"test ./..."}},
		{"test panic", "windows", []string{"panic: boom\n" + gcCrash(mosaic)}, 1, []string{"test ./..."}},
		{"deadlock", "windows", []string{"fatal error: all goroutines are asleep - deadlock!\nFAIL\t" + mosaic + "\t0.1s\n"}, 1, []string{"test ./..."}},
		{"build failure", "windows", []string{"FAIL\t" + mosaic + " [build failed]\nFAIL\n"}, 1, []string{"test ./..."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for i, out := range tc.runs {
				if err := os.WriteFile(filepath.Join(dir, "out"+strconv.Itoa(i+1)), []byte(out), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			stand := filepath.Join(dir, "go")
			body := `#!/usr/bin/env bash
if [ "$1" = env ]; then echo "$FAKE_GOOS"; exit 0; fi
n=$(( $(cat "$FAKE_DIR/count" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$FAKE_DIR/count"
echo "$*" >> "$FAKE_DIR/calls"
[ -f "$FAKE_DIR/out$n" ] || exit 99
cat "$FAKE_DIR/out$n"
grep -q '^FAIL' "$FAKE_DIR/out$n" && exit 1
exit 0
`
			if err := os.WriteFile(stand, []byte(body), 0o700); err != nil { //nolint:gosec // An executable stand-in inside the test temporary directory.
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "bash", script) //nolint:gosec // Repository script with a stand-in go command.
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GO="+filepath.ToSlash(stand), "FAKE_GOOS="+tc.goos, "FAKE_DIR="+filepath.ToSlash(dir))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			status := 0
			if exit, isExit := err.(*exec.ExitError); isExit { //nolint:errorlint // Run returns the exit error itself.
				status = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			calls, err := os.ReadFile(filepath.Join(dir, "calls")) //nolint:gosec // Output inside the test temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Split(strings.TrimSuffix(string(calls), "\n"), "\n"); status != tc.status || !slices.Equal(got, tc.calls) {
				t.Fatalf("status %d calls %q, want %d %q; stdout=%s stderr=%s", status, got, tc.status, tc.calls, &stdout, &stderr)
			}
		})
	}
}

// TestSnapshotEngineTagSelection runs the workflow's real selection command with
// engine and independent catalog tags both at HEAD and on nearer ancestors.
func TestSnapshotEngineTagSelection(t *testing.T) {
	for _, tool := range []string{"sh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("snapshot workflow requires %s: %v", tool, err)
		}
	}
	b, err := os.ReadFile("../../../.github/workflows/release-smoke.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct{ Steps []struct{ Name, Run string } }
	}
	if err := yaml.Unmarshal(b, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Name == "Select engine snapshot tag" {
				script = step.Run
			}
		}
	}
	if script == "" {
		t.Fatal("snapshot engine selection missing")
	}
	dir := t.TempDir()
	run := func(binary string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), binary, args...) //nolint:gosec // Fixed workflow command and isolated test repository.
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com", "GITHUB_ENV="+filepath.Join(dir, "selected.env"))
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s %v: %v stdout=%s stderr=%s", binary, args, err, &stdout, &stderr)
		}
	}
	commit := func() { run("git", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "fixture") }
	check := func(want string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "selected.env"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		run("sh", "-e", "-c", script)
		got, err := os.ReadFile(filepath.Join(dir, "selected.env")) //nolint:gosec // Test-owned workflow output.
		if err != nil || strings.TrimSpace(string(got)) != "GORELEASER_CURRENT_TAG="+want {
			t.Fatalf("selected %q, want %s: %v", got, want, err)
		}
	}
	run("git", "init", "-q")
	commit()
	run("git", "tag", "v0.22.0")
	commit()
	run("git", "tag", "game-catalog")
	run("git", "tag", "phone-counter-v1.0.0")
	check("v0.22.0")
	commit()
	check("v0.22.0")
	run("git", "tag", "v0.23.0")
	check("v0.23.0")
}

// hygienePackage holds the maintainer tests of the decision table.
const hygienePackage = "internal/dev/hygiene"

// unregisteredTests returns the declared tests of a package that no decision lists.
func unregisteredTests(pkg string, declared []string, decisions []hygiene.Decision) []string {
	listed := map[string]bool{}
	for _, d := range decisions {
		for _, test := range d.Tests {
			listed[test.Package+"/"+test.Name] = true
		}
	}
	var out []string
	for _, name := range declared {
		if !listed[pkg+"/"+name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// packageUse is one reference to a member of a standard package, with the top-level
// declaration that holds it (a method as Type.method, a package variable as "package").
type packageUse struct{ fn, member string }

// packageUses lists a file's references to the named members of a standard package
// under any import alias. A reference counts, not only a call, so assigning the
// function to a variable does not hide it; a dot import hides every reference and is
// reported as the member ".".
func packageUses(f *ast.File, pkg string, members ...string) []packageUse {
	var uses []packageUse
	aliases := map[string]bool{}
	for _, imp := range f.Imports {
		if imported, err := strconv.Unquote(imp.Path.Value); err != nil || imported != pkg {
			continue
		}
		switch {
		case imp.Name == nil:
			aliases[path.Base(pkg)] = true
		case imp.Name.Name == ".":
			uses = append(uses, packageUse{"import", "."})
		default:
			aliases[imp.Name.Name] = true
		}
	}
	for _, decl := range f.Decls {
		holder := "package"
		if fn, ok := decl.(*ast.FuncDecl); ok {
			holder = fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				recv := fn.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if index, ok := recv.(*ast.IndexExpr); ok {
					recv = index.X
				}
				if id, ok := recv.(*ast.Ident); ok {
					holder = id.Name + "." + holder
				}
			}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// A package name resolves to no object of the file; a local variable of the same name does.
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil && aliases[id.Name] && slices.Contains(members, sel.Sel.Name) {
				uses = append(uses, packageUse{holder, sel.Sel.Name})
			}
			return true
		})
	}
	return uses
}

// allowedUses reports the uses of one file that the allowlist does not name, and marks
// the entries it met. An entry is "file:holder" with its reason.
func allowedUses(name, pkg string, uses []packageUse, allow map[string]string, met map[string]bool) []string {
	var out []string
	for _, u := range uses {
		key := name + ":" + u.fn
		if allow[key] != "" {
			met[key] = true
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s uses %s.%s", name, u.fn, path.Base(pkg), u.member))
	}
	return out
}

// directOutputAllowlist names the functions of cmd/gozen that may create a file or a
// directory with package os themselves, each with its reason. Every other output goes
// through cmd/internal/cli (Publish, WriteNew, Destination), which refuses an existing
// destination, stages the result and removes it on failure. The list may only shrink:
// an entry whose function no longer writes directly must be deleted.
var directOutputAllowlist = map[string]string{
	"cmd/gozen/capture.go:writeShotIndex":            "writes shots.json and index.html into the stage directory cli.Publish gives the capture",
	"cmd/gozen/capture_diff.go:compareShot":          "writes the previous and highlight pictures into that same stage directory",
	"cmd/gozen/check_baseline.go:writeCheckBaseline": "creates --write-baseline with O_EXCL and reports an existing file as an existing output; to remove: cli.WriteNew does the same",
	"cmd/gozen/design.go:command.design":             "creates --out itself after cli.Destination, removes it on failure, and writes debug.log, summary.json and report.md into it",
	"cmd/gozen/design.go:writeBand":                  "writes band.png into the --out directory command.design created",
	"cmd/gozen/design.go:writeTimeline":              "writes a persona's timeline into the --out directory command.design created",
	"cmd/gozen/faces.go:command.faces":               "rewrites assets/faces.json inside the game: the file the command maintains, not a new output",
	"cmd/gozen/main.go:startProfile":                 "writes the CPU profile to the path the developer set in GOZEN_CPUPROFILE: a diagnostic, not a command output",
	"cmd/gozen/memos.go:command.renderMemos":         "writes index.html into the stage directory cli.Publish gives the memo page",
	"cmd/gozen/memos.go:thumbnail":                   "writes a thumbnail into that same stage directory",
	"cmd/gozen/simscript.go:command.executeScript":   "creates the --trace file and its parent; to remove: it overwrites an existing trace",
	"cmd/gozen/trial.go:writeList":                   "creates the parent of --list before cli.WriteNew; to remove: WriteNew creates the parent itself",
}

// TestCommandOutputs keeps command outputs on the publication helper.
func TestCommandOutputs(t *testing.T) {
	met := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasPrefix(name, "cmd/gozen/") || strings.HasSuffix(name, "_test.go") {
			return
		}
		uses := packageUses(parsed(t, name, b), "os", "Mkdir", "MkdirAll", "Create", "WriteFile", "OpenFile")
		for _, v := range allowedUses(name, "os", uses, directOutputAllowlist, met) {
			t.Errorf("%s; publish the output through cmd/internal/cli (Publish, WriteNew) so it is new, staged and removed on failure", v)
		}
	})
	for _, key := range slices.Sorted(maps.Keys(directOutputAllowlist)) {
		if !met[key] {
			t.Errorf("%s: no direct output is left there; remove it from directOutputAllowlist", key)
		}
	}
}

// wallClockAllowlist names the functions under cmd/ that may read the wall clock, each
// with its reason. A command's output otherwise depends on its inputs alone, so two
// runs can be compared byte for byte. The list may only shrink.
var wallClockAllowlist = map[string]string{
	"cmd/gozen/capture.go:writeShotIndex": "prints the minute the capture was made into index.html for the author who opens the page; shots.json and the pictures carry no time",
}

// TestCommandClock keeps the wall clock out of command output.
func TestCommandClock(t *testing.T) {
	met := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasPrefix(name, "cmd/") || strings.HasSuffix(name, "_test.go") {
			return
		}
		uses := packageUses(parsed(t, name, b), "time", "Now", "Since", "Until")
		for _, v := range allowedUses(name, "time", uses, wallClockAllowlist, met) {
			t.Errorf("%s; take the time from the command's input so the same input gives the same output", v)
		}
	})
	for _, key := range slices.Sorted(maps.Keys(wallClockAllowlist)) {
		if !met[key] {
			t.Errorf("%s: the wall clock is no longer read there; remove it from wallClockAllowlist", key)
		}
	}
}

// TestCommandUsesGuard proves the scanner behind TestCommandOutputs and
// TestCommandClock: it sees a call, an alias, a stored reference and a dot import,
// names the declaration that holds each, and passes the helper, reads and a local
// variable that only shares the package's name.
func TestCommandUsesGuard(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []packageUse
	}{
		{"call", `package main; import "os"; func write() { _ = os.WriteFile("out", nil, 0o600) }`, []packageUse{{"write", "WriteFile"}}},
		{"alias", `package main; import std "os"; func write() { _ = std.MkdirAll("out", 0o700) }`, []packageUse{{"write", "MkdirAll"}}},
		{"reference", `package main; import "os"; func write() { create := os.Create; _, _ = create("out") }`, []packageUse{{"write", "Create"}}},
		{"closure in a method", `package main; import "os"; type c struct{}; func (*c) run() { func() { _ = os.Mkdir("out", 0o700) }() }`, []packageUse{{"c.run", "Mkdir"}}},
		{"package variable", `package main; import "os"; var open = os.OpenFile`, []packageUse{{"package", "OpenFile"}}},
		{"dot import", `package main; import . "os"; func write() { _ = WriteFile("out", nil, 0o600) }`, []packageUse{{"import", "."}}},
		{"helper", `package main; import "example.com/cli"; func write() { _ = cli.WriteNew("out", nil) }`, nil},
		{"read", `package main; import "os"; func read() { _, _ = os.ReadFile("in"); _, _ = os.MkdirTemp("", "stage") }`, nil},
		{"local variable", `package main; type files struct{}; func (files) Create(string) {}; func write() { os := files{}; os.Create("out") }`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := packageUses(parsed(t, tc.name, []byte(tc.source)), "os", "Mkdir", "MkdirAll", "Create", "WriteFile", "OpenFile")
			if !slices.Equal(got, tc.want) {
				t.Fatalf("uses = %v, want %v", got, tc.want)
			}
		})
	}
	clock := packageUses(parsed(t, "clock", []byte(`package main; import "time"; func stamp() string { return time.Now().Format(time.RFC3339) + time.Since(time.Time{}).String() }`)), "time", "Now", "Since", "Until")
	if !slices.Equal(clock, []packageUse{{"stamp", "Now"}, {"stamp", "Since"}}) {
		t.Fatalf("clock uses = %v", clock)
	}
	met := map[string]bool{}
	allow := map[string]string{"cmd/gozen/a.go:stamp": "sample reason", "cmd/gozen/a.go:gone": "sample reason"}
	if got := allowedUses("cmd/gozen/a.go", "time", clock, allow, met); len(got) != 0 || !met["cmd/gozen/a.go:stamp"] || met["cmd/gozen/a.go:gone"] {
		t.Fatalf("allowed file: violations %v, met %v", got, met)
	}
	if got := allowedUses("cmd/gozen/b.go", "time", clock, allow, met); !slices.Equal(got, []string{"cmd/gozen/b.go: stamp uses time.Now", "cmd/gozen/b.go: stamp uses time.Since"}) {
		t.Fatalf("other file: violations %v", got)
	}
}

// pathFilter is one path pattern of the CI configuration: the file that holds it, the
// pattern as written, and what one "*" matches there. A shell case pattern lets it
// cross directories; a workflow filter needs "**" for that.
type pathFilter struct{ source, pattern, star string }

// shellCasePatterns returns the patterns of the case statements over subject in a
// shell script, without the empty word and the catch-all "*", which name no path.
func shellCasePatterns(script, subject string) []string {
	var patterns []string
	inCase, inBody := false, false
	for line := range strings.SplitSeq(script, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "case "):
			inCase, inBody = line == "case "+subject+" in", false
		case line == "esac":
			inCase = false
		case inCase && inBody:
			inBody = !strings.Contains(line, ";;")
		case inCase:
			words, body, ok := strings.Cut(line, ")")
			if !ok {
				continue
			}
			for word := range strings.SplitSeq(words, "|") {
				if word = strings.TrimSpace(word); word != "" && word != `""` && word != "*" {
					patterns = append(patterns, word)
				}
			}
			inBody = !strings.Contains(body, ";;")
		}
	}
	return patterns
}

// workflowPaths returns the paths and paths-ignore entries of a workflow's events,
// without the "!" of a negated entry: that pattern too must name something.
func workflowPaths(workflow []byte) ([]string, error) {
	var w struct {
		On any `yaml:"on"`
	}
	if err := yaml.Unmarshal(workflow, &w); err != nil {
		return nil, err
	}
	events, ok := w.On.(map[string]any)
	if !ok { // a bare event name or a list of them carries no filter
		return nil, nil
	}
	var patterns []string
	for _, event := range slices.Sorted(maps.Keys(events)) {
		settings, ok := events[event].(map[string]any)
		if !ok { // a schedule's list, or an event without settings
			continue
		}
		for _, key := range []string{"paths", "paths-ignore"} {
			entries, ok := settings[key].([]any)
			if !ok {
				continue
			}
			for _, entry := range entries {
				patterns = append(patterns, strings.TrimPrefix(fmt.Sprint(entry), "!"))
			}
		}
	}
	return patterns, nil
}

// stalePathFilters reports the filters that match none of the files, and the entries of
// absent (source + ": " + pattern, with the reason it may match nothing) that match one
// after all or that no filter has any more.
func stalePathFilters(filters []pathFilter, files []string, absent map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range filters {
		key := f.source + ": " + f.pattern
		seen[key] = true
		if strings.ContainsAny(f.pattern, `?[]{}+\`) {
			out = append(out, key+": glob operator the check does not read; extend stalePathFilters")
			continue
		}
		parts := strings.Split(f.pattern, "**")
		for i, part := range parts {
			parts[i] = strings.ReplaceAll(regexp.QuoteMeta(part), `\*`, f.star)
		}
		re := regexp.MustCompile("^" + strings.Join(parts, ".*") + "$")
		matched := slices.ContainsFunc(files, re.MatchString)
		switch _, allowed := absent[key]; {
		case !matched && !allowed:
			out = append(out, key+": matches no file, so the filter no longer selects what it was written for; name the path that replaced it or delete the pattern")
		case matched && allowed:
			out = append(out, key+": matches a file now; remove it from absentPathFilters")
		}
	}
	for _, key := range slices.Sorted(maps.Keys(absent)) {
		if !seen[key] {
			out = append(out, key+": no such filter is left; remove it from absentPathFilters")
		}
	}
	return out
}

// absentPathFilters are the CI path patterns that may match no file, with the reason.
var absentPathFilters = map[string]string{
	"scripts/ci_changes.sh: LICENSE.*": "a licence text added under its other usual name (LICENSE.txt) is documentation too",
	"scripts/ci_changes.sh: LICENSE-*": "a second licence added under its usual name (LICENSE-APACHE) is documentation too",
}

// TestCIPathFilters requires every path pattern of scripts/ci_changes.sh and every
// paths or paths-ignore entry of the workflows to match a file of the checkout. A
// pattern left behind by a moved directory matches nothing and silently turns its job
// off: the performance job was skipped that way after the level directories were renamed.
// TestCIChangeScope proves the classifier on sample diffs; this checks its patterns
// against the repository. An untracked local file can satisfy a pattern, so CI's clean
// checkout is the authority.
func TestCIPathFilters(t *testing.T) {
	var files []string
	repositoryFiles(t, func(name string) { files = append(files, name) })
	const script = "scripts/ci_changes.sh"
	b, err := os.ReadFile("../../../" + script)
	if err != nil {
		t.Fatal(err)
	}
	var filters []pathFilter
	for _, p := range shellCasePatterns(string(b), `"${f}"`) {
		filters = append(filters, pathFilter{script, p, ".*"})
	}
	if len(filters) == 0 {
		t.Fatalf(`%s: no case statement over "${f}" found; teach TestCIPathFilters where the path patterns are now`, script)
	}
	const dir = ".github/workflows"
	entries, err := os.ReadDir("../../../" + dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile("../../../" + dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		patterns, err := workflowPaths(b)
		if err != nil {
			t.Fatalf("%s/%s: %v", dir, e.Name(), err)
		}
		for _, p := range patterns {
			filters = append(filters, pathFilter{dir + "/" + e.Name(), p, "[^/]*"})
		}
	}
	for _, v := range stalePathFilters(filters, files, absentPathFilters) {
		t.Error(v)
	}
}

// TestCIPathFiltersGuard proves the scanner on a sample script and workflow: a stale
// directory, a stale exact file and a stale workflow entry are reported; live patterns,
// the catch-all, another case subject and a declared absence are not.
func TestCIPathFiltersGuard(t *testing.T) {
	script := `
case "${event}" in
  pull_request) ;;
esac
while IFS= read -r f; do
  case "${f}" in
    "") ;;
    # a comment (with a parenthesis)
    *.md | docs/* | internal/layer1/*) ;;
    internal/base/* | gone.ttf | LICENSE.*) perf=true ;;
    cmd/*)
      perf=true
      ;;
    *) emit true ;;
  esac
done
`
	workflow := `
on:
  push:
    branches: [main]
    paths:
      - docs/registry/**
      - cmd/*/main.go
      - cmd/*.go
  pull_request:
    paths-ignore:
      - '!website/**'
      - '*.md'
`
	shell := shellCasePatterns(script, `"${f}"`)
	if want := []string{"*.md", "docs/*", "internal/layer1/*", "internal/base/*", "gone.ttf", "LICENSE.*", "cmd/*"}; !slices.Equal(shell, want) {
		t.Fatalf("shell patterns = %q, want %q", shell, want)
	}
	paths, err := workflowPaths([]byte(workflow))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"website/**", "*.md", "docs/registry/**", "cmd/*/main.go", "cmd/*.go"}; !slices.Equal(paths, want) {
		t.Fatalf("workflow paths = %q, want %q", paths, want)
	}
	var filters []pathFilter
	for _, p := range shell {
		filters = append(filters, pathFilter{"ci.sh", p, ".*"})
	}
	for _, p := range paths {
		filters = append(filters, pathFilter{"ci.yml", p, "[^/]*"})
	}
	filters = append(filters, pathFilter{"ci.yml", "docs/[a-z]*", "[^/]*"})
	files := []string{"README.md", "docs/guide/start.md", "docs/registry/catalog.json", "internal/base/expr/expr.go", "cmd/gozen/main.go"}
	absent := map[string]string{"ci.sh: LICENSE.*": "sample reason", "ci.sh: docs/*": "sample reason", "ci.sh: removed/*": "sample reason"}
	got := stalePathFilters(filters, files, absent)
	want := []string{
		"ci.sh: docs/*: matches a file now",
		"ci.sh: internal/layer1/*: matches no file",
		"ci.sh: gone.ttf: matches no file",
		"ci.yml: website/**: matches no file",
		"ci.yml: cmd/*.go: matches no file",
		"ci.yml: docs/[a-z]*: glob operator",
		"ci.sh: removed/*: no such filter is left",
	}
	if len(got) != len(want) {
		t.Fatalf("reports = %q, want %q", got, want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("report %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
}
