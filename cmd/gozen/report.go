package main

import (
	"fmt"
	"strings"

	md "github.com/nao1215/markdown"
)

// reportBullet adds one item. markdown owns the list syntax; indentation is
// supplied here because its flat-list API does not represent nested event trees.
func reportBullet(doc *md.Markdown, indent, format string, args ...any) {
	item := md.NewMarkdown(nil).BulletList(strings.ReplaceAll(fmt.Sprintf(format, args...), "\n", "\n  ")).String()
	doc.PlainText(indent + strings.ReplaceAll(item, "\n", "\n"+indent))
}

func reportCells(values ...any) []string {
	cells := make([]string, len(values))
	for i, v := range values {
		cells[i] = fmt.Sprint(v)
	}
	return cells
}

// reportMarkdown keeps command reports deterministic across operating systems.
// The builder chooses native line endings; Gozen reports use LF and one final newline.
func reportMarkdown(doc *md.Markdown) string {
	return strings.TrimRight(strings.ReplaceAll(doc.String(), "\r\n", "\n"), "\n") + "\n"
}
