#!/usr/bin/env python3
"""Generate the 81px tab bar icon set with a consistent rounded outline style."""
from pathlib import Path
from PIL import Image, ImageDraw

SIZE = 81
STROKE = 5
RADIUS = 3
COLORS = {"default": "#7A828C", "active": "#202A37"}
OUTPUT = Path(__file__).resolve().parent.parent / "src/static/tabbar"


def canvas(color):
    image = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    return image, ImageDraw.Draw(image), color


def line(draw, points, color, width=STROKE):
    draw.line(points, fill=color, width=width, joint="curve")
    r = width / 2
    for x, y in (points[0], points[-1]):
        draw.ellipse((x-r, y-r, x+r, y+r), fill=color)


def rounded_outline(draw, box, color, radius=RADIUS, width=STROKE):
    draw.rounded_rectangle(box, radius=radius, outline=color, width=width)


def home(color):
    image, draw, color = canvas(color)
    # A small studio/home: pitched roof, framed facade, and door.
    line(draw, [(15, 39), (40.5, 18), (66, 39)], color)
    line(draw, [(20, 36), (20, 64), (61, 64), (61, 36)], color)
    rounded_outline(draw, (34, 46, 47, 64), color, 2, STROKE)
    return image


def classroom(color):
    image, draw, color = canvas(color)
    # Presentation window with a play symbol, suitable for a teacher's class.
    rounded_outline(draw, (13, 19, 68, 59), color, 6, STROKE)
    line(draw, [(13, 31), (68, 31)], color, 4)
    draw.polygon([(35, 38), (35, 51), (48, 44.5)], fill=color)
    line(draw, [(29, 65), (52, 65)], color, 4)
    return image


def enterprise(color):
    image, draw, color = canvas(color)
    # Briefcase outline, with a compact handle and central clasp.
    rounded_outline(draw, (12, 29, 69, 62), color, 6, STROKE)
    rounded_outline(draw, (31, 20, 50, 33), color, 4, STROKE)
    line(draw, [(12, 43), (69, 43)], color, 4)
    rounded_outline(draw, (36, 40, 45, 47), color, 2, 4)
    return image


def profile(color):
    image, draw, color = canvas(color)
    # Person silhouette rendered as two outline contours.
    draw.ellipse((29, 16, 52, 39), outline=color, width=STROKE)
    draw.arc((17, 36, 64, 74), 198, 342, fill=color, width=STROKE)
    line(draw, [(20, 64), (61, 64)], color)
    return image


ICONS = {
    "home": home,
    "classroom": classroom,
    "enterprise": enterprise,
    "profile": profile,
}


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    for name, draw_icon in ICONS.items():
        for state, color in COLORS.items():
            suffix = "" if state == "default" else "-active"
            draw_icon(color).save(OUTPUT / f"{name}{suffix}.png", "PNG")


if __name__ == "__main__":
    main()
