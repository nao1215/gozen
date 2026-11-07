package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
	lang "github.com/nao1215/gozen/internal/text"
)

func TestTraceCheckStructuredDiagnosticCodes(t *testing.T) {
	file := traceFile(t, `{"t":"start","tick":0,"language":"en"}`, `{"t":"voice","tick":60,"file":"missing.ogg","missing":true}`)
	for _, format := range []string{"json", "jsonl"} {
		out, stderr := invoke(t, 1, "trace", file, "--check", "--format", format)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		var report struct {
			Issues []traceIssue `json:"issues"`
		}
		if err := json.Unmarshal(resultData(t, lines[len(lines)-1], "trace"), &report); err != nil {
			t.Fatal(err)
		}
		if stderr != "" || len(report.Issues) != 1 || report.Issues[0].Code != diag.VoiceFault.String() || report.Issues[0].Tick != 60 {
			t.Fatalf("%s lost the trace diagnostic: %s; %s", format, out, stderr)
		}
	}
}

func stand(since int, extra string) string {
	return fmt.Sprintf(`{"t":"stand","tick":%d,"since":%d%s}`, since, since, extra)
}

func figure(id string, x float64, flags string) string {
	return fmt.Sprintf(`{"id":%q,"face":"n","side":"left","x":%g,"alpha":1%s}`, id, x, flags)
}

func checkTraceOut(t *testing.T, want int, lines ...string) string {
	t.Helper()
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	all := append([]string{`{"t":"start","tick":0,"game":"g","seed":1,"language":"en"}`}, lines...)
	out, _ := invoke(t, want, "trace", traceFile(t, all...), "--check")
	return out
}

func TestTraceCheckFindsFaultsOfTheStandingPictures(t *testing.T) {
	sayA := `{"t":"say","tick":10,"who":"Mira","text":"Hello there"}`
	out := checkTraceOut(t, 1,
		sayA,
		// A figure that slides during one conversation.
		stand(20, `,"set":1,"speaker":"cast:mira","standing":true,"location":"camp","figures":[`+figure("cast:mira", 0.25, `,"speaking":true`)+`]`),
		stand(30, `,"set":1,"speaker":"cast:mira","standing":true,"location":"camp","figures":[`+figure("cast:mira", 0.5, `,"speaking":true`)+`]`),
		// The background changes, and a figure that did not speak stays on the stage.
		`{"t":"say","tick":40,"who":"Rin","text":"New place"}`,
		stand(50, `,"set":2,"speaker":"cast:rin","standing":true,"location":"cave","figures":[`+figure("cast:mira", 0.5, ``)+`,`+figure("cast:rin", 0.75, `,"speaking":true`)+`]`),
		// A picture over a full-screen picture.
		stand(60, `,"set":2,"speaker":"cast:rin","standing":true,"location":"cave","cg":"cg/a.png","cg_page":"cg/a.png","over":true,"figures":[`+figure("cast:rin", 0.75, `,"speaking":true`)+`]`),
		// The speaker has no picture.
		`{"t":"say","tick":70,"who":"Sam","text":"Where am I"}`,
		stand(80, `,"set":3,"speaker":"cast:sam","standing":true,"location":"cave","figures":[]`),
		// The location's background hides the stage.
		`{"t":"say","tick":90,"who":"Sam","text":"Hidden again"}`,
		stand(95, `,"set":3,"speaker":"cast:sam","standing":true,"location":"cave","cg":"cg/bg.png","cg_page":"cg/bg.png","hides":true,"cg_bg":true,"figures":[]`),
		// A speaker that is fading in or not standing is not a fault.
		stand(100, `,"set":3,"speaker":"cast:sam","standing":true,"fading":true,"location":"cave","figures":[]`),
		stand(101, `,"set":3,"speaker":"cast:sam","standing":false,"location":"cave","figures":[]`),
		`{"t":"end","tick":200}`,
	)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	find := func(kind string) string {
		for _, l := range lines {
			if f := strings.Fields(l); len(f) > 1 && f[1] == kind {
				return l
			}
		}
		return ""
	}
	if l := find("moved"); !strings.HasPrefix(l, "00:00.50") || !strings.Contains(l, "mira") || !strings.Contains(l, "0.25") || !strings.Contains(l, "0.50") || !strings.Contains(l, `Mira"Hello there"`) || !strings.Contains(l, diag.PictureFault.String()) {
		t.Errorf("moved: %q", l)
	}
	if l := find("left-over"); !strings.HasPrefix(l, "00:00.83") || !strings.Contains(l, "mira") || !strings.Contains(l, "@cave") || !strings.Contains(l, `Rin"New place"`) {
		t.Errorf("left-over: %q", l)
	}
	if l := find("over-cg"); !strings.HasPrefix(l, "00:01.00") || !strings.Contains(l, "cg/a.png") || !strings.Contains(l, "rin") {
		t.Errorf("over-cg: %q", l)
	}
	var noStand []string
	for _, l := range lines {
		if f := strings.Fields(l); len(f) > 1 && f[1] == "no-stand" {
			noStand = append(noStand, l)
		}
	}
	if len(noStand) != 2 || !strings.Contains(noStand[0], "sam") || !strings.Contains(noStand[1], "faces: true") || !strings.Contains(noStand[1], "cg/bg.png") {
		t.Errorf("no-stand: %q", noStand)
	}
	last := lines[len(lines)-1]
	if last != "check: 5 issues (4 lines, 8 stand records)" {
		t.Errorf("summary: %q", last)
	}
	if strings.Contains(out, diag.MusicFault.String()) {
		t.Errorf("no music fault was planted: %q", out)
	}
}

func TestTraceCheckCleanTraceExitsZero(t *testing.T) {
	out := checkTraceOut(t, 0,
		`{"t":"say","tick":10,"who":"Mira","text":"Hi"}`,
		stand(20, `,"set":1,"speaker":"cast:mira","standing":true,"location":"camp","figures":[`+figure("cast:mira", 0.25, `,"speaking":true`)+`]`),
		stand(30, `,"idle":true`),
		stand(40, `,"set":2,"speaker":"cast:mira","standing":true,"location":"camp","figures":[`+figure("cast:mira", 0.5, `,"speaking":true`)+`]`),
		`{"t":"end","tick":100}`,
	)
	if !strings.Contains(out, "check: 0") {
		t.Errorf("%q", out)
	}
}

func TestTraceCheckAWindowWithoutStandLines(t *testing.T) {
	out := checkTraceOut(t, 0, `{"t":"say","tick":10,"text":"Only text"}`)
	if !strings.Contains(out, "stand") || !strings.Contains(out, "check: 0") {
		t.Errorf("a trace without stand lines says the pictures were not checked: %q", out)
	}
	out = checkTraceOut(t, 0)
	if strings.Contains(out, "not checked") || strings.Contains(out, "stand 0") && strings.Contains(out, "say 1") {
		t.Errorf("an empty trace has nothing to warn about: %q", out)
	}
}

func TestTraceCheckSkipVoiceAndRollbackFaults(t *testing.T) {
	out := checkTraceOut(t, 1,
		`{"t":"say","tick":10,"who":"Mira","text":"Read already","read":true}`,
		`{"t":"voice","tick":11,"file":"v/missing.ogg","missing":true}`,
		`{"t":"rollback","tick":12,"from":3,"to":3}`,
		`{"t":"rollback","tick":13,"from":3,"to":2}`,
		`{"t":"rollback","tick":14,"from":1,"to":4,"back_into":true}`,
		`{"t":"skip","tick":20,"on":true}`,
		`{"t":"say","tick":21,"text":"Never seen","read":false}`,
		`{"t":"voice","tick":22,"file":"v/loud.ogg"}`,
		`{"t":"skip","tick":23,"on":false}`,
		`{"t":"voice","tick":24,"file":"v/fine.ogg"}`,
	)
	for _, want := range []struct{ kind, code, text string }{
		{"voice", diag.VoiceFault.String(), "v/missing.ogg"},
		{"rollback", diag.RollbackFault.String(), "3"},
		{"skip-unread", diag.SkipFault.String(), "Never seen"},
		{"skip-voice", diag.SkipFault.String(), "v/loud.ogg"},
	} {
		var hit string
		for l := range strings.SplitSeq(out, "\n") {
			if f := strings.Fields(l); len(f) > 1 && f[1] == want.kind {
				hit = l
			}
		}
		if hit == "" || !strings.Contains(hit, want.code) || !strings.Contains(hit, want.text) {
			t.Errorf("%s: %q (want %s and %q)\n%s", want.kind, hit, want.code, want.text, out)
		}
	}
	if strings.Count(out, " rollback ") != 1 {
		t.Errorf("only the rollback that did not go back is a fault (one that goes back into the event before may go forward in line numbers):\n%s", out)
	}
	if strings.Contains(out, "v/fine.ogg") {
		t.Errorf("a voice after skip mode is fine:\n%s", out)
	}
	if !strings.Contains(out, "check: 4") {
		t.Errorf("summary:\n%s", out)
	}
}

// TestTraceCheckSkipAllPassesUnreadLines: a skip line marked unread (the setting
// "Text to skip: all") lets lines not read pass without a fault; the next skip without
// the mark checks them again.
func TestTraceCheckSkipAllPassesUnreadLines(t *testing.T) {
	out := checkTraceOut(t, 1,
		`{"t":"skip","tick":20,"on":true,"unread":true}`,
		`{"t":"say","tick":21,"text":"Passed on purpose","read":false}`,
		`{"t":"skip","tick":22,"on":false,"why":"choice"}`,
		`{"t":"skip","tick":25,"scene":true,"on":true,"unread":true}`,
		`{"t":"say","tick":26,"text":"Scene passed on purpose","read":false}`,
		`{"t":"skip","tick":27,"scene":true,"on":false,"why":"choice"}`,
		`{"t":"skip","tick":30,"on":true}`,
		`{"t":"say","tick":31,"text":"Not read","read":false}`,
	)
	if strings.Contains(out, "assed on purpose") || !strings.Contains(out, "Not read") || !strings.Contains(out, "check: 1") {
		t.Errorf("only the line passed without the setting is a fault:\n%s", out)
	}
}

// TestTraceCheckBattleLines: a line said in a battle counts and is checked like any other.
func TestTraceCheckBattleLines(t *testing.T) {
	out := checkTraceOut(t, 1,
		`{"t":"skip","tick":20,"on":true}`,
		`{"t":"say","tick":21,"text":"Battle cry","read":false,"in_battle":true}`,
	)
	if !strings.Contains(out, diag.SkipFault.String()) || !strings.Contains(out, "Battle cry") || !strings.Contains(out, "check: 1") {
		t.Errorf("a battle line passed while skipping is a fault:\n%s", out)
	}
}

func TestTraceCheckMusicArrivesLateOrNever(t *testing.T) {
	// The default is half a second; the game speed divides the delay.
	out := checkTraceOut(t, 1,
		`{"t":"music","tick":10,"wanted":"calm","song":"","asked_at":10}`,
		`{"t":"music","tick":100,"wanted":"calm","song":"calm","wait":90}`,
	)
	if !strings.Contains(out, " music-late ") || !strings.Contains(out, diag.MusicFault.String()) || !strings.Contains(out, "1.5") {
		t.Errorf("a song that took 1.5 s: %q", out)
	}
	out = checkTraceOut(t, 0,
		`{"t":"music","tick":10,"wanted":"calm","song":"","asked_at":10}`,
		`{"t":"music","tick":20,"wanted":"calm","song":"calm","wait":20}`,
	)
	if strings.Contains(out, "music-late") {
		t.Errorf("0.33 s is not late: %q", out)
	}
	// A second song asked for before the first was heard.
	out = checkTraceOut(t, 1,
		`{"t":"music","tick":10,"wanted":"calm","song":"","asked_at":10}`,
		`{"t":"music","tick":200,"wanted":"storm","song":"calm"}`,
		`{"t":"end","tick":260}`,
	)
	if strings.Count(out, " music-late ") != 2 || !strings.Contains(out, `Song calm did not play for 3.2 s before the next song "storm" was requested`) || !strings.Contains(out, "Song storm was requested but never played until the end (1.0 s)") {
		t.Errorf("music asked for twice and never heard: %q", out)
	}
	// A faster game waits less: at speed 4, 100 ticks are 0.4 s.
	out = checkTraceOut(t, 0,
		`{"t":"start","tick":0,"game":"g","seed":1,"speed":4,"language":"en"}`,
		`{"t":"music","tick":10,"wanted":"calm","song":""}`,
		`{"t":"music","tick":110,"wanted":"calm","song":"calm","wait":100}`,
	)
	if strings.Contains(out, "music-late") {
		t.Errorf("speed 4: %q", out)
	}
	// --music-late sets the limit.
	file := traceFile(t,
		`{"t":"start","tick":0,"game":"g","seed":1,"language":"en"}`,
		`{"t":"music","tick":10,"wanted":"calm","song":""}`,
		`{"t":"music","tick":100,"wanted":"calm","song":"calm","wait":90}`,
	)
	invoke(t, 0, "trace", file, "--check", "--music-late", "2")
	invoke(t, 1, "trace", file, "--check", "--music-late", "1")
}

// These recorded observations do not define quality thresholds. Layout ratios
// use the actual trace field, including unreadable equal-color text and cropping.
func TestTraceFactsNeverFailCheck(t *testing.T) {
	records := []string{
		`{"t":"screen","tick":1,"controls":[{"label":"Wait","rect":[-1,0,50,20],"enabled":false,"reason":"cooldown","focused":true}],"texts":[{"text":"Note"}],"figures":[],"phone":"page"}`,
		`{"t":"section","tick":1,"name":"stress","scope":"mark","frames":1,"ticks":1,"fps":0.01,"tps":0.01,"slow_frame_share":1,"heap_inuse_bytes":9000000000,"allocations_per_frame":1000000}`,
		`{"t":"layout","tick":1,"since":0,"figures":[{"picture":"x.png","rect":[-100,-100,500,500],"content":[-100,-100,500,500],"clip":[0,0,10,10],"source_cut":["top","bottom"],"alpha":1}],"texts":[{"text":"Note","rect":[0,0,100,24],"size":24,"color":"#000000","back":"#000000","ratio":1}]}`,
		`{"t":"layout","tick":2,"since":1,"texts":[{"text":"Note","rect":[1000,1000,5000,5000],"size":24,"color":"#000000","back":"#ffffff","ratio":21}]}`,
		`{"t":"state","tick":1000000,"gold":0,"party":[{"name":"Guide","hp":0,"mhp":1000000,"mp":0,"mmp":100}]}`,
	}
	for _, record := range records {
		report, err := checkTrace(strings.NewReader(record+"\n"), 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.issues) != 0 {
			t.Fatalf("facts produced diagnostics: %+v", report.issues)
		}
		file := traceFile(t, record)
		for _, format := range []string{"text", "json", "jsonl"} {
			_, stderr := invoke(t, 0, "trace", file, "--check", "--format", format)
			if stderr != "" {
				t.Fatal(stderr)
			}
			_, stderr = invoke(t, 0, "trace", file, "--format", format)
			if stderr != "" {
				t.Fatal(stderr)
			}
		}
	}
}
