// Package decodeguard keeps a third-party decoder from crashing the host on
// untrusted input.
//
// goccy/go-yaml v1.19.2 dereferences nil when a tagged scalar is decoded into a
// slice or an array (for example "names: !x 0"). Game data, books and scripts are
// untrusted input (SECURITY.md), so every YAML decode in Gozen runs through Run
// or RunNode. They refuse a tagged scalar before the decoder sees it, and report
// any other panic of the decoder as an error. The fuzz tests of chat and react
// keep inputs of that shape in their corpus.
//
// Recovering the nil dereference is not enough. On Windows it is a hardware
// exception, and Go 1.27 corrupted the heap while recovering one (golang/go#81238):
// the process died later, in the garbage collector. So the input is refused
// first, on every system, and the recover is the second line.
package decodeguard

import (
	"bytes"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Run checks src, the YAML that decode reads, and calls decode. It returns an
// error for a tagged scalar in src without calling decode, decode's own error,
// or an error describing the panic when decode panics.
func Run(src []byte, decode func() error) error {
	if err := Source(src); err != nil {
		return err
	}
	return guarded(decode)
}

// RunNode is Run for a decode that reads a parsed node (yaml.NodeToValue).
func RunNode(node ast.Node, decode func() error) error {
	if err := Node(node); err != nil {
		return err
	}
	return guarded(decode)
}

func guarded(decode func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yaml: the decoder rejected the input: %v", r)
		}
	}()
	return decode()
}

// Source reports a tagged scalar in YAML text. Text that does not parse is left
// to the decoder, which reports it with its own message.
func Source(src []byte) error {
	// A tag starts with "!": most files have none and are not parsed twice.
	if !bytes.ContainsRune(src, '!') {
		return nil
	}
	file, err := parser.ParseBytes(src, 0)
	if err != nil {
		return nil //nolint:nilerr // The decoder reports the syntax error.
	}
	for _, doc := range file.Docs {
		if err := Node(doc.Body); err != nil {
			return err
		}
	}
	return nil
}

// Node reports a tagged scalar under a parsed node: a tag such as !x or !!str
// on a value that is not a mapping, a sequence or empty. The decoder dereferences
// nil when such a value meets a list, whatever the tag, and Gozen's formats use
// no tags, so every tagged scalar is refused rather than only those under a list.
func Node(node ast.Node) error {
	if node == nil {
		return nil
	}
	var found *ast.TagNode
	ast.Walk(visitor(func(n ast.Node) bool {
		tag, ok := n.(*ast.TagNode)
		if found == nil && ok && scalar(tag.Value) {
			found = tag
		}
		return found == nil
	}), node)
	if found == nil {
		return nil
	}
	return &yaml.SyntaxError{
		Message: fmt.Sprintf("the tag %s on a single value is not supported; write the value without a tag", found.Start.Value),
		Token:   found.Start,
	}
}

func scalar(n ast.Node) bool {
	switch v := n.(type) {
	case nil, *ast.NullNode, *ast.MappingNode, *ast.MappingValueNode, *ast.SequenceNode:
		return false
	case *ast.AnchorNode:
		return scalar(v.Value)
	case *ast.TagNode:
		return scalar(v.Value)
	}
	return true
}

type visitor func(ast.Node) bool

func (v visitor) Visit(n ast.Node) ast.Visitor {
	if v(n) {
		return v
	}
	return nil
}
