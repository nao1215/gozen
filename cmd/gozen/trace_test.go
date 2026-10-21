package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
	lang "github.com/nao1215/gozen/internal/text"
)

func traceFile(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trace.jsonl")
	write(t, p, strings.Join(lines, "\n")+"\n")
	return p
}

// Every kind of record has one line in the readable form: the time as MM:SS.ss, the kind, and what happened.
func TestPlayTracePrintsEveryKindOfRecord(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	file := traceFile(t,
		`{"t":"start","tick":0,"game":"sample","seed":7,"language":"en"}`,
		`{"t":"say","tick":60,"text":"Hello","who":"Mira","choices":["Yes","No"],"read":false}`,
		`{"t":"say","tick":61,"text":"Plain"}`,
		`{"t":"skip","tick":62,"on":true}`,
		`{"t":"skip","tick":63,"on":false,"why":"unread line"}`,
		`{"t":"skip","tick":64,"on":true,"scene":true,"event":"intro","line":3,"read":2,"lines":9}`,
		`{"t":"skip","tick":65,"on":false,"scene":true,"line":8}`,
		`{"t":"voice","tick":66,"file":"v/1.ogg","who":"Mira","missing":true}`,
		`{"t":"voice","tick":67,"file":"v/2.ogg"}`,
		`{"t":"jump","tick":68,"event":"cave","line":4}`,
		`{"t":"dev_set","tick":69,"do":"flag.a = true"}`,
		`{"t":"dev_do","tick":70,"do":"call x","error":"no such event"}`,
		`{"t":"rollback","tick":71,"event":"intro","from":9,"to":6}`,
		`{"t":"rollback","tick":72,"from":2,"to":1,"back_into":true}`,
		`{"t":"resume","tick":73,"event":"intro","line":5,"at":"say"}`,
		`{"t":"notice","tick":74,"text":"Saved"}`,
		`{"t":"answer","tick":75,"index":1}`,
		`{"t":"scene","tick":76,"stack":["title","field"],"location":"camp"}`,
		`{"t":"battle","tick":77,"text":"The foe appears."}`,
		`{"t":"battle","tick":78,"text":"x","cutin":"Spark"}`,
		`{"t":"battle_end","tick":79,"result":"won"}`,
		`{"t":"menu","tick":80,"items":["Items","Status","Quit"],"sel":1}`,
		`{"t":"menu","tick":81,"items":[]}`,
		`{"t":"view","tick":82,"pictures":["a.png","b.png"],"missing":["b.png"],"shot":"s1"}`,
		`{"t":"view","tick":83,"pictures":["b.png","c.png"]}`,
		`{"t":"state","tick":84,"gold":50,"location":"cave","floor":2,"pos":"3,4","facing":"north","party":[{"name":"Mira","lv":3,"hp":10,"mhp":20,"mp":1,"mmp":5},{"name":"Rin","back":true,"lv":2,"hp":5,"mhp":5,"mp":0,"mmp":0}]}`,
		`{"t":"state","tick":85,"gold":0,"location":"camp","party":[]}`,
		`{"t":"music","tick":86,"song":"calm","module":"m1","asked":"field","mood":"day"}`,
		`{"t":"music","tick":87,"song":"calm"}`,
		`{"t":"battle_capture","tick":88,"number":2,"capture":{"troop":"pack"}}`,
		`{"t":"battle_result","tick":89,"result":"won","gains":{"exp":5}}`,
		`{"t":"act","tick":90,"name":"quit","args":{"x":1}}`,
		`{"t":"request","tick":91,"kind":"choice","n":1}`,
		`{"t":"input","tick":92,"key":"z"}`,
		`{"t":"problem","tick":93,"text":"oops"}`,
		`{"t":"shot","tick":94,"name":"s"}`,
		`{"t":"diverge","tick":95,"why":"x"}`,
		`{"t":"end","tick":96}`,
		`{"t":"custom","tick":97,"anything":1}`,
	)
	out, stderr := invoke(t, 0, "play", "trace", file)
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
	lines := strings.Split(out, "\n")
	byPrefix := func(prefix string) string {
		for _, l := range lines {
			if strings.HasPrefix(l, prefix) {
				return l
			}
		}
		return ""
	}
	checks := []struct{ prefix, want string }{
		{"00:00.00 start", "sample seed 7"},
		{"00:01.00 say", `Mira"Hello" ? Yes / No  [unread]`},
		{"00:01.02 say", "Plain"},
		{"00:01.03 skip", "skip on"},
		{"00:01.05 skip", "skip off (unread line)"},
		{"00:01.07 skip", "scene skip from intro line 3 (2 of 9 lines read)"},
		{"00:01.08 skip", "scene skip off at line 8"},
		{"00:01.10 voice", "Mira: v/1.ogg  MISSING"},
		{"00:01.12 voice", "v/2.ogg"},
		{"00:01.13 jump", "to cave line 4"},
		{"00:01.15 dev_set", "flag.a = true"},
		{"00:01.17 dev_do", "call x  ERROR no such event"},
		{"00:01.18 rollback", "back from line 9 to line 6 of intro"},
		{"00:01.20 rollback", "back from line 2 to line 1 of (an arrival) (the event before)"},
		{"00:01.22 resume", "load at line 5 of intro (reached say)"},
		{"00:01.23 notice", "notice: Saved"},
		{"00:01.25 answer", "choice #2"},
		{"00:01.27 scene", "title > field  @camp"},
		{"00:01.28 battle", "The foe appears."},
		{"00:01.30 battle", "[cut-in] Spark"},
		{"00:01.32 battle_end", "won"},
		{"00:01.33 menu", "Items | [Status] | Quit"},
		{"00:01.35 menu", "(closed)"},
		{"00:01.37 view", "+a.png +b.png  MISSING b.png  [s1]"},
		{"00:01.38 view", "+c.png -a.png"},
		{"00:01.40 state", "50G @cave B2F (3,4) north  Mira Lv3 10/20 1/5, Rin (back) Lv2 5/5 0/0"},
		{"00:01.43 music", "calm / m1  (asked field/day)"},
		{"00:01.45 music", "calm"},
		{"00:01.47 battle_capture", "battle 2 troop pack"},
		{"00:01.48 battle_result", "result won"},
		{"00:01.50 act", `quit {"x":1}`},
		{"00:01.52 request", `{"kind":"choice","n":1}`},
		{"00:01.53 input", ""},
		{"00:01.55 problem", `{"text":"oops"}`},
		{"00:01.57 shot", `{"name":"s"}`},
		{"00:01.58 diverge", `{"why":"x"}`},
		{"00:01.60 end", "{}"},
		{"00:01.62 custom", `"anything":1`},
	}
	for _, c := range checks {
		got := byPrefix(c.prefix)
		if c.prefix == "00:01.53 input" {
			if got != "" {
				t.Errorf("input is hidden without --input: %q", got)
			}
			continue
		}
		if got == "" || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q lacks %q", c.prefix, got, c.want)
		}
	}
	tail := lines[len(lines)-2]
	if !strings.HasPrefix(tail, "lines: ") || !strings.Contains(tail, "say 2") || !strings.Contains(tail, "skip 4") || !strings.Contains(tail, "input 1") {
		t.Errorf("the summary counts every record, shown or not: %q", tail)
	}
}

func TestPlayTraceFilters(t *testing.T) {
	file := traceFile(t,
		`{"t":"start","tick":0,"game":"g","seed":1,"language":"en"}`,
		`{"t":"view","tick":30,"pictures":["a.png"]}`,
		`{"t":"say","tick":60,"text":"one"}`,
		`{"t":"input","tick":90,"key":"z"}`,
		`{"t":"view","tick":120,"pictures":["a.png","b.png"]}`,
		`{"t":"say","tick":180,"text":"two"}`,
		`{"t":"say","tick":240,"text":"three"}`,
	)
	out, _ := invoke(t, 0, "play", "trace", file, "--only", "say")
	if strings.Count(out, " say        ") != 3 || strings.Contains(out, "start      ") || strings.Contains(out, "view       ") {
		t.Errorf("--only say: %q", out)
	}
	if !strings.Contains(out, "lines: input 1, say 3, start 1, view 2") {
		t.Errorf("filters do not change the count: %q", out)
	}
	out, _ = invoke(t, 0, "play", "trace", file, "--from", "2", "--to", "3")
	if !strings.Contains(out, "00:02.00 view") || !strings.Contains(out, "00:03.00 say") || strings.Contains(out, "one") || strings.Contains(out, "three") {
		t.Errorf("--from 2 --to 3: %q", out)
	}
	// A view before the range still counts as shown, so only the difference is printed.
	if !strings.Contains(out, "+b.png") || strings.Contains(out, "+a.png") {
		t.Errorf("the view diff must start from what was on screen: %q", out)
	}
	out, _ = invoke(t, 0, "play", "trace", file, "--input", "--only", "input,say")
	if !strings.Contains(out, `{"key":"z"}`) {
		t.Errorf("--input: %q", out)
	}
	for _, args := range [][]string{
		{"--from", "-1"}, {"--to", "-1"}, {"--from", "5", "--to", "2"}, {"--from", "NaN"}, {"--to", "Inf"}, {"--music-late", "-1"},
	} {
		invoke(t, 2, append([]string{"play", "trace", file}, args...)...)
	}
	invoke(t, 2, "play", "trace")
	invoke(t, 1, "play", "trace", filepath.Join(t.TempDir(), "absent.jsonl"))
}

func TestPlayTraceLanguageFollowsTheStartRecord(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	state := `{"t":"state","tick":0,"gold":1,"location":"a","party":[{"name":"X","back":true,"lv":1,"hp":1,"mhp":1,"mp":0,"mmp":0}]}`
	ja := traceFile(t, `{"t":"start","tick":0,"game":"g","seed":1,"language":"ja"}`, state)
	out, _ := invoke(t, 0, "play", "trace", ja)
	if !strings.Contains(out, "X後 Lv1") {
		t.Errorf("a Japanese trace shows its words: %q", out)
	}
	// A trace written before the language was recorded used the Japanese words.
	old := traceFile(t, `{"t":"start","tick":0,"game":"g","seed":1}`, state)
	if out, _ = invoke(t, 0, "play", "trace", old); !strings.Contains(out, "X後 Lv1") {
		t.Errorf("old trace: %q", out)
	}
	// An unknown language falls back to English.
	odd := traceFile(t, `{"t":"start","tick":0,"game":"g","seed":1,"language":"zz"}`, state)
	if out, _ = invoke(t, 0, "play", "trace", odd); !strings.Contains(out, "X (back) Lv1") {
		t.Errorf("unknown language: %q", out)
	}
}

func TestPlayTraceRejectsBrokenRecords(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"not json", `{`},
		{"not an object", `[1]`},
		{"no kind", `{"tick":1}`},
		{"empty kind", `{"t":"","tick":1}`},
		{"negative tick", `{"t":"say","tick":-1}`},
		{"tick not a number", `{"t":"say","tick":"1"}`},
		{"party not a list", `{"t":"state","tick":1,"party":3}`},
		{"member not an object", `{"t":"state","tick":1,"party":[1]}`},
		{"capture missing", `{"t":"battle_capture","tick":1}`},
	} {
		_, stderr := invoke(t, 1, "play", "trace", traceFile(t, tc.line))
		if !strings.Contains(stderr, diag.TraceBroken.String()) {
			t.Errorf("%s: %q", tc.name, stderr)
		}
	}
	_, stderr := invoke(t, 1, "play", "trace", traceFile(t, `{"t":"say","tick":1}`, `{`), "--check")
	if !strings.Contains(stderr, diag.TraceBroken.String()) {
		t.Errorf("--check with a broken line: %q", stderr)
	}
}
