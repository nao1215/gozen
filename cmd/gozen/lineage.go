package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	data "github.com/nao1215/gozen/schema"
)

// lineageLines writes a species' family tree as an indented Markdown list: each line is a
// species with how it is obtained, and a pair's parents follow it one level deeper.
func lineageLines(db *gamedata.DB, lin *gamedata.Lineage, id string) []string {
	root := lin.Tree(id, nil)
	if root == nil {
		return nil
	}
	var lines []string
	root.Walk(func(n *gamedata.LineageNode) {
		lines = append(lines, strings.Repeat("  ", n.Depth+1)+"- "+lineageLine(db, n))
	})
	if rs := lin.Recipes(id); len(rs) > 1 {
		var others []string
		for _, r := range rs[1:] {
			others = append(others, recipeText(db, r))
		}
		lines = append(lines, "  - Other recipes: "+strings.Join(others, "; "))
	}
	return lines
}

func lineageLine(db *gamedata.DB, n *gamedata.LineageNode) string {
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

func recipeText(db *gamedata.DB, r gamedata.Recipe) string {
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

// bookLineageJSON writes every monster's recipes and default family tree as JSON.
func (c *command) bookLineageJSON(dir string) error {
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	if err := gamedata.UseWords(fsys); err != nil {
		return err
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		return err
	}
	lin := gamedata.BuildLineage(db)
	monsters := []lineageMonster{}
	for _, id := range db.Ordered("enemies") {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		recipes := lin.Recipes(id)
		if recipes == nil {
			recipes = []gamedata.Recipe{}
		}
		monsters = append(monsters, lineageMonster{ID: id, Name: e.Name, Family: e.Family, Rank: e.Rank, Sex: e.Sex, Recipes: recipes, Lineage: lin.Tree(id, nil)})
	}
	b, err := json.MarshalIndent(struct {
		Monsters []lineageMonster `json:"monsters"`
	}{monsters}, "", "  ")
	if err != nil {
		return err
	}
	c.out.Println(string(b))
	return nil
}
