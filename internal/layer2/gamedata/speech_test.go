package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestSpeechRulesAreOptInAndLanguageSpecific(t *testing.T) {
	game := "schema_version: 10\nid: studio\nlanguage: en\nstart: {location: room}\n"
	world := `locations: {room: {name: Room}}
actors:
  potter:
    name: Potter
    speech:
      en:
        first_person: {preferred: I, avoid: [we]}
        addresses: {weaver: {preferred: Master, avoid: [buddy]}}
        endings: {preferred: please, patterns: ['dude[.!]?$']}
      ja:
        match: substring
        first_person: {preferred: watashi, avoid: [boku]}
  weaver: {name: Weaver}
events:
  hello:
    do:
    - say: {who: potter, text: 'we agree, buddy, dude!'}
    - say: {who: potter, text: 'twelve boku'}
    - say: {who: weaver, text: 'we agree, buddy, dude!'}
`
	db, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(game)}, "data/world.yaml": {Data: []byte(world)}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, warning := range db.SceneWarnings() {
		if strings.Contains(warning.Error(), "GZN0123") {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("got %d speech warnings: %v", count, db.SceneWarnings())
	}
	if speechContains("twelve", "we", "word") || !speechContains("we agree", "we", "word") || !speechContains("prefixwe", "we", "substring") {
		t.Fatal("matching boundaries")
	}
}
