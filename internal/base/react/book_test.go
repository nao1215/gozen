package react

import "testing"

// TestBookRefusesNumbersThatAreNotFinite: a weight, a mood, a like, a delta and a
// remembered amount written as .nan or .inf are refused. A range written as two
// comparisons lets NaN through, the mood becomes NaN and the mind cannot be saved.
func TestBookRefusesNumbersThatAreNotFinite(t *testing.T) {
	const rules = "    rules:\n      - {id: r, say: [hi]}\n"
	for name, src := range map[string]string{
		"delta nan":    "people:\n  - id: host\n    rules:\n      - {id: r, say: [hi], delta: .nan}\n",
		"delta inf":    "people:\n  - id: host\n    rules:\n      - {id: r, say: [hi], delta: .inf}\n",
		"likes nan":    "lexicon: {cat: [cat]}\npeople:\n  - id: host\n    likes: {cat: .nan}\n" + rules,
		"weight nan":   "lexicon: {cat: [{word: cat, weight: .nan}]}\npeople:\n  - id: host\n" + rules,
		"mood nan":     "people:\n  - id: host\n    mood: .nan\n" + rules,
		"mood_min nan": "people:\n  - id: host\n    mood_min: .nan\n    mood_max: 5\n" + rules,
		"mood_max inf": "people:\n  - id: host\n    mood_max: .inf\n" + rules,
		"add nan":      "people:\n  - id: host\n    rules:\n      - {id: r, remember: [{key: k, add: .nan}]}\n",
		"time inf":     "people:\n  - id: host\n    rules:\n      - {id: r, remember: [{key: k, add: 1, time: .inf}]}\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Parse([]byte("lexicon: {cat: [cat]}\npeople:\n  - id: host\n    mood: 50\n    likes: {cat: -2.5}\n    rules:\n      - {id: r, say: [hi], delta: -1, remember: [{key: k, add: 1, time: 3}]}\n")); err != nil {
		t.Errorf("finite numbers refused: %v", err)
	}
}
