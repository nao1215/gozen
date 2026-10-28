package play

import (
	"strings"
)

// firstLanguage is the first of the device's languages prefs that the game offers: one
// named exactly, else one of the same first part ("en-US" is "en"). Empty when the game
// offers none of them.
func firstLanguage(prefs, offered []string) string {
	for _, p := range prefs {
		p = strings.ReplaceAll(strings.TrimSpace(p), "_", "-")
		if p == "" {
			continue
		}
		for _, o := range offered {
			if strings.EqualFold(o, p) {
				return o
			}
		}
		base, _, _ := strings.Cut(p, "-")
		for _, o := range offered {
			if b, _, _ := strings.Cut(o, "-"); strings.EqualFold(b, base) {
				return o
			}
		}
	}
	return ""
}

// localeTag is a POSIX locale (en_US.UTF-8) as a language tag (en-US); empty for C and
// POSIX, which name no language.
func localeTag(s string) string {
	s, _, _ = strings.Cut(s, "@")
	s, _, _ = strings.Cut(s, ".")
	if s == "C" || s == "POSIX" {
		return ""
	}
	return strings.ReplaceAll(s, "_", "-")
}

// envLocales are the languages of the locale variables LC_ALL, LC_MESSAGES and LANG, in
// that order, read with getenv.
func envLocales(getenv func(string) string) []string {
	var out []string
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if t := localeTag(getenv(k)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// appleLanguages are the languages of macOS's `defaults read -g AppleLanguages`: a list
// such as ("en-US", "ja-JP").
func appleLanguages(out string) []string {
	return strings.FieldsFunc(out, func(r rune) bool { return strings.ContainsRune("(),\" \t\r\n", r) })
}
