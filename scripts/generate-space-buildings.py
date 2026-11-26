"""Build the Lantern Quarter building materials from CC0 Poly Haven textures, using Pillow.

The photographic sources are CC0-1.0 textures from https://polyhaven.com: each
1k color map and its OpenGL normal map, checked against the MD5 that Poly Haven's
API publishes. The windows, half-timbering, door sign and canopy are drawn over
them here, into the color and the normal map alike. Run from the checkout:

    python3 scripts/generate-space-buildings.py
"""
import hashlib
import io
import urllib.request
from pathlib import Path

from PIL import Image, ImageDraw, ImageEnhance, ImageFilter

root = Path(__file__).resolve().parents[1] / 'examples/space-buildings/game/assets'
SOURCES = {  # Poly Haven asset: MD5s of its 1k diffuse and nor_gl JPEGs
    'cobblestone_floor_08': ('bb640125356d18e3ad8515b9d5bfccef', '7a90780c7c49a1307e928d4d32c4773c'),
    'old_wooden_floor_01': ('624f0bef4c7edb616250e09f1d4831b8', '0be60a8fe330fd21b112afdaf17887c2'),
    'marble_mosaic_tiles': ('87d75615bfc7457462370045bfc946a7', 'ee181f743b986c438726215c4c1e1bbd'),
    'white_plaster_02': ('7b10c2686bcfd76d1f309a20fd06b57e', '55fc03203ebf1dc703a380fde93ff502'),
    'weathered_brown_planks': ('e2fbdf7ed402e272e33e3f7ba707cf69', '1180a2ce3566529b25128252e02bd049'),
    'red_brick': ('269e69449755abd1ede473e634b670e7', '3a49f27fb909b2db794f60bf1ce6659f'),
    'stone_wall_04': ('0cc56635b6f6b1298f49fd97d4e0653d', 'b1153b5648efe22657b6656a7358fde2'),
    'clay_roof_tiles_02': ('43c45fa3ac4c68ffb28aa813cec32608', '9e24ddc6fbf048291f8e24bfb1f16e6d'),
}
URL = 'https://dl.polyhaven.org/file/ph-assets/Textures/jpg/1k/{0}/{0}_{1}_1k.jpg'
FLAT = (128, 128, 255)  # a normal map's "straight out of the surface"


def fetch(name, kind):
    data = urllib.request.urlopen(urllib.request.Request(URL.format(name, kind), headers={'User-Agent': 'gozen-example-build'})).read()
    want = SOURCES[name][0 if kind == 'diff' else 1]
    if hashlib.md5(data).hexdigest() != want:  # #nosec -- an integrity check against Poly Haven's published MD5
        raise SystemExit(f'{name} {kind}: unexpected download')
    return Image.open(io.BytesIO(data)).convert('RGB')


def turn_normal(image):
    """Rotate a normal map a quarter turn counterclockwise, turning its vectors too."""
    r, g, b = image.rotate(90, expand=True).split()
    return Image.merge('RGB', (g.point(lambda v: 255 - v), r, b))


def tiled(image, size):
    """Repeat a seamless texture to fill size, keeping its scale."""
    out = Image.new('RGB', size)
    for y in range(0, size[1], image.height):
        for x in range(0, size[0], image.width):
            out.paste(image, (x, y))
    return out


def window(canvas, normal, box, frame, frame_normal, lit=False):
    """A wooden casement with four panes and a sill, drawn into canvas and its normal map."""
    x0, y0, x1, y1 = box
    wood = frame.resize((x1 - x0 + 16, y1 - y0 + 24))
    canvas.paste(wood, (x0 - 8, y0 - 8))
    normal.paste(frame_normal.resize(wood.size), (x0 - 8, y0 - 8))
    n = ImageDraw.Draw(normal)
    n.rectangle((x0 + 6, y0 + 6, x1 - 7, y1 - 7), fill=FLAT)
    # The glass sits back in the frame: its inner edges face into the opening.
    n.rectangle((x0 + 6, y0 + 6, x0 + 11, y1 - 7), fill=(200, 128, 200))
    n.rectangle((x1 - 12, y0 + 6, x1 - 7, y1 - 7), fill=(56, 128, 200))
    n.rectangle((x0 + 6, y0 + 6, x1 - 7, y0 + 11), fill=(128, 56, 200))
    n.rectangle((x0 + 6, y1 - 12, x1 - 7, y1 - 7), fill=(128, 200, 200))
    normal.paste(frame_normal.resize((x1 - x0 + 36, 18)), (x0 - 18, y1 + 6))
    glass = Image.new('RGB', (x1 - x0 - 12, y1 - y0 - 12))
    g = ImageDraw.Draw(glass)
    top, bottom = ((255, 214, 140), (196, 130, 60)) if lit else ((120, 150, 168), (44, 62, 76))
    for y in range(glass.height):
        t = y / max(1, glass.height - 1)
        g.line((0, y, glass.width, y), fill=tuple(int(a + (b - a) * t) for a, b in zip(top, bottom)))
    g.polygon([(glass.width * .15, 0), (glass.width * .45, 0), (glass.width * .1, glass.height), (0, glass.height)], fill=(150, 178, 192) if not lit else (255, 230, 170))
    canvas.paste(glass, (x0 + 6, y0 + 6))
    d = ImageDraw.Draw(canvas)
    mx, my = (x0 + x1) // 2, (y0 + y1) // 2
    d.rectangle((mx - 4, y0 + 6, mx + 4, y1 - 6), fill=(70, 48, 30))
    d.rectangle((x0 + 6, my - 4, x1 - 6, my + 4), fill=(70, 48, 30))
    canvas.paste(frame.resize((x1 - x0 + 36, 18)), (x0 - 18, y1 + 6))


def main():
    root.mkdir(parents=True, exist_ok=True)
    src = {name: fetch(name, 'diff') for name in SOURCES}
    nor = {name: fetch(name, 'nor_gl') for name in SOURCES}
    planks, planks_n = src['weathered_brown_planks'], nor['weathered_brown_planks']
    (root / 'normal').mkdir(exist_ok=True)

    # Floors and plain walls: the textures as they are, at 1024 pixels.
    for name, out in (('cobblestone_floor_08', 'cobble'), ('old_wooden_floor_01', 'boards'), ('marble_mosaic_tiles', 'mosaic'), ('clay_roof_tiles_02', 'roof')):
        src[name].save(root / f'{out}.jpg', quality=88)
        nor[name].save(root / 'normal' / f'{out}.jpg', quality=92)
    src['stone_wall_04'].crop((0, 0, 1024, 768)).save(root / 'stone.jpg', quality=88)
    nor['stone_wall_04'].crop((0, 0, 1024, 768)).save(root / 'normal' / 'stone.jpg', quality=92)

    # The inn: one 4 x 3 facade, whitewashed plaster framed by dark timbers,
    # two windows on its upper story.
    inn = ImageEnhance.Brightness(tiled(src['white_plaster_02'], (1024, 768))).enhance(1.15)
    inn_n = tiled(nor['white_plaster_02'], (1024, 768))
    beam_v, beam_vn = planks.rotate(90, expand=True), turn_normal(planks_n)
    for x in (0, 256, 512, 768, 1000):
        inn.paste(beam_v.resize((24, 768)), (x, 0))
        inn_n.paste(beam_vn.resize((24, 768)), (x, 0))
    for y in (0, 366, 748):
        inn.paste(planks.resize((1024, 20)), (0, y))
        inn_n.paste(planks_n.resize((1024, 20)), (0, y))
    for x in (60, 572):
        window(inn, inn_n, (x, 90, x + 150, 270), planks, planks_n)
    inn.save(root / 'timber.jpg', quality=88)
    inn_n.save(root / 'normal' / 'timber.jpg', quality=92)

    # The shop: a 6 x 1.5 brick front (its doorway is the third unit), a display
    # window each side and a sign over the left one. Two frames, the sign's lamps out and lit, animate it.
    brick = ImageEnhance.Color(src['red_brick']).enhance(1.1)
    front = tiled(brick.resize((512, 512)), (2048, 512))
    front_n = tiled(nor['red_brick'].resize((512, 512)), (2048, 512))
    for x0, x1 in ((110, 590), (1260, 1900)):
        window(front, front_n, (x0, 190, x1, 450), planks, planks_n)
    board = (70, 34, 630, 140)
    front_n.paste(planks_n.resize((board[2] - board[0], board[3] - board[1])), board[:2])
    front_n.save(root / 'normal' / 'shop.jpg', quality=92)
    for frame, lit in enumerate((False, True)):
        shop = front.copy()
        shop.paste(planks.resize((board[2] - board[0], board[3] - board[1])), board[:2])
        d = ImageDraw.Draw(shop)
        d.rectangle(board, outline=(196, 160, 82), width=6)
        bulbs = range(board[0] + 40, board[2] - 20, 64)
        if lit:
            # Each lamp throws a soft round glow over the board and the bricks.
            glow = Image.new('L', shop.size, 0)
            for x in bulbs:
                ImageDraw.Draw(glow).ellipse((x - 46, board[1] + 7, x + 46, board[1] + 99), fill=255)
            glow = glow.filter(ImageFilter.GaussianBlur(18))
            shop = Image.composite(Image.new('RGB', shop.size, (255, 222, 140)), shop, glow.point(lambda v: v * 3 // 4))
            d = ImageDraw.Draw(shop)
        for x in bulbs:
            color = (255, 250, 220) if lit else (92, 78, 52)
            d.ellipse((x - 18, board[1] + 35, x + 18, board[1] + 71), fill=color, outline=(60, 44, 28), width=3)
        (root / 'shop').mkdir(exist_ok=True)
        shop.save(root / 'shop' / f'{frame}.jpg', quality=88)

    # The canopy: cream and red awning stripes with soft folds.
    canopy = Image.new('RGB', (512, 256))
    d = ImageDraw.Draw(canopy)
    for x in range(0, 512, 64):
        d.rectangle((x, 0, x + 31, 255), fill=(182, 58, 48))
        d.rectangle((x + 32, 0, x + 63, 255), fill=(236, 226, 204))
    shade = Image.new('L', (512, 256))
    ImageDraw.Draw(shade).rectangle((0, 0, 511, 255), fill=0)
    for x in range(0, 512, 32):
        ImageDraw.Draw(shade).line((x, 0, x, 255), fill=70, width=6)
    shade = shade.filter(ImageFilter.GaussianBlur(6))
    canopy = Image.composite(Image.new('RGB', canopy.size, (40, 20, 16)), canopy, shade)
    canopy.save(root / 'canopy.jpg', quality=88)


if __name__ == '__main__':
    main()
