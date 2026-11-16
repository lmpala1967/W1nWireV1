"""Generate the W1nWire application icon.

The icon is drawn from scratch so that it can be regenerated at any size and
committed as a build artifact instead of being an opaque binary. Run it with
Pillow installed:

    python packaging/icon/make_icon.py

It writes w1nwire.ico (16, 24, 32, 48, 64, 128, 256) next to itself, plus a
256x256 PNG preview used in the README.
"""

from __future__ import annotations

import math
import os

from PIL import Image, ImageDraw, ImageFilter, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
SIZES = (16, 24, 32, 48, 64, 128, 256)

# The accent gradient of the interface.
TOP_LEFT = (91, 140, 255)      # #5B8CFF
BOTTOM_RIGHT = (139, 92, 246)  # #8B5CF6
DARK = (10, 12, 17)            # #0A0C11


def rounded_mask(size: int, radius_ratio: float = 0.225) -> Image.Image:
    """Return an antialiased mask of a rounded square, supersampled 4x."""
    scale = 4
    big = Image.new("L", (size * scale, size * scale), 0)
    draw = ImageDraw.Draw(big)
    radius = int(size * scale * radius_ratio)
    draw.rounded_rectangle(
        (0, 0, size * scale - 1, size * scale - 1), radius=radius, fill=255
    )
    return big.resize((size, size), Image.LANCZOS)


def gradient(size: int) -> Image.Image:
    """A diagonal gradient, computed per pixel so it stays smooth at any size."""
    img = Image.new("RGB", (size, size))
    pixels = img.load()
    for y in range(size):
        for x in range(size):
            t = (x / max(size - 1, 1) + y / max(size - 1, 1)) / 2
            # Ease the gradient so the middle stays vivid.
            t = t ** 0.9
            pixels[x, y] = (
                int(TOP_LEFT[0] + (BOTTOM_RIGHT[0] - TOP_LEFT[0]) * t),
                int(TOP_LEFT[1] + (BOTTOM_RIGHT[1] - TOP_LEFT[1]) * t),
                int(TOP_LEFT[2] + (BOTTOM_RIGHT[2] - TOP_LEFT[2]) * t),
            )
    return img


def load_font(px: int) -> ImageFont.FreeTypeFont:
    candidates = [
        r"C:\Windows\Fonts\segoeuib.ttf",
        r"C:\Windows\Fonts\arialbd.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    ]
    for path in candidates:
        if os.path.exists(path):
            return ImageFont.truetype(path, px)
    return ImageFont.load_default()


def draw_letter(canvas: Image.Image, size: int) -> None:
    """Draw the W mark, optically centred."""
    font = load_font(int(size * 0.56))
    layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    box = draw.textbbox((0, 0), "W", font=font)
    w = box[2] - box[0]
    h = box[3] - box[1]
    x = (size - w) / 2 - box[0]
    # Segoe UI has a large ascent; shift down a hair so the glyph sits
    # optically in the middle of the square.
    y = (size - h) / 2 - box[1] + size * 0.015
    draw.text((x, y), "W", font=font, fill=(255, 255, 255, 255))
    canvas.alpha_composite(layer)


def draw_glow(canvas: Image.Image, size: int) -> None:
    """A soft light from the top left, which gives the flat square depth."""
    glow = Image.new("L", (size, size), 0)
    draw = ImageDraw.Draw(glow)
    draw.ellipse(
        (-size * 0.35, -size * 0.45, size * 0.75, size * 0.55), fill=70
    )
    glow = glow.filter(ImageFilter.GaussianBlur(size * 0.12))
    white = Image.new("RGBA", (size, size), (255, 255, 255, 0))
    white.putalpha(glow)
    canvas.alpha_composite(white)


def render(size: int) -> Image.Image:
    base = gradient(size).convert("RGBA")
    draw_glow(base, size)
    draw_letter(base, size)
    base.putalpha(rounded_mask(size))
    return base


def main() -> None:
    master = render(1024)
    master.save(os.path.join(HERE, "w1nwire.png"))
    master.resize((256, 256), Image.LANCZOS).save(
        os.path.join(HERE, "w1nwire-256.png")
    )

    images = [render(s) for s in SIZES]
    ico_path = os.path.join(HERE, "w1nwire.ico")
    images[-1].save(
        ico_path,
        format="ICO",
        sizes=[(s, s) for s in SIZES],
        append_images=images[:-1],
    )
    print("wrote", ico_path)
    print("wrote", os.path.join(HERE, "w1nwire.png"))


if __name__ == "__main__":
    main()
