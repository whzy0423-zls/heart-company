#!/usr/bin/env python3
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont, ImageFilter, ImageOps
import math
import os


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "src" / "static" / "share"
AVATARS = ROOT / "src" / "static" / "avatars"
WIDTH, HEIGHT = 1000, 800
SHARE_SIZE = (500, 400)

def resolve_font(env_name, candidates):
    configured = os.environ.get(env_name)
    for candidate in ([configured] if configured else []) + candidates:
        if candidate and Path(candidate).is_file():
            return candidate
    raise RuntimeError(f"未找到分享卡字体，请通过 {env_name} 指定可用字体文件")


FONT_CN = resolve_font("NX_SHARE_FONT_CN", [
    "/System/Library/Fonts/STHeiti Medium.ttc",
    "/System/Library/Fonts/PingFang.ttc",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf",
])
FONT_LATIN = resolve_font("NX_SHARE_FONT_LATIN", [
    "/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
    "/System/Library/Fonts/Helvetica.ttc",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    FONT_CN,
])

TYPES = {
    1: ("完美型 · 理想主义者", "THE REFORMER", "原则 · 责任 · 自我要求"),
    2: ("助人型 · 给予者", "THE HELPER", "关怀 · 连接 · 被需要"),
    3: ("成就型 · 实干家", "THE ACHIEVER", "目标 · 效率 · 影响力"),
    4: ("自我型 · 浪漫者", "THE INDIVIDUALIST", "独特 · 感受 · 深度"),
    5: ("观察型 · 思想者", "THE INVESTIGATOR", "理性 · 求知 · 边界"),
    6: ("忠诚型 · 怀疑者", "THE LOYALIST", "忠诚 · 责任 · 安全感"),
    7: ("活跃型 · 探索者", "THE ENTHUSIAST", "乐观 · 可能性 · 体验"),
    8: ("领袖型 · 挑战者", "THE CHALLENGER", "力量 · 掌控 · 保护"),
    9: ("和平型 · 调停者", "THE PEACEMAKER", "包容 · 稳定 · 调和"),
}

NAVY = (15, 23, 42)
NAVY_LIGHT = (30, 42, 65)
IVORY = (250, 247, 239)
GOLD = (217, 181, 109)
GOLD_LIGHT = (239, 218, 168)
MUTED = (179, 190, 207)


def font(size, latin=False):
    return ImageFont.truetype(FONT_LATIN if latin else FONT_CN, size=size)


def radial_background():
    image = Image.new("RGB", (WIDTH, HEIGHT), NAVY)
    pixels = image.load()
    glow_x, glow_y = 770, 310
    for y in range(HEIGHT):
        for x in range(WIDTH):
            dx = (x - glow_x) / 620
            dy = (y - glow_y) / 520
            glow = max(0.0, 1.0 - math.sqrt(dx * dx + dy * dy))
            edge = max(0.0, 1.0 - math.sqrt(((x - 140) / 900) ** 2 + ((y - 650) / 900) ** 2))
            amount = min(1.0, glow * 0.48 + edge * 0.08)
            pixels[x, y] = tuple(
                round(NAVY[channel] * (1 - amount) + NAVY_LIGHT[channel] * amount)
                for channel in range(3)
            )
    return image


def draw_brand(draw):
    draw.rounded_rectangle((82, 70, 122, 78), radius=4, fill=GOLD)
    draw.text((140, 52), "九型芯之力", fill=IVORY, font=font(34))
    draw.text((82, 108), "ENNEAGRAM · PERSONAL GROWTH", fill=MUTED, font=font(18, latin=True))


def draw_right_visual(image, draw, type_number=None):
    panel = Image.new("RGBA", (390, 640), (0, 0, 0, 0))
    panel_draw = ImageDraw.Draw(panel)
    panel_draw.rounded_rectangle((0, 0, 390, 640), radius=56, fill=(*IVORY, 245))

    shadow = Image.new("RGBA", image.size, (0, 0, 0, 0))
    shadow_draw = ImageDraw.Draw(shadow)
    shadow_draw.rounded_rectangle((586, 100, 946, 728), radius=58, fill=(0, 0, 0, 105))
    shadow = shadow.filter(ImageFilter.GaussianBlur(28))
    image.paste(shadow, (0, 0), shadow)
    image.paste(panel, (570, 84), panel)

    center = (765, 385)
    radius = 150
    draw.ellipse((center[0] - radius, center[1] - radius, center[0] + radius, center[1] + radius), outline=(222, 211, 185), width=3)
    draw.ellipse((center[0] - 116, center[1] - 116, center[0] + 116, center[1] + 116), fill=(238, 239, 235), outline=GOLD_LIGHT, width=4)

    for index in range(9):
        angle = -math.pi / 2 + index * math.tau / 9
        x = center[0] + math.cos(angle) * radius
        y = center[1] + math.sin(angle) * radius
        active = type_number is not None and index == type_number - 1
        dot_radius = 12 if active else 7
        fill = GOLD if active else (196, 190, 174)
        draw.ellipse((x - dot_radius, y - dot_radius, x + dot_radius, y + dot_radius), fill=fill)

    if type_number is None:
        draw.text((center[0], center[1] - 38), "九型", anchor="mm", fill=(32, 43, 59), font=font(74))
        draw.text((center[0], center[1] + 54), "看见自己", anchor="mm", fill=(100, 107, 114), font=font(26))
    else:
        avatar = Image.open(AVATARS / f"{type_number}.png").convert("RGBA")
        avatar = ImageOps.fit(avatar, (214, 214), method=Image.Resampling.LANCZOS)
        mask = Image.new("L", avatar.size, 0)
        ImageDraw.Draw(mask).ellipse((0, 0, avatar.width - 1, avatar.height - 1), fill=255)
        image.paste(avatar, (center[0] - 107, center[1] - 107), mask)

    draw.rounded_rectangle((638, 608, 892, 660), radius=26, fill=(233, 222, 196))
    draw.text((765, 633), "测测你是哪一型", anchor="mm", fill=(44, 52, 64), font=font(25))


def make_card(type_number=None):
    image = radial_background()
    draw = ImageDraw.Draw(image)
    draw_brand(draw)
    draw_right_visual(image, draw, type_number)

    if type_number is None:
        draw.text((82, 220), "发现你的", fill=IVORY, font=font(66))
        draw.text((82, 302), "性格能量", fill=GOLD_LIGHT, font=font(66))
        draw.text((86, 412), "理解自己 · 看见关系", fill=MUTED, font=font(29))
        draw.text((86, 458), "找到更清晰的成长方向", fill=MUTED, font=font(29))
    else:
        title, english, keywords = TYPES[type_number]
        draw.text((78, 178), f"{type_number:02d}", fill=GOLD_LIGHT, font=font(146, latin=True))
        draw.rounded_rectangle((86, 360, 132, 368), radius=4, fill=GOLD)
        draw.text((86, 396), title, fill=IVORY, font=font(46))
        draw.text((86, 468), english, fill=GOLD, font=font(23, latin=True))
        draw.text((86, 516), keywords, fill=MUTED, font=font(26))

    draw.text((84, 716), "看见性格，也看见成长方向", fill=(142, 155, 175), font=font(23))
    return image


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    cards = [("default", None)] + [(str(index), index) for index in range(1, 10)]
    for name, type_number in cards:
        output = OUTPUT / f"result-{name}.jpg"
        share_card = make_card(type_number).resize(SHARE_SIZE, Image.Resampling.LANCZOS)
        share_card.save(output, "JPEG", quality=90, optimize=True, progressive=False, subsampling=1)
        print(output.relative_to(ROOT))


if __name__ == "__main__":
    main()
