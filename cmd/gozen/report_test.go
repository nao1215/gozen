package main

import (
	"testing"

	md "github.com/nao1215/markdown"
)

func TestReportPreservesNestedMultilineItemsAndCanonicalNewlines(t *testing.T) {
	doc := md.NewMarkdown(nil, md.WithBlockSpacing()).H2("Notes")
	reportBullet(doc, "", "%s", "Parent")
	reportBullet(doc, "  ", "%s", "First\r\nSecond")
	got := reportMarkdown(doc)
	want := "## Notes\n\n- Parent\n  - First\n    Second\n"
	if got != want {
		t.Fatalf("report = %q, want %q", got, want)
	}
}
