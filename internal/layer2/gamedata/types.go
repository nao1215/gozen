// Package gamedata loads the game's YAML into the DB and checks structure, references and expressions before the game starts.
// Pass Load an fs.FS holding game.yaml, the YAML under data/ and the assets under assets/.
// See the Schema page (https://nao1215.github.io/gozen/schema/) for all fields, data/schema/ for the machine-readable schema, and docs/architecture/core-api.md for the candidate public entry points.
// The editor schema alone does not guarantee references or expressions, so Load's semantic checks are also needed.
package gamedata

import (
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"github.com/nao1215/gozen/internal/expr"
)

// Expr is an expression of the expr language, parsed when the YAML is read. Its zero
// value is "no expression": a condition that holds and a formula that is 0.
type Expr struct {
	*expr.Expr
}

// UnmarshalYAML parses the scalar as an expression, reporting the line on an error.
func (e *Expr) UnmarshalYAML(n ast.Node) error {
	src, err := scalarText(n)
	if err != nil {
		return err
	}
	p, err := expr.Parse(src)
	if err != nil {
		return &yaml.SyntaxError{Message: err.Error(), Token: n.GetToken()}
	}
	e.Expr = p
	return nil
}

// Set reports whether an expression was given.
func (e Expr) Set() bool { return e.Expr != nil }

// MarshalText is the expression as written ("" when none was given).
func (e Expr) MarshalText() ([]byte, error) {
	if e.Expr == nil {
		return []byte{}, nil
	}
	return e.Expr.MarshalText()
}

func scalarText(n ast.Node) (string, error) {
	switch x := n.(type) {
	case *ast.StringNode:
		return x.Value, nil
	case *ast.LiteralNode:
		return x.Value.Value, nil
	case *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode, *ast.NullNode:
		return n.GetToken().Value, nil
	}
	return "", fmt.Errorf("line %d: expected an expression (a string), found %s", n.GetToken().Position.Line, n.Type())
}

// Color is a color written "#rrggbb" or "#rrggbbaa".
type Color struct {
	color.NRGBA
	Valid bool
}

// UnmarshalYAML reads a color written as a string straight from its node. The YAML
// library hands a TextUnmarshaler the node written back out as YAML and read again, which
// was a tenth of loading a game with many colors; any other node still goes that way.
func (c *Color) UnmarshalYAML(n ast.Node) error {
	if x, ok := n.(*ast.StringNode); ok {
		return c.UnmarshalText([]byte(x.Value))
	}
	return yaml.NodeToValue(n, (*textColor)(c), yaml.Strict())
}

// textColor is a Color read only through UnmarshalText (the library's own way).
type textColor Color

func (c *textColor) UnmarshalText(b []byte) error { return (*Color)(c).UnmarshalText(b) }

// UnmarshalText parses the color.
func (c *Color) UnmarshalText(b []byte) error {
	s := strings.TrimPrefix(string(b), "#")
	if len(s) != 6 && len(s) != 8 {
		return fmt.Errorf("color %q: want #rrggbb or #rrggbbaa", string(b))
	}
	if len(s) == 6 {
		s += "ff"
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return fmt.Errorf("color %q: %w", string(b), err)
	}
	c.NRGBA = color.NRGBA{uint8((v >> 24) & 255), uint8((v >> 16) & 255), uint8((v >> 8) & 255), uint8(v & 255)}
	c.Valid = true
	return nil
}

// Or is c if it was given, else def.
func (c Color) Or(def color.NRGBA) color.NRGBA {
	if c.Valid {
		return c.NRGBA
	}
	return def
}

// StatNames are the stats every battler has, in the order the menus show them.
var StatNames = []string{"mhp", "mmp", "atk", "def", "mat", "mdf", "agi", "luk"}

// StatLabels are the names the menus show for the stats (put through lang.T where they are drawn).
var StatLabels = map[string]string{
	"mhp": "最大HP", "mmp": "最大MP", "atk": "攻撃力", "def": "防御力",
	"mat": "魔法力", "mdf": "魔法防御", "agi": "素早さ", "luk": "運",
}

// Stats are numbers by stat name (mhp, mmp, atk, def, mat, mdf, agi, luk).
type Stats map[string]int

// StatFormulas are a formula by stat name, in terms of level (a class's growth).
type StatFormulas map[string]Expr

// StatRates are multipliers by stat name (a state's effect on the stats).
type StatRates map[string]float64

// ---- game.yaml

// Game is game.yaml: the game's title, screen and starting state.
type Game struct {
	State StateNames `yaml:"state"`
	// SchemaVersion is the YAML spec version. 0 (omitted) means the old spec.
	SchemaVersion int `yaml:"schema_version,omitempty"`
	// DataVersion is the version of the game's content. 0 (omitted) means unmanaged. It is separate from the save format version.
	DataVersion int `yaml:"data_version,omitempty"`
	// Language picks the engine's built-in words for the screens: "en" (English, the
	// default when empty) or "ja" (Japanese). The host passes it to text.Use.
	Language string `yaml:"language"`
	// Text changes the engine's own words on the screens (text), on top of Language: the
	// engine's key, its Japanese source text (a format with its verbs too) -> the game's.
	Text map[string]string `yaml:"text"`
	// WrittenIn is the language the files under data/ are written in (default: Language);
	// lang/<code>.yaml translates them into the others (schema_version 4, ADR 0027).
	WrittenIn string `yaml:"written_in"`
	// Languages are the languages the player may choose in the settings (default:
	// Language alone; schema_version 4).
	Languages []string `yaml:"languages"`
	ID        string   `yaml:"id"` // names the save directory
	Title     string   `yaml:"title"`
	Subtitle  string   `yaml:"subtitle"`
	Screen    Screen   `yaml:"screen"`
	Fonts     Fonts    `yaml:"fonts"`
	TitleBG   Picture  `yaml:"title_background"`
	TitleArt  Picture  `yaml:"title_art"`
	// TitleFigures stand on the title screen in their order (the later ones in front); with
	// them, title_art is not drawn.
	TitleFigures []TitleFigure `yaml:"title_figures"`
	// TitleText letters the title and the subtitle on the title screen (schema_version 3).
	TitleText TitleText `yaml:"title_text"`
	TitleBGM  MusicCue  `yaml:"title_music"`
	// Gallery is what the title's gallery shows: the event pictures seen and the scenes
	// whose flag was set, in any play (kept beside the saves).
	Gallery  Gallery `yaml:"gallery"`
	Start    Start   `yaml:"start"`
	Slots    []Slot  `yaml:"equip_slots"`
	MaxParty int     `yaml:"max_party"`
	// JoinExp is an expression for the lower bound of experience when a member joins (the event's join), evaluated on the party before joining (party.avg_exp and so on).
	// If the joining member has less experience than that, it is raised to that value (never lowered).
	JoinExp  Expr      `yaml:"join_exp"`
	Battle   BattleCfg `yaml:"battle"`
	Terms    Terms     `yaml:"terms"`
	GameOver GameOver  `yaml:"game_over"`
	UI       UIStyle   `yaml:"ui"`
	// Meters are gauges of the player's numbers that the field, the dungeon and the status
	// menu show (a need that grows, a count toward something).
	Meters []Meter `yaml:"meters"`
	// CardBubbles are small particles (such as poison bubbles) that well up from party cards (the status frames in dungeon, town and battle).
	CardBubbles []CardBubble `yaml:"card_bubbles"`
	// Memo is the test player's notes (play memo.go): a key stops the game and keeps the
	// screen, the place and a word or two, to read later as the author's own judgment of the play.
	Memo MemoCfg `yaml:"memo"`
	// StandIns is the file listing placeholder CGs (under assets/; one per line, as a name relative to assets/).
	// A CG whose art is not drawn yet and which has a stand-in background has no speaker in the picture, so it does not hide the portraits (same as faces: true).
	// Once the finished art is added and removed from the list, it hides them as before.
	StandIns string `yaml:"stand_ins"`
	// CGReview is the set of tags attached in the CG skim (a debug mode when the whole gallery is opened; play cgreview.go)
	// and the groups of pictures that can be chosen for inclusion.
	CGReview CGReviewCfg `yaml:"cg_review"`
	// SceneReview is the set of tags attached to recollections in the recollection check page (a debug mode when the whole gallery is opened; play scenecheck.go).
	SceneReview SceneReviewCfg `yaml:"scene_review"`
	// Asides are events inserted after the lines of fixed speakers (a pause to brood, a picture in the mind, and so on).
	Asides []Aside `yaml:"asides"`
	// Verbs are the names of custom commands implemented in Go (spec version 2). Declared names can be written as commands, and the check does not reject them as unknown commands.
	// The Go implementation hook is added at a later stage. Until then, it records "no implementation" at run time and skips.
	Verbs []string `yaml:"verbs"`
	// Screens are the names of custom screens made in Go (spec version 2). A declared name can be opened with open.
	Screens []string `yaml:"screens"`
}

// Aside inserts the Call event right after a line by a speaker (any of actor, face and name; face may be a pattern) is shown, if When holds
// (and returns to the original conversation when it ends). It does not insert while a CG is shown (a picture that hides the portraits), during battle,
// in Skip events (name patterns, including events called from them), or inside an inserted event.
// Counts, how opponents rotate and the daily cap are written in the Call event and the When expression (the engine does not count).
type Aside struct {
	Actor string   `yaml:"actor"`
	Face  string   `yaml:"face"`
	Name  string   `yaml:"name"`
	When  Expr     `yaml:"when"`
	Call  string   `yaml:"call"`
	Skip  []string `yaml:"skip"`
}

// MemoCfg is what a test player's note asks and keeps: the kinds to choose from and the
// numbers kept with each note (an expression of the state's names each).
type MemoCfg struct {
	Kinds []MemoKind `yaml:"kinds"`
	Vars  []MemoVar  `yaml:"vars"`
}

// CGReviewCfg holds the rules of the CG skim. Tags are the tags attached to one picture (the first 10 in order are attached with the number keys 1 to 9 and 0).
// Extra is the pictures (patterns) added to the skim that are not in the gallery column, and Groups are the groups of pictures that can be chosen for inclusion before the skim
// (not included by default. An Extra picture that matches a Group follows that group's choice).
type CGReviewCfg struct {
	Tags   []CGReviewTag   `yaml:"tags"`
	Extra  []string        `yaml:"extra"`
	Groups []CGReviewGroup `yaml:"groups"`
}

// Configured reports whether the authors have configured the CG check.
func (c CGReviewCfg) Configured() bool {
	return len(c.Tags)+len(c.Extra)+len(c.Groups) > 0
}

// CGReviewTag is a tag: the id kept in the record, the displayed name, and whether it is an OK tag (pictures with the OK tag are dropped by "exclude OK").
type CGReviewTag struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	OK    bool   `yaml:"ok"`
}

// SceneReviewCfg holds the recollection tags (one OK tag. If absent, only OK and NG).
type SceneReviewCfg struct {
	// Title is the name of the check column. If omitted, the translatable "回想検査".
	Title string        `yaml:"title"`
	Tags  []CGReviewTag `yaml:"tags"`
	// Groups is the order of headings (headings not listed come after, in order of appearance).
	Groups []string `yaml:"groups"`
}

// Configured reports whether the authors have configured the recollection check.
// A configuration with only the old tags and headings is also valid; with nothing set, no column is created.
func (c SceneReviewCfg) Configured() bool {
	return c.Title != "" || len(c.Tags)+len(c.Groups) > 0
}

// CGReviewGroup is a group of pictures that can be chosen for inclusion (a name and picture patterns).
type CGReviewGroup struct {
	Label string   `yaml:"label"`
	Files []string `yaml:"files"`
}

// MemoKind is a kind of note: its id (kept in the file) and the label shown.
type MemoKind struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// MemoVar is a number kept with each note.
type MemoVar struct {
	Name  string `yaml:"name"`
	Value Expr   `yaml:"value"`
}

// Meter is a gauge of a number: value out of max, shown while when holds. In the status
// menu it is shown on the page of actor (or on every page when actor is empty). At max the
// bar takes full_color and full is written after it.
type Meter struct {
	Label string  `yaml:"label"`
	Value Expr    `yaml:"value"`
	Max   float64 `yaml:"max"` // default 100
	// MaxValue is an expression with which the game author specifies a state-dependent cap. When evaluation succeeds it takes priority over
	// Max and is rounded to at least 1. When unspecified or evaluation fails, Max is used.
	// When specified, current/cap is shown (Full at a full gauge still takes priority).
	MaxValue  Expr   `yaml:"max_value"`
	When      Expr   `yaml:"when"`
	Color     Color  `yaml:"color"`
	Full      string `yaml:"full"`
	FullColor Color  `yaml:"full_color"`
	Actor     string `yaml:"actor"`
	// Marks are values marked on the bar (where something becomes possible).
	Marks []float64 `yaml:"marks"`
}

// CardBubble is a rule for particles welling up from a card. For cards of Actor (anyone if empty), tied to State
// (any if empty), while When holds (always if empty), Count particles rise from the card's top edge by Rise
// at Speed (pixels/second), fading out. If several rules apply, all are drawn.
type CardBubble struct {
	Actor string  `yaml:"actor"`
	State string  `yaml:"state"`
	When  Expr    `yaml:"when"`
	Color Color   `yaml:"color"` // default white
	Size  float64 `yaml:"size"`  // particle radius (default 3)
	Count int     `yaml:"count"` // number shown at once (default 4, max 24)
	Speed float64 `yaml:"speed"` // rising speed (default 30)
	Rise  float64 `yaml:"rise"`  // rising height (default 50)
}

// Screen is the logical screen size.
type Screen struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

// Fonts are the font files (paths under assets/).
type Fonts struct {
	Regular string `yaml:"regular"`
	Pop     string `yaml:"pop"`
	// Title is the typeface of the title screen's title and subtitle (none: the regular one).
	Title string `yaml:"title"`
	// Subtitle is the typeface of the title screen's subtitle (none: the title's).
	Subtitle string `yaml:"subtitle"`
}

// TitleText is how the title screen letters the title and the subtitle. Sizes are in the
// screen's pixels; tracking is the space between letters as a part of the size.
type TitleText struct {
	Size             float64  `yaml:"size"`              // 0: 96
	Tracking         *float64 `yaml:"tracking"`          // nil: 0.12
	SubtitleSize     float64  `yaml:"subtitle_size"`     // 0: 30
	SubtitleTracking *float64 `yaml:"subtitle_tracking"` // nil: 0.3
	// Color fills the letters and Edge outlines them (none: the ui's text and window colors).
	Color Color `yaml:"color"`
	Edge  Color `yaml:"edge"`
}

// TitleFigure is one picture standing on the title screen. X is its center and Bottom its
// feet (parts of the screen's width and height); Height is a part of the screen's height.
type TitleFigure struct {
	File   string  `yaml:"file"`
	X      float64 `yaml:"x"`
	Bottom float64 `yaml:"bottom"` // 0: 1 (the bottom of the screen)
	Height float64 `yaml:"height"` // 0: 0.95
}

// Start is the state a new game begins in.
type Start struct {
	Location string         `yaml:"location"`
	Party    []string       `yaml:"party"`
	Gold     int            `yaml:"gold"`
	Items    map[string]int `yaml:"items"`
	Event    string         `yaml:"event"`
}

// Slot is an equipment slot.
type Slot struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Manual is the slot that the menu's "さいきょうそうび" and strong-equipment recommendations do not touch (a slot for things
	// that cannot be compared by total numbers, such as those that work through combinations).
	Manual bool `yaml:"manual"`
}

// Passive is a state applied to the actor at the start of battle (when its when holds).
type Passive struct {
	Actor string `yaml:"actor"`
	State string `yaml:"state"`
	When  Expr   `yaml:"when"`
	// Level is the tier shown on the skill tree screen (an expression; not shown if omitted). Before it opens (when does not hold), the name is hidden too.
	Level Expr `yaml:"level"`
}

// BattleCfg are the rules every battle shares.
type BattleCfg struct {
	// Escape is the chance the party gets away (party, foes in scope).
	Escape Expr `yaml:"escape"`
	// Critical is the chance of a critical hit for a skill without its own (a, b).
	Critical Expr `yaml:"critical"`
	// CriticalRate multiplies a critical hit's damage.
	CriticalRate float64 `yaml:"critical_rate"`
	// Variance is the default spread of damage and healing (0.1 = ±10%).
	Variance float64 `yaml:"variance"`
	// Speed is the order key of an action (a; higher acts first).
	Speed Expr `yaml:"speed"`
	// GuardRate multiplies the damage a guarding battler takes.
	GuardRate float64 `yaml:"guard_rate"`
	// Music is the song of a battle whose troop names none (default: the song named
	// battle, if there is one).
	Music MusicCue `yaml:"music"`
	// Attack and Guard are the skills of the ATTACK and GUARD commands.
	// AI is how a battler's rules choose: "priority" (the default: the highest priority, by
	// weight) or "gambit" (the first that holds, from the top, as Final Fantasy XII's gambits).
	AI string `yaml:"ai"`
	// AutoTactics is the auto tactics per named AI. An AI not specified uses the legacy rules.
	AutoTactics map[string][]TacticRow `yaml:"auto_tactics"`
	// SimpleTactics uses editing with five lines and only conditions and actions.
	SimpleTactics bool                    `yaml:"simple_tactics"`
	TacticPresets map[string]TacticPreset `yaml:"tactic_presets"`
	// Tempo is how fast a battle's messages and moves go by (1: the default; 1.5 half as fast again).
	Tempo float64 `yaml:"tempo"`
	// FrontWeight and BackWeight are the weights with which an enemy aims at one front-row or back-row member (default 4 and 1.
	// one front-row member is aimed at 4 times as often as one back-row member). 0 means never aimed at (if all are 0, all are aimed at equally).
	FrontWeight *float64 `yaml:"front_weight"`
	BackWeight  *float64 `yaml:"back_weight"`
	// AfterWin is an event run after every battle won (a gauge to fill, a line to say).
	AfterWin string `yaml:"after_win"`
	// Before is the event ID run just before a battle request. Nothing happens if empty.
	// It is used for the game author's resource consumption and join conditions. If the party moves, the original progress is aborted,
	// and the screen's retry starts from the state after this processing.
	Before string `yaml:"before"`
	// After is the event ID run after the result is applied, at the next choice or battle, or at the end of the event.
	// It is used for the game author's resource checks and returning. If empty it is skipped,
	// and for a normal defeat with no defeat branch, the wipe-out processing takes priority.
	After string `yaml:"after"`
	// FoeScale multiplies the stats of the foes that are not bosses (mhp: 1.4, atk: 1.25…):
	// the tension of the common battles, tuned in one place.
	FoeScale map[string]float64 `yaml:"foe_scale"`
	// FoeScaleWhen, when set, is when foe_scale holds (var.floor >= 3: not the very first floors).
	FoeScaleWhen Expr `yaml:"foe_scale_when"`
	// Passives are states applied to party members at the start of battle (while when holds; hidden abilities and so on). They do not remain after the battle.
	Passives []Passive `yaml:"passives"`
	Attack   string    `yaml:"attack_skill"`
	Guard    string    `yaml:"guard_skill"`
	// Retry asks "redo from before fighting" on a defeat (restore the state just before the battle and fight the same battle again).
	// If declined, it goes to on_lose or game_over as before. It can be turned off with retry: false on a battle command.
	Retry bool `yaml:"retry"`
	// BossWinMusic is the song for winning a battle with a guardian (a Boss enemy) (played once and stopped). The enemy group's
	// win_music takes precedence. If neither exists, the victory mood of the battle music (music rules) is used as before.
	// While playing it continues into the post-win event, and the place's music returns after it finishes.
	BossWinMusic MusicCue `yaml:"boss_win_music"`
	// AutoStart advances battles on auto from the start (the default when the player has not decided the setting "start battles on auto").
	// Releasing it with X makes that battle manually chosen.
	AutoStart bool `yaml:"auto_start"`
	// AutoTempo is the speed multiplier for text and waits during auto (default 1; 1.5 is 1.5 times faster).
	// Conversation inside battle also advances without waiting for confirmation during auto (waits are shortened by this multiplier).
	AutoTempo float64 `yaml:"auto_tempo"`
}

// Terms are the words the menus use.
type Terms struct {
	Currency string `yaml:"currency"`
	Level    string `yaml:"level"`
	HP       string `yaml:"hp"`
	MP       string `yaml:"mp"`
	Exp      string `yaml:"exp"`
}

// GameOver is what is shown when the party falls.
type GameOver struct {
	// Event, when given, is run instead of ending the game: the fallen party carried back
	// (healed) and whatever the loss costs (a roguelike's way).
	Event      string   `yaml:"event"`
	Background Picture  `yaml:"background"`
	Text       string   `yaml:"text"`
	Music      MusicCue `yaml:"music"`
}

// UIStyle are the colors of the windows.
type UIStyle struct {
	// Message is the message window: "box" (a framed box, the default) or "novel" (as a
	// visual novel's: across the bottom of the screen, the speaker's picture large behind
	// it, and buttons for the backlog, auto and hiding the window).
	Message string `yaml:"message"`
	// Balloon is the text height of the balloons over the people of a painted scene, as a
	// fraction of the screen's height (0.037 when not given; schema_version 3). The face
	// beside it is three times as tall.
	Balloon float64 `yaml:"balloon"`
	// TalkPortrait places the speaker's standing picture beside the "box" message window
	// (schema_version 3). Empty, it stands as tall as the screen at the box's left end.
	TalkPortrait TalkPortrait `yaml:"talk_portrait"`
	// Backlog is how many lines the backlog keeps (default 300).
	Backlog int `yaml:"backlog"`
	// Rollback is how many earlier events the player can roll back into, besides the one
	// running (default 8; 0 turns rolling back off). Needs schema version 4 (ADR 0025).
	Rollback *int `yaml:"rollback"`
	// Profiles puts Characters, the character book, in the title, the field menu and the
	// novel bar (ADR 0031). Under --dev the menus offer it anyway.
	Profiles bool `yaml:"profiles"`
	// Voice shapes how voices play: cut at the next line, auto waits for them
	// (schema_version 4, ADR 0028).
	Voice   VoiceStyle `yaml:"voice"`
	Window  Color      `yaml:"window"`
	Border  Color      `yaml:"border"`
	Text    Color      `yaml:"text"`
	Accent  Color      `yaml:"accent"`
	Cursor  Color      `yaml:"cursor"`
	Shadow  Color      `yaml:"shadow"`
	NameTag Color      `yaml:"name_tag"`
	// StageScale is the size multiplier per portrait (picture file to multiplier; default 1). The head position stays the same and it is drawn smaller
	// (to pull back pictures where the body fills the picture and looks bigger than the others).
	StageScale map[string]float64 `yaml:"stage_scale"`
	// StageDrop is the amount to lower per portrait (picture file to a ratio of the screen height; default 0).
	// All pictures are drawn with the top of the head aligned, so short characters or those with hair piled high look as tall as tall ones. Lowering fixes that.
	StageDrop    map[string]float64 `yaml:"stage_drop"`
	FieldNotices *bool              `yaml:"field_notices"`
	// BattleFx is the strength of battle effects (play's battle_drama.go).
	BattleFx BattleFx `yaml:"battle_fx"`
	// SfxVary is the variation when the same sound effect plays repeatedly in a short time (sound's vary.go).
	// pitch is the pitch (default 0.03 for ±3%) and volume is the volume (default 0.1 for ±10%). 0 turns it off.
	SfxVary SfxVary `yaml:"sfx_vary"`
}

// TalkPortrait is where the speaker's standing picture (or moving portrait) stands beside
// the "box" message window, in fractions of the screen; 0 keeps a field's default. A
// height above 1 brings the speaker closer: with the bottom below the screen, only the
// upper body shows above the window. The speaker on the right stands mirrored at 1-x.
type TalkPortrait struct {
	// X is the picture's center as a fraction of the screen's width (0: 230 pixels right of the window's left end).
	X float64 `yaml:"x"`
	// Bottom is where the picture's bottom edge stands, as a fraction of the screen's height (0: 120 pixels below the screen).
	Bottom float64 `yaml:"bottom"`
	// Height is the picture's height as a fraction of the screen's height (0: 1.02).
	Height float64 `yaml:"height"`
}

// SfxVary is the variation in pitch and volume of sound effects played in succession (defaults if not written).
type SfxVary struct {
	Pitch  *float64 `yaml:"pitch"`
	Volume *float64 `yaml:"volume"`
}

// Values are the sizes of the variation (items not written use the defaults 0.03 and 0.1).
func (v SfxVary) Values() (pitch, volume float64) {
	pitch, volume = 0.03, 0.1
	if v.Pitch != nil {
		pitch = *v.Pitch
	}
	if v.Volume != nil {
		volume = *v.Volume
	}
	return pitch, volume
}

// BattleFx is the multiplier for the strength of battle effects. 1 if not written, and 0 turns that effect off.
// Light is kept short and restrained; the flashiness comes from shake, stops, number bounce and pauses.
type BattleFx struct {
	Shake   *float64 `yaml:"shake"`   // screen shake on critical, weakness and finishing blows
	Flash   *float64 `yaml:"flash"`   // short white flash on critical and weakness
	Pop     *float64 `yaml:"pop"`     // size and bounce of damage numbers
	Hitstop *float64 `yaml:"hitstop"` // how long the screen stops on a critical or weakness hit
	Charge  *float64 `yaml:"charge"`  // length of the charge before a guardian's big move (cut-in, all targets)
	Finish  *float64 `yaml:"finish"`  // stop and slow motion of the finishing blow, and the length of the display for defeating a guardian
	// Auto is a further multiplier applied on top of the above multipliers while fighting on auto (so the player is not kept waiting).
	Auto BattleFxAuto `yaml:"auto"`
}

// BattleFxAuto is the multiplier for stops, charges and slow motion during auto (shake and number bounce are unchanged).
// If not written, hitstop, charge and finish are 0 (none) and win is 0.4.
type BattleFxAuto struct {
	Hitstop *float64 `yaml:"hitstop"` // stop on critical and weakness
	Charge  *float64 `yaml:"charge"`  // charge before a guardian's big move
	Finish  *float64 `yaml:"finish"`  // stop and slow motion of the finishing blow
	Win     *float64 `yaml:"win"`     // time during which the guardian-defeat band cannot be skipped with confirm
}

// Get returns the multiplier (def if nil, 0 if negative).
func (BattleFxAuto) Get(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return max(0, *p)
}

// Get returns the multiplier (1 if nil, 0 if negative).
func (BattleFx) Get(p *float64) float64 {
	if p == nil {
		return 1
	}
	return max(0, *p)
}

// Picture is a picture under assets/, optionally tinted (multiplied by a color) and
// darkened. Written as a path, or as a map.
type Picture struct {
	numbersResolved bool    // effective value of spec version 1. The Go zero value is treated as the legacy omission.
	File            string  `yaml:"file"`
	Tint            Color   `yaml:"tint"`
	Bright          float64 `yaml:"brightness"` // omitted means 1. 0 is 1 under the old spec and black under the new spec.
	Blur            bool    `yaml:"blur"`
}

// UnmarshalYAML reads a path or a map.
func (p *Picture) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		p.File = s
		return nil
	}
	type plain Picture
	return unmarshal((*plain)(p))
}

// CG is an event's picture over the whole screen (the image command). Written as a path
// ("" hides the picture shown), or as a map:
//
//	image: {file: cg/x.jpg, fit: cover, tint: "#ffd8e0", brightness: 0.9, fade: 1, faces: true}
//
// files shows several pictures one after another, a tap between them (the last stays
// shown for what follows). A file may be a pattern (cg/night_*.jpg: the files it matches,
// in name order; it may match none), so a scene takes more pictures without its events
// written again.
type CG struct {
	fadeGiven       bool     // an omitted value on clear inherits the seconds of the picture being shown.
	numbersResolved bool     // an explicit 0 is also kept by the consumer.
	File            string   `yaml:"file"`
	Files           []string `yaml:"files"`
	Fit             string   `yaml:"fit"` // contain (default: all of it, on a blurred copy) or cover (fills the screen)
	Tint            Color    `yaml:"tint"`
	Bright          float64  `yaml:"brightness"` // omitted means 1. 0 is 1 under the old spec and black under the new spec.
	Fade            float64  `yaml:"fade"`       // seconds. Omitted means 0.25. 0 is 0.25 under the old spec and immediate under the new spec.
	Faces           bool     `yaml:"faces"`      // keep the speakers' pictures over it (hidden by default)
	Page            int      `yaml:"page"`       // with files: show only this one of the pages (1 is the first; past the last, the last)
	// Over is a picture standing in the middle of it, its feet above the message window
	// (the monster met in a dungeon, its battle picture over the place's background, shown
	// while the party talks before the battle). OverSize is its height, times the screen's
	// (default 0.55).
	Over     Picture `yaml:"over"`
	OverSize float64 `yaml:"over_size"`
	// Pick chooses which of files to show by an expression (the value modulo the number of files, plus 1; shows a different picture each time).
	Pick Expr `yaml:"pick"`
	// Optional means files need not match anything yet (it shows once drawn; shows nothing while unmatched).
	// Use it for slots where reuse is to be avoided and no substitute picture is placed.
	Optional bool `yaml:"optional"`
	// Look is the way of showing. "dream" is a picture in the mind (remembering, imagining): it blurs the edge, overlays a haze of the Haze color from the edge,
	// and slowly zooms in while shown. Without Haze, a pale pink.
	Look string `yaml:"look"`
	Haze Color  `yaml:"haze"`
	// Auto is the seconds per picture when advancing through files in order. When written, it advances to the next picture without a press (a press advances at once).
	// A slideshow showing only pictures (no conversation window). With 0 it waits for a press.
	Auto float64 `yaml:"auto"`
}

// PageBase is the picture a page belongs to: the name without a ".NNN" before the
// extension (cg/x.002.jpg and cg/x.jpg are pages of cg/x.jpg).
func PageBase(file string) string {
	ext := path.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	if i := strings.LastIndexByte(stem, '.'); i >= 0 {
		if n := stem[i+1:]; n != "" && strings.Trim(n, "0123456789") == "" {
			return stem[:i] + ext
		}
	}
	return file
}

// UnmarshalYAML reads a path or a map.
func (c *CG) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*c = CG{File: s}
		return nil
	}
	type plain CG
	return unmarshal((*plain)(c))
}

// Picture is the CG as a picture.
func (c CG) Picture() Picture {
	return Picture{File: c.File, Tint: c.Tint, Bright: c.Bright, numbersResolved: c.numbersResolved}
}

// Same reports whether o is the same picture shown the same way.
func (c CG) Same(o CG) bool {
	return c.File == o.File && slices.Equal(c.Files, o.Files) && c.Fit == o.Fit && c.Tint == o.Tint &&
		c.numbersResolved == o.numbersResolved && c.Bright == o.Bright && c.Fade == o.Fade && c.Faces == o.Faces && c.Over == o.Over && c.OverSize == o.OverSize &&
		c.Pick.Expr == o.Pick.Expr && c.Look == o.Look && c.Haze == o.Haze
}

// Pages are the pictures of files, the patterns matched in fsys under assets/ (a pattern
// matching nothing is left out). With no files, the one file.
func (c CG) Pages(fsys fs.FS) []string {
	if len(c.Files) == 0 {
		return []string{c.File}
	}
	var out []string
	for _, f := range c.Files {
		if !strings.ContainsAny(f, "*?[") {
			out = append(out, f)
			continue
		}
		m, err := fs.Glob(fsys, "assets/"+f)
		if err != nil {
			continue
		}
		sort.Strings(m)
		for _, p := range m {
			out = append(out, strings.TrimPrefix(p, "assets/"))
		}
	}
	return out
}

// CountPages is how many pictures name has under assets/ in fsys: with a pattern
// ("cg/x.*.jpg") the files it matches, otherwise name.* (cg/x → cg/x.jpg, cg/x.002.jpg …).
// It is pages('…') in the expressions.
func CountPages(fsys fs.FS, name string) int {
	if fsys == nil || name == "" {
		return 0
	}
	if !strings.ContainsAny(name, "*?[") {
		name += ".*"
	}
	m, err := fs.Glob(fsys, "assets/"+name)
	if err != nil {
		return 0
	}
	return len(m)
}

// HasFade reports whether seconds were stated explicitly on the clear side, including an explicit 0 in the new version.
func (c CG) HasFade() bool { return c.fadeGiven || (!c.numbersResolved && c.Fade > 0) }

// FadeTicks is how many ticks (60 a second) the picture fades in or out over.
func (c CG) FadeTicks() int {
	if !c.numbersResolved && c.Fade <= 0 {
		return 15
	}
	if c.Fade == 0 {
		return 0
	}
	return max(1, int(c.Fade*60))
}

// MusicCue is a song and the mood it starts in ("song" or "song/mood", or a map).
type MusicCue struct {
	Song string `yaml:"song"`
	Mood string `yaml:"mood"`
	// Fade is the seconds the music command fades in a song, or out with Stop
	// (schema_version 4, ADR 0028).
	Fade float64 `yaml:"fade"`
	// Stop stops the music (the music command only).
	Stop bool `yaml:"stop"`
}

// UnmarshalYAML reads "song", "song/mood" or a map.
func (m *MusicCue) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		m.Song, m.Mood, _ = strings.Cut(s, "/")
		return nil
	}
	type plain MusicCue
	return unmarshal((*plain)(m))
}

// ---- the sections of the data files

// Class is how a kind of actor grows: stats by level, the skills learned and the
// equipment allowed.
type Class struct {
	Name string `yaml:"name"`
	// Tree is the skill tree: skills (and passive nodes) raised with SP, each node opening
	// once the nodes it requires are raised far enough.
	Tree []TreeNode `yaml:"tree"`
	// SP is the skill points given at each level gained (default 1); StartSP those at level 1.
	SP      int          `yaml:"sp"`
	StartSP int          `yaml:"start_sp"`
	Stats   StatFormulas `yaml:"stats"`
	ExpFor  Expr         `yaml:"exp_for"` // the total exp needed to reach level (level)
	Skills  []Learning   `yaml:"skills"`
	Equip   []string     `yaml:"equip"` // equipment kinds allowed
	AI      AIRef        `yaml:"ai"`    // the rules of AUTO
	Max     int          `yaml:"max_level"`
	// SPPitch is the one-liner when sp_suggest recommends an allocation (a reason matched to this class's role).
	SPPitch string `yaml:"sp_pitch"`
}

// TreeNode is a node of a skill tree: a skill (its level is the SP put in it) or a passive
// bonus (stats a level).
type TreeNode struct {
	ID       string    `yaml:"id"`
	Name     string    `yaml:"name"`  // default the skill's
	Desc     string    `yaml:"desc"`  // default the skill's
	Skill    string    `yaml:"skill"` // the skill learned at level 1 (none: a passive node)
	Stats    Stats     `yaml:"stats"` // added a level
	Max      int       `yaml:"max"`   // the highest level (default 10)
	Requires []Require `yaml:"requires"`
	Col      int       `yaml:"col"` // where it is drawn: the column (tier) and the row
	Row      int       `yaml:"row"`
}

// Require is a node that must be raised to a level before another opens.
type Require struct {
	Node  string `yaml:"node"`
	Level int    `yaml:"level"` // default 1
}

// StoneSet is a combination of magic stones worn together by one person: it grants a skill
// and stats while all its stones are worn.
type StoneSet struct {
	// Family is a family ID independent of the display name. If omitted, the same name is treated as one family as before.
	Family string   `yaml:"family"`
	Name   string   `yaml:"name"`
	Desc   string   `yaml:"desc"`
	Stones []string `yaml:"stones"`
	Skill  string   `yaml:"skill"`
	Stats  Stats    `yaml:"stats"`
	// For is the party member this combination suits (starting from the first; the default wearer for stone_suggest).
	For []string `yaml:"for"`
	// Only holds just when the for member wears it (nothing happens if someone else wears it).
	Only bool `yaml:"only"`
	// Pitch is the one-liner when the stone shop recommends it (it is enough to write it on the first of a family's sequence).
	Pitch string `yaml:"pitch"`
}

// Fusion makes a stone of two (at the stone shop).
type Fusion struct {
	From  []string `yaml:"from"`
	To    string   `yaml:"to"`
	Price int      `yaml:"price"`
}

// Learning is a skill learned at a level.
type Learning struct {
	Level int    `yaml:"level"`
	Skill string `yaml:"skill"`
}

// Actor is a character who can join the party.
type Actor struct {
	Name    string            `yaml:"name"`
	Class   string            `yaml:"class"`
	Level   int               `yaml:"level"`
	Faces   string            `yaml:"faces"` // a directory under assets/ holding <face>.png
	Face    string            `yaml:"face"`  // the face shown by default
	Equip   map[string]string `yaml:"equip"` // slot -> item
	Profile string            `yaml:"profile"`
	Back    bool              `yaml:"back"` // starts in the back row
	Color   Color             `yaml:"color"`
	AI      AIRef             `yaml:"ai"`
	Cutin   string            `yaml:"cutin"` // the picture of the skills' cut-in (default the standing picture)
	// CutinLines are lines shouted in a cut-in, one picked in turn.
	CutinLines []string `yaml:"cutin_lines"`
	// CutinBox is the face range of the cut-in picture (Cutin): the center (ratio of width and height) and the size (from the top of the head
	// to the chin, as a ratio of the width). If absent, the head is searched for from the top of the picture (on slim pictures the head was read small and only the hair fell in the band).
	CutinBox []float64 `yaml:"cutin_box"`
	// FaceBox is where the face is on the standing pictures (for the face icons and the
	// cut-in): the middle (x of the width, y of the height) and the size (of the width).
	// Without it the head is found from the picture's top (which hair ornaments push up).
	FaceBox []float64 `yaml:"face_box"`
	// FaceShade darkens the face icons cut from the standing pictures (0 = as drawn, 1 =
	// black): sinks into shadow the face of a protagonist whose standing picture is not shown. Gallery and standing pictures are unchanged.
	FaceShade float64 `yaml:"face_shade"`
	// FaceShadeTop is the darkness at the top of the icon, falling evenly to FaceShade at
	// the bottom (shadow the eyes and let the mouth show faintly). If not written, same as FaceShade.
	FaceShadeTop float64 `yaml:"face_shade_top"`
	// NoStanding: the actor is not shown standing in the talks nor in the cut-ins (the
	// protagonist the player stands in for); the face icon stays.
	NoStanding bool `yaml:"no_standing"`
	// Puppet is a moving portrait under assets/ (see docs/architecture/puppet.md). The
	// talk window draws it instead of the face picture when it has a face of that name.
	Puppet   string     `yaml:"puppet"`
	FaceRule []FaceRule `yaml:"face_rules"`
	// Voices is a folder under assets/ holding the actor's voices, named by say.voice
	// (schema_version 4, ADR 0028).
	Voices string `yaml:"voices"`
	// Reactions are faces shown for a moment: on hit, act, heal, win, fall.
	Reactions map[string]string `yaml:"reactions"`
	// Tree are the nodes of the class's skill tree raised from the start (node -> level),
	// paid from the start's SP: a skill to use from the first battle.
	Tree map[string]int `yaml:"tree"`
	// Doll is how the actor looks as a small doll walking a stage when leading the party
	// (schema_version 3); the colors come from the face pictures.
	Doll *Doll `yaml:"doll"`
}

// FaceRule picks an actor's face from their state (self in scope): the first that holds.
type FaceRule struct {
	When Expr   `yaml:"when"`
	Face string `yaml:"face"`
}

// Item is a thing the party carries: a consumable, a piece of equipment, or a key item.
type Item struct {
	Name string `yaml:"name"`
	// Short is the short name (lined up like "白と蒼" in combination suggestions; name if absent).
	Short string `yaml:"short"`
	Desc  string `yaml:"desc"`
	Kind  string `yaml:"kind"` // item, key, or an equipment kind (sword, staff, ...)
	// Event is run when the item is used from the menu (used up; the menu closes): a return
	// to town, a map of the floor... (when is a condition for using it)
	Event   string `yaml:"event"`
	EventOK Expr   `yaml:"event_when"`
	// Command shows an event item as the top action of the menu instead of in the item list (visible only while its usable condition
	// event_when holds). Use it for items that are "things to do" rather than possessions.
	Command     bool               `yaml:"command"`
	Slot        string             `yaml:"slot"` // the equip slot, for equipment
	Price       int                `yaml:"price"`
	Stats       Stats              `yaml:"stats"`
	Use         *Usage             `yaml:"use"`
	Consume     *bool              `yaml:"consume"` // used up when used (default true)
	Icon        Color              `yaml:"color"`
	Element     string             `yaml:"element"` // the element of a weapon's attack
	States      []Grant            `yaml:"states"`  // states a weapon's attack may inflict
	StateResist map[string]float64 `yaml:"state_resist"`
}

// ShortName is the short name of the item (its name when it has none).
func (it *Item) ShortName() string {
	if it.Short != "" {
		return it.Short
	}
	return it.Name
}

// Consumed reports whether the item is used up.
func (it *Item) Consumed() bool { return it.Consume == nil || *it.Consume }

// Equipment reports whether the item can be equipped.
func (it *Item) Equipment() bool { return it.Slot != "" }

// Grant is a state given with a chance.
type Grant struct {
	State  string `yaml:"state"`
	Chance Expr   `yaml:"chance"`
}

// Usage is what using an item or a skill does.
type Usage struct {
	Scope   string   `yaml:"scope"` // see Scopes
	Where   []string `yaml:"where"` // battle, menu (default both for items, battle for skills)
	Effects []Effect `yaml:"effects"`
	Message string   `yaml:"message"`
	Anim    string   `yaml:"anim"` // slash, magic, heal, buff, debuff, none
	SFX     string   `yaml:"sfx"`
	Color   Color    `yaml:"color"`
	// Fx is the drawn effect on the targets (see Fxs; default: from the damage's element
	// when it is named like one, else from anim). Color tints it.
	Fx    string `yaml:"fx"`
	Hits  int    `yaml:"hits"`
	Hit   Expr   `yaml:"hit"`  // chance to hit each target (default 1)
	Crit  Expr   `yaml:"crit"` // critical chance (default battle.critical; "0" for none)
	Speed Expr   `yaml:"speed"`
	// Cutin shows the user's picture across the screen with the name before the action
	// (a big skill; a boss's special attack).
	Cutin bool `yaml:"cutin"`
	// Sacrifice: the user falls after using it (blowing itself up).
	Sacrifice bool `yaml:"sacrifice"`
}

// Fxs are the effects a skill or an item may draw on its targets (Usage.Fx): cuts, a
// thrust, a blow, the elements' bursts, a plain magic burst, healing, a state put on a
// friend (buff) or a foe (debuff), or nothing.
var Fxs = []string{"slash", "pierce", "strike", "fire", "ice", "thunder", "holy", "dark", "magic", "heal", "buff", "debuff", "none"}

// Scopes are the targets a skill or an item may have.
var Scopes = []string{"one_enemy", "all_enemies", "random_enemy", "one_ally", "all_allies", "self", "dead_ally", "all_dead_allies", "everyone", "none"}

// Skill is an action in battle (or from the menu).
type Skill struct {
	Name  string `yaml:"name"`
	Desc  string `yaml:"desc"`
	MP    int    `yaml:"mp"`
	Usage `yaml:",inline"`
}

// Effect is one thing a skill or an item does to each target.
type Effect struct {
	When     Expr     `yaml:"when"` // a, b in scope: skip the effect unless it holds
	Damage   Expr     `yaml:"damage"`
	Heal     Expr     `yaml:"heal"`
	Drain    float64  `yaml:"drain"`  // part of the damage the user gains back
	Recoil   float64  `yaml:"recoil"` // part of the damage done the user loses (never below 1 HP)
	Stat     string   `yaml:"stat"`   // hp (default) or mp
	Element  string   `yaml:"element"`
	Variance *float64 `yaml:"variance"`
	AddState string   `yaml:"add_state"`
	Chance   Expr     `yaml:"chance"`
	Turns    Expr     `yaml:"turns"`        // with add_state: how many turns it lasts (a, b in scope; the state's turns when unset)
	Remove   string   `yaml:"remove_state"` // a state id, or "bad" for every bad state
	Revive   Expr     `yaml:"revive"`       // brings a fallen ally back with this HP
	Escape   bool     `yaml:"escape"`       // the party runs away
	// Summon calls an enemy (an id of enemies) into the battle on the user's side: in the
	// place of a fallen foe, or in a new place while fewer than SummonMax (default 5) stand.
	Summon    string `yaml:"summon"`
	SummonMax int    `yaml:"summon_max"`
	Message   string `yaml:"message"`
	// OnUser makes the effect land on the user instead of the target (b is still the
	// target in the expressions): what is taken from the target goes to the user.
	OnUser bool `yaml:"on_user"`
	// Quiet shows no text (only the heal light and numbers; for per-turn auto heal and the like). If the heal is 0, nothing is shown.
	Quiet bool `yaml:"quiet"`
}

// State is a condition a battler is in: poison, sleep, a buff.
type State struct {
	Name     string `yaml:"name"`
	Short    string `yaml:"short"` // the badge shown by the HP bar
	Color    Color  `yaml:"color"`
	Turns    int    `yaml:"turns"`    // 0: until cured
	Restrict string `yaml:"restrict"` // "", skip, confuse
	// RestrictChance: with restrict skip, the one with the state loses an action only with this
	// chance (0 means always). The command is still chosen; the roll is made when it acts.
	RestrictChance float64 `yaml:"restrict_chance"`
	// Desc tells the player what the state does (the status menu shows it).
	Desc    string    `yaml:"desc"`
	Stats   StatRates `yaml:"stats"`
	Tick    []Effect  `yaml:"on_turn_end"`
	Wake    float64   `yaml:"remove_on_damage"` // chance a hit removes it
	Bad     bool      `yaml:"bad"`
	Persist bool      `yaml:"persist"` // stays after the battle
	// DamageRate multiplies the damage the one with the state takes (0.5 halves it; 0
	// means 1). Rates of several states multiply.
	DamageRate float64 `yaml:"damage_rate"`
	// Counter: the chance (an expression: a the one with the state, b the attacker) of striking
	// back with the attack skill when hit by a physical attack on one target.
	Counter Expr `yaml:"counter"`
	// Cover: the one with the state takes a physical attack on one target aimed at an ally
	// while this holds (a the one with the state, b the ally aimed at; e.g. "b.hp_rate < 0.5").
	Cover    Expr      `yaml:"cover"`
	Messages StateMsgs `yaml:"messages"`
	// Fatal: the one it takes falls at once (an instant death). Its chance is the effect's
	// chance times the resistances (an enemy's state_resist, the gear's and states' state_resist).
	Fatal bool `yaml:"fatal"`
	// StateResist multiplies the chance of other states taking the one with this state
	// (a ward against a death curse: {doom: 0}).
	StateResist map[string]float64 `yaml:"state_resist"`
	// StatsBy decides the stat multipliers by an expression (a is the holder of the state; var.* also usable. Example {atk: "1 + 0.05 * var.n"}).
	// It is multiplied with stats.
	StatsBy map[string]Expr `yaml:"stats_by"`
	// Again is the probability (an expression; a is the holder of the state) of attacking once more after a normal attack. If it hits, it rolls again (up to 3 in a row).
	Again Expr `yaml:"again"`
	// Endure is the number of times a fatal blow is withstood at HP 1 (per battle; an expression; a is the holder of the state).
	Endure Expr `yaml:"endure"`
	// TickAllies is an effect at the end of the turn on all standing allies of the state's holder (including itself) (a is the holder, b is the ally).
	TickAllies []Effect `yaml:"on_turn_end_allies"`
}

// StateMsgs are the battle messages of a state ({target} is replaced).
type StateMsgs struct {
	Add    string `yaml:"add"`
	Remove string `yaml:"remove"`
	Stay   string `yaml:"stay"` // shown when the state stops an action
	// Counter and Cover are shown when they happen ({target} the one with the state, {user}
	// the attacker or the ally covered)
	Counter string `yaml:"counter"`
	Cover   string `yaml:"cover"`
}

// Enemy is a kind of foe.
type Enemy struct {
	Name    string             `yaml:"name"`
	Image   Picture            `yaml:"image"`
	Scale   float64            `yaml:"scale"`
	Stats   Stats              `yaml:"stats"`
	Level   int                `yaml:"level"`
	Exp     int                `yaml:"exp"`
	Gold    int                `yaml:"gold"`
	Drops   []Drop             `yaml:"drops"`
	Resist  map[string]float64 `yaml:"resist"`       // damage multiplier by element
	SResist map[string]float64 `yaml:"state_resist"` // chance multiplier by state
	AI      AIRef              `yaml:"ai"`
	Boss    bool               `yaml:"boss"`
	// Title is the small line over a boss's name in the banner its battle begins with (the
	// default is the lang string "迷宮の番人"; a strong foe that is not a guardian says what it is).
	Title   string `yaml:"title"`
	Actions int    `yaml:"actions"` // actions per turn (default 1)
	// Hard: the plain attack (not a critical one) does at most this much damage to it; skills
	// and criticals do their full damage (a foe with a hard shell).
	Hard     int               `yaml:"hard"`
	Faces    map[string]string `yaml:"faces"` // image by mood (hurt, angry ...) for bosses drawn as people
	FaceRule []FaceRule        `yaml:"face_rules"`
	// Reactions are faces (keys of faces) shown for a moment: on hit, act, fall.
	Reactions map[string]string `yaml:"reactions"`
}

// Drop is an item an enemy may leave.
type Drop struct {
	Item   string  `yaml:"item"`
	Chance float64 `yaml:"chance"`
	// When is a condition for the drop (the battle's names; self is the beaten foe, whose
	// self.fell_with holds the states it had when it fell, turn the turn the battle ended):
	// a reward for how it was beaten.
	When Expr `yaml:"when"`
}

// AIRef is a battler's rules, inline or by the name of a set in the ai section.
type AIRef struct {
	Name  string
	Rules []Rule
}

// UnmarshalYAML reads a name or a list of rules.
func (a *AIRef) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		a.Name = s
		return nil
	}
	return unmarshal(&a.Rules)
}

// Rule is one rule of a battler's AI: when it holds, the battler may use the skill on the
// target. Of the rules that hold, those of the highest priority are kept, and one of them
// is drawn by weight.
type Rule struct {
	When     Expr    `yaml:"when"`     // self, allies, foes, turn, flag, var in scope
	Chance   Expr    `yaml:"chance"`   // the rule is skipped unless a draw passes
	Priority int     `yaml:"priority"` // higher wins
	Weight   float64 `yaml:"weight"`   // default 1
	Use      string  `yaml:"use"`      // a skill
	Item     string  `yaml:"item"`     // an item from the party's bag (party members only)
	Target   Target  `yaml:"target"`
	Say      string  `yaml:"say"` // a line shown before the action
	Once     bool    `yaml:"once"`
}

// Target picks a skill's target: from a side (foes or allies), those that pass where (t in
// scope), the one with the least or most by, or a random one.
type Target struct {
	Preset string `yaml:"preset"` // an existing shorthand field that is also usable in detailed maps.
	Side   string `yaml:"side"`
	Where  Expr   `yaml:"where"`
	Pick   string `yaml:"pick"` // min, max, random (default random)
	By     Expr   `yaml:"by"`
}

// TargetPresets are the named targets.
var TargetPresets = []string{"random", "self", "weakest", "strongest", "lowest_hp", "lowest_hp_rate", "highest_hp", "ally_lowest_hp_rate", "ally_dead", "first"}

// UnmarshalYAML reads a preset name or a map.
func (t *Target) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		t.Preset = s
		return nil
	}
	type plain Target
	return unmarshal((*plain)(t))
}

// Troop is a group of enemies met together.
type Troop struct {
	Name       string       `yaml:"name"`
	Members    []string     `yaml:"members"`
	Background Picture      `yaml:"background"`
	Music      MusicCue     `yaml:"music"`
	CanEscape  *bool        `yaml:"can_escape"`
	Events     []TroopEvent `yaml:"events"`
	Intro      string       `yaml:"intro"`
	// WinMusic is the song for winning against this enemy group (played once; takes precedence over game.yaml's battle.boss_win_music).
	WinMusic MusicCue `yaml:"win_music"`
	// LevelScale toughens the troop a little when the party comes stronger than meant (a boss).
	LevelScale *LevelScale `yaml:"level_scale"`
}

// LevelScale toughens a troop when the party's average level (rounded down) is above Level: the listed stats
// of its foes grow by Rate for each level above, up to Max (0.25: at most +25%). Past the cap
// every further level makes the battle easier again (training never loses).
type LevelScale struct {
	Level int      `yaml:"level"` // the level the battle is meant for
	Rate  float64  `yaml:"rate"`  // the growth per level above (0.05: +5%)
	Max   float64  `yaml:"max"`   // the cap of the growth (0.25: +25%)
	Stats []string `yaml:"stats"` // the stats it grows (default mhp, atk, mat)
	// Unbounded lifts the bosses' limit on Max (0.5): with no Max the foe keeps growing with the
	// party; with a Max over 0.5 it follows the party further than a boss before training wins (a
	// foe meant to stay above the party for long; not for the bosses on the way down).
	Unbounded bool `yaml:"unbounded"`
}

// Factor is the multiplier of the stats for a party of average level lv (rounded down; 1 at
// or below Level).
func (l *LevelScale) Factor(lv int) float64 {
	if l == nil {
		return 1
	}
	if l.Unbounded {
		f := max(0, l.Rate*float64(lv-l.Level))
		if l.Max > 0 {
			f = min(l.Max, f)
		}
		return 1 + f
	}
	return 1 + min(l.Max, max(0, l.Rate*float64(lv-l.Level)))
}

// Scales reports whether the stat grows.
func (l *LevelScale) Scales(stat string) bool {
	if l == nil {
		return false
	}
	if len(l.Stats) == 0 {
		return stat == "mhp" || stat == "atk" || stat == "mat"
	}
	return slices.Contains(l.Stats, stat)
}

// Escapable reports whether the party may run.
func (t *Troop) Escapable() bool { return t.CanEscape == nil || *t.CanEscape }

type TroopEvent struct {
	When Expr      `yaml:"when"`
	Once *bool     `yaml:"once"` // default true
	Do   []Command `yaml:"do"`
}

// RunsOnce reports whether the event fires only once per battle.
func (e *TroopEvent) RunsOnce() bool { return e.Once == nil || *e.Once }

// Location is a place the party can be: a picture with people to talk to, things to do
// and ways out.
type Location struct {
	Name       string    `yaml:"name"`
	Area       string    `yaml:"area"`
	Background Picture   `yaml:"background"`
	BattleBG   Picture   `yaml:"battle_background"`
	Music      MusicCue  `yaml:"music"`
	Desc       string    `yaml:"desc"`
	People     []Person  `yaml:"people"`
	Actions    []Action  `yaml:"actions"`
	Exits      []Exit    `yaml:"exits"`
	Encounters Encounter `yaml:"encounters"`
	OnEnter    []Command `yaml:"on_enter"`
	Safe       bool      `yaml:"safe"` // saving allowed here
	// TalkOnEnter: instead of listing "話す：〜", play one person's talk each time the party enters (people with a new talk first, otherwise in order).
	TalkOnEnter bool `yaml:"talk_on_enter"`
	NoMenu      bool `yaml:"no_menu"` // do not open the menu (status, equipment, save ...) here (for example, a town opens it only at the inn)
	// BattleMusic is the song for common battles here (the enemy group's music takes precedence. If the dungeon's looks has battle_music, that one.
	// Otherwise game.yaml's battle.music)
	BattleMusic MusicCue `yaml:"battle_music"`
	// Outfit: the actors talking here wear <faces>/<outfit>/<face>.png when there is one
	// (a winter coat in a snowy town); the face icons keep the usual pictures.
	Outfit  string   `yaml:"outfit"`
	Dungeon *Dungeon `yaml:"dungeon"` // a maze walked in first person instead of a menu
	// Map shows the location as a picture with points and roads (schema_version 3, ADR 0022).
	Map *Atlas `yaml:"map"`
	// Stage shows the location as its background painted from a fixed camera, walked by
	// small dolls (schema_version 3, ADR 0022).
	Stage *Stage `yaml:"stage"`
	// ResumeTo: when a save made here is loaded, start from this place (an ID in locations)
	// (for example, saving in a small place inside a shop starts from the town's first menu).
	// If not written, it stays at the place saved. The place name on the load screen also becomes this.
	ResumeTo string `yaml:"resume_to"`
}

// Person is someone at a location to talk to.
type Person struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Image string `yaml:"image"` // a picture under assets/
	When  Expr   `yaml:"when"`
	// New: an expression that holds only while talking to the person gives a new talk (a one-time talk, or one that advances relationships or the story).
	// It is not shown on screen (a hint for the automatic player to pick up new talks).
	New Expr `yaml:"new"`
	// Mark: an expression that, while it holds, puts a mark (lang.T("　！")) on the name in the "話す：〜" list.
	// What the mark signals is up to the game. If not written, no mark is added.
	Mark Expr      `yaml:"mark"`
	Talk []Command `yaml:"talk"`
	// Bubbles are lines said now and then in a balloon over the person's doll in a stage
	// (schema_version 3). The screen only: the rules never see them.
	Bubbles []Bubble `yaml:"bubbles"`
}

// Action is a thing to do at a location ("Search the chest", "Rest at the inn").
type Action struct {
	ID    string    `yaml:"id"` // an operation identifier that does not depend on the display name
	Label string    `yaml:"label"`
	When  Expr      `yaml:"when"`
	New   Expr      `yaml:"new"`  // an expression that holds only while there is a new talk (same as Person.New; not shown on screen)
	Mark  Expr      `yaml:"mark"` // an expression that, while it holds, marks the list label (same as Person.Mark)
	Do    []Command `yaml:"do"`
}

// Exit leads to another location.
type Exit struct {
	To    string    `yaml:"to"`
	Label string    `yaml:"label"`
	When  Expr      `yaml:"when"`
	Do    []Command `yaml:"do"` // run before moving; an exit with only do does not move
	// into a dungeon: the map character to arrive at, and the way to face
	At     string `yaml:"at"`
	Facing string `yaml:"facing"`
}

// Encounter is the battles met at a location: rate is the chance on arriving and on each
// search (in a dungeon: on each step).
type Encounter struct {
	Rate Expr `yaml:"rate"`
	// Gauge: each step fills a gauge by rate (shown on the screen); the chance of a battle on a
	// step is the gauge's fill times gauge (e.g. 0.3), and it empties after a battle.
	Gauge  float64     `yaml:"gauge"`
	Troops []TroopPick `yaml:"troops"`
	Search string      `yaml:"search"` // the label of a search action that may meet them
}

// TroopPick is a troop with its weight.
type TroopPick struct {
	Troop  string  `yaml:"troop"`
	Weight float64 `yaml:"weight"`
	When   Expr    `yaml:"when"`
}

// Event is a named list of commands: run from another event (call), or by itself when
// its trigger fires.
type Event struct {
	Trigger  string    `yaml:"trigger"` // "", enter, auto
	Location string    `yaml:"location"`
	When     Expr      `yaml:"when"`
	Once     bool      `yaml:"once"`
	Do       []Command `yaml:"do"`
}

// Shop is a list of goods.
type Shop struct {
	Name     string   `yaml:"name"`
	Goods    []string `yaml:"goods"`
	Keeper   string   `yaml:"keeper"` // a picture
	Greeting string   `yaml:"greeting"`
	Sell     *bool    `yaml:"sell"`
	// Spent is a var.<name> that every purchase here adds its price to (empty: not counted).
	// Several shops may share one, to count what was spent at one keeper.
	Spent string `yaml:"spent"`
}

// Music is the songs and the rules choosing the mood.
type Music struct {
	Bars    int                   `yaml:"bars"`          // a module's length (default 8)
	Beats   int                   `yaml:"beats_per_bar"` // default 4
	Songs   map[string]Song       `yaml:"songs"`
	OneShot []string              `yaml:"one_shot"`
	Rules   map[string][]MoodRule `yaml:"rules"` // by context: field, battle
	Volume  float64               `yaml:"volume"`
	// MoodNames are the displayed names of moods (in the gallery's music column, a one-shot is shown as "song title, name". If absent, the mood's id).
	MoodNames map[string]string `yaml:"mood_names"`
}

// Song is a piece of music, of one of two kinds:
//
//   - a track (file): one file played whole and looped from loop_start (seconds). The music
//     changes by changing the song (a location's or a battle's music, a rule's play).
//   - modular (dir, bpm, moods): modules of the same length (bars), one per mood, any of
//     which can follow any other; the rules' mood changes the music at the next module's
//     start, within the song (as the old Zelda games change a piece to fit the moment).
type Song struct {
	File      string            `yaml:"file"`
	LoopStart float64           `yaml:"loop_start"`
	Dir       string            `yaml:"dir"`
	BPM       int               `yaml:"bpm"`
	Moods     map[string]string `yaml:"moods"`
	Name      string            `yaml:"name"`
}

// MoodRule chooses the music by the moment (the first that holds): mood plays the mood of
// the modular song playing (song: only when that song plays); play changes the song itself
// (a track or a modular one), back to the place's own when no rule with play holds.
type MoodRule struct {
	When Expr   `yaml:"when"`
	Mood string `yaml:"mood"`
	Song string `yaml:"song"`
	Play string `yaml:"play"`
	// Now switches to the mood at once (no waiting for the module's end): a pinch, a boss
	// changing, the moments that should be heard as they happen.
	Now bool `yaml:"now"`
}

// Fits reports whether the item goes in an equip slot: its slot, or a numbered one of it
// (an item of slot stone fits stone1, stone2, stone3).
func (it *Item) Fits(slot string) bool {
	if it.Slot == "" {
		return false
	}
	if it.Slot == slot {
		return true
	}
	rest, ok := strings.CutPrefix(slot, it.Slot)
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Gallery is the title's gallery: pictures to look at again and scenes to play again.
type Gallery struct {
	CGs    []GalleryCG    `yaml:"cgs"`
	Scenes []GalleryScene `yaml:"scenes"`
	// Music is the song when a recollection starts (the scene's music if it has none, otherwise this one, and if that is absent, the place's song).
	// A music: inside the scene works as it is.
	Music MusicCue `yaml:"music"`
	// MusicPictures is the sequence of pictures shown per song (a songs id) in the music column. Choosing a song switches to the first;
	// while it plays it goes through the sequence in order, and once exhausted, randomly among the opened CGs. Unopened CGs are skipped.
	MusicPictures map[string][]string `yaml:"music_pictures"`
}

// GalleryCG is a picture of the gallery: open once it (any of its pages) has been shown by
// an image command (or its flag has been set). files takes the pages of one picture the
// way the image command does (cg/x.jpg, cg/x.*.jpg: the picture and its later stages),
// looked at one after another.
type GalleryCG struct {
	File  string   `yaml:"file"`
	Files []string `yaml:"files"`
	Title string   `yaml:"title"`
	Flag  string   `yaml:"flag"`
}

// Key is the picture's name in the record of what is open (its first file).
func (g GalleryCG) Key() string {
	if g.File == "" && len(g.Files) > 0 {
		return g.Files[0]
	}
	return g.File
}

// Pages are the pictures it shows, in order (the patterns matched in fsys under assets/).
func (g GalleryCG) Pages(fsys fs.FS) []string { return CG{File: g.File, Files: g.Files}.Pages(fsys) }

// Has reports whether file is one of its pictures (matches its file or one of its files).
func (g GalleryCG) Has(file string) bool {
	return MatchAny(append([]string{g.File}, g.Files...), file)
}

// MatchAny reports whether file is one of the paths or matches one of the patterns.
func MatchAny(patterns []string, file string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if ok, err := path.Match(p, file); (err == nil && ok) || p == file {
			return true
		}
	}
	return false
}

// GalleryScene is a scene played again from the gallery: the event, open once its flag
// has been set (and when holds, and a picture matching seen has been shown; at least one
// of the three). It plays with the starting party and no lasting effect (battles count as
// won, nothing is saved): a new state with the flag set, then set (flag.x / var.x:
// expression) written over it, so a scene can be played as a later visit or with the
// partner it needs.
type GalleryScene struct {
	// ID is a fixed name independent of the title and unlock condition. Old records are carried over from LegacyKeys.
	ID         string      `yaml:"id"`
	LegacyKeys []string    `yaml:"legacy_keys"`
	Title      string      `yaml:"title"`
	Event      string      `yaml:"event"`
	Flag       string      `yaml:"flag"`
	When       Expr        `yaml:"when"`
	Seen       string      `yaml:"seen"`
	Set        Assignments `yaml:"set"`
	Picture    string      `yaml:"picture"` // shown in the list (default none)
	// Group is the heading that groups entries in the recollection check page (a character name and so on; empty means "その他").
	Group string `yaml:"group"`
	// Location is the place where the recollection starts (default is the place where the event normally happens (EventPlaces); if undecided,
	// the first place). Background is the picture shown behind the recollection (default is that place's background. For a dungeon place, without moving the state,
	// that place's background or the battle background. Write it for things that should use a different picture, like a deep dungeon floor).
	Location   string  `yaml:"location"`
	Background Picture `yaml:"background"`
	// Music is the song when a recollection starts (default is gallery's music, otherwise the place's song).
	Music MusicCue `yaml:"music"`
	// Pages are entry points into the middle of a recollection. If omitted, it plays from the start as before.
	Pages []GalleryPage `yaml:"pages"`
}

// GalleryPage is a playback position counting lines and questions from 1, plus the answers to choices up to there.
// If Picture is given, only lines over that picture are counted into Line.
type GalleryPage struct {
	Title   string `yaml:"title"`
	Line    int    `yaml:"line"`
	Picture string `yaml:"picture"`
	Answers []int  `yaml:"answers"`
}

// Key is the key of the unlock record. For old YAML that omits ID, the legacy computation is kept.
func (g GalleryScene) Key() string {
	if g.ID != "" {
		return "scene:id:" + g.ID
	}
	if !g.When.Set() && g.Seen == "" {
		return "scene:" + g.Flag
	}
	return "scene:" + g.Event + "/" + g.Title
}

// MigrateUnlocks maps unlocks under old keys to fixed IDs. Unknown records and old keys are also kept.
// It can be applied repeatedly to the same record and returns true if a newly opened key exists.
func (g Gallery) MigrateUnlocks(unlocked map[string]bool) bool {
	changed := false
	for _, sc := range g.Scenes {
		if sc.ID == "" || unlocked[sc.Key()] {
			continue
		}
		for _, old := range sc.LegacyKeys {
			if unlocked[old] {
				unlocked[sc.Key()] = true
				changed = true
				break
			}
		}
	}
	return changed
}

// Brightness is the brightness to draw. The old spec and the Go zero value are 1; 0 in spec version 1 is black.
func (p Picture) Brightness() float64 {
	if !p.numbersResolved && p.Bright <= 0 {
		return 1
	}
	return p.Bright
}
