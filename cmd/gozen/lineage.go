package main

import (
	"fmt"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// lineageLines writes a species' family tree as an indented Markdown list: each line is a
// species with how it is obtained, and a pair's parents follow it one level deeper.
func lineageLines(db *bookReport, lin map[string]lineageMonster, id string) []string {
	root := lin[id].Lineage
	if root == nil {
		return nil
	}
	var lines []string
	root.Walk(func(n *gamedata.LineageNode) {
		lines = append(lines, strings.Repeat("  ", n.Depth+1)+"- "+lineageLine(db, n))
	})
	if rs := lin[id].Recipes; len(rs) > 1 {
		var others []string
		for _, r := range rs[1:] {
			others = append(others, recipeText(db, r))
		}
		lines = append(lines, "  - Other recipes: "+strings.Join(others, "; "))
	}
	return lines
}

func lineageLine(db *bookReport, n *gamedata.LineageNode) string {
	name := n.Species
	if e := db.Enemies[n.Species]; e != nil {
		name = fmt.Sprintf("%s (%s)", e.Name, n.Species)
	}
	switch n.Ref {
	case "seen":
		return name + ": shown above"
	case "cycle":
		return name + ": loops back to an ancestor, shown above"
	}
	return name + ": " + recipeText(db, n.Recipe) + func() string {
		if len(n.Sites) > 0 {
			return " at " + strings.Join(n.Sites, ", ")
		}
		return ""
	}()
}

func recipeText(db *bookReport, r gamedata.Recipe) string {
	switch r.Kind {
	case gamedata.RecipeCapture:
		return "capture"
	case gamedata.RecipeFamily:
		return fmt.Sprintf("%s x %s family rule (%s x %s), %d generations", db.MonsterFamilies[r.Families[0]], db.MonsterFamilies[r.Families[1]], r.Parents[0], r.Parents[1], r.Steps)
	}
	extra := ""
	if r.MinPlus > 0 {
		extra = fmt.Sprintf(", needs plus %d", r.MinPlus)
	}
	return fmt.Sprintf("%s x %s, %d generations%s", r.Parents[0], r.Parents[1], r.Steps, extra)
}

type lineageMonster struct {
	ID      string                `json:"id"`
	Name    string                `json:"name"`
	Family  string                `json:"family,omitempty"`
	Rank    int                   `json:"rank,omitempty"`
	Sex     string                `json:"sex,omitempty"`
	Recipes []gamedata.Recipe     `json:"recipes"`
	Lineage *gamedata.LineageNode `json:"lineage"`
}

func renderLineage(b *bookReport) string {
	var out strings.Builder
	for _, id := range b.Ordered("enemies") {
		if e := b.Enemies[id]; e != nil {
			fmt.Fprintf(&out, "### %s (%s)\n\n", e.Name, id)
			for _, line := range lineageLines(b, b.Lineage, id) {
				fmt.Fprintln(&out, line)
			}
			out.WriteByte('\n')
		}
	}
	return out.String()
}
