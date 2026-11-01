package planning_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/planning"
	"github.com/nao1215/gozen/internal/rpg"
)

const workshop = `schema_version: 9
id: workshop
start: {location: room}
planning:
  attributes:
    vigor: {type: number, initial: 70, min: 0, max: 100, hidden: true}
    active: {type: bool, initial: true}
    place: {type: string, initial: workshop}
  entities:
    potter:
      name: Potter
      tags: [resident]
      observations: {condition: 'self.vigor >= 50 ? "ready" : "tired"'}
    weaver:
      name: Weaver
      tags: [resident]
  calendar: {start: '2030-07-31', end: '2030-08-04', phases: [morning, evening], weekdays_off: [0, 6]}
  activities:
    prepare: {name: Prepare clay, effort: 2, assignee: potter}
    shape: {name: Shape bowls, effort: 4, after: [prepare], assignee: potter}
  ledgers: {usage: {retain: 2}}
  schedule: [{id: gathering, date: '2030-08-01', phase: morning, event: gather}]
`

func state(t *testing.T) *rpg.State {
	t.Helper()
	db, err := data.Load(fstest.MapFS{"game.yaml": {Data: []byte(workshop)}, "data/story.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {gather: {do: [{say: Welcome}]}}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	return rpg.New(db, nil)
}

func value(t *testing.T, s string) data.Expr {
	t.Helper()
	e, err := expr.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return data.Expr{Expr: e}
}

func apply(t *testing.T, s *rpg.State, c data.PlanCommand) []data.Command {
	t.Helper()
	out, err := planning.Apply(s, c)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func snapshot(t *testing.T, s *rpg.State) []byte {
	t.Helper()
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestObservationAndAtomicSelection(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "observe"})
	visible := s.EntityValues("potter", false)
	if _, ok := visible["vigor"]; ok || visible["condition"] != "ready" {
		t.Fatalf("observation leaks or missing: %v", visible)
	}
	apply(t, s, data.PlanCommand{Op: "set", Tag: "resident", Attribute: "vigor", Value: value(t, "self.vigor - 30")})
	if s.EntityValues("potter", false)["condition"] != "ready" {
		t.Fatal("observation changed before refresh")
	}
	apply(t, s, data.PlanCommand{Op: "observe"})
	if s.EntityValues("potter", false)["condition"] != "tired" {
		t.Fatal("observation not refreshed")
	}
	before := snapshot(t, s)
	_, err := planning.Apply(s, data.PlanCommand{Op: "set", Attribute: "vigor", Value: value(t, `self.id == "potter" ? 30 : 200`)})
	if err == nil || string(before) != string(snapshot(t, s)) {
		t.Fatal("failed batch mutated state")
	}
	apply(t, s, data.PlanCommand{Op: "set", Where: value(t, `self.id == "missing"`), Attribute: "vigor", Value: value(t, "0")})
	if string(before) != string(snapshot(t, s)) {
		t.Fatal("empty selection mutated state")
	}
}

func TestObservationSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		activity  string
		observed  float64
		entity    string
		tag       string
		where     string
		condition string
	}{
		{name: "all", condition: "tired", observed: 1},
		{name: "activity", activity: "prepare", condition: "ready", observed: 1},
		{name: "activity predicate", activity: "prepare", where: "self.progress > 0", condition: "ready", observed: 1},
		{name: "empty activity predicate", activity: "prepare", where: "false", condition: "ready"},
		{name: "predicate only", where: "false", condition: "ready"},
		{name: "matching tag", tag: "resident", condition: "tired"},
		{name: "intersected filters", entity: "potter", tag: "absent", condition: "ready"},
		{name: "one entity", entity: "potter", condition: "tired"},
		{name: "other entity", entity: "weaver", condition: "ready"},
		{name: "empty predicate", entity: "potter", where: "false", condition: "ready"},
		{name: "empty tag", tag: "absent", condition: "ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := state(t)
			def := s.DB.Game.Planning.Activities["prepare"]
			def.Observation = value(t, "self.progress")
			s.DB.Game.Planning.Activities["prepare"] = def
			apply(t, s, data.PlanCommand{Op: "observe"})
			apply(t, s, data.PlanCommand{Op: "set", Entity: "potter", Attribute: "vigor", Value: value(t, "20")})
			apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "1")})
			before := snapshot(t, s)
			restored, err := rpg.Parse(s.DB, before, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, st := range []*rpg.State{s, restored} {
				cmd := data.PlanCommand{Op: "observe", Entity: tc.entity, Tag: tc.tag, Activity: tc.activity}
				if tc.where != "" {
					cmd.Where = value(t, tc.where)
				}
				apply(t, st, cmd)
				if got := st.EntityValues("potter", false)["condition"]; got != tc.condition {
					t.Fatalf("entity condition = %v, want %s", got, tc.condition)
				}
				wantObserved := tc.observed
				if got := st.Planning.Activities["prepare"].Observed; got != wantObserved {
					t.Fatalf("activity observed = %g, want %g", got, wantObserved)
				}
				if st.Planning.Activities["shape"].Observed != 0 || st.Planning.Activities["prepare"].Assignee != "potter" {
					t.Fatal("observation changed another activity or assignment")
				}
				if st.Planning.Activities["prepare"].Progress != 1 {
					t.Fatal("observation changed true progress")
				}
			}
			if !reflect.DeepEqual(s.Planning, restored.Planning) {
				t.Fatal("observation differs after resume")
			}
		})
	}
}

func TestObservationFailureIsAtomic(t *testing.T) {
	for _, target := range []string{"activity", "entity", "predicate", "unknown entity", "unknown activity", "mixed entity", "mixed tag", "activity predicate"} {
		t.Run(target, func(t *testing.T) {
			s := state(t)
			def := s.DB.Game.Planning.Activities["prepare"]
			def.Observation = value(t, "self.progress")
			s.DB.Game.Planning.Activities["prepare"] = def
			apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "1")})
			cmd := data.PlanCommand{Op: "observe"}
			switch target {
			case "activity":
				def = s.DB.Game.Planning.Activities["shape"]
				def.Observation = value(t, "self.missing")
				s.DB.Game.Planning.Activities["shape"] = def
			case "entity":
				entity := s.DB.Game.Planning.Entities["weaver"]
				entity.Observations = map[string]data.Expr{"condition": value(t, "self.missing")}
				s.DB.Game.Planning.Entities["weaver"] = entity
			case "predicate":
				cmd.Where = value(t, "self.missing")
			case "unknown entity":
				cmd.Entity = "missing"
			case "unknown activity":
				cmd.Activity = "missing"
			case "mixed entity":
				cmd.Activity, cmd.Entity = "prepare", "potter"
			case "mixed tag":
				cmd.Activity, cmd.Tag = "prepare", "resident"
			case "activity predicate":
				cmd.Activity = "prepare"
				cmd.Where = value(t, "self.missing")
			}
			before := snapshot(t, s)
			if _, err := planning.Apply(s, cmd); err == nil {
				t.Fatal("invalid observation succeeded")
			}
			if string(snapshot(t, s)) != string(before) {
				t.Fatal("failed observation changed saved state")
			}
			apply(t, s, data.PlanCommand{Op: "advance"})
			if _, err := rpg.Parse(s.DB, snapshot(t, s), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDependenciesReworkAndSave(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "4")})
	if s.Planning.Activities["shape"].Progress != 0 {
		t.Fatal("dependency ignored")
	}
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "10")})
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "4")})
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "-1")})
	if s.Planning.Activities["shape"].Completed != "" {
		t.Fatal("rework kept completion")
	}
	apply(t, s, data.PlanCommand{Op: "assign", Activity: "shape", Entity: "weaver"})
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("round trip lost planning state")
	}
	for _, st := range []*rpg.State{s, restored} {
		apply(t, st, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "1")})
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("resume differs")
	}
}

func TestCalendarDispatchAndPeriodClose(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: "potter", Value: value(t, "2")})
	apply(t, s, data.PlanCommand{Op: "advance"})
	apply(t, s, data.PlanCommand{Op: "advance"})
	if s.Planning.Date != "2030-08-01" || s.Planning.Phase != 0 {
		t.Fatal("month transition failed")
	}
	// A delayed declaration belongs to the previous month, before explicit closing.
	apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Period: "2030-07", Entity: "potter", Value: value(t, "3")})
	apply(t, s, data.PlanCommand{Op: "close", Ledger: "usage", Period: "2030-07"})
	for _, op := range []string{"record", "close"} {
		if _, err := planning.Apply(s, data.PlanCommand{Op: op, Ledger: "usage", Period: "2030-07", Value: value(t, "1")}); err == nil {
			t.Fatal("closed month accepted mutation")
		}
	}
	v, err := value(t, `period_value("usage", "potter", -1)`).Eval(s.Env())
	if err != nil || v != float64(5) {
		t.Fatalf("period history: %v %v", v, err)
	}
	if s.Planning.Ledgers["usage"]["2030-07"]["potter"] != 5 {
		t.Fatal("incorrect ledger total")
	}
	if len(apply(t, s, data.PlanCommand{Op: "dispatch"})) != 1 {
		t.Fatal("reservation not dispatched")
	}
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apply(t, restored, data.PlanCommand{Op: "dispatch"})) != 0 {
		t.Fatal("duplicate dispatch after resume")
	}
	for !s.Planning.Finished {
		apply(t, s, data.PlanCommand{Op: "advance"})
	}
	if s.Planning.Date != "2030-08-04" || s.Planning.Phase != 0 {
		t.Fatal("weekend did not consume one phase")
	}
}

func TestRejectCorruptPlanningSave(t *testing.T) {
	s := state(t)
	var envelope map[string]any
	if err := json.Unmarshal(snapshot(t, s), &envelope); err != nil {
		t.Fatal(err)
	}
	state, ok := envelope["state"].(map[string]any)
	if !ok {
		t.Fatal("state is not an object")
	}
	p, ok := state["planning"].(map[string]any)
	if !ok {
		t.Fatal("planning is not an object")
	}
	p["phase"] = 99
	b, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rpg.Parse(s.DB, b, nil); err == nil {
		t.Fatal("invalid phase accepted")
	}
}

func TestRulesAndPolicyUseObservations(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Rules = map[string]data.PlanRule{"recover": {Params: []string{"amount"}, Do: []data.PlanCommand{{Op: "set", Attribute: "vigor", Value: value(t, "self.vigor + arg.amount")}, {Op: "observe"}}}}
	apply(t, s, data.PlanCommand{Op: "rule", Rule: "recover", Args: map[string]data.Expr{"amount": value(t, "-40")}})
	s.DB.Game.Planning.Policies = map[string][]data.PolicyRule{"steady": {{When: value(t, `entity.potter.condition == "tired"`), Event: "gather"}}}
	cmds := apply(t, s, data.PlanCommand{Op: "policy", Policy: "steady"})
	if len(cmds) != 1 || cmds[0].Call != "gather" {
		t.Fatal("policy ignored observation")
	}
	s.DB.Game.Planning.Policies["hidden"] = []data.PolicyRule{{When: value(t, "entity.potter.vigor < 50"), Event: "gather"}}
	if _, err := planning.Apply(s, data.PlanCommand{Op: "policy", Policy: "hidden"}); err == nil {
		t.Fatal("policy read hidden attribute")
	}
	before := snapshot(t, s)
	if _, err := planning.Apply(s, data.PlanCommand{Op: "rule", Rule: "recover", Args: map[string]data.Expr{}}); err == nil || string(before) != string(snapshot(t, s)) {
		t.Fatal("invalid arguments changed state")
	}
}

func TestReportedActivityProgress(t *testing.T) {
	s := state(t)
	a := s.DB.Game.Planning.Activities["prepare"]
	a.Observation = value(t, "self.progress + 1")
	s.DB.Game.Planning.Activities["prepare"] = a
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "0.5")})
	if s.Planning.Activities["prepare"].Observed != 0 {
		t.Fatal("drawing progress changed without refresh")
	}
	apply(t, s, data.PlanCommand{Op: "observe"})
	if s.Planning.Activities["prepare"].Observed != 1.5 || s.Planning.Activities["prepare"].Progress != .5 {
		t.Fatal("reported and true progress conflated")
	}
}

func TestLedgerRetentionKeepsClosure(t *testing.T) {
	s := state(t)
	for _, month := range []string{"2030-05", "2030-06", "2030-07"} {
		apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: "potter", Period: month, Value: value(t, "1")})
		apply(t, s, data.PlanCommand{Op: "close", Ledger: "usage", Period: month})
	}
	if len(s.Planning.Ledgers["usage"]) != 2 {
		t.Fatal("retention did not prune totals")
	}
	before := snapshot(t, s)
	if _, err := planning.Apply(s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: "potter", Period: "2030-05", Value: value(t, "1")}); err == nil || string(before) != string(snapshot(t, s)) {
		t.Fatal("pruned closed period was reopened")
	}
}

func TestEffortChangesCompletionDependenciesAndSave(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "2")})
	apply(t, s, data.PlanCommand{Op: "effort", Activity: "prepare", Value: value(t, "activity.prepare.effort * 2")})
	if s.Planning.Activities["prepare"].Completed != "" || s.ActivityEffort("prepare") != 4 {
		t.Fatal("increased budget did not reopen completed work")
	}
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "4")})
	if s.Planning.Activities["shape"].Progress != 0 {
		t.Fatal("dependency used the old budget")
	}
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*rpg.State{s, restored} {
		apply(t, st, data.PlanCommand{Op: "effort", Activity: "prepare", Value: value(t, "1")})
		a := st.Planning.Activities["prepare"]
		if a.Progress != 1 || a.Observed != 1 || a.Completed == "" {
			t.Fatalf("reduced budget did not clamp and complete: %+v", a)
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "4")})
		if st.Planning.Activities["shape"].Completed == "" {
			t.Fatal("reduced dependency did not unlock work")
		}
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("resumed effort changes differ")
	}
	for _, input := range []string{"0", "-1", `"invalid"`, "1/0"} {
		before := snapshot(t, s)
		if _, err := planning.Apply(s, data.PlanCommand{Op: "effort", Activity: "shape", Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
			t.Fatalf("invalid effort %s was accepted or mutated state", input)
		}
	}
	for _, invalid := range []float64{-1, .5} {
		var envelope struct {
			State map[string]any `json:"state"`
		}
		if err := json.Unmarshal(snapshot(t, s), &envelope); err != nil {
			t.Fatal(err)
		}
		plan, ok := envelope.State["planning"].(map[string]any)
		if !ok {
			t.Fatal("missing planning state")
		}
		activities, ok := plan["activities"].(map[string]any)
		if !ok {
			t.Fatal("missing activities")
		}
		activity, ok := activities["prepare"].(map[string]any)
		if !ok {
			t.Fatal("missing activity")
		}
		activity["effort"] = invalid
		corrupt, err := json.Marshal(envelope.State)
		if err != nil {
			t.Fatal(err)
		}
		// Keep the ordinary envelope metadata while replacing only its state.
		var saved map[string]json.RawMessage
		if err := json.Unmarshal(snapshot(t, s), &saved); err != nil {
			t.Fatal(err)
		}
		saved["state"] = corrupt
		encoded, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rpg.Parse(s.DB, encoded, nil); err == nil {
			t.Fatalf("accepted saved effort %g inconsistent with progress", invalid)
		}
	}
	// Unmodified saves omit the override and still use the definition's budget.
	legacy := state(t)
	loaded, err := rpg.Parse(legacy.DB, snapshot(t, legacy), nil)
	if err != nil || loaded.ActivityEffort("prepare") != 2 {
		t.Fatalf("legacy budget: %v", err)
	}
}

func TestCancellationPreservesWorkAndRestoresDependencies(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, ".5")})
	apply(t, s, data.PlanCommand{Op: "cancel", Activity: "prepare", Value: value(t, "true")})
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*rpg.State{s, restored} {
		st.DB.Game.Planning.Policies = map[string][]data.PolicyRule{"excluded": {{When: value(t, "activity.prepare.cancelled"), Event: "bell"}}}
		commands := apply(t, st, data.PlanCommand{Op: "policy", Policy: "excluded"})
		if len(commands) != 1 || commands[0].Call != "bell" {
			t.Fatal("policy cannot observe cancellation")
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: "prepare", Value: value(t, "2")})
		if a := st.Planning.Activities["prepare"]; a.Progress != .5 || a.Completed != "" || !a.Cancelled {
			t.Fatalf("cancel lost work: %+v", a)
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "1")})
		if st.Planning.Activities["shape"].Progress != 1 {
			t.Fatal("cancelled dependency blocked work")
		}
		apply(t, st, data.PlanCommand{Op: "cancel", Activity: "prepare", Value: value(t, "false")})
		apply(t, st, data.PlanCommand{Op: "progress", Activity: "shape", Value: value(t, "1")})
		if st.Planning.Activities["shape"].Progress != 1 {
			t.Fatal("restored dependency ignored")
		}
	}
	for _, input := range []string{"1", `"true"`} {
		before := snapshot(t, s)
		if _, err := planning.Apply(s, data.PlanCommand{Op: "cancel", Activity: "prepare", Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
			t.Fatal("invalid cancellation was not atomic")
		}
	}
}

func TestDeadlineChangesPersistAndRejectInvalidDates(t *testing.T) {
	s := state(t)
	def := s.DB.Game.Planning.Activities["prepare"]
	def.From, def.To = "2030-07-31", "2030-08-03"
	s.DB.Game.Planning.Activities["prepare"] = def
	apply(t, s, data.PlanCommand{Op: "deadline", Activity: "prepare", Value: value(t, `"2030-08-01"`)})
	if s.ActivityDeadline("prepare") != "2030-08-01" || s.DB.Game.Planning.Activities["prepare"].To != "2030-08-03" {
		t.Fatal("deadline did not override the authored date independently")
	}
	loaded, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActivityDeadline("prepare") != "2030-08-01" {
		t.Fatal("deadline lost on load")
	}
	for _, input := range []string{`"2030-07-30"`, `"2030-02-30"`, `"2030-8-01"`, `42`, `true`, `""`} {
		before := snapshot(t, s)
		if _, err := planning.Apply(s, data.PlanCommand{Op: "deadline", Activity: "prepare", Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
			t.Fatalf("invalid deadline %s accepted or mutated state", input)
		}
	}
	s.Planning.Activities["prepare"].Deadline = "2030-07-01"
	if _, err := s.Save(); err == nil {
		t.Fatal("invalid deadline serialized")
	}
	s.Planning.Activities["prepare"].Deadline = ""
	if s.ActivityDeadline("prepare") != "2030-08-03" {
		t.Fatal("legacy save must retain authored deadline")
	}
}

// Overflow must not poison the saved state or prevent the next operation.
func TestNonFiniteScalarRollback(t *testing.T) {
	huge := "1" + strings.Repeat("0", 200)
	for _, formula := range []string{huge + " * " + huge, "-" + huge + " * " + huge, "(" + huge + " * " + huge + ") - (" + huge + " * " + huge + ")"} {
		for _, target := range []string{"observation", "argument", "attribute", "activity"} {
			t.Run(target+"/"+formula[:8], func(t *testing.T) {
				s := state(t)
				apply(t, s, data.PlanCommand{Op: "observe"})
				cmd := data.PlanCommand{Op: "observe"}
				switch target {
				case "observation":
					entity := s.DB.Game.Planning.Entities["weaver"]
					entity.Observations = map[string]data.Expr{"report": value(t, formula)}
					s.DB.Game.Planning.Entities["weaver"] = entity
					apply(t, s, data.PlanCommand{Op: "set", Entity: "potter", Attribute: "vigor", Value: value(t, "20")})
				case "argument":
					s.DB.Game.Planning.Rules = map[string]data.PlanRule{"step": {Params: []string{"unused"}, Do: []data.PlanCommand{{Op: "advance"}}}}
					cmd = data.PlanCommand{Op: "rule", Rule: "step", Args: map[string]data.Expr{"unused": value(t, formula)}}
				case "attribute":
					cmd = data.PlanCommand{Op: "set", Attribute: "vigor", Value: value(t, formula)}
				case "activity":
					def := s.DB.Game.Planning.Activities["shape"]
					def.Observation = value(t, formula)
					s.DB.Game.Planning.Activities["shape"] = def
				}
				before := snapshot(t, s)
				if _, err := planning.Apply(s, cmd); err == nil {
					t.Fatal("non-finite scalar accepted")
				}
				if string(snapshot(t, s)) != string(before) {
					t.Fatal("rejected operation changed saved state")
				}
				apply(t, s, data.PlanCommand{Op: "advance"})
				restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(s.Planning, restored.Planning) {
					t.Fatal("resume changed planning state")
				}
			})
		}
	}
}

// Numeric operations reject implicit scalar coercion and roll back earlier rule steps.
func TestNumericOperationTypes(t *testing.T) {
	for _, formula := range []string{"true", "false", "nil", "\"2\"", "2", "int(true)", "entity.potter.active"} {
		for _, op := range []string{"progress", "effort", "record", "observe"} {
			t.Run(op+"/"+formula, func(t *testing.T) {
				s := state(t)
				cmd := data.PlanCommand{Op: op, Activity: "prepare", Value: value(t, formula)}
				if op == "record" {
					cmd = data.PlanCommand{Op: op, Ledger: "usage", Value: value(t, formula)}
				}
				if op == "observe" {
					def := s.DB.Game.Planning.Activities["prepare"]
					def.Observation = value(t, formula)
					s.DB.Game.Planning.Activities["prepare"] = def
					cmd = data.PlanCommand{Op: op, Activity: "prepare"}
				}
				s.DB.Game.Planning.Rules = map[string]data.PlanRule{"update": {Do: []data.PlanCommand{{Op: "advance"}, cmd}}}
				before := snapshot(t, s)
				_, err := planning.Apply(s, data.PlanCommand{Op: "rule", Rule: "update"})
				valid := formula == "2" || formula == "int(true)"
				if valid {
					if err != nil {
						t.Fatal(err)
					}
					want := 2.0
					if formula == "int(true)" {
						want = 1
					}
					switch op {
					case "progress":
						if s.Planning.Activities["prepare"].Progress != want {
							t.Fatal("progress mismatch")
						}
					case "effort":
						if s.ActivityEffort("prepare") != want {
							t.Fatal("effort mismatch")
						}
					case "observe":
						if s.Planning.Activities["prepare"].Observed != want {
							t.Fatal("observation mismatch")
						}
					case "record":
						for _, id := range []string{"potter", "weaver"} {
							if s.Planning.Ledgers["usage"]["2030-07"][id] != want {
								t.Fatal("ledger mismatch")
							}
						}
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), "finite number") {
						t.Fatalf("expected numeric diagnostic, got %v", err)
					}
					if string(snapshot(t, s)) != string(before) {
						t.Fatal("rejected rule changed state")
					}
					apply(t, s, data.PlanCommand{Op: "advance"})
				}
				restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(s.Planning, restored.Planning) {
					t.Fatal("resume changed state")
				}
			})
		}
	}
}
