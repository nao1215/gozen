package gfx

import (
	"math"
	"strings"
	"testing"
)

func TestPaginateFallsBackAfterPagePartLimit(t *testing.T) {
	s := strings.Repeat("A.\n", maxPageParts+1)
	wrap := func(text string, _ float64) []string {
		return strings.Split(text, "\n")
	}
	pages, _ := paginateWith(s, 12, 1, wrap)
	if len(pages) != maxPageParts+1 {
		t.Fatalf("got %d pages, want %d", len(pages), maxPageParts+1)
	}
}

func TestPaginateHandlesExtremeLineCountAndTinySize(t *testing.T) {
	wrap := func(text string, _ float64) []string { return strings.Split(text, "\n") }
	pages, _ := paginateWith("first\nsecond", math.SmallestNonzeroFloat64, math.MaxInt, wrap)
	if len(pages) != 1 || len(pages[0]) != 2 {
		t.Fatalf("unexpected pages: %v", pages)
	}
	pages, _ = paginateWith("first\nsecond", math.SmallestNonzeroFloat64, 1, wrap)
	if len(pages) != 2 {
		t.Fatalf("tiny size: unexpected pages: %v", pages)
	}
}
