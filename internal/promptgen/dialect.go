package promptgen

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	lang "github.com/nao1215/gozen/internal/text"
)

type Block struct {
	Words    []Word
	Boundary string
}

// checkDialect accepts the dialects the catalog can be written for; sdxl is the only one.
func checkDialect(name string) error {
	if name != "sdxl" {
		return problem(lang.T("未知の方言です。%s"), name)
	}
	return nil
}

const sdxlMaxWeight = 1.5

type sdxl struct{}

func (sdxl) Weight(strength int) float64 {
	switch strength {
	case 0:
		return 0.9
	case 1:
		return 1
	case 2:
		return 1.1
	case 3:
		return 1.2
	case 4:
		return sdxlMaxWeight
	default:
		return 1.6
	}
}

func (d sdxl) Render(blocks []Block) string {
	var out strings.Builder
	for _, b := range blocks {
		if len(b.Words) == 0 {
			continue
		}
		if out.Len() > 0 {
			if b.Boundary == "block" {
				out.WriteString(" BREAK ")
			} else {
				out.WriteString(", ")
			}
		}
		for i, w := range b.Words {
			if i > 0 {
				out.WriteString(", ")
			}
			weight := d.Weight(w.Strength)
			if w.Fixed {
				weight = 1
			}
			if weight == 1 {
				out.WriteString(w.Word)
			} else {
				out.WriteString("(" + w.Word + ":" + strconv.FormatFloat(weight, 'f', -1, 64) + ")")
			}
		}
	}
	return out.String()
}

var scoreTerm = regexp.MustCompile(`(?i)\bscore[_ ]\d`)

func abstractTerm(s string) error {
	if core(s) == "" || scoreTerm.MatchString(s) {
		return problem("%s", lang.T("語が空か方言固有の指定です"))
	}
	for token := range strings.FieldsSeq(s) {
		if strings.EqualFold(token, "BREAK") || token == "AND" {
			return problem("%s", lang.T("区切りは抽象指定で書いてください"))
		}
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(",:()[]{}<>|\\", r) || (r > 127 && (unicode.IsPunct(r) || unicode.IsSymbol(r))) {
			return problem("%s", lang.T("語に制御構文・括弧・全角記号は書けません"))
		}
	}
	return nil
}

func (sdxl) ValidateTerm(s string) error {
	if err := abstractTerm(s); err != nil {
		return err
	}
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return problem("%s", lang.T("この方言では日本語のタグを使えません"))
		}
	}
	return nil
}

var (
	tokenParts   = regexp.MustCompile(`[A-Za-z0-9]+|[^A-Za-z0-9\s]`)
	outputWeight = regexp.MustCompile(`\([^():]+:([0-9.]+)\)`)
)

func (sdxl) CountTokens(s string) int {
	s = outputWeight.ReplaceAllStringFunc(s, func(v string) string { body, _, _ := strings.Cut(v[1:], ":"); return body })
	n := 2
	for _, part := range tokenParts.FindAllString(s, -1) {
		if part[0] < 128 && (unicode.IsLetter(rune(part[0])) || unicode.IsDigit(rune(part[0]))) {
			n += (len(part) + 3) / 4
		} else {
			n++
		}
	}
	return n
}

func (d sdxl) ValidateOutput(r Result) []Diagnostic {
	var out []Diagnostic
	for _, side := range []string{"positive", "negative"} {
		value := r.Positive
		if side == "negative" {
			value = r.Negative
		}
		words := sideWords(r.Words, side)

		if side == "positive" && value == "" && len(words) == 0 {
			continue
		}
		for block := range strings.SplitSeq(value, " BREAK ") {
			if strings.TrimSpace(block) == "" {
				out = append(out, diagnostic("sdxl.break", "error", lang.T("空の区切りがあります"), words))
			}
			if d.CountTokens(block) > 77 {
				out = append(out, diagnostic("sdxl.tokens", "warning", lang.T("区切り内が近似77トークンを超えています"), words))
			}
		}
		for block := range strings.SplitSeq(value, " BREAK ") {
			for raw := range strings.SplitSeq(block, ",") {
				tag := strings.TrimSpace(raw)
				if tag == "" {
					continue
				}
				if strings.HasPrefix(tag, "(") && strings.HasSuffix(tag, ")") {
					body, numeric, ok := strings.Cut(tag[1:len(tag)-1], ":")
					weight, err := strconv.ParseFloat(numeric, 64)
					if !ok || err != nil || math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 || weight > sdxlMaxWeight {
						out = append(out, diagnostic("sdxl.weight", "error", lang.T("出力した重みが範囲外です"), words))
					}
					tag = body
				}
				if err := d.ValidateTerm(tag); err != nil {
					out = append(out, diagnostic("sdxl.syntax", "error", err.Error(), words))
				}
			}
		}
	}
	return out
}
