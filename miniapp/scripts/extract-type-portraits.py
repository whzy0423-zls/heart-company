#!/usr/bin/env python3
"""Extract the existing nine-type illustrations without changing the artwork.

Requires Pillow. The pale blue background is flood-filled from the image edge,
so light clothing enclosed by the silhouette stays intact. The original images
are never modified. Run from any directory with:
    python3 miniapp/scripts/extract-type-portraits.py
"""

from collections import deque
from pathlib import Path

from PIL import Image, ImageDraw


MINIAPP = Path(__file__).resolve().parents[1]
SOURCE = MINIAPP / "src/static/enneagram"
DESTINATION = MINIAPP / "src/static/enneagram-cutouts"
BACKGROUND = (238, 241, 247)
HEIGHT = 160
PADDING = 4


def cutout(source: Path) -> Image.Image:
    image = Image.open(source).convert("RGBA")
    width, height = image.size
    pixels = image.load()
    visited = bytearray(width * height)
    queue = deque()

    def add(x: int, y: int) -> None:
        position = y * width + x
        if visited[position]:
            return
        visited[position] = 1
        red, green, blue, _ = pixels[x, y]
        distance = sum((channel - background) ** 2
                       for channel, background in zip((red, green, blue), BACKGROUND))
        # The color/chroma restriction distinguishes the cool background from
        # the neutral or warm whites in shirts, collars and the camera strap.
        if distance <= 625 and blue - red >= 3 and blue >= green:
            pixels[x, y] = (0, 0, 0, 0)
            queue.append((x, y))

    for x in range(width):
        add(x, 0)
        add(x, height - 1)
    for y in range(height):
        add(0, y)
        add(width - 1, y)
    while queue:
        x, y = queue.popleft()
        for next_x, next_y in ((x - 1, y), (x + 1, y), (x, y - 1), (x, y + 1)):
            if 0 <= next_x < width and 0 <= next_y < height:
                add(next_x, next_y)

    box = image.getbbox()
    if box is None:
        raise ValueError(f"No portrait found in {source}")
    cropped = image.crop(box)
    target_width = round(cropped.width * HEIGHT / cropped.height)
    # Pillow's RGBA resampling premultiplies alpha, avoiding a dark edge when
    # the antialiased portrait is displayed on cream or colored selection cards.
    resized = cropped.resize((target_width, HEIGHT), Image.Resampling.LANCZOS)
    result = Image.new("RGBA", (target_width + PADDING * 2, HEIGHT))
    result.paste(resized, (PADDING, 0))
    return result


def main() -> None:
    DESTINATION.mkdir(parents=True, exist_ok=True)
    contact = Image.new("RGB", (1200, 660), "#efe9df")
    draw = ImageDraw.Draw(contact)
    total_bytes = 0
    for number in range(1, 10):
        portrait = cutout(SOURCE / f"{number}.png")
        output = DESTINATION / f"{number}.png"
        # Indexed PNG preserves transparency while keeping the nine bundled
        # selection thumbnails within the WeChat main-package size budget.
        portrait.quantize(colors=112, method=Image.Quantize.FASTOCTREE,
                          dither=Image.Dither.NONE).save(output, optimize=True)
        saved = Image.open(output).convert("RGBA")
        total_bytes += output.stat().st_size
        print(f"{number}: {saved.width}x{saved.height}, {output.stat().st_size} bytes")
        column, row = (number - 1) % 3, (number - 1) // 3
        original = Image.open(SOURCE / f"{number}.png").resize(
            (160, 160), Image.Resampling.LANCZOS)
        contact.paste(original, (column * 400 + 20, row * 220 + 45))
        position = (column * 400 + 220 + (160 - saved.width) // 2, row * 220 + 45)
        contact.paste(saved, position, saved)
        draw.text((column * 400 + 20, row * 220 + 20),
                  f"{number} / ORIGINAL", fill="#403b35")
        draw.text((column * 400 + 220, row * 220 + 20),
                  f"{number} / TRANSPARENT", fill="#403b35")
    contact.save("/tmp/nine-xing-type-cutouts.jpg", quality=94)
    print(f"Total: {total_bytes} bytes")


if __name__ == "__main__":
    main()
