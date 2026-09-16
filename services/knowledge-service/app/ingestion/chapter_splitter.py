import re

from app.ingestion.metadata import ExtractedSection


HEADING = re.compile(r"^(第[一二三四五六七八九十百零〇0-9]+[章节篇部].*)$")
KIND_PREFIXES = {"定义：": "definition", "案例：": "case", "练习：": "exercise", "警告：": "warning"}


def split_chapters(text: str) -> list[ExtractedSection]:
    title: str | None = None
    result: list[ExtractedSection] = []
    body: list[str] = []

    def flush_body() -> None:
        if body:
            result.append(ExtractedSection("\n".join(body), {}, title=title, kind="body"))
            body.clear()

    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if HEADING.match(line):
            flush_body()
            title = line
            continue
        kind = next((value for prefix, value in KIND_PREFIXES.items() if line.startswith(prefix)), None)
        if kind:
            flush_body()
            result.append(ExtractedSection(line, {}, title=title, kind=kind))
        else:
            body.append(line)
    flush_body()
    return result

