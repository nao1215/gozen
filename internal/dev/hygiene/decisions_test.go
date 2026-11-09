package hygiene_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
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

func TestDecisionCoverage(t *testing.T) {
	found := map[string]bool{}
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasSuffix(name, "_test.go") || regexp.MustCompile(`(?m)^//go:build `).Match(b) {
			return
		}
		f := parsed(t, name, b)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Type.Params.NumFields() == 1 && fn.Type.Results == nil {
				found[path.Dir(name)+"/"+fn.Name.Name] = true
			}
		}
	})
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

func componentOf(name string, components map[string]archComponent) string {
	for id, c := range components {
		var patterns []string
		switch v := c.In.(type) {
		case string:
			patterns = []string{v}
		case []any:
			for _, p := range v {
				patterns = append(patterns, fmt.Sprint(p))
			}
		}
		for _, p := range patterns {
			base := strings.TrimSuffix(p, "/**")
			if name == base || (strings.HasSuffix(p, "/**") && strings.HasPrefix(name, base+"/")) {
				return id
			}
		}
	}
	return ""
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
	"cmd/internal/checkdata/check.go:Load":         "func(fsys fs.FS, out, diagnostics io.Writer) (*gamedata.DB, error)",
	"internal/session/game.go:Load":                "func(fsys fs.FS, dir string, extra ...fs.FS) (*Game, error)",
	"internal/layer2/gamedata/load.go:Load":        "func(fsys fs.FS) (*DB, error)",
	"internal/layer2/gamedata/load_dir.go:LoadDir": "func(fsys fs.FS, path string) (*DB, error)",
	"internal/layer5/sound/files.go:LoadEffects":   "func(fsys fs.FS) int",
	"internal/layer5/sound/bgm.go:LoadBGM":         "func(fsys fs.FS, songs []Song, first string, oneShot []string, vol float64)",
	"internal/play/msgfit.go:LoadFonts":            "func(db *data.DB, fsys fs.FS) error",
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
	yamlBaselineLimit = 744
	yamlBaseline      = `
acceleration accent accepts act action activation activity activity_attributes
add add_state addresses admission advice affect affection affiliation
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
consume control controls cost count counter cover crit
critical critical_rate critical_ratio crowd currency cursor cut cutin
cutin_box cutin_lines cx damage damage_rate darkness data data_version
date date_format date_link day deadline deal defeat defend
defense dependency_ratio depth depth_bias desc description destination detail
dim dir dive done door dot drag drain
draw_roads drop drops due duo_chance duos each east
echo edge effect effects element elements else enabled
end_turn end_turn_button endings endure enemy enter entity episodes
equip equip_slots equip_stones escape evening event_label event_when every
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
into intro item items join join_exp joined keeper
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
on_enter on_escape on_leave on_lose on_turn on_turn_end on_turn_end_allies on_user
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
release remove remove_on_damage remove_state repetitions reply reposts
requires reserve reset_actor reset_tree resist respond rest restart
restore_checkpoint restrict restrict_chance resume_to retain retry revive ring
rise roamers rollback roofs rooms rope rounds row
rule rules sacrifice safe safe_area sample sample_freshness sample_label
save scene_review scenes schedule screen_operations scroll search seed
seed_xor seen selection_panel selector self self_backdrop self_label self_stage
sell send sex sex_ratio sfx sfx_vary shadow shake
shape share shop shops short side sight sign
simple_tactics size skill skill_slots skip sky slide slot
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
		if !strings.HasPrefix(name, "internal/layer2/gamedata/") || strings.HasSuffix(name, "_test.go") {
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
		if imported == module+"internal/clihelp" {
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
		{"parser", `package main; import cli "github.com/nao1215/gozen/internal/clihelp"; func handler() { cli.Parse(nil,nil) }`, true},
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
	for _, tc := range []struct {
		name, event string
		files       []string
		history     bool
		code, docs  string
	}{
		{"release notes PR", "pull_request", []string{"CHANGELOG.md"}, true, "false", "false"},
		{"mixed docs PR", "pull_request", []string{"CHANGELOG.md", "README.md"}, true, "false", "true"},
		{"mixed code PR", "pull_request", []string{"CHANGELOG.md", "main.go"}, true, "true", "true"},
		{"executable examples", "pull_request", []string{"docs/examples/game.yaml"}, true, "true", "true"},
		{"unknown path", "pull_request", []string{"unknown"}, true, "true", "true"},
		{"main push", "push", []string{"CHANGELOG.md"}, true, "true", "true"},
		{"manual run", "workflow_dispatch", []string{"CHANGELOG.md"}, true, "true", "true"},
		{"empty diff", "pull_request", nil, true, "true", "true"},
		{"missing history", "pull_request", []string{"CHANGELOG.md"}, false, "true", "true"},
	} {
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
			for _, want := range []string{"code=" + tc.code + "\n", "docs_runtime=" + tc.docs + "\n"} {
				if !strings.Contains("\n"+string(got), "\n"+want) {
					t.Fatalf("outputs %q missing %q", got, want)
				}
			}
		})
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
	ok := "ok  \tgithub.com/nao1215/gozen/internal/rpg\t0.1s\n"
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
		{"unlisted package", "windows", []string{gcCrash("github.com/nao1215/gozen/internal/play")}, 1, []string{"test ./..."}},
		{"crash beside an ordinary failure", "windows", []string{gcCrash(mosaic) + "--- FAIL: TestY (0.00s)\nFAIL\nFAIL\tgithub.com/nao1215/gozen/internal/rpg\t0.1s\n"}, 1, []string{"test ./..."}},
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
