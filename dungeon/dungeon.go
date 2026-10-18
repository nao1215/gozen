// Package dungeon は独自の探索操作と地図の観測を画面なしで組み立てる入口を提供する。
// DB と State は呼出側が所有し、同時に複数の goroutine から操作しない。
// State は同じ DB から rpg.New または rpg.Parse で作り、対象の迷宮を現在地にする。
// CellAt と SlideEnd にはその迷宮の nil でない定義を渡す。
// イベント命令を返す操作は、返された命令を event または headless で実行する。
package dungeon

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	core "github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/rpg"
)

// Cell は現在のマスの位置・地形・通行可否。Tile は借用した定義で変更しない。
type Cell = core.Cell

// Of は現在の階の定義を返す。返す定義は借用し、呼出側で変更しない。
func Of(db *data.DB, st *rpg.State) *data.Dungeon {
	return core.Of(db, st)
}

// CellAt は条件と状態を反映したマスを返す。Tile は借用する。
func CellAt(d *data.Dungeon, st *rpg.State, x, y int) Cell {
	return core.CellAt(d, st, x, y)
}

// Dive は保存状態の seed と生成器の設定から指定の階を用意し、座標を配置する。
// st.Location は先に loc にする。同じ状態からは同じ地図を使う。
func Dive(db *data.DB, st *rpg.State, loc string, floor int) {
	core.DiveAt(db, st, loc, floor, "", "")
}

// Place は指定の場所・目印・向きへ座標を配置し、到着したマスを記録する。
// st.Location は呼出側が loc に設定してから呼ぶ。
func Place(db *data.DB, st *rpg.State, loc, at, facing string) {
	core.Place(db, st, loc, at, facing)
}

// Turn は向きを四方向で n 段回す。地図・イベントは進めない。
func Turn(st *rpg.State, n int) { core.Turn(st, n) }

// Forward は一歩進み、実行するイベント命令を返す。呼出側は返された命令を一度実行する。
func Forward(db *data.DB, st *rpg.State) (bool, []data.Command) {
	return core.Forward(db, st)
}

// Look は正面の操作で生じる命令を返す。呼出側は返された命令を一度実行する。
func Look(db *data.DB, st *rpg.State) []data.Command {
	return core.Look(db, st)
}

// Front は現在の正面のマスを返す。
func Front(db *data.DB, st *rpg.State) Cell {
	return core.Front(db, st)
}

// Seen は指定した場所のマスが訪問済みかを返す。
func Seen(db *data.DB, st *rpg.State, loc string, x, y int) bool {
	return core.Seen(db, st, loc, x, y)
}

// Shown は現在の階で表示できるマスを行優先の新しい配列で返す。
func Shown(db *data.DB, st *rpg.State) []bool {
	return core.Shown(db, st)
}

// Spent は指定したマスの一回限りの操作が完了したかを返す。
func Spent(st *rpg.State, loc string, x, y int) bool {
	return core.Spent(st, loc, x, y)
}

// Completed は指定した場所の現在の階が完了済みかを返す。
func Completed(st *rpg.State, loc string) bool {
	return core.Completed(st, loc)
}

// Walked は全域の到達を判定して状態に記録し、初回の完了命令を返す。
func Walked(db *data.DB, st *rpg.State) []data.Command {
	return core.Walked(db, st)
}

// Hunters は条件付きの移動敵を更新し、出現時の命令を返す。
func Hunters(db *data.DB, st *rpg.State) []data.Command {
	return core.Hunters(db, st)
}

// Trigger は nil でないタイルの操作方法（step・bump・look）を返す。
func Trigger(t *data.Tile, solid bool) string {
	return core.Trigger(t, solid)
}

// Slides はマスに移送設定があるかを返す。
func Slides(c Cell) bool {
	return core.Slides(c)
}

// SlideEnd は移送後の位置・向き・経路を計算する。状態は変更しない。
func SlideEnd(d *data.Dungeon, st *rpg.State, x, y, moving int) (ex, ey, facing int, way [][2]int) {
	return core.SlideEnd(d, st, x, y, moving)
}

// BattleMusic は階と敵グループの指定から戦闘曲を選ぶ。
func BattleMusic(db *data.DB, st *rpg.State, troop *data.Troop) data.MusicCue {
	return core.BattleMusic(db, st, troop)
}

// Next は目印までの次の操作を返す。操作は here・left・right・look・forward。経路が無い場合は false。
func Next(db *data.DB, st *rpg.State, goal rune) (string, bool) {
	return core.Next(db, st, goal)
}
