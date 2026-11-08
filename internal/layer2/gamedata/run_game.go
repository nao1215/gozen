package gamedata

import (
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

// RunGameCommand is consumed by event to launch a declared runner. Outputs map
// result names to existing flag/var assignments; no solver receives story state.
type RunGameCommand struct {
	ID      string            `yaml:"id"`
	Outputs map[string]string `yaml:"outputs"`
}

func (c *checker) runGame(where string, q *RunGameCommand, names []string) {
	c.needsVersion(where, "casual games", SchemaVersion12)
	if q == nil {
		return
	}
	if c.db.Game.Runners[q.ID] == nil {
		c.errorf(diag.UnknownReference, where+".id", "unknown runner %q", q.ID)
	}
	targets := map[string]bool{}
	for key, target := range q.Outputs {
		value := "0"
		switch key {
		case "score", "lives", "stock", "stage", "completed_stages", "best_completed_stages", "misses", "ticks", "distance", "collected", "streak", "life_progress":
		case "completed", "failed", "cancelled":
			value = "false"
		default:
			c.errorf(diag.Shape, where+".outputs", "unknown result %q", key)
			continue
		}
		if (!strings.HasPrefix(target, "var.") && !strings.HasPrefix(target, "flag.")) || targets[target] {
			c.errorf(diag.Shape, where+".outputs", "result target %q must be a distinct flag or variable", target)
			continue
		}
		targets[target] = true
		c.assignments(where+".outputs", map[string]Expr{target: MustExpr(value)}, names)
	}
}
