// Package react makes characters react to what the player says or does, without a
// language model. See docs/decisions/0015-reaction-and-chat.md.
//
// A Stimulus (typed text, a game event, numbers such as a tip) runs through the
// built-in analyzers and any game-supplied Analyzer. They attach Tags with a
// confidence in 0..1: lexicon words, acrostics, repetition and references to what the
// character remembers. A Person's rules, whose conditions are expr expressions, then
// pick a Reaction. When no rule holds, the person dodges in character (teases, asks
// back, changes the subject) with a line chosen by how sure the analysis was. Judging
// is deliberately simple; the person's manner is what makes it feel clever.
//
// Load and Parse read a Book (a shared lexicon and the people) and validate every
// reference. New makes a Mind, the running state of one person: mood, memory, cooldowns
// and a pending question. A Mind is not safe for concurrent use. Save and Restore keep
// that state as bytes.
package react
