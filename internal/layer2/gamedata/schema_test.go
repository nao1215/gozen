package gamedata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// updateSchema は環境変数 GOZEN_UPDATE_SCHEMA=1 のときに真で、YAML スキーマと項目一覧を書き直す。
var updateSchema = os.Getenv("GOZEN_UPDATE_SCHEMA") == "1"

type schemaNode map[string]any

type fieldContract struct {
	Default       string `json:"default"`
	LegacyDefault string `json:"legacy_default"`
	Zero          string `json:"zero"`
	Required      string `json:"required"`
	Reference     string `json:"reference"`
	Version       string `json:"version"`
}

// 型・数値契約・命令タグは実装から読む。文脈による継承と参照だけを補足する。
var schemaDefaults = map[string]string{
	"StoneSet.family": "省略時は従来の名前による系列。明示 ID とは別系列",
	"GalleryScene.id": "省略時は従来のフラグまたはイベントと題名によるキー", "GalleryScene.legacy_keys": "旧キーからの引き継ぎなし",
	"GalleryScene.pages": "頁選択なし。先頭から再生", "GalleryPage.picture": "回想開始から台詞と問いを数える", "GalleryPage.answers": "未指定の選択肢で止まる",
	"Game.schema_version": "0（旧仕様）", "Game.language": "en（英語）", "Game.data_version": "0（未管理）",
	"Game.equip_slots": "weapon・armor・accessory の３枠", "Game.id": "旧版は空文字。新版は必須。標準保存先の準備には空でない ID が必要",
	"Actor.face": "normal", "Actor.face_shade_top": "face_shade を継承",
	"BattleCfg.attack_skill": "attack", "BattleCfg.guard_skill": "guard", "BattleCfg.ai": "priority",
	"BattleCfg.critical_rate": "1.5", "BattleCfg.variance": "0.1", "BattleCfg.guard_rate": "0.5",
	"BattleCfg.front_weight": "4", "BattleCfg.back_weight": "1", "BattleCfg.music": "名前が battle の曲があれば使用",
	"BattleCfg.escape":   "0.5",
	"BattleCfg.critical": "会心なし", "BattleCfg.speed": "a.agi × (0.8 + 0.4 × 乱数)",
	"Terms.currency": "G", "Terms.level": "Lv", "Terms.hp": "HP", "Terms.mp": "MP", "Terms.exp": "EXP",
	"TreeNode.id": "skill の ID", "TreeNode.name": "参照した技の名前", "TreeNode.desc": "参照した技の説明",
	"Item.kind": "item", "Item.consume": "true", "Usage.scope": "技は one_enemy、品は one_ally",
	"Usage.where": "技は [battle]、品は [battle, menu]", "Usage.hit": "1", "Usage.crit": "battle.critical（両方省略なら会心なし）",
	"Usage.speed": "行動順への加算なし（指定時は battle.speed に加算）", "Effect.stat": "hp", "Effect.variance": "battle.variance を継承",
	"Effect.chance": "1", "Grant.chance": "1", "Rule.chance": "1", "Roamer.wait": "0", "Generate.size": "width・height の寸法", "Effect.turns": "付与する状態の turns", "Affect.turns": "付与する状態の turns",
	"Target.pick": "random", "Song.name": "曲の ID", "Music.mood_names": "空の対応表。未指定の気分名は ID", "GalleryScene.location": "イベントから推定。決まらなければ開始場所", "GalleryScene.background": "回想の場所の背景（迷宮では戦闘背景も参照）", "GalleryScene.music": "gallery.music、次に回想の場所の曲", "Gallery.music": "回想の場所の曲", "Target.side": "行動の効果範囲から決める", "Target.preset": "行動の効果範囲から決める",
	"Troop.can_escape": "true", "TroopEvent.once": "true", "Shop.sell": "true",
	"LevelScale.stats": "[mhp, atk, mat]", "Music.volume": "0.7", "UIStyle.message": "box",
	"UIStyle.field_notices": "true", "UIStyle.stage_scale": "空の対応表。未指定の絵は倍率１", "UIStyle.stage_drop": "空の対応表。未指定の絵は移動０",
	"CG.fit": "contain", "CG.look": "通常表示", "CG.haze": "dream の場合は薄い桃色",
	"CG.fade": "表示は0.25秒。消去は表示中の秒数を継承", "CG.page": "0（全頁を順に表示）",
	"DungeonLook.darkness": "元の迷宮の darkness を継承", "Roamer.far": "通常は max(6, 最遠距離 × 3/5)、appear ありは０",
	"Roamer.move": "wander と同じうろつき", "Roamer.count": "1", "Roamer.every": "1", "Put.count": "1",
	"Tile.look": "floor", "Tile.trigger": "wall は look、solid は bump、それ以外は step",
	"Tile.else": ".（床）", "Tile.ask": "翻訳した調べる", "Tile.question": "タイルの name",
	"Tile.facing": "north", "Exit.facing": "到着タイルの facing。未指定なら north",
	"Command.facing": "到着タイルの facing。未指定なら north", "Command.retry": "battle.retry を継承",
	"Command.fighters": "現在のパーティ全員", "Command.floor": "1", "Say.side": "仲間は left、それ以外は right",
	"SceneReviewCfg.title": "翻訳した回想検査", "SceneReviewCfg.tags": "OK と NG の２札",
	"TacticCondition.weight": "0（template の Rule 変換時に１）", "StateName.initial": "省略不可。宣言型と同じリテラル",
}

var schemaReferences = map[string]string{
	"GalleryPage.picture": "assets/ の画像。その絵の上の台詞と問いを数える", "GalleryPage.answers": "選択肢の０始まりの答え[]",
	"StoneSet.family": "表示名と独立した系列 ID。英数字で始まる英数字・_・-。同系列の全候補に同じ ID",
	"GalleryScene.id": "回想の固定 ID。英数字で始まる英数字・_・-。回想内で一意", "GalleryScene.legacy_keys": "scene: で始まる旧開放キー[]。id が必須で別回想との共有不可",
	"Start.location": "locations", "Start.party": "actors[]", "Start.items": "キーは items", "Start.event": "events",
	"Game.language": "en（英語）・ja（日本語）", "Game.text": "text の辞書の原文（キーはエンジンの日本語の文言）。language の文言の上に重ねる", "Fonts.regular": "assets/ のフォント", "Fonts.pop": "assets/ のフォント", "Fonts.title": "assets/ のフォント",
	"Game.stand_ins": "assets/ の一覧ファイル", "TitleFigure.file": "assets/ の画像", "Picture.file": "assets/ の画像",
	"CG.file": "assets/ の画像", "CG.files": "assets/ の画像またはパターン[]", "CG.pick": "頁番号を選ぶ式",
	"MusicCue.song": "music.songs", "MusicCue.mood": "参照した曲の moods", "MoodRule.song": "music.songs", "MoodRule.play": "music.songs または曲/気分", "MoodRule.mood": "曲の moods",
	"Song.file": "assets/ の音声", "Song.dir": "assets/ のモジュール曲ディレクトリ", "Song.moods": "値は曲ディレクトリ内のレイヤー指定", "Music.one_shot": "曲の moods の名前[]",
	"Actor.class": "classes", "Actor.equip": "キーは equip_slots、値は items", "Actor.faces": "assets/ の表情ディレクトリ", "Actor.face": "faces 内の表情", "Actor.cutin": "assets/ の画像", "Actor.tree": "キーは所属職業の tree[].id",
	"FaceRule.face": "対象の表情", "Enemy.faces": "値は assets/ の画像", "Enemy.resist": "キーは elements", "Enemy.state_resist": "キーは states", "Item.state_resist": "キーは states", "State.state_resist": "キーは states",
	"Class.equip": "items の kind[]", "TreeNode.skill": "skills", "Require.node": "同じ職業の tree[].id", "Learning.skill": "skills",
	"Item.event": "events", "Item.slot": "equip_slots[].id", "Item.element": "elements", "Grant.state": "states", "Drop.item": "items",
	"Effect.element": "elements", "Effect.add_state": "states", "Effect.remove_state": "states または bad", "Effect.summon": "enemies",
	"Troop.members": "enemies[]", "TroopPick.troop": "troops", "Rule.use": "skills", "Rule.item": "items",
	"StoneSet.stones": "items[]", "StoneSet.skill": "skills", "StoneSet.for": "actors[]", "Fusion.from": "items[]", "Fusion.to": "items",
	"Location.resume_to": "locations", "Event.location": "locations", "Exit.to": "locations", "Exit.at": "到着迷宮の tiles の文字",
	"Shop.goods": "items[]", "Shop.keeper": "assets/ の画像", "Shop.spent": "数値の var の名前", "Person.image": "assets/ の画像",
	"Aside.actor": "actors", "Aside.face": "表情画像またはパターン", "Aside.call": "events", "Aside.skip": "events のパターン[]",
	"BattleCfg.tactic_presets": "TacticPreset の ID と定義", "TacticPreset.rows": "キーは ai、値は TacticRow[]", "TacticAction.target": "tactic_targets",
	"BattleCfg.attack_skill": "skills", "BattleCfg.guard_skill": "skills", "BattleCfg.after_win": "events", "BattleCfg.before": "events", "BattleCfg.after": "events", "BattleCfg.auto_tactics": "キーは ai、値は TacticRow[]",
	"Passive.actor": "actors", "Passive.state": "states", "GameOver.event": "events", "Meter.actor": "actors", "CardBubble.actor": "actors", "CardBubble.state": "states",
	"Command.join": "actors", "Command.leave": "actors", "Command.heal": "actors または all", "Command.battle": "troops", "Command.fighters": "actors[]",
	"Command.move": "locations", "Command.dive": "generate を持つ locations", "Command.at": "到着迷宮の tiles の文字", "Command.call": "events", "Command.shop": "shops",
	"Command.reset_tree": "actors または all", "Command.open_wall": "現在の迷宮の tiles の文字", "Command.set": "キーは代入先、値は式",
	"Learn.actor": "actors", "Learn.skill": "skills", "ItemCount.item": "items", "Affect.who": "actors または all", "Affect.add": "states", "Affect.remove": "states または bad", "Hurt.who": "actors または all",
	"EquipStones.actor": "actors の ID を返す式", "EquipStones.set": "stone_sets の ID を返す式", "Say.who": "actors または任意の名前", "Say.face": "仲間の表情または assets/ の画像",
	"TacticRow.condition": "tactic_conditions", "TacticRow.target": "tactic_targets", "TacticRow.action": "tactic_actions", "TacticCondition.state": "states", "TacticAction.use": "skills", "TacticAction.item": "items",
	"Dungeon.pictures": "キー・値とも assets/ の画像", "DungeonLook.pictures": "キー・値とも assets/ の画像", "Generate.start": "tiles の文字", "Put.char": "tiles の文字", "Put.near": "先に配置した place[].char", "Tile.else": "tiles または組込み文字", "Tile.after": "tiles または組込み文字",
	"GalleryCG.file": "assets/ の画像", "GalleryCG.files": "assets/ の画像またはパターン[]", "GalleryCG.flag": "flag の名前", "GalleryScene.event": "events", "GalleryScene.flag": "flag の名前", "GalleryScene.seen": "assets/ の画像またはパターン", "GalleryScene.picture": "assets/ の画像", "GalleryScene.location": "locations", "GalleryScene.set": "キーは代入先、値は式",
}

var schemaRequired = map[string]string{
	"GalleryPage.title": "必須。空白だけを拒否", "GalleryPage.line": "必須。１以上",
	"Game.start": "必須", "Game.id": "仕様版１で必須", "Start.location": "必須", "Start.party": "１人以上必須、重複不可",
	"Class.stats": "全８能力必須", "Class.exp_for": "必須", "Actor.class": "必須", "Enemy.image": "必須（file が必要）", "Enemy.stats": "全８能力必須", "Troop.members": "１体以上必須",
	"Dungeon.map": "generate がなければ必須", "Generate.width": "必須、９以上の奇数", "Generate.height": "必須、９以上の奇数", "Generate.start": "必須",
	"TacticCondition.name": "必須", "TacticCondition.kind": "必須", "TacticCondition.subject": "HP・MP・生死・状態条件で必須", "TacticCondition.state": "状態条件で必須", "TacticCondition.when": "template で必須",
	"TacticTarget.name": "必須", "TacticTarget.scope": "必須", "TacticAction.name": "必須", "TacticAction.use": "use・item の一方だけ必須", "TacticAction.item": "use・item の一方だけ必須",
	"TacticRow.condition": "必須（無効行も検査）", "TacticRow.target": "行動の target がない従来形式で必須。簡易作戦では指定不可", "TacticRow.action": "必須（無効行も検査）", "TacticRow.percent": "従来の HP・MP 条件で必須。簡易作戦では指定不可",
	"StateName.type": "必須", "StateName.initial": "必須", "ItemCount.item": "必須", "Learn.actor": "必須", "Learn.skill": "必須",
	"TitleFigure.file": "必須", "Meter.value": "必須", "Aside.actor": "actor・face・name のいずれか必須", "Aside.face": "actor・face・name のいずれか必須", "Aside.name": "actor・face・name のいずれか必須",
	"MemoKind.id": "必須、種類内で一意", "MemoKind.label": "必須", "MemoVar.name": "必須", "MemoVar.value": "必須", "CGReviewTag.id": "札内で一意な ID が必要", "CGReviewTag.label": "札の表示名が必要", "CGReviewGroup.label": "必須", "CGReviewGroup.files": "１件以上必須",
	"TreeNode.id": "skill なしでは必須、職業内で一意", "TreeNode.skill": "skill・stats のいずれか必須", "TreeNode.stats": "skill・stats のいずれか必須", "Require.node": "必須",
	"Item.event": "command が true なら必須", "Exit.to": "to・do のいずれか必須", "Exit.do": "to・do のいずれか必須", "Song.bpm": "file なしのモジュール曲では必須", "Song.file": "file なしでは bpm が必要、file と moods は併用不可",
	"MoodRule.mood": "mood・song・play のいずれか必須", "MoodRule.song": "mood・song・play のいずれか必須", "MoodRule.play": "mood・song・play のいずれか必須", "Rule.use": "use・item の一方だけ必須", "Rule.item": "use・item の一方だけ必須", "Target.by": "pick が min・max なら必須",
	"Hurt.amount": "必須", "Affect.add": "add・remove のいずれか必須", "Affect.remove": "add・remove のいずれか必須", "EquipStones.actor": "必須", "EquipStones.set": "必須",
	"Effect.damage": "効果の種類を１つだけ指定", "Effect.heal": "効果の種類を１つだけ指定", "Effect.add_state": "効果の種類を１つだけ指定", "Effect.remove_state": "効果の種類を１つだけ指定", "Effect.revive": "効果の種類を１つだけ指定", "Effect.escape": "効果の種類を１つだけ指定", "Effect.summon": "効果の種類を１つだけ指定", "Effect.turns": "add_state だけに指定可",
	"StoneSet.name": "必須", "StoneSet.stones": "１件以上必須", "StoneSet.skill": "skill・stats のいずれか必須", "StoneSet.stats": "skill・stats のいずれか必須", "StoneSet.for": "only が true なら１人以上必須", "Fusion.from": "１件以上必須", "Fusion.to": "必須",
	"LevelScale.level": "必須、１以上", "LevelScale.rate": "必須、０より大きく0.2以下", "LevelScale.max": "unbounded 以外では必須、０より大きく0.5以下", "Put.char": "必須", "FloorPlan.seed": "seed・map の一方だけ必須", "FloorPlan.map": "seed・map の一方だけ必須",
	"Roamer.picture": "picture・icon のいずれか必須", "Roamer.icon": "picture・icon のいずれか必須", "Roamer.troops": "troops・meet のいずれか必須", "Roamer.meet": "troops・meet のいずれか必須", "Roamer.appear": "count と併用不可", "Roamer.count": "appear と併用不可",
	"GalleryCG.title": "必須", "GalleryCG.file": "file・files の一方だけ必須", "GalleryCG.files": "file・files の一方だけ必須", "GalleryScene.title": "必須", "GalleryScene.event": "必須", "GalleryScene.flag": "flag・when・seen のいずれか必須", "GalleryScene.when": "flag・when・seen のいずれか必須", "GalleryScene.seen": "flag・when・seen のいずれか必須",
}

type schemaBuilder struct {
	defs  schemaNode
	rows  map[string]map[string]fieldContract
	types map[string]reflect.Type
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{schemaNode{}, map[string]map[string]fieldContract{}, map[string]reflect.Type{}}
}

func schemaName(t reflect.Type) string {
	if t == reflect.TypeFor[file]() {
		return "DataFile"
	}
	return t.Name()
}

func (b *schemaBuilder) node(t reflect.Type) schemaNode {
	if t.Kind() == reflect.Pointer {
		n := b.node(t.Elem())
		if t.Elem().Kind() == reflect.Bool {
			n["type"] = []string{"boolean", "null"}
		}
		return n
	}
	if t == reflect.TypeFor[Expr]() {
		return schemaNode{"type": []string{"string", "number", "boolean", "null"}, "description": "式。構文・名前・型は data.Load で検査する"}
	}
	if t == reflect.TypeFor[Color]() {
		return schemaNode{"type": "string", "pattern": "^#?[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$"}
	}
	if t == reflect.TypeFor[AIRef]() {
		return schemaNode{"anyOf": []any{schemaNode{"type": "string"}, schemaNode{"type": []string{"array", "null"}, "items": b.node(reflect.TypeFor[Rule]())}}}
	}
	switch t.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Struct:
		name := schemaName(t)
		if _, ok := b.defs[name]; !ok {
			b.defs[name] = schemaNode{}
			b.types[name] = t
			b.rows[name] = map[string]fieldContract{}
			props := schemaNode{}
			for _, f := range schemaFields(t) {
				owner, field := f.owner, f.field
				contract := schemaContract(owner, field, f.name)
				b.rows[name][f.name] = contract
				n := b.node(field.Type)
				n["x-contract"] = contract
				props[f.name] = n
			}
			n := schemaNode{"type": "object", "properties": props, "additionalProperties": false}
			b.constraints(t, n, props)
			switch t {
			case reflect.TypeFor[Picture](), reflect.TypeFor[CG](), reflect.TypeFor[Say](), reflect.TypeFor[MusicCue](), reflect.TypeFor[Target]():
				b.defs[name] = schemaNode{"anyOf": []any{schemaNode{"type": "string"}, n}}
			default:
				b.defs[name] = n
			}
		}
		return schemaNode{"$ref": "#/$defs/" + name}
	case reflect.Map:
		return schemaNode{"type": []string{"object", "null"}, "additionalProperties": b.node(t.Elem())}
	case reflect.Slice, reflect.Array:
		n := schemaNode{"type": []string{"array", "null"}, "items": b.node(t.Elem())}
		if t.Kind() == reflect.Array {
			n["minItems"], n["maxItems"] = t.Len(), t.Len()
		}
		return n
	case reflect.String:
		return schemaNode{"type": "string"}
	case reflect.Bool:
		return schemaNode{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return schemaNode{"type": "integer"}
	case reflect.Float64:
		return schemaNode{"type": "number"}
	case reflect.Interface:
		return schemaNode{"type": []string{"boolean", "number", "string"}}
	default:
		panic(fmt.Sprintf("未対応の YAML 型 %v", t))
	}
}

type schemaField struct {
	owner reflect.Type
	field reflect.StructField
	name  string
}

func schemaFields(t reflect.Type) []schemaField {
	var out []schemaField
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		tag, ok := f.Tag.Lookup("yaml")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if opts == "inline" {
			out = append(out, schemaFields(f.Type)...)
			continue
		}
		out = append(out, schemaField{t, f, name})
	}
	return out
}

func schemaContract(owner reflect.Type, field reflect.StructField, name string) fieldContract {
	c := fieldContract{Default: "空の定義（子項目の既定値を適用）", Zero: "数値以外", Required: "省略可", Reference: "なし", Version: "０・１"}
	t := field.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		c.Default = "未指定（nil）。定義を作らない"
	}
	switch t.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.String:
		c.Default = "空文字"
	case reflect.Bool:
		c.Default = "false"
	case reflect.Int, reflect.Int64, reflect.Uint64, reflect.Float64:
		c.Default, c.Zero = "0", "値０を保持"
	case reflect.Map:
		c.Default = "空の対応表"
	case reflect.Slice, reflect.Array:
		c.Default = "空の一覧"
	}
	if field.Type.Kind() == reflect.Pointer {
		c.Default = "未指定（nil）。定義を作らない"
	}
	if t == reflect.TypeFor[Expr]() {
		c.Default = "未指定の式（条件は成立、数式は０。個別の継承は下記）"
		c.Zero = "式の数値０（条件なら不成立）"
		c.Reference = "式の文脈は DSL と data.Load で検査"
	}
	if t == reflect.TypeFor[Color]() {
		c.Default = "未指定の色（表示先の既定色）"
	}
	if t == reflect.TypeFor[AIRef]() {
		c.Default = "未指定の AI（仲間は職業の AI を継承）"
		c.Reference = "ai の ID または Rule[]"
	}

	key := owner.Name() + "." + name
	if value, ok := schemaDefaults[key]; ok {
		c.Default = value
	}
	if value, ok := schemaReferences[key]; ok {
		c.Reference = value
	}
	if value, ok := schemaRequired[key]; ok {
		c.Required = value
	}
	if rule, ok := numericContracts[owner][name]; ok {
		if _, overridden := schemaDefaults[key]; !overridden {
			c.Default = fmt.Sprint(rule.fallback)
		}
		c.Zero = "新版は値０を保持。旧版は数値監査表の項目別規則"
		if rule.positive || rule.lower > 0 {
			c.Zero = "新版は拒否。旧版は数値監査表の項目別規則"
		}
		c.Version = "０・１（省略・０の違いは数値監査表）"
	}
	switch key {
	case "GalleryPage.line":
		c.Zero = "両版で拒否"
	case "Affect.turns":
		c.Zero = "両版で状態の turns を使う"
	case "State.turns":
		c.Zero = "両版で治すまで続く"
	case "State.restrict_chance":
		c.Zero = "両版で常に行動制限（restrict を外せば制限なし）"
	case "Dungeon.darkness", "DungeonLook.darkness":
		c.Zero = "両版で暗くしない"
	case "Class.start_sp", "Actor.face_shade":
		c.Zero = "両版で値０を保持"
	case "Learning.level":
		c.Zero = "旧版は最初から習得、新版は拒否"
	case "Generate.rooms":
		c.Zero = "新版は部屋なし。旧版は size の式があると最小１"
	case "Generate.width", "Generate.height":
		c.Zero = "両版で拒否"
	case "Command.wait":
		c.Zero = "旧版は動詞なしで拒否、新版は待たずに進む"
	case "CG.page", "CG.auto":
		c.Zero = "両版で全頁を手動送り"
	case "BattleCfg.critical_rate", "BattleCfg.variance", "BattleCfg.guard_rate", "Music.volume":
		c.Zero = "旧版は既定値、新版は値０を保持"
	case "BattleCfg.front_weight", "BattleCfg.back_weight", "Effect.variance":
		c.Zero = "両版で値０を保持"
	}
	if owner == reflect.TypeFor[Command]() {
		role := field.Tag.Get("command")
		if role == "verb" {
			c.Required = "命令ごとに動詞１つだけ必須"
		} else {
			c.Required = "省略可。許可する動詞は " + role
		}
	}
	c.LegacyDefault = c.Default
	switch key {
	case "Learning.level":
		c.LegacyDefault = "0（最初から習得対象）"
	case "Screen.height":
		c.LegacyDefault = "width が０なら720、それ以外は０のまま"
	case "Generate.rooms":
		c.LegacyDefault = "0。ただし size の式があれば最小１"
	}
	return c
}

func required(names ...string) schemaNode { return schemaNode{"required": names} }
func (b *schemaBuilder) constraints(t reflect.Type, n, props schemaNode) {
	var req []string
	for _, f := range schemaFields(t) {
		requirement := schemaRequired[f.owner.Name()+"."+f.name]
		if strings.HasPrefix(requirement, "必須") || strings.HasPrefix(requirement, "全８能力必須") || requirement == "１人以上必須、重複不可" || requirement == "１件以上必須" || requirement == "１体以上必須" {
			req = append(req, f.name)
		}
	}
	if len(req) > 0 {
		n["required"] = req
	}
	switch t {
	case reflect.TypeFor[GalleryPage]():
		props["title"].(schemaNode)["pattern"] = "\\S"                     //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["line"].(schemaNode)["minimum"] = 1                          //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["answers"].(schemaNode)["items"].(schemaNode)["minimum"] = 0 //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	case reflect.TypeFor[Game]():
		props["schema_version"].(schemaNode)["enum"] = []int{0, 1}        //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["data_version"].(schemaNode)["minimum"] = 0                 //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["language"].(schemaNode)["enum"] = []string{"", "en", "ja"} //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		n["if"] = schemaNode{"required": []string{"schema_version"}, "properties": schemaNode{"schema_version": schemaNode{"const": 1}}}
		n["then"] = schemaNode{"required": []string{"id"}, "properties": schemaNode{"id": schemaNode{"type": "string", "pattern": gameIDPattern.String()}}}
	case reflect.TypeFor[Start]():
		props["party"].(schemaNode)["type"] = "array"     //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["party"].(schemaNode)["minItems"] = 1       //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["party"].(schemaNode)["uniqueItems"] = true //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	case reflect.TypeFor[Troop]():
		n["required"] = []string{"members"}
		props["members"].(schemaNode)["type"] = "array" //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["members"].(schemaNode)["minItems"] = 1   //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	case reflect.TypeFor[Class]():
		props["stats"].(schemaNode)["required"] = StatNames //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["stats"].(schemaNode)["type"] = "object"      //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	case reflect.TypeFor[Enemy]():
		props["stats"].(schemaNode)["required"] = StatNames //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		props["stats"].(schemaNode)["type"] = "object"      //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	case reflect.TypeFor[Command]():
		var branches []any
		for _, verb := range commandFields {
			if verb.role != "verb" {
				continue
			}
			allowed := schemaNode{}
			for _, field := range commandFields {
				if field.name == verb.name || (field.role != "verb" && field.role != "internal" && slices.Contains(field.owners, verb.name)) {
					allowed[field.name] = schemaNode{}
				}
			}
			branches = append(branches, schemaNode{"required": []string{verb.name}, "properties": allowed, "additionalProperties": false})
		}
		n["oneOf"] = branches
		for _, name := range []string{"end", "clear", "save"} {
			props[name].(schemaNode)["const"] = true //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		}
	case reflect.TypeFor[TacticAction]():
		n["oneOf"] = []any{required("use"), required("item")}
	case reflect.TypeFor[StateName]():
		props["type"].(schemaNode)["enum"] = []string{"bool", "number", "string"} //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		var branches []any
		for k, v := range map[string]string{"bool": "boolean", "number": "number", "string": "string"} {
			branches = append(branches, schemaNode{"properties": schemaNode{"type": schemaNode{"const": k}, "initial": schemaNode{"type": v}}})
		}
		sort.Slice(branches, func(i, j int) bool { return fmt.Sprint(branches[i]) < fmt.Sprint(branches[j]) })
		n["oneOf"] = branches
	case reflect.TypeFor[TacticRow]():
		props["percent"].(schemaNode)["minimum"], props["percent"].(schemaNode)["maximum"] = 0, 100 //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	}
}

func schemaJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n')
}

// TestSchemaCoverage は型の追加・削除・命令タグ・数値契約と配布する表のずれを検出する。
// 更新は GOZEN_UPDATE_SCHEMA=1 go test ./internal/layer2/gamedata -run '^TestSchemaCoverage$'。
func TestSchemaCoverage(t *testing.T) {
	b := newSchemaBuilder()
	b.node(reflect.TypeFor[Game]())
	b.node(reflect.TypeFor[file]())
	artifacts := map[string][]byte{}
	artifacts["../../../data/schema/types.schema.json"] = schemaJSON(t, schemaNode{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": b.defs, "description": "共通の YAML 型。範囲・参照・式の意味検査は data.Load が正"})
	for file, name := range map[string]string{"game": "Game", "data": "DataFile"} {
		artifacts["../../../data/schema/"+file+".schema.json"] = schemaJSON(t, schemaNode{"$schema": "https://json-schema.org/draft/2020-12/schema", "$ref": "types.schema.json#/$defs/" + name})
	}
	for _, metadata := range []map[string]string{schemaDefaults, schemaReferences, schemaRequired} {
		for key := range metadata {
			owner, name, ok := strings.Cut(key, ".")
			if !ok {
				t.Fatalf("契約キーが不正 %s", key)
			}
			if _, ok := b.rows[owner][name]; !ok {
				t.Errorf("存在しない項目の契約 %s", key)
			}
		}
	}

	for typ, rules := range numericContracts {
		for name := range rules {
			found := false
			for _, field := range schemaFields(typ) {
				if field.name == name {
					found = true
				}
			}
			if !found || b.types[schemaName(typ)] == nil {
				t.Errorf("数値契約の項目を網羅していない %s.%s", typ.Name(), name)
			}
		}
	}
	for name, typ := range b.types {
		fields := schemaFields(typ)
		for field := range typ.Fields() {
			if field.IsExported() && field.Tag.Get("yaml") == "" {
				t.Errorf("%s.%s に YAML の公開区分が無い", name, field.Name)
			}
		}
		if len(fields) != len(b.rows[name]) {
			t.Errorf("%s の項目が重複", name)
		}
		for _, f := range fields {
			c := b.rows[name][f.name]
			if c.Default == "" || c.LegacyDefault == "" || c.Zero == "" || c.Required == "" || c.Reference == "" || c.Version == "" {
				t.Errorf("契約が不足 %s.%s", name, f.name)
			}
		}
	}
	var md strings.Builder
	md.WriteString(schemaIntroduction) //nolint:errcheck // strings.Builder writes always succeed.
	names := make([]string, 0, len(b.rows))
	for name := range b.rows {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&md, "\n## %s\n\n| 項目 | YAML の型 | 省略（旧版／新版） | ０ | 必須性・配置 | 参照先 | 仕様版 |\n| --- | --- | --- | --- | --- | --- | --- |\n", name) //nolint:errcheck // strings.Builder writes always succeed.
		keys := make([]string, 0, len(b.rows[name]))
		for key := range b.rows[name] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fields := map[string]reflect.Type{}
		for _, f := range schemaFields(b.types[name]) {
			fields[f.name] = f.field.Type
		}
		for _, key := range keys {
			c := b.rows[name][key]
			fmt.Fprintf(&md, "| `%s` | %s | %s | %s | %s | %s | %s |\n", key, schemaTypeLabel(fields[key]), schemaDefaultLabel(c), c.Zero, c.Required, c.Reference, c.Version) //nolint:errcheck // strings.Builder writes always succeed.
		}
	}
	artifacts["../../../docs/schema.md"] = []byte(md.String())
	for name, want := range artifacts {
		if updateSchema {
			if err := os.MkdirAll(filepath.Dir(name), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, want, 0600); err != nil {
				t.Fatal(err)
			}
		}
		got, err := os.ReadFile(name) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s が実装と不一致。GOZEN_UPDATE_SCHEMA=1 で再生成して差分を確認する", name)
		}
	}
	t.Logf("%d 型の YAML 項目と３スキーマ・項目一覧を照合", len(b.rows))
}

func schemaDefaultLabel(c fieldContract) string {
	if c.LegacyDefault == c.Default {
		return c.Default
	}
	return "旧版 " + c.LegacyDefault + "／新版 " + c.Default
}

func schemaTypeLabel(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return schemaTypeLabel(t.Elem())
	}
	switch t {
	case reflect.TypeFor[Expr]():
		return "式のスカラー"
	case reflect.TypeFor[Color]():
		return "色の文字列"
	case reflect.TypeFor[AIRef]():
		return "AI の ID または Rule[]"
	case reflect.TypeFor[Stats]():
		return "能力名 → 整数"
	case reflect.TypeFor[StatFormulas]():
		return "能力名 → 式"
	case reflect.TypeFor[StatRates]():
		return "能力名 → 倍率"
	}
	switch t.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Struct:
		result := "[" + schemaName(t) + "](#" + strings.ToLower(schemaName(t)) + ")"
		switch t {
		case reflect.TypeFor[Picture](), reflect.TypeFor[CG](), reflect.TypeFor[Say](), reflect.TypeFor[MusicCue](), reflect.TypeFor[Target]():
			result += " または文字列"
		}
		return result
	case reflect.Slice:
		return schemaTypeLabel(t.Elem()) + "[]"
	case reflect.Array:
		return fmt.Sprintf("%s[%d]", schemaTypeLabel(t.Elem()), t.Len())
	case reflect.Map:
		return schemaTypeLabel(t.Key()) + " → " + schemaTypeLabel(t.Elem())
	case reflect.String:
		return "文字列"
	case reflect.Bool:
		return "真偽値"
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return "整数"
	case reflect.Float64:
		return "数値"
	case reflect.Interface:
		return "真偽値・数値・文字列のリテラル"
	}
	panic(t)
}

const schemaIntroduction = `# YAML 定義の契約

Gozen のゲーム作者向けの型・省略値・検証規則です。data.Load(fsys, path) は game.yaml と data/ 以下を読み込みます。
機械可読スキーマは data/schema/ にあります。参照 ID と式の意味は Load で検査してください。
仕様版 0 は互換読込、1 は省略と明示したゼロを区別します。保存の形式版とは別です。
更新: GOZEN_UPDATE_SCHEMA=1 go test ./internal/layer2/gamedata -run '^TestSchemaCoverage$'
`
