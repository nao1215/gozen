package gamedata

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

// Vocabulary added in spec version 2 (ADR 0021): commands, the left-hand sides of set, game.yaml's verbs and screens, and the names usable in event expressions.

// NewVerbs are the commands added in spec version 2.
var NewVerbs = []string{"pick", "turn", "step", "walk", "use", "equip", "buy", "sell", "act"}

// EventNames are names usable only in expressions inside event commands (progress and screen names outside the state).
// menu is an item of the current place's menu, map is a row of the current dungeon floor's map, cell(x, y) is a dungeon cell,
// roamers are the strong enemies on the current dungeon floor (kind, name, troop, x, y), result is the outcome of the last battle,
// over and cleared tell whether it has ended, and repairs is the correction applied when a save was loaded.
var EventNames = []string{"menu", "map", "cell", "roamers", "result", "over", "cleared", "repairs", "ran", "completed", "moved"}

// DevSetFields are the member values that set may overwrite only under --dev (the test and development screen).
// In real play they are not overwritten but only recorded, and the check warns (author's decision 5).
var DevSetFields = []string{"hp", "mp", "exp", "level", "sp"}

// SetFields are the member values that set can write (actor.<ID>.<name>).
var SetFields = append(slices.Clone(DevSetFields), "tactics")

var declaredName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// withEventNames adds EventNames to the names usable inside commands.
func withEventNames(names []string) []string {
	if !slices.Contains(names, "flag") || slices.Contains(names, "menu") {
		return names
	}
	out := slices.Clone(names)
	for _, n := range EventNames {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// needsVersion2 reports that spec version 2 vocabulary was used in version 1 or earlier.
func (c *checker) needsVersion2(where, what string) bool {
	return c.needsVersion(where, what, SchemaVersion2)
}

// needsVersion reports that spec version n vocabulary was used in an earlier version.
func (c *checker) needsVersion(where, what string, n int) bool {
	if c.script || c.db.Game.SchemaVersion >= n {
		return true
	}
	c.errorf(diag.SchemaVersion, where, "%s needs schema_version: %d in game.yaml", what, n)
	return false
}

// declarations checks game.yaml's verbs and screens.
func (c *checker) declarations() {
	g := &c.db.Game
	for _, v := range slices.Sorted(maps.Keys(g.Verbs)) {
		w := "game.yaml.verbs." + v
		c.needsVersion2("game.yaml verbs", "verbs")
		if !declaredName.MatchString(v) {
			c.errorf(diag.NotAWord, w, "want a name of lowercase letters, digits and _, not %q", v)
		}
		if commandKeys[v] {
			c.errorf(diag.NotAWord, w, "%q is a built-in command or one of its fields", v)
		}
		if g.Verbs[v] == "" || c.db.Events[g.Verbs[v]] == nil {
			c.errorf(diag.UnknownReference, w, "unknown event %q", g.Verbs[v])
		}
	}
	for _, name := range slices.Sorted(maps.Keys(g.Screens)) {
		w := "game.yaml.screens." + name
		c.needsVersion2("game.yaml screens", "screens")
		if !declaredName.MatchString(name) {
			c.errorf(diag.NotAWord, w, "want a name of lowercase letters, digits and _, not %q", name)
		}
		if slices.Contains(OpenScreens, name) {
			c.errorf(diag.NotAWord, w, "%q is a built-in screen", name)
		}
		screen := g.Screens[name]
		if screen == nil {
			c.errorf(diag.Shape, w, "a screen needs a title and at least one entry")
			continue
		}
		if screen.Title == "" || len(screen.Entries) == 0 {
			c.errorf(diag.Shape, w, "a screen needs a title and at least one entry")
		}
		c.text(w+".title", screen.Title, FieldNames)
		for i, e := range screen.Entries {
			ew := fmt.Sprintf("%s.entries[%d]", w, i)
			if e.Label == "" {
				c.errorf(diag.Shape, ew, "an entry needs a label")
			}
			c.text(ew+".label", e.Label, FieldNames)
			c.text(ew+".text", e.Text, FieldNames)
			c.file(ew+".picture", e.Picture)
			c.expr(ew+".when", e.When, FieldNames)
		}
	}
}

// unknownFields checks fields the command does not know. If the name declared in game.yaml's verbs is
// the only verb, it passes as a custom command; anything else (a typo) is an error with a similar name attached.
// It returns true if there is an unknown field (it does not check the Verb).
func (c *checker) unknownFields(w string, cmd *Command) bool {
	fields := customFields(cmd)
	if len(fields) == 0 {
		return false
	}
	builtin := len(cmd.given) > 0
	if len(fields) == 1 && !builtin && c.db.Game.Verbs[fields[0]] != "" {
		c.needsVersion2(w+"."+fields[0], "a declared command")
		if !c.db.resolveVerb(cmd) {
			c.errorf(diag.Shape, w, "invoke %s with true; its body is the event %s", fields[0], c.db.Game.Verbs[fields[0]])
			return true
		}
		return false
	}
	for _, f := range fields {
		msg := fmt.Sprintf("unknown field %q", f)
		if near := c.nearest(f); near != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", near)
		} else if !builtin {
			msg += " (a command of the game's own is declared in game.yaml verbs)"
		}
		c.errorf(diag.Shape, w+"."+f, "%s", msg)
	}
	return true
}

// nearest is the command field or declared command name closest to name (candidate for a typo). "" if none.
func (c *checker) nearest(name string) string {
	best, bestD := "", 3
	candidates := slices.Sorted(maps.Keys(commandKeys))
	candidates = append(candidates, slices.Sorted(maps.Keys(c.db.Game.Verbs))...)
	for _, k := range candidates {
		if d := editDistance(name, k); d < bestD && d < len([]rune(name)) {
			best, bestD = k, d
		}
	}
	return best
}

// editDistance is an edit distance that counts swapping two adjacent characters as 1.
func editDistance(a, b string) int {
	// Suggestions only use distances below three. Large names cannot be suggestions;
	// avoid a quadratic allocation for hostile author input.
	if len(a) > 256 || len(b) > 256 {
		return 3
	}
	x, y := []rune(a), []rune(b)
	d := append(make([][]int, len(x)), nil)
	for i := range d {
		d[i] = append(make([]int, len(y)), 0)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(x); i++ {
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(x)][len(y)]
}

// newVerb checks the spec version 2 commands (other than use).
func (c *checker) newVerb(w, verb string, cmd *Command) {
	db := c.db
	switch verb {
	case "pick":
		if strings.TrimSpace(cmd.Pick) == "" {
			c.errorf(diag.Shape, w+".pick", "pick needs the label of a menu entry")
		}
	case "turn":
		if cmd.Turn != "left" && cmd.Turn != "right" && cmd.Turn != "back" {
			c.errorf(diag.NotAWord, w+".turn", "want left, right or back, not %q", cmd.Turn)
		}
	case "step":
		if cmd.Step < 1 {
			c.errorf(diag.OutOfRange, w+".step", "step is the number of steps (1 or more)")
		}
	case "walk":
		if r := []rune(cmd.Walk); len(r) != 1 || !c.dungeonTile(cmd.Walk) {
			c.errorf(diag.UnknownReference, w+".walk", "%q is not a tile of any dungeon (one character in tiles)", cmd.Walk)
		}
	case "equip":
		e := cmd.Equip
		_, ok := db.Actors[e.Actor]
		c.ref(w+".equip.actor", "actor", e.Actor, ok)
		if e.Item != "" {
			it, ok := db.Items[e.Item]
			c.ref(w+".equip.item", "item", e.Item, ok)
			if ok && !it.Equipment() {
				c.errorf(diag.UnknownReference, w+".equip.item", "%s is not equipment (it has no slot)", e.Item)
			}
			if ok && e.Slot != "" && !it.Fits(e.Slot) {
				c.errorf(diag.UnknownReference, w+".equip.slot", "%s does not go in %s", e.Item, e.Slot)
			}
		}
		if e.Slot != "" && !slices.ContainsFunc(db.Game.Slots, func(s Slot) bool { return s.ID == e.Slot || strings.HasPrefix(e.Slot, s.ID) }) {
			c.errorf(diag.UnknownReference, w+".equip.slot", "%q is not an equip slot", e.Slot)
		}
	case "buy", "sell":
		t := cmd.Buy
		if verb == "sell" {
			t = cmd.Sell
		}
		sh, ok := db.Shops[t.Shop]
		c.ref(w+"."+verb+".shop", "shop", t.Shop, ok)
		it, iok := db.Items[t.Item]
		c.ref(w+"."+verb+".item", "item", t.Item, iok)
		if t.Count < 0 {
			c.errorf(diag.OutOfRange, w+"."+verb+".count", "count is 1 or more")
		}
		if ok && iok && verb == "buy" && !slices.Contains(sh.Goods, t.Item) {
			c.errorf(diag.UnknownReference, w+".buy.item", "%s does not sell %s", t.Shop, t.Item)
		}
		if ok && iok && verb == "sell" && (sh.Sell != nil && !*sh.Sell || it.Kind == "key" || it.Price <= 0) {
			c.errorf(diag.UnknownReference, w+".sell", "%s cannot be sold at %s", t.Item, t.Shop)
		}
	case "act":
		a := cmd.Act
		_, ok := db.Actors[a.Actor]
		c.ref(w+".act.actor", "actor", a.Actor, ok)
		if (a.Skill == "") == (a.Item == "") {
			c.errorf(diag.Shape, w+".act", "act takes a skill or an item")
		}
		if a.Skill != "" {
			_, ok := db.Skills[a.Skill]
			c.ref(w+".act.skill", "skill", a.Skill, ok)
		}
		if a.Item != "" {
			_, ok := db.Items[a.Item]
			c.ref(w+".act.item", "item", a.Item, ok)
		}
		if a.Target != "" && db.Actors[a.Target] == nil && db.Enemies[a.Target] == nil {
			c.errorf(diag.UnknownReference, w+".act.target", "%q is neither an actor nor an enemy", a.Target)
		}
	}
}

// setKey checks the left-hand side of set. Anything other than flag and var is spec version 2.
func (c *checker) setKey(w, k string, e Expr) {
	if strings.HasPrefix(k, "flag.") || strings.HasPrefix(k, "var.") {
		return
	}
	if c.campaignSetKey(w, k, e) {
		return
	}
	db := c.db
	kind, rest, _ := strings.Cut(k, ".")
	text := ""
	if LiteralSet(k) && e.Set() {
		if v, err := e.Eval(nil); err == nil { // a name written as is is an expression of a quoted string
			if s, ok := v.(string); ok {
				text = s
			}
		}
	}
	switch {
	case k == "gold" || k == "calm" || k == "pos.x" || k == "pos.y" || k == "steps":
	case k == "pos.facing":
		if FacingIndex(text) < 0 || text == "" {
			c.errorf(diag.NotAWord, w+"."+k, "facing is north, east, south or west, not %q", text)
		}
	case k == "roamers":
		if text != "none" {
			c.errorf(diag.Shape, w+"."+k, "roamers can be set only to none (the strong foes leave the floor)")
		}
	case k == "location":
		_, ok := db.Locations[text]
		c.ref(w+"."+k, "location", text, ok)
	case k == "spot":
		c.needsVersion(w+"."+k, "set spot", SchemaVersion3)
	case k == "party":
		ids := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })
		if len(ids) == 0 || len(ids) > db.Game.MaxParty {
			c.errorf(diag.OutOfRange, w+"."+k, "party takes 1 to %d actors", db.Game.MaxParty)
		}
		for _, id := range ids {
			_, ok := db.Actors[id]
			c.ref(w+"."+k, "actor", id, ok)
		}
	case kind == "item" && rest != "":
		_, ok := db.Items[rest]
		c.ref(w+"."+k, "item", rest, ok)
	case kind == "actor" && rest != "":
		id, field, _ := strings.Cut(rest, ".")
		_, ok := db.Actors[id]
		c.ref(w+"."+k, "actor", id, ok)
		if !slices.Contains(SetFields, field) {
			c.errorf(diag.Shape, w+"."+k, "set can change actor.<id>.%s only (not %q)", strings.Join(SetFields, ", actor.<id>."), field)
			return
		}
		if _, preset := db.Game.Battle.TacticPresets[text]; field == "tactics" && text != "auto" && text != "manual" && !preset && !strings.HasPrefix(text, "{") {
			c.errorf(diag.NotAWord, w+"."+k, "tactics is auto, manual or a preset of battle.tactic_presets, not %q", text)
		}
		if field == "tactics" && strings.HasPrefix(text, "{") {
			c.tacticsMapping(w+"."+k, text)
		}
	default:
		c.errorf(diag.Shape, w, "%q: set changes flag.<name>, var.<name>, gold, item.<id>, actor.<id>.<%s>, party, location, spot, calm, pos.x, pos.y, pos.facing, steps and roamers", k, strings.Join(SetFields, "|"))
		return
	}
	c.needsVersion2(w+"."+k, "set "+k)
}

// tacticsMapping checks set actor.<id>.tactics written as a mapping (the shape of saved
// tactics, kept as JSON by literalText): its mode, and the preset it names, which the game
// would otherwise take and give the member no rows in battle.
func (c *checker) tacticsMapping(where, text string) {
	var t struct {
		Mode   string      `json:"mode"`
		Preset string      `json:"preset"`
		Rows   []TacticRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(text), &t); err != nil {
		c.errorf(diag.Shape, where, "tactics: %v", err)
		return
	}
	switch t.Mode {
	case "auto", "manual", "preset":
	default:
		c.errorf(diag.NotAWord, where, "tactics: mode is auto, manual or preset, not %q", t.Mode)
		return
	}
	if t.Mode == "preset" && t.Preset == "" {
		c.errorf(diag.Shape, where, "tactics: mode preset needs preset (a name of battle.tactic_presets)")
	}
	if _, ok := c.db.Game.Battle.TacticPresets[t.Preset]; t.Preset != "" && !ok {
		c.errorf(diag.UnknownReference, where, "tactics: no preset %q in battle.tactic_presets", t.Preset)
	}
}

// DevSet reports whether the left-hand side of set is a value overwritten only under --dev (member stats and level, position, steps, strong enemies).
func DevSet(key string) bool {
	switch key {
	case "pos.x", "pos.y", "pos.facing", "steps", "roamers", "spot":
		return true
	}
	rest, ok := strings.CutPrefix(key, "actor.")
	if !ok {
		return false
	}
	_, field, _ := strings.Cut(rest, ".")
	return slices.Contains(DevSetFields, field)
}

// Assignments are the left-hand side and value of set. Values for flag, var, gold, item, and member numbers are expressions,
// while left-hand sides that take only strings (location, party, actor.<ID>.tactics) take the string as written
// (location: camp, party: [scout, guide]). There is no way to choose a place by an expression (kept simple by a constraint; ADR 0021).
type Assignments map[string]Expr

// LiteralSet reports whether the left-hand side of set accepts the value as a string as written.
func LiteralSet(key string) bool {
	if key == "location" || key == "party" || key == "pos.facing" || key == "roamers" || key == "spot" {
		return true
	}
	rest, ok := strings.CutPrefix(key, "actor.")
	if !ok {
		return false
	}
	_, field, _ := strings.Cut(rest, ".")
	return field == "tactics"
}

// UnmarshalYAML reads the value as an expression or as a string as written, depending on the left-hand side.
func (a *Assignments) UnmarshalYAML(n ast.Node) error {
	for {
		switch x := n.(type) {
		case *ast.TagNode:
			n = x.Value
			continue
		case *ast.AnchorNode:
			n = x.Value
			continue
		}
		break
	}
	out := Assignments{}
	if _, ok := n.(*ast.NullNode); ok {
		*a = out
		return nil
	}
	m, ok := n.(*ast.MappingNode)
	if !ok {
		if v, ok := n.(*ast.MappingValueNode); ok {
			m = &ast.MappingNode{Values: []*ast.MappingValueNode{v}}
		} else {
			return &yaml.SyntaxError{Message: "set takes a mapping of targets and values", Token: n.GetToken()}
		}
	}
	for _, entry := range m.Values {
		key := sourceKey(entry.Key)
		if _, dup := out[key]; dup {
			return &yaml.SyntaxError{Message: fmt.Sprintf("set %s is written twice", key), Token: entry.Key.GetToken()}
		}
		var e Expr
		if LiteralSet(key) {
			text, err := literalText(entry.Value)
			if err != nil {
				return err
			}
			if e, err = parseExpr(quoteExprString(text)); err != nil {
				return err
			}
		} else if err := e.UnmarshalYAML(entry.Value); err != nil {
			return err
		}
		out[key] = e
	}
	*a = out
	return nil
}

// literalText is a string as written, a list of strings (joined with ,), or a mapping (turned into JSON, e.g. tactic rows).
func literalText(n ast.Node) (string, error) {
	if _, ok := n.(*ast.MappingNode); ok {
		var v any
		if err := yaml.NodeToValue(n, &v); err != nil {
			return "", err
		}
		b, err := json.Marshal(v)
		return string(b), err
	}
	if s, ok := n.(*ast.SequenceNode); ok {
		parts := make([]string, 0, len(s.Values))
		for _, v := range s.Values {
			t, err := scalarText(v)
			if err != nil {
				return "", err
			}
			parts = append(parts, t)
		}
		return strings.Join(parts, ", "), nil
	}
	return scalarText(n)
}

// resolveVerb reduces a reusable command to the existing event call contract.
// Validation, replay, traversal and simulation then share exactly one execution path.
func (db *DB) resolveVerb(c *Command) bool {
	if len(c.custom) != 1 || len(c.given) != 0 || !isTrue(c.customArg) {
		return false
	}
	target := db.Game.Verbs[c.custom[0]]
	if target == "" {
		return false
	}
	*c = Command{Call: target}
	return true
}

func isTrue(v any) bool { b, ok := v.(bool); return ok && b }
