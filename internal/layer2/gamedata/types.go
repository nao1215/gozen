// Package data はゲームの YAML を DB へ読み、開始前に構造・参照・式を検査する。
// game.yaml、data/ 以下の YAML、assets/ 以下の素材を持つ fs.FS を Load へ渡す。
// 全項目と機械可読スキーマは docs/schema.md、公開候補の入口は docs/architecture/core-api.md を参照。
// エディタのスキーマだけでは参照先や式を保証しないため、Load の意味検査も必要。
package gamedata

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"io/fs"
	"path"
	"slices"
	"sort"

	"github.com/nao1215/gozen/expr"
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
	// SchemaVersion は YAML の仕様版。０（省略）は旧仕様。
	SchemaVersion int `yaml:"schema_version,omitempty"`
	// DataVersion は作品の内容の版。０（省略）は未管理。セーブ形式版とは別。
	DataVersion int `yaml:"data_version,omitempty"`
	// Text changes the engine's own words on the screens (text): the engine's
	// Japanese text (a format with its verbs too) -> the game's.
	Text     map[string]string `yaml:"text"`
	ID       string            `yaml:"id"` // names the save directory
	Title    string            `yaml:"title"`
	Subtitle string            `yaml:"subtitle"`
	Screen   Screen            `yaml:"screen"`
	Fonts    Fonts             `yaml:"fonts"`
	TitleBG  Picture           `yaml:"title_background"`
	TitleArt Picture           `yaml:"title_art"`
	// TitleFigures stand on the title screen in their order (the later ones in front); with
	// them, title_art is not drawn.
	TitleFigures []TitleFigure `yaml:"title_figures"`
	TitleBGM     MusicCue      `yaml:"title_music"`
	// Gallery is what the title's gallery shows: the event pictures seen and the scenes
	// whose flag was set, in any play (kept beside the saves).
	Gallery  Gallery `yaml:"gallery"`
	Start    Start   `yaml:"start"`
	Slots    []Slot  `yaml:"equip_slots"`
	MaxParty int     `yaml:"max_party"`
	// JoinExp は、仲間が加わる（イベントの join）ときの経験値の下限の式。加わる前のパーティで見る
	// （party.avg_exp など）。加わる人の経験値がそれより少なければ、そこまで上げる（下げない）。
	JoinExp  Expr      `yaml:"join_exp"`
	Battle   BattleCfg `yaml:"battle"`
	Terms    Terms     `yaml:"terms"`
	GameOver GameOver  `yaml:"game_over"`
	UI       UIStyle   `yaml:"ui"`
	// Meters are gauges of the player's numbers that the field, the dungeon and the status
	// menu show (a need that grows, a count toward something).
	Meters []Meter `yaml:"meters"`
	// CardBubbles は 仲間の 札（迷宮・街・戦闘の ステータスの 枠）から 湧く 小さな 粒（毒の 泡 など）。
	CardBubbles []CardBubble `yaml:"card_bubbles"`
	// Memo is the test player's notes (play memo.go): a key stops the game and keeps the
	// screen, the place and a word or two, to read later as the author's own judgment of the play.
	Memo MemoCfg `yaml:"memo"`
	// StandIns は 仮の 一枚絵の 一覧の ファイル（assets/ の 下。1 行に 1 枚、assets/ からの 名前）。
	// 絵が まだ 描けず 背景を 仮に 置いた 一枚絵は、絵に 話し手が いないので 立ち絵を 隠さない（faces: true と 同じ）。
	// 描けた 絵が 入って 一覧から 外れると、今まで どおり 隠す。
	StandIns string `yaml:"stand_ins"`
	// CGReview は 一枚絵の 流し見（ギャラリーを 全部 開いた 時の デバッグ。play cgreview.go）で 付ける 札と、
	// 入れるかを 選べる 絵の 組。
	CGReview CGReviewCfg `yaml:"cg_review"`
	// SceneReview は 回想検査の 頁（ギャラリーを 全部 開いた 時の デバッグ。play scenecheck.go）で 回想に 付ける 札。
	SceneReview SceneReviewCfg `yaml:"scene_review"`
	// Asides は 決まった 話し手の 台詞の 後に 挟む イベント（考え込む 間・頭の 中の 絵 など）。
	Asides []Aside `yaml:"asides"`
}

// Aside は 話し手（actor・face・name の どれか。face は パターン可）の 台詞が 出た 直後に、When が 成り立てば
// Call の イベントを 挟む（終われば 元の 会話に 戻る）。一枚絵が 出て いる 間（立ち絵を 隠す 絵）、戦闘の 中、
// Skip の イベント（名前の パターン。そこから 呼ばれた 先も）、挟んだ イベントの 中 では 挟まない。
// 回数・相手の 回し方・一日の 上限は Call の イベントと When の 式で 書く（エンジンは 数えない）。
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

// CGReviewCfg は 一枚絵の 流し見の 決まり。Tags は 1 枚に 付ける 札（並びの 先頭 10 個は 数字の キー 1〜9・0 で 付く）。
// Extra は ギャラリーの 欄に 無いが 流し見に 足す 絵（パターン）、Groups は 入れるかを 流し見の 前に 選べる 絵の 組
// （既定は 入れない。Extra の 絵も Groups に 当たれば その 組の 選びに 従う）。
type CGReviewCfg struct {
	Tags   []CGReviewTag   `yaml:"tags"`
	Extra  []string        `yaml:"extra"`
	Groups []CGReviewGroup `yaml:"groups"`
}

// Configured は制作側が一枚絵の検査を設定しているかを返す。
func (c CGReviewCfg) Configured() bool {
	return len(c.Tags)+len(c.Extra)+len(c.Groups) > 0
}

// CGReviewTag は 札: 記録に 残す id、出す 名前、OK の 札か（OK を 付けた 絵は「OK 済みを 除く」で 外れる）。
type CGReviewTag struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	OK    bool   `yaml:"ok"`
}

// SceneReviewCfg は 回想の 札（OK の 札は 1 つ。無ければ OK と NG だけ）。
type SceneReviewCfg struct {
	// Title は検査欄の名前。省略時は翻訳可能な「回想検査」。
	Title string        `yaml:"title"`
	Tags  []CGReviewTag `yaml:"tags"`
	// Groups は 見出しの 順（書いて いない 見出しは 後ろに、出た 順）。
	Groups []string `yaml:"groups"`
}

// Configured は制作側が回想の検査を設定しているかを返す。
// 従来の札・見出しだけの設定も有効とし、未設定では欄を作らない。
func (c SceneReviewCfg) Configured() bool {
	return c.Title != "" || len(c.Tags)+len(c.Groups) > 0
}

// CGReviewGroup は 入れるかを 選べる 絵の 組（名前と 絵の パターン）。
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
	// MaxValue はゲーム作者が状態に応じた上限を指定する式。評価成功時は
	// Max より優先し、1 以上に丸める。未指定・評価失敗時は Max を使う。
	// 指定時は現在値/上限を表示する（満杯時の Full は引き続き優先）。
	MaxValue  Expr   `yaml:"max_value"`
	When      Expr   `yaml:"when"`
	Color     Color  `yaml:"color"`
	Full      string `yaml:"full"`
	FullColor Color  `yaml:"full_color"`
	Actor     string `yaml:"actor"`
	// Marks are values marked on the bar (where something becomes possible).
	Marks []float64 `yaml:"marks"`
}

// CardBubble は 札から 粒が 湧く 決まり。Actor（空なら 誰でも）の 札で、State に かかって いて
// （空なら 問わない）When が 成り立つ 間（空なら いつも）、札の 上の 辺から Count 個の 粒が
// Speed（画素/秒）で Rise だけ 上り、薄れて 消える。当てはまる 決まりが 複数 あれば 全部 描く。
type CardBubble struct {
	Actor string  `yaml:"actor"`
	State string  `yaml:"state"`
	When  Expr    `yaml:"when"`
	Color Color   `yaml:"color"` // 既定 白
	Size  float64 `yaml:"size"`  // 粒の 半径（既定 3）
	Count int     `yaml:"count"` // 同時に 出る 数（既定 4、上限 24）
	Speed float64 `yaml:"speed"` // 上る 速さ（既定 30）
	Rise  float64 `yaml:"rise"`  // 上る 高さ（既定 50）
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
	// Manual はメニューの「さいきょうそうび」と、強い装備のおすすめで触らない枠（組み合わせで
	// 効く物など、合計の数値で比べられない物の枠）。
	Manual bool `yaml:"manual"`
}

// Passive は 戦いの 始めに actor に 付ける 状態 state（when が 成り立つ とき）。
type Passive struct {
	Actor string `yaml:"actor"`
	State string `yaml:"state"`
	When  Expr   `yaml:"when"`
	// Level は スキルツリーの 画面に 出す 段（式。書かなければ 出さない）。開く 前（when が 成り立たない）は 名前も 伏せる。
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
	// AutoTactics は名前付き AI ごとのおまかせ作戦。未指定の AI は従来の規則を使う。
	AutoTactics map[string][]TacticRow `yaml:"auto_tactics"`
	// SimpleTactics は５行・条件と行動だけの編集を使う。
	SimpleTactics bool                    `yaml:"simple_tactics"`
	TacticPresets map[string]TacticPreset `yaml:"tactic_presets"`
	// Tempo is how fast a battle's messages and moves go by (1: the default; 1.5 half as fast again).
	Tempo float64 `yaml:"tempo"`
	// FrontWeight と BackWeight は、敵が前列・後列の 1 人を狙う重み（既定は 4 と 1。
	// 前列の 1 人は後列の 1 人の 4 倍狙われる）。0 なら狙わない（全員 0 なら等しく狙う）。
	FrontWeight *float64 `yaml:"front_weight"`
	BackWeight  *float64 `yaml:"back_weight"`
	// AfterWin is an event run after every battle won (a gauge to fill, a line to say).
	AfterWin string `yaml:"after_win"`
	// Before は戦闘要求の直前に実行するイベント ID。空なら何もしない。
	// ゲーム作者の資源消費や参加条件に使う。移動したら元の進行を中止し、
	// 画面の再試行はこの処理後の状態から始まる。
	Before string `yaml:"before"`
	// After は結果を反映した後、次の選択・戦闘またはイベント終了時に
	// 実行するイベント ID。ゲーム作者の資源判定・帰還に使う。空なら省略し、
	// 敗北分岐がない通常敗北では全滅処理を優先する。
	After string `yaml:"after"`
	// FoeScale multiplies the stats of the foes that are not bosses (mhp: 1.4, atk: 1.25…):
	// the tension of the common battles, tuned in one place.
	FoeScale map[string]float64 `yaml:"foe_scale"`
	// FoeScaleWhen, when set, is when foe_scale holds (var.floor >= 3: not the very first floors).
	FoeScaleWhen Expr `yaml:"foe_scale_when"`
	// Passives は 戦いの 始めに 仲間に 付ける 状態（when が 成り立つ 間。隠しの 能力など）。戦いの 後には 残らない。
	Passives []Passive `yaml:"passives"`
	Attack   string    `yaml:"attack_skill"`
	Guard    string    `yaml:"guard_skill"`
	// Retry は負けたときに「たたかう前からやり直す」を聞く（戦う直前の状態に戻して同じ戦いを
	// もう一度）。断ると今までどおり on_lose か game_over へ。battle の命令の retry: false で外せる。
	Retry bool `yaml:"retry"`
	// BossWinMusic は 番人（Boss の 敵）の いる 戦いに 勝った ときの 曲（一度 鳴らして 止める）。敵グループの
	// win_music が 先。どちらも 無ければ 今まで どおり 戦闘の 曲の 勝ちの 気分（music の rules）。
	// 鳴って いる 間は 勝った 後の イベントにも 続き、鳴り終わって から 場所の 曲に 戻る。
	BossWinMusic MusicCue `yaml:"boss_win_music"`
	// AutoStart は 戦いを はじめから オートで 進める（設定の「戦闘を オートで はじめる」を
	// 遊び手が 決めて いない ときの 既定）。X で 解除すれば その 戦いは 手で 選ぶ。
	AutoStart bool `yaml:"auto_start"`
	// AutoTempo は オートの 間の 文と 待ちの 速さの 倍率（既定 1。1.5 で 1.5 倍 速い）。
	// 戦闘の 中の 会話も オートの 間は 決定を 待たずに 進む（待ちを この 倍率で 縮める）。
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
	// Backlog is how many lines the backlog keeps (default 300).
	Backlog int   `yaml:"backlog"`
	Window  Color `yaml:"window"`
	Border  Color `yaml:"border"`
	Text    Color `yaml:"text"`
	Accent  Color `yaml:"accent"`
	Cursor  Color `yaml:"cursor"`
	Shadow  Color `yaml:"shadow"`
	NameTag Color `yaml:"name_tag"`
	// StageScale は 立ち絵ごとの 大きさの 倍率（絵の ファイル → 倍率。既定 1）。頭の 位置は そのままで 小さく 描く
	// （体が 絵いっぱいに 描かれて ほかの 人より 大きく 見える 絵を 引いて 見せる）
	StageScale map[string]float64 `yaml:"stage_scale"`
	// StageDrop は 立ち絵ごとに 下げる 量（絵の ファイル → 画面の 高さに 対する 割合。既定 0）。
	// どの 絵も 頭の 上の 端を そろえて 描くので、背の 低い 人・髪を 高く 結った 人が 背の 高い 人と 同じ 背に 見える。それを 下げて 直す
	StageDrop    map[string]float64 `yaml:"stage_drop"`
	FieldNotices *bool              `yaml:"field_notices"`
	// BattleFx は 戦闘の 演出の 強さ（play の battle_drama.go）。
	BattleFx BattleFx `yaml:"battle_fx"`
	// SfxVary は 同じ 効果音を 短い 間に 続けて 鳴らす ときの 揺らぎ（sound の vary.go）。
	// pitch は 音程（既定 0.03 で ±3%）、volume は 音量（既定 0.1 で ±10%）。0 で 切る。
	SfxVary SfxVary `yaml:"sfx_vary"`
}

// SfxVary は 続けて 鳴る 効果音の 音程と 音量の 揺れ（書かなければ 既定）。
type SfxVary struct {
	Pitch  *float64 `yaml:"pitch"`
	Volume *float64 `yaml:"volume"`
}

// Values は 揺れの 大きさ（書いて いない 物は 既定の 0.03 と 0.1）。
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

// BattleFx は 戦闘の 演出の 強さの 倍率。書かなければ 1、0 で その 演出を 切る。
// 光は 短く 控えめに し、揺れ・止め・数字の 弾み・間で 派手に 見せる。
type BattleFx struct {
	Shake   *float64 `yaml:"shake"`   // 会心・弱点・とどめの 画面の 揺れ
	Flash   *float64 `yaml:"flash"`   // 会心・弱点の 白い 閃光（短い）
	Pop     *float64 `yaml:"pop"`     // ダメージの 数字の 大きさと 弾み
	Hitstop *float64 `yaml:"hitstop"` // 会心・弱点の 一撃で 画面を 止める 長さ
	Charge  *float64 `yaml:"charge"`  // 番人の 大技（カットイン・全体）の 前の 溜めの 長さ
	Finish  *float64 `yaml:"finish"`  // とどめの 一撃の 止めと スロー、番人を 倒した 表示の 長さ
	// Auto は オートで 戦って いる 間に 上の 倍率へ さらに かける 倍率（遊び手を 待たせない）。
	Auto BattleFxAuto `yaml:"auto"`
}

// BattleFxAuto は オートの 間の 止め・溜め・スローの 倍率（揺れと 数字の 弾みは 変えない）。
// 書かなければ hitstop・charge・finish は 0（無し）、win は 0.4。
type BattleFxAuto struct {
	Hitstop *float64 `yaml:"hitstop"` // 会心・弱点の 止め
	Charge  *float64 `yaml:"charge"`  // 番人の 大技の 前の 溜め
	Finish  *float64 `yaml:"finish"`  // とどめの 止めと スロー
	Win     *float64 `yaml:"win"`     // 番人を 倒した 帯を 決定で 飛ばせない 間
}

// Get は 倍率（nil なら def、負なら 0）。
func (BattleFxAuto) Get(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return max(0, *p)
}

// Get は 倍率（nil なら 1、負なら 0）。
func (BattleFx) Get(p *float64) float64 {
	if p == nil {
		return 1
	}
	return max(0, *p)
}

// Picture is a picture under assets/, optionally tinted (multiplied by a color) and
// darkened. Written as a path, or as a map.
type Picture struct {
	numbersResolved bool    // 仕様版１の実効値。Go のゼロ値は従来の省略として扱う。
	File            string  `yaml:"file"`
	Tint            Color   `yaml:"tint"`
	Bright          float64 `yaml:"brightness"` // 省略は１。０は旧仕様で１、新仕様で黒。
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
	fadeGiven       bool     // 消去時の省略は、表示中の絵の秒数を引き継ぐ。
	numbersResolved bool     // 明示した０を利用側でも保持する。
	File            string   `yaml:"file"`
	Files           []string `yaml:"files"`
	Fit             string   `yaml:"fit"` // contain (default: all of it, on a blurred copy) or cover (fills the screen)
	Tint            Color    `yaml:"tint"`
	Bright          float64  `yaml:"brightness"` // 省略は１。０は旧仕様で１、新仕様で黒。
	Fade            float64  `yaml:"fade"`       // 秒数。省略は0.25。０は旧仕様で0.25、新仕様で即時。
	Faces           bool     `yaml:"faces"`      // keep the speakers' pictures over it (hidden by default)
	Page            int      `yaml:"page"`       // with files: show only this one of the pages (1 is the first; past the last, the last)
	// Over is a picture standing in the middle of it, its feet above the message window
	// (the monster met in a dungeon, its battle picture over the place's background, shown
	// while the party talks before the battle). OverSize is its height, times the screen's
	// (default 0.55).
	Over     Picture `yaml:"over"`
	OverSize float64 `yaml:"over_size"`
	// Pick は files の 何枚目を 出すかを 式で 選ぶ（値を 枚数で 割った 余り + 1 枚目。回ごとに 違う 絵を 出す）。
	Pick Expr `yaml:"pick"`
	// Optional は files が まだ 何にも 当たらなくて よい（描けたら 出る。当たらない 間は 何も 出さない）。
	// 使い回しを 避けて、代わりの 絵を 置かない 欄に 使う。
	Optional bool `yaml:"optional"`
	// Look は 見せ方。"dream" は 頭の 中の 絵（思い出す・思い描く）: 縁を ぼかし、縁から Haze の 色の 霞を 掛け、
	// 出て いる 間 ゆっくり 寄る。Haze が 無ければ 薄い 桃色。
	Look string `yaml:"look"`
	Haze Color  `yaml:"haze"`
	// Auto は files を 順に 送る ときの 1 枚の 秒数。書くと 押さなくても 次の 絵へ 進む（押せば すぐ 進む）。
	// 絵だけを 見せる スライドショー（会話の 窓は 出ない）。0 なら 押すまで 待つ。
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

// HasFade は消去側に秒数を明記したか。新版の明示した０も含む。
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
	// SPPitch は sp_suggest が振り方を勧めるときのひとこと（この職の役割に合わせた理由）。
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
	// Family は表示名と独立した系列 ID。省略時は従来どおり同じ名前を一系列として扱う。
	Family string   `yaml:"family"`
	Name   string   `yaml:"name"`
	Desc   string   `yaml:"desc"`
	Stones []string `yaml:"stones"`
	Skill  string   `yaml:"skill"`
	Stats  Stats    `yaml:"stats"`
	// For は、この組み合わせを着けるのに合う仲間（先の人から。stone_suggest の既定の着け手）。
	For []string `yaml:"for"`
	// Only は for の人が着けたときだけ成り立つ（ほかの人が着けても何も起きない）。
	Only bool `yaml:"only"`
	// Pitch は石屋が勧めるときのひとこと（同じ系列の並びの最初の物に書けばよい）。
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
	// CutinBox は カットインの 絵（Cutin）の 顔の 範囲: 中心（幅・高さの 割合）と 大きさ（頭の てっぺんから
	// あごまで、幅の 割合）。無ければ 絵の 上から 頭を 探す（細い 絵では 頭を 小さく 見て 髪だけが 帯に 入った）。
	CutinBox []float64 `yaml:"cutin_box"`
	// FaceBox is where the face is on the standing pictures (for the face icons and the
	// cut-in): the middle (x of the width, y of the height) and the size (of the width).
	// Without it the head is found from the picture's top (which hair ornaments push up).
	FaceBox []float64 `yaml:"face_box"`
	// FaceShade darkens the face icons cut from the standing pictures (0 = as drawn, 1 =
	// black): 立ち絵を 出さない 主人公の 顔を 影に 沈める。ギャラリーや 立ち絵は 変えない。
	FaceShade float64 `yaml:"face_shade"`
	// FaceShadeTop is the darkness at the top of the icon, falling evenly to FaceShade at
	// the bottom (目元を 影にして 口元を うっすら 見せる)。書かなければ FaceShade と 同じ。
	FaceShadeTop float64 `yaml:"face_shade_top"`
	// NoStanding: the actor is not shown standing in the talks nor in the cut-ins (the
	// protagonist the player stands in for); the face icon stays.
	NoStanding bool       `yaml:"no_standing"`
	FaceRule   []FaceRule `yaml:"face_rules"`
	// Reactions are faces shown for a moment: on hit, act, heal, win, fall.
	Reactions map[string]string `yaml:"reactions"`
	// Tree are the nodes of the class's skill tree raised from the start (node -> level),
	// paid from the start's SP: a skill to use from the first battle.
	Tree map[string]int `yaml:"tree"`
}

// FaceRule picks an actor's face from their state (self in scope): the first that holds.
type FaceRule struct {
	When Expr   `yaml:"when"`
	Face string `yaml:"face"`
}

// Item is a thing the party carries: a consumable, a piece of equipment, or a key item.
type Item struct {
	Name string `yaml:"name"`
	// Short は短い呼び名（組み合わせの提案で「白と蒼」のように並べる。無ければ name）。
	Short string `yaml:"short"`
	Desc  string `yaml:"desc"`
	Kind  string `yaml:"kind"` // item, key, or an equipment kind (sword, staff, ...)
	// Event is run when the item is used from the menu (used up; the menu closes): a return
	// to town, a map of the floor... (when is a condition for using it)
	Event   string `yaml:"event"`
	EventOK Expr   `yaml:"event_when"`
	// Command は event の品を、どうぐの一覧でなくメニューの一番上の行動として出す（使える条件
	// event_when が成り立つときだけ見える）。持ち物というより「すること」の品に使う。
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
	// Quiet は 文を 出さない（回復の 光と 数字だけ。毎ターンの 自動回復など）。回復が 0 なら 何も 出さない。
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
	// StatsBy は 能力の 倍率を 式で 決める（a は 状態を 持つ者。var.* も 使える。例 {atk: "1 + 0.05 * var.n"}）。
	// stats と 掛け合わせる。
	StatsBy map[string]Expr `yaml:"stats_by"`
	// Again は、ふつうの 攻撃の 後に もう一度 攻撃する 確率（式。a は 状態を 持つ者）。当たれば また 振る（続けて 3 回まで）。
	Again Expr `yaml:"again"`
	// Endure は、倒れる 一撃を HP 1 で 耐える 回数（1 回の 戦いで。式。a は 状態を 持つ者）。
	Endure Expr `yaml:"endure"`
	// TickAllies は ターンの 終わりに、状態を 持つ者の 立って いる 味方 全員（自分も）に 効く 効果（a は 持つ者、b は 味方）。
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
	// default is lang の「迷宮の番人」; a strong foe that is not a guardian says what it is).
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
	Preset string `yaml:"preset"` // 詳細マップでも使える既存の略記欄。
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
	// WinMusic は この 敵グループに 勝った ときの 曲（一度 鳴らす。game.yaml の battle.boss_win_music より 先）。
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
	for _, s := range l.Stats {
		if s == stat {
			return true
		}
	}
	return false
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
	// TalkOnEnter: 「話す：〜」を 並べず、入る たびに 人の 話を 1 つ 流す（新しい 話の ある 人を 先に、無ければ 順番に）。
	TalkOnEnter bool `yaml:"talk_on_enter"`
	NoMenu      bool `yaml:"no_menu"` // ここでは メニュー（ステータス・装備・セーブ…）を 開かない（街は 宿だけで 開く、など）
	// BattleMusic は ここの 雑魚戦の 曲（敵グループの music が 先。迷宮の looks の battle_music が あれば そちら。
	// 無ければ game.yaml の battle.music）
	BattleMusic MusicCue `yaml:"battle_music"`
	// Outfit: the actors talking here wear <faces>/<outfit>/<face>.png when there is one
	// (a winter coat in a snowy town); the face icons keep the usual pictures.
	Outfit  string   `yaml:"outfit"`
	Dungeon *Dungeon `yaml:"dungeon"` // a maze walked in first person instead of a menu
	// ResumeTo: ここで セーブした 物を ロードすると、この 場所（locations の ID）から 始める
	// （店の 中の 小さな 場所で セーブしても、街の 最初の メニューから 始める、など）。
	// 書かなければ セーブした 場所の まま。ロードの 画面の 場所の 名前も こちらに なる。
	ResumeTo string `yaml:"resume_to"`
}

// Person is someone at a location to talk to.
type Person struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Image string `yaml:"image"` // a picture under assets/
	When  Expr   `yaml:"when"`
	// New: 話しかけると新しい話（一度だけの話・関係や物語が進む話）がある間だけ成り立つ式。
	// 画面には出さない（自動の遊び手が新しい話を拾う手がかり）。
	New Expr `yaml:"new"`
	// Mark: 成り立つ間、「話す：〜」の一覧で名前に印（lang.T("　！")）を付ける式。
	// 印が何を知らせるかはゲームが決める。書かなければ印は付かない。
	Mark Expr      `yaml:"mark"`
	Talk []Command `yaml:"talk"`
}

// Action is a thing to do at a location ("Search the chest", "Rest at the inn").
type Action struct {
	ID    string    `yaml:"id"` // 表示名に依存しない操作の識別子
	Label string    `yaml:"label"`
	When  Expr      `yaml:"when"`
	New   Expr      `yaml:"new"`  // 新しい話がある間だけ成り立つ式（Person.New と同じ。画面には出さない）
	Mark  Expr      `yaml:"mark"` // 成り立つ間、一覧のラベルに印を付ける（Person.Mark と同じ）
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
	// MoodNames は 気分の 見せる 名前（ギャラリーの 音楽の 欄で 一発ものを「曲名・名前」と 出す。無ければ 気分の id）。
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
func (i *Item) Fits(slot string) bool {
	if i.Slot == "" {
		return false
	}
	if i.Slot == slot {
		return true
	}
	rest, ok := strings.CutPrefix(slot, i.Slot)
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
	// Music は回想を始めるときの曲（場面の music が無ければこれ、これも無ければ場所の曲）。
	// 場面の中の music: はそのまま効く。
	Music MusicCue `yaml:"music"`
	// MusicPictures は 音楽の 欄で 曲（songs の id）ごとに 出す 絵の 並び。曲を 選ぶと 1 つめへ 替わり、
	// 流して いる 間は 並びを 順に、尽きたら ひらいた 一枚絵から ランダムに。ひらいて いない 一枚絵は 飛ばす。
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
	// ID は題名や開放条件と独立した固定名。旧記録は LegacyKeys から引き継ぐ。
	ID         string          `yaml:"id"`
	LegacyKeys []string        `yaml:"legacy_keys"`
	Title      string          `yaml:"title"`
	Event      string          `yaml:"event"`
	Flag       string          `yaml:"flag"`
	When       Expr            `yaml:"when"`
	Seen       string          `yaml:"seen"`
	Set        map[string]Expr `yaml:"set"`
	Picture    string          `yaml:"picture"` // shown in the list (default none)
	// Group は 回想検査の 頁で まとめる 見出し（キャラの 名前など。空は「その他」）。
	Group string `yaml:"group"`
	// Location は 回想を 始める 場所（既定は その イベントが ふだん 起きる 場所（EventPlaces）、決まらなければ
	// はじめの 場所）。Background は 回想の 後ろに 出す 絵（既定は その 場所の 背景。迷宮の 場所なら 状態は 動かさず、
	// その 場所の 背景か 戦闘の 背景。迷宮の 深い 階のように 別の 絵に したい 物に 書く）。
	Location   string  `yaml:"location"`
	Background Picture `yaml:"background"`
	// Music は回想を始めるときの曲（既定は gallery の music、それも無ければ場所の曲）。
	Music MusicCue `yaml:"music"`
	// Pages は回想の途中へ移る入口。省略時は従来どおり先頭から再生する。
	Pages []GalleryPage `yaml:"pages"`
}

// GalleryPage は台詞と問いを１から数えた再生位置と、そこまでの選択肢の答え。
// Picture があれば、その絵の上の台詞だけを Line に数える。
type GalleryPage struct {
	Title   string `yaml:"title"`
	Line    int    `yaml:"line"`
	Picture string `yaml:"picture"`
	Answers []int  `yaml:"answers"`
}

// Key は開放記録のキー。ID を省略した旧 YAML では従来の計算を維持する。
func (g GalleryScene) Key() string {
	if g.ID != "" {
		return "scene:id:" + g.ID
	}
	if !g.When.Set() && g.Seen == "" {
		return "scene:" + g.Flag
	}
	return "scene:" + g.Event + "/" + g.Title
}

// MigrateUnlocks は旧キーの開放を固定 ID へ写す。未知の記録と旧キーも保持する。
// 同じ記録へ繰り返し適用でき、新しく開いたキーがあれば true を返す。
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

// Brightness は描画する明るさ。旧仕様と Go のゼロ値は１、仕様版１の０は黒。
func (p Picture) Brightness() float64 {
	if !p.numbersResolved && p.Bright <= 0 {
		return 1
	}
	return p.Bright
}
