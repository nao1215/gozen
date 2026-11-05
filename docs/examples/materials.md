# Layered interface materials

Use `ui.material` to add soft backdrop sampling to planning and dialogue windows.
Omission retains the classic renderer. The workshop example uses this configuration:

```yaml
ui:
  material:
    kind: glass
    blur: 12
    opacity: 0.68
    border: 0.55
    shadow: 0.7
    surfaces: {base: mica, card: acrylic, control: glass}
    wallpaper: ["#e5e9ef", "#b6d4d5", "#d9ccbb"]
```

`kind` supplies the default. `surfaces` overrides base (header), chart (work plot), card
(resident cards), and control (other panels). Mica keeps at least 90% tint;
acrylic keeps at least 72%; glass uses the specified opacity and gently offsets
backdrop samples near the edge. Opaque uses only tint. This is an engine material,
not a native operating-system compositor or an exact Liquid Glass reproduction.

Blur is a logical-pixel downsampling footprint (0–64). Opacity, border and shadow
are strengths from 0 to 1. Three wallpaper colors create a cached low-resolution
smooth field on light planning screens without an authored background image.
Scene pictures remain sharp outside panels. Dialogue captures the rendered scene
before drawing text, so its panels sample scenery and portraits, never previous
frame text. Planning controls capture the completed content layer.

A shared reduced backdrop is reused within a layer and reallocated only on size
or blur changes. The gradient texture is cached for the loaded game. Rounded
signed-distance edges are antialiased in output pixels; shadow size and reflection
width follow logical scaling. Hover brightens interactive panels slightly; mouse press darkens
and lowers the surface. Text remains unfiltered above the material.


For visible transmission, use glass surfaces with a low tint opacity (for example
0.24), saturated wallpaper colors and `wallpaper_strength: 1`. `refraction: 16`
adds logical-pixel displacement near the edge (range 0–32). Panels sample the
current drawing layer, including earlier windows, rather than only the wallpaper.
Text is painted after the blur and stays sharp. Mica and acrylic retain their
higher minimum tint opacity for authors who prefer opaque content surfaces.

Use `surfaces: {base: glass, chart: mica, card: glass, control: glass}`
for a quiet chart on a translucent dashboard. The chart inherits `base` when its
own override is absent. Mica removes edge refraction and keeps at least 90% tint,
so grid lines, legends and data remain readable over colorful wallpaper.

A fullscreen islands view can set `window: Workshop project — Workboard` to draw
a desktop frame and title bar. Its decorative minimize, maximize and close shapes
share the call window styling. Authored hotspots should start below logical y=104;
the chart occupies y=104–284 and resident cards y=292–602. The existing bottom tabs
provide navigation. This option changes presentation only, not event execution.

Meeting chat displays join, leave and camera notices plus the short text selected
by `view.chat.reaction`. Spoken lines appear only in subtitles and dialogue history.
Responses use public observations, stable participant order and at most one new
response per spoken line. An unchanged response is posted once per session.
