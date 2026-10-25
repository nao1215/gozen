package diag

import (
	"fmt"
	"strings"
)

// Family is one hundreds range of codes: what the reader has to fix.
type Family struct {
	Prefix  string // "GZN11xx"
	Exit    int
	Meaning string
}

// families lists every range, in the order the reference page shows them.
var families = []Family{
	{"GZN01xx", 0, "warnings of check: the game loads, but something is likely a mistake"},
	{"GZN02xx", 0, "warnings of a measurement or a record"},
	{"GZN11xx", 1, "the definitions of the game: YAML, references, expressions, assets"},
	{"GZN12xx", 1, "fonts and text layout"},
	{"GZN13xx", 1, "a simulation could not run or reported a problem"},
	{"GZN14xx", 1, "a trace of a play is broken or shows faults"},
	{"GZN15xx", 1, "an output path: it exists, overlaps an input, or cannot be written"},
	{"GZN16xx", 1, "taking screenshots with the game binary"},
	{"GZN17xx", 1, "building a release or a trial tree"},
	{"GZN18xx", 1, "picture and asset tools: catalogs, models, ONNX Runtime, pictures"},
	{"GZN19xx", 1, "reading an input or writing the output"},
	{"GZN21xx", 2, "the shape of the command line: commands, operands, options"},
	{"GZN22xx", 2, "the value of an option"},
	{"GZN23xx", 2, "options that do not go together, or need each other"},
	{"GZN24xx", 2, "a path or a name the command line gives"},
	{"GZN25xx", 2, "gozen help"},
}

// Families returns the code ranges in display order for the diagnostic reference generator.
func Families() []Family { return families }

func family(c Code) *Family {
	p := fmt.Sprintf("GZN%02dxx", int(c)/100)
	for i := range families {
		if families[i].Prefix == p {
			return &families[i]
		}
	}
	return nil
}

// URL is the published reference page.
const URL = "https://nao1215.github.io/gozen/errors/"

// Text renders one entry for gozen help <code>.
func Text(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s - %s\n\n", e.Code, e.Summary)
	b.WriteString(wrap(e.Detail, 78))
	b.WriteString("\n\nFix: ")
	b.WriteString(wrap(e.Fix, 73))
	fmt.Fprintf(&b, "\n\nExit status %d. Since %s.\n%s#%s\n", e.Code.ExitCode(), e.Since, URL, strings.ToLower(e.Code.String()))
	return b.String()
}

// wrap breaks one paragraph into lines of at most width columns.
func wrap(s string, width int) string {
	var b strings.Builder
	col := 0
	for i, word := range strings.Fields(s) {
		switch {
		case i == 0:
			col = len(word)
		case col+1+len(word) > width:
			b.WriteByte('\n')
			col = len(word)
		default:
			b.WriteByte(' ')
			col += 1 + len(word)
		}
		b.WriteString(word)
	}
	return b.String()
}
