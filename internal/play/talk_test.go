package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
)

func TestInstantPages(t *testing.T) {
	for _, instant := range []bool{false, true} {
		line := talk{say: event.Say{Instant: instant}, pages: [][]string{{"First page"}, {"Second page"}}}
		for page := range line.pages {
			line.setPage(page)
			if instant && line.shown != line.total || !instant && line.shown != 0 {
				t.Fatalf("instant %v, page %d: %d/%d", instant, page, line.shown, line.total)
			}
		}
	}
}
