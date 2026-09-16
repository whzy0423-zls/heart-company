import re
from pathlib import Path


_COPY_MARKERS = re.compile(r"(?:[-_ ]?(?:副本|copy|复制|扫描版|文字版|完整版))+$", re.IGNORECASE)
_EDITION_MARKERS = re.compile(r"(?:[-_ ]?(?:第\d+版|修订版|珍藏版))+$", re.IGNORECASE)


def normalized_title(path: Path) -> str:
    title = path.stem.strip().casefold()
    title = _COPY_MARKERS.sub("", title)
    title = _EDITION_MARKERS.sub("", title)
    return re.sub(r"[\s_\-—]+", "", title)

