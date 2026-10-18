package gamedata

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Hurt は戦闘外で指定した仲間の HP を減らす命令。amount の self は対象の仲間。
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

// Command はイベントの１命令。command タグが動詞と付随項目の唯一の登録表。
// verb は公開動詞、internal は Go 専用、その他は付随項目を許す動詞の並び。
type Command struct {
	Say    *Say            `yaml:"say" command:"verb"`
	Choice []ChoiceOption  `yaml:"choice" command:"verb"`
	Prompt string          `yaml:"prompt" command:"choice,stone_choose,stone_wearer"` // a line shown with the choice
	If     Expr            `yaml:"if" command:"verb"`
	Then   []Command       `yaml:"then" command:"if"`
	Else   []Command       `yaml:"else" command:"if"`
	SetV   map[string]Expr `yaml:"set" command:"verb"`
	Give   *ItemCount      `yaml:"give" command:"verb"`
	Take   *ItemCount      `yaml:"take" command:"verb"`
	Gold   Expr            `yaml:"gold" command:"verb"`
	Join   string          `yaml:"join" command:"verb"`
	Leave  string          `yaml:"leave" command:"verb"`
	Heal   string          `yaml:"heal" command:"verb"`
	// Done marks st.Done[Done] (not written in YAML: the engine's own, a once dungeon tile
	// spent only when its question is answered yes).
	Done string `yaml:"-" command:"internal"`
	// Hurt takes HP from members outside battle (a poison floor, a trap); never below 1.
	Hurt *Hurt `yaml:"hurt" command:"verb"`
	// Affect puts members in a state, or takes one away, outside battle.
	Affect *Affect   `yaml:"affect" command:"verb"`
	Battle string    `yaml:"battle" command:"verb"`
	OnWin  []Command `yaml:"on_win" command:"battle"`
	OnLose []Command `yaml:"on_lose" command:"battle"`
	// Fighters are the actors who fight the battle (default: the party): a duel.
	Fighters []string `yaml:"fighters" command:"battle"`
	// Retry（retry: false）は負けても「やり直す」を聞かない（負けが話の続きになる戦い）。
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
	waitSet bool      // 仕様版１では待ち時間０も命令として扱う。
	// Notice は画面の上に短く出す知らせ（会話の窓を出さず、止めずに先へ進む。床の仕掛けの痛みなど地の文だけの知らせ）。
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
	// OpenWall は、迷宮で 今 立って いる マスの 隣の 壁（向いて いる 方を 先に。無ければ いちばん 近い、床に 面した 壁）を
	// その 文字の タイルに 変え、その 壁の 前で それを 向く。階ごとに 残り、セーブにも 残る（探索 100% で 現れる 階段など）。
	OpenWall string `yaml:"open_wall" command:"verb"`
	// StoneSuggest は、持っている石全部でいちばん強い着け方を探し、今より強ければ人ごとの着け替えを
	// var.<名前>_* に書く（var.<名前> は着け替える人数）。StoneChoose は組める組み合わせを一つ選ばせ、
	// StoneWearer は着ける人を選ばせる（自分で選びたい人のため）。
	// StoneDecline は断ったと覚える（持っている石が変わるまで stone_suggest は勧めない）。
	StoneSuggest string `yaml:"stone_suggest" command:"verb"`
	// StoneApply は stone_suggest の着け方（いちばん強い物）に、全員の石をまとめて着け替える。
	StoneApply   string `yaml:"stone_apply" command:"verb"`
	StoneChoose  string `yaml:"stone_choose" command:"verb"`
	StoneWearer  string `yaml:"stone_wearer" command:"verb"`
	StoneDecline string `yaml:"stone_decline" command:"verb"`
	// SPSuggest は、余った SP を仲間ごとにおまかせ（SpendSP と同じ振り方）で振る計画を作り、
	// var.<名前>_* に書く（var.<名前> は振る人数）。振らない。SPApply は計画どおりにまとめて振る。
	// SPDecline は断ったと覚える（仲間の SP の数が変わるまで sp_suggest は勧めない）。
	SPSuggest string `yaml:"sp_suggest" command:"verb"`
	SPApply   string `yaml:"sp_apply" command:"verb"`
	SPDecline string `yaml:"sp_decline" command:"verb"`
	// EquipStones は組み合わせの石をその人の石の枠に着ける（袋から、無ければ組み合わせに使っていない仲間の石を外して）。
	EquipStones *EquipStones `yaml:"equip_stones" command:"verb"`
	// given は YAML に明示された項目。false・０・空文字・null も省略と区別する。
	given []string
}

// EquipStones puts the stones of a stone set on an actor (both are expressions giving ids).
type EquipStones struct {
	Actor Expr `yaml:"actor"`
	Set   Expr `yaml:"set"`
}

// Say is a line of dialogue.
type Say struct {
	Who  string `yaml:"who"`  // an actor id, or any name
	Face string `yaml:"face"` // a face of the actor (or a picture path)
	Text string `yaml:"text"`
	Side string `yaml:"side"` // left, right (default: left for party, right for others)
	// Instead は戦闘中に who が倒れているときの言い替え。倒れていない最初の人の台詞を出す
	// （全員倒れているか、無ければ飛ばす）。戦闘の外では使わない。
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
	ID    string    `yaml:"id"` // 同じ問いの中で一意な識別子
	Label string    `yaml:"label"`
	When  Expr      `yaml:"when"`
	Do    []Command `yaml:"do"`
}

// ItemCount is an item and how many.
type ItemCount struct {
	Item  string `yaml:"item"`
	Count *int   `yaml:"count,omitempty"` // 省略は１。仕様版１の０は増減なし。
}

// N は個数。省略したときだけ１を返す。旧仕様の０は Load で省略へ戻す。
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

// commandField は Command のタグから一度だけ作る実行時の登録情報。
type commandField struct {
	name   string
	index  int
	role   string
	owners []string
}

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

// UnmarshalYAML は値に加えて明示したキーを保持する。動詞の妥当性は仕様版の補完後に確かめる。
func (c *Command) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Command
	var out plain
	if err := unmarshal(&out); err != nil {
		return err
	}
	var raw yaml.MapSlice
	if err := unmarshal(&raw); err != nil {
		return err
	}
	for _, entry := range raw {
		out.given = append(out.given, fmt.Sprint(entry.Key))
	}
	*c = Command(out)
	return nil
}

// Verb は動詞を返し、動詞の不足・重複と付随項目の配置を同じ登録表で検査する。
// Go から作る命令では非ゼロ値を使う。YAML はゼロ値でも書かれた項目として数える。
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
	if len(verbs) == 0 {
		return "", fmt.Errorf("a command without a verb")
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
