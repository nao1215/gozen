// Package casual runs authored real-time games without presentation or storage.
// The player and headless tests advance an attempt with one normalized input per tick.
package casual

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Config identifies an attempt for event records. Zero is a valid seed. Authored
// rows need no random draws; keeping the seed does not consume a game's RNG.
type Config struct{ Seed uint64 }

// Input is one logical 60 Hz tick supplied by the player or a headless driver.
// Steer is in [-1,1]; nonfinite values become zero. Action is a one-tick press.
type Input struct {
	Steer         float64
	Boost, Action bool
}

// Event describes one rule transition for presentation and deterministic traces.
// Kind is pickup, life, stock, miss, near_wall, crash, failed, course,
// stage_clear, completed, retry, action or cancelled. Tile is the authored key
// for a collectible (empty for an earned life). Stage identifies the stage
// involved, zero-based; course and stage_clear identify the stage just finished.
// Events are ordered and are copied into Snapshot after Step, Retry or Cancel.
type Event struct {
	Kind, Tile           string
	Value, Streak, Stage int
}

// Snapshot is an owned observation for drawing and outcome handling. Rows and
// Ahead contain authored tile keys; picked items and cleared walls become '.'.
// Stage is zero-based, StageRow counts generated rows, Distance counts all
// forward rows including repeated travel after retry, and Ticks excludes pauses
// and action freezes. Status is running, missed, completed, failed or cancelled.
type Snapshot struct {
	Events                                                     []Event
	CompletedStages, BestCompletedStages, Misses, LifeProgress int
	// Diagnostic explains an engine safety-budget failure, otherwise empty.
	Diagnostic                                          string
	Seed                                                uint64
	Status                                              string
	X, Scroll, Boost                                    float64
	Rows                                                []string
	Ahead                                               string
	Stage, StageRow, Distance, Score, Collected, Streak int
	Lives, Stock, Ticks, ClearTicks                     int
	Paused, Finishing                                   bool
}

// Runner owns one attempt and is not safe for concurrent use. New copies the
// definition, so editing author data cannot mutate a running attempt.
type Runner struct {
	events                   []Event
	bestCompleted, misses    int
	def                      data.RunnerDefinition
	seed                     uint64
	diagnostic               string
	status                   string
	rows                     [][]byte
	ahead                    []byte
	course                   []string
	ends                     []int
	taken                    map[int]bool
	progress, distance       int
	x, scroll, boost         float64
	score, collected, streak int
	lives, stock, towardLife int
	ticks, clearTicks, held  int
	direction                float64
	behind, paused           bool
}

// New validates and copies a runner definition for presentation or headless play.
// The attempt starts running; ready/result screens belong to the frontend.
func New(def *data.RunnerDefinition, cfg Config) (*Runner, error) {
	if def == nil {
		return nil, errors.New("runner definition is nil")
	}
	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	r := &Runner{
		def: *def, seed: cfg.Seed, status: "running", x: def.StartX,
		boost: 1, lives: def.Lives, stock: def.Stock, taken: make(map[int]bool),
	}
	r.def.Tiles = maps.Clone(def.Tiles)
	r.def.Stages = slices.Clone(def.Stages)
	for i := range r.def.Stages {
		r.def.Stages[i].Rows = slices.Clone(def.Stages[i].Rows)
		r.course = append(r.course, def.Stages[i].Rows...)
		r.ends = append(r.ends, len(r.course))
	}
	r.rows = make([][]byte, def.ViewRows)
	r.rebuild()
	return r, nil
}

// Snapshot returns an independent view. A caller may retain or edit the slices
// without affecting subsequent movement or collision checks.
func (r *Runner) Snapshot() Snapshot {
	stage, row := r.stage()
	s := Snapshot{
		Events: slices.Clone(r.events), CompletedStages: r.completedStages(), BestCompletedStages: r.bestCompleted, Misses: r.misses, LifeProgress: r.towardLife,
		Seed: r.seed, Status: r.status, Diagnostic: r.diagnostic, X: r.x, Scroll: r.scroll, Boost: r.boost,
		Stage: stage, StageRow: row, Distance: r.distance, Score: r.score,
		Collected: r.collected, Streak: r.streak, Lives: r.lives, Stock: r.stock,
		Ticks: r.ticks, ClearTicks: r.clearTicks, Paused: r.paused,
		Finishing: r.progress >= len(r.course), Ahead: string(r.ahead), Rows: make([]string, len(r.rows)),
	}
	for i, row := range r.rows {
		s.Rows[i] = string(row)
	}
	return s
}

// Step advances exactly one logical tick. Steering precedes the limited action,
// then boost and scrolling; crashing while steering therefore prevents action.
// Terminal attempts, missed attempts and paused attempts do not advance.
func (r *Runner) Step(in Input) {
	if r.status != "running" || r.paused {
		return
	}
	r.events = r.events[:0]
	steer := in.Steer
	if math.IsNaN(steer) || math.IsInf(steer, 0) {
		steer = 0
	}
	steer = max(-1, min(1, steer))
	dir := math.Copysign(1, steer)
	if steer == 0 {
		dir = 0
	}
	if dir != r.direction {
		r.direction, r.held = dir, 0
	}
	if r.clearTicks > 0 {
		r.clearTicks--
		return
	}
	if steer != 0 {
		r.held++
		top := r.steerSpeed()
		speed := top
		if r.def.RampTicks > 0 {
			speed = r.def.SteerStart + (top-r.def.SteerStart)*min(1, float64(r.held-1)/float64(r.def.RampTicks))
		}
		r.move(steer * speed / 60)
	}
	if r.status != "running" {
		return
	}
	if in.Action && r.stock > 0 {
		r.clear()
		if r.clearTicks > 0 {
			return
		}
	}
	if r.ticks >= 24*60*60*60 {
		r.status, r.diagnostic = "failed", "runner exceeded the 24-hour simulation budget"
		r.emit("failed", "", 0)
		return
	}
	r.ticks++
	if in.Boost {
		r.boost = min(r.def.BoostMax, r.boost+r.def.BoostRate/60)
	}
	before := r.scroll
	next := before + r.speed()*r.boost/60
	if next >= 1 {
		// At the maximum authored speed a tick may cross both the half-row
		// contact and row boundary. Resolve the old contact before shifting.
		if before < .5 {
			r.scroll = .5
			r.reachHalf()
			if r.status != "running" {
				return
			}
		}
		r.scroll = next - 1
		r.advance()
		before = 0
	} else {
		r.scroll = next
	}
	if r.status == "running" && before < .5 && r.scroll >= .5 {
		r.reachHalf()
	}
}

// Pause freezes an attempt and clears transient steering acceleration. The
// frontend cancels its captured input before calling this on focus loss.
func (r *Runner) Pause(on bool) {
	r.paused = on
	r.direction, r.held = 0, 0
}

// Cancel produces one terminal cancelled outcome. Calling it again or after a
// completed/failed result cannot replace that result.
func (r *Runner) Cancel() {
	if r.status == "running" || r.status == "missed" {
		r.events = r.events[:0]
		r.status = "cancelled"
		r.emit("cancelled", "", 0)
		r.direction, r.held = 0, 0
	}
}

// GiveUp turns a pending collision retry into a failed result. The player's
// retry screen uses it when a player declines another life. It does not replace
// an existing terminal result or interrupt a live run; Cancel handles that exit.
func (r *Runner) GiveUp() {
	if r.status == "missed" {
		r.events = r.events[:0]
		r.status = "failed"
		r.emit("failed", "", 0)
	}
}

// Retry spends one spare life after a collision. It restores the authored road
// RetryRows earlier (or its beginning when RetryRows is zero), but keeps picked
// item identities, score and stocks. Cleared walls are rebuilt. No terminal
// outcome can be retried; the frontend starts a new attempt after one.
func (r *Runner) Retry() bool {
	if r.status != "missed" || r.lives <= 0 {
		return false
	}
	r.events = r.events[:0]
	r.lives--
	r.status = "running"
	if r.def.RetryRows == 0 {
		r.progress = 0
	} else {
		r.progress = max(0, r.progress-r.def.RetryRows)
	}
	r.scroll, r.boost = 0, 1
	r.held, r.direction, r.streak, r.clearTicks = 0, 0, 0, 0
	r.behind, r.paused = true, false
	r.rebuild()
	r.x = r.openColumn(r.x)
	r.emit("retry", "", 0)
	r.pick()
	return true
}

func (r *Runner) completedStages() int {
	row := r.progress - 1 - r.def.PlayerRow
	completed := 0
	for _, end := range r.ends {
		if row <= end {
			break
		}
		completed++
	}
	return completed
}

func (r *Runner) emit(kind, tile string, value int) {
	stage, _ := r.stage()
	e := Event{Kind: kind, Tile: tile, Value: value, Stage: stage}
	if kind == "pickup" {
		e.Streak = r.streak
	}
	r.events = append(r.events, e)
}

func (r *Runner) stage() (int, int) {
	start := 0
	for i, end := range r.ends {
		if r.progress < end {
			return i, r.progress - start
		}
		start = end
	}
	return len(r.ends) - 1, r.progress - len(r.course)
}

func (r *Runner) speed() float64 {
	if r.progress >= len(r.course) && r.def.FinishSpeed > 0 {
		return r.def.FinishSpeed
	}
	stage, _ := r.stage()
	if v := r.def.Stages[stage].Speed; v > 0 {
		return v
	}
	return r.def.Speed
}

func (r *Runner) steerSpeed() float64 {
	if r.progress >= len(r.course) && r.def.FinishSteerSpeed > 0 {
		return r.def.FinishSteerSpeed
	}
	stage, _ := r.stage()
	if v := r.def.Stages[stage].SteerSpeed; v > 0 {
		return v
	}
	return r.def.SteerSpeed
}

func (r *Runner) rowAt(id int) []byte {
	row := strings.Repeat(".", r.def.Width)
	if id == 0 && r.def.Ahead != "" {
		row = r.def.Ahead
	} else if id > 0 && id <= len(r.course) {
		row = r.course[id-1]
	}
	b := []byte(row)
	if id >= 0 {
		for x := range b {
			if r.taken[id*r.def.Width+x] {
				b[x] = '.'
			}
		}
	}
	return b
}

func (r *Runner) rebuild() {
	for y := range r.rows {
		r.rows[y] = r.rowAt(r.progress - 1 - y)
	}
	r.ahead = r.rowAt(r.progress)
}

func (r *Runner) span(x float64) (int, int) {
	return max(0, int(x-r.def.Radius)), min(r.def.Width-1, int(x+r.def.Radius-1e-9))
}

func (r *Runner) tile(key byte) data.RunnerTile { return r.def.Tiles[string(key)] }

func (r *Runner) blocked(x float64, y int) bool {
	lo, hi := r.span(x)
	for c := lo; c <= hi; c++ {
		if r.tile(r.rows[y][c]).Kind == "wall" {
			return true
		}
	}
	return false
}

func (r *Runner) move(dx float64) {
	y := r.def.PlayerRow
	if r.behind && y+1 < len(r.rows) {
		y++
	}
	for dx != 0 {
		d := math.Copysign(min(math.Abs(dx), .05), dx)
		next := max(r.def.Radius, min(float64(r.def.Width)-r.def.Radius, r.x+d))
		if next == r.x {
			break
		}
		r.x = next
		if r.blocked(next, y) {
			r.crash()
			return
		}
		dx -= d
	}
	r.pick()
}

func (r *Runner) advance() {
	r.passBy()
	oldStage, _ := r.stage()
	copy(r.rows[1:], r.rows[:len(r.rows)-1])
	r.rows[0] = r.ahead
	r.progress++
	r.distance++
	r.ahead = r.rowAt(r.progress)
	r.behind = true
	if r.progress == r.ends[oldStage] {
		r.events = append(r.events, Event{Kind: "course", Stage: oldStage})
		if r.def.Stages[oldStage].Clear {
			r.events = append(r.events, Event{Kind: "stage_clear", Stage: oldStage})
		}
	}
	complete := r.progress >= len(r.course)+r.def.FinishRows
	if complete {
		r.emit("completed", "", 0)
	}
	r.pick()
	lo, hi := r.span(r.x)
	if lo > 0 && hi < r.def.Width-1 && r.tile(r.rows[r.def.PlayerRow][lo-1]).Kind == "wall" && r.tile(r.rows[r.def.PlayerRow][hi+1]).Kind == "wall" {
		r.emit("near_wall", "", 0)
	}
	r.bestCompleted = max(r.bestCompleted, r.completedStages())
	if r.status == "running" && complete {
		r.status = "completed"
	}
}

func (r *Runner) reachHalf() {
	r.passBy()
	if r.blocked(r.x, r.def.PlayerRow) {
		r.crash()
	}
}

func (r *Runner) passBy() {
	if !r.behind || r.def.PlayerRow+1 >= len(r.rows) {
		return
	}
	r.behind = false
	for _, key := range r.rows[r.def.PlayerRow+1] {
		switch r.tile(key).Kind {
		case "pickup":
			r.streak = 0
			r.emit("miss", string(key), 0)
		case "life":
			r.emit("miss", string(key), 0)
		}
	}
}

func (r *Runner) pick() {
	lo, hi := r.span(r.x)
	for x := lo; x <= hi; x++ {
		r.pickAt(r.def.PlayerRow, x)
		if r.behind && r.def.PlayerRow+1 < len(r.rows) {
			r.pickAt(r.def.PlayerRow+1, x)
		}
	}
}

func (r *Runner) pickAt(y, x int) {
	if r.status != "running" {
		return
	}
	tile := r.tile(r.rows[y][x])
	if tile.Value > 1_000_000_000-r.score {
		r.status, r.diagnostic = "failed", "runner score exceeded the supported range"
		r.emit("failed", "", 0)
		return
	}
	switch tile.Kind {
	case "pickup":
		r.collected++
		r.streak++
		r.emit("pickup", string(r.rows[y][x]), tile.Value)
		r.towardLife++
		if r.def.LifeEvery > 0 && r.towardLife >= r.def.LifeEvery {
			r.towardLife -= r.def.LifeEvery
			if r.lives < r.def.MaxLives {
				r.lives++
				r.emit("life", "", 0)
			}
		}
	case "life":
		r.lives = min(r.def.MaxLives, r.lives+1)
		r.emit("life", string(r.rows[y][x]), tile.Value)
	case "stock":
		if r.stock < r.def.MaxStock {
			r.stock++
			r.emit("stock", string(r.rows[y][x]), tile.Value)
		}
	default:
		return
	}
	r.score += tile.Value
	r.taken[(r.progress-1-y)*r.def.Width+x] = true
	r.rows[y][x] = '.'
}

func (r *Runner) crash() {
	r.streak = 0
	r.misses++
	r.emit("crash", "", 0)
	r.status = "missed"
	if r.lives <= 0 {
		r.status = "failed"
		r.emit("failed", "", 0)
	}
}

func (r *Runner) clear() {
	r.stock--
	clearRow := func(row []byte) {
		for x, key := range row {
			if r.tile(key).Kind == "wall" {
				row[x] = '.'
			}
		}
	}
	for _, row := range r.rows {
		clearRow(row)
	}
	clearRow(r.ahead)
	r.clearTicks = r.def.ClearTicks
	r.emit("action", "", 0)
}

func (r *Runner) openColumn(x float64) float64 {
	best, longest, nearest := r.def.Width/2, -1, math.MaxFloat64
	for col := range r.def.Width {
		run := 0
		for y := min(r.def.PlayerRow+1, len(r.rows)-1); y >= 0 && run < 8 && r.tile(r.rows[y][col]).Kind != "wall"; y-- {
			run++
		}
		distance := math.Abs(float64(col) + .5 - x)
		if run > longest || run == longest && distance < nearest {
			best, longest, nearest = col, run, distance
		}
	}
	return float64(best) + .5
}
