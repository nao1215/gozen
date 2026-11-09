package traceformat_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/base/traceformat"
)

func TestExactReplayViewportChanges(t *testing.T) {
	initial := traceformat.Viewport{Width: 1280, Height: 720, Scale: 1}
	startup := traceformat.Viewport{Width: 960, Height: 540, Scale: 1}
	retina := traceformat.Viewport{Width: 2560, Height: 1440, Scale: 2}
	nativeResize := traceformat.Viewport{Width: 960, Height: 540, Scale: 0.75}
	for _, tt := range []struct {
		name    string
		tick    int
		current traceformat.Viewport
		wantErr bool
	}{
		{name: "startup layout", current: startup},
		{name: "startup device scale", current: retina},
		{name: "startup native scale", current: nativeResize},
		{name: "first tick layout", tick: 1, current: startup},
		{name: "first tick device scale", tick: 1, current: retina},
		{name: "unchanged", tick: 2, current: initial},
		{name: "device scale after start", tick: 2, current: retina, wantErr: true},
		{name: "native scale after first tick", tick: 2, current: nativeResize, wantErr: true},
		{name: "native scale at later tick", tick: 60, current: nativeResize, wantErr: true},
		{name: "fractional device scale", tick: 2, current: traceformat.Viewport{Width: 1920, Height: 1080, Scale: 1.5}, wantErr: true},
		{name: "logical resize", tick: 2, current: startup, wantErr: true},
		{name: "scale without matching pixels", tick: 2, current: traceformat.Viewport{Width: 1280, Height: 720, Scale: 2}, wantErr: true},
		{name: "portrait layout", tick: 2, current: traceformat.Viewport{Width: 1280, Height: 720, Scale: 1, Tall: true}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var exact traceformat.ExactReplay
			check := func(tick int, viewport traceformat.Viewport) error {
				t.Helper()
				raw, err := json.Marshal(viewport)
				if err != nil {
					t.Fatal(err)
				}
				return exact.Check("viewport", tick, raw)
			}
			if err := check(0, initial); err != nil {
				t.Fatal(err)
			}
			err := check(tt.tick, tt.current)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "the recording changed the window size") {
					t.Fatalf("resize: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := check(2, tt.current); err != nil {
				t.Fatalf("settled layout: %v", err)
			}
			changed := tt.current
			changed.Width += 100
			if err := check(3, changed); err == nil {
				t.Fatal("accepted a logical resize after settling")
			}
		})
	}
}

func TestExactReplayRequiresSourceAuditRevision(t *testing.T) {
	for _, revision := range []int{0, 1, 2, 4} {
		var exact traceformat.ExactReplay
		err := exact.Check("start", 0, fmt.Appendf(nil, `{"exact_version":%d}`, revision))
		if err == nil || !strings.Contains(err.Error(), "record again") {
			t.Fatalf("revision %d: %v", revision, err)
		}
	}
}

// TestRecordFieldReaders: numbers and lists read the same before and after a record
// goes through JSON, and anything else reads as the zero value.
func TestRecordFieldReaders(t *testing.T) {
	for _, v := range []any{2.0, 2, uint64(2)} {
		if traceformat.Number(v) != 2 {
			t.Errorf("Number(%T) = %v", v, traceformat.Number(v))
		}
	}
	if traceformat.Number("2") != 0 || traceformat.Number(nil) != 0 {
		t.Error("a non-number must read as 0")
	}
	if got := traceformat.Strings([]any{"a", 1.0, "b"}); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("decoded list %q", got)
	}
	if got := traceformat.Strings([]string{"a"}); !slices.Equal(got, []string{"a"}) {
		t.Errorf("unwritten list %q", got)
	}
	if traceformat.Strings("a") != nil {
		t.Error("a non-list must read as nil")
	}
}
