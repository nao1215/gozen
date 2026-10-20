package gamedata

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Hurt is a command that reduces the HP of the given party member outside battle. The self in amount is the target member.
type Hurt struct {
	Who    string `yaml:"who"`
	Amount Expr   `yaml:"amount"`
}

// Affect is the affect command: who (all, or an actor id) gets the state add for turns
// (0: the state's own), or loses the state remove ("bad": every bad state).
type Affect struct {
	Who    string `yaml:"who"`
	Add    string `yaml:"add"`
	Turns  int    `yaml:"turns"`
	Remove string `yaml:"remove"`
	// Resist: the gear's and states' state_resist may keep the state off (a trap's
	// mist that a stone wards off), rolled for each one; without it the state is put on.
	Resist bool `yaml:"resist"`
}

// Command is a single event command. The command tags are the only registry of verbs and their accompanying fields.
// verb is a public verb, internal is Go-only, and any other value lists the verbs that allow the accompanying field.
type Command struct {
	Say    *Say           `yaml:"say" command:"verb"`
	Choice []ChoiceOption `yaml:"choice" command:"verb"`
	Prompt string         `yaml:"prompt" command:"choice,stone_choose,stone_wearer"` // a line shown with the choice
	If     Expr           `yaml:"if" command:"verb"`
	Then   []Command      `yaml:"then" command:"if"`
	Else   []Command      `yaml:"else" command:"if"`
	SetV   Assignments    `yaml:"set" command:"verb"`
	Give   *ItemCount     `yaml:"give" command:"verb"`
	Take   *ItemCount     `yaml:"take" command:"verb"`
	Gold   Expr           `yaml:"gold" command:"verb"`
	Join   string         `yaml:"join" command:"verb"`
	Leave  string         `yaml:"leave" command:"verb"`
	Heal   string         `yaml:"heal" command:"verb"`
	// Done marks st.Done[Done] (not written in YAML: the engine's own, a once dungeon tile
	// spent only when its question is answered yes).
	Done string `yaml:"-" command:"internal"`
	// Ran counts a run of the event (not written in YAML: the arrival puts it before the
	// enter events it copies; the runner counts the events it starts by themselves).
	Ran string `yaml:"-" command:"internal"`
	// Cross and Reach go along a road of a map and stand on a point (not written in YAML:
	// a point of a map taken from the menu puts them, one road at a time; ADR 0022).
	Cross string `yaml:"-" command:"internal"`
	Reach string `yaml:"-" command:"internal"`
	// Hurt takes HP from members outside battle (a poison floor, a trap); never below 1.
	Hurt *Hurt `yaml:"hurt" command:"verb"`
	// Affect puts members in a state, or takes one away, outside battle.
	Affect *Affect   `yaml:"affect" command:"verb"`
	Battle string    `yaml:"battle" command:"verb"`
	OnWin  []Command `yaml:"on_win" command:"battle"`
	OnLose []Command `yaml:"on_lose" command:"battle"`
	// Fighters are the actors who fight the battle (default: the party): a duel.
	Fighters []string `yaml:"fighters" command:"battle"`
	// Retry (retry: false) does not ask "try again" after a loss (for battles where losing is part of the story).
	Retry   *bool     `yaml:"retry" command:"battle"`
	OnRun   []Command `yaml:"on_escape" command:"battle"`
	Move    string    `yaml:"move" command:"verb"`
	At      string    `yaml:"at" command:"move,dive"`     // with move into a dungeon: the map character to arrive at
	Facing  string    `yaml:"facing" command:"move,dive"` // and the way to face
	Music   *MusicCue `yaml:"music" command:"verb"`
	Mood    string    `yaml:"mood" command:"verb"`
	Image   *CG       `yaml:"image" command:"verb"`
	Shop    string    `yaml:"shop" command:"verb"`
	Call    string    `yaml:"call" command:"verb"`
	Wait    float64   `yaml:"wait" command:"verb"`
	waitSet bool      // waitSet bool // in spec version 1, a wait time of 0 also counts as a command.
	// Notice is a short message shown at the top of the screen (no dialogue window, and play continues without stopping; narration-only notices such as the pain of a floor trap).
	Notice string `yaml:"notice" command:"verb"`
	SFX    string `yaml:"sfx" command:"verb"`
	Exp    Expr   `yaml:"exp" command:"verb"`
	Learn  *Learn `yaml:"learn" command:"verb"`
	// ResetTree gives back the skill points put into the skill tree of an actor (an id, or
	// "all" for everyone who has joined): to raise it again another way.
	ResetTree string `yaml:"reset_tree" command:"verb"`
	Effect    string `yaml:"effect" command:"verb"`
	End       bool   `yaml:"end" command:"verb"`
	Clear     bool   `yaml:"clear" command:"verb"`
	Save      bool   `yaml:"save" command:"verb"`
	Open      string `yaml:"open" command:"verb"`
	Dive      string `yaml:"dive" command:"verb"`
	Floor     Expr   `yaml:"floor" command:"dive"`
	// OpenWall turns the wall next to the cell the player is standing on in the dungeon (the facing one first, otherwise the nearest wall facing a floor)
	// into the tile of that character and faces it. It persists per floor and in saves (e.g. stairs that appear at 100% exploration).
	OpenWall string `yaml:"open_wall" command:"verb"`
	// StoneSuggest finds the strongest way to equip all owned stones and, if it beats the current setup, writes per-member changes to
	// var.<name>_* (var.<name> is the number of members to change). StoneChoose lets the player pick one buildable combination,
	// StoneWearer lets the player pick who wears it (for players who want to choose themselves).
	// StoneDecline remembers a refusal (stone_suggest does not advise again until the owned stones change).
	StoneSuggest string `yaml:"stone_suggest" command:"verb"`
	// StoneApply re-equips everyone's stones at once to the stone_suggest arrangement (the strongest one).
	StoneApply   string `yaml:"stone_apply" command:"verb"`
	StoneChoose  string `yaml:"stone_choose" command:"verb"`
	StoneWearer  string `yaml:"stone_wearer" command:"verb"`
	StoneDecline string `yaml:"stone_decline" command:"verb"`
	// SPSuggest builds a plan that spends leftover SP automatically per member (the same allocation as SpendSP) and writes it to
	// var.<name>_* (var.<name> is the number of members to allocate for). It does not spend. SPApply spends everything according to the plan.
	// SPDecline remembers a refusal (sp_suggest does not advise again until a member's SP changes).
	SPSuggest string `yaml:"sp_suggest" command:"verb"`
	SPApply   string `yaml:"sp_apply" command:"verb"`
	SPDecline string `yaml:"sp_decline" command:"verb"`
	// EquipStones equips the combination's stones into that member's stone slots (from the bag, otherwise taking stones from members not used in the combination).
	EquipStones *EquipStones `yaml:"equip_stones" command:"verb"`
	// Pick selects an item of the current place's menu (member, action, exit) by display name. If no item has exactly that name, it takes the first item whose display name contains it (spec version 2).
	Pick string `yaml:"pick" command:"verb"`
	// Turn changes facing in the dungeon: left, right, back (spec version 2).
	Turn string `yaml:"turn" command:"verb"`
	// Step moves forward Step cells in the dungeon. Floor traps and battles happen on each step (spec version 2).
	Step int `yaml:"step" command:"verb"`
	// Walk walks the shortest path to the tile of that character in the dungeon (spec version 2).
	Walk string `yaml:"walk" command:"verb"`
	// Use uses an item from the menu. Target is the receiving member (the first member if omitted; spec version 2).
	Use    string `yaml:"use" command:"verb"`
	Target string `yaml:"target" command:"use"`
	// Equip equips a bag item on a member. Omitting Item removes the Slot, and omitting both selects "strongest equipment" (spec version 2).
	Equip *EquipItem `yaml:"equip" command:"verb"`
	// Buy buys at a shop and Sell sells at a shop (same as the shop screen's "buy" and "sell"; spec version 2).
	Buy  *Trade `yaml:"buy" command:"verb"`
	Sell *Trade `yaml:"sell" command:"verb"`
	// Act is one move of the next AUTO battle (user, skill or item, target). Moves are used in written order on the turn when that user's turn comes (spec version 2).
	Act *BattleAct `yaml:"act" command:"verb"`
	// custom is the name of a custom command declared in game.yaml's verbs (an unknown YAML field), and customArg is its value.
	custom    []string
	customArg any
	// given is a field explicitly written in YAML. false, 0, an empty string, and null are distinguished from omitted.
	given []string
}

// EquipStones puts the stones of a stone set on an actor (both are expressions giving ids).
type EquipStones struct {
	Actor Expr `yaml:"actor"`
	Set   Expr `yaml:"set"`
}

// EquipItem is the equip command: the actor puts on the item (from the bag) in the slot
// (empty: the item's first slot). Without item the slot is emptied.
type EquipItem struct {
	Actor string `yaml:"actor"`
	Item  string `yaml:"item"`
	Slot  string `yaml:"slot"`
}

// Trade is the content of buy and sell: shop, item, and count (default 1).
type Trade struct {
	Shop  string `yaml:"shop"`
	Item  string `yaml:"item"`
	Count int    `yaml:"count"`
}

// BattleAct is the content of act: the user (member ID), a skill or an item, and the target (an enemy ID or member ID; if omitted, the skill's default).
type BattleAct struct {
	Actor  string `yaml:"actor"`
	Skill  string `yaml:"skill"`
	Item   string `yaml:"item"`
	Target string `yaml:"target"`
}

// CustomVerb returns the name of a custom command declared in game.yaml's verbs and its value (as written in YAML).
// It returns "" if the command is not a custom one.
func CustomVerb(c *Command) (string, any) {
	if len(c.custom) == 0 {
		return "", nil
	}
	return c.custom[0], c.customArg
}

// customFields returns the names of fields the command does not know (ones declared in game.yaml's verbs, or typos).
func customFields(c *Command) []string { return c.custom }

// Say is a line of dialogue.
type Say struct {
	Who  string `yaml:"who"`  // an actor id, or any name
	Face string `yaml:"face"` // a face of the actor (or a picture path)
	Text string `yaml:"text"`
	Side string `yaml:"side"` // left, right (default: left for party, right for others)
	// Instead is a substitute used when who is down during battle. It shows the line of the first member who is not down
	// (skipped if everyone is down or there is none). It is not used outside battle.
	Instead []Say `yaml:"instead"`
}

func (s *Say) UnmarshalYAML(unmarshal func(any) error) error {
	var t string
	if err := unmarshal(&t); err == nil {
		s.Text = t
		return nil
	}
	type plain Say
	return unmarshal((*plain)(s))
}

// ChoiceOption is one answer of a choice.
type ChoiceOption struct {
	ID    string    `yaml:"id"` // unique identifier within the same question
	Label string    `yaml:"label"`
	When  Expr      `yaml:"when"`
	Do    []Command `yaml:"do"`
}

// ItemCount is an item and how many.
type ItemCount struct {
	Item  string `yaml:"item"`
	Count *int   `yaml:"count,omitempty"` // omitted means 1. In spec version 1, 0 means no change.
}

// N is the count. It returns 1 only when omitted. Load turns the legacy 0 back into omitted.
func (i ItemCount) N() int {
	if i.Count == nil {
		return 1
	}
	return *i.Count
}

// Learn teaches an actor a skill.
type Learn struct {
	Actor string `yaml:"actor"`
	Skill string `yaml:"skill"`
}

// commandField is runtime registration info built once from the Command tags.
type commandField struct {
	name   string
	index  int
	role   string
	owners []string
}

// commandKeys are the YAML field names of Command (to tell them apart from custom commands).
var commandKeys = func() map[string]bool {
	keys := map[string]bool{}
	for f := range reflect.TypeFor[Command]().Fields() {
		if name, _, _ := strings.Cut(f.Tag.Get("yaml"), ","); name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}()

var commandFields = func() []commandField {
	var fields []commandField
	for f := range reflect.TypeFor[Command]().Fields() {
		role := f.Tag.Get("command")
		if role == "" {
			continue
		}
		name := f.Tag.Get("yaml")
		if role == "internal" {
			name = strings.ToLower(f.Name)
		}
		fields = append(fields, commandField{name, f.Index[0], role, strings.Split(role, ",")})
	}
	return fields
}()

// UnmarshalYAML keeps the explicitly written keys in addition to the values. Verb validity is checked after the spec version is filled in.
// Unknown fields are held as a custom command (game.yaml's verbs) with name and value, and the validation checks whether it is declared.
func (c *Command) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Command
	var out plain
	var raw yaml.MapSlice
	if err := unmarshal(&raw); err != nil {
		return err
	}
	known := make(yaml.MapSlice, 0, len(raw))
	var custom []string
	var customArg any
	for _, entry := range raw {
		key := fmt.Sprint(entry.Key)
		if commandKeys[key] {
			known = append(known, entry)
			continue
		}
		if len(custom) == 0 {
			customArg = entry.Value
		}
		custom = append(custom, key)
	}
	if len(custom) == 0 {
		if err := unmarshal(&out); err != nil {
			return err
		}
	} else if len(known) > 0 {
		b, err := yaml.Marshal(known)
		if err != nil {
			return err
		}
		if err := yaml.UnmarshalWithOptions(b, &out, yaml.Strict()); err != nil {
			return err
		}
	}
	for _, entry := range known {
		out.given = append(out.given, fmt.Sprint(entry.Key))
	}
	out.custom, out.customArg = custom, customArg
	*c = Command(out)
	return nil
}

// Verb returns the verb and validates missing or duplicate verbs and the placement of accompanying fields against the same registry.
// Commands built from Go use non-zero values. YAML counts a written field even if it is the zero value.
func (c *Command) Verb() (string, error) {
	value := reflect.ValueOf(c).Elem()
	var verbs []string
	var companions []commandField
	active := false
	for _, field := range commandFields {
		set := !value.Field(field.index).IsZero()
		if field.name == "wait" {
			set = c.Wait > 0 || c.waitSet
		}
		if !set && !slices.Contains(c.given, field.name) {
			continue
		}
		if field.role == "verb" || field.role == "internal" {
			verbs = append(verbs, field.name)
			active = set
		} else {
			companions = append(companions, field)
		}
	}
	if len(c.custom) > 0 {
		switch {
		case len(verbs) > 0:
			return "", fmt.Errorf("a %s command with the unknown field %s", verbs[0], strings.Join(c.custom, ", "))
		case len(c.custom) > 1:
			sort.Strings(c.custom)
			return "", fmt.Errorf("a command with several unknown verbs (%s)", strings.Join(c.custom, ", "))
		case len(companions) > 0:
			return "", fmt.Errorf("%s belongs to %s, not %s", companions[0].name, strings.Join(companions[0].owners, " or "), c.custom[0])
		}
		return c.custom[0], nil
	}
	if len(verbs) == 0 {
		return "", errors.New("a command without a verb")
	}
	if len(verbs) > 1 {
		sort.Strings(verbs)
		return "", fmt.Errorf("a command with several verbs (%s): write them as separate list items", strings.Join(verbs, ", "))
	}
	verb := verbs[0]
	for _, field := range companions {
		if !slices.Contains(field.owners, verb) {
			return "", fmt.Errorf("%s belongs to %s, not %s", field.name, strings.Join(field.owners, " or "), verb)
		}
	}
	if !active {
		return "", fmt.Errorf("a command without a verb: %s has no active value", verb)
	}
	return verb, nil
}
