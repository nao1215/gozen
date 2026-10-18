package data

import "github.com/nao1215/gozen/internal/layer2/gamedata"

// AIRef is a battler's rules, inline or by the name of a set in the ai section.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type AIRef = gamedata.AIRef

// Action is a thing to do at a location ("Search the chest", "Rest at the inn").
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Action = gamedata.Action

// Actor is a character who can join the party.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Actor = gamedata.Actor

// Affect is the affect command: who (all, or an actor id) gets the state add for turns
// (0: the state's own), or loses the state remove ("bad": every bad state).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Affect = gamedata.Affect

// Aside は 話し手（actor・face・name の どれか。face は パターン可）の 台詞が 出た 直後に、When が 成り立てば
// Call の イベントを 挟む（終われば 元の 会話に 戻る）。一枚絵が 出て いる 間（立ち絵を 隠す 絵）、戦闘の 中、
// Skip の イベント（名前の パターン。そこから 呼ばれた 先も）、挟んだ イベントの 中 では 挟まない。
// 回数・相手の 回し方・一日の 上限は Call の イベントと When の 式で 書く（エンジンは 数えない）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Aside = gamedata.Aside

// BattleCfg are the rules every battle shares.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type BattleCfg = gamedata.BattleCfg

// BattleFx は 戦闘の 演出の 強さの 倍率。書かなければ 1、0 で その 演出を 切る。
// 光は 短く 控えめに し、揺れ・止め・数字の 弾み・間で 派手に 見せる。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type BattleFx = gamedata.BattleFx

// BattleFxAuto は オートの 間の 止め・溜め・スローの 倍率（揺れと 数字の 弾みは 変えない）。
// 書かなければ hitstop・charge・finish は 0（無し）、win は 0.4。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type BattleFxAuto = gamedata.BattleFxAuto

// CG is an event's picture over the whole screen (the image command). Written as a path
// ("" hides the picture shown), or as a map:
//
//	image: {file: cg/x.jpg, fit: cover, tint: "#ffd8e0", brightness: 0.9, fade: 1, faces: true}
//
// files shows several pictures one after another, a tap between them (the last stays
// shown for what follows). A file may be a pattern (cg/night_*.jpg: the files it matches,
// in name order; it may match none), so a scene takes more pictures without its events
// written again.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type CG = gamedata.CG

// CGReviewCfg は 一枚絵の 流し見の 決まり。Tags は 1 枚に 付ける 札（並びの 先頭 10 個は 数字の キー 1〜9・0 で 付く）。
// Extra は ギャラリーの 欄に 無いが 流し見に 足す 絵（パターン）、Groups は 入れるかを 流し見の 前に 選べる 絵の 組
// （既定は 入れない。Extra の 絵も Groups に 当たれば その 組の 選びに 従う）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type CGReviewCfg = gamedata.CGReviewCfg

// CGReviewGroup は 入れるかを 選べる 絵の 組（名前と 絵の パターン）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type CGReviewGroup = gamedata.CGReviewGroup

// CGReviewTag は 札: 記録に 残す id、出す 名前、OK の 札か（OK を 付けた 絵は「OK 済みを 除く」で 外れる）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type CGReviewTag = gamedata.CGReviewTag

// CardBubble は 札から 粒が 湧く 決まり。Actor（空なら 誰でも）の 札で、State に かかって いて
// （空なら 問わない）When が 成り立つ 間（空なら いつも）、札の 上の 辺から Count 個の 粒が
// Speed（画素/秒）で Rise だけ 上り、薄れて 消える。当てはまる 決まりが 複数 あれば 全部 描く。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type CardBubble = gamedata.CardBubble

// ChoiceOption is one answer of a choice.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type ChoiceOption = gamedata.ChoiceOption

// Class is how a kind of actor grows: stats by level, the skills learned and the
// equipment allowed.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Class = gamedata.Class

// Color is a color written "#rrggbb" or "#rrggbbaa".
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Color = gamedata.Color

// Command はイベントの１命令。command タグが動詞と付随項目の唯一の登録表。
// verb は公開動詞、internal は Go 専用、その他は付随項目を許す動詞の並び。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Command = gamedata.Command

// DB is a loaded game: game.yaml and every section of the data files.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type DB = gamedata.DB

// Drop is an item an enemy may leave.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Drop = gamedata.Drop

// Dungeon is the game definition contract for authors and consumers of loaded data.
// Its fields and methods follow the YAML contract in docs/schema.md.
type Dungeon = gamedata.Dungeon

// DungeonLook is another look of a dungeon (see Dungeon.Looks). What it leaves empty stays
// as the dungeon says.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type DungeonLook = gamedata.DungeonLook

// Effect is one thing a skill or an item does to each target.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Effect = gamedata.Effect

// Encounter is the battles met at a location: rate is the chance on arriving and on each
// search (in a dungeon: on each step).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Encounter = gamedata.Encounter

// Enemy is a kind of foe.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Enemy = gamedata.Enemy

// EquipStones puts the stones of a stone set on an actor (both are expressions giving ids).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type EquipStones = gamedata.EquipStones

// Event is a named list of commands: run from another event (call), or by itself when
// its trigger fires.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Event = gamedata.Event

// Exit leads to another location.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Exit = gamedata.Exit

// Expr is an expression of the expr language, parsed when the YAML is read. Its zero
// value is "no expression": a condition that holds and a formula that is 0.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Expr = gamedata.Expr

// FaceRule picks an actor's face from their state (self in scope): the first that holds.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type FaceRule = gamedata.FaceRule

// FloorPlan は 生成の 迷宮の 1 つの 階の 作り方（Generate.Floors）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type FloorPlan = gamedata.FloorPlan

// Fonts are the font files (paths under assets/).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Fonts = gamedata.Fonts

// Fusion makes a stone of two (at the stone shop).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Fusion = gamedata.Fusion

// Gallery is the title's gallery: pictures to look at again and scenes to play again.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Gallery = gamedata.Gallery

// GalleryCG is a picture of the gallery: open once it (any of its pages) has been shown by
// an image command (or its flag has been set). files takes the pages of one picture the
// way the image command does (cg/x.jpg, cg/x.*.jpg: the picture and its later stages),
// looked at one after another.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type GalleryCG = gamedata.GalleryCG

// GalleryPage は台詞と問いを１から数えた再生位置と、そこまでの選択肢の答え。
// Picture があれば、その絵の上の台詞だけを Line に数える。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type GalleryPage = gamedata.GalleryPage

// GalleryScene is a scene played again from the gallery: the event, open once its flag
// has been set (and when holds, and a picture matching seen has been shown; at least one
// of the three). It plays with the starting party and no lasting effect (battles count as
// won, nothing is saved): a new state with the flag set, then set (flag.x / var.x:
// expression) written over it, so a scene can be played as a later visit or with the
// partner it needs.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type GalleryScene = gamedata.GalleryScene

// Game is game.yaml: the game's title, screen and starting state.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Game = gamedata.Game

// GameOver is what is shown when the party falls.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type GameOver = gamedata.GameOver

// Generate is how a dungeon's floors are made: the size of the maze (odd numbers), how
// many rooms, and the characters placed on its floor (count an expression of floor).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Generate = gamedata.Generate

// Grant is a state given with a chance.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Grant = gamedata.Grant

// Hurt は戦闘外で指定した仲間の HP を減らす命令。amount の self は対象の仲間。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Hurt = gamedata.Hurt

// Item is a thing the party carries: a consumable, a piece of equipment, or a key item.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Item = gamedata.Item

// ItemCount is an item and how many.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type ItemCount = gamedata.ItemCount

// Learn teaches an actor a skill.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Learn = gamedata.Learn

// Learning is a skill learned at a level.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Learning = gamedata.Learning

// LevelScale toughens a troop when the party's average level (rounded down) is above Level: the listed stats
// of its foes grow by Rate for each level above, up to Max (0.25: at most +25%). Past the cap
// every further level makes the battle easier again (training never loses).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type LevelScale = gamedata.LevelScale

// Location is a place the party can be: a picture with people to talk to, things to do
// and ways out.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Location = gamedata.Location

// MemoCfg is what a test player's note asks and keeps: the kinds to choose from and the
// numbers kept with each note (an expression of the state's names each).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type MemoCfg = gamedata.MemoCfg

// MemoKind is a kind of note: its id (kept in the file) and the label shown.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type MemoKind = gamedata.MemoKind

// MemoVar is a number kept with each note.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type MemoVar = gamedata.MemoVar

// Meter is a gauge of a number: value out of max, shown while when holds. In the status
// menu it is shown on the page of actor (or on every page when actor is empty). At max the
// bar takes full_color and full is written after it.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Meter = gamedata.Meter

// MoodRule chooses the music by the moment (the first that holds): mood plays the mood of
// the modular song playing (song: only when that song plays); play changes the song itself
// (a track or a modular one), back to the place's own when no rule with play holds.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type MoodRule = gamedata.MoodRule

// Music is the songs and the rules choosing the mood.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Music = gamedata.Music

// MusicCue is a song and the mood it starts in ("song" or "song/mood", or a map).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type MusicCue = gamedata.MusicCue

// Passive は 戦いの 始めに actor に 付ける 状態 state（when が 成り立つ とき）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Passive = gamedata.Passive

// Person is someone at a location to talk to.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Person = gamedata.Person

// Picture is a picture under assets/, optionally tinted (multiplied by a color) and
// darkened. Written as a path, or as a map.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Picture = gamedata.Picture

// Put is a character placed on a made floor: how many (an expression of floor, the floor's
// number), the farthest from the start (far: stairs down) or anywhere.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Put = gamedata.Put

// Require is a node that must be raised to a level before another opens.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Require = gamedata.Require

// Roamer is a kind of strong foe walking a made floor (see Dungeon.Roamers).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Roamer = gamedata.Roamer

// Rule is one rule of a battler's AI: when it holds, the battler may use the skill on the
// target. Of the rules that hold, those of the highest priority are kept, and one of them
// is drawn by weight.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Rule = gamedata.Rule

// Say is a line of dialogue.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Say = gamedata.Say

// SceneReviewCfg は 回想の 札（OK の 札は 1 つ。無ければ OK と NG だけ）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type SceneReviewCfg = gamedata.SceneReviewCfg

// Screen is the logical screen size.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Screen = gamedata.Screen

// SfxVary は 続けて 鳴る 効果音の 音程と 音量の 揺れ（書かなければ 既定）。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type SfxVary = gamedata.SfxVary

// Shop is a list of goods.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Shop = gamedata.Shop

// Skill is an action in battle (or from the menu).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Skill = gamedata.Skill

// Slot is an equipment slot.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Slot = gamedata.Slot

// Song is a piece of music, of one of two kinds:
//
//   - a track (file): one file played whole and looped from loop_start (seconds). The music
//     changes by changing the song (a location's or a battle's music, a rule's play).
//   - modular (dir, bpm, moods): modules of the same length (bars), one per mood, any of
//     which can follow any other; the rules' mood changes the music at the next module's
//     start, within the song (as the old Zelda games change a piece to fit the moment).
//
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Song = gamedata.Song

// Start is the state a new game begins in.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Start = gamedata.Start

// StatFormulas are a formula by stat name, in terms of level (a class's growth).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StatFormulas = gamedata.StatFormulas

// StatRates are multipliers by stat name (a state's effect on the stats).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StatRates = gamedata.StatRates

// State is a condition a battler is in: poison, sleep, a buff.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type State = gamedata.State

// StateMsgs are the battle messages of a state ({target} is replaced).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StateMsgs = gamedata.StateMsgs

// StateName の Initial は式でなくリテラル。生成名の初期値は状態へ書かない。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StateName = gamedata.StateName

// StateNames は任意の変数宣言と、未知名を拒否する設定。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StateNames = gamedata.StateNames

// Stats are numbers by stat name (mhp, mmp, atk, def, mat, mdf, agi, luk).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Stats = gamedata.Stats

// StoneSet is a combination of magic stones worn together by one person: it grants a skill
// and stats while all its stones are worn.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type StoneSet = gamedata.StoneSet

// TacticAction は仲間が選べる行動と自動で選ぶ対象。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TacticAction = gamedata.TacticAction

// TacticCondition は条件の選択肢。template の式・抽選・優先度は作者だけが編集する。
// Subject の target は単体候補の絞り込み、self/allies/foes は発動条件を表す。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TacticCondition = gamedata.TacticCondition

// TacticPreset は作者が用意する型。仲間の名前付き AI ごとに５行まで定義する。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TacticPreset = gamedata.TacticPreset

// TacticRow は選択式の作戦１行。表示名や式を保存せず、カタログの安定 ID を参照する。
// Enabled のゼロ値は無効。Percent は閾値条件だけに必要で、nil と０を区別する。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TacticRow = gamedata.TacticRow

// TacticTarget は効果範囲と選び方を結び付ける。Selector の式は編集側に公開しない。
// Scope は Usage.Scope と同じ名前。self だけは one_ally にも使える。
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TacticTarget = gamedata.TacticTarget

// Target picks a skill's target: from a side (foes or allies), those that pass where (t in
// scope), the one with the least or most by, or a random one.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Target = gamedata.Target

// Terms are the words the menus use.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Terms = gamedata.Terms

// Tile is what a character of a dungeon's map is.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Tile = gamedata.Tile

// TitleFigure is one picture standing on the title screen. X is its center and Bottom its
// feet (parts of the screen's width and height); Height is a part of the screen's height.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TitleFigure = gamedata.TitleFigure

// TreeNode is a node of a skill tree: a skill (its level is the SP put in it) or a passive
// bonus (stats a level).
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TreeNode = gamedata.TreeNode

// Troop is a group of enemies met together.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Troop = gamedata.Troop

// TroopEvent is the game definition contract for authors and consumers of loaded data.
// Its fields and methods follow the YAML contract in docs/schema.md.
type TroopEvent = gamedata.TroopEvent

// TroopPick is a troop with its weight.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type TroopPick = gamedata.TroopPick

// UIStyle are the colors of the windows.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type UIStyle = gamedata.UIStyle

// Usage is what using an item or a skill does.
// This definition is exposed for game authors and hosts; see docs/schema.md.
type Usage = gamedata.Usage
