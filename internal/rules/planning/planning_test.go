package planning_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/planning"
)

const workshop = `schema_version: 10
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
	out, err := applyUnbound(s, c)
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
	_, err := applyUnbound(s, data.PlanCommand{Op: "set", Attribute: "vigor", Value: value(t, `self.id == "potter" ? 30 : 200`)})
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
			apply(t, s, data.PlanCommand{Op: "set", Entity: data.IDRef{ID: "potter"}, Attribute: "vigor", Value: value(t, "20")})
			apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "1")})
			before := snapshot(t, s)
			restored, err := rpg.Parse(s.DB, before, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, st := range []*rpg.State{s, restored} {
				cmd := data.PlanCommand{Op: "observe", Entity: data.IDRef{ID: tc.entity}, Tag: tc.tag, Activity: data.IDRef{ID: tc.activity}}
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
			apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "1")})
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
				cmd.Entity = data.IDRef{ID: "missing"}
			case "unknown activity":
				cmd.Activity = data.IDRef{ID: "missing"}
			case "mixed entity":
				cmd.Activity, cmd.Entity = data.IDRef{ID: "prepare"}, data.IDRef{ID: "potter"}
			case "mixed tag":
				cmd.Activity, cmd.Tag = data.IDRef{ID: "prepare"}, "resident"
			case "activity predicate":
				cmd.Activity = data.IDRef{ID: "prepare"}
				cmd.Where = value(t, "self.missing")
			}
			before := snapshot(t, s)
			if _, err := applyUnbound(s, cmd); err == nil {
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
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "4")})
	if s.Planning.Activities["shape"].Progress != 0 {
		t.Fatal("dependency ignored")
	}
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "10")})
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "4")})
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "-1")})
	if s.Planning.Activities["shape"].Completed != "" {
		t.Fatal("rework kept completion")
	}
	apply(t, s, data.PlanCommand{Op: "assign", Activity: data.IDRef{ID: "shape"}, Entity: data.IDRef{ID: "weaver"}})
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("round trip lost planning state")
	}
	for _, st := range []*rpg.State{s, restored} {
		apply(t, st, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "1")})
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("resume differs")
	}
}

func TestCalendarDispatchAndPeriodClose(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: data.IDRef{ID: "potter"}, Value: value(t, "2")})
	apply(t, s, data.PlanCommand{Op: "advance"})
	apply(t, s, data.PlanCommand{Op: "advance"})
	if s.Planning.Date != "2030-08-01" || s.Planning.Phase != 0 {
		t.Fatal("month transition failed")
	}
	// A delayed declaration belongs to the previous month, before explicit closing.
	apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Period: "2030-07", Entity: data.IDRef{ID: "potter"}, Value: value(t, "3")})
	apply(t, s, data.PlanCommand{Op: "close", Ledger: "usage", Period: "2030-07"})
	for _, op := range []string{"record", "close"} {
		if _, err := applyUnbound(s, data.PlanCommand{Op: op, Ledger: "usage", Period: "2030-07", Value: value(t, "1")}); err == nil {
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
	if len(cmds) != 1 || cmds[0].Call.Event != "gather" {
		t.Fatal("policy ignored observation")
	}
	s.DB.Game.Planning.Policies["hidden"] = []data.PolicyRule{{When: value(t, "entity.potter.vigor < 50"), Event: "gather"}}
	if _, err := applyUnbound(s, data.PlanCommand{Op: "policy", Policy: "hidden"}); err == nil {
		t.Fatal("policy read hidden attribute")
	}
	before := snapshot(t, s)
	if _, err := applyUnbound(s, data.PlanCommand{Op: "rule", Rule: "recover", Args: map[string]data.Expr{}}); err == nil || string(before) != string(snapshot(t, s)) {
		t.Fatal("invalid arguments changed state")
	}
}

func TestPolicyCollectionsUseOnlyObservedState(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Policies = map[string][]data.PolicyRule{}
	apply(t, s, data.PlanCommand{Op: "observe"})
	s.Planning.Activities["prepare"].Progress = 1.5
	s.Planning.Activities["prepare"].Observed = .5
	for _, formula := range []string{
		`sum(activities, it.progress) == .5`,
		`sum(activities, it.observed) == .5`,
		`first(activities, nil).id == "prepare"`,
		`count(entities, it.active) == 2`,
		`first(filter(entities, it.id == "potter"), nil).condition == "ready"`,
	} {
		s.DB.Game.Planning.Policies["public"] = []data.PolicyRule{{When: value(t, formula), Event: "gather"}}
		before := snapshot(t, s)
		calls := apply(t, s, data.PlanCommand{Op: "policy", Policy: "public"})
		if len(calls) != 1 || calls[0].Call.Event != "gather" || string(before) != string(snapshot(t, s)) {
			t.Fatalf("public query changed state or read true work: %s", formula)
		}
	}
	for _, formula := range []string{`sum(entities, it.vigor) > 0`, `count(activities, it.completed) > 0`} {
		s.DB.Game.Planning.Policies["private"] = []data.PolicyRule{{When: value(t, formula), Event: "gather"}}
		before := snapshot(t, s)
		if _, err := applyUnbound(s, data.PlanCommand{Op: "policy", Policy: "private"}); err == nil || string(before) != string(snapshot(t, s)) {
			t.Fatalf("private query accepted or mutated state: %s", formula)
		}
	}
}

func TestReportedActivityProgress(t *testing.T) {
	s := state(t)
	a := s.DB.Game.Planning.Activities["prepare"]
	a.Observation = value(t, "self.progress + 1")
	s.DB.Game.Planning.Activities["prepare"] = a
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "0.5")})
	if s.Planning.Activities["prepare"].Observed != 0 {
		t.Fatal("drawing progress changed without refresh")
	}
	apply(t, s, data.PlanCommand{Op: "observe"})
	if s.ActivityValues("prepare")["observed"] != 1.5 || s.ActivityValues("prepare")["progress"] != .5 {
		t.Fatal("reported and true progress conflated")
	}
}

func TestLedgerRetentionKeepsClosure(t *testing.T) {
	s := state(t)
	for _, month := range []string{"2030-05", "2030-06", "2030-07"} {
		apply(t, s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: data.IDRef{ID: "potter"}, Period: month, Value: value(t, "1")})
		apply(t, s, data.PlanCommand{Op: "close", Ledger: "usage", Period: month})
	}
	if len(s.Planning.Ledgers["usage"]) != 2 {
		t.Fatal("retention did not prune totals")
	}
	before := snapshot(t, s)
	if _, err := applyUnbound(s, data.PlanCommand{Op: "record", Ledger: "usage", Entity: data.IDRef{ID: "potter"}, Period: "2030-05", Value: value(t, "1")}); err == nil || string(before) != string(snapshot(t, s)) {
		t.Fatal("pruned closed period was reopened")
	}
}

func TestEffortChangesCompletionDependenciesAndSave(t *testing.T) {
	s := state(t)
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "2")})
	apply(t, s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "activity.prepare.effort * 2")})
	if s.Planning.Activities["prepare"].Completed != "" || s.ActivityEffort("prepare") != 4 {
		t.Fatal("increased budget did not reopen completed work")
	}
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "4")})
	if s.Planning.Activities["shape"].Progress != 0 {
		t.Fatal("dependency used the old budget")
	}
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*rpg.State{s, restored} {
		apply(t, st, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "1")})
		a := st.Planning.Activities["prepare"]
		if a.Progress != 1 || a.Observed != 1 || a.Completed == "" {
			t.Fatalf("reduced budget did not clamp and complete: %+v", a)
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "4")})
		if st.Planning.Activities["shape"].Completed == "" {
			t.Fatal("reduced dependency did not unlock work")
		}
	}
	if !reflect.DeepEqual(s.Planning, restored.Planning) {
		t.Fatal("resumed effort changes differ")
	}
	for _, input := range []string{"0", "-1", `"invalid"`, "1/0"} {
		before := snapshot(t, s)
		if _, err := applyUnbound(s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "shape"}, Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
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
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, ".5")})
	apply(t, s, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "true")})
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*rpg.State{s, restored} {
		st.DB.Game.Planning.Policies = map[string][]data.PolicyRule{"excluded": {{When: value(t, "activity.prepare.cancelled"), Event: "bell"}}}
		commands := apply(t, st, data.PlanCommand{Op: "policy", Policy: "excluded"})
		if len(commands) != 1 || commands[0].Call.Event != "bell" {
			t.Fatal("policy cannot observe cancellation")
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "2")})
		if a := st.Planning.Activities["prepare"]; a.Progress != .5 || a.Completed != "" || !a.Cancelled {
			t.Fatalf("cancel lost work: %+v", a)
		}
		apply(t, st, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "1")})
		if st.Planning.Activities["shape"].Progress != 1 {
			t.Fatal("cancelled dependency blocked work")
		}
		apply(t, st, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "false")})
		apply(t, st, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "shape"}, Value: value(t, "1")})
		if st.Planning.Activities["shape"].Progress != 1 {
			t.Fatal("restored dependency ignored")
		}
	}
	for _, input := range []string{"1", `"true"`} {
		before := snapshot(t, s)
		if _, err := applyUnbound(s, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "prepare"}, Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
			t.Fatal("invalid cancellation was not atomic")
		}
	}
}

func TestDeadlineChangesPersistAndRejectInvalidDates(t *testing.T) {
	s := state(t)
	def := s.DB.Game.Planning.Activities["prepare"]
	def.From, def.To = "2030-07-31", "2030-08-03"
	s.DB.Game.Planning.Activities["prepare"] = def
	apply(t, s, data.PlanCommand{Op: "deadline", Activity: data.IDRef{ID: "prepare"}, Value: value(t, `"2030-08-01"`)})
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
		if _, err := applyUnbound(s, data.PlanCommand{Op: "deadline", Activity: data.IDRef{ID: "prepare"}, Value: value(t, input)}); err == nil || string(before) != string(snapshot(t, s)) {
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
					apply(t, s, data.PlanCommand{Op: "set", Entity: data.IDRef{ID: "potter"}, Attribute: "vigor", Value: value(t, "20")})
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
				if _, err := applyUnbound(s, cmd); err == nil {
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
				cmd := data.PlanCommand{Op: op, Activity: data.IDRef{ID: "prepare"}, Value: value(t, formula)}
				if op == "record" {
					cmd = data.PlanCommand{Op: op, Ledger: "usage", Value: value(t, formula)}
				}
				if op == "observe" {
					def := s.DB.Game.Planning.Activities["prepare"]
					def.Observation = value(t, formula)
					s.DB.Game.Planning.Activities["prepare"] = def
					cmd = data.PlanCommand{Op: op, Activity: data.IDRef{ID: "prepare"}}
				}
				s.DB.Game.Planning.Rules = map[string]data.PlanRule{"update": {Do: []data.PlanCommand{{Op: "advance"}, cmd}}}
				before := snapshot(t, s)
				_, err := applyUnbound(s, data.PlanCommand{Op: "rule", Rule: "update"})
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

func TestExpressionTargetsAreAtomic(t *testing.T) {
	for _, formula := range []string{`arg.resident`, `true`, `nil`, `""`, `"missing"`, `rand(2) == 0 ? "potter" : "weaver"`} {
		t.Run(formula, func(t *testing.T) {
			s := state(t)
			rng := rand.NewPCG(7, 11)
			s.Rand = rand.New(rng) //nolint:gosec // Deterministic gameplay test, not secrets.
			randomBefore, err := rng.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			s.DB.Game.Planning.Rules = map[string]data.PlanRule{}
			// A later bad target must roll back earlier rule steps as well.
			s.DB.Game.Planning.Rules["assign"] = data.PlanRule{Params: []string{"task", "resident"}, Do: []data.PlanCommand{
				{Op: "advance"},
				{Op: "assign", Activity: data.IDRef{Expr: value(t, "arg.task")}, Entity: data.IDRef{Expr: value(t, formula)}},
			}}
			before := snapshot(t, s)
			_, err = applyUnbound(s, data.PlanCommand{Op: "rule", Rule: "assign", Args: map[string]data.Expr{
				"task": value(t, `"shape"`), "resident": value(t, `"missing"`),
			}})
			if err == nil || string(before) != string(snapshot(t, s)) {
				t.Fatalf("target failure changed state: %v", err)
			}
			randomAfter, err := rng.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if string(randomBefore) != string(randomAfter) {
				t.Fatal("target failure consumed randomness")
			}
		})
	}
	s := state(t)
	_, err := planning.ApplyIn(s, data.PlanCommand{Op: "assign", Activity: data.IDRef{Expr: value(t, "arg.task")}, Entity: data.IDRef{Expr: value(t, "arg.resident")}}, expr.Map{"arg": expr.Map{"task": "shape", "resident": "weaver"}})
	if err != nil || s.Planning.Activities["shape"].Assignee != "weaver" || s.Planning.Activities["prepare"].Assignee != "potter" {
		t.Fatalf("assignment: %v", err)
	}
	apply(t, s, data.PlanCommand{Op: "assign", Activity: data.IDRef{ID: "shape"}})
	if s.Planning.Activities["shape"].Assignee != "" {
		t.Fatal("literal omitted entity no longer clears assignment")
	}
}

func TestLiteralAndExpressionTargetsAgree(t *testing.T) {
	for _, command := range []data.PlanCommand{
		{Op: "assign", Activity: data.IDRef{ID: "prepare"}, Entity: data.IDRef{ID: "weaver"}},
		{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "1")},
		{Op: "effort", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "3")},
		{Op: "cancel", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "true")},
		{Op: "deadline", Activity: data.IDRef{ID: "prepare"}, Value: value(t, `"2030-08-02"`)},
		{Op: "set", Entity: data.IDRef{ID: "weaver"}, Attribute: "vigor", Value: value(t, "20")},
		{Op: "observe", Entity: data.IDRef{ID: "weaver"}},
		{Op: "observe", Activity: data.IDRef{ID: "prepare"}},
		{Op: "record", Entity: data.IDRef{ID: "weaver"}, Ledger: "usage", Value: value(t, "2")},
	} {
		t.Run(command.Op, func(t *testing.T) {
			literal, dynamic := state(t), state(t)
			apply(t, literal, command)
			if command.Entity.ID != "" {
				command.Entity = data.IDRef{Expr: value(t, `"`+command.Entity.ID+`"`)}
			}
			if command.Activity.ID != "" {
				command.Activity = data.IDRef{Expr: value(t, `"`+command.Activity.ID+`"`)}
			}
			apply(t, dynamic, command)
			if string(snapshot(t, literal)) != string(snapshot(t, dynamic)) {
				t.Fatal("literal and expression targets produce different saved states")
			}
		})
	}
}

func TestActivityAttributes(t *testing.T) {
	s := state(t)
	// A crop's moisture is task-local state, independent of its assignee.
	s.DB.Game.Planning.ActivityAttributes = map[string]data.Attribute{"moisture": {Type: "number", Initial: float64(0)}}
	s = rpg.New(s.DB, nil)
	apply(t, s, data.PlanCommand{Op: "set", Activity: data.IDRef{ID: "prepare"}, Attribute: "moisture", Value: value(t, "self.moisture+3")})
	got, err := value(t, "sum(activities, it.moisture)").Eval(s.Env())
	if err != nil || got != float64(3) {
		t.Fatalf("aggregate: %v, %v", got, err)
	}
	if _, err := applyUnbound(s, data.PlanCommand{Op: "set", Activity: data.IDRef{ID: "missing"}, Attribute: "moisture", Value: value(t, "1")}); err == nil {
		t.Fatal("unknown activity accepted")
	}

	before := snapshot(t, s)
	for _, v := range []string{`"wet"`, strings.Repeat("9", 200) + "*" + strings.Repeat("9", 200)} {
		if _, err := applyUnbound(s, data.PlanCommand{Op: "set", Activity: data.IDRef{ID: "prepare"}, Attribute: "moisture", Value: value(t, v)}); err == nil {
			t.Fatal("invalid attribute accepted")
		}
		if !reflect.DeepEqual(before, snapshot(t, s)) {
			t.Fatal("failed write changed state")
		}
	}
	// A failed multi-step rule restores earlier activity writes as well.
	if s.DB.Game.Planning.Rules == nil {
		s.DB.Game.Planning.Rules = map[string]data.PlanRule{}
	}
	s.DB.Game.Planning.Rules["water"] = data.PlanRule{Do: []data.PlanCommand{
		{Op: "set", Activity: data.IDRef{ID: "shape"}, Attribute: "moisture", Value: value(t, "5")},
		{Op: "set", Activity: data.IDRef{ID: "prepare"}, Attribute: "missing", Value: value(t, "2")},
	}}
	if _, err := applyUnbound(s, data.PlanCommand{Op: "rule", Rule: "water"}); err == nil {
		t.Fatal("invalid rule accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, s)) {
		t.Fatal("failed rule retained a write")
	}
	// Missing overrides read defaults; stored overrides must remain typed and known.
	for _, attrs := range []map[string]any{{"moisture": "wet"}, {"unknown": float64(1)}} {
		s.Planning.Activities["shape"].Attributes = attrs
		if _, err := s.Save(); err == nil {
			t.Fatal("invalid saved attributes accepted")
		}
	}
	s.Planning.Activities["shape"].Attributes = nil

	restored, err := rpg.Parse(s.DB, before, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = value(t, "activity.prepare.moisture").Eval(restored.Env())
	if err != nil || got != float64(3) {
		t.Fatalf("restored: %v, %v", got, err)
	}
}

func TestWorkHistoryUsesObservationsAndSurvivesSave(t *testing.T) {
	s := state(t)
	s.DB.Game.Screens = map[string]*data.AuthorScreen{"studio": {Kind: "islands", View: data.PlanningView{Burnup: &data.BurnupView{}}}}
	def := s.DB.Game.Planning.Activities["prepare"]
	def.Observation = value(t, "self.progress * 0.5")
	s.DB.Game.Planning.Activities["prepare"] = def
	apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "2")})
	apply(t, s, data.PlanCommand{Op: "observe"})
	first := s.Planning.WorkHistory[s.Planning.Date]
	if first.Total != 6 || first.Observed != 1 {
		t.Fatalf("true progress leaked: %+v", first)
	}
	apply(t, s, data.PlanCommand{Op: "observe"})
	if len(s.Planning.WorkHistory) != 1 {
		t.Fatal("repeated observation appended a sample")
	}
	restored, err := rpg.Parse(s.DB, snapshot(t, s), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Planning.WorkHistory, s.Planning.WorkHistory) {
		t.Fatal("history lost on load")
	}
	s.Planning.Date = "2030-08-01"
	apply(t, s, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "shape"}, Value: value(t, "true")})
	apply(t, s, data.PlanCommand{Op: "observe"})
	if s.Planning.WorkHistory["2030-08-01"].Total != 2 || s.Planning.WorkHistory["2030-07-31"] != first {
		t.Fatal("scope change rewrote history")
	}
	savedSample := s.Planning.WorkHistory["2030-07-31"]
	s.Planning.WorkHistory["2030-07-31"] = rpg.WorkSample{Total: 1, Observed: 2}
	if _, err := s.Save(); err == nil {
		t.Fatal("corrupt chart history accepted")
	}
	s.Planning.WorkHistory["2030-07-31"] = savedSample
	before := snapshot(t, s)
	def.Observation = value(t, `"invalid"`)
	s.DB.Game.Planning.Activities["prepare"] = def
	if _, err := applyUnbound(s, data.PlanCommand{Op: "observe"}); err == nil {
		t.Fatal("invalid observation accepted")
	}
	if string(before) != string(snapshot(t, s)) {
		t.Fatal("failed observation changed history")
	}
}

func TestRevisedChildBudgetPreservesRemainingWork(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	for _, command := range [][3]string{{"effort", "mix", "2"}, {"progress", "mix", "1.9"}, {"progress", "prepare", ".5"}} {
		apply(t, s, data.PlanCommand{Op: command[0], Activity: data.IDRef{ID: command[1]}, Value: value(t, command[2])})
	}
	if s.Planning.Activities["mix"].Progress < 1.9 || math.Abs(s.Planning.Activities["prepare"].Progress-2.4) > 1e-9 {
		t.Fatal("parent work used obsolete child budgets", s.Planning.Activities["mix"], s.Planning.Activities["prepare"])
	}
	apply(t, s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "7")})
	if s.ActivityEffort("mix") != 4 || s.ActivityEffort("knead") != 3 {
		t.Fatal("parent revision discarded child estimates")
	}
}

func TestDecompositionCompletionRoundingSurvivesSave(t *testing.T) {
	for _, amount := range []string{"0.499999999999", "0.499999"} {
		t.Run(amount, func(t *testing.T) {
			s := state(t)
			s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
			s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
			s.Planning.Activities["mix"] = &rpg.ActivityState{}
			s.Planning.Activities["knead"] = &rpg.ActivityState{}
			apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
			apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "knead"}, Value: value(t, "1.5")})
			apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "mix"}, Value: value(t, amount)})
			complete := amount == "0.499999999999"
			for _, id := range []string{"mix", "prepare"} {
				a := s.Planning.Activities[id]
				if (a.Completed != "") != complete || (a.Progress == s.ActivityEffort(id)) != complete {
					t.Fatalf("%s completion and work disagree: %+v", id, a)
				}
			}
			if _, err := rpg.Parse(s.DB, snapshot(t, s), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDecompositionConservesWorkAndReports(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	cmd := func(op, id, amount string) {
		t.Helper()
		apply(t, s, data.PlanCommand{Op: op, Activity: data.IDRef{ID: id}, Value: value(t, amount)})
	}
	cmd("progress", "prepare", "1")
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	if s.Planning.Activities["mix"].Progress != .25 || s.Planning.Activities["knead"].Progress != .75 || s.Planning.Activities["prepare"].Assignee != "" {
		t.Fatal(s.Planning.Activities)
	}
	saved := snapshot(t, s)
	restored, err := rpg.Parse(s.DB, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	s = restored
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	if !reflect.DeepEqual(saved, snapshot(t, s)) {
		t.Fatal("repeated split changed state")
	}
	cmd("progress", "mix", ".25")
	if s.Planning.Activities["prepare"].Progress != 1.25 {
		t.Fatal("child work did not roll up")
	}
	cmd("progress", "prepare", "-.5")
	if s.Planning.Activities["prepare"].Progress != .75 {
		t.Fatal("rework lost work")
	}
	cmd("effort", "prepare", "4")
	if s.ActivityEffort("mix") != 1 || s.ActivityEffort("knead") != 3 {
		t.Fatal("budget not conserved")
	}
	cmd("effort", "mix", "1.5")
	if s.ActivityEffort("prepare") != 4.5 {
		t.Fatal("child budget did not roll up")
	}
	apply(t, s, data.PlanCommand{Op: "observe", Activity: data.IDRef{ID: "prepare"}})
	if s.Planning.Activities["prepare"].Observed != .75 {
		t.Fatal("root did not observe children")
	}
	cmd("cancel", "prepare", "true")
	cmd("progress", "mix", "100")
	if !s.Planning.Activities["knead"].Cancelled || s.Planning.Activities["prepare"].Progress != .75 {
		t.Fatal("cancel did not reach children")
	}
	before := snapshot(t, s)
	if _, err := applyUnbound(s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "mix"}}); err == nil {
		t.Fatal("split child accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, s)) {
		t.Fatal("failed split changed state")
	}
}

func TestScheduleDependenciesPresenceAndExclusion(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Schedule = []data.Reservation{
		{ID: "first", Phase: "morning", Event: "gather", Exclusive: "meeting", When: value(t, "flag.present")},
		{ID: "second", Phase: "morning", Event: "later", After: []string{"first"}, Exclusive: "meeting"},
	}
	dispatch := func() []data.Command {
		t.Helper()
		out, err := applyUnbound(s, data.PlanCommand{Op: "dispatch"})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := dispatch(); len(out) != 0 {
		t.Fatal("absent participant selected", out)
	}
	s.Flags["present"] = true
	if out := dispatch(); len(out) != 1 || out[0].Call.Event != "gather" {
		t.Fatal(out)
	}
	s.MarkRan("gather")
	if out := dispatch(); len(out) != 0 {
		t.Fatal("same-day exclusion", out)
	}
	s.DB.Game.Planning.Schedule[0].When = value(t, "false")
	s.Planning.Date = "2030-08-01"
	if out := dispatch(); len(out) != 1 || out[0].Call.Event != "later" {
		t.Fatal("dependent scene", out)
	}
}

// applyUnbound exercises the same entry point as events without call arguments.
func applyUnbound(s *rpg.State, c data.PlanCommand) ([]data.Command, error) {
	return planning.ApplyIn(s, c, nil)
}

func TestChildBudgetOverflowIsAtomic(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	apply(t, s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "mix"}, Value: value(t, "pow(10, 308)")})
	before := snapshot(t, s)
	_, err := applyUnbound(s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "knead"}, Value: value(t, "pow(10, 308)")})
	if err == nil {
		t.Fatal("accepted child budgets whose total overflows")
	}
	if got := snapshot(t, s); string(got) != string(before) {
		t.Fatal("failed budget change modified state")
	}
}

func FuzzPlanningTransitions(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 64 {
			return
		}
		s := state(t)
		s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
		s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
		s.Planning.Activities["mix"] = &rpg.ActivityState{}
		s.Planning.Activities["knead"] = &rpg.ActivityState{}
		amounts := []string{"0", "1", "-1", "0.1", "0.000000000001", "pow(10, 308)", "pow(10, -308)", "2", "3"}
		ids := []string{"prepare", "mix", "knead"}
		for i, step := range steps {
			c := data.PlanCommand{Activity: data.IDRef{ID: ids[int(step)/7%len(ids)]}}
			switch step % 7 {
			case 0:
				c.Op = "split"
			case 1:
				c.Op, c.Value = "progress", value(t, amounts[int(step)%len(amounts)])
			case 2:
				c.Op, c.Value = "effort", value(t, amounts[int(step)%len(amounts)])
			case 3:
				c.Op = "observe"
			case 4:
				c.Op, c.Value = "cancel", value(t, "true")
			case 5:
				c.Op, c.Value = "cancel", value(t, "false")
			case 6:
				c.Op, c.Activity = "advance", data.IDRef{}
			}
			before := snapshot(t, s)
			_, applyErr := applyUnbound(s, c)
			saved, err := s.Save()
			if err != nil {
				t.Fatalf("step %d (%+v) left unsavable state: %v", i, c, err)
			}
			if applyErr != nil && string(before) != string(saved) {
				t.Fatalf("step %d failed but changed state: %v", i, applyErr)
			}
			if _, err := rpg.Parse(s.DB, saved, nil); err != nil {
				t.Fatalf("step %d (%+v) cannot reload: %v", i, c, err)
			}
		}
	})
}

func TestTinyActivityEffortDoesNotCompleteWithoutWork(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	apply(t, s, data.PlanCommand{Op: "effort", Activity: data.IDRef{ID: "prepare"}, Value: value(t, "0.000000000001")})
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	apply(t, s, data.PlanCommand{Op: "observe"})
	for _, id := range []string{"prepare", "mix", "knead"} {
		a := s.Planning.Activities[id]
		if a.Progress != 0 || a.Completed != "" {
			t.Errorf("%s completed without work: %+v", id, a)
		}
	}
}

func TestParentProgressUpdatesUnhiddenChildObservations(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	for _, amount := range []string{"1", "-.5", "2"} {
		apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, amount)})
		for _, id := range []string{"prepare", "mix", "knead"} {
			a := s.Planning.Activities[id]
			if a.Observed != a.Progress {
				t.Errorf("%s after %s: observed %g differs from unhidden progress %g", id, amount, a.Observed, a.Progress)
			}
		}
	}
}

// splitWorkshop is the workshop with prepare split into mix (0.5) and knead (1.5, with the
// after it is given).
func splitWorkshop(t *testing.T, kneadAfter ...string) *rpg.State {
	t.Helper()
	s := state(t)
	s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
	s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5, After: kneadAfter}
	s.Planning.Activities["mix"] = &rpg.ActivityState{}
	s.Planning.Activities["knead"] = &rpg.ActivityState{}
	apply(t, s, data.PlanCommand{Op: "split", Activity: data.IDRef{ID: "prepare"}})
	return s
}

// wantProgress compares the progress of prepare, mix and knead.
func wantProgress(t *testing.T, s *rpg.State, step string, prepare, mix, knead float64) {
	t.Helper()
	a := s.Planning.Activities
	for id, want := range map[string]float64{"prepare": prepare, "mix": mix, "knead": knead} {
		if math.Abs(a[id].Progress-want) > 1e-9 {
			t.Fatalf("%s: prepare %g, mix %g, knead %g, want %g, %g, %g", step, a["prepare"].Progress, a["mix"].Progress, a["knead"].Progress, prepare, mix, knead)
		}
	}
}

// TestParentProgressSkipsCancelledChildren: work and rework given to a split parent go
// to its children that are not cancelled, as work given to a cancelled child is ignored.
func TestParentProgressSkipsCancelledChildren(t *testing.T) {
	s := splitWorkshop(t)
	progress := func(id, amount string) {
		apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: id}, Value: value(t, amount)})
	}
	progress("knead", "0.6")
	apply(t, s, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "knead"}, Value: value(t, "true")})
	progress("prepare", "0.3")
	wantProgress(t, s, "work with knead cancelled", 0.9, 0.3, 0.6)
	progress("prepare", "-0.2")
	wantProgress(t, s, "rework with knead cancelled", 0.7, 0.1, 0.6)
	progress("prepare", "3")
	wantProgress(t, s, "more work than mix has room for", 1.1, 0.5, 0.6)
	apply(t, s, data.PlanCommand{Op: "cancel", Activity: data.IDRef{ID: "knead"}, Value: value(t, "false")})
	progress("prepare", "0.3")
	wantProgress(t, s, "work with knead back", 1.4, 0.5, 0.9)
}

// TestParentProgressSkipsChildrenWaitingOnDependency: work given to a split parent does
// not reach a child whose after is not done, as work given to that child is ignored.
func TestParentProgressSkipsChildrenWaitingOnDependency(t *testing.T) {
	s := splitWorkshop(t, "mix")
	progress := func(amount string) {
		apply(t, s, data.PlanCommand{Op: "progress", Activity: data.IDRef{ID: "prepare"}, Value: value(t, amount)})
	}
	progress("0.4")
	wantProgress(t, s, "mix not done", 0.4, 0.4, 0)
	progress("1")
	wantProgress(t, s, "mix done by this work", 0.5, 0.5, 0)
	progress("1")
	wantProgress(t, s, "mix done before this work", 1.5, 0.5, 1)
	progress("-0.6")
	wantProgress(t, s, "rework", 0.9, 0.3, 0.6)
}

// TestRandomDecomposition: random sequences of activity operations keep the split parent
// the sum of its children, keep the state saveable, and give a cancelled child no work
// (seed 9 once gave a cancelled child a share of its parent's work).
func TestRandomDecomposition(t *testing.T) {
	amounts := []string{"0", "1", "-1", "0.1", "0.3", "-0.3", "0.7", "2", "3", "0.05"}
	ids := []string{"prepare", "mix", "knead"}
	failures := map[string]bool{}
	for seed := uint64(1); seed <= 20; seed++ {
		rng := rand.New(rand.NewPCG(seed, 7)) //nolint:gosec // A seeded generator, so a failing sequence can be replayed.
		s := state(t)
		s.DB.Game.Planning.Activities["mix"] = data.Activity{Name: "Mix", Parent: "prepare", Effort: .5}
		s.DB.Game.Planning.Activities["knead"] = data.Activity{Name: "Knead", Parent: "prepare", Effort: 1.5}
		s.Planning.Activities["mix"] = &rpg.ActivityState{}
		s.Planning.Activities["knead"] = &rpg.ActivityState{}
		var log []string
		for range 12 {
			c := data.PlanCommand{Activity: data.IDRef{ID: ids[rng.IntN(len(ids))]}}
			amount := amounts[rng.IntN(len(amounts))]
			switch rng.IntN(6) {
			case 0:
				c.Op, c.Activity = "split", data.IDRef{ID: "prepare"}
			case 1, 2:
				c.Op, c.Value = "progress", value(t, amount)
			case 3:
				c.Op, c.Value = "effort", value(t, amount)
			case 4:
				c.Op, c.Value = "cancel", value(t, []string{"true", "false"}[rng.IntN(2)])
			case 5:
				c.Op = "observe"
			}
			log = append(log, c.Op+" "+c.Activity.ID+" "+amount)
			was := map[string]rpg.ActivityState{}
			for id, a := range s.Planning.Activities {
				was[id] = *a
			}
			if _, err := applyUnbound(s, c); err != nil {
				continue
			}
			report := func(kind, format string, args ...any) {
				if !failures[kind] {
					failures[kind] = true
					t.Errorf("seed %d %s: %s\n  steps: %s", seed, kind, fmt.Sprintf(format, args...), strings.Join(log, "; "))
				}
			}
			if _, err := s.Save(); err != nil {
				report("unsaveable", "%v", err)
			}
			p := s.Planning.Activities["prepare"]
			if p.Split {
				sum := s.Planning.Activities["mix"].Progress + s.Planning.Activities["knead"].Progress
				if math.Abs(sum-p.Progress) > 1e-9 {
					report("parent is not the sum", "parent %g, children %g", p.Progress, sum)
				}
				for _, id := range []string{"mix", "knead"} {
					a := s.Planning.Activities[id]
					if w := was[id]; w.Cancelled && a.Cancelled && !was["prepare"].Cancelled && a.Progress > w.Progress+1e-12 && c.Op == "progress" {
						report("cancelled child worked", "%s cancelled went %g -> %g", id, w.Progress, a.Progress)
					}
				}
				if c.Op == "progress" && c.Activity.ID == "prepare" && !was["prepare"].Cancelled && was["prepare"].Split && !was["mix"].Cancelled && !was["knead"].Cancelled {
					n, err := strconv.ParseFloat(amount, 64)
					if err != nil {
						t.Fatal(err)
					}
					want := math.Max(0, math.Min(s.ActivityEffort("prepare"), was["prepare"].Progress+n))
					if math.Abs(p.Progress-want) > 1e-9 {
						report("parent work lost", "parent %g -> %g by %s, want %g (mix %+v knead %+v; was mix %+v knead %+v)", was["prepare"].Progress, p.Progress, amount, want,
							*s.Planning.Activities["mix"], *s.Planning.Activities["knead"], was["mix"], was["knead"])
					}
				}
			}
		}
	}
}

// TestDispatchMarksOfPastDaysAreDropped: the marks of the reservations dispatched on a
// day are only read on that day, so the saved state keeps today's and no earlier day's.
func TestDispatchMarksOfPastDaysAreDropped(t *testing.T) {
	s := state(t)
	s.DB.Game.Planning.Schedule = []data.Reservation{{ID: "daily", Phase: "morning", Event: "gather"}}
	days := 0
	for !s.Planning.Finished {
		if s.Planning.Phase == 0 {
			days++
		}
		if out := apply(t, s, data.PlanCommand{Op: "dispatch"}); (len(out) == 1) != (s.Planning.Phase == 0) {
			t.Fatalf("%s phase %d: dispatched %v, want the reservation once a day", s.Planning.Date, s.Planning.Phase, out)
		}
		apply(t, s, data.PlanCommand{Op: "advance"})
	}
	if n := len(s.Planning.Dispatched); n > 1 {
		t.Fatalf("%d dispatch marks saved after %d days; only today's can be read: %v", n, days, s.Planning.Dispatched)
	}
}
