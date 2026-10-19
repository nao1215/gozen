package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"

	lang "github.com/nao1215/gozen/text"
)

func (c *command) traceCmd(args []string) (err error) {
	fl := flag.NewFlagSet("gozen trace", flag.ContinueOnError)
	only := fl.String("only", "", "only these kinds (comma separated: say, scene, battle, menu, view, state, act, problem...)")
	from := fl.Float64("from", 0, "from this second")
	to := fl.Float64("to", 0, "to this second (0: the end)")
	input := fl.Bool("input", false, "show the input too")
	check := fl.Bool("check", false, "list the faults of the standing pictures and the music (trace_check.go); exit 1 if any")
	musicLate := fl.Float64("music-late", 0.5, "with -check: a song heard later than this (seconds) after it was asked for is late")
	if err := c.parse(fl, args); err != nil {
		return err
	}
	if fl.NArg() != 1 {
		return cli.Invalid("expected <trace.jsonl>")
	}
	inputPath := fl.Arg(0)
	if math.IsNaN(*from) || math.IsNaN(*to) || math.IsNaN(*musicLate) || math.IsInf(*from, 0) || math.IsInf(*to, 0) || math.IsInf(*musicLate, 0) || *from < 0 || *to < 0 || (*to > 0 && *to < *from) || *musicLate < 0 {
		return cli.Invalid("invalid time range or music delay")
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
	for _, k := range strings.Split(*only, ",") {
		if k != "" {
			kinds[k] = true
		}
	}
	var shown []string
	count := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l map[string]any
		if err := decodeTrace(sc.Bytes(), &l); err != nil {
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
		case "say":
			what = str(l["text"])
			if w := str(l["who"]); w != "" {
				what = w + "「" + what + "」"
			}
			if c := strs(l["choices"]); len(c) > 0 {
				what += " ? " + strings.Join(c, " / ")
			}
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
				return fmt.Errorf("invalid party record")
			}
			for _, value := range party {
				m, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid party member")
				}
				row := ""
				if b, ok := m["back"].(bool); ok && b {
					row = "後"
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
				return fmt.Errorf("invalid capture record")
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

func decodeTrace(b []byte, out *map[string]any) error {
	if err := json.Unmarshal(b, out); err != nil {
		return err
	}
	if str((*out)["t"]) == "" {
		return fmt.Errorf("trace record needs a nonempty string t")
	}
	if v, ok := (*out)["tick"]; ok {
		n, isNumber := v.(float64)
		if !isNumber || n < 0 || n > float64(math.MaxInt) || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("invalid trace tick")
		}
	}
	return nil
}
