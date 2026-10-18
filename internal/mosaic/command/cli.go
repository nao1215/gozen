// Package command implements the mosaic process contract. It never exits the process.
package command

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const requirements = "Requirements: Go 1.26.4+ to build. ONNX inference needs a C compiler and CGO_ENABLED=1 at build time, a user-supplied ONNX Runtime shared library (-ort or ONNXRUNTIME_LIB), and models (-model; optionally -face, -seg, -tagger). Help and manual-only processing need no ONNX runtime. See https://github.com/nao1215/gozen/blob/main/README.md#mosaic"
const usage = `usage:
 mosaic auto [flags] <game-dir> [asset-relative-images...]
 mosaic inspect [flags] <game-dir> [images...]
 mosaic manual <list|show|view|add|drop|grow|skip|check|clear> [flags] <game-dir> [image]
Flags must precede positional arguments. Models are never bundled.
`

type output struct {
	io.Writer
	err error
}

func (o *output) Write(b []byte) (int, error) {
	n, err := o.Writer.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	o.err = errors.Join(o.err, err)
	return n, err
}
func (o *output) printf(s string, v ...any) { o.write(fmt.Sprintf(s, v...)) }
func (o *output) println(v ...any)          { o.write(fmt.Sprintln(v...)) }
func (o *output) write(s string) {
	if o.err == nil {
		_, o.err = io.WriteString(o, s)
	}
}

type cli struct{ out, diag *output }
type argumentError struct{ error }

// Run returns 0 for success/help, 2 for argument errors, and 1 for processing or
// I/O errors. Caller owns writers. No process state or working directory changes.
func Run(args []string, out, diagnostics io.Writer) int {
	c := &cli{&output{Writer: out}, &output{Writer: diagnostics}}
	var err error
	help := false
	for _, a := range args {
		if a == "--help" || a == "-help" || a == "-h" {
			help = true
		}
	}
	if len(args) == 2 && args[0] == "manual" && args[1] == "help" {
		help = true
	}
	if help {
		c.out.write(usage)
		if len(args) > 0 && args[0] == "manual" {
			c.out.println(manualHelp)
		}
		if len(args) > 0 && args[0] == "inspect" {
			c.out.println(inspectHelp)
		}
		fs := flag.NewFlagSet("mosaic", flag.ContinueOnError)
		fs.SetOutput(c.out)
		c.commonFlags(fs)
		fs.PrintDefaults()
		if len(args) > 0 {
			switch args[0] {
			case "auto":
				c.out.println("auto: -dry (no writes)")
			case "inspect":
				c.out.println("inspect: -pretty -redetect (no writes)")
			case "manual":
				c.out.println("manual: -rect -box -poly -ellipse -norm -region-id -by -left -top -right -bottom -note -src -region -size -step -o")
			}
		}
		c.out.println(requirements)
	} else if len(args) == 0 {
		err = argumentError{fmt.Errorf("a subcommand is required\n%s", usage)}
	} else {
		switch args[0] {
		case "auto":
			err = c.autoCmd(args[1:])
		case "manual":
			err = c.manualCmd(args[1:])
		case "inspect":
			err = c.inspectCmd(args[1:])
		default:
			err = argumentError{fmt.Errorf("unknown subcommand %q\n%s", args[0], usage)}
		}
	}
	err = errors.Join(err, c.out.err, c.diag.err)
	if err == nil {
		return 0
	}
	c.diag.println(err)
	c.diag.println(requirements)
	var arg argumentError
	if errors.As(err, &arg) {
		return 2
	}
	return 1
}
