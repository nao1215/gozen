package play

import (
	"strings"
	"testing"
)

// Every command parseScript accepts has a row in the verb table, so step can
// run whatever a script holds.
func TestScriptVerbTableCoversParsedCommands(t *testing.T) {
	lines := []string{
		"battle-log on", "battle-hp enemy 0 5", "do set: {flag.met: true}", "order raid", "end_turn",
		"type hello", "skip 3", "pick Start", "memo note text", "hold ok 2", "wait 1", "read 4", "gap 2",
		"drag 1 2 3 4", "click 1 2", "window 320 240", "wait-graphs", "press ok", "shot title", "until",
		"quit", "guides on", "look", "mark here", "snapshot", "recruit slime 3", "guarantee-recruit slime",
		"walk door", "face door", "jump intro", "lines intro", "play intro",
	}
	s, err := parseScript(strings.Join(lines, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.cmds) != len(lines) {
		t.Fatalf("parsed %d commands from %d lines", len(s.cmds), len(lines))
	}
	for i, c := range s.cmds {
		if v, ok := scriptVerbOf(c.verb); !ok || v.run == nil {
			t.Errorf("%q: no row for %q", lines[i], c.verb)
		}
	}
}

// A command without a row stops the script with an error instead of spinning.
func TestScriptStepRefusesAnUnknownVerb(t *testing.T) {
	s := &script{cmds: []scriptCmd{{verb: "dance", n: 1}}}
	s.step(&input{})
	if s.err == nil || !strings.Contains(s.err.Error(), "dance") {
		t.Fatalf("err %v", s.err)
	}
}

// The commands that only call a hook either rest before the next command or let
// it run in the same tick.
func TestScriptHookVerbsRestOrRunOn(t *testing.T) {
	var got []string
	s, err := parseScript("mark a\nmark b\njump intro 2\nmark c")
	if err != nil {
		t.Fatal(err)
	}
	s.mark = func(name string) { got = append(got, name) }
	s.jump = func(name string, _ int) { got = append(got, name) }
	s.step(&input{})
	if strings.Join(got, ",") != "a,b,intro" || s.wait != 2 || s.pc != 3 {
		t.Fatalf("after the first tick: %v wait %d pc %d", got, s.wait, s.pc)
	}
}
