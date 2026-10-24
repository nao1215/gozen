package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/gozen/internal/campaignsim"
	"github.com/nao1215/gozen/internal/testgame"
)

func campaignDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "data", "main.yaml"), testgame.Data)
	write(t, filepath.Join(dir, "assets", "x.png"), "x")
	for name, s := range testgame.CampaignFiles(nil) {
		write(t, filepath.Join(dir, name), s)
	}
	return dir
}

// TestSimCampaign: gozen sim campaign plays the campaign with each strategy; the same seed
// gives the same JSON, every campaign ends (won, lost or cut), and the regions of each
// turn are counted.
func TestSimCampaign(t *testing.T) {
	dir := campaignDir(t)
	for _, as := range campaignsim.Strategies {
		out, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "12", "--as", as, "--format", "json")
		again, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "12", "--as", as, "--format", "json")
		if out != again {
			t.Fatalf("--as %s: the same seed gave two results", as)
		}
		var r campaignsim.Result
		if err := json.Unmarshal(resultData(t, out, "sim campaign"), &r); err != nil {
			t.Fatal(err)
		}
		if r.Won+r.Lost+r.Cut != 12 || len(r.Regions) == 0 || r.Regions[0].Mean["blue"] != 1 || r.Orders["end_turn"] == 0 {
			t.Fatalf("--as %s: %+v", as, r)
		}
	}
	out, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "5")
	for _, want := range []string{"5 campaigns from seed 1 as greedy", "regions held at the start of each turn", "orders:"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	invoke(t, 2, "sim", "campaign", dir, "--as", "lazy")
	invoke(t, 2, "sim", "campaign", dir, "-n", "0")
	plain := t.TempDir()
	write(t, filepath.Join(plain, "game.yaml"), "schema_version: 5\nid: plain\n"+testgame.Game)
	write(t, filepath.Join(plain, "data", "main.yaml"), testgame.Data)
	write(t, filepath.Join(plain, "assets", "x.png"), "x")
	invoke(t, 2, "sim", "campaign", plain)
}

func TestSimCampaignCompare(t *testing.T) {
	dir := campaignDir(t)
	for _, mode := range []string{"strategies", "tactics"} {
		out, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "3", "--compare", mode, "--format", "json")
		again, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "3", "--compare", mode, "--format", "json")
		if out != again {
			t.Fatalf("--compare %s: same seeds gave different JSON", mode)
		}
		var got campaignsim.Comparison
		if err := json.Unmarshal(resultData(t, out, "sim campaign"), &got); err != nil {
			t.Fatal(err)
		}
		if got.Mode != mode || len(got.Rows) == 0 {
			t.Fatalf("--compare %s: %+v", mode, got)
		}
		for _, row := range got.Rows {
			if row.WinRate < 0 || row.WinRate > 1 || row.MeanTurns <= 0 || len(row.Regions) == 0 {
				t.Fatalf("--compare %s row: %+v", mode, row)
			}
			for _, region := range row.Regions {
				if region.EndHolders == nil || region.MeanChanges < 0 || region.PlayerWinRateByTurn == nil {
					t.Fatalf("--compare %s region: %+v", mode, region)
				}
			}
		}
	}
	out, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "2", "--compare", "strategies")
	if !strings.Contains(out, "mean regions held") || !strings.Contains(out, "mean first taken turn") {
		t.Fatalf("comparison table is incomplete:\n%s", out)
	}
	invoke(t, 2, "sim", "campaign", dir, "--compare", "factions")
	ordinary, _ := invoke(t, 0, "sim", "campaign", dir, "-n", "2", "--format", "json")
	if strings.Contains(ordinary, "region_metrics") {
		t.Fatalf("ordinary JSON unexpectedly changed: %s", ordinary)
	}
}
