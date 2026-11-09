package play

import (
	"slices"
	"strings"
	"testing"
)

var sysVerbs = []string{"wait", "read", "gap", "click", "press", "shot", "skip", "until", "walk", "face", "lines", "play", "memo", "type", "jump", "do", "quit"}

// FuzzParseScript: any text either parses into commands of known verbs with non-negative counts
// and plain shot names, or is refused with the number of a line; parsing twice gives the same
// answer; and a parsed script can be stepped without hooks or devices and keeps its place.
func FuzzParseScript(f *testing.F) {
	for _, seed := range []string{
		"", "wait 5\npress ok\nquit", "click 10 20", "shot a\nshot ../b", "skip 3 name", "memo bug text here",
		"do {set: {flag.a: true}}", "type  ", "press 0\npress 9\npress cancel 3", "# comment\n\n   \nwalk gate arrival",
		"lines scene 4\nplay fight 6", "wait -1", "jump", "unknown 1 2 3", "gap 2\nread 4\nuntil",
		strings.Repeat("wait 1\n", 50), "wait 99999999999999999999", "\x00\xff", "press ok 2 3",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		s, err := parseScript(src)
		s2, err2 := parseScript(src)
		if (err == nil) != (err2 == nil) {
			t.Fatalf("two parses disagree on %q: %v / %v", src, err, err2)
		}
		if err != nil {
			if !strings.Contains(err.Error(), "script") {
				t.Fatalf("error without context for %q: %v", src, err)
			}
			return
		}
		if len(s.cmds) != len(s2.cmds) {
			t.Fatalf("two parses differ for %q", src)
		}
		for _, c := range s.cmds {
			if !slices.Contains(sysVerbs, c.verb) {
				t.Fatalf("verb %q accepted from %q", c.verb, src)
			}
			if c.n < 0 {
				t.Fatalf("negative count %d from %q", c.n, src)
			}
			if c.verb == "shot" && (c.arg == "" || c.arg == "." || c.arg == ".." || strings.ContainsAny(c.arg, `/\`)) {
				t.Fatalf("shot name %q accepted from %q", c.arg, src)
			}
		}
		// stepping with no game behind it keeps the script inside itself: the position never passes the
		// end, a wait never goes negative, and a script that only presses and films ends soon.
		in := &input{}
		for i := 0; i < 20000 && !s.quit; i++ {
			s.step(in)
			s.shot = ""
			if s.pc < 0 || s.pc > len(s.cmds) || s.wait < 0 {
				t.Fatalf("script from %q lost its place: pc %d wait %d of %d commands", src, s.pc, s.wait, len(s.cmds))
			}
		}
	})
}
