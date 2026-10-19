// Package text holds the words of the engine's screens (the menus, the battle's messages,
// the hints, the settings, the development panels), so a game can show them in its own
// language and change any of them.
//
// Every word has a key: the Japanese text as the engine's source writes it, a format with
// its %d and %s too. The engine ships two built-in sets of words, Japanese (each key is
// its own word) and English. game.yaml's language picks one, and game.yaml's text puts
// the game's own words on top of it:
//
//	language: en
//	text:
//	  "にげられない！": "No way out!"
//	  "%sを %dこ かった！": "Bought %[2]d %[1]s."
//
// Use copies the supplied words, and T returns a word or the key itself. Both are
// synchronized. The chosen words are shared by every game in the process: initialize
// them at the host boundary and do not switch languages from concurrent games.
package text
