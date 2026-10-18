package promptgen

import (
	"os"
	"strings"
	"testing"
)

func TestSDXLEmphasisLevels(t *testing.T) {
	data, err := os.ReadFile("../../docs/examples/production/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		emphasis string
		output   string
	}{
		{"soft", "(traveler:0.9)"},
		{"normal", "traveler"},
		{"strong", "(traveler:1.1)"},
		{"stronger", "(traveler:1.2)"},
		{"strongest", "(traveler:1.5)"},
		{"excessive", ""},
	} {
		t.Run(tc.emphasis, func(t *testing.T) {
			catalogData := strings.Replace(string(data), "strongest]", "strongest, excessive]", 1)
			catalogData = strings.ReplaceAll(catalogData, "emphasis: strongest}", "emphasis: "+tc.emphasis+"}")
			c, err := Parse([]byte(catalogData))
			if tc.output == "" {
				if err == nil {
					t.Fatal("accepted emphasis above the SDXL limit")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p, err := c.Scene("portrait")
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Build(p)
			if err != nil {
				t.Fatal(err)
			}
			if r.Positive != "quiet garden, soft light, "+tc.output+", adult" {
				t.Fatalf("unexpected emphasis: %q", r.Positive)
			}
			if ds := c.Lint(); len(ds) != 0 {
				t.Fatalf("lint rejected valid emphasis: %+v", ds)
			}
		})
	}
}

func TestSDXLWeightLimit(t *testing.T) {
	for _, side := range []string{"positive", "negative"} {
		for _, weight := range []string{"1.5", "1.5001"} {
			r := Result{Positive: "quiet garden", Negative: "noise"}
			value := "(soft light:" + weight + ")"
			if side == "positive" {
				r.Positive = value
			} else {
				r.Negative = value
			}
			ds := (sdxl{}).ValidateOutput(r)
			if weight == "1.5" && len(ds) != 0 {
				t.Fatalf("%s: maximum weight rejected: %+v", side, ds)
			}
			if weight != "1.5" && (len(ds) != 1 || ds[0].Rule != "sdxl.weight" || ds[0].Severity != "error") {
				t.Fatalf("%s: excessive weight not rejected: %+v", side, ds)
			}
		}
	}
}
