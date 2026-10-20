package gamedata

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

// Voice (schema_version 4, ADR 0028): a line's say.voice names a sound, found in the
// speaker's actors.<id>.voices folder or, for anyone else, as a path under assets/.

// VoiceStyle shapes how voices play (ui.voice).
type VoiceStyle struct {
	// Cut stops a voice when the next line comes (default true; false lets it finish).
	Cut *bool `yaml:"cut"`
	// AutoWait makes auto wait for the voice to end as well as for the text (default true).
	AutoWait *bool `yaml:"auto_wait"`
}

// Cuts reports whether the next line stops a voice.
func (v VoiceStyle) Cuts() bool { return v.Cut == nil || *v.Cut }

// Waits reports whether auto waits for a voice.
func (v VoiceStyle) Waits() bool { return v.AutoWait == nil || *v.AutoWait }

// voiceExts are the kinds of sound a voice may be, tried in this order for a name without one.
var voiceExts = []string{".ogg", ".wav"}

// VoicePath is the file under assets/ of a line's voice: for an actor with voices, the name
// in that folder (the extension may be left out); otherwise a path under assets/. false
// when there is no such file.
func (db *DB) VoicePath(who, voice string) (string, bool) {
	if voice == "" {
		return "", false
	}
	p := voice
	if a, ok := db.Actors[who]; ok && a.Voices != "" {
		p = path.Join(a.Voices, voice)
	}
	if path.Ext(p) == "" {
		for _, ext := range voiceExts {
			if db.Exists("assets/" + p + ext) {
				return p + ext, true
			}
		}
		return p, false
	}
	return p, db.Exists("assets/" + p)
}

// voice checks a line's voice.
func (c *checker) voice(w string, s *Say) {
	if a, ok := c.db.Actors[s.Who]; ok && a.Voices != "" {
		c.voiceLines(s.Who, w, s.Voice != "")
	}
	if s.Voice == "" || !c.needsVersion(w+".voice", "voice", SchemaVersion4) {
		return
	}
	p, ok := c.db.VoicePath(s.Who, s.Voice)
	if ext := strings.ToLower(path.Ext(p)); ext != "" && !slices.Contains(voiceExts, ext) {
		c.errorf(diag.NotAWord, w+".voice", "a voice is .ogg or .wav, not %q", p)
		return
	}
	if !ok {
		c.errorf(diag.MissingAsset, w+".voice", "no voice assets/%s", p)
	}
}

// voices checks an actor's folder of voices.
func (c *checker) voices(w string, a *Actor) {
	if a.Voices == "" || !c.needsVersion(w, "voices", SchemaVersion4) {
		return
	}
	if st, err := fs.Stat(c.db.FS, "assets/"+a.Voices); err != nil || !st.IsDir() {
		c.errorf(diag.MissingAsset, w, "no folder assets/%s", a.Voices)
	}
}

// voiceCount is how many of a voiced actor's lines have a voice.
type voiceCount struct {
	lines, voiced int
	missing       []string
}

func (c *checker) voiceLines(who, w string, voiced bool) {
	if c.voiced == nil {
		c.voiced = map[string]*voiceCount{}
	}
	n := c.voiced[who]
	if n == nil {
		n = &voiceCount{}
		c.voiced[who] = n
	}
	n.lines++
	if voiced {
		n.voiced++
	} else {
		n.missing = append(n.missing, w)
	}
}

// voiceWarnings warn of the lines of voiced actors that have no voice yet.
func (c *checker) voiceWarnings() {
	for _, who := range slices.Sorted(func(yield func(string) bool) {
		for k := range c.voiced {
			if !yield(k) {
				return
			}
		}
	}) {
		n := c.voiced[who]
		if len(n.missing) == 0 {
			continue
		}
		first := n.missing[:min(3, len(n.missing))]
		more := ""
		if len(n.missing) > len(first) {
			more = fmt.Sprintf(" and %d more", len(n.missing)-len(first))
		}
		c.warnf(diag.Unvoiced, "actors."+who+".voices", "%d of %d lines of %s have no voice: %s%s", len(n.missing), n.lines, who, strings.Join(first, ", "), more)
	}
}
