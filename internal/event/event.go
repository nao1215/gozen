// Package event runs the commands of events (see data.Command): it changes the game state
// itself and asks the screen for what needs one (a line to read, a choice, a battle, a
// shop). The screen calls Next, shows the Request it gets, and calls Answer (for a choice
// or a battle) before the next Next.
package event

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/safeint"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/scope"
	"github.com/nao1215/gozen/internal/rpg"
)

// Host applies immediate effects for a game frontend or a simulation.
// Handle is called synchronously; the host owns any resources it uses.
type Host interface{ Handle(Effect) }

// HostFunc adapts a host callback without passing an upper-layer concrete type
// into the event runtime. A nil callback discards immediate effects.
type HostFunc func(Effect)

// Handle delivers the effect synchronously.
func (f HostFunc) Handle(e Effect) {
	if f != nil {
		f(e)
	}
}

// Effect is an immediate presentation or movement effect. It does not pause Next.
type Effect interface{ isEffect() }

// MoveEffect asks the host to update the location after dungeon placement.
type MoveEffect struct{ Location string }

// MusicEffect selects a music cue without requiring an audio backend.
type MusicEffect struct{ Cue data.MusicCue }

// MoodEffect changes the music mood.
type MoodEffect struct{ Name string }

// ImageEffect sets the current picture; an empty file clears it.
type ImageEffect struct{ Picture data.CG }

// SoundEffect requests a named sound.
type SoundEffect struct{ Name string }

// VisualEffect requests a named visual effect: a word of the whole screen (shake, flash,
// fade) or a reaction of a portrait; Cue holds the rest of a map form (ADR 0028).
type VisualEffect struct {
	Name string
	Cue  data.EffectCue
}

// StandEffect brings a portrait onto the stage of the novel window or takes it off (ADR 0028).
type StandEffect struct {
	// Who is the actor (or the name), "" with None.
	Who string
	// Face is the picture under assets/ (the actor's face resolved), "" when leaving.
	Face string
	// At is where it stands, a fraction of the screen's width.
	At float64
	// Enter is fade or slide; Leave (fade or slide) takes it off; None takes everyone off.
	Enter, Leave string
	None         bool
	// Default: no face was written (Face is the actor's own); a portrait already standing
	// keeps its face.
	Default bool
}

func (MoveEffect) isEffect()   {}
func (MusicEffect) isEffect()  {}
func (MoodEffect) isEffect()   {}
func (ImageEffect) isEffect()  {}
func (SoundEffect) isEffect()  {}
func (VisualEffect) isEffect() {}
func (StandEffect) isEffect()  {}
func (VoiceEffect) isEffect()  {}

// CampaignEffect tells that a campaign's order was given (Kind order: its id, general,
// target and result) or that a turn ended (Kind end_turn: the turn that begins).
type CampaignEffect struct {
	Kind, Order, General, Target, Result string
	Turn                                 int
}

func (CampaignEffect) isEffect() {}

// VoiceEffect is the voice of the line that comes next: File under assets/, Who the speaker.
type VoiceEffect struct{ File, Who string }

// Request is something the screen must show before the event goes on.
type Request interface{ isRequest() }

// Say is a line to read.
type Say struct {
	Name  string // "" for narration
	Face  string // a picture under assets/ ("" for none)
	Text  string
	Right bool // the picture stands on the right
	Actor string
	// Voice is the line's voice, a file under assets/ ("" for none; ADR 0028).
	Voice string
	// Key is the key of the line's source (SourceKey of who and text as written, before
	// {expressions} and translation): the screen remembers read lines by it (ADR 0026).
	Key uint64
	// Instead is the replacement line used when the speaker is down during battle (instead of data.Say).
	Instead []Say
	// text and who are what Text and Name were made from, so Reword can show the line
	// again in another language (the zero Phrase keeps the text as it is).
	text, who data.Phrase
}

// Reword is the line in the language the game shows now (ADR 0027), filled in with the
// values its {expressions} had when it was first shown.
func (s Say) Reword(db *data.DB) Say {
	if s.text.Source != "" {
		s.Text = data.Again(db, s.text)
	}
	if a, ok := db.Actors[s.Actor]; ok {
		s.Name = data.Again(db, data.Phrase{Source: a.Name, Values: s.who.Values})
	} else if s.who.Source != "" {
		s.Name = data.Again(db, s.who)
	}
	if len(s.Instead) > 0 {
		alt := make([]Say, len(s.Instead))
		for i, l := range s.Instead {
			alt[i] = l.Reword(db)
		}
		s.Instead = alt
	}
	return s
}

// Choice is a question: the screen answers with the index of the option taken.
type Choice struct {
	IDs     []string // identifiers in the same order as the displayed choices (empty if unspecified)
	Prompt  string
	Options []string
	// Keys are the keys of the options' sources (ChoiceKey of the prompt and the label as
	// written), in the same order: the screen marks an option taken before (ADR 0026).
	Keys []uint64
	// prompt and labels are what Prompt and Options were made from (labels is empty, or
	// has one phrase per option), so Reword can ask again in another language.
	prompt data.Phrase
	labels []data.Phrase
}

// Question is the prompt as a line to read, which Reword shows again in another language.
func (c Choice) Question() Say { return Say{Text: c.Prompt, text: c.prompt} }

// Reword is the question in the language the game shows now (ADR 0027).
func (c Choice) Reword(db *data.DB) Choice {
	if c.prompt.Source != "" {
		c.Prompt = data.Again(db, c.prompt)
	}
	if len(c.labels) == len(c.Options) && len(c.labels) > 0 {
		opts := make([]string, len(c.Options))
		for i, p := range c.labels {
			opts[i] = data.Again(db, p)
		}
		c.Options = opts
	}
	return c
}

// SourceKey is the key of a line's source: a 64-bit FNV-1a hash of who and text as written in
// the YAML, joined by a zero byte. A line keeps its key whatever its {expressions} hold and
// in whatever language it is shown (ADR 0026).
func SourceKey(who, text string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(who))
	h.Write([]byte{0})
	h.Write([]byte(text))
	return h.Sum64()
}

// ChoiceKey is the key of an option's source: "choice", the prompt and the label as written,
// joined by zero bytes.
func ChoiceKey(prompt, label string) uint64 {
	return SourceKey("choice\x00"+prompt, label)
}

// Battle is a battle to fight: the screen answers with its battle.Result.
type Battle struct {
	Troop   string
	CanLose bool // losing does not end the game
	// Fighters fight instead of the party (none: the party).
	Fighters []string
	// NoRetry: losing does not offer to fight again from just before (retry: false).
	NoRetry bool
}

// Shop opens a shop.
type Shop struct{ ID string }

// Wait pauses.
type Wait struct {
	Seconds float64
	// NoSkip holds even in skip mode (wait: {skip: false}).
	NoSkip bool
	// Sound waits for the end of that sound effect (sfx: {wait: true}) instead of Seconds.
	Sound string
}

// Save opens the save screen.
type Save struct{}

// Open opens one of the screens of the game (formation, skills, fusion).
type Open struct{ Screen string }

// Notice is a short notice shown at the top of the screen without stopping (the notice command). The screen shows it and moves on at once (no answer needed).
type Notice struct{ Text string }

// Tap waits for a tap with nothing to read (between the pictures of an image's files).
// If Auto is greater than 0, advance to the next after that many seconds without a button press (image auto; slideshow).
type Tap struct{ Auto float64 }

// Clear ends the game happily.
type Clear struct{}

// GameOver ends the game.
type GameOver struct{}

func (Say) isRequest()      {}
func (Choice) isRequest()   {}
func (Battle) isRequest()   {}
func (Shop) isRequest()     {}
func (Wait) isRequest()     {}
func (Save) isRequest()     {}
func (Open) isRequest()     {}
func (Clear) isRequest()    {}
func (Tap) isRequest()      {}
func (Notice) isRequest()   {}
func (GameOver) isRequest() {}

// ClashScene is a march decided by a clash shown as units (fight: clash_view, ADR 0030):
// the rules have played it and written it back; the screen plays it back (it needs no
// answer).
type ClashScene struct{ Clash *campaign.Clash }

func (ClashScene) isRequest() {}

// SortieScene asks who marches on a march with a clash shown as units or more than one
// general (Runner.Sortie, ADR 0030): the generals who can (Ready, in the written order),
// the most who go (Cap), and the region. March gives the order with them (the leader
// first, the rest with, each one's tactic); the scene closes without it to cancel.
type SortieScene struct {
	Order, Target string
	Ready         []string
	Cap           int
	March         func(generals []string, tactics map[string]string)
}

func (SortieScene) isRequest() {}

// The answers of a battle.
const (
	BattleWon  = 1
	BattleLost = 2
	BattleFled = 3
)

type frame struct {
	cmds         []data.Command
	pc           int
	prepared     bool
	location     string
	checkedAfter bool
}

// Runner runs events.
type Runner struct {
	db    *data.DB
	st    *rpg.State
	host  Host
	stack []frame
	notes []string
	// ups are the notes that tell of a level gained: a sound goes with them when they are shown
	ups map[string]bool
	// waiting for an answer
	options []data.ChoiceOption
	// pick answers a choice made by the engine (stone_choose, stone_wearer) instead of options' do
	pick   func(n int)
	asked  *data.Command // the choice whose answer is waited for
	battle *data.Command
	// Extra is looked up before the state's names (a battle's scope, for troop events).
	Extra        expr.Env
	gameOver     bool
	afterPending bool // after the win/lose branch is settled, run post-processing before the next choice or battle
	// pages are the pictures of an image's files still to show, a tap before each
	pages []data.CG
	// later holds commands that run independently after the current event ends (after_win following a victory). They are not discarded by end.
	later [][]data.Command
	// shown is the last picture shown (restored after asides; not carried across pictures that hide the portrait. If the screen has AsideBlocked, that is used instead)
	shown data.CG
	// Log receives the errors of expressions (nil: they are ignored).
	Log func(error)
	// BlockAside lets a screen suppress inserted events during replay or overlays.
	// When non-nil it takes precedence over Host's optional AsideBlocker.
	// The runner calls it synchronously; it must not advance this runner.
	BlockAside func() bool
	// Sortie asks who marches on a march (more than one general, or a clash shown as
	// units) with one SortieScene after the target instead of a choice per general, when
	// the order does not name its general: the play screen sets it.
	Sortie bool
	// Dev allows set to change the stats and levels of actors (actor.<id>.hp, mp, exp,
	// level, sp): the development mode and tests. Without it such a set is logged and skipped.
	Dev bool
	// UseItem uses an item from the menu (the use command) on the target actor ("" for the
	// first member) and returns the lines to show. Nil logs the command as unsupported.
	UseItem func(item, target string) ([]string, error)
	// Act takes a battle move (the act command: the actor, its skill or item, the target)
	// for the next battle fought on AUTO. Nil logs the command as unsupported.
	Act func(actor, skill, item, target string)
	// events are the events by the first command of their do (countRun).
	events map[*data.Command]string
	// walks are the step and walk commands still to go (see cont).
	walks map[*data.Command]walking
	// last is the last battle's answer and whether the game ended, for the expressions.
	last scope.Last
	// OnStart is told of an event started when nothing was running (before any of it runs):
	// a test player's note keeps the state there, to begin the event again from it.
	OnStart func(cmds []data.Command)
	// optionAt is the place of each option shown in its choice as written (chose.<id> of an
	// option without an id is that number).
	optionAt []int
}

// New is a runner on the state.
func New(db *data.DB, st *rpg.State, host Host) *Runner {
	return &Runner{db: db, st: st, host: host}
}

// Start begins running cmds (after whatever is running).
func (r *Runner) Start(cmds []data.Command) {
	// OnStart keeps the state the event begins from, so it comes before the run is counted:
	// begun again from that state (a save made in the middle of the event, ADR 0025), the event
	// is counted once, not once more for every save and load.
	if len(cmds) > 0 && r.OnStart != nil && !r.Running() {
		r.OnStart(cmds)
	}
	r.countRun(cmds)
	if len(cmds) > 0 {
		r.stack = append([]frame{{cmds: cmds}}, r.stack...)
	}
}

// Running reports whether there is anything left to run.
func (r *Runner) Running() bool {
	return r.gameOver || len(r.stack) > 0 || len(r.notes) > 0 || r.options != nil || r.battle != nil || len(r.later) > 0 || r.afterPending
}

// Asking is the command whose answer is waited for: the choice or the battle just asked
// (nil when none). Its slices are the data's own (shared by copies of the command), so
// they tell one place in the data from another that reads the same.
func (r *Runner) Asking() *data.Command {
	if r.options != nil {
		return r.asked
	}
	return r.battle
}

// Stop drops everything left.
func (r *Runner) Stop() {
	r.afterPending = false
	r.stack, r.notes, r.options, r.battle, r.later, r.pick, r.ups, r.walks = nil, nil, nil, nil, nil, nil, nil, nil
}

// runLater runs cmds separately from the current event (and its end), after the current event finishes.
func (r *Runner) runLater(cmds []data.Command) {
	if len(cmds) > 0 {
		r.later = append(r.later, cmds)
	}
}

// Won is called when a battle is won. It schedules the game.yaml battle.after_win event
// as independent processing after the current event (shared by the screen and headless).
func (r *Runner) Won() {
	if ev := r.db.Events[r.db.Game.Battle.AfterWin]; ev != nil {
		r.runLater(ev.Do)
	}
}

func (r *Runner) env() expr.Env {
	if r.Extra != nil {
		return expr.Chain{r.Extra, scope.Env(r.db, r.st, &r.last), r.st.Env()}
	}
	return expr.Chain{scope.Env(r.db, r.st, &r.last), r.st.Env()}
}

func (r *Runner) logf(err error) {
	if err != nil && r.Log != nil {
		r.Log(err)
	}
}

// Answer answers the last Choice (the option's index) or Battle (BattleWon ...).
func (r *Runner) Answer(n int) {
	switch {
	case r.options != nil:
		if n >= 0 && n < len(r.options) {
			if r.pick != nil {
				r.pick(n)
			} else {
				r.chose(n)
				r.push(r.options[n].Do)
			}
		}
		r.options, r.pick = nil, nil
	case r.battle != nil:
		c := r.battle
		r.battle = nil
		r.last.Result = scope.ResultName(n)
		r.afterPending = r.db.Events[r.db.Game.Battle.After] != nil
		switch n {
		case BattleWon:
			if troop := r.db.Troops[c.Battle]; troop != nil {
				for _, id := range troop.Members {
					key := "bestiary." + id + ".defeated"
					old, _ := expr.ToNumber(r.st.Vars[key])
					r.st.Vars[key] = old + 1
				}
			}
			r.push(c.OnWin)
			r.offerCaptures(c.Battle)
		case BattleLost:
			if c.OnLose == nil {
				r.Stop()
				r.gameOver = true
				return
			}
			r.push(c.OnLose)
		case BattleFled:
			r.push(c.OnRun)
		}
	}
}

// offerCaptures rolls once per defeated recruitable enemy. Each successful roll
// becomes a normal choice, so policies, traces, scripts and the screen agree.
func (r *Runner) offerCaptures(troopID string) {
	troop := r.db.Troops[troopID]
	if troop == nil {
		return
	}
	for _, id := range slices.Backward(troop.Members) {
		e := r.db.Enemies[id]
		if e == nil || e.Recruit == nil || e.Boss {
			continue
		}
		chance := e.Recruit.Chance
		if e.Recruit.BonusItem != "" && r.st.ItemCount(e.Recruit.BonusItem, true) > 0 {
			chance += e.Recruit.Bonus
		}
		if e.Recruit.BonusSkill != "" {
			for _, a := range r.st.Members() {
				if slices.Contains(a.Skills(), e.Recruit.BonusSkill) {
					chance += e.Recruit.Bonus
					break
				}
			}
		}
		if r.st.Rand.Float64() >= min(1, chance) {
			continue
		}
		r.push([]data.Command{
			{Stand: &data.Stand{Who: e.Name, Face: e.Image.File, At: data.StandAt("center"), Enter: "slide"}},
			{Effect: data.EffectCue{Name: "flash"}},
			{Say: &data.Say{Who: e.Name, Face: e.Image.File, Text: e.Name + " rises and looks toward your party."}},
			{Choice: []data.ChoiceOption{{ID: "yes", Label: "Yes", Do: []data.Command{{Capture: id}}}, {ID: "no", Label: "No"}}, Prompt: "Let " + e.Name + " join?"},
			{Stand: &data.Stand{None: true}},
		})
	}
}

// chose remembers the option taken at a choice with an id (chose.<id>; ADR 0025): the
// option's id, or its number as written.
func (r *Runner) chose(n int) {
	if r.asked == nil || r.asked.ID == "" || n >= len(r.optionAt) {
		return
	}
	v := r.options[n].ID
	if v == "" {
		v = strconv.Itoa(r.optionAt[n])
	}
	if r.st.Chose == nil {
		r.st.Chose = map[string]string{}
	}
	r.st.Chose[r.asked.ID] = v
}

func (r *Runner) push(cmds []data.Command) {
	r.countRun(cmds)
	if len(cmds) > 0 {
		r.stack = append(r.stack, frame{cmds: cmds})
	}
}

// Frames are the command lists being run, the outermost first (a call adds one), and the
// index of the next command of each (for a note of where the play is: which event).
func (r *Runner) Frames() (cmds [][]data.Command, pc []int) {
	for _, f := range r.stack {
		cmds = append(cmds, f.cmds)
		pc = append(pc, f.pc)
	}
	return cmds, pc
}

// Next runs commands until one needs the screen, and returns it (nil when the events are
// over).
func (r *Runner) Next() Request {
	if r.gameOver {
		r.gameOver = false
		r.Stop()
		r.last.Over = true
		return GameOver{}
	}
	if len(r.pages) > 0 {
		shown := r.pages[0]
		r.show(shown)
		r.pages = r.pages[1:]
		if len(r.pages) > 0 {
			return Tap{Auto: shown.Auto}
		}
	}
	for {
		if len(r.notes) > 0 {
			n := r.notes[0]
			r.notes = r.notes[1:]
			if r.ups[n] {
				delete(r.ups, n)
				r.emit(SoundEffect{"levelup"})
			}
			return Say{Text: n}
		}
		if len(r.stack) == 0 {
			if len(r.later) == 0 {
				if r.afterPending {
					r.afterPending = false
					r.push(r.db.Events[r.db.Game.Battle.After].Do)
					continue
				}
				return nil
			}
			r.stack = []frame{{cmds: r.later[0]}}
			r.later = r.later[1:]
			continue
		}
		if len(r.stack) > 64 {
			r.logf(errors.New("events nested too deep (a call loop?)"))
			r.Stop()
			return nil
		}
		top := &r.stack[len(r.stack)-1]
		if top.pc >= len(top.cmds) {
			r.stack = r.stack[:len(r.stack)-1]
			continue
		}
		c := &top.cmds[top.pc]
		if r.afterPending && (c.Battle != "" || len(c.Choice) > 0) {
			r.afterPending = false
			top.checkedAfter, top.location = true, r.st.Location
			r.push(r.db.Events[r.db.Game.Battle.After].Do)
			continue
		}
		if (top.prepared || top.checkedAfter) && top.location != r.st.Location {
			r.Stop()
			return nil
		}
		if c.Battle != "" && !top.prepared {
			if ev := r.db.Events[r.db.Game.Battle.Before]; ev != nil {
				top.prepared, top.location = true, r.st.Location
				r.push(ev.Do)
				continue
			}
		}
		top.prepared, top.checkedAfter = false, false
		top.pc++
		if req := r.run(c); req != nil {
			return req
		}
	}
}

func (r *Runner) run(c *data.Command) Request {
	verb, err := c.Verb()
	if err != nil {
		r.logf(err)
		return nil
	}
	env := r.env()
	switch verb {
	case "say":
		req := r.say(c.Say, env)
		if line, ok := req.(Say); ok {
			if line.Voice != "" {
				r.emit(VoiceEffect{File: line.Voice, Who: c.Say.Who})
			}
			r.aside(line, env)
		}
		return req
	case "choice":
		var labels, ids []string
		var keys []uint64
		var words []data.Phrase
		r.options, r.optionAt = nil, nil
		for i, o := range c.Choice {
			ok, err := o.When.Bool(env)
			r.logf(err)
			if ok {
				r.options = append(r.options, o)
				r.optionAt = append(r.optionAt, i)
				label, p := data.Fill(r.db, o.Label, env)
				labels, words = append(labels, label), append(words, p)
				ids = append(ids, o.ID)
				keys = append(keys, ChoiceKey(c.Prompt, o.Label))
			}
		}
		if len(labels) == 0 {
			r.options = nil
			return nil
		}
		r.asked = c
		prompt, p := data.Fill(r.db, c.Prompt, env)
		return Choice{Prompt: prompt, Options: labels, IDs: ids, Keys: keys, prompt: p, labels: words}
	case "ran":
		r.st.MarkRan(c.Ran)
	case "done":
		r.st.Done[c.Done] = true
	case "cross":
		r.push(atlas.CrossRoad(r.db, r.st, c.Cross))
	case "reach":
		r.push(atlas.Reach(r.db, r.st, c.Reach))
	case "if":
		ok, err := c.If.Bool(env)
		r.logf(err)
		if ok {
			r.push(c.Then)
		} else {
			r.push(c.Else)
		}
	case "set":
		// Both the old/new version and the recollection assign in lexicographic order, one at a time. The right side is evaluated once, and earlier assignments are visible to later right sides.
		keys := make([]string, 0, len(c.SetV))
		for k := range c.SetV {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if !r.setAllowed(k) {
				continue
			}
			v, err := c.SetV[k].Eval(env)
			r.logf(err)
			if err == nil {
				err = r.st.SetPath(k, v)
				r.logf(err)
				if err == nil && k == "location" { // for a dungeon, stand at the entrance (arrival handling is not run)
					if l := r.db.Locations[r.st.Location]; l.Dungeon != nil && l.Dungeon.Generate != nil {
						floor, ok := expr.ToNumber(r.st.Vars[dungeon.VarFloor]) // a generated dungeon dives to the floor in var.floor (floor 1 if unset)
						if !ok {
							floor = 1
						}
						dungeon.DiveAt(r.db, r.st, r.st.Location, int(floor), "", "")
					} else {
						dungeon.Ensure(r.db, r.st)
					}
				}
			}
		}
	case "give":
		if n := c.Give.N(); n != 0 {
			r.st.AddItem(c.Give.Item, n)
		}
	case "take":
		if n := c.Take.N(); n != 0 {
			r.st.Take(c.Take.Item, n)
		}
	case "gold":
		n, err := c.Gold.Number(env)
		r.logf(err)
		r.st.Gold = max(0, r.st.Gold+safeint.From(n))
	case "join":
		// game.yaml join_exp: look up the lower bound from the party before joining, so late joiners catch up
		floor := -1.0
		if r.db.Game.JoinExp.Set() && !slices.Contains(r.st.Party, c.Join) {
			n, err := r.db.Game.JoinExp.Number(env)
			r.logf(err)
			floor = n
		}
		if r.st.Join(c.Join) {
			r.notes = append(r.notes, r.db.Actors[c.Join].Name+lang.T("が なかまに くわわった！"))
			if a := r.st.Actors[c.Join]; floor > float64(a.Exp) {
				a.GainExp(int(floor) - a.Exp)
				a.Recover()
			}
		}
	case "leave":
		if !r.st.Leave(c.Leave) {
			r.logf(fmt.Errorf("leave: %s is the last member of the party, who stays (a party is never empty)", c.Leave))
		}
	case "capture":
		if e := r.db.Enemies[c.Capture]; e != nil && e.Recruit != nil {
			id, ok := r.st.Recruit(c.Capture, max(1, e.Recruit.Level), 0, nil)
			if ok {
				sign := "♀"
				if r.st.Actors[id].Sex == "male" {
					sign = "♂"
				}
				r.notes = append(r.notes, fmt.Sprintf("%s %s joined as %s!", e.Name, sign, id))
				r.push([]data.Command{{Say: &data.Say{Who: e.Name + " " + sign, Face: e.Image.File, Text: e.Name + " " + sign + " joined your party!"}}})
			}
		}
	case "reserve":
		if !r.st.SwapMonster(c.Reserve.In, c.Reserve.Out) {
			r.logf(fmt.Errorf("reserve: cannot swap %s for %s", c.Reserve.In, c.Reserve.Out))
		}
	case "release":
		if !r.st.ReleaseMonster(c.Release) {
			r.logf(fmt.Errorf("release: %s is not a releasable monster", c.Release))
		}
	case "breed":
		first, second := r.st.Actors[c.Breed.Parents[0]], r.st.Actors[c.Breed.Parents[1]]
		var nameA, nameB, faceA, faceB string
		if first != nil && second != nil {
			nameA, nameB = first.Name(), second.Name()
			if first.Sex == "male" {
				nameA += " ♂"
			} else {
				nameA += " ♀"
			}
			if second.Sex == "male" {
				nameB += " ♂"
			} else {
				nameB += " ♀"
			}
			faceA, faceB = r.db.Enemies[first.Monster].Image.File, r.db.Enemies[second.Monster].Image.File
		}
		id, err := r.st.Breed(c.Breed.Parents[0], c.Breed.Parents[1], c.Breed.Skills)
		if err != nil {
			r.logf(err)
			break
		}
		child := r.st.Actors[id]
		childName := child.Name() + " ♀"
		if child.Sex == "male" {
			childName = child.Name() + " ♂"
		}
		r.push([]data.Command{
			{Stand: &data.Stand{Who: nameA, Face: faceA, At: data.StandAt("left"), Enter: "fade"}},
			{Stand: &data.Stand{Who: nameB, Face: faceB, At: data.StandAt("right"), Enter: "fade"}},
			{Effect: data.EffectCue{Name: "fade", Seconds: 0.6}},
			{Stand: &data.Stand{Who: nameA, Leave: "fade"}},
			{Stand: &data.Stand{Who: nameB, Leave: "fade"}},
			{Effect: data.EffectCue{Name: "flash"}},
			{Stand: &data.Stand{Who: childName, Face: r.db.Enemies[child.Monster].Image.File, At: data.StandAt("center"), Enter: "fade"}},
			{Say: &data.Say{Who: childName, Face: r.db.Enemies[child.Monster].Image.File, Text: "A new monster appeared: " + childName + "!"}},
			{Stand: &data.Stand{None: true}},
		})
	case "heal":
		for _, a := range r.st.Members() {
			if c.Heal == "all" || a.ID == c.Heal {
				a.Recover()
			}
		}
	case "hurt":
		for _, a := range r.st.Members() {
			if (c.Hurt.Who == "all" || a.ID == c.Hurt.Who) && a.HP > 0 {
				v, err := c.Hurt.Amount.Number(expr.Chain{expr.Map{"self": rpg.ActorView{A: a}}, env})
				r.logf(err)
				a.HP = max(1, a.HP-max(0, safeint.From(v)))
			}
		}
	case "affect":
		for _, a := range r.st.Members() {
			if c.Affect.Who != "all" && a.ID != c.Affect.Who {
				continue
			}
			if s := c.Affect.Add; s != "" && r.db.States[s] != nil && (!c.Affect.Resist || r.st.Rand.Float64() < a.StateResist(s)) {
				turns := c.Affect.Turns
				if turns <= 0 {
					turns = r.db.States[s].Turns
				}
				if turns <= 0 { // same as the grant during battle, no expiry is held as StateForever
					turns = rpg.StateForever
				}
				a.States[s] = turns
			}
			for s := range a.States {
				if s == c.Affect.Remove || (c.Affect.Remove == "bad" && r.db.States[s] != nil && r.db.States[s].Bad) {
					delete(a.States, s)
				}
			}
		}
	case "battle":
		r.battle = c
		if troop := r.db.Troops[c.Battle]; troop != nil {
			for _, id := range troop.Members {
				r.st.Vars["bestiary."+id+".seen"] = true
			}
		}
		return Battle{Troop: c.Battle, CanLose: c.OnLose != nil, Fighters: c.Fighters, NoRetry: c.Retry != nil && !*c.Retry}
	case "move":
		atlas.Place(r.db, r.st, r.st.Location, c.Move, c.At)
		dungeon.Place(r.db, r.st, c.Move, c.At, c.Facing)
		r.emit(MoveEffect{c.Move})
	case "music":
		r.emit(MusicEffect{*c.Music})
	case "mood":
		r.emit(MoodEffect{c.Mood})
	case "image":
		if len(c.Image.Files) == 0 {
			r.show(*c.Image)
			break
		}
		pages := c.Image.Pages(r.db.FS)
		if len(pages) == 0 {
			break
		}
		n := c.Image.Page
		if c.Image.Pick.Set() { // pick by expression (the value modulo the number of images, plus 1 for the first image)
			v, err := c.Image.Pick.Number(r.env())
			if err != nil {
				v = 0
			}
			k := int(math.Floor(v)) % len(pages)
			if k < 0 {
				k += len(pages)
			}
			n = k + 1
		}
		if n > 0 { // show just one stage (written per stage between passages of text)
			cg := *c.Image
			cg.File, cg.Files, cg.Page, cg.Pick = pages[min(n, len(pages))-1], nil, 0, data.Expr{}
			r.show(cg)
			break
		}
		r.pages = r.pages[:0]
		for _, f := range pages {
			cg := *c.Image
			cg.File, cg.Files = f, nil
			r.pages = append(r.pages, cg)
		}
		r.show(r.pages[0])
		r.pages = r.pages[1:]
		if len(r.pages) > 0 {
			return Tap{Auto: c.Image.Auto}
		}
	case "shop":
		return Shop{ID: c.Shop}
	case "call":
		r.push(r.db.Events[c.Call].Do)
	case "wait":
		return Wait{Seconds: c.Wait.Seconds, NoSkip: !c.Wait.Skippable()}
	case "notice":
		return Notice{Text: data.Render(r.db.Tr(c.Notice), env)}
	case "sfx":
		r.emit(SoundEffect{c.SFX.Name})
		if c.SFX.Wait {
			return Wait{Sound: c.SFX.Name}
		}
	case "stand":
		r.emit(r.stand(c.Stand))
	case "effect":
		r.emit(VisualEffect{Name: c.Effect.Name, Cue: c.Effect})
	case "exp":
		n, err := c.Exp.Number(env)
		r.logf(err)
		// group members who reached the same level into one line (five lines in a row after an action were being forced out)
		var levels []int
		names := map[int][]string{}
		for _, a := range r.st.Members() {
			if !a.Alive() {
				continue
			}
			for _, up := range a.GainExp(safeint.From(n)) {
				if names[up.Level] == nil {
					levels = append(levels, up.Level)
				}
				names[up.Level] = append(names[up.Level], a.Name())
			}
		}
		alive := 0
		for _, a := range r.st.Members() {
			if a.Alive() {
				alive++
			}
		}
		for _, lv := range levels {
			line := rpg.LevelUpLine(names[lv], alive, r.db.Game.Terms.Level, lv)
			r.notes = append(r.notes, line)
			if r.ups == nil {
				r.ups = map[string]bool{}
			}
			r.ups[line] = true
		}
	case "learn":
		if a := r.st.Actors[c.Learn.Actor]; a != nil && !slices.Contains(a.Skills(), c.Learn.Skill) {
			a.Learnt = append(a.Learnt, c.Learn.Skill)
			r.notes = append(r.notes, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), a.Name(), r.db.Skills[c.Learn.Skill].Name))
		}
	case "reset_tree":
		for id, a := range r.st.Actors {
			if c.ResetTree == "all" || c.ResetTree == id {
				a.ResetTree()
			}
		}
	case "end":
		r.stack = nil
	case "clear":
		r.stack = nil
		r.last.Cleared = true
		return Clear{}
	case "save":
		return Save{}
	case "open":
		return Open{Screen: c.Open}
	case "dive": // a made dungeon: a new dive from outside, the next floor from inside
		floor := 1.0
		if c.Floor.Set() {
			n, err := c.Floor.Number(env)
			r.logf(err)
			floor = n
		}
		dungeon.DiveAt(r.db, r.st, c.Dive, int(floor), c.At, c.Facing)
		r.emit(MoveEffect{c.Dive})
	case "open_wall": // a wall by the party becomes the tile (stairs that appear)
		dungeon.OpenWall(r.db, r.st, c.OpenWall)
	case "stone_suggest", "stone_apply", "stone_choose", "stone_wearer", "stone_decline", "equip_stones":
		return r.stones(verb, c, env)
	case "sp_suggest", "sp_apply", "sp_decline":
		r.sp(verb, c)
	case "pick":
		r.pickEntry(data.Render(c.Pick, env))
	case "turn":
		r.turn(c.Turn)
	case "step":
		r.step(c)
	case "walk":
		r.walk(c)
	case "use":
		r.use(c)
	case "equip":
		r.equip(c.Equip)
	case "buy", "sell":
		r.trade(verb, c)
	case "order":
		return r.order(c)
	case "end_turn":
		r.endTurn()
	case "clash":
		r.clash(c)
	case "game_over":
		r.Stop()
		r.last.Over = true
		return GameOver{}
	case "act":
		if r.Act == nil {
			r.logf(errors.New("act: only a session's battles take a move (Act is nil)"))
			break
		}
		r.Act(c.Act.Actor, c.Act.Skill, c.Act.Item, c.Act.Target)
	default:
		if name, _ := data.CustomVerb(c); name != "" {
			r.logf(fmt.Errorf("command %q is declared in game.yaml verbs but has no Go implementation yet", name))
		}
	}
	return nil
}

// say builds the line: the speaker's name and face come from the actor when who is one.
func (r *Runner) say(s *data.Say, env expr.Env) Request {
	out, ok := r.line(s, env)
	if !ok {
		return nil
	}
	for i := range s.Instead {
		if alt, ok := r.line(&s.Instead[i], env); ok {
			out.Instead = append(out.Instead, alt)
		}
	}
	return out
}

// line builds one line (false when it has nothing to show).
func (r *Runner) line(s *data.Say, env expr.Env) (Say, bool) {
	out := Say{Name: s.Who, Key: SourceKey(s.Who, s.Text)}
	out.Text, out.text = data.Fill(r.db, s.Text, env)
	if s.Voice != "" {
		out.Voice, _ = r.db.VoicePath(s.Who, s.Voice)
	}
	if a, ok := r.db.Actors[s.Who]; ok {
		out.Name = a.Name
		out.Actor = s.Who
		face := s.Face
		if face == "" {
			face = a.Face
		}
		if a.Faces != "" && !a.NoStanding {
			outfit := ""
			if loc := r.db.Locations[r.st.Location]; loc != nil {
				outfit = loc.Outfit
			}
			out.Face = r.db.FacePicture(a, face, outfit)
		}
		out.Right = s.Side == "right"
	} else {
		if s.Face != "" {
			out.Face = s.Face
		}
		out.Right = s.Side != "left"
	}
	out.Name, out.who = data.Fill(r.db, out.Name, env)
	if out.Actor != "" {
		out.who.Source = "" // Reword takes the actor's name as the language shows it
	}
	if strings.TrimSpace(out.Text) == "" && out.Face == "" {
		return Say{}, false
	}
	return out, true
}

// stand resolves a stand's face to its picture, as a line does.
func (r *Runner) stand(s *data.Stand) StandEffect {
	if s.None {
		return StandEffect{None: true}
	}
	x, _ := s.At.X()
	out := StandEffect{Who: s.Who, At: x, Enter: s.Enter, Leave: s.Leave, Default: s.Face == ""}
	if s.Leave != "" {
		return out
	}
	out.Face = s.Face
	if a, ok := r.db.Actors[s.Who]; ok {
		face := s.Face
		if face == "" {
			face = a.Face
		}
		outfit := ""
		if loc := r.db.Locations[r.st.Location]; loc != nil {
			outfit = loc.Outfit
		}
		out.Face = r.db.FacePicture(a, face, outfit)
	}
	return out
}

// asideBlocker answers whether the screen should not insert the event placed after a speaker (game.yaml asides) right now
// (a picture that hides the portrait is showing, a recollection is playing, and so on). For a Host without it, the runner decides by the picture it showed.
type asideBlocker interface {
	AsideBlocked() bool
}

// aside pushes one matching entry of game.yaml asides after a speaker's line (it runs after the line and, once done, returns to where it was).
func (r *Runner) aside(s Say, env expr.Env) {
	if len(r.db.Game.Asides) == 0 || r.Extra != nil { // do not insert during battle
		return
	}
	if r.BlockAside != nil {
		if r.BlockAside() {
			return
		}
	} else if b, ok := r.host.(asideBlocker); ok {
		if b.AsideBlocked() {
			return
		}
	} else if r.shown.File != "" && !r.shown.Faces && !r.db.StandIn(r.shown.File) {
		return
	}
	for _, a := range r.db.Game.Asides {
		if !asideSpeaker(a, s) || r.asideInside(a) {
			continue
		}
		ok, err := a.When.Bool(env)
		r.logf(err)
		if !ok {
			continue
		}
		if ev := r.db.Events[a.Call]; ev != nil && len(ev.Do) > 0 {
			// after the inserted event, restore the picture from before the insertion (the location background or the faces background; clear it if none)
			back := r.shown
			r.push([]data.Command{{Image: &back}})
			r.push(ev.Do)
			return
		}
	}
}

func asideSpeaker(a data.Aside, s Say) bool {
	switch {
	case a.Actor != "":
		return s.Actor == a.Actor
	case a.Face != "":
		ok, err := path.Match(a.Face, s.Face)
		return err == nil && ok && s.Face != ""
	default:
		return s.Name == a.Name
	}
}

// asideInside reports whether any currently running event is the inserted event itself or matches a skip pattern.
func (r *Runner) asideInside(a data.Aside) bool {
	for _, f := range r.stack {
		if len(f.cmds) == 0 {
			continue
		}
		for name, ev := range r.db.Events {
			if len(ev.Do) == 0 || &ev.Do[0] != &f.cmds[0] {
				continue
			}
			if name == a.Call {
				return true
			}
			for _, p := range a.Skip {
				if ok, err := path.Match(p, name); err == nil && ok {
					return true
				}
			}
		}
	}
	return false
}

// show shows a picture (and remembers the last picture shown).
func (r *Runner) show(c data.CG) {
	// notify both the screen and the screenless run of the portrait display over a provisional background, with the same value.
	if r.db.StandIn(c.File) {
		c.Faces = true
	}
	r.shown = c
	r.emit(ImageEffect{c})
}

func (r *Runner) emit(effect Effect) {
	if r.host != nil {
		r.host.Handle(effect)
	}
}
