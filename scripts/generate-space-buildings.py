"""Generate original neutral building materials (CC0-1.0), using Pillow."""
from pathlib import Path
from PIL import Image, ImageDraw

root = Path(__file__).resolve().parents[1] / 'examples/space-buildings/game/assets'
root.mkdir(parents=True, exist_ok=True)
for name in ('timber', 'shop', 'stone', 'roof', 'canopy'):
    image = Image.new('RGB', (256, 256), '#e4d6b1')
    draw = ImageDraw.Draw(image)
    if name == 'timber':
        for x in (0, 64, 128, 192, 252):
            draw.rectangle((x, 0, x + 5, 255), fill='#513928')
        for y in (0, 122, 250):
            draw.rectangle((0, y, 255, y + 6), fill='#513928')
        for x in (12, 140):
            draw.rectangle((x, 24, x + 38, 84), fill='#654a30')
            draw.rectangle((x + 4, 28, x + 34, 80), fill='#57777b')
            draw.line((x + 19, 28, x + 19, 80), fill='#e4d6b1', width=3)
            draw.line((x + 4, 54, x + 34, 54), fill='#e4d6b1', width=3)
    elif name == 'shop':
        draw.rectangle((0, 0, 255, 255), fill='#a55340')
        for y in range(0, 256, 32):
            draw.line((0, y, 255, y), fill='#733a2e', width=2)
        for x in (12, 180):
            draw.rectangle((x, 48, x + 56, 190), fill='#f0cf8c')
            draw.rectangle((x + 5, 54, x + 51, 184), fill='#405b65')
            draw.line((x + 28, 54, x + 28, 184), fill='#f0cf8c', width=4)
        draw.rectangle((77, 12, 174, 43), fill='#dfb868')
    elif name == 'stone':
        draw.rectangle((0, 0, 255, 255), fill='#8b9399')
        for row, y in enumerate(range(0, 256, 32)):
            draw.line((0, y, 255, y), fill='#555e67', width=3)
            for x in range(-32 if row % 2 else 0, 256, 64):
                draw.line((x, y, x, y + 32), fill='#555e67', width=3)
    elif name == 'roof':
        draw.rectangle((0, 0, 255, 255), fill='#efd3ba')
        for y in range(0, 256, 32):
            draw.line((0, y, 255, y), fill='#94705d', width=4)
            for x in range(0, 256, 32):
                draw.arc((x, y - 22, x + 32, y + 12), 0, 180, fill='#ad8a70', width=2)
    else:
        for x in range(0, 256, 32):
            draw.rectangle((x, 0, x + 15, 255), fill='#b65b45')
    image.save(root / (name + '.png'))
