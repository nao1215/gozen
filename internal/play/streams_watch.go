package play

import (
	"cmp"
	"fmt"
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
	// spoken reports the end of the line read aloud (nil: none is being read).
	spoken func() bool

	next, from, watched  int // the next beat, the beat come in at, the beats watched
	seen, thanked, ended bool

	ask      *data.StreamAsk
	askHost  int
	mine     int // the option the player chose (-1: none)
	npcAt    []time.Time
	comments []string // comments the streamer has not answered yet

	caption    string
	captionWho int
	age        int

	viewers int
	nextTip time.Time

	field  textField
	typing bool
	naming bool
	tips   bool
	focus  int // the control chosen with the keys (watchControls)
}

// streamSay is a line a host says: the host's index, the text and the face.
type streamSay struct {
	host       int
	text, face string
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
	w.nextTip = now.Add(time.Duration(40+g.desk.rng.IntN(60)) * time.Second)
	w.field.start(g, func() (float64, float64) {
		sw, sh := g.screen()
		return w.field.caretX(sw-streamChatW+24, 16), sh - 46
	})
	w.naming = g.st.Streams.Viewer == ""
	if !w.naming {
		w.greet(g)
	}
	g.emit("stream", traceLine{"show": show.Key, "episode": show.Episode.Title, "beat": w.next})
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

// The layout of the watch view: the chat column on the right, the stage on the left, and
// the caption's letters and the part of the stage's width it takes.
const (
	streamChatW  = 380.0
	captionSize  = 21.0
	captionShare = 0.86
)

func (g *Game) watchStage() (x, y, w, h float64) {
	sw, _ := g.screen()
	w = sw - streamChatW
	return 0, streamBarH, w, w * 9 / 16
}

// fillLine translates a line and puts in whom it is said to ({from}) and the days.
func (g *Game) fillLine(line, from string, days int) string {
	return strings.NewReplacer("{from}", from, "{days}", strconv.Itoa(days)).Replace(g.db.Tr(line))
}

// greet says what the host says to a viewer coming in: the first visit's line, or the
// lines of the affection step and the days in a row.
func (w *streamWatch) greet(g *Game) {
	c := w.show.Hosts[0]
	p := broadcast.Progress(g.st.Streams, c.ID)
	pick := func(lines []string) {
		if len(lines) > 0 {
			w.queue = append(w.queue, streamSay{text: g.fillLine(lines[g.desk.rng.IntN(len(lines))], g.st.Streams.Viewer, p.Run)})
		}
	}
	if broadcast.Visit(p, g.streamNow()) {
		pick(c.First)
		return
	}
	if s := g.step(c); s < len(c.Greet) {
		for _, l := range c.Greet[s] {
			w.queue = append(w.queue, streamSay{text: g.fillLine(l, g.st.Streams.Viewer, p.Run)})
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
	w.caption, w.captionWho = l.text, l.host
	w.chat.post(chat.Message{From: g.channelName(w.show.Hosts[l.host]), Text: l.text, Values: map[string]float64{chatHost: 1}})
	w.talk[l.host] = now.Add(time.Duration(min(4, 0.5+0.07*float64(runes)) * float64(time.Second)))
	w.busy = now.Add(seconds(d.Timing.LineSeconds(l.text)))
	if g.desk.speak && canSpeak() {
		v := w.show.Hosts[l.host].Voice
		language := "en-US"
		if g.db.Shown() == "ja" {
			language = "ja-JP"
		}
		w.spoken = speak(strings.NewReplacer("「", "", "」", "", "『", "", "』", "").Replace(l.text), language, v.Pitch, v.Rate)
		limit := now.Add(seconds(5 + 0.4*float64(runes)))
		w.busy, w.talk[l.host] = limit, limit
	}
}

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
		if w.reply(g, 0, react.Stimulus{Kind: react.KindSay, From: g.st.Streams.Viewer, Text: text}, false) {
			g.earn(w.show.Hosts[0], g.db.Streams.Affection.Reply)
		}
		w.step(g)
	case w.next < len(w.show.Beats):
		b := w.show.Beats[w.next]
		w.next++
		w.watched++
		if !w.seen && broadcast.Seen(w.watched, w.from, len(w.show.Beats)) {
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
		w.say(g, streamSay{host: w.host(b.Speaker), text: g.db.Tr(b.Line.Text), face: b.Line.Face})
	case b.Chat != nil:
		w.chat.post(chat.Message{From: g.db.Tr(b.Chat[0]), Text: g.db.Tr(b.Chat[1])})
		w.busy = now.Add(seconds(t.Chat))
	case b.Ask != nil:
		w.askHost = w.host(b.Ask.Who)
		w.say(g, streamSay{host: w.askHost, text: g.db.Tr(b.Ask.Q)})
		w.ask, w.mine, w.focus = b.Ask, -1, 0
		w.busy = w.busy.Add(seconds(t.Ask))
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
		w.queue = append(w.queue, streamSay{host: w.askHost, text: g.fillLine(r, who, 0)})
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
// read back only when it is short, and then by chance (echo).
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
	if e := g.db.Streams.Echo; r.Rule == "dodge.unknown" && (utf8.RuneCountInString(s.Text) > e.MaxRunes || g.desk.rng.Float64() >= e.Chance) {
		return false
	}
	lines := make([]streamSay, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = streamSay{host: host, text: l, face: r.Face}
	}
	if first {
		w.queue = slices.Concat(lines, w.queue)
	} else {
		w.queue = append(w.queue, lines...)
	}
	return len(lines) > 0
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
	w.reply(g, 0, react.Stimulus{Kind: tipKind, From: g.st.Streams.Viewer, Text: text, Values: map[string]float64{"amount": float64(amount)}}, true)
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
	w.reply(g, host, react.Stimulus{Kind: tipKind, From: who, Text: text, Values: map[string]float64{"amount": float64(amount)}}, false)
}

// markSeen moves the channel on to its next episode.
func (w *streamWatch) markSeen(g *Game) {
	w.seen = true
	broadcast.MarkSeen(g.st.Streams, w.show)
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
		w.queue = append(w.queue, streamSay{text: g.fillLine(c.Stayed[g.desk.rng.IntN(len(c.Stayed))], g.st.Streams.Viewer, 0)})
		w.step(g)
		return
	}
	w.ended = true
	w.caption = ""
	g.saveStreams()
}

// leave goes back to the lobby.
func (w *streamWatch) leave(g *Game) {
	if w.spoken != nil {
		stopSpeaking()
	}
	closeBodies(w.bodies)
	g.sfx("cancel")
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
)

// controls are the controls shown now, in order: [kind, option].
func (w *streamWatch) controls() [][2]int {
	var out [][2]int
	if w.ask != nil && w.mine < 0 {
		for i := range w.ask.Options {
			out = append(out, [2]int{int(ctlAnswer), i})
		}
	}
	out = append(out, [2]int{int(ctlComment), 0}, [2]int{int(ctlTip), 0})
	if canSpeak() {
		out = append(out, [2]int{int(ctlSpeak), 0})
	}
	return out
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
	if w.naming {
		w.updateName(g)
		return
	}
	g.keepCrowd(w.crowd)
	w.chat.pull(w.crowd, now.Sub(w.opened))
	if w.age%180 == 0 {
		w.viewers = max(20, w.viewers+g.desk.rng.IntN(17)-7)
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
		if w.spoken != nil && w.spoken() {
			w.spoken = nil
			w.busy = now.Add(250 * time.Millisecond)
			w.talk[w.captionWho] = now
		}
		if !now.Before(w.busy) {
			w.spoken = nil
			w.step(g)
		}
	}
	w.input(g)
}

// updateName reads the viewer's name, asked before the first show.
func (w *streamWatch) updateName(g *Game) {
	in := &g.in
	if in.devices() && w.field.readTyping() {
		return
	}
	ok := !in.devices() && in.pressed[btnOK] || in.devices() && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter))
	sw, sh := g.screen()
	if !ok && !g.clickedIn(sw/2-60, sh/2+40, 120, 40) {
		return
	}
	name := strings.TrimSpace(w.field.take())
	g.st.Streams.Viewer = cmp.Or(string([]rune(name)[:min(utf8.RuneCountInString(name), 16)]), lang.T("名無しの視聴者"))
	w.naming = false
	g.sfx("decide")
	w.greet(g)
	g.saveStreams()
}

// input reads the keys and the pointer: typing a comment, the answers, the tips, reading
// aloud and going back.
func (w *streamWatch) input(g *Game) {
	in := &g.in
	sw, sh := g.screen()
	if w.ended {
		_, sy, stw, sth := g.watchStage()
		if in.ok() || in.cancel() || g.clickedIn(stw/2-80, sy+sth/2+20, 160, 44) {
			w.leave(g)
		}
		return
	}
	if w.typing {
		if in.devices() && w.field.readTyping() {
			return
		}
		send := !in.devices() && in.pressed[btnOK] || in.devices() && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter))
		switch {
		case send || g.clickedIn(sw-streamChatW+streamChatW-150, sh-62, 64, 44):
			w.comment(g)
		case !in.devices() && in.cancel() || in.devices() && inpututil.IsKeyJustPressed(ebiten.KeyEscape):
			w.field.composer.Cancel()
			w.typing = false
		}
		if !g.clickedIn(sw-streamChatW, sh-62, streamChatW, 62) && in.click {
			w.typing = false
		}
		return
	}
	if w.tips {
		w.updateTips(g)
		return
	}
	ctl := w.controls()
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
			if !g.desk.speak && w.spoken != nil {
				stopSpeaking()
			}
			g.sfx("decide")
		}
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

// controlRect is where a control is drawn.
func (w *streamWatch) controlRect(g *Game, c [2]int) (x, y, cw, ch float64) {
	sw, sh := g.screen()
	cx := sw - streamChatW + 14
	switch watchControl(c[0]) {
	case ctlAnswer:
		n := len(w.ask.Options)
		return cx, sh - 70 - float64(n-c[1])*40, streamChatW - 28, 34
	case ctlComment:
		return cx, sh - 58, streamChatW - 170, 42
	case ctlTip:
		return sw - 150, sh - 58, 64, 42
	case ctlSpeak:
	}
	return sw - 80, sh - 58, 66, 42
}

// tipRect is where the button of the i-th tip is drawn.
func (w *streamWatch) tipRect(g *Game, i int) (x, y, tw, th float64) {
	sw, sh := g.screen()
	n := len(g.db.Streams.Tips)
	return sw - streamChatW + 14, sh - 110 - float64(n-i)*46, streamChatW - 28, 40
}

func (w *streamWatch) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	dst.Fill(streamPage)
	x, y, stw, sth := g.watchStage()
	rect(dst, x, y, stw, sth, streamInk)
	n := len(w.show.Hosts)
	for i, c := range w.show.Hosts {
		shift, zoom := 0.0, 1.0
		if n > 1 {
			shift, zoom = 0.22-0.44*float64(i), 1.25
		}
		g.drawFramed(dst, c, w.bodies[i], x, y, stw, sth, shift, zoom, i == 0)
	}
	gfx.FillRoundRect(dst, float32(x+12), float32(y+12), 46, 22, 3, streamLive)
	gfx.DrawText(dst, "LIVE", x+35, y+14, 14, streamPanel)
	w.drawCaption(g, dst, x, y, stw, sth)
	if w.ended {
		rect(dst, x, y, stw, sth, withAlpha(colorBlack, 0x99))
		gfx.DrawText(dst, lang.T("配信は終了しました"), stw/2, y+sth/2-30, 24, streamPanel)
		gfx.FillRoundRect(dst, float32(stw/2-80), float32(y+sth/2+20), 160, 44, 6, streamBrand)
		gfx.DrawText(dst, lang.T("一覧へ"), stw/2, y+sth/2+30, 18, streamPanel)
	}
	w.drawInfo(g, dst, y+sth, stw)
	w.drawColumn(g, dst, sw, sh)
	g.streamTop(dst, lang.T("配信の一覧"))
	if w.naming {
		w.drawName(dst, sw, sh)
	}
	g.drawToast(dst)
}

// drawCaption draws the whole line being said as white text on black bands, with the
// speaker's name over it in a duo.
func (w *streamWatch) drawCaption(g *Game, dst *ebiten.Image, x, y, stw, sth float64) {
	if w.caption == "" {
		return
	}
	lines := gfx.Wrap(w.caption, captionSize, stw*captionShare)
	lh := gfx.LineHeight(captionSize)
	top := y + sth*0.95 - float64(len(lines))*lh
	if len(w.show.Hosts) > 1 {
		c := w.show.Hosts[w.captionWho]
		name := g.channelName(c)
		nw := gfx.TextWidth(name, 14)
		rect(dst, x+stw/2-nw/2-6, top-24, nw+12, 22, withAlpha(colorBlack, 0xb8))
		gfx.DrawText(dst, name, x+stw/2, top-22, 14, c.Color.Or(streamPanel))
	}
	for i, l := range lines {
		lw := gfx.TextWidth(l, captionSize)
		ly := top + float64(i)*lh
		rect(dst, x+stw/2-lw/2-8, ly, lw+16, lh, withAlpha(colorBlack, 0xc0))
		gfx.DrawText(dst, l, x+stw/2, ly+2, captionSize, streamPanel)
	}
}

// drawInfo draws the channel's information under the stage.
func (w *streamWatch) drawInfo(g *Game, dst *ebiten.Image, top, width float64) {
	_, sh := g.screen()
	rect(dst, 0, top, width, sh-top, streamPanel)
	c := w.show.Hosts[0]
	g.drawAvatar(dst, c, 16, top+14, 48)
	title, _ := gfx.FitLines(g.db.Tr(w.show.Episode.Title), 17, width-260, 2)
	for i, l := range title {
		gfx.DrawTextLeft(dst, l, 78, top+12+float64(i)*24, 17, streamInk)
	}
	ly := top + 16 + float64(len(title))*24
	name := g.showName(w.show)
	gfx.DrawTextLeft(dst, name, 78, ly, 15, streamInk)
	lx := 78 + gfx.TextWidth(name, 15) + 12
	gfx.DrawTextLeft(dst, g.db.Tr(c.Tag), lx, ly, 15, c.Color.Or(streamBrand))
	lx += gfx.TextWidth(g.db.Tr(c.Tag), 15) + 12
	g.drawStars(dst, lx, ly+3, g.step(c))
	if c.Tagline != "" {
		gfx.DrawTextLeft(dst, gfx.Ellipsis(g.db.Tr(c.Tagline), 14, width-100), 78, ly+26, 14, streamInk2)
	}
	gfx.DrawTextRight(dst, fmt.Sprintf(lang.T("%d人が視聴中"), w.viewers), width-16, top+16, 15, streamLive)
}

// drawColumn draws the chat column: the chat, the question's answers, the comment field
// and the buttons, and the tips when they are open.
func (w *streamWatch) drawColumn(g *Game, dst *ebiten.Image, sw, sh float64) {
	cx := sw - streamChatW
	rect(dst, cx, streamBarH, streamChatW, sh-streamBarH, streamPanel)
	rect(dst, cx, streamBarH, 1, sh-streamBarH, streamRule)
	gfx.DrawText(dst, lang.T("配信チャット"), cx+streamChatW/2, streamBarH+10, 14, streamInk2)
	rect(dst, cx, streamBarH+38, streamChatW, 1, streamRule)
	ctl := w.controls()
	bottom := sh - 70
	if w.ask != nil && w.mine < 0 {
		bottom -= float64(len(w.ask.Options))*40 + 28
	}
	g.drawChat(dst, w.chat, cx+14, streamBarH+46, bottom-streamBarH-52, g.money)
	if w.ask != nil && w.mine < 0 {
		gfx.DrawTextLeft(dst, lang.T("答える："), cx+14, bottom+2, 14, streamInk2)
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
			if w.naming || w.field.text+w.field.comp == "" && !w.typing {
				gfx.DrawTextLeft(dst, lang.T("コメントする"), x+10, y+11, 16, streamInk3)
			} else {
				w.field.draw(dst, x+10, y+11, cw-20, 16, streamInk, streamBrand, w.typing, w.age)
			}
			if w.typing {
				gfx.FillRoundRect(dst, float32(sw-150), float32(y), 64, float32(ch), 6, streamBrand)
				gfx.DrawText(dst, lang.T("送信"), sw-118, y+11, 15, streamPanel)
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
			label := lang.T("読み上げ")
			if g.desk.speak {
				edge = streamBrand
			}
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 6, 1.5, edge)
			gfx.DrawText(dst, label, x+cw/2, y+12, 13, streamInk2)
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

// drawName draws the box that asks the viewer's name.
func (w *streamWatch) drawName(dst *ebiten.Image, sw, sh float64) {
	fill(dst, withAlpha(colorBlack, 0x80))
	bx, by := sw/2-260, sh/2-120
	gfx.FillRoundRect(dst, float32(bx), float32(by), 520, 220, 12, streamPanel)
	gfx.DrawTextLeft(dst, lang.T("あなたの名前"), bx+28, by+22, 20, streamInk)
	gfx.DrawTextLeft(dst, lang.T("配信者に読まれる名前です。"), bx+28, by+56, 15, streamInk2)
	gfx.StrokeRoundRect(dst, float32(bx+28), float32(by+90), 464, 44, 6, 1.5, streamBrand)
	if w.field.text+w.field.comp == "" {
		gfx.DrawTextLeft(dst, lang.T("名無しの視聴者"), bx+40, by+101, 17, streamInk3)
	}
	w.field.draw(dst, bx+40, by+101, 440, 17, streamInk, streamBrand, true, w.age)
	gfx.FillRoundRect(dst, float32(sw/2-60), float32(sh/2+40), 120, 40, 6, streamBrand)
	gfx.DrawText(dst, lang.T("決める"), sw/2, sh/2+49, 16, streamPanel)
}
