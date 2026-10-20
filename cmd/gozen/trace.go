package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"

	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/text/catalog"
)

func (c *command) traceCmd(args []string) (err error) {
	fl := clihelp.NewFlagSet("gozen play trace")
	only := fl.String("only", "", "print only these `KINDS`, separated by commas: say, scene, battle, menu, view, state, act, problem, and so on")
	from := fl.Float64("from", 0, "start at this `SECOND`")
	to := fl.Float64("to", 0, "stop at this `SECOND`; 0 is the end")
	input := fl.Bool("input", false, "also print the player's input")
	check := fl.Bool("check", false, "list the faults of the standing pictures and the music instead, and exit 1 if there are any")
	musicLate := fl.Float64("music-late", 0.5, "with --check, a song heard more than these `SECONDS` after it was asked for is late")
	if err := c.parse(fl, args); err != nil {
		return err
	}
	if fl.NArg() != 1 {
		return cli.Invalid(diag.Operands, "expected <trace.jsonl>")
	}
	inputPath := fl.Arg(0)
	if math.IsNaN(*from) || math.IsNaN(*to) || math.IsNaN(*musicLate) || math.IsInf(*from, 0) || math.IsInf(*to, 0) || math.IsInf(*musicLate, 0) || *from < 0 || *to < 0 || (*to > 0 && *to < *from) || *musicLate < 0 {
		return cli.Invalid(diag.BadValue, "invalid time range or music delay")
	}
	if *check {
		n, err := checkTrace(inputPath, *musicLate, c.out)
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("trace: %d issues", n)
		}
		return nil
	}
	f, err := os.Open(inputPath) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	kinds := map[string]bool{}
	for k := range strings.SplitSeq(*only, ",") {
		if k != "" {
			kinds[k] = true
		}
	}
	var shown []string
	count := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		l, err := decodeTrace(sc.Bytes())
		if err != nil {
			return err
		}
		k := str(l["t"])
		count[k]++
		sec := num(l["tick"]) / 60
		if sec < *from || (*to > 0 && sec > *to) || (len(kinds) > 0 && !kinds[k]) || (k == "input" && !*input) {
			if k == "view" {
				shown = strs(l["pictures"])
			}
			continue
		}
		at := fmt.Sprintf("%02d:%05.2f", int(sec)/60, sec-float64(int(sec)/60*60))
		var what string
		switch k {
		case "start":
			what = fmt.Sprintf("%v seed %.0f", l["game"], num(l["seed"]))
			if err := useTraceLanguage(l); err != nil {
				return err
			}
		case "say":
			what = str(l["text"])
			if w := str(l["who"]); w != "" {
				what = w + fmt.Sprintf(lang.T("「%s」"), what)
			}
			if c := strs(l["choices"]); len(c) > 0 {
				what += " ? " + strings.Join(c, " / ")
			}
			if read, ok := l["read"].(bool); ok && !read {
				what += "  [unread]"
			}
		case "skip":
			what = skipText(l)
		case "voice":
			what = str(l["file"])
			if w := str(l["who"]); w != "" {
				what = w + ": " + what
			}
			if l["missing"] == true {
				what += "  MISSING"
			}
		case "rollback":
			what = fmt.Sprintf("back from line %v to line %v of %s", l["from"], l["to"], cmp.Or(str(l["event"]), "(an arrival)"))
			if l["back_into"] == true {
				what += " (the event before)"
			}
		case "resume":
			what = fmt.Sprintf("load at line %v of %s (reached %v)", l["line"], cmp.Or(str(l["event"]), "(an arrival)"), l["at"])
		case "notice":
			what = "notice: " + str(l["text"])
		case "answer":
			what = fmt.Sprintf("choice #%v", num(l["index"])+1)
		case "scene":
			what = strings.Join(strs(l["stack"]), " > ") + "  @" + str(l["location"])
		case "battle":
			what = str(l["text"])
			if c := str(l["cutin"]); c != "" {
				what = "[cut-in] " + c
			}
		case "battle_end":
			what = str(l["result"])
		case "menu":
			items := strs(l["items"])
			if len(items) == 0 {
				what = "(closed)"
			} else {
				sel := int(num(l["sel"]))
				for i := range items {
					if i == sel {
						items[i] = "[" + items[i] + "]"
					}
				}
				what = strings.Join(items, " | ")
			}
		case "view":
			now := strs(l["pictures"])
			var add, del []string
			for _, p := range now {
				if !slices.Contains(shown, p) {
					add = append(add, "+"+p)
				}
			}
			for _, p := range shown {
				if !slices.Contains(now, p) {
					del = append(del, "-"+p)
				}
			}
			shown = now
			what = strings.Join(append(add, del...), " ")
			if m := strs(l["missing"]); len(m) > 0 {
				what += "  MISSING " + strings.Join(m, ", ")
			}
			if sh := str(l["shot"]); sh != "" {
				what += "  [" + sh + "]"
			}
		case "state":
			var ps []string
			party, ok := l["party"].([]any)
			if !ok {
				return diag.TraceBroken.Errorf("invalid party record")
			}
			for _, value := range party {
				m, ok := value.(map[string]any)
				if !ok {
					return diag.TraceBroken.Errorf("invalid party member")
				}
				row := ""
				if b, ok := m["back"].(bool); ok && b {
					row = lang.T("後")
				}
				ps = append(ps, fmt.Sprintf("%s%s Lv%v %v/%v %v/%v", str(m["name"]), row, m["lv"], m["hp"], m["mhp"], m["mp"], m["mmp"]))
			}
			what = fmt.Sprintf("%vG @%s", l["gold"], str(l["location"]))
			if fl, ok := l["floor"]; ok {
				what += fmt.Sprintf(" B%vF (%v) %v", fl, l["pos"], l["facing"])
			}
			what += "  " + strings.Join(ps, ", ")
		case "music":
			what = str(l["song"])
			if m := str(l["module"]); m != "" {
				what += " / " + m
			}
			if a := str(l["asked"]); a != "" {
				what += "  (asked " + a + "/" + str(l["mood"]) + ")"
			}
		case "battle_capture":
			capture, ok := l["capture"].(map[string]any)
			if !ok {
				return diag.TraceBroken.Errorf("invalid capture record")
			}
			c := capture
			what = fmt.Sprintf(lang.T("戦闘 %v 敵集団 %s（開始直前の保存・乱数・作戦を記録）"), l["number"], str(c["troop"]))
		case "battle_result":
			b, err := json.Marshal(l["gains"])
			if err != nil {
				return err
			}
			what = fmt.Sprintf(lang.T("結果 %v 報酬 %s（終了時の状態・乱数を記録）"), l["result"], b)
		case "act":
			b, err := json.Marshal(l["args"])
			if err != nil {
				return err
			}
			what = str(l["name"]) + " " + string(b)
		case "request", "input", "problem", "shot", "diverge", "end":
			c := map[string]any{}
			for kk, v := range l {
				if kk != "t" && kk != "tick" {
					c[kk] = v
				}
			}
			b, err := json.Marshal(c)
			if err != nil {
				return err
			}
			what = string(b)
		default:
			b, err := json.Marshal(l)
			if err != nil {
				return err
			}
			what = string(b)
		}
		c.out.Printf("%s %-10s %s\n", at, k, what)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	var ks []string
	for k := range count {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	var parts []string
	for _, k := range ks {
		parts = append(parts, fmt.Sprintf("%s %d", k, count[k]))
	}
	c.out.Println("lines:", strings.Join(parts, ", "))
	return nil
}

// skipText describes a line of skip mode or scene skip (ADR 0026).
func skipText(l map[string]any) string {
	what := "skip"
	if l["scene"] == true {
		what = "scene skip"
	}
	if l["on"] == true {
		if l["scene"] == true {
			return fmt.Sprintf("%s from %s line %v (%v of %v lines read)", what, str(l["event"]), l["line"], l["read"], l["lines"])
		}
		return what + " on"
	}
	what += " off"
	if why := str(l["why"]); why != "" {
		what += " (" + why + ")"
	}
	if l["scene"] == true {
		what += fmt.Sprintf(" at line %v", l["line"])
	}
	return what
}

func num(v any) float64 {
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}

func str(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func strs(v any) []string {
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, s := range a {
		if s, ok := s.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func decodeTrace(b []byte) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, diag.TraceBroken.Wrap(err)
	}
	if str(out["t"]) == "" {
		return nil, diag.TraceBroken.Errorf("trace record needs a nonempty string t")
	}
	if v, ok := out["tick"]; ok {
		n, isNumber := v.(float64)
		if !isNumber || n < 0 || n > float64(math.MaxInt) || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, diag.TraceBroken.Errorf("invalid trace tick")
		}
	}
	return out, nil
}

// useTraceLanguage shows the engine's words in the language the trace was written in.
// Traces from before the language was recorded were written with the Japanese words.
func useTraceLanguage(start map[string]any) error {
	language := cmp.Or(str(start["language"]), lang.Japanese)
	if _, err := catalog.Words(language); err != nil {
		language = lang.English
	}
	return lang.Use(language, nil)
}
