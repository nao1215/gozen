# State-selected portraits and instant narration

A speaker can select an existing face with a pure expression. The selection does
not consume random numbers or change the line's read identity. An empty result
uses the actor's default face. `face` and `face_expr` are mutually exclusive.
The author supplies the images in the actor's `faces` directory.

```yaml
schema_version: 11
# In an event:
do:
  - say:
      who: potter
      face_expr: 'flag.festival ? "ceremony" : "work"'
      text: The kiln is ready.
  - say:
      instant: true
      text: A new season begins.
```

`instant` reveals each page immediately, so a single advance moves to the next
page or line. It also applies after reflow and language changes.

An ensemble's `view.guests: [visitor]` includes an actor without adding an entity
to the simulation. The pottery workshop demonstrates a visiting speaker and an
instant line. Guest portraits share the resident framing and first available
resident backdrop. They do not expose entity actions or enter entity counts.

Ensemble portraits use detected face boxes from `assets/faces.json` to align the
face top, height, and horizontal center. Images are clipped to their own tile.
Without usable metadata, framing uses a centered upper-body fallback. A novel
stage can use `ui.stage_framing` to specify the same anchors explicitly; its
`head` is the face center X, top Y, and height as source-image fractions, while
`top` and `height` are fractions of the screen height.

## Meeting transcript

An ensemble can opt into a bounded 4:3 tile grid and a read-only transcript:

```yaml
screens:
  roundtable:
    title: Workshop roundtable
    kind: ensemble
    view:
      order: [potter, weaver]
      visible: present
      camera: camera
      portrait: portrait
      icon: icon
      backdrop: room
      mask: private_room
      chat:
        title: Workshop messages
        joined: joined
        left: left
        camera_off: Camera off
        camera_on: Camera on
        reaction: response
```

Use `say: {who: potter, text: The kiln is ready., ensemble: roundtable}` to
append a spoken line, highlight the speaker, and show a caption. The novel
presentation paginates captions beside the transcript. `response` is an optional
public observation, such as `"Ready to weave."`; an empty string means silence.
After each spoken line, at most one changed response is posted in participant
order. No random number is drawn. Presence and camera transitions are compared
when a line opens; rendering does not append messages. A standalone ensemble
shows current presence notices. The transcript retains the latest 100 entries;
the column displays the newest entries that fit. Use the ordinary dialogue log
to review earlier spoken lines. The meeting transcript is presentation state,
not a saved record: another ensemble, a non-ensemble line, or a new date/phase
starts a new session. It does not change the planning simulation.

With `screen.native: true`, the fitted 16:9 target uses device pixels (including
fractional device scale factors). A 3840 by 2160 window renders at that size;
a 2560 by 1080 window renders a 1920 by 1080 center with side bars, without
upscaling. Fixed-size games retain their authored framebuffer. Image quality
still depends on the source face's pixel count, not just the whole image size.


## Desktop calls

A fullscreen ensemble view may set `call` alongside `chat`. Camera-on speakers occupy the
main stage, with other participants in the strip and camera-off icons remaining
small. Captions replace the dialogue box; game navigation stays outside the browser.
Motion uses presentation ticks and a stable participant phase, never game randomness.
Detected face bounds anchor breathing, sway and speaking nods. Optional full-canvas
variants must have identical dimensions and alignment to the base image:

```yaml
call:
  address: call.example.test/workshop
  controls: [Mic, Camera, Share, Hand, Chat, Leave]
  mouth: {portraits/potter.png: portraits/potter-speaking.png}
  blink: {portraits/potter.png: portraits/potter-blink.png}
```

Blink takes precedence over mouth. Missing variants fall back to the base portrait.
Controls and the address are decorative, and never connect to a network service.
