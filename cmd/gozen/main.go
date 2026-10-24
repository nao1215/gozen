// Command gozen runs, checks, simulates, documents, and packages game definitions, and
// runs the picture and asset tools (ADR 0016, 0024).
package main

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	mosaiccmd "github.com/nao1215/gozen/internal/mosaic/command"
	promptcmd "github.com/nao1215/gozen/internal/promptgen/command"
)

type command struct {
	out, stderr *cliio.Output
	path        string // the command path reached, "sim battle"; "" at the top
	report      cliio.Report
}

// reporting registers structured result output for a migrated command.
func (c *command) reporting(f *pflag.FlagSet) {
	c.report.Command = strings.TrimPrefix(f.Name(), "gozen ")
	f.StringVar(&c.report.Format, "format", "text", "result encoding: text, json, or jsonl")
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// node is one command of the tree: a group holds subs, a leaf runs.
type node struct {
	run      func(c *command, args []string) error
	delegate func(args []string, out, stderr io.Writer) int // a leaf with its own exit contract
	subs     []string                                       // the names of a group's commands, in order
}

// tree maps a command path to its node.
var tree = map[string]node{
	"":             {subs: []string{"new", "check", "sim", "play", "book", "pack", "art", "theme", "version", "help"}},
	"new":          {run: (*command).newGame},
	"check":        {run: (*command).checkCmd},
	"sim":          {subs: []string{"battle", "dive", "floors", "design", "campaign", "routes", "breed"}},
	"sim breed":    {run: (*command).breedCmd},
	"sim campaign": {run: (*command).campaignCmd},
	"sim routes":   {run: (*command).routes},
	"sim battle":   {run: (*command).sim},
	"sim dive":     {run: (*command).curve},
	"sim floors":   {run: (*command).floorsCmd},
	"sim design":   {run: (*command).design},
	"play":         {subs: []string{"run", "trace", "notes", "shots"}},
	"play run":     {run: (*command).playRun},
	"play trace":   {run: (*command).traceCmd},
	"play notes":   {run: (*command).notesCmd},
	"play shots":   {run: (*command).shots},
	"book":         {run: (*command).bookCmd},
	"pack":         {subs: []string{"desktop", "web", "release", "trial"}},
	"pack web":     {run: (*command).web},
	"pack desktop": {run: (*command).desktop},
	"pack release": {run: (*command).release},
	"pack trial":   {run: (*command).trialCmd},
	"art":          {subs: []string{"prompt", "mosaic", "fetch", "faces", "puppet"}},
	"art prompt":   {delegate: promptcmd.Run},
	"art mosaic":   {delegate: mosaiccmd.Run},
	"art fetch":    {delegate: mosaiccmd.Fetch},
	"art faces":    {run: (*command).faces},
	"art puppet":   {run: (*command).puppetCmd},
	"theme":        {subs: []string{"add", "list"}},
	"theme add":    {run: (*command).themeAdd},
	"theme list":   {run: (*command).themeList},
	"version":      {run: (*command).versionCmd},
}

// parse parses a command's options. On --help it prints the page and returns
// clihelp.ErrHelp, which run turns into exit 0.
func (c *command) parse(fs *pflag.FlagSet, args []string) error {
	if c.report.Command != "" {
		c.report.Format = clihelp.OutputFormat(fs, args)
	}
	help, err := clihelp.Parse(fs, args)
	if err != nil {
		if c.report.Command != "" {
			// A partial parse may have overwritten the preselected last --format.
			c.report.Format = clihelp.OutputFormat(fs, args)
		}
		return cliio.Parse(err)
	}
	if help {
		c.out.Print(pages[strings.TrimPrefix(fs.Name(), "gozen ")].String(fs))
		return clihelp.ErrHelp
	}
	if c.report.Command != "" {
		return c.report.Validate()
	}
	return nil
}

func run(args []string, out, stderr io.Writer) int {
	c := &command{out: &cliio.Output{Writer: out}, stderr: &cliio.Output{Writer: stderr}}
	if len(args) > 0 && args[0] == "help" {
		// "gozen help <command> ..." is the same as "gozen <command> ... --help".
		words := args[1:]
		for len(words) > 0 && words[0] == "help" {
			words = words[1:]
		}
		switch {
		case len(words) == 1 && diag.Syntax(words[0]):
			return c.finish(c.explain(words[0]))
		case len(words) == 0:
			args = []string{"--help"}
		default:
			args = append(slices.Clone(words), "--help")
		}
	}
	// Walk the tree to a leaf or a group.
	n := tree[""]
	for n.subs != nil && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		next := strings.TrimSpace(c.path + " " + args[0])
		child, ok := tree[next]
		if !ok || !slices.Contains(n.subs, args[0]) {
			return c.finish(c.unknown(args))
		}
		c.path, n, args = next, child, args[1:]
	}
	switch {
	case n.delegate != nil:
		return n.delegate(args, out, stderr)
	case n.run != nil:
		return c.finish(n.run(c, args))
	case c.path == "":
		return c.finish(c.top(args))
	default:
		return c.finish(c.group(n, args))
	}
}

// finish prints err and returns the exit value.
func (c *command) finish(err error) int {
	if errors.Is(err, clihelp.ErrHelp) {
		err = nil
	}
	err = errors.Join(err, c.out.Err, c.stderr.Err)
	if err == nil {
		return 0
	}
	if c.report.Machine() {
		if writeErr := c.report.Diagnostic(c.stderr, err); writeErr != nil || c.out.Err != nil || c.stderr.Err != nil {
			return 1
		}
		if _, ok := errors.AsType[*cliio.UsageError](err); ok {
			return 2
		}
		return 1
	}
	c.stderr.Println(err)
	if _, ok := errors.AsType[*cliio.UsageError](err); ok {
		help := "gozen --help"
		if c.path != "" {
			help = "gozen " + c.path + " --help"
		}
		c.stderr.Println(`Run "` + help + `" for usage.`)
		return 2
	}
	return 1
}

// top handles gozen with no command: -h/--help and -v/--version.
func (c *command) top(args []string) error {
	if len(args) == 0 {
		return cliio.Invalid(diag.CommandMissing, "usage: gozen <command> [OPTIONS] [ARGUMENTS]; use --help")
	}
	fs := clihelp.NewFlagSet("gozen")
	ver := fs.BoolP("version", "v", false, "print the gozen version and exit")
	help, err := clihelp.Parse(fs, args)
	if err != nil {
		return cliio.Parse(err)
	}
	if fs.NArg() != 0 {
		return cliio.Invalid(diag.OptionBeforeCommand, "options go after the command: gozen <command> [OPTIONS]; use --help")
	}
	switch {
	case help:
		c.out.Print(topPage.String(fs))
	case *ver:
		c.out.Println("gozen", clihelp.Version())
	default:
		return cliio.Invalid(diag.CommandMissing, "usage: gozen <command> [OPTIONS] [ARGUMENTS]; use --help")
	}
	return nil
}

// group handles a group without a command: its page on --help, else exit 2
// with its commands.
func (c *command) group(n node, args []string) error {
	fs := clihelp.NewFlagSet("gozen " + c.path)
	help, err := clihelp.Parse(fs, args)
	if err != nil {
		return cliio.Parse(err)
	}
	if help {
		c.out.Print(groups[c.path].String(fs))
		return clihelp.ErrHelp
	}
	return cliio.Invalid(diag.CommandMissing, "gozen %s needs a command: %s", c.path, strings.Join(n.subs, ", "))
}

// unknown reports a command that is not in the tree.
func (c *command) unknown(args []string) error {
	name := args[0]
	if c.path == "" {
		return cliio.Invalid(diag.UnknownCommand, "unknown command %q; use --help", name)
	}
	return cliio.Invalid(diag.UnknownCommand, "unknown command %q of gozen %s: use %s", name, c.path, strings.Join(tree[c.path].subs, ", "))
}

// explain prints the entry of a diagnostic code (gozen help GZN1101).
func (c *command) explain(code string) error {
	if e, ok := diag.Parse(code); ok {
		entry, _ := diag.Lookup(e)
		c.out.Print(diag.Text(entry))
		return nil
	}
	n, _ := diag.Number(code)
	var near []string
	for _, e := range diag.Near(n) {
		near = append(near, e.Code.String())
	}
	hint := "see " + diag.URL
	if len(near) > 0 {
		hint = "codes of the same family: " + strings.Join(near, ", ") + "; see " + diag.URL
	}
	return cliio.Invalid(diag.UnknownCode, "unknown diagnostic code %s; %s", strings.ToUpper(code), hint)
}

func (c *command) versionCmd(args []string) error {
	f := clihelp.NewFlagSet("gozen version")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return cliio.Invalid(diag.Operands, "version takes no operands")
	}
	c.out.Println("gozen", clihelp.Version())
	return nil
}

func (c *command) bookCmd(args []string) error {
	f := clihelp.NewFlagSet("gozen book")
	format := f.String("format", "markdown", "output format: markdown, or json for the monsters' lineage")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cliio.Invalid(diag.Operands, "book expects one directory")
	}
	if *format != "markdown" && *format != "json" {
		return cliio.Invalid(diag.NotAChoice, "--format is markdown or json, not %q", *format)
	}
	if *format == "json" {
		return c.bookLineageJSON(f.Arg(0))
	}
	return c.book(f.Arg(0))
}

func (c *command) notesCmd(args []string) error {
	f := clihelp.NewFlagSet("gozen play notes")
	memoOut := f.StringP("out", "o", "", "write the HTML to this new `DIR` (default: <memo-dir>-html)")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cliio.Invalid(diag.Operands, "play notes expects one directory")
	}
	return c.memosCmd(f.Args(), *memoOut)
}

func (c *command) checkCmd(args []string) error {
	f := clihelp.NewFlagSet("gozen check")
	only := f.String("only", "", "check only `PART`: structure (definitions, references, expressions, and assets; the font is not read) or layout (the text layout)")
	format := f.String("format", "text", "the output: text, or json (an array of {file, line, col, path, message, severity, code} on stdout, warnings included)")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cliio.Invalid(diag.Operands, "check expects one directory")
	}
	switch *only {
	case "", "structure", "layout":
	default:
		return cliio.Invalid(diag.NotAChoice, "--only is structure or layout, not %q", *only)
	}
	switch *format {
	case "text":
	case "json":
		return c.checkJSON(f.Arg(0), *only)
	default:
		return cliio.Invalid(diag.NotAChoice, "--format is text or json, not %q", *format)
	}
	switch *only {
	case "structure":
		return c.checkStructure(f.Arg(0))
	case "layout":
		return c.layout(f.Arg(0), c.out)
	}
	return c.check(f.Arg(0))
}
