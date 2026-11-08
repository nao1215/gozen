package play

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer3/casual"
)

func TestRunnerControlsGestures(t *testing.T) {
	var controls runnerControls
	in := input{runnerCapture: true, runnerScale: 1}
	step := func(samples ...runnerPointerSample) casual.Input {
		in.runnerPointers = samples
		return controls.advance(&in, 400, 600)
	}
	p := func(id int, x, y float64) runnerPointerSample { return runnerPointerSample{ID: id, X: x, Y: y} }
	step(p(1, 100, 200))
	if got := step(p(1, 111, 200)); got.Steer != 0 {
		t.Fatal("steered inside deadzone")
	}
	if got := step(p(1, 112, 200)); got.Steer != 1 {
		t.Fatal("did not steer at threshold")
	}
	if got := step(p(1, 200, 200)); got.Steer != 1 {
		t.Fatal("did not follow anchor")
	}
	if got := step(p(1, 139, 200)); got.Steer != -1 {
		t.Fatal("reversal did not follow shifted anchor")
	}
	step()
	controls.reset()
	step(p(1, 100, 200))
	if got := step(p(1, 112, 188)); got.Steer != 1 || got.Boost {
		t.Fatalf("dominant axis tie: %+v", got)
	}
	if got := step(p(1, 112, 187)); got.Steer != 0 || !got.Boost {
		t.Fatalf("up gesture: %+v", got)
	}
	step()
	controls.reset()
	step(p(1, 100, 200))
	step(p(1, 120, 200), p(2, 200, 300))
	if got := step(p(1, 120, 200)); got.Action || got.Steer != 1 {
		t.Fatalf("single second-finger tap: %+v", got)
	}
	step(p(1, 120, 200), p(2, 200, 300))
	if got := step(p(1, 120, 200)); !got.Action || got.Steer != 1 {
		t.Fatalf("double tap while steering: %+v", got)
	}
	if got := step(p(1, 120, 200)); got.Action {
		t.Fatal("action repeated without second tap")
	}
	controls.reset()
	in.runnerScale = 2
	step(p(1, 100, 200))
	if got := step(p(1, 123, 200)); got.Steer != 0 {
		t.Fatal("DIP threshold did not scale")
	}
	if got := step(p(1, 124, 200)); got.Steer != 1 {
		t.Fatal("scaled threshold failed")
	}
}

func TestRunnerControlsCancellationAndPause(t *testing.T) {
	for _, outside := range []runnerPointerSample{{ID: 1, X: -1, Y: 100}, {ID: 1, X: 400, Y: 100}, {ID: 1, X: 100, Y: 600}, {ID: 1, X: math.NaN(), Y: 100}} {
		var controls runnerControls
		in := input{runnerCapture: true, runnerScale: 1, runnerPointers: []runnerPointerSample{{ID: 1, X: 100, Y: 100}}}
		controls.advance(&in, 400, 600)
		in.runnerPointers = nil
		controls.advance(&in, 400, 600)
		in.runnerPointers = []runnerPointerSample{outside}
		if got := controls.advance(&in, 400, 600); got != (casual.Input{}) {
			t.Fatalf("outside gesture produced input %+v", got)
		}
		in.runnerPointers = nil
		if got := controls.advance(&in, 400, 600); got.Action {
			t.Fatal("cancelled gesture became doubletap")
		}
	}
	var controls runnerControls
	in := input{runnerCapture: true, runnerScale: 1, runnerPointers: []runnerPointerSample{{ID: 1, X: 100, Y: 100}, {ID: 2, X: 380, Y: 20}}}
	controls.advance(&in, 400, 600)
	in.runnerPointers = in.runnerPointers[:1]
	if got := controls.advance(&in, 400, 600); got.Action || !controls.pause {
		t.Fatal("second finger pause was not isolated from action")
	}
	in.runnerCancel = true
	in.pressed[btnOK] = true
	if got := controls.advance(&in, 400, 600); got != (casual.Input{}) {
		t.Fatal("explicit cancellation leaked keyboard action")
	}
	if len(controls.pointers) != 0 || controls.lastTap != 0 {
		t.Fatal("cancel retained gesture")
	}
}

func TestRunnerPhonePointers(t *testing.T) {
	src := input{runnerCapture: true, runnerScale: 2, runnerPointers: []runnerPointerSample{{ID: 3, X: 110, Y: 220}, {ID: 4, X: 130, Y: 240}}}
	var dst input
	transformRunnerPointers(&dst, &src, 100, 200, 100, 200, 200, 400)
	if dst.runnerCancel || dst.runnerScale != 4 || !reflect.DeepEqual(dst.runnerPointers, []runnerPointerSample{{ID: 3, X: 20, Y: 40}, {ID: 4, X: 60, Y: 80}}) {
		t.Fatalf("phone transform: %+v", dst)
	}
	dst.runnerPointers[0].X = 0
	if src.runnerPointers[0].X != 110 {
		t.Fatal("child input aliases parent")
	}
	src.runnerPointers[1].X = 200
	transformRunnerPointers(&dst, &src, 100, 200, 100, 200, 200, 400)
	if !dst.runnerCancel {
		t.Fatal("outside phone pointer was treated as release")
	}
}

func TestRunnerPointerTraceRoundTrip(t *testing.T) {
	var original, replayed runnerControls
	frames := [][]runnerPointerSample{
		{{ID: 1, X: 100, Y: 200}},
		{{ID: 1, X: 130, Y: 200}, {ID: 2, X: 220, Y: 300}},
		{{ID: 1, X: 130, Y: 200}},
		{{ID: 1, X: 130, Y: 200}, {ID: 2, X: 220, Y: 300}},
		{{ID: 1, X: 130, Y: 200}},
		nil,
	}
	for i, pointers := range frames {
		in := input{runnerCapture: true, runnerScale: 1, runnerPointers: pointers}
		want := original.advance(&in, 400, 600)
		var buf bytes.Buffer
		g := &Game{tr: newTracer(&buf), tick: i + 1}
		g.traceInput(&in)
		var line traceLine
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		r := &replay{inputs: map[int]traceLine{i + 1: line}}
		var restored input
		r.feed(&restored, i+1)
		if !reflect.DeepEqual(restored.runnerPointers, pointers) || restored.runnerScale != 1 || !restored.runnerCapture {
			t.Fatalf("pointer state changed: %+v", restored)
		}
		if got := replayed.advance(&restored, 400, 600); got != want {
			t.Fatalf("tick %d got %+v want %+v", i, got, want)
		}
	}
	for _, raw := range []any{
		nil,
		map[string]any{"scale": 0.0, "cancel": false, "pointers": nil},
		map[string]any{"scale": 1.0, "cancel": "false", "pointers": nil},
		map[string]any{"scale": 1.0, "cancel": false, "pointers": []any{map[string]any{"id": 1.5, "x": 1.0, "y": 2.0}}},
		map[string]any{"scale": 1.0, "cancel": false, "pointers": []any{map[string]any{"id": 1.0, "x": 1.0, "y": 2.0}, map[string]any{"id": 1.0, "x": 2.0, "y": 3.0}}},
		map[string]any{"scale": 1.0, "cancel": false, "pointers": make([]any, 5)},
	} {
		if _, err := parseRunnerInput(raw); err == nil {
			t.Fatalf("accepted malformed input %#v", raw)
		}
	}
}

func TestRunnerSyntheticMouseSuppression(t *testing.T) {
	in := input{runnerCapture: true, click: true, pdown: true, pheld: true}
	in.quietRunnerMouse(true)
	if in.click || in.pdown || in.pheld {
		t.Fatal("touch leaked a synthetic mouse press")
	}
	for range 59 {
		in.click, in.pdown, in.pheld = true, true, true
		in.quietRunnerMouse(false)
		if in.click || in.pdown || in.pheld {
			t.Fatal("synthetic mouse was accepted too soon")
		}
	}
	in.click, in.pdown, in.pheld = true, true, true
	in.quietRunnerMouse(false)
	if !in.click || !in.pdown || !in.pheld {
		t.Fatal("real mouse remained suppressed after timeout")
	}
	ordinary := input{click: true, pheld: true}
	ordinary.quietRunnerMouse(true)
	if !ordinary.click || !ordinary.pheld {
		t.Fatal("runner adapter changed ordinary UI")
	}
}

func TestRunnerControlsAcknowledgeScript(t *testing.T) {
	for _, command := range []string{"hold left 3", "hold up 3", "press ok", "drag 100 200 140 200"} {
		t.Run(command, func(t *testing.T) {
			s := sysScript(t, command+"\nquit")
			s.acknowledge = true
			in := input{script: s, runnerCapture: true, runnerScale: 1}
			var controls runnerControls
			for range 100 {
				in.update()
				controls.advance(&in, 400, 600)
				in.finishOffer()
				if s.quit || s.err != nil {
					break
				}
			}
			if s.err != nil || !s.quit {
				t.Fatalf("runner did not consume %s: pc=%d err=%v", command, s.pc, s.err)
			}
		})
	}
	s := sysScript(t, "press down\nquit")
	s.acknowledge = true
	in := input{script: s, runnerCapture: true}
	var controls runnerControls
	for range 3 {
		in.update()
		controls.advance(&in, 400, 600)
		in.finishOffer()
	}
	if s.pc != 0 {
		t.Fatal("runner acknowledged unused down key")
	}
}

func TestRunnerOverlappingDirections(t *testing.T) {
	var c runnerControls
	in := input{runnerCapture: true, runnerScale: 1}
	for _, step := range []struct {
		left, right bool
		want        float64
	}{{true, false, -1}, {true, true, 1}, {false, true, 1}, {true, true, -1}, {true, false, -1}, {false, false, 0}} {
		in.held[btnLeft], in.held[btnRight] = step.left, step.right
		if got := c.advance(&in, 400, 600).Steer; got != step.want {
			t.Fatalf("left=%v right=%v got%v want%v", step.left, step.right, got, step.want)
		}
	}
	c.reset()
	in.held[btnLeft], in.held[btnRight] = true, true
	if got := c.advance(&in, 400, 600).Steer; got != 0 {
		t.Fatal("simultaneous first press must be neutral")
	}
	c.reset()
	in.held[btnRight] = false
	in.runnerPointers = []runnerPointerSample{{ID: 1, X: 100, Y: 200}}
	c.advance(&in, 400, 600)
	in.runnerPointers[0].X = 130
	if got := c.advance(&in, 400, 600).Steer; got != 1 {
		t.Fatal("new touch direction must win over held keyboard")
	}
	in.runnerPointers = nil
	if got := c.advance(&in, 400, 600).Steer; got != -1 {
		t.Fatal("lifting touch must preserve keyboard direction")
	}
}

func TestRunnerBindingsAndCombinedPressEdges(t *testing.T) {
	in := input{runnerCapture: true}
	if got := in.runnerKeys(btnOK, keys[btnOK]); !reflect.DeepEqual(got, []ebiten.Key{ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyX}) {
		t.Fatal("runner confirm aliases changed")
	}
	if got := in.runnerKeys(btnCancel, keys[btnCancel]); !reflect.DeepEqual(got, runnerCancelKeys) {
		t.Fatal("runner cancellation aliases changed")
	}
	in.held[btnOK], in.pressed[btnOK] = true, true
	previous := in.held
	in.runnerButtonEdges(previous)
	if in.pressed[btnOK] {
		t.Fatal("another alias triggered the held action twice")
	}
	in.runnerButtonEdges([numButtons]bool{})
	if !in.pressed[btnOK] {
		t.Fatal("fresh action was lost")
	}
	in.runnerCapture = false
	if !reflect.DeepEqual(in.runnerKeys(btnOK, keys[btnOK]), keys[btnOK]) {
		t.Fatal("runner bindings changed ordinary UI")
	}
}

func TestRunnerInputOwnership(t *testing.T) {
	g := &Game{stack: []scene{&runnerScene{}}}
	g.captureRunnerInput()
	if !g.in.runnerCapture {
		t.Fatal("active runner did not own controls")
	}
	g.stack = append(g.stack, &title{})
	g.captureRunnerInput()
	if g.in.runnerCapture {
		t.Fatal("covered runner changed menu controls")
	}
	child := &Game{stack: []scene{&runnerScene{}}}
	g.phone = &phoneView{child: &phoneGameRuntime{game: child}}
	g.captureRunnerInput()
	if !g.in.runnerCapture {
		t.Fatal("host failed to collect child pointers")
	}
	child.stack = append(child.stack, &title{})
	g.captureRunnerInput()
	if g.in.runnerCapture {
		t.Fatal("host used child controls over a menu")
	}
}
