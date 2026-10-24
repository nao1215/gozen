// Command gozen runs, checks, simulates, documents, and packages game definitions, and
// runs the picture and asset tools (ADR 0016, 0024).
package main

import (
	"context"
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
	build    func(c *command) clihelp.Operation
	delegate func(args []string, out, stderr io.Writer) int // a leaf with its own exit contract
	subs     []string                                       // the names of a group's commands, in order
}

// tree maps a command path to its node.
var tree = map[string]node{
	"":             {subs: []string{"new", "run", "export", "check", "sim", "trace", "notes", "release", "trial", "capture", "book", "prompt", "art", "theme", "version", "help"}},
	"new":          {build: (*command).newGame},
	"check":        {build: (*command).checkCmd},
	"sim":          {subs: []string{"battle", "dive", "floors", "design", "campaign", "routes", "breed"}},
	"sim breed":    {build: (*command).breedCmd},
	"sim campaign": {build: (*command).campaignCmd},
	"sim routes":   {build: (*command).routes},
	"sim battle":   {build: (*command).sim},
	"sim dive":     {build: (*command).curve},
	"sim floors":   {build: (*command).floorsCmd},
	"sim design":   {build: (*command).design},
	"run":          {build: (*command).playRun},
	"trace":        {build: (*command).traceCmd},
	"notes":        {build: (*command).notesCmd},
	"capture":      {build: (*command).capture},
	"book":         {build: (*command).bookCmd},
	"export":       {build: (*command).exportGame},
	"release":      {build: (*command).release},
	"trial":        {build: (*command).trialCmd},
	"art":          {subs: []string{"mosaic", "fetch", "faces", "puppet"}},
	"prompt":       {build: (*command).prompt, subs: []string{"lint"}},
	"prompt lint":  {build: (*command).prompt},
	"art mosaic":   {delegate: mosaiccmd.Run},
	"art fetch":    {delegate: mosaiccmd.Fetch},
	"art faces":    {build: (*command).faces},
	"art puppet":   {build: (*command).puppetCmd},
	"theme":        {subs: []string{"add", "list"}},
	"theme add":    {build: (*command).themeAdd},
	"theme list":   {build: (*command).themeList},
	"version":      {build: (*command).versionCmd},
}

func (c *command) prompt() clihelp.Operation {
	return promptcmd.New(promptcmd.Config{Input: os.Stdin, Output: c.out, Diagnostics: c.stderr, Report: &c.report, Lint: c.path == "prompt lint"})
}

// parse parses a command's options. On --help it prints the page and returns
// clihelp.ErrHelp, which run turns into exit 0.
func (c *command) parse(fs *pflag.FlagSet, args []string, page clihelp.Page) error {
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
		c.out.Print(page.String(fs))
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
	case n.build != nil:
		op := c.operation(n)
		if err := c.parse(op.Flags, args, op.Page); err != nil {
			return c.finish(err)
		}
		return c.finish(op.Run(context.Background()))
	case c.path == "":
		return c.finish(c.top(args))
	default:
		return c.finish(c.group(n, args))
	}
}

func (c *command) operation(n node) clihelp.Operation {
	op := n.build(c)
	if op.Page.Summary == "" {
		op.Page = pages[c.path]
	}
	return op
}

// finish prints err and returns the exit value.
func (c *command) finish(err error) int {
	if errors.Is(err, clihelp.ErrHelp) {
		err = nil
	}
	report := c.report
	if report.Command == "" {
		report.Command = c.path
	}
	return report.Finish(c.out, c.stderr, err)
}

// top handles gozen with no command: -h/--help and --version.
func (c *command) top(args []string) error {
	if len(args) == 0 {
		return cliio.Invalid(diag.CommandMissing, "usage: gozen <command> [OPTIONS] [ARGUMENTS]; use --help")
	}
	fs := clihelp.NewFlagSet("gozen")
	ver := fs.Bool("version", false, "print the gozen version and exit")
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

func (c *command) versionCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen version")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 0 {
			return cliio.Invalid(diag.Operands, "version takes no operands")
		}
		c.out.Println("gozen", clihelp.Version())
		return nil
	}}
}

func (c *command) bookCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen book")
	format := f.String("format", "markdown", "output format: markdown, or json for the monsters' lineage")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
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
	}}
}

func (c *command) notesCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen notes")
	memoOut := f.StringP("out", "o", "", "write the HTML to this new `DIR`; required")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "notes expects one directory")
		}
		if *memoOut == "" {
			return cliio.Invalid(diag.OptionRequired, "notes requires --out DIR")
		}
		return c.writeMemos(f.Arg(0), *memoOut)
	}}
}

func (c *command) checkCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen check")
	only := f.String("only", "", "check only `PART`: structure (definitions, references, expressions, and assets; the font is not read) or layout (the text layout)")
	format := f.String("format", "text", "the output: text, or json (an array of {file, line, col, path, message, severity, code} on stdout, warnings included)")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
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
	}}
}
