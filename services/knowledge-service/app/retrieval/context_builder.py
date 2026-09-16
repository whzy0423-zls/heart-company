from app.domain.documents import RetrievedDocument


def build_context(documents: list[RetrievedDocument], *, max_runes: int) -> str:
    if max_runes <= 0:
        return ""
    parts: list[str] = []
    used = 0
    for document in documents:
        locator = ", ".join(f"{key}={value}" for key, value in sorted(document.locator.items()))
        header = f"[{document.source}{'; ' + locator if locator else ''}]\n"
        separator = "\n\n" if parts else ""
        available = max_runes - used - len(separator)
        if available <= 0:
            break
        value = (header + document.content)[:available]
        if value:
            parts.append(value)
            used += len(separator) + len(value)
        if used >= max_runes:
            break
    return "\n\n".join(parts)

