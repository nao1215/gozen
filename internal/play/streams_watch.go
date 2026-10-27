package play

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/chat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/puppetdef"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/internal/react"
	lang "github.com/nao1215/gozen/internal/text"
)

// streamWatch is the watch view of one show: the stage with the hosts and the captions,
// the information under it, and the chat with the comment field, the answers and the tips.
type streamWatch struct {
	show   *broadcast.Show
	bodies []*puppet.Puppet // one for each host
	talk   []time.Time      // until when each host's mouth moves
	minds  map[string]*react.Mind
	chat   *streamChat
	crowd  *chat.Crowd
	opened time.Time

	queue []streamSay // lines to say before the next beat
	busy  time.Time   // the next step is due then
	// hold is how long the next step waits after the line said ends, read aloud or not:
	// timing.ask for a question, for the viewers to answer.
	hold time.Duration
	// limit is when the line read aloud is given up: its voice still loading or its
	// speech not reported ended (a line whose voice does not come lasts until then).
	limit time.Time
	// spoken reports the end of the line read aloud (nil: none is being read), and voice
	// is the recorded voice of the line said while it loads (nil: none).
	spoken func() bool
	voice  *voiceLoad

	next, from, watched  int // the next beat, the beat come in at, the beats watched
	seen, thanked, ended bool
	// part is the episode playing (Show.Part), partFrom the beat the player came in at in
	// it and partWatched the beats watched of it; seen is whether it is marked seen.
	part, partFrom, partWatched int

	ask      *data.StreamAsk
	askHost  int
	mine     int // the option the player chose (-1: none)
	npcAt    []time.Time
	comments []string   // comments the streamer has not answered yet
	later    []laterSay // comments read back later (reply)

	caption    string
	captionWho int
	// said is when the caption was shown, and off when it is taken away (zero: while its
	// line is said): captionLinger after the chat beat that follows it begins.
	said, off time.Time
	age       int

	viewers int
	nextTip time.Time

	field     textField
	typing    bool
	speaker   stamp // the reading-aloud button's speaker
	chatFling fling
	tips      bool
	focus     int // the control chosen with the keys (watchControls)
}

// streamSay is a line a host says: the host's index, the text and the face, and the line
// as shown before its {fields} were filled, which names its recorded voice ("" for none).
type streamSay struct {
	host            int
	text, face, raw string
}

// tipKind is the stimulus kind of a tip; value.amount is its amount.
const tipKind = "tip"

func newStreamWatch(g *Game, show *broadcast.Show, viewers int) *streamWatch {
	now := g.streamNow()
	w := &streamWatch{show: show, minds: map[string]*react.Mind{}, chat: newStreamChat(streamChatW - 28), crowd: g.newCrowd(), opened: now, mine: -1, viewers: viewers}
	for i, c := range show.Hosts {
		w.bodies = append(w.bodies, g.channelPuppet(c, fmt.Sprintf("watch:%s:%d", show.Key, i)))
	}
	w.talk = make([]time.Time, len(show.Hosts))
	w.next = show.BeatAt(now)
	if w.next < len(show.Beats) && show.Beats[w.next].Ask != nil {
		w.next++
	}
	w.from = w.next
	w.part, w.partFrom = show.Part(w.next), w.next
	w.nextTip = now.Add(time.Duration(40+g.desk.rng.IntN(60)) * time.Second)
	w.field.start(g, func() (float64, float64) {
		x, y, _, _ := w.controlRect(g, [2]int{int(ctlComment), 0})
		return w.field.caretX(x+10, 16), y + 12
	})
	w.greet(g)
	g.emit("stream", traceLine{"show": show.Key, "episode": show.Episodes[w.part].Title, "beat": w.next})
	return w
}

func (w *streamWatch) modal() bool { return true }

// watching is the watch view on top of the screens, or nil.
func (g *Game) watching() *streamWatch {
	if len(g.stack) == 0 {
		return nil
	}
	if w, ok := g.stack[len(g.stack)-1].(*streamWatch); ok {
		return w
	}
	return nil
}

// The layout of the watch view: the widest chat column, the caption's letters and the part
// of the stage's width it takes, and the height of the information under the stage in the
// portrait layout.
const (
	streamChatW  = 380.0
	captionSize  = 21.0
	captionShare = 0.86
	tallInfoH    = 74.0
	// The caption on the small stage of the portrait layout: smaller letters across more of it.
	tallCaption, tallCaptionShare = 17.0, 0.95
)

// watchLayout is where the parts of the watch view are, in UI units: the stage from the
// left edge, the information under it (none in full screen), the chat column down to the
// bottom of the screen, the part of the stage the caption is centred in, its letters and the
// part of that width it takes. zoom is how many times the stage's height the puppets' frame
// is.
type watchLayout struct {
	stageY, stageW, stageH float64
	infoH                  float64
	colX, colTop, colW     float64
	capX, capW, capH       float64
	capSize, capShare      float64
	zoom                   float64
}

// watchLayout lays out the watch view. In the landscape layout the chat column is on the
// right of the stage, and in full screen the stage fills the screen at 16:9 under the chat;
// in the portrait layout the stage is at the top with the information and the chat under
// it, and in full screen the stage takes the bar's place at the top, still 16:9, so the
// puppets show the same part of themselves, with the chat under it.
func (g *Game) watchLayout() watchLayout {
	sw, sh := g.screen()
	full := g.cfg.StreamFull
	if g.view.tall {
		h := sw * 9 / 16
		if full {
			return watchLayout{stageW: sw, stageH: h, colTop: h, colW: sw, capW: sw, capH: h, capSize: tallCaption, capShare: tallCaptionShare, zoom: 1}
		}
		return watchLayout{stageY: streamBarH, stageW: sw, stageH: h, infoH: tallInfoH, colTop: streamBarH + h + tallInfoH, colW: sw, capW: sw, capH: h, capSize: tallCaption, capShare: tallCaptionShare, zoom: 1}
	}
	cw := min(streamChatW, max(300, sw*0.3))
	if full { // the caption centred on the screen clear of the chat, or left of it when narrow
		h := min(sh, sw*9/16)
		capX, capW := cw, sw-2*cw
		if capW < 480 {
			capX, capW = 0, sw-cw
		}
		return watchLayout{stageY: (sh - h) / 2, stageW: sw, stageH: h, colX: sw - cw, colW: cw, capX: capX, capW: capW, capH: h, capSize: captionSize, capShare: captionShare, zoom: 1}
	}
	w := sw - cw
	h := w * 9 / 16
	return watchLayout{stageY: streamBarH, stageW: w, stageH: h, infoH: sh - streamBarH - h, colX: w, colTop: streamBarH, colW: cw, capW: w, capH: h, capSize: captionSize, capShare: captionShare, zoom: 1}
}

// chatRect is where the chat's messages are: the chat column under its heading, over the
// answers and the comment field.
func (w *streamWatch) chatRect(g *Game) (x, y, cw, h float64) {
	_, sh := g.screen()
	lay := g.watchLayout()
	top := lay.colTop + 38 // under the heading
	switch {
	case g.cfg.StreamFull && g.view.tall:
		top = lay.colTop + 8
	case g.cfg.StreamFull:
		top = lay.colTop + 44
	}
	bottom := sh - 70
	if w.ask != nil && w.mine < 0 {
		bottom -= float64(len(w.ask.Options))*40 + 28
	}
	return lay.colX + 14, top + 8, lay.colW - 28, bottom - top - 14
}

// toggleStreamFull switches the watch view between the page and the full screen, kept in
// the settings for every save.
func (g *Game) toggleStreamFull() {
	g.cfg.StreamFull = !g.cfg.StreamFull
	g.sfx("decide")
	if err := g.saveConfig(); err != nil {
		g.failure = err
	}
}

// dataLine is a line of the data the host says: translated, with whom it is said to
// ({from}) and the days put in.
func (g *Game) dataLine(host int, line, from string, days int) streamSay {
	raw := g.db.Tr(line)
	return streamSay{host: host, raw: raw, text: strings.NewReplacer("{from}", from, "{days}", strconv.Itoa(days)).Replace(raw)}
}

// greet says what the host says to a viewer coming in: the first visit's line, or the
// lines of the affection step and the days in a row.
func (w *streamWatch) greet(g *Game) {
	c := w.show.Hosts[0]
	p := broadcast.Progress(g.st.Streams, c.ID)
	pick := func(lines []string) {
		if len(lines) > 0 {
			w.queue = append(w.queue, g.dataLine(0, lines[g.desk.rng.IntN(len(lines))], g.st.Streams.Viewer, p.Run))
		}
	}
	if broadcast.Visit(p, g.streamNow()) {
		pick(c.First)
		return
	}
	if s := g.step(c); s < len(c.Greet) {
		for _, l := range c.Greet[s] {
			w.queue = append(w.queue, g.dataLine(0, l, g.st.Streams.Viewer, p.Run))
		}
	}
	if p.Run >= broadcast.StreakDays {
		pick(c.Streak)
	}
}

// say has a host say a line: the face, the mouth, the caption and the chat, and for how
// long (until it has been read aloud, when that is on).
func (w *streamWatch) say(g *Game, l streamSay) {
	now := g.streamNow()
	d := g.db.Streams
	if p := w.bodies[l.host]; p != nil {
		face, _ := p.Rig().Face(l.face)
		p.Motion.SetFace(face)
		if d.Bounces(l.text) {
			p.Motion.React(puppetdef.Reaction{Kind: "jiggle", Amount: d.Bounce.Amount})
		}
	}
	runes := utf8.RuneCountInString(l.text)
	w.caption, w.captionWho, w.said, w.off = l.text, l.host, now, time.Time{}
	w.chat.post(chat.Message{From: g.channelName(w.show.Hosts[l.host]), Text: l.text, Values: map[string]float64{chatHost: 1}})
	w.talk[l.host] = now.Add(time.Duration(min(4, 0.5+0.07*float64(runes)) * float64(time.Second)))
	w.busy = now.Add(seconds(d.Timing.LineSeconds(l.text)) + w.hold)
	w.voice = nil
	if !g.desk.speak {
		return
	}
	if f := d.Voice(w.show.Hosts[l.host].ID, l.raw); f != "" && g.desk.voices {
		// the line waits for its voice to load (a browser may still be fetching it), then
		// for it to end
		if w.voice = loadVoice(g.db.FS, f); !w.voice.loaded() {
			w.limit = now.Add(seconds(5 + 0.4*float64(runes)))
			w.busy, w.talk[l.host] = w.limit.Add(w.hold), w.limit
		}
		w.voiced(g)
		return
	}
	w.readAloud(g, l.host, l.text)
}

// readAloud reads the line the host says with speech synthesis (a browser build), the
// next step waiting for it to end.
func (w *streamWatch) readAloud(g *Game, host int, text string) {
	if !canSpeak() {
		return
	}
	v := w.show.Hosts[host].Voice
	language := "en-US"
	if g.db.Shown() == "ja" {
		language = "ja-JP"
	}
	w.spoken = speak(strings.NewReplacer("「", "", "」", "", "『", "", "』", "").Replace(text), language, v.Pitch, v.Rate)
	w.limit = g.streamNow().Add(seconds(5 + 0.4*float64(utf8.RuneCountInString(text))))
	w.busy, w.talk[host] = w.limit.Add(w.hold), w.limit
}

// voiced plays the recorded voice of the line said once it has loaded, the next step
// waiting for it to end; a line whose voice did not load is read aloud as a line without
// one (captioned only, outside a browser).
func (w *streamWatch) voiced(g *Game) {
	v := w.voice
	if v == nil || !v.loaded() {
		return
	}
	w.voice = nil
	now := g.streamNow()
	l := traceLine{"file": v.file, "who": w.show.Hosts[w.captionWho].ID}
	if v.err != nil {
		l["missing"] = true
		g.emit("voice", l)
		// the line's own timing, from now when it waited for the voice to load
		w.busy = minTime(w.busy, now.Add(seconds(g.db.Streams.Timing.LineSeconds(w.caption))+w.hold))
		w.talk[w.captionWho] = minTime(w.talk[w.captionWho], now.Add(time.Duration(min(4, 0.5+0.07*float64(utf8.RuneCountInString(w.caption)))*float64(time.Second))))
		w.readAloud(g, w.captionWho, w.caption)
		return
	}
	sound.PlayVoice(v.pcm, g.voiceGain(w.show.Hosts[w.captionWho].Actor))
	end := now.Add(seconds(sound.ClipSeconds(v.pcm)))
	l["ticks"] = int(math.Ceil(sound.ClipSeconds(v.pcm) * ebiten.DefaultTPS))
	g.emit("voice", l)
	w.busy, w.talk[w.captionWho] = end.Add(voiceGap+w.hold), end
}

// The times of a caption: at least captionLeast before the next step, whatever the line's
// timing or voice (a short line would go before it is read; the watch view lags the
// stream's clock for it as for a voice), and captionLinger after the chat beat that
// follows its line begins.
const (
	captionLeast  = 2500 * time.Millisecond
	captionLinger = 1500 * time.Millisecond
)

// shownCaption is the caption drawn now ("" for none).
func (w *streamWatch) shownCaption(g *Game) string {
	if !w.off.IsZero() && !g.streamNow().Before(w.off) {
		return ""
	}
	return w.caption
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// voiceGap is the pause after a line read aloud, before the next step.
const voiceGap = 250 * time.Millisecond

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// step goes on to the next thing the show says: a line waiting, the answer to the
// question, the reply to a comment, the next beat, or the end.
func (w *streamWatch) step(g *Game) {
	switch {
	case len(w.queue) > 0:
		l := w.queue[0]
		w.queue = w.queue[1:]
		w.say(g, l)
	case w.ask != nil:
		w.answer(g)
	case len(w.comments) > 0:
		text := w.comments[0]
		w.comments = w.comments[1:]
		if w.reply(g, 0, react.Stimulus{Kind: react.KindSay, From: g.st.Streams.Viewer, Text: text, Values: w.values(g)}, false) {
			g.earn(w.show.Hosts[0], g.db.Streams.Affection.Reply)
		}
		w.step(g)
	case len(w.later) > 0 && (w.later[0].at <= w.next || w.next >= len(w.show.Beats)):
		w.queue = append(w.queue, w.later[0].lines...)
		w.later = w.later[1:]
		w.step(g)
	case w.next < len(w.show.Beats):
		b := w.show.Beats[w.next]
		if part := w.show.Part(w.next); part != w.part {
			w.part, w.partFrom, w.partWatched, w.seen = part, w.next, 0, false
		}
		w.next++
		w.watched++
		w.partWatched++
		if !w.seen && broadcast.Seen(w.partWatched, w.partFrom, w.show.PartEnd(w.part)) {
			w.markSeen(g)
		}
		w.run(g, b)
	default:
		w.finish(g)
	}
}

// host is the index of a channel among the hosts (the host when it is none of them).
func (w *streamWatch) host(id string) int {
	return max(0, slices.IndexFunc(w.show.Hosts, func(c *data.Channel) bool { return c.ID == id }))
}

// run plays a beat.
func (w *streamWatch) run(g *Game, b data.Beat) {
	now := g.streamNow()
	t := g.db.Streams.Timing
	switch {
	case b.Line != nil:
		raw := g.db.Tr(b.Line.Text)
		w.say(g, streamSay{host: w.host(b.Speaker), text: raw, face: b.Line.Face, raw: raw})
	case b.Chat != nil:
		w.chat.post(chat.Message{From: g.db.Tr(b.Chat[0]), Text: g.db.Tr(b.Chat[1])})
		w.busy = now.Add(seconds(t.Chat))
		if w.off.IsZero() {
			w.off = now.Add(captionLinger)
		}
	case b.Ask != nil:
		w.askHost = w.host(b.Ask.Who)
		q := g.db.Tr(b.Ask.Q)
		w.hold = seconds(t.Ask)
		w.say(g, streamSay{host: w.askHost, text: q, raw: q})
		w.ask, w.mine, w.focus = b.Ask, -1, 0
		w.npcAt = nil
		for k := range b.Ask.NPC {
			w.npcAt = append(w.npcAt, now.Add(seconds(1.5+2.2*float64(k))))
		}
	}
}

// answer reads one answer to the question: the player's, by the chance of the host's
// affection step, or a bot viewer's.
func (w *streamWatch) answer(g *Game) {
	q, d := w.ask, g.db.Streams
	w.ask = nil
	var who string
	var option int
	if w.mine >= 0 && broadcast.PicksAnswer(d, g.step(w.show.Hosts[0]), g.desk.rng.Float64()) {
		who, option = g.st.Streams.Viewer, w.mine
		for _, c := range w.show.Hosts {
			g.earn(c, d.Affection.Answer)
		}
	} else {
		n := q.NPC[g.desk.rng.IntN(len(q.NPC))]
		who, option = g.db.Tr(n.Name), n.Option
	}
	for _, r := range q.Options[option].R {
		w.queue = append(w.queue, g.dataLine(w.askHost, r, who, 0))
	}
	w.step(g)
}

// mind is the reaction state of the host's actor, made on first use (nil: no book).
func (w *streamWatch) mind(g *Game, host int) *react.Mind {
	actor := w.show.Hosts[host].Actor
	if m, ok := w.minds[actor]; ok {
		return m
	}
	m, err := g.st.Mind(actor, g.desk.rng)
	if err != nil {
		m = nil // a channel without a reaction book does not answer
	}
	w.minds[actor] = m
	return m
}

// reply has the host react to the stimulus and queues what it says (first, when
// first), reporting whether it says anything. A comment the host found nothing in is
// read back only when it is short, and then by chance (echo), echoLater beats later, as if
// the streamer came upon it in the chat; the others flow by unanswered.
func (w *streamWatch) reply(g *Game, host int, s react.Stimulus, first bool) bool {
	m := w.mind(g, host)
	if m == nil {
		return false
	}
	r, err := m.React(s)
	if err != nil {
		g.emit("stream_error", traceLine{"error": err.Error()})
		return false
	}
	if err := g.st.KeepMind(w.show.Hosts[host].Actor, m); err != nil {
		g.emit("stream_error", traceLine{"error": err.Error()})
	}
	if s.Kind == react.KindSay && len(r.Tags) == 0 && (r.Rule == "" || r.Rule == "dodge.unknown") {
		g.noteUnanswered(w.show.Hosts[host].ID, s.Text)
	}
	if e := g.db.Streams.Echo; r.Rule == "dodge.unknown" && (utf8.RuneCountInString(s.Text) > e.MaxRunes || g.desk.rng.Float64() >= e.Chance) {
		return false
	}
	lines := make([]streamSay, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = streamSay{host: host, text: l, face: r.Face, raw: r.Written[i]}
	}
	switch {
	case r.Rule == "dodge.unknown" && s.Kind == react.KindSay:
		w.later = append(w.later, laterSay{at: w.next + echoLater, lines: lines})
	case first:
		w.queue = slices.Concat(lines, w.queue)
	default:
		w.queue = append(w.queue, lines...)
	}
	return len(lines) > 0
}

// unansweredFile, beside the saves (a file of the save folder, a key of a browser's
// localStorage), keeps the last unansweredMax comments of the player that no reaction book
// found anything in, for the author to read and grow the books from (ADR 0052). Nothing
// sends it anywhere.
const (
	unansweredFile = "stream_unanswered.json"
	unansweredMax  = 200
)

// unanswered is a comment kept in unansweredFile: when, to which channel, in which language.
type unanswered struct {
	At       string `json:"at"`
	Channel  string `json:"channel"`
	Language string `json:"language"`
	Text     string `json:"text"`
}

// noteUnanswered adds the comment to unansweredFile, dropping the oldest beyond
// unansweredMax. A damaged file starts again; a failed write is logged.
func (g *Game) noteUnanswered(channel, text string) {
	var list []unanswered
	if b, err := g.store.Read(unansweredFile); err == nil && json.Unmarshal(b, &list) != nil {
		list = nil
	}
	list = append(list, unanswered{At: g.streamNow().Format(time.RFC3339), Channel: channel, Language: g.db.Shown(), Text: text})
	list = list[max(0, len(list)-unansweredMax):]
	b, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		err = g.store.Write(unansweredFile, b)
	}
	if err != nil {
		log.Printf("streams: %s: %v", unansweredFile, err)
	}
}

// echoLater is how many beats after a comment the streamer reads it back (reply).
const echoLater = 2

// laterSay are lines said once the show has come to the beat at (or to its end).
type laterSay struct {
	at    int
	lines []streamSay
}

// values are the numbers of the player's comment or tip that the reaction book's rules
// read as value.NAME: the hour (0 to 23), the visits to the host's channel, the days in a
// row, the affection step and whether the episode playing is a family's (family, 1 or 0).
func (w *streamWatch) values(g *Game) map[string]float64 {
	c := w.show.Hosts[0]
	p := broadcast.Progress(g.st.Streams, c.ID)
	return map[string]float64{"hour": float64(g.streamNow().Hour()), "visits": float64(p.Visits), "days": float64(p.Run), "step": float64(g.step(c)), "family": w.family()}
}

// family is 1 when the episode playing is a family's (family: true), else 0.
func (w *streamWatch) family() float64 {
	if w.show.Episodes[w.part].Family {
		return 1
	}
	return 0
}

// quote is a tip's comment as the streamer reads it out.
func (g *Game) quote(text string) string {
	if g.db.Shown() == "ja" {
		return "「" + text + "」"
	}
	return `"` + text + `"`
}

// tip sends a tip with the comment field's text.
func (w *streamWatch) tip(g *Game, amount int) {
	text := strings.TrimSpace(w.field.take())
	text = string([]rune(text)[:min(utf8.RuneCountInString(text), 40)])
	g.st.Gold -= amount
	c := w.show.Hosts[0]
	w.chat.post(chat.Message{From: g.st.Streams.Viewer, Text: text, Values: map[string]float64{chatTip: float64(amount), chatMine: 1}})
	g.earn(c, float64(amount)/g.db.Streams.Affection.TipPer)
	g.emit("stream_tip", traceLine{"show": w.show.Key, "amount": amount})
	values := w.values(g)
	values["amount"] = float64(amount)
	w.reply(g, 0, react.Stimulus{Kind: tipKind, From: g.st.Streams.Viewer, Text: text, Values: values}, true)
	if text != "" {
		w.queue = append([]streamSay{{text: g.quote(text)}}, w.queue...)
	}
	if p := w.bodies[0]; p != nil && g.db.Streams.Bounce.Amount > 0 {
		p.Motion.React(puppetdef.Reaction{Kind: "jiggle", Amount: g.db.Streams.Bounce.Amount})
	}
	g.saveStreams()
}

// botTip is a bot viewer's tip, read out by one of the hosts.
func (w *streamWatch) botTip(g *Game) {
	d, rng := g.db.Streams, g.desk.rng
	amount := d.Tips[rng.IntN(min(2, len(d.Tips)))]
	who := g.db.Tr(d.Crowd.Names[rng.IntN(len(d.Crowd.Names))])
	text := g.db.Tr(d.Crowd.TipLines[rng.IntN(len(d.Crowd.TipLines))])
	w.chat.post(chat.Message{From: who, Text: text, Values: map[string]float64{chatTip: float64(amount)}})
	host := rng.IntN(len(w.show.Hosts))
	if text != "" {
		w.queue = append(w.queue, streamSay{host: host, text: g.quote(text)})
	}
	w.reply(g, host, react.Stimulus{Kind: tipKind, From: who, Text: text, Values: map[string]float64{"amount": float64(amount), "hour": float64(g.streamNow().Hour()), "family": w.family()}}, false)
}

// markSeen marks the episode playing seen: the channel goes on to the episode after it.
func (w *streamWatch) markSeen(g *Game) {
	w.seen = true
	broadcast.MarkSeen(g.st.Streams, w.show, w.part)
	g.saveStreams()
}

// finish ends the show: a viewer who watched most of it is thanked by name first.
func (w *streamWatch) finish(g *Game) {
	if !w.seen {
		w.markSeen(g)
	}
	c := w.show.Hosts[0]
	if !w.thanked && broadcast.Seen(w.watched, w.from, len(w.show.Beats)) && w.watched > 0 && len(c.Stayed) > 0 {
		w.thanked = true
		g.earn(c, g.db.Streams.Affection.Stayed)
		w.queue = append(w.queue, g.dataLine(0, c.Stayed[g.desk.rng.IntN(len(c.Stayed))], g.st.Streams.Viewer, 0))
		w.step(g)
		return
	}
	w.ended = true
	w.caption = ""
	g.saveStreams()
}

// leave goes back to the lobby.
func (w *streamWatch) leave(g *Game) {
	w.hush()
	keepAwake(false)
	closeBodies(w.bodies)
	g.sfx("cancel")
	w.speaker.free()
	g.saveStreams()
	g.pop()
}

// comment sends the comment field's text to the chat; the host answers it later.
func (w *streamWatch) comment(g *Game) {
	text := strings.TrimSpace(w.field.take())
	if text == "" {
		return
	}
	text = string([]rune(text)[:min(utf8.RuneCountInString(text), 60)])
	w.chat.post(chat.Message{From: g.st.Streams.Viewer, Text: text, Values: map[string]float64{chatMine: 1}})
	g.earn(w.show.Hosts[0], g.db.Streams.Affection.Comment)
	g.emit("stream_comment", traceLine{"show": w.show.Key, "text": text})
	if len(w.comments) < 2 {
		w.comments = append(w.comments, text)
	}
}

// choose answers the question with the option.
func (w *streamWatch) choose(g *Game, i int) {
	if w.ask == nil || w.mine >= 0 || i >= len(w.ask.Options) {
		return
	}
	w.mine = i
	g.sfx("decide")
	w.chat.post(chat.Message{From: g.st.Streams.Viewer, Text: g.db.Tr(w.ask.Options[i].C), Values: map[string]float64{chatMine: 1}})
}

// watchControl is a control of the chat column chosen with the keys.
type watchControl int

const (
	ctlAnswer watchControl = iota // an option of the question
	ctlComment
	ctlTip
	ctlSpeak
	ctlFull // the full screen and back
)

// hush stops the line being read aloud or its voice.
func (w *streamWatch) hush() {
	if w.spoken != nil {
		stopSpeaking()
		w.spoken = nil
	}
	w.voice = nil
	sound.StopVoice()
}

// canRead reports whether the lines can be read aloud: with speech synthesis (a browser
// build) or the streams' recorded voices.
func (g *Game) canRead() bool { return canSpeak() || g.desk.voices }

// controls are the controls shown now, in order: [kind, option].
func (w *streamWatch) controls(g *Game) [][2]int {
	var out [][2]int
	if w.ask != nil && w.mine < 0 {
		for i := range w.ask.Options {
			out = append(out, [2]int{int(ctlAnswer), i})
		}
	}
	out = append(out, [2]int{int(ctlComment), 0}, [2]int{int(ctlTip), 0})
	if g.canRead() {
		out = append(out, [2]int{int(ctlSpeak), 0})
	}
	return append(out, [2]int{int(ctlFull), 0})
}

func (w *streamWatch) update(g *Game) {
	now := g.streamNow()
	w.age++
	for i, p := range w.bodies {
		if p != nil {
			p.Motion.Speak(now.Before(w.talk[i]))
			p.Motion.Step()
		}
	}
	g.keepCrowd(w.crowd)
	w.chat.pull(w.crowd, now.Sub(w.opened))
	if w.age%180 == 0 {
		w.viewers = max(20, w.viewers+g.desk.rng.IntN(17)-7)
	}
	if w.age%300 == 1 {
		keepAwake(g.desk.speak) // a browser may have let the lock go while the page was hidden
	}
	if !w.ended {
		for len(w.npcAt) > 0 && w.ask != nil && !now.Before(w.npcAt[0]) {
			n := w.ask.NPC[len(w.ask.NPC)-len(w.npcAt)]
			w.chat.post(chat.Message{From: g.db.Tr(n.Name), Text: g.db.Tr(w.ask.Options[n.Option].C)})
			w.npcAt = w.npcAt[1:]
		}
		if !now.Before(w.nextTip) {
			w.nextTip = now.Add(time.Duration(60+g.desk.rng.IntN(90)) * time.Second)
			w.botTip(g)
		}
		if (w.voice != nil || w.spoken != nil) && !now.Before(w.limit) {
			// given up (speech still going is stopped): a voice or speech that ends later
			// does not move the next step
			w.hush()
		}
		w.voiced(g)
		if w.spoken != nil && w.spoken() {
			w.spoken = nil
			w.busy = now.Add(voiceGap + w.hold)
			w.talk[w.captionWho] = now
		}
		if !now.Before(w.busy) && !now.Before(w.said.Add(captionLeast)) {
			w.spoken, w.voice, w.hold = nil, nil, 0
			w.step(g)
		}
	}
	w.input(g)
}

// input reads the keys and the pointer: typing a comment, the answers, the tips, reading
// aloud and going back.
func (w *streamWatch) input(g *Game) {
	in := &g.in
	_, sh := g.screen()
	lay := g.watchLayout()
	if w.ended {
		if in.ok() || in.cancel() || g.clickedIn(lay.stageW/2-80, lay.stageY+lay.stageH/2+20, 160, 44) {
			w.leave(g)
		}
		return
	}
	w.scrollChat(g)
	if w.typing {
		// the keys that were text are not Enter or Escape; the send button is pressed even
		// while a phone's keyboard still composes (sending confirms it)
		typed := in.devices() && w.field.readTyping()
		send := !in.devices() && in.pressed[btnOK] || in.devices() && !typed && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter))
		right := lay.colX + lay.colW
		sent := g.clickedIn(right-150, sh-62, 64, 44)
		switch {
		case send || sent:
			w.comment(g)
		case !in.devices() && in.cancel() || in.devices() && !typed && inpututil.IsKeyJustPressed(ebiten.KeyEscape):
			w.field.composer.Cancel()
			w.typing = false
		}
		if x, y, cw, ch := w.controlRect(g, [2]int{int(ctlComment), 0}); sent || g.clickedIn(x, y, cw, ch) || !in.click || in.swiped || !w.typing {
			return
		}
		// a click elsewhere, the read-aloud button too, stops typing (the text stays in the
		// field, a phone's keyboard closes) and goes on to what it clicked
		w.field.composer.Confirm()
		w.typing = false
	}
	if w.tips {
		w.updateTips(g)
		return
	}
	ctl := w.controls(g)
	w.focus = min(w.focus, len(ctl)-1)
	activate := func(c [2]int) {
		switch watchControl(c[0]) {
		case ctlAnswer:
			w.choose(g, c[1])
		case ctlComment:
			w.typing = true
		case ctlTip:
			w.tips, w.focus = true, 0
			g.sfx("decide")
		case ctlSpeak:
			g.desk.speak = !g.desk.speak
			keepAwake(g.desk.speak)
			if !g.desk.speak {
				w.hush()
			}
			g.sfx("decide")
		case ctlFull:
			g.toggleStreamFull()
		}
	}
	// A finger down on the comment field starts typing then, so the browser's keyboard
	// opens when it lifts (a page's text field takes the focus only in a touch's events).
	if x, y, cw, ch := w.controlRect(g, [2]int{int(ctlComment), 0}); in.began && g.touchIn(x, y, cw, ch) {
		w.typing = true
		if in.devices() {
			w.field.readTyping()
		}
		return
	}
	for i, c := range ctl {
		if x, y, cw, ch := w.controlRect(g, c); g.clickedIn(x, y, cw, ch) {
			w.focus = i
			activate(c)
			return
		}
	}
	switch {
	case in.pressed[btnNext]:
		w.typing = true
	case in.pressed[btnHide]:
		g.toggleStreamFull()
	case in.digit > 0 && w.ask != nil && w.mine < 0:
		w.choose(g, in.digit-1)
	case in.up():
		w.focus = (w.focus + len(ctl) - 1) % len(ctl)
		g.sfx("cursor")
	case in.down():
		w.focus = (w.focus + 1) % len(ctl)
		g.sfx("cursor")
	case in.ok():
		activate(ctl[w.focus])
	case in.cancel() || g.clickedIn(0, 0, 140, streamBarH):
		w.leave(g)
	}
	if bs := g.barButtons(nil); !g.cfg.StreamFull {
		if i := g.barClick(bs); i >= 0 {
			g.switchLanguage(bs[i].lang)
		}
	}
}

// scrollChat scrolls the chat back and forth with the wheel over it, or a finger swiping
// from it (gliding on when it lifts).
func (w *streamWatch) scrollChat(g *Game) {
	in := &g.in
	x, y, cw, h := w.chatRect(g)
	w.chat.fit(cw)
	m := in.mouse
	switch {
	case in.wheel != 0 && float64(m.X) >= x && float64(m.X) < x+cw && float64(m.Y) >= y && float64(m.Y) < y+h:
		w.chat.scroll(in.wheel*3*gfx.LineHeight(chatSize), h)
		in.wheel = 0 // not the keys' up and down
	default:
		if d := w.chatFling.step(in, g.touchIn(x, y, cw, h)); d != 0 {
			w.chat.scroll(d, h)
		}
	}
}

// updateTips reads the choice of a tip's amount.
func (w *streamWatch) updateTips(g *Game) {
	in := &g.in
	tips := g.db.Streams.Tips
	pay := func(i int) {
		if g.st.Gold < tips[i] {
			g.sfx("buzzer")
			return
		}
		g.sfx("decide")
		w.tips = false
		w.tip(g, tips[i])
	}
	for i := range tips {
		if x, y, tw, th := w.tipRect(g, i); g.clickedIn(x, y, tw, th) {
			pay(i)
			return
		}
	}
	switch {
	case in.up() || in.left():
		w.focus = (w.focus + len(tips) - 1) % len(tips)
	case in.down() || in.right():
		w.focus = (w.focus + 1) % len(tips)
	case in.ok():
		pay(w.focus)
	case in.cancel() || in.click:
		w.tips = false
	}
}

// controlRect is where a control is drawn: at the bottom of the chat column, the full
// screen's at the top of it (at the top right of the screen in full screen).
func (w *streamWatch) controlRect(g *Game, c [2]int) (x, y, cw, ch float64) {
	sw, sh := g.screen()
	lay := g.watchLayout()
	cx, right := lay.colX+14, lay.colX+lay.colW
	switch watchControl(c[0]) {
	case ctlAnswer:
		n := len(w.ask.Options)
		return cx, sh - 70 - float64(n-c[1])*40, lay.colW - 28, 34
	case ctlComment:
		return cx, sh - 58, lay.colW - 170, 42
	case ctlTip:
		return right - 150, sh - 58, 64, 42
	case ctlFull:
		if g.cfg.StreamFull {
			return sw - 86, 10, 72, 28
		}
		return right - 86, lay.colTop + 5, 72, 28
	case ctlSpeak:
	}
	return right - 80, sh - 58, 66, 42
}

// drawSpeaker draws a speaker 22 wide from x, centered on cy: with sound waves when
// on, crossed out when off.
func drawSpeaker(dst *ebiten.Image, x, cy float32, on bool, clr color.Color) {
	gfx.FillRect(dst, x, cy-4, 5, 8, clr, true)
	for i := float32(0); i < 6; i++ {
		gfx.FillRect(dst, x+5+i, cy-4-i, 1.2, 8+2*i, clr, true)
	}
	if !on {
		gfx.StrokeLine(dst, x+14, cy-4, x+21, cy+4, 2, clr, true)
		gfx.StrokeLine(dst, x+14, cy+4, x+21, cy-4, 2, clr, true)
		return
	}
	for _, r := range []float32{5, 9} {
		px, py := x+12+r*float32(math.Cos(-0.9)), cy+r*float32(math.Sin(-0.9))
		for a := -0.9 + 0.15; a <= 0.9; a += 0.15 {
			nx, ny := x+12+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a))
			gfx.StrokeLine(dst, px, py, nx, ny, 1.8, clr, true)
			px, py = nx, ny
		}
	}
}

// tipRect is where the button of the i-th tip is drawn.
func (w *streamWatch) tipRect(g *Game, i int) (x, y, tw, th float64) {
	_, sh := g.screen()
	lay := g.watchLayout()
	n := len(g.db.Streams.Tips)
	return lay.colX + 14, sh - 110 - float64(n-i)*46, lay.colW - 28, 40
}

func (w *streamWatch) draw(g *Game, dst *ebiten.Image) {
	_, sh := g.screen()
	dst.Fill(streamPage)
	lay := g.watchLayout()
	y, stw, sth := lay.stageY, lay.stageW, lay.stageH
	rect(dst, 0, y, stw, sth, streamInk)
	n := len(w.show.Hosts)
	for i, c := range w.show.Hosts {
		shift, zoom := 0.0, lay.zoom
		if n > 1 {
			shift, zoom = 0.22-0.44*float64(i), 1.25*lay.zoom
		}
		g.drawFramed(dst, c, w.bodies[i], 0, y, stw, sth, shift, zoom, i == 0)
	}
	lx := 12.0
	if g.cfg.StreamFull { // the way back over the stage
		back := "‹ " + lang.T("配信の一覧")
		bw := gfx.TextWidth(back, 15) + 28
		rect(dst, 0, 8, bw, 30, withAlpha(colorBlack, 0x80))
		gfx.DrawTextLeft(dst, back, 14, 13, 15, streamPanel)
		lx = bw + 12
	}
	gfx.FillRoundRect(dst, float32(lx), float32(y+12), 46, 22, 3, streamLive)
	gfx.DrawText(dst, "LIVE", lx+23, y+14, 14, streamPanel)
	w.drawCaption(g, dst, lay, y)
	if w.ended {
		rect(dst, 0, y, stw, sth, withAlpha(colorBlack, 0x99))
		gfx.DrawText(dst, lang.T("配信は終了しました"), stw/2, y+sth/2-30, 24, streamPanel)
		gfx.FillRoundRect(dst, float32(stw/2-80), float32(y+sth/2+20), 160, 44, 6, streamBrand)
		gfx.DrawText(dst, lang.T("一覧へ"), stw/2, y+sth/2+30, 18, streamPanel)
	}
	if lay.infoH > 0 {
		w.drawInfo(g, dst, y+sth, stw, lay.infoH)
	}
	w.drawColumn(g, dst, lay, sh)
	if !g.cfg.StreamFull {
		g.streamTop(dst, lang.T("配信の一覧"), g.barButtons(nil), -1)
	}
	g.drawToast(dst)
}

// drawCaption draws the whole line being said as white text on black bands, with the
// speaker's name over it in a duo.
func (w *streamWatch) drawCaption(g *Game, dst *ebiten.Image, lay watchLayout, y float64) {
	caption := w.shownCaption(g)
	if caption == "" {
		return
	}
	x, stw, sth, size := lay.capX, lay.capW, lay.capH, lay.capSize
	lines := gfx.Wrap(caption, size, stw*lay.capShare)
	lh := gfx.LineHeight(size)
	top := y + sth*0.95 - float64(len(lines))*lh
	if len(w.show.Hosts) > 1 {
		c := w.show.Hosts[w.captionWho]
		name := g.channelName(c)
		nw := gfx.TextWidth(name, 14)
		rect(dst, x+stw/2-nw/2-6, top-24, nw+12, 22, withAlpha(colorBlack, 0xb8))
		gfx.DrawText(dst, name, x+stw/2, top-22, 14, c.Color.Or(streamPanel))
	}
	for i, l := range lines {
		lw := gfx.TextWidth(l, size)
		ly := top + float64(i)*lh
		rect(dst, x+stw/2-lw/2-8, ly, lw+16, lh, withAlpha(colorBlack, 0xc0))
		gfx.DrawText(dst, l, x+stw/2, ly+2, size, streamPanel)
	}
}

// drawInfo draws the channel's information under the stage, height tall: the title in
// two lines (one in the portrait layout), the names, the tag and the stars, the tagline
// (not in the portrait layout) and the viewers.
func (w *streamWatch) drawInfo(g *Game, dst *ebiten.Image, top, width, height float64) {
	rect(dst, 0, top, width, height, streamPanel)
	c := w.show.Hosts[0]
	g.drawAvatar(dst, c, 16, top+14, 48)
	views := fmt.Sprintf(lang.T("%d人が視聴中"), w.viewers)
	lines := 2
	if g.view.tall {
		lines = 1
	}
	title, size := gfx.FitLines(g.db.Tr(w.show.Episodes[w.part].Title), 17, width-78-gfx.TextWidth(views, 15)-32, lines)
	for i, l := range title {
		gfx.DrawTextLeft(dst, l, 78, top+12+float64(i)*24, size, streamInk)
	}
	ly := top + 16 + float64(len(title))*24
	name := g.showName(w.show)
	gfx.DrawTextLeft(dst, name, 78, ly, 15, streamInk)
	lx := 78 + gfx.TextWidth(name, 15) + 12
	gfx.DrawTextLeft(dst, g.db.Tr(c.Tag), lx, ly, 15, c.Color.Or(streamBrand))
	lx += gfx.TextWidth(g.db.Tr(c.Tag), 15) + 12
	g.drawStars(dst, lx, ly+3, g.step(c))
	if c.Tagline != "" && !g.view.tall {
		gfx.DrawTextLeft(dst, gfx.Ellipsis(g.db.Tr(c.Tagline), 14, width-100), 78, ly+26, 14, streamInk2)
	}
	gfx.DrawTextRight(dst, views, width-16, top+16, 15, streamLive)
}

// drawColumn draws the chat column: the chat, the question's answers, the comment field
// and the buttons, and the tips when they are open.
func (w *streamWatch) drawColumn(g *Game, dst *ebiten.Image, lay watchLayout, sh float64) {
	cx, colW := lay.colX, lay.colW
	ink, ink2 := streamInk, streamInk2
	if g.cfg.StreamFull { // over the stage, on a darkened band
		ink, ink2 = streamPanel, streamRule
		rect(dst, cx, lay.colTop, colW, sh-lay.colTop, withAlpha(colorBlack, 0x70))
	} else {
		rect(dst, cx, lay.colTop, colW, sh-lay.colTop, streamPanel)
		if g.view.tall {
			rect(dst, cx, lay.colTop, colW, 1, streamRule)
		} else {
			rect(dst, cx, lay.colTop, 1, sh-lay.colTop, streamRule)
		}
		gfx.DrawTextLeft(dst, lang.T("配信チャット"), cx+14, lay.colTop+10, 14, streamInk2)
		rect(dst, cx, lay.colTop+38, colW, 1, streamRule)
	}
	ctl := w.controls(g)
	x, y, chw, chh := w.chatRect(g)
	g.drawChat(dst, w.chat, x, y, chw, chh, g.money, ink)
	if w.ask != nil && w.mine < 0 {
		gfx.DrawTextLeft(dst, lang.T("答える："), cx+14, y+chh+8, 14, ink2)
	}
	for i, c := range ctl {
		x, y, cw, ch := w.controlRect(g, c)
		focused := i == w.focus && !w.typing && !w.tips
		edge := streamRule
		if focused {
			edge = streamBrand
		}
		switch watchControl(c[0]) {
		case ctlAnswer:
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, streamPage)
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 1.5, edge)
			gfx.DrawTextLeft(dst, fmt.Sprintf("%d  %s", c[1]+1, gfx.Ellipsis(g.db.Tr(w.ask.Options[c[1]].C), 15, cw-40)), x+10, y+7, 15, streamInk)
		case ctlComment:
			if w.typing {
				edge = streamBrand
			}
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, streamPage)
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 1.5, edge)
			if w.field.text+w.field.comp == "" && !w.typing {
				gfx.DrawTextLeft(dst, lang.T("コメントする"), x+10, y+11, 16, streamInk3)
			} else {
				w.field.draw(dst, x+10, y+11, cw-20, 16, streamInk, streamBrand, w.typing, w.age)
			}
			if w.typing {
				gfx.FillRoundRect(dst, float32(cx+colW-150), float32(y), 64, float32(ch), 6, streamBrand)
				gfx.DrawText(dst, lang.T("送信"), cx+colW-118, y+11, 15, streamPanel)
			}
		case ctlTip:
			if w.typing {
				continue
			}
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, streamStar)
			if focused {
				gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 2, streamInk)
			}
			gfx.DrawText(dst, lang.T("投げ銭"), x+cw/2, y+11, 15, streamPanel)
		case ctlSpeak:
			label, ink := lang.T("オフ"), ink2
			if g.desk.speak {
				label, ink = lang.T("オン"), streamPanel
				gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, streamBrand)
			} else {
				gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 1.5, edge)
			}
			on := g.desk.speak
			w.speaker.draw(dst, fmt.Sprint(on, ink), x+8, y+ch/2-12, 24, 24, func(img *ebiten.Image) { drawSpeaker(img, 0, 12, on, ink) })
			gfx.DrawTextLeft(dst, label, x+34, y+12, 13, ink)
		case ctlFull:
			label := lang.T("全画面")
			if g.cfg.StreamFull {
				label = lang.T("通常表示")
			}
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, streamPage)
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 1.5, edge)
			gfx.DrawText(dst, label, x+cw/2, y+5, 13, streamInk2)
		}
	}
	if w.tips {
		tips := g.db.Streams.Tips
		x, y, tw, _ := w.tipRect(g, 0)
		gfx.FillRoundRect(dst, float32(x-6), float32(y-36), float32(tw+12), float32(len(tips))*46+44, 8, streamPanel)
		gfx.StrokeRoundRect(dst, float32(x-6), float32(y-36), float32(tw+12), float32(len(tips))*46+44, 8, 1.5, streamRule)
		gfx.DrawTextLeft(dst, lang.T("入力欄の文も一緒に送られます。"), x, y-28, 13, streamInk2)
		for i, a := range tips {
			bx, by, bw, bh := w.tipRect(g, i)
			col := streamTips[g.tipTier(a)]
			ink := streamInk
			if g.st.Gold < a {
				ink = streamInk3
			}
			gfx.FillRoundRect(dst, float32(bx), float32(by), float32(bw), float32(bh), 6, col)
			if i == w.focus {
				gfx.StrokeRoundRect(dst, float32(bx), float32(by), float32(bw), float32(bh), 6, 2, streamBrand)
			}
			gfx.DrawText(dst, g.money(a), bx+bw/2, by+9, 17, ink)
		}
	}
}
