package gamedata

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
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
	// Plan performs one deterministic planning operation.
	Plan *PlanCommand `yaml:"plan" command:"verb"`

	Respond *Respond       `yaml:"respond" command:"verb"`
	Say     *Say           `yaml:"say" command:"verb"`
	Choice  []ChoiceOption `yaml:"choice" command:"verb"`
	Prompt  string         `yaml:"prompt" command:"choice,stone_choose,stone_wearer"` // a line shown with the choice
	// ID names the choice: the state remembers the option taken there as chose.<id>
	// (schema_version 4, ADR 0025).
	ID    string      `yaml:"id" command:"choice"`
	If    Expr        `yaml:"if" command:"verb"`
	Then  []Command   `yaml:"then" command:"if"`
	Else  []Command   `yaml:"else" command:"if"`
	SetV  Assignments `yaml:"set" command:"verb"`
	Give  *ItemCount  `yaml:"give" command:"verb"`
	Take  *ItemCount  `yaml:"take" command:"verb"`
	Gold  Expr        `yaml:"gold" command:"verb"`
	Join  string      `yaml:"join" command:"verb"`
	Leave string      `yaml:"leave" command:"verb"`
	// Reserve swaps a reserve monster into one active slot.
	Reserve *ReserveSwap `yaml:"reserve" command:"verb"`
	// Release permanently removes one monster instance.
	Release string `yaml:"release" command:"verb"`
	// Breed consumes two monsters and creates a level-one child.
	Breed *BreedCmd `yaml:"breed" command:"verb"`
	// Capture recruits a species under its admission policy, also used by battle offers.
	Capture string `yaml:"capture" command:"verb"`
	Heal    string `yaml:"heal" command:"verb"`
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
	// Clash ends a campaign's march decided by a battle (not written in YAML: won or lost
	// and the order's id, as "won:attack", put after the battle by the order; ADR 0030).
	Clash string `yaml:"-" command:"internal"`
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
	// Recover defaults to true; false keeps defeated participants at their final HP.
	Recover *bool     `yaml:"recover" command:"battle"`
	Retry   *bool     `yaml:"retry" command:"battle"`
	OnRun   []Command `yaml:"on_escape" command:"battle"`
	Move    string    `yaml:"move" command:"verb"`
	At      string    `yaml:"at" command:"move,dive"`     // with move into a dungeon: the map character to arrive at
	Facing  string    `yaml:"facing" command:"move,dive"` // and the way to face
	Music   *MusicCue `yaml:"music" command:"verb"`
	Mood    string    `yaml:"mood" command:"verb"`
	// Carry keeps the values game.yaml carry names for the next new game (ADR 0053).
	Carry   bool      `yaml:"carry" command:"verb"`
	Image   *CG       `yaml:"image" command:"verb"`
	Shop    string    `yaml:"shop" command:"verb"`
	Call    EventCall `yaml:"call" command:"verb"`
	Wait    WaitCue   `yaml:"wait" command:"verb"`
	waitSet bool      // waitSet bool // in spec version 1, a wait time of 0 also counts as a command.
	// Notice is a short message shown at the top of the screen (no dialog window, and play continues without stopping; narration-only notices such as the pain of a floor trap).
	Notice string `yaml:"notice" command:"verb"`
	// Message queues a chat message (schema 10): a notification in the top-right
	// corner of a full-screen surface, opened into a bubble in the middle of the
	// screen. Queuing does not stop the event; opening uses no game time.
	Message *MessageCue `yaml:"message" command:"verb" json:",omitempty"`
	// CallView changes only how a video call looks (no state changes): for
	// example a participant's camera turning on in the middle of a call.
	CallView *CallViewCue `yaml:"call_view" command:"verb" json:",omitempty"`
	// Phone shows a phone over the scene, held upright or on its side, with one app
	// on it: a chat, a photo feed or a video (ADR 0057). It changes no state; the
	// event waits until the player puts the phone down.
	Phone *PhoneCue `yaml:"phone" command:"verb" json:",omitempty"`
	SFX   SFXCue    `yaml:"sfx" command:"verb"`
	Exp   Expr      `yaml:"exp" command:"verb"`
	Learn *Learn    `yaml:"learn" command:"verb"`
	// ResetTree gives back the skill points put into the skill tree of an actor (an id, or
	// "all" for everyone who has joined): to raise it again another way.
	ResetTree string `yaml:"reset_tree" command:"verb"`
	// ResetActor puts an actor back to the starting state the actors file gives: level, exp,
	// skills learnt, skill tree and SP, HP, MP and states (an id). Joined or not, equipment
	// and the name stay. Level sets the level it starts from (default: the actor's own level).
	ResetActor string    `yaml:"reset_actor" command:"verb"`
	Level      int       `yaml:"level" command:"reset_actor"`
	Effect     EffectCue `yaml:"effect" command:"verb"`
	// Stand brings a portrait onto the stage of the novel window or takes it off
	// (schema_version 4, ADR 0028).
	Stand *Stand `yaml:"stand" command:"verb"`
	End   bool   `yaml:"end" command:"verb"`
	Clear bool   `yaml:"clear" command:"verb"`
	// Checkpoint replaces the saved retry point; its value is the event run on restoration.
	Checkpoint string `yaml:"checkpoint" command:"verb"`
	// RestoreCheckpoint abandons the current execution and resumes the saved retry point.
	RestoreCheckpoint bool       `yaml:"restore_checkpoint" command:"verb"`
	Save              bool       `yaml:"save" command:"verb"`
	Open              ScreenCall `yaml:"open" command:"verb"`
	Dive              string     `yaml:"dive" command:"verb"`
	Floor             Expr       `yaml:"floor" command:"dive"`
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
	// Order gives an order of the campaign (schema_version 5, ADR 0030): the general and
	// the target region it needs are asked when not given.
	Order *OrderCmd `yaml:"order" command:"verb"`
	// EndTurn ends the campaign's turn: the other factions move and the next turn begins.
	EndTurn bool `yaml:"end_turn" command:"verb"`
	// GameOver ends the game lost, as a lost battle does (schema_version 5).
	GameOver bool `yaml:"game_over" command:"verb"`
	// custom is the name of a custom command declared in game.yaml's verbs (an unknown YAML field), and customArg is its value.
	custom    []string
	customArg any
	// given is a field explicitly written in YAML. false, 0, an empty string, and null are distinguished from omitted.
	given []string
}

// ReserveSwap exchanges an active monster with one in the reserve.
type ReserveSwap struct {
	In  string `yaml:"in"`
	Out string `yaml:"out"`
}

// BreedCmd names two owned instances and the skills to pass to the child.
type BreedCmd struct {
	Parents []string `yaml:"parents"`
	Skills  []string `yaml:"skills"`
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

// customFields returns the names of fields the command does not know (ones declared in game.yaml's verbs, or typos).
func customFields(c *Command) []string { return c.custom }

// Say is a line of dialog.
type Say struct {
	Instant  bool `yaml:"instant"`
	FaceExpr Expr `yaml:"face_expr,omitempty" json:",omitzero"`
	// Ensemble names a planning participant screen shown behind this line.
	Ensemble string `yaml:"ensemble"`
	Who      IDRef  `yaml:"who"`  // a literal name, or an actor expression
	Face     string `yaml:"face"` // a face of the actor (or a picture path)
	Text     string `yaml:"text"`
	// TextExpr selects a source template before translation and read-key hashing.
	// Narrative consumers require a pure string expression, exclusive with Text.
	TextExpr Expr   `yaml:"text_expr,omitempty" json:",omitzero"`
	Side     string `yaml:"side"` // left, right (default: left for party, right for others)
	// Voice is the line's voice: a name in the speaker's voices folder, or a path under
	// assets/ (schema_version 4, ADR 0028).
	Voice string `yaml:"voice"`
	// Instead is a substitute used when who is down during battle. It shows the line of the first member who is not down
	// (skipped if everyone is down or there is none). It is not used outside battle.
	Instead []Say `yaml:"instead"`
	// Values are the values of the text's {fields} that the game gives, by the field's
	// whole name (a reaction book's reply of respond: its from, text, mood, slot.TAG and
	// mem.KEY): the text is filled with them in place of its {expressions}.
	Values map[string]string `yaml:"-"`
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
	// LabelExpr selects a source template before translation and choice read-key hashing.
	// It is exclusive with Label, pure, and must return a string.
	LabelExpr Expr      `yaml:"label_expr,omitempty" json:",omitzero"`
	ID        string    `yaml:"id"` // unique identifier within the same question
	Label     string    `yaml:"label"`
	When      Expr      `yaml:"when"`
	Do        []Command `yaml:"do"`
}

// ItemCount is an item and how many.
type ItemCount struct {
	short bool   // A single item written as its ID (schema 11).
	Item  string `yaml:"item"`
	Count *int   `yaml:"count,omitempty"` // omitted means 1. In spec version 1, 0 means no change.
}

// UnmarshalYAML accepts an item ID for one item, or an explicit item/count map.
func (i *ItemCount) UnmarshalYAML(unmarshal func(any) error) error {
	var id string
	if err := unmarshal(&id); err == nil {
		*i = ItemCount{Item: id, short: true}
		return nil
	}
	type plain ItemCount
	return unmarshal((*plain)(i))
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
	// Inspect only the keys of ordinary commands. Decoding their entire subtree
	// into an untyped map first repeats that work at every nesting level. Keep
	// the caller's decoder for values so aliases retain their document context.
	var node ast.Node
	if err := unmarshal(&node); err != nil {
		return err
	}
	if mapping, ok := node.(*ast.MappingNode); ok {
		allKnown := true
		for _, entry := range mapping.Values {
			key, _ := decodedKey(entry.Key)
			if !commandKeys[key] {
				allKnown = false
				break
			}
		}
		if allKnown {
			if err := unmarshal(&out); err != nil {
				return err
			}
			for _, entry := range mapping.Values {
				key, _ := decodedKey(entry.Key)
				out.given = append(out.given, key)
			}
			*c = Command(out)
			return nil
		}
	}
	// Aliases and custom commands need the decoded key/value representation.
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
			set = c.Wait.Seconds > 0 || c.waitSet || c.Wait.mapped
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

// MessageCue is a queued chat message: who (an actor, or an actor expression),
// the interpolated text and the event that asks for the decision.
type MessageCue struct {
	Who  IDRef  `yaml:"who"`
	Text string `yaml:"text"`
	// Event is the parameterless event run when the message is opened, under
	// the bubble: a message asks for a decision (its choice), never only reads.
	Event string `yaml:"event"`
	// Tag is a short interpolated mark on the notification and the bubble, such
	// as DM for a one-to-one message (empty shows none).
	Tag string `yaml:"tag"`
}

// PhoneCue is a phone shown over the scene (ADR 0057). Hold is portrait (the
// default) or landscape; App is chat, feed, video, page or lock and names the
// one list the phone shows: Messages, Posts, Video, Rows or Notices; banner
// shows its Notices as banners at the top of the screen without waiting. Title is
// the app's header (interpolated). A page has either a URL in a browser's
// address bar or a Title on an app bar of Color (#rrggbb). A lock screen shows
// Time (the phone's clock when empty) and Date over its notices, which arrive
// on a press or, with Auto, by themselves. Var receives the result the phone
// was put down with: a page button's id (its label by default), answer or
// decline for a call.
type PhoneCue struct {
	Hold     string         `yaml:"hold"`
	App      string         `yaml:"app"`
	Title    string         `yaml:"title"`
	URL      string         `yaml:"url"`
	Color    string         `yaml:"color"`
	Time     string         `yaml:"time"`
	Date     string         `yaml:"date"`
	Auto     bool           `yaml:"auto"`
	Var      string         `yaml:"var"`
	Messages []PhoneMessage `yaml:"messages" json:",omitempty"`
	Posts    []PhonePost    `yaml:"posts" json:",omitempty"`
	Video    *PhoneVideo    `yaml:"video" json:",omitempty"`
	Rows     []PhoneRow     `yaml:"rows" json:",omitempty"`
	Notices  []PhoneNotice  `yaml:"notices" json:",omitempty"`
}

// PhoneRow is one row of a page, stacked top to bottom: Text (paragraphs, of
// Size large or small, Bold), a Picture shown whole with its Caption,
// Pictures in a grid of Columns (2 or 3, 0 is 3) a line, an Item of a list, or
// a Button that puts the phone down when pressed, setting Set and the phone's
// Var to its ID. A row is one of them. A picture or a grid is Locked (blurred,
// with a lock and its Label) while the condition holds when the phone comes up.
type PhoneRow struct {
	Text     string      `yaml:"text"`
	Size     string      `yaml:"size"`
	Bold     bool        `yaml:"bold"`
	Picture  string      `yaml:"picture"`
	Caption  string      `yaml:"caption"`
	Pictures []string    `yaml:"pictures" json:",omitempty"`
	Columns  int         `yaml:"columns"`
	Item     *PhoneItem  `yaml:"item" json:",omitempty"`
	Button   string      `yaml:"button"`
	ID       string      `yaml:"id"`
	Set      Assignments `yaml:"set" json:",omitempty"`
	Locked   Expr        `yaml:"locked"`
	Label    string      `yaml:"label"`
	// Shut is Locked worked out when the phone came up.
	Shut bool `yaml:"-" json:"-"`
}

// PhoneItem is a row of a list on a page: a round Icon under assets/, a Title,
// a Text under it and a Value on the right (a sum, a time).
type PhoneItem struct {
	Icon  string `yaml:"icon"`
	Title string `yaml:"title"`
	Text  string `yaml:"text"`
	Value string `yaml:"value"`
}

// PhoneNotice is a notification on the lock screen: the App's short name, its
// Icon under assets/, a Title and a Text. Call makes it an incoming call that
// fills the screen: Title is the caller; OK answers (setting Set) and Cancel
// declines. A banner's Event is the parameterless event a click on it opens.
type PhoneNotice struct {
	App   string      `yaml:"app"`
	Icon  string      `yaml:"icon"`
	Title string      `yaml:"title"`
	Text  string      `yaml:"text"`
	Call  bool        `yaml:"call"`
	Set   Assignments `yaml:"set" json:",omitempty"`
	Event string      `yaml:"event"`
}

// PhoneResult is one way the phone can be put down that the story keeps: a
// page's button (Row), a call answered (Notice) or the last call declined
// (both -1). ID is what the phone's Var receives and Set what it sets.
type PhoneResult struct {
	Row, Notice int
	ID          string
	Set         Assignments
}

// Results are the phone's results in order, when it keeps any (it has a Var
// or one of them sets something); nil otherwise.
func (p PhoneCue) Results() []PhoneResult {
	var out []PhoneResult
	keeps := p.Var != ""
	for i, r := range p.Rows {
		if r.Button == "" {
			continue
		}
		id := r.ID
		if id == "" {
			id = r.Button
		}
		out = append(out, PhoneResult{Row: i, Notice: -1, ID: id, Set: r.Set})
		keeps = keeps || len(r.Set) > 0
	}
	calls := false
	for i, n := range p.Notices {
		if n.Call && p.App == "lock" {
			out = append(out, PhoneResult{Row: -1, Notice: i, ID: "answer", Set: n.Set})
			keeps, calls = keeps || len(n.Set) > 0, true
		}
	}
	if calls {
		out = append(out, PhoneResult{Row: -1, Notice: -1, ID: "decline"})
	}
	if !keeps {
		return nil
	}
	return out
}

// Do are the commands a result runs: its sets and the phone's Var.
func (p PhoneCue) Do(r PhoneResult) []Command {
	var out []Command
	if len(r.Set) > 0 {
		out = append(out, Command{SetV: r.Set})
	}
	if p.Var != "" {
		out = append(out, Command{SetV: Assignments{p.Var: MustExpr(strconv.Quote(r.ID))}})
	}
	return out
}

// PhoneMessage is one bubble of a chat. Mine puts it on the right, as sent by
// the phone's owner; From names the sender of the others (interpolated) and Icon
// is their round picture under assets/. Text, Picture (under assets/) or Video
// is the bubble's content; a video plays on the phone turned on its side when
// the player opens it.
//
// Time shows small beside the bubble, Read marks the owner's message as read,
// and Day puts a day's separator over the message (a message of only Day is
// the separator alone). All are interpolated.
type PhoneMessage struct {
	Time    string      `yaml:"time"`
	Read    bool        `yaml:"read"`
	Day     string      `yaml:"day"`
	From    string      `yaml:"from"`
	Icon    string      `yaml:"icon"`
	Mine    bool        `yaml:"mine"`
	Text    string      `yaml:"text"`
	Picture string      `yaml:"picture"`
	Video   *PhoneVideo `yaml:"video" json:",omitempty"`
}

// PhonePost is one post of a photo feed: who posted it (interpolated) with their
// round Icon, the Picture under assets/ or a Video playing in its place, its
// caption Text and the number of Likes.
//
// A post with only Text is a card of text, as a timeline shows it; Replies and
// Reposts are counted next to its likes.
type PhonePost struct {
	Who     string      `yaml:"who"`
	Icon    string      `yaml:"icon"`
	Picture string      `yaml:"picture"`
	Video   *PhoneVideo `yaml:"video" json:",omitempty"`
	Text    string      `yaml:"text"`
	Likes   int         `yaml:"likes"`
	Time    string      `yaml:"time"`
	Replies int         `yaml:"replies"`
	Reposts int         `yaml:"reposts"`
}

// PhoneVideo is a clip: its Picture under assets/ fills the frame (pushed in
// slowly, so a still plays as a clip), or Frames, pictures under assets/ (a
// pattern such as clips/battle/*.jpg stands for the files it matches, sorted),
// play as footage at FPS frames a second (0 is 12) over the Picture, which is
// then its poster. Fade cross-fades each frame into the next over its last
// seconds, and Pan pushes in slowly on each frame, so a few pictures at a low
// FPS play smoothly. Puppet, a puppet file under assets/, stands in front and
// moves as it does on the stage: breathing, swaying and, at each second in
// Bounce, a jiggle. Zoom is how many times the frame's height the puppet is
// drawn (0 is 1.5: its upper body). Text is the title (interpolated) and
// Seconds its length on the seek bar (0 is the frames' length, or 30).
type PhoneVideo struct {
	Picture string    `yaml:"picture"`
	Frames  []string  `yaml:"frames" json:",omitempty"`
	FPS     float64   `yaml:"fps"`
	Fade    float64   `yaml:"fade"`
	Pan     bool      `yaml:"pan"`
	Puppet  string    `yaml:"puppet"`
	Zoom    float64   `yaml:"zoom"`
	Bounce  []float64 `yaml:"bounce" json:",omitempty"`
	Text    string    `yaml:"text"`
	Seconds float64   `yaml:"seconds"`
}

// FrameRate is the frames the clip shows a second (0 is 12).
func (v PhoneVideo) FrameRate() float64 {
	if v.FPS > 0 {
		return v.FPS
	}
	return 12
}

// Length is the clip's length in seconds when it has frames pictures: Seconds,
// else the frames' length, else 30.
func (v PhoneVideo) Length(frames int) float64 {
	switch {
	case v.Seconds > 0:
		return v.Seconds
	case frames > 0:
		return float64(frames) / v.FrameRate()
	}
	return 30
}

// FramePictures are the pictures of Frames, the patterns matched in fsys under
// assets/ as an image's files are (CG.Pages). With no fsys, Frames as written.
func (v PhoneVideo) FramePictures(fsys fs.FS) []string {
	if len(v.Frames) == 0 || fsys == nil {
		return v.Frames
	}
	return CG{Files: v.Frames}.Pages(fsys)
}

// CallViewCue is a presentation-only change of the call on screen.
type CallViewCue struct {
	Camera *CallCamera `yaml:"camera" json:",omitempty"`
	Share  *CallShare  `yaml:"share" json:",omitempty"`
}

// CallShare puts a shared screen (a picture under assets/) on the call's stage in
// place of the speaker's view until the call's lines end; Who is the participant
// who shares, and an empty Picture ends the sharing.
type CallShare struct {
	Who     string `yaml:"who"`
	Picture string `yaml:"picture"`
}

// CallCamera turns one participant's camera view on or off until the call's
// lines end; Who is a planning entity or an actor.
type CallCamera struct {
	Who string `yaml:"who"`
	On  bool   `yaml:"on"`
}

type Respond struct {
	Who  string `yaml:"who"`
	Text string `yaml:"text"`
	From string `yaml:"from"`
}
