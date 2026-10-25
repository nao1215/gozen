package dungeon

import (
	"fmt"
	"math"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

const (
	freeRadius     = .18
	freeBodyHeight = .9
	freeSegment    = .04
	freeTick       = 1.0 / 60
)

// FreeInput is one simulation tick of normalized camera-relative controls from
// play (or a headless test). Forward/Strafe combine without diagonal speedup;
// Turn is clockwise. Values are clamped to [-1,1]; nonfinite input is ignored.
type FreeInput struct{ Forward, Strafe, Turn float64 }

// FreeController owns transient walking acceleration and contact state for play.
// Its zero value is ready. One Update is exactly 1/60 second, independent of Draw.
// The saved pose belongs to State; the controller must not be shared by sessions.
type FreeController struct {
	held, speed                  float64
	fromSpeed, targetSpeed, ramp float64
	forward, strafe              float64
	contact                      string
	state                        *rpg.State
	pose                         *rpg.SpacePose
	last                         rpg.SpacePose
}

// Reset stops motion when play opens blocking UI or loses focus. It does not
// move the party or discard saved fractional distance. Contact stays armed until
// release or separation, so closing dialog while holding does not retrigger it.
func (c *FreeController) Reset() {
	c.held, c.speed, c.forward, c.strafe = 0, 0, 0, 0
	c.fromSpeed, c.targetSpeed, c.ramp = 0, 0, 0
}

// Update advances a supported opt-in space. dispatch executes entry/contact/
// encounter commands synchronously and returns false for blocking UI or transfer.
// Position replacement also stops the old path, even if dispatch returns true.
// A nil dispatcher or unsupported location leaves state unchanged.
func (c *FreeController) Update(db *data.DB, st *rpg.State, in FreeInput, dispatch func([]data.Command) bool) {
	if st == nil || dispatch == nil {
		c.Reset()
		return
	}
	loc := db.Locations[st.Location]
	if loc == nil || loc.Dungeon == nil || loc.Dungeon.Space == nil || loc.Dungeon.Space.FreeMove == nil {
		c.Reset()
		return
	}
	d := loc.Dungeon
	p := ensureFreePose(db, st)
	if c.state != st || c.pose != p || c.last != *p {
		if _, fits := freeFits(d, st, st.Pos, p.X, p.Z); !fits {
			st.SpacePose = nil
			p = ensureFreePose(db, st)
			if _, fits = freeFits(d, st, st.Pos, p.X, p.Z); !fits {
				Place(db, st, st.Location, "", "")
				p = ensureFreePose(db, st)
			}
		}
		c.Reset()
		c.contact = ""
		c.state = st
		c.pose = p
	}
	defer func() {
		if st.SpacePose == p {
			c.last = *p
		}
	}()
	finite := func(v float64) float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return max(-1, min(1, v))
	}
	in.Forward, in.Strafe, in.Turn = finite(in.Forward), finite(in.Strafe), finite(in.Turn)
	cfg := d.Space.FreeMove
	if in.Turn != 0 {
		p.Yaw = math.Mod(p.Yaw+in.Turn*cfg.TurnSpeed*math.Pi/180*freeTick+2*math.Pi, 2*math.Pi)
	}
	st.Moved = false
	st.Pos.Facing = freeFacing(p.Yaw, st.Pos.Facing)
	length := math.Hypot(in.Forward, in.Strafe)
	if length == 0 {
		c.held, c.speed, c.forward, c.strafe = 0, 0, 0, 0
		c.contact = ""
		return
	}
	if length > 1 {
		in.Forward /= length
		in.Strafe /= length
	}
	if in.Forward*c.forward+in.Strafe*c.strafe < -.001 {
		c.held, c.speed = 0, 0
		c.contact = ""
	}
	c.forward, c.strafe = in.Forward, in.Strafe
	target := cfg.Speed
	for _, stage := range cfg.Stages {
		if c.held+1e-10 >= stage.After {
			target = stage.Speed
		}
	}
	if c.speed == 0 || *cfg.Acceleration == 0 {
		c.speed = target
		c.targetSpeed = target
		c.fromSpeed = target
		c.ramp = 0
	} else {
		if target != c.targetSpeed {
			c.fromSpeed, c.targetSpeed, c.ramp = c.speed, target, 0
		}
		c.ramp += freeTick
		c.speed = c.fromSpeed + (target-c.fromSpeed)*min(1, c.ramp / *cfg.Acceleration)
	}
	x := (in.Forward*math.Sin(p.Yaw) + in.Strafe*math.Cos(p.Yaw)) * c.speed * freeTick
	z := (-in.Forward*math.Cos(p.Yaw) + in.Strafe*math.Sin(p.Yaw)) * c.speed * freeTick
	moved, stopped := c.move(db, st, d, x, z, dispatch)
	st.Moved = moved
	if stopped || !moved {
		c.held, c.speed = 0, 0
	} else {
		c.held += freeTick
	}
}

func freeFacing(yaw float64, current int) int {
	delta := math.Remainder(yaw-float64(current&3)*math.Pi/2, 2*math.Pi)
	if math.Abs(delta) <= math.Pi/4+.04 {
		return current & 3
	}
	return int(math.Floor(yaw/(math.Pi/2)+.5)+4) & 3
}

func ensureFreePose(db *data.DB, st *rpg.State) *rpg.SpacePose {
	Ensure(db, st)
	p := st.SpacePose
	valid := p != nil
	if valid {
		for _, v := range []float64{p.X, p.Z, p.Yaw, p.Travel, p.Encounter} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
			}
		}
		valid = valid && p.In == st.Location && p.Level == st.Pos.Level && math.Floor(p.X) == float64(st.Pos.X) && math.Floor(p.Z) == float64(st.Pos.Y) && p.Travel >= 0 && p.Travel < 1 && p.Encounter >= 0 && p.Encounter < 1 && freeFacing(p.Yaw, st.Pos.Facing) == st.Pos.Facing
	}
	if !valid {
		st.SpacePose = &rpg.SpacePose{In: st.Location, Level: st.Pos.Level, X: float64(st.Pos.X) + .5, Z: float64(st.Pos.Y) + .5, Yaw: float64(st.Pos.Facing) * math.Pi / 2}
	}
	return st.SpacePose
}

// freeSupport preserves explicit stair connectivity. X then Z is the stable tie
// order for support queries; a point-only side-cell touch is not a tile entry.
func freeSupport(s *data.Space, from rpg.Pos, x, z int) (rpg.Pos, bool) {
	out := from
	for _, axis := range []int{0, 1} {
		delta := x - out.X
		if axis == 1 {
			delta = z - out.Y
		}
		if delta == 0 {
			continue
		}
		if delta < -1 || delta > 1 {
			return out, false
		}
		facing := 1
		if axis == 0 && delta < 0 {
			facing = 3
		}
		if axis == 1 {
			facing = 2
			if delta < 0 {
				facing = 0
			}
		}
		old := out
		old.Facing = facing
		next, ok := spaceStep(s, old)
		if !ok {
			return out, false
		}
		out = next
	}
	out.Facing = from.Facing
	return out, true
}

func freeCell(d *data.Dungeon, st *rpg.State, level, x, z int) Cell {
	copyState := *st
	copyState.Pos.Level = level
	return CellAt(data.DungeonLevel(d, level), &copyState, x, z)
}

// freeFits checks the footprint against nearby cells, including missing support,
// walls on other levels and the underside of floors/stair volumes above the body.
func freeFits(d *data.Dungeon, st *rpg.State, target rpg.Pos, x, z float64) (Cell, bool) {
	feet := data.SpaceHeightAt(d.Space, target.Level, x, z)
	for iz := int(math.Floor(z - freeRadius)); iz <= int(math.Floor(z+freeRadius)); iz++ {
		for ix := int(math.Floor(x - freeRadius)); ix <= int(math.Floor(x+freeRadius)); ix++ {
			nearX := max(float64(ix), min(float64(ix+1), x))
			nearZ := max(float64(iz), min(float64(iz+1), z))
			if math.Hypot(x-nearX, z-nearZ) >= freeRadius-1e-9 {
				continue
			}
			support, ok := freeSupport(d.Space, target, ix, iz)
			cell := freeCell(d, st, support.Level, ix, iz)
			if !ok || cell.Solid {
				return cell, false
			}
			for level := range d.Space.Levels {
				if level == support.Level {
					continue
				}
				other := freeCell(d, st, level, ix, iz)
				if other.Char == data.CellNone {
					continue
				}
				bottom := float64(level) * data.SpaceStoryHeight
				top := bottom
				if other.Solid {
					top += data.SpaceStoryHeight
				}
				if bottom-.15 < feet+freeBodyHeight-1e-8 && top > feet+1e-8 {
					return other, false
				}
			}
		}
	}
	return Cell{}, true
}

func (c *FreeController) send(st *rpg.State, cmds []data.Command, dispatch func([]data.Command) bool) bool {
	if len(cmds) == 0 {
		return true
	}
	pose := st.SpacePose
	before := *pose
	pos := st.Pos
	return dispatch(cmds) && st.Location == before.In && st.SpacePose == pose && *pose == before && st.Pos == pos
}

// move bounds collision queries and splits each candidate at every crossed grid
// boundary, so even a long headless segment cannot skip a trigger. Exact corner
// ties enter only the diagonal cell; X then Z resolves support connectivity.
func (c *FreeController) move(db *data.DB, st *rpg.State, d *data.Dungeon, x, z float64, dispatch func([]data.Command) bool) (bool, bool) {
	n := max(1, int(math.Ceil(math.Hypot(x, z)/freeSegment)))
	x /= float64(n)
	z /= float64(n)
	moved := false
	for range n {
		beforeX, beforeZ := st.SpacePose.X, st.SpacePose.Z
		ok, stop, blocked := c.segment(db, st, d, x, z, dispatch)
		if stop {
			return true, true
		}
		if !ok {
			// Slide along one free axis. Resolving both separately would make diagonal
			// travel consume a Manhattan distance budget even in open space.
			if x != 0 {
				ok, stop, _ = c.segment(db, st, d, x, 0, dispatch)
			}
			if !ok && !stop && z != 0 {
				ok, stop, _ = c.segment(db, st, d, 0, z, dispatch)
			}
			if stop {
				return true, true
			}
			if !ok {
				key := fmt.Sprintf("%s/%d/%d/%d", st.Location, st.Pos.Level, blocked.X, blocked.Y)
				if key != c.contact {
					c.contact = key
					if !c.send(st, trigger(st, blocked, "bump"), dispatch) {
						return moved, true
					}
				}
			}
		}
		if st.SpacePose.X != beforeX || st.SpacePose.Z != beforeZ {
			moved = true
			if ok {
				c.contact = ""
			}
		}
	}
	return moved, false
}

func (c *FreeController) segment(db *data.DB, st *rpg.State, d *data.Dungeon, dx, dz float64, dispatch func([]data.Command) bool) (bool, bool, Cell) {
	moved := false
	for math.Hypot(dx, dz) > 1e-10 {
		p := st.SpacePose
		fraction := 1.0
		for _, axis := range [][2]float64{{p.X, dx}, {p.Z, dz}} {
			if axis[1] == 0 {
				continue
			}
			edge := math.Floor(axis[0]) + 1
			if axis[1] < 0 {
				edge = math.Floor(axis[0])
				if math.Abs(axis[0]-edge) < 1e-10 {
					edge--
				}
			}
			f := (edge - axis[0]) / axis[1]
			if f >= 0 && f < fraction {
				fraction = min(1, f+1e-8)
			}
		}
		x, z := p.X+dx*fraction, p.Z+dz*fraction
		target, ok := freeSupport(d.Space, st.Pos, int(math.Floor(x)), int(math.Floor(z)))
		if !ok {
			return moved, false, Cell{X: target.X, Y: target.Y, Solid: true}
		}
		if blocked, fits := freeFits(d, st, target, x, z); !fits {
			return moved, false, blocked
		}
		entered := target.X != st.Pos.X || target.Y != st.Pos.Y || target.Level != st.Pos.Level
		distance := math.Hypot(x-p.X, z-p.Z)
		eligible := freeEncounterEligible(db, st)
		p.X, p.Z, p.Level = x, z, target.Level
		st.Pos = target
		st.Moved = true
		moved = true
		p.Travel += distance
		for p.Travel+1e-10 >= 1 {
			p.Travel = max(0, p.Travel-1)
			st.Steps++
		}
		// Accumulate distance in the area actually traversed. Zero/invalid rates
		// clear the encounter residual, preventing a free roll upon area reentry.
		eligible = eligible && freeEncounterEligible(db, st)
		if eligible {
			p.Encounter += distance
		} else {
			p.Encounter = 0
		}
		roll := p.Encounter+1e-10 >= 1
		if roll {
			p.Encounter = max(0, p.Encounter-1)
		}
		if entered {
			Reveal(db, st)
			if !c.send(st, complete(Of(db, st), st), dispatch) {
				return true, true, Cell{}
			}
			if !c.send(st, trigger(st, freeCell(d, st, target.Level, target.X, target.Y), "step"), dispatch) {
				return true, true, Cell{}
			}
		}
		if !freeEncounterEligible(db, st) {
			p.Encounter = 0
			roll = false
		}
		if roll {
			if !c.send(st, encounter(db, st), dispatch) {
				return true, true, Cell{}
			}
		}
		dx *= 1 - fraction
		dz *= 1 - fraction
	}
	return moved, false, Cell{}
}

func freeEncounterEligible(db *data.DB, st *rpg.State) bool {
	if rate := db.Locations[st.Location].Encounters.Rate; rate.Set() && !st.Calm {
		value, err := rate.Number(st.Env())
		return err == nil && value > 0
	}
	return false
}

// freeFront traverses exact cell boundaries, with X first on corner ties just
// like terrain rays. Sampling at fixed distances can miss a thin wall crossing.
// Interaction remains on the active support level and within 1.25 cell units.
func freeFront(db *data.DB, st *rpg.State) Cell {
	d := db.Locations[st.Location].Dungeon
	p := st.SpacePose
	vx, vz := math.Sin(p.Yaw), -math.Cos(p.Yaw)
	x, z := st.Pos.X, st.Pos.Y
	axis := func(pos, velocity float64) (int, float64, float64) {
		if math.Abs(velocity) < 1e-12 {
			return 1, math.Inf(1), math.Inf(1)
		}
		step, edge := 1, math.Floor(pos)+1
		if velocity < 0 {
			step, edge = -1, math.Floor(pos)
		}
		return step, (edge - pos) / velocity, 1 / math.Abs(velocity)
	}
	sx, tx, dx := axis(p.X, vx)
	sz, tz, dz := axis(p.Z, vz)
	eye := data.SpaceHeightAt(d.Space, p.Level, p.X, p.Z) + .65
	for range 8 {
		distance := tx
		if tx <= tz+1e-10 {
			x += sx
			tx += dx
		} else {
			distance = tz
			z += sz
			tz += dz
		}
		if distance > 1.25+1e-10 {
			return Cell{}
		}
		// Upper walls and horizontal slabs can occlude a ray from a stair even
		// before the logical support changes to their level.
		for level := range d.Space.Levels {
			if level == p.Level {
				continue
			}
			other := freeCell(d, st, level, x, z)
			if other.Char == data.CellNone {
				continue
			}
			base := float64(level) * data.SpaceStoryHeight
			if (other.Solid && eye >= base && eye < base+data.SpaceStoryHeight) || (eye >= base-.15 && eye <= base) {
				return Cell{Solid: true}
			}
		}
		cell := freeCell(d, st, p.Level, x, z)
		base := float64(p.Level) * data.SpaceStoryHeight
		if eye >= base && eye < base+data.SpaceStoryHeight {
			if cell.Tile != nil && Lookable(cell.Tile, cell.Solid) {
				return cell
			}
			if cell.Solid {
				return cell
			}
		}
	}
	return Cell{}
}
