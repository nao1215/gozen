package gamedata_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// A command list is named from the person's name as written, so the name a save keeps in
// one language finds its list (and its fingerprint) after the player changes the language.
// A name kept by an older save, with the name as it was shown, finds it in that language.
func TestNamedCommandsSurviveLanguageChange(t *testing.T) {
	db, err := testgame.TryLoad(map[string]string{
		"game.yaml":    "schema_version: 4\nid: lang-probe\nlanguage: en\nlanguages: [en, ja]\n" + testgame.Game,
		"lang/ja.yaml": "text: {Courier: はいたつにん}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	talk := db.Locations["camp"].People[0].Talk
	saved := db.CommandsName(talk)
	if _, err := db.CommandsFingerprint(saved); err != nil {
		t.Fatal(err)
	}
	db.Show("ja")
	if got := db.NamedCommands(saved); got == nil {
		t.Errorf("after Show(ja), NamedCommands(%q) is nil; the list is now named %q", saved, db.CommandsName(talk))
	}
	if _, err := db.CommandsFingerprint(saved); err != nil {
		t.Errorf("after Show(ja): %v", err)
	}
	if got := db.CommandsName(talk); got != saved {
		t.Errorf("after Show(ja), the list is named %q, not %q", got, saved)
	}
	if got := db.NamedCommands("camp.はいたつにん"); len(got) == 0 || &got[0] != &talk[0] {
		t.Error("the name an older save kept while Japanese was shown no longer finds the talk")
	}
	db.Show("en")
	if got := db.NamedCommands("camp.はいたつにん"); got != nil {
		t.Error("a name shown in another language found a list")
	}
}

// Two entries of one place that share a name (the same person before and after a flag)
// have their own command-list names: the first keeps <place>.<name>, as older saves name
// it, and the second is <place>.<name>#2, so a save made in the second talk resumes it.
func TestNamedCommandsAreUniquePerList(t *testing.T) {
	db, err := testgame.TryLoad(map[string]string{"data/review.yaml": `
locations:
  hall:
    name: Hall
    people:
      - {name: Keeper, when: "!flag.open", talk: [{say: "The door is shut."}]}
      - {name: Keeper, when: "flag.open", talk: [{say: "Welcome in."}, {set: {flag.greeted: true}}]}
    exits: [{to: camp}]
events:
  opener: {do: [{set: {flag.open: true}}, {if: "flag.greeted", then: [{say: "Done."}]}]}
`})
	if err != nil {
		t.Fatal(err)
	}
	first, second := db.Locations["hall"].People[0].Talk, db.Locations["hall"].People[1].Talk
	if got := db.CommandsName(first); got != "hall.Keeper" {
		t.Errorf("the first talk is named %q, want hall.Keeper", got)
	}
	name := db.CommandsName(second)
	if name != "hall.Keeper#2" {
		t.Errorf("the second talk is named %q, want hall.Keeper#2", name)
	}
	got := db.NamedCommands(name)
	if len(got) != len(second) || &got[0] != &second[0] {
		t.Errorf("NamedCommands(CommandsName(second talk)) = the list of %d commands starting %q, want the second talk (%d commands)", len(got), got[0].Say.Text, len(second))
	}
}
