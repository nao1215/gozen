"""Build the Lantern Quarter building materials from CC0 Poly Haven textures, using Pillow.

The photographic sources are CC0-1.0 textures from https://polyhaven.com, downloaded
at 1k and checked against the MD5 that Poly Haven's API publishes. The windows,
half-timbering, door sign and canopy are drawn over them here. Run from the checkout:

    python3 scripts/generate-space-buildings.py
"""
import hashlib
import io
import urllib.request
from pathlib import Path

from PIL import Image, ImageDraw, ImageEnhance, ImageFilter

root = Path(__file__).resolve().parents[1] / 'examples/space-buildings/game/assets'
SOURCES = {  # Poly Haven asset: MD5 of its 1k diffuse JPEG
    'cobblestone_floor_08': 'bb640125356d18e3ad8515b9d5bfccef',
    'old_wooden_floor_01': '624f0bef4c7edb616250e09f1d4831b8',
    'marble_mosaic_tiles': '87d75615bfc7457462370045bfc946a7',
    'white_plaster_02': '7b10c2686bcfd76d1f309a20fd06b57e',
    'weathered_brown_planks': 'e2fbdf7ed402e272e33e3f7ba707cf69',
    'red_brick': '269e69449755abd1ede473e634b670e7',
    'stone_wall_04': '0cc56635b6f6b1298f49fd97d4e0653d',
    'clay_roof_tiles_02': '43c45fa3ac4c68ffb28aa813cec32608',
}
URL = 'https://dl.polyhaven.org/file/ph-assets/Textures/jpg/1k/{0}/{0}_diff_1k.jpg'


def fetch(name):
    data = urllib.request.urlopen(urllib.request.Request(URL.format(name), headers={'User-Agent': 'gozen-example-build'})).read()
    want = SOURCES[name]
    if hashlib.md5(data).hexdigest() != want:  # #nosec -- an integrity check against Poly Haven's published MD5
        raise SystemExit(f'{name}: unexpected download')
    return Image.open(io.BytesIO(data)).convert('RGB')


def tiled(image, size):
    """Repeat a seamless texture to fill size, keeping its scale."""
    out = Image.new('RGB', size)
    for y in range(0, size[1], image.height):
        for x in range(0, size[0], image.width):
            out.paste(image, (x, y))
    return out


def window(canvas, box, frame, lit=False):
    """A wooden casement with four panes and a sill, drawn into canvas."""
    x0, y0, x1, y1 = box
    wood = frame.resize((x1 - x0 + 16, y1 - y0 + 24))
    canvas.paste(wood, (x0 - 8, y0 - 8))
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
    src = {name: fetch(name) for name in SOURCES}
    planks = src['weathered_brown_planks']

    # Floors and plain walls: the textures as they are, at 1024 pixels.
    src['cobblestone_floor_08'].save(root / 'cobble.jpg', quality=88)
    src['old_wooden_floor_01'].save(root / 'boards.jpg', quality=88)
    src['marble_mosaic_tiles'].save(root / 'mosaic.jpg', quality=88)
    src['clay_roof_tiles_02'].save(root / 'roof.jpg', quality=88)
    src['stone_wall_04'].crop((0, 0, 1024, 768)).save(root / 'stone.jpg', quality=88)

    # The inn: one 4 x 3 facade, whitewashed plaster framed by dark timbers,
    # two windows on its upper story.
    inn = ImageEnhance.Brightness(tiled(src['white_plaster_02'], (1024, 768))).enhance(1.15)
    beam_v = planks.rotate(90, expand=True)
    for x in (0, 256, 512, 768, 1000):
        inn.paste(beam_v.resize((24, 768)), (x, 0))
    for y in (0, 366, 748):
        inn.paste(planks.resize((1024, 20)), (0, y))
    for x in (60, 572):
        window(inn, (x, 90, x + 150, 270), planks)
    inn.save(root / 'timber.jpg', quality=88)

    # The shop: a 6 x 1.5 brick front (its doorway is the third unit), a display
    # window each side and a sign over the left one. Two frames, the sign's lamps out and lit, animate it.
    brick = ImageEnhance.Color(src['red_brick']).enhance(1.1)
    front = tiled(brick.resize((512, 512)), (2048, 512))
    for x0, x1 in ((110, 590), (1260, 1900)):
        window(front, (x0, 190, x1, 450), planks)
    for frame, lit in enumerate((False, True)):
        shop = front.copy()
        board = (70, 34, 630, 140)
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
