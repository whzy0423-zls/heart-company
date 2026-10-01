#!/usr/bin/env python3
"""Build the stable 5:4 share fallback from the bundled original teacher photo."""
import os
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont, ImageOps

root = Path(__file__).resolve().parents[1]
font_path = next((p for p in [
    os.environ.get('NX_SHARE_FONT_CN'),
    '/System/Library/Fonts/STHeiti Medium.ttc',
    '/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc',
] if p and Path(p).is_file()), None)
if not font_path:
    raise RuntimeError('Set NX_SHARE_FONT_CN to a Chinese font file')

photo = Image.open(root / 'src/static/teacher/hero-portrait.jpg').convert('RGB')
canvas = ImageOps.fit(photo, (1000, 800), centering=(0.5, 0))
draw = ImageDraw.Draw(canvas)
ivory, muted, gold = '#F7F4EB', '#C0BBAF', '#CEB593'
def font(size): return ImageFont.truetype(font_path, size)

logo = Image.open(root / 'src/static/brand/logo.png').convert('RGBA')
logo.thumbnail((62, 62))
canvas.paste(logo, (44, 44), logo)
draw.text((123, 47), '九型芯之力', font=font(36), fill=ivory)
draw.text((47, 218), '懂自己，', font=font(76), fill=ivory)
draw.text((47, 324), '也懂彼此。', font=font(76), fill=ivory)
draw.line((49, 468, 120, 468), fill=gold, width=3)
draw.text((49, 502), '看见自己 · 理解关系', font=font(28), fill=muted)
draw.text((49, 562), '老师日常  /  课程共学', font=font(26), fill=gold)
canvas.resize((500, 400), Image.Resampling.LANCZOS).save(root / 'src/static/share/studio.jpg', quality=92, optimize=True)
